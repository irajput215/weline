package timeline

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// Name is the module name used for routes and permissions.
const Name = "timeline"

// Page is how many items a timeline page holds.
const Page = 100

// Service builds timelines.
type Service struct {
	db        *gorm.DB
	providers provider.Resolver
	now       func() time.Time
}

// NewService builds the service.
func NewService(db *gorm.DB, providers provider.Resolver) *Service {
	return &Service{db: db, providers: providers, now: time.Now}
}

// Contact is who the timeline is about (number masked).
type Contact struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	PhoneMasked string    `json:"phone_masked"`
	CreatedAt   time.Time `json:"created_at"`
}

// Item is one thing that happened. Fields hold short, administrative values
// (types, statuses, names of rules or campaigns), never message text.
type Item struct {
	Kind   string            `json:"kind"`
	At     time.Time         `json:"at"`
	Fields map[string]string `json:"fields"`
}

// Timeline is a page of a contact's history, newest first.
type Timeline struct {
	Contact                 Contact    `json:"contact"`
	Items                   []Item     `json:"items"`
	NextBefore              *time.Time `json:"next_before"`
	AppointmentsUnavailable bool       `json:"appointments_unavailable"`
}

// Item kinds.
const (
	KindMessageIn   = "message_in"
	KindMessageOut  = "message_out"
	KindBroadcast   = "broadcast"
	KindAppointment = "appointment"
	KindReminder    = "reminder"
	KindConsent     = "consent"
	KindAdArrival   = "ad_arrival"
	KindAI          = "ai"
	KindHandover    = "handover"
)

// Mask keeps the first 2 and last 3 digits of a number.
func Mask(phone string) string {
	if len(phone) <= 5 {
		return strings.Repeat("•", len(phone))
	}
	return phone[:2] + strings.Repeat("•", len(phone)-5) + phone[len(phone)-3:]
}

type contactRow struct {
	ID          uuid.UUID
	ProfileName string
	PhoneNumber string
	CreatedAt   time.Time
}

func (s *Service) contact(ctx context.Context, orgID, id uuid.UUID) (*contactRow, error) {
	var rows []contactRow
	if err := s.db.WithContext(ctx).Raw(`SELECT id, profile_name, phone_number, created_at FROM contacts
		WHERE organization_id = ? AND id = ? AND deleted_at IS NULL`, orgID, id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, kit.NotFound("contact not found")
	}
	return &rows[0], nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Search finds contacts by name or number (at most 20, numbers masked).
func (s *Service) Search(ctx context.Context, orgID uuid.UUID, q string) ([]Contact, error) {
	q = strings.TrimSpace(q)
	if len([]rune(q)) < 2 || len(q) > 100 {
		return nil, kit.Invalid("type at least 2 characters (at most 100)")
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, q)
	if strings.HasPrefix(digits, "0") && len(digits) == 10 {
		digits = "61" + digits[1:]
	}
	sql := `SELECT id, profile_name, phone_number, created_at FROM contacts WHERE organization_id = ? AND deleted_at IS NULL
		AND (profile_name ILIKE ?`
	args := []any{orgID, "%" + escapeLike(q) + "%"}
	if len(digits) >= 4 {
		sql += ` OR phone_number LIKE ?`
		args = append(args, "%"+digits+"%")
	}
	sql += `) ORDER BY profile_name, created_at DESC LIMIT 20`
	var rows []contactRow
	if err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Contact, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contact{ID: r.ID, Name: r.ProfileName, PhoneMasked: Mask(r.PhoneNumber), CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// source is one query feeding the timeline; every query filters by the
// organization and the contact.
type source struct {
	kind string
	sql  string
	cols []string // names of the item's fields, in the order c0, c1, …
}

var sources = []source{
	{kind: "message", sql: `SELECT created_at AS at, direction AS c0, message_type AS c1, status AS c2 FROM messages
		WHERE organization_id = ? AND contact_id = ? AND deleted_at IS NULL AND created_at < ? ORDER BY created_at DESC LIMIT ?`,
		cols: []string{"direction", "type", "status"}},
	{kind: KindBroadcast, sql: `SELECT COALESCE(r.sent_at, r.created_at) AS at, c.name AS c0, r.status AS c1 FROM bulk_message_recipients r
		JOIN bulk_message_campaigns c ON c.id = r.campaign_id AND c.organization_id = ? AND c.deleted_at IS NULL
		WHERE r.phone_number = ? AND r.deleted_at IS NULL AND COALESCE(r.sent_at, r.created_at) < ? ORDER BY 1 DESC LIMIT ?`,
		cols: []string{"campaign", "status"}},
	{kind: KindReminder, sql: `SELECT s.created_at AS at, r.name AS c0, r.kind AS c1, s.status AS c2, s.reason AS c3, s.reply AS c4
		FROM wl.reminder_sends s JOIN wl.reminder_rules r ON r.id = s.rule_id AND r.organization_id = s.organization_id
		WHERE s.organization_id = ? AND s.contact_id = ? AND s.created_at < ? ORDER BY s.created_at DESC LIMIT ?`,
		cols: []string{"rule", "rule_kind", "status", "reason", "reply"}},
	{kind: KindConsent, sql: `SELECT given_at AS at, action AS c0, source AS c1 FROM wl.consent_events
		WHERE organization_id = ? AND contact_id = ? AND given_at < ? ORDER BY given_at DESC LIMIT ?`,
		cols: []string{"action", "source"}},
	{kind: KindAdArrival, sql: `SELECT a.arrived_at AS at, e.name AS c0, e.channel AS c1 FROM wl.ads_arrivals a
		JOIN wl.ads_entrypoints e ON e.id = a.entrypoint_id AND e.organization_id = a.organization_id
		WHERE a.organization_id = ? AND a.contact_id = ? AND a.arrived_at < ? ORDER BY a.arrived_at DESC LIMIT ?`,
		cols: []string{"entry_point", "channel"}},
	{kind: KindAI, sql: `SELECT created_at AS at, intent AS c0, outcome AS c1 FROM wl.ai_runs
		WHERE organization_id = ? AND contact_id = ? AND created_at < ? ORDER BY created_at DESC LIMIT ?`,
		cols: []string{"intent", "outcome"}},
	{kind: KindHandover, sql: `SELECT transferred_at AS at, status AS c0, source AS c1 FROM agent_transfers
		WHERE organization_id = ? AND contact_id = ? AND deleted_at IS NULL AND transferred_at < ? ORDER BY transferred_at DESC LIMIT ?`,
		cols: []string{"status", "source"}},
}

type row struct {
	At                 time.Time
	C0, C1, C2, C3, C4 *string
}

func (r row) values() []*string { return []*string{r.C0, r.C1, r.C2, r.C3, r.C4} }

// Get returns a page of the contact's timeline, newest first, before a time.
func (s *Service) Get(ctx context.Context, orgID, contactID uuid.UUID, before *time.Time) (*Timeline, error) {
	c, err := s.contact(ctx, orgID, contactID)
	if err != nil {
		return nil, err
	}
	until := s.now().AddDate(1, 0, 0) // include upcoming appointments on the first page
	if before != nil {
		until = *before
	}
	out := &Timeline{Contact: Contact{ID: c.ID, Name: c.ProfileName, PhoneMasked: Mask(c.PhoneNumber), CreatedAt: c.CreatedAt}}
	for _, src := range sources {
		var args []any
		if src.kind == KindBroadcast {
			args = []any{orgID, c.PhoneNumber, until, Page}
		} else {
			args = []any{orgID, c.ID, until, Page}
		}
		var rows []row
		if err := s.db.WithContext(ctx).Raw(src.sql, args...).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			it := Item{Kind: src.kind, At: r.At.UTC(), Fields: map[string]string{}}
			for i, col := range src.cols {
				if v := r.values()[i]; v != nil && *v != "" {
					it.Fields[col] = *v
				}
			}
			if src.kind == "message" {
				it.Kind = KindMessageOut
				if it.Fields["direction"] == "incoming" {
					it.Kind = KindMessageIn
				}
				delete(it.Fields, "direction")
			}
			out.Items = append(out.Items, it)
		}
	}
	appts, err := s.appointments(ctx, orgID, c, until)
	if err != nil {
		out.AppointmentsUnavailable = true
	}
	out.Items = append(out.Items, appts...)
	sort.SliceStable(out.Items, func(i, j int) bool { return out.Items[i].At.After(out.Items[j].At) })
	if len(out.Items) > Page {
		out.Items = out.Items[:Page]
		next := out.Items[Page-1].At
		out.NextBefore = &next
	}
	if out.Items == nil {
		out.Items = []Item{}
	}
	return out, nil
}

// appointments reads the contact's appointments from the connected system,
// from a year ago to a year ahead (never creating a customer there).
func (s *Service) appointments(ctx context.Context, orgID uuid.UUID, c *contactRow, until time.Time) ([]Item, error) {
	if s.providers == nil {
		return nil, nil
	}
	p, err := s.providers.For(ctx, orgID)
	if err != nil {
		return nil, err
	}
	in := provider.CustomerInput{ContactID: c.ID, Name: c.ProfileName, Mobile: c.PhoneNumber}
	var cust *provider.Customer
	if f, ok := p.(provider.Finder); ok {
		cust, err = f.FindCustomer(ctx, in)
	} else {
		cust, _, err = p.FindOrCreateCustomer(ctx, in)
	}
	if err != nil {
		if kit.StatusOf(err) == 404 {
			return nil, nil // not a customer there: no appointments
		}
		return nil, err
	}
	from := s.now().AddDate(-1, 0, 0)
	to := until
	if max := s.now().AddDate(1, 0, 0); to.After(max) {
		to = max
	}
	if !to.After(from) {
		return nil, nil
	}
	list, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to, CustomerID: cust.ID})
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(list))
	for _, a := range list {
		items = append(items, Item{Kind: KindAppointment, At: a.Start.UTC(), Fields: map[string]string{"service": a.ServiceName,
			"practitioner": a.PractitionerName, "location": a.LocationName, "status": a.Status, "time_zone": a.TimeZone}})
	}
	return items, nil
}
