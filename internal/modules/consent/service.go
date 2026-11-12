package consent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Actions, sources and states (data model, wl.consent_events).
const (
	ActionOptIn  = "opt_in"
	ActionOptOut = "opt_out"

	SourceKeyword = "keyword"
	SourceButton  = "button"
	SourceStaff   = "staff"
	SourceImport  = "import"
	SourceMeta    = "meta"

	OptedIn     = "opted_in"
	OptedOut    = "opted_out"
	NotRecorded = "not_recorded"

	maxEvidence = 500
)

// Logger is the part of Weline's logger the module uses.
type Logger interface {
	Info(msg string, fields ...any)
	Error(msg string, fields ...any)
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// Service holds the consent rules. Every method takes the organization first and
// filters every query by it.
type Service struct {
	db     *gorm.DB
	rdb    *redis.Client // imports and the sweep lock; nil disables both
	sender kit.Sender    // confirmation replies; nil sends none
	log    Logger
	cache  settingsCache
	now    func() time.Time
}

// NewService returns the consent service.
func NewService(db *gorm.DB, rdb *redis.Client, sender kit.Sender, log Logger) *Service {
	if log == nil {
		log = nopLogger{}
	}
	return &Service{db: db, rdb: rdb, sender: sender, log: log, now: time.Now}
}

// Event is one recorded change, as the history shows it.
type Event struct {
	ID         uuid.UUID  `json:"id"`
	Contact    ContactRef `json:"contact"`
	Action     string     `json:"action"`
	Source     string     `json:"source"`
	Previous   *string    `json:"previous"`
	GivenAt    time.Time  `json:"given_at"`
	RecordedAt time.Time  `json:"recorded_at"`
	RecordedBy *UserRef   `json:"recorded_by"`
	Keyword    string     `json:"keyword"`
	Evidence   string     `json:"evidence"`
	ImportID   *uuid.UUID `json:"import_id"`
}

// ContactRef is a contact with a masked number.
type ContactRef struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	PhoneMasked string    `json:"phone_masked"`
}

// UserRef names a staff member.
type UserRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// eventRow is wl.consent_events.
type eventRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	ContactID      uuid.UUID `gorm:"type:uuid"`
	Action         string
	Source         string
	Previous       *string
	GivenAt        time.Time
	RecordedAt     time.Time
	RecordedByID   *uuid.UUID `gorm:"type:uuid"`
	MessageID      *uuid.UUID `gorm:"type:uuid"`
	Keyword        string
	Evidence       string
	ImportID       *uuid.UUID `gorm:"type:uuid"`
}

func (eventRow) TableName() string { return "wl.consent_events" }

// statusRow is wl.consent_status.
type statusRow struct {
	OrganizationID uuid.UUID `gorm:"type:uuid;primaryKey"`
	ContactID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	Marketing      string
	Source         string
	GivenAt        time.Time
	LastEventID    uuid.UUID `gorm:"type:uuid"`
	UpdatedAt      time.Time
}

func (statusRow) TableName() string { return "wl.consent_status" }

var nonDigits = regexp.MustCompile(`\D`)

func maskPhone(p string) string {
	d := nonDigits.ReplaceAllString(p, "")
	if len(d) <= 5 {
		return strings.Repeat("•", len(d))
	}
	return d[:2] + strings.Repeat("•", len(d)-5) + d[len(d)-3:]
}

// ---------- changing state ----------

// Change is one opt-in or opt-out from any source (design.md, "Changing state").
type Change struct {
	Action     string
	Source     string
	GivenAt    time.Time
	Evidence   string
	Keyword    string
	MessageID  *uuid.UUID
	RecordedBy *uuid.UUID
	ImportID   *uuid.UUID
}

// Result says what Apply did.
type Result struct {
	Changed   bool      // the state changed
	Recorded  bool      // an event was written (also for extra proof on an unchanged opt-in)
	Marketing string    // the state after
	EventID   uuid.UUID // when Recorded
	Skipped   string    // "opted_out_later" for an import row older than an opt-out
}

var errOptedOutLater = errors.New("opted out after this opt-in")

func target(action string) string {
	if action == ActionOptOut {
		return OptedOut
	}
	return OptedIn
}

type lockedContact struct {
	ID              uuid.UUID
	Tags            models.JSONBArray `gorm:"type:jsonb"`
	MarketingOptOut bool
}

// withTag adds or removes a tag (case-insensitive), keeping the others.
func withTag(tags models.JSONBArray, tag string, add bool) models.JSONBArray {
	out := models.JSONBArray{}
	found := false
	for _, t := range tags {
		s, _ := t.(string)
		if strings.EqualFold(s, tag) {
			found = true
			if !add {
				continue
			}
		}
		out = append(out, t)
	}
	if add && !found {
		out = append(out, tag)
	}
	return out
}

// Apply records a change for a contact and keeps Weline's marketing_opt_out
// flag and the opt-out tag in step, in one transaction.
func (s *Service) Apply(ctx context.Context, orgID, contactID uuid.UUID, ch Change) (*Result, error) {
	if ch.Action != ActionOptIn && ch.Action != ActionOptOut {
		return nil, kit.Invalid("action must be %s or %s", ActionOptIn, ActionOptOut)
	}
	return s.apply(ctx, orgID, contactID, &ch)
}

// reconcile records a change Meta made to the flag that our status doesn't know
// about yet (the sweep, R4). Result.Recorded says whether there was one.
func (s *Service) reconcile(ctx context.Context, orgID, contactID uuid.UUID) (*Result, error) {
	return s.apply(ctx, orgID, contactID, nil)
}

func (s *Service) apply(ctx context.Context, orgID, contactID uuid.UUID, ch *Change) (*Result, error) {
	st, err := s.Settings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if ch != nil && ch.GivenAt.IsZero() {
		ch.GivenAt = s.now()
	}
	res := &Result{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cs []lockedContact
		if err := tx.Raw(`SELECT id, tags, marketing_opt_out FROM contacts
			WHERE organization_id = ? AND id = ? AND deleted_at IS NULL FOR UPDATE`, orgID, contactID).Scan(&cs).Error; err != nil {
			return fmt.Errorf("lock contact: %w", err)
		}
		if len(cs) == 0 {
			return kit.NotFound("contact not found")
		}
		c := cs[0]
		var cur []statusRow
		if err := tx.Where("organization_id = ? AND contact_id = ?", orgID, contactID).Limit(1).Find(&cur).Error; err != nil {
			return fmt.Errorf("load status: %w", err)
		}
		var previous *string
		if len(cur) == 1 {
			p := cur[0].Marketing
			previous = &p
		}

		// Meta changed the flag and we haven't recorded it yet: record that first,
		// so the history has it before whatever comes next.
		statusOut := previous != nil && *previous == OptedOut
		if c.MarketingOptOut != statusOut {
			action := ActionOptIn
			if c.MarketingOptOut {
				action = ActionOptOut
			}
			id, err := s.write(tx, orgID, &c, Change{Action: action, Source: SourceMeta, GivenAt: s.now()}, previous, st.OptOutTag, true)
			if err != nil {
				return err
			}
			p := target(action)
			previous = &p
			res.Changed, res.Recorded, res.EventID, res.Marketing = true, true, id, p
		}
		if ch == nil {
			return nil
		}
		want := target(ch.Action)
		res.Marketing = want
		if ch.Source == SourceImport && ch.Action == ActionOptIn {
			var later int64
			if err := tx.Model(&eventRow{}).Where("organization_id = ? AND contact_id = ? AND action = ? AND given_at > ?",
				orgID, contactID, ActionOptOut, ch.GivenAt).Count(&later).Error; err != nil {
				return fmt.Errorf("check later opt-out: %w", err)
			}
			if later > 0 {
				return errOptedOutLater
			}
		}
		same := previous != nil && *previous == want
		extraProof := same && ch.Action == ActionOptIn && ch.Evidence != "" && (ch.Source == SourceStaff || ch.Source == SourceImport)
		if same && !extraProof {
			res.Changed, res.Recorded = false, false
			return nil
		}
		id, err := s.write(tx, orgID, &c, *ch, previous, st.OptOutTag, !same)
		if err != nil {
			return err
		}
		res.Changed, res.Recorded, res.EventID = !same, true, id
		return nil
	})
	if errors.Is(err, errOptedOutLater) {
		return &Result{Marketing: OptedOut, Skipped: "opted_out_later"}, nil
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

// write inserts the event and, when the state changes, the status row and the
// contact's flag and tag.
func (s *Service) write(tx *gorm.DB, orgID uuid.UUID, c *lockedContact, ch Change, previous *string, tag string, changes bool) (uuid.UUID, error) {
	ev := eventRow{OrganizationID: orgID, ContactID: c.ID, Action: ch.Action, Source: ch.Source, Previous: previous,
		GivenAt: ch.GivenAt, RecordedAt: s.now(), RecordedByID: ch.RecordedBy, MessageID: ch.MessageID,
		Keyword: ch.Keyword, Evidence: ch.Evidence, ImportID: ch.ImportID}
	if err := tx.Create(&ev).Error; err != nil {
		return uuid.Nil, fmt.Errorf("record consent: %w", err)
	}
	if !changes {
		return ev.ID, nil
	}
	state := target(ch.Action)
	row := statusRow{OrganizationID: orgID, ContactID: c.ID, Marketing: state, Source: ch.Source, GivenAt: ch.GivenAt,
		LastEventID: ev.ID, UpdatedAt: s.now()}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "contact_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"marketing", "source", "given_at", "last_event_id", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return uuid.Nil, fmt.Errorf("save status: %w", err)
	}
	if err := s.syncContact(tx, orgID, *c, state, tag); err != nil {
		return uuid.Nil, err
	}
	c.MarketingOptOut = state == OptedOut
	c.Tags = withTag(c.Tags, tag, c.MarketingOptOut)
	return ev.ID, nil
}

// syncContact sets Weline's flag and tag through upstream's model, so its
// campaign worker and template send refuse marketing to opted-out contacts.
func (s *Service) syncContact(tx *gorm.DB, orgID uuid.UUID, c lockedContact, state, tag string) error {
	out := state == OptedOut
	err := tx.Model(&models.Contact{}).Where("organization_id = ? AND id = ?", orgID, c.ID).
		Updates(map[string]any{"marketing_opt_out": out, "tags": withTag(c.Tags, tag, out)}).Error
	if err != nil {
		return fmt.Errorf("update contact: %w", err)
	}
	return nil
}

// ---------- checks for other modules (R9, R10) ----------

// Purpose is what a module wants to send.
type Purpose string

const (
	PurposeMarketing Purpose = "marketing"
	PurposeService   Purpose = "service"
)

// Reasons Allowed refuses.
const (
	ReasonOptedOut = "opted_out"
	ReasonNoOptIn  = "no_opt_in"
)

// Allowed reports whether a module may send this kind of message to the contact
// now, and why not.
func (s *Service) Allowed(ctx context.Context, orgID, contactID uuid.UUID, purpose Purpose) (bool, string, error) {
	st, err := s.Settings(ctx, orgID)
	if err != nil {
		return false, "", err
	}
	var flags []struct {
		MarketingOptOut bool
		Marketing       *string
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT c.marketing_opt_out, cs.marketing FROM contacts c
		LEFT JOIN wl.consent_status cs ON cs.organization_id = c.organization_id AND cs.contact_id = c.id
		WHERE c.organization_id = ? AND c.id = ? AND c.deleted_at IS NULL`, orgID, contactID).Scan(&flags).Error; err != nil {
		return false, "", fmt.Errorf("load consent: %w", err)
	}
	if len(flags) == 0 {
		return false, "", kit.NotFound("contact not found")
	}
	f := flags[0]
	out := f.MarketingOptOut || (f.Marketing != nil && *f.Marketing == OptedOut)
	switch purpose {
	case PurposeMarketing:
		if out {
			return false, ReasonOptedOut, nil
		}
		if st.RequireOptIn && (f.Marketing == nil || *f.Marketing != OptedIn) {
			return false, ReasonNoOptIn, nil
		}
		return true, "", nil
	case PurposeService:
		if out && st.ServiceAfterStop == ServiceStop {
			return false, ReasonOptedOut, nil
		}
		return true, "", nil
	}
	return false, "", kit.Invalid("unknown purpose %q", purpose)
}

// ---------- reading ----------

// Summary counts contacts by consent (R12).
type Summary struct {
	OptedIn       int  `json:"opted_in"`
	OptedOut      int  `json:"opted_out"`
	NotRecorded   int  `json:"not_recorded"`
	TotalContacts int  `json:"total_contacts"`
	RequireOptIn  bool `json:"require_opt_in"`
}

// Summary returns the organization's consent counts.
func (s *Service) Summary(ctx context.Context, orgID uuid.UUID) (*Summary, error) {
	st, err := s.Settings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var row struct{ Total, In, Out int }
	if err := s.db.WithContext(ctx).Raw(`SELECT count(*) AS total,
			count(*) FILTER (WHERE cs.marketing = 'opted_in' AND NOT c.marketing_opt_out) AS "in",
			count(*) FILTER (WHERE cs.marketing = 'opted_out' OR c.marketing_opt_out) AS "out"
		FROM contacts c LEFT JOIN wl.consent_status cs ON cs.organization_id = c.organization_id AND cs.contact_id = c.id
		WHERE c.organization_id = ? AND c.deleted_at IS NULL`, orgID).Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("count consent: %w", err)
	}
	return &Summary{OptedIn: row.In, OptedOut: row.Out, NotRecorded: row.Total - row.In - row.Out, TotalContacts: row.Total,
		RequireOptIn: st.RequireOptIn}, nil
}

// EventFilter narrows the history.
type EventFilter struct {
	Action    string
	Source    string
	From, To  *time.Time
	ContactID *uuid.UUID
	Before    *time.Time
	Limit     int
}

var sources = map[string]bool{SourceKeyword: true, SourceButton: true, SourceStaff: true, SourceImport: true, SourceMeta: true, "ad": true, "web_form": true}

// eventView is a history row with names. Flat on purpose: GORM's Scan doesn't
// fill an embedded struct from a raw select.
type eventView struct {
	ID           uuid.UUID
	ContactID    uuid.UUID
	Action       string
	Source       string
	Previous     *string
	GivenAt      time.Time
	RecordedAt   time.Time
	RecordedByID *uuid.UUID
	Keyword      string
	Evidence     string
	ImportID     *uuid.UUID
	ContactName  string
	ContactPhone string
	UserName     *string
}

// Events returns the history, newest first (R11, R12).
func (s *Service) Events(ctx context.Context, orgID uuid.UUID, f EventFilter) ([]Event, error) {
	if f.Action != "" && f.Action != ActionOptIn && f.Action != ActionOptOut {
		return nil, kit.Invalid("action must be %s or %s", ActionOptIn, ActionOptOut)
	}
	if f.Source != "" && !sources[f.Source] {
		return nil, kit.Invalid("unknown source %q", f.Source)
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	q := s.db.WithContext(ctx).Table("wl.consent_events e").
		Select(`e.*, COALESCE(c.profile_name, '') AS contact_name, COALESCE(c.phone_number, '') AS contact_phone, u.full_name AS user_name`).
		Joins("LEFT JOIN contacts c ON c.id = e.contact_id AND c.organization_id = e.organization_id").
		Joins(`LEFT JOIN users u ON u.id = e.recorded_by_id AND (u.organization_id = e.organization_id OR EXISTS
			(SELECT 1 FROM user_organizations uo WHERE uo.user_id = u.id AND uo.organization_id = e.organization_id AND uo.deleted_at IS NULL))`).
		Where("e.organization_id = ?", orgID)
	if f.Action != "" {
		q = q.Where("e.action = ?", f.Action)
	}
	if f.Source != "" {
		q = q.Where("e.source = ?", f.Source)
	}
	if f.From != nil {
		q = q.Where("e.recorded_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("e.recorded_at < ?", *f.To)
	}
	if f.ContactID != nil {
		q = q.Where("e.contact_id = ?", *f.ContactID)
	}
	if f.Before != nil {
		q = q.Where("e.recorded_at < ?", *f.Before)
	}
	var rows []eventView
	if err := q.Order("e.recorded_at DESC, e.id").Limit(f.Limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list consent events: %w", err)
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		ev := Event{ID: r.ID, Contact: ContactRef{ID: r.ContactID, Name: r.ContactName, PhoneMasked: maskPhone(r.ContactPhone)},
			Action: r.Action, Source: r.Source, Previous: r.Previous, GivenAt: r.GivenAt, RecordedAt: r.RecordedAt,
			Keyword: r.Keyword, Evidence: r.Evidence, ImportID: r.ImportID}
		if r.RecordedByID != nil {
			name := ""
			if r.UserName != nil {
				name = *r.UserName
			}
			ev.RecordedBy = &UserRef{ID: *r.RecordedByID, Name: name}
		}
		out = append(out, ev)
	}
	return out, nil
}

// ContactConsent is one contact's state and history.
type ContactConsent struct {
	Contact   ContactRef `json:"contact"`
	Marketing string     `json:"marketing"`
	Source    *string    `json:"source"`
	GivenAt   *time.Time `json:"given_at"`
	Events    []Event    `json:"events"`
}

type contactState struct {
	ID              uuid.UUID
	ProfileName     string
	PhoneNumber     string
	MarketingOptOut bool
	Marketing       *string
	Source          *string
	GivenAt         *time.Time
}

func (c contactState) state() string {
	switch {
	case c.MarketingOptOut || (c.Marketing != nil && *c.Marketing == OptedOut):
		return OptedOut
	case c.Marketing != nil:
		return *c.Marketing
	}
	return NotRecorded
}

const contactStateSQL = `SELECT c.id, c.profile_name, c.phone_number, c.marketing_opt_out, cs.marketing, cs.source, cs.given_at
	FROM contacts c LEFT JOIN wl.consent_status cs ON cs.organization_id = c.organization_id AND cs.contact_id = c.id
	WHERE c.organization_id = ? AND c.deleted_at IS NULL`

// Contact returns one contact's consent and history.
func (s *Service) Contact(ctx context.Context, orgID, contactID uuid.UUID) (*ContactConsent, error) {
	var rows []contactState
	if err := s.db.WithContext(ctx).Raw(contactStateSQL+" AND c.id = ?", orgID, contactID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load contact: %w", err)
	}
	if len(rows) == 0 {
		return nil, kit.NotFound("contact not found")
	}
	c := rows[0]
	evs, err := s.Events(ctx, orgID, EventFilter{ContactID: &contactID, Limit: 100})
	if err != nil {
		return nil, err
	}
	return &ContactConsent{Contact: ContactRef{ID: c.ID, Name: c.ProfileName, PhoneMasked: maskPhone(c.PhoneNumber)},
		Marketing: c.state(), Source: c.Source, GivenAt: c.GivenAt, Events: evs}, nil
}

// FoundContact is a search result.
type FoundContact struct {
	ContactRef
	Marketing string `json:"marketing"`
}

// Search finds contacts by name or number (POST body, never a URL).
func (s *Service) Search(ctx context.Context, orgID uuid.UUID, q string) ([]FoundContact, error) {
	q = strings.TrimSpace(q)
	if len([]rune(q)) < 2 {
		return nil, kit.Invalid("type at least 2 characters")
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
	digits := nonDigits.ReplaceAllString(q, "")
	sql := contactStateSQL + " AND (c.profile_name ILIKE ?"
	args := []any{orgID, like}
	if len(digits) >= 3 {
		digits = strings.TrimPrefix(digits, "0") // 0412… is stored as 61412…
		sql += " OR regexp_replace(c.phone_number, '[^0-9]', '', 'g') LIKE ?"
		args = append(args, "%"+digits+"%")
	}
	sql += ") ORDER BY c.profile_name, c.id LIMIT 20"
	var rows []contactState
	if err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("search contacts: %w", err)
	}
	out := make([]FoundContact, 0, len(rows))
	for _, c := range rows {
		out = append(out, FoundContact{ContactRef: ContactRef{ID: c.ID, Name: c.ProfileName, PhoneMasked: maskPhone(c.PhoneNumber)}, Marketing: c.state()})
	}
	return out, nil
}

// ---------- staff records (R6) ----------

// StaffInput is a change recorded by staff.
type StaffInput struct {
	Action   string `json:"action"`
	GivenAt  string `json:"given_at"` // YYYY-MM-DD, default today
	Evidence string `json:"evidence"`
}

// parseGivenAt reads a date given (today = now; earlier = midday UTC), not in the
// future and not more than 10 years ago.
func (s *Service) parseGivenAt(v string) (time.Time, error) {
	now := s.now().UTC()
	if strings.TrimSpace(v) == "" {
		return now, nil
	}
	d, err := time.Parse("2006-01-02", strings.TrimSpace(v))
	if err != nil {
		return time.Time{}, kit.Invalid("given_at: use a date like 2026-03-01")
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch {
	case d.After(today.Add(24 * time.Hour)):
		return time.Time{}, kit.Invalid("given_at: can't be in the future")
	case d.Before(today.AddDate(-10, 0, 0)):
		return time.Time{}, kit.Invalid("given_at: more than 10 years ago")
	case !d.Before(today):
		return now, nil
	}
	return d.Add(12 * time.Hour), nil
}

// Record saves a change staff were told about (desk, phone, paper form).
func (s *Service) Record(ctx context.Context, orgID, userID, contactID uuid.UUID, in StaffInput) (*Result, error) {
	if in.Action != ActionOptIn && in.Action != ActionOptOut {
		return nil, kit.Invalid("action must be %s or %s", ActionOptIn, ActionOptOut)
	}
	ev := strings.TrimSpace(in.Evidence)
	if in.Action == ActionOptIn && ev == "" {
		return nil, kit.Invalid("evidence: say how the person agreed (for example, signed new patient form)")
	}
	if len([]rune(ev)) > maxEvidence {
		return nil, kit.Invalid("evidence: at most %d characters", maxEvidence)
	}
	given, err := s.parseGivenAt(in.GivenAt)
	if err != nil {
		return nil, err
	}
	return s.Apply(ctx, orgID, contactID, Change{Action: in.Action, Source: SourceStaff, GivenAt: given, Evidence: ev, RecordedBy: &userID})
}
