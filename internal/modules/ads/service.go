package ads

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Channels an entry point can be (R1).
var channels = map[string]bool{"meta_ad": true, "google_ad": true, "website": true, "print": true, "email": true, "other": true}

const (
	maxName       = 100
	maxCampaign   = 60
	maxPrefill    = 500
	maxTag        = 50
	maxMatchRunes = 1000
	repeatWindow  = 24 * time.Hour   // one arrival per contact and entry point (R5)
	newWindow     = 10 * time.Minute // first message at most this long before (R6, Q3)
	creditWindow  = 30 * 24 * time.Hour
	codeCacheTTL  = time.Minute
	pageSize      = 50
)

var (
	codeRe   = regexp.MustCompile(`^[A-Z0-9]{3,12}$`)
	tagRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,50}$`)
	numberRe = regexp.MustCompile(`^[1-9][0-9]{7,14}$`)
	slugRe   = regexp.MustCompile(`[^a-z0-9]+`)
	nonDigit = regexp.MustCompile(`\D`)
)

// Logger is the part of Weline's logger the module uses.
type Logger interface {
	Info(msg string, fields ...any)
	Error(msg string, fields ...any)
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// Service holds the ads rules. Every method takes the organization first and
// filters every query by it.
type Service struct {
	db    *gorm.DB
	log   Logger
	codes codeCache
	now   func() time.Time
}

// NewService returns the ads service.
func NewService(db *gorm.DB, log Logger) *Service {
	if log == nil {
		log = nopLogger{}
	}
	return &Service{db: db, log: log, now: time.Now}
}

// ---------- rows ----------

type entryRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	Name           string
	Code           string
	Channel        string
	Campaign       string
	WaNumber       string
	PrefillText    string
	Tag            string
	TagCampaign    bool
	ArchivedAt     *time.Time
	CreatedByID    *uuid.UUID `gorm:"type:uuid"`
	UpdatedByID    *uuid.UUID `gorm:"type:uuid"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (entryRow) TableName() string { return "wl.ads_entrypoints" }

type arrivalRow struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID  `gorm:"type:uuid"`
	EntrypointID   uuid.UUID  `gorm:"type:uuid"`
	ContactID      uuid.UUID  `gorm:"type:uuid"`
	MessageID      *uuid.UUID `gorm:"type:uuid"`
	Code           string
	ArrivedAt      time.Time
	IsNew          bool
}

func (arrivalRow) TableName() string { return "wl.ads_arrivals" }

// ---------- views ----------

// Stats are an entry point's counts for a period (R9).
type Stats struct {
	Conversations int `json:"conversations"`
	NewContacts   int `json:"new_contacts"`
	OptedIn       int `json:"opted_in"`
	Booked        int `json:"booked"`
}

func (s *Stats) add(o Stats) {
	s.Conversations += o.Conversations
	s.NewContacts += o.NewContacts
	s.OptedIn += o.OptedIn
	s.Booked += o.Booked
}

// EntryPoint is an entry point as the API returns it.
type EntryPoint struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Code        string     `json:"code"`
	Channel     string     `json:"channel"`
	Campaign    string     `json:"campaign"`
	WaNumber    string     `json:"wa_number"`
	PrefillText string     `json:"prefill_text"`
	Link        string     `json:"link"`
	Tag         string     `json:"tag"`
	TagCampaign bool       `json:"tag_campaign"`
	CampaignTag string     `json:"campaign_tag"`
	ArchivedAt  *time.Time `json:"archived_at"`
	Stats       Stats      `json:"stats"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Link builds the wa.me link that opens a chat with the message typed (R2).
// Spaces become %20, not +, which WhatsApp would show literally.
func Link(number, text string) string {
	return "https://wa.me/" + number + "?text=" + strings.ReplaceAll(url.QueryEscape(text), "+", "%20")
}

// campaignTag is the tag for a campaign label: "campaign-" plus its slug.
func campaignTag(campaign string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(campaign), "-"), "-")
	if slug == "" {
		return ""
	}
	t := "campaign-" + slug
	if len(t) > maxTag {
		t = strings.TrimRight(t[:maxTag], "-")
	}
	return t
}

func (r entryRow) view(st Stats) EntryPoint {
	ct := ""
	if r.TagCampaign {
		ct = campaignTag(r.Campaign)
	}
	return EntryPoint{ID: r.ID, Name: r.Name, Code: r.Code, Channel: r.Channel, Campaign: r.Campaign, WaNumber: r.WaNumber,
		PrefillText: r.PrefillText, Link: Link(r.WaNumber, r.PrefillText), Tag: r.Tag, TagCampaign: r.TagCampaign,
		CampaignTag: ct, ArchivedAt: r.ArchivedAt, Stats: st, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// ---------- codes ----------

// tokens splits text into upper-case runs of letters and digits.
func tokens(text string) []string {
	return strings.FieldsFunc(strings.ToUpper(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// firstCode returns the first token of text that is in codes (R4).
func firstCode[V any](text string, codes map[string]V) (string, bool) {
	for _, t := range tokens(text) {
		if _, ok := codes[t]; ok {
			return t, true
		}
	}
	return "", false
}

// normaliseCode upper-cases and checks a code (R1): 3-12 letters and digits
// with at least one of each.
func normaliseCode(c string) (string, error) {
	c = strings.ToUpper(strings.TrimSpace(c))
	if !codeRe.MatchString(c) || !strings.ContainsAny(c, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") || !strings.ContainsAny(c, "0123456789") {
		return "", kit.Invalid("code: 3 to 12 letters and digits, with at least one of each (for example FLU24)")
	}
	return c, nil
}

// normaliseNumber keeps the digits of a WhatsApp number with country code.
func normaliseNumber(n string) (string, error) {
	n = strings.TrimSpace(n)
	if strings.ContainsFunc(n, func(r rune) bool { return !strings.ContainsRune("0123456789 +-()", r) }) {
		return "", kit.Invalid("wa_number: digits only, with the country code (for example 61 400 000 000)")
	}
	d := nonDigit.ReplaceAllString(n, "")
	if strings.HasPrefix(d, "0") {
		return "", kit.Invalid("wa_number: start with the country code, not 0 (for example 61 for Australia)")
	}
	if !numberRe.MatchString(d) {
		return "", kit.Invalid("wa_number: 8 to 15 digits with the country code")
	}
	return d, nil
}

// ---------- create and change ----------

// Input is an entry point as staff fill it in. Code is used on create only.
type Input struct {
	Name        string `json:"name"`
	Code        string `json:"code"`
	Channel     string `json:"channel"`
	Campaign    string `json:"campaign"`
	WaNumber    string `json:"wa_number"`
	PrefillText string `json:"prefill_text"`
	Tag         string `json:"tag"`
	TagCampaign *bool  `json:"tag_campaign"`
}

// check validates everything but the code against the entry point's code.
func (in *Input) check(code string) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > maxName {
		return kit.Invalid("name: 1 to %d characters", maxName)
	}
	if !channels[in.Channel] {
		return kit.Invalid("channel: one of meta_ad, google_ad, website, print, email, other")
	}
	in.Campaign = strings.Join(strings.Fields(in.Campaign), " ")
	if len([]rune(in.Campaign)) > maxCampaign {
		return kit.Invalid("campaign: at most %d characters", maxCampaign)
	}
	n, err := normaliseNumber(in.WaNumber)
	if err != nil {
		return err
	}
	in.WaNumber = n
	in.PrefillText = strings.TrimSpace(in.PrefillText)
	if in.PrefillText == "" || len([]rune(in.PrefillText)) > maxPrefill {
		return kit.Invalid("prefill_text: 1 to %d characters", maxPrefill)
	}
	if _, ok := firstCode(in.PrefillText, map[string]bool{code: true}); !ok {
		return kit.Invalid("prefill_text: must contain the code %s", code)
	}
	in.Tag = strings.TrimSpace(in.Tag)
	if in.Tag == "" {
		in.Tag = "from-" + strings.ToLower(code)
	}
	if !tagRe.MatchString(in.Tag) {
		return kit.Invalid("tag: up to %d letters, digits, - and _", maxTag)
	}
	if in.TagCampaign == nil {
		t := true
		in.TagCampaign = &t
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "ads_entrypoints_org_code")
}

// Create adds an entry point (R1).
func (s *Service) Create(ctx context.Context, orgID, userID uuid.UUID, in Input) (*EntryPoint, error) {
	code, err := normaliseCode(in.Code)
	if err != nil {
		return nil, err
	}
	if err := in.check(code); err != nil {
		return nil, err
	}
	now := s.now()
	row := entryRow{OrganizationID: orgID, Name: in.Name, Code: code, Channel: in.Channel, Campaign: in.Campaign,
		WaNumber: in.WaNumber, PrefillText: in.PrefillText, Tag: in.Tag, TagCampaign: *in.TagCampaign,
		CreatedByID: &userID, UpdatedByID: &userID, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, kit.Conflict("code: %s is already used in this organization (archived entry points included)", code)
		}
		return nil, fmt.Errorf("create entry point: %w", err)
	}
	s.codes.drop(orgID)
	v := row.view(Stats{})
	return &v, nil
}

func (s *Service) load(ctx context.Context, db *gorm.DB, orgID, id uuid.UUID, lock bool) (*entryRow, error) {
	q := db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []entryRow
	if err := q.Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load entry point: %w", err)
	}
	if len(rows) == 0 {
		return nil, kit.NotFound("entry point not found")
	}
	return &rows[0], nil
}

// Update changes an entry point; the code never changes (R3).
func (s *Service) Update(ctx context.Context, orgID, userID, id uuid.UUID, in Input) (*EntryPoint, error) {
	row, err := s.load(ctx, s.db, orgID, id, false)
	if err != nil {
		return nil, err
	}
	if row.ArchivedAt != nil {
		return nil, kit.Conflict("this entry point is archived and can't be edited")
	}
	if err := in.check(row.Code); err != nil {
		return nil, err
	}
	upd := map[string]any{"name": in.Name, "channel": in.Channel, "campaign": in.Campaign, "wa_number": in.WaNumber,
		"prefill_text": in.PrefillText, "tag": in.Tag, "tag_campaign": *in.TagCampaign, "updated_by_id": userID, "updated_at": s.now()}
	res := s.db.WithContext(ctx).Model(&entryRow{}).Where("organization_id = ? AND id = ? AND archived_at IS NULL", orgID, id).Updates(upd)
	if res.Error != nil {
		return nil, fmt.Errorf("update entry point: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, kit.Conflict("this entry point is archived and can't be edited")
	}
	s.codes.drop(orgID)
	return s.Get(ctx, orgID, id, Period{})
}

// Archive stops an entry point's code matching (R3). Archiving twice is a no-op.
func (s *Service) Archive(ctx context.Context, orgID, userID, id uuid.UUID) (*EntryPoint, error) {
	if _, err := s.load(ctx, s.db, orgID, id, false); err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Model(&entryRow{}).Where("organization_id = ? AND id = ? AND archived_at IS NULL", orgID, id).
		Updates(map[string]any{"archived_at": now, "updated_by_id": userID, "updated_at": now}).Error; err != nil {
		return nil, fmt.Errorf("archive entry point: %w", err)
	}
	s.codes.drop(orgID)
	return s.Get(ctx, orgID, id, Period{})
}

// Delete removes an entry point nobody arrived through (R3).
func (s *Service) Delete(ctx context.Context, orgID, id uuid.UUID) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.load(ctx, tx, orgID, id, true); err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&arrivalRow{}).Where("organization_id = ? AND entrypoint_id = ?", orgID, id).Count(&n).Error; err != nil {
			return fmt.Errorf("count arrivals: %w", err)
		}
		if n > 0 {
			return kit.Conflict("people have arrived through this entry point; archive it instead")
		}
		if err := tx.Where("organization_id = ? AND id = ?", orgID, id).Delete(&entryRow{}).Error; err != nil {
			return fmt.Errorf("delete entry point: %w", err)
		}
		return nil
	})
	if err == nil {
		s.codes.drop(orgID)
	}
	return err
}

// ---------- reading ----------

// Period is a half-open range on arrived_at; zero values mean the last 30 days.
type Period struct {
	From, To time.Time
}

func (s *Service) period(p Period) Period {
	if p.To.IsZero() {
		p.To = s.now()
	}
	if p.From.IsZero() {
		p.From = p.To.Add(-creditWindow)
	}
	return p
}

// creditedSQL says whether an arrival a led to an event at <when> within 30
// days, with no later arrival of the same contact before it (R9, Q4).
// a.next_at is the contact's next arrival after a (any entry point), or null.
func creditedSQL(when string) string {
	return when + ` > a.arrived_at AND ` + when + ` <= a.arrived_at + interval '30 days'
		AND (a.next_at IS NULL OR ` + when + ` <= a.next_at)`
}

var (
	optedInSQL = `EXISTS (SELECT 1 FROM wl.consent_events e WHERE e.organization_id = a.organization_id
		AND e.contact_id = a.contact_id AND e.action = 'opt_in' AND ` + creditedSQL("e.given_at") + `)`
	bookedSQL = `EXISTS (SELECT 1 FROM wl.booking_appointments b WHERE b.organization_id = a.organization_id
		AND b.contact_id = a.contact_id AND b.source <> 'cliniko' AND ` + creditedSQL("b.created_at") + `)`
)

// arrivalsSQL is the organization's arrivals with each one's next_at. It
// reads only arrivals from the period's start on: an earlier arrival is never
// "next" to one in the period.
const arrivalsSQL = `WITH a AS (
	SELECT x.*, lead(x.arrived_at) OVER (PARTITION BY x.contact_id ORDER BY x.arrived_at, x.id) AS next_at
	FROM wl.ads_arrivals x WHERE x.organization_id = ? AND x.arrived_at >= ?)
`

// stats counts arrivals per entry point in a period; ids limits it to some.
func (s *Service) stats(ctx context.Context, orgID uuid.UUID, p Period, ids []uuid.UUID) (map[uuid.UUID]Stats, error) {
	q := arrivalsSQL + `SELECT a.entrypoint_id AS id, count(*) AS conversations, count(*) FILTER (WHERE a.is_new) AS new_contacts,
			count(DISTINCT a.contact_id) FILTER (WHERE ` + optedInSQL + `) AS opted_in,
			count(DISTINCT a.contact_id) FILTER (WHERE ` + bookedSQL + `) AS booked
		FROM a WHERE a.arrived_at < ?`
	args := []any{orgID, p.From, p.To}
	if ids != nil {
		q += ` AND a.entrypoint_id IN ?`
		args = append(args, ids)
	}
	q += ` GROUP BY a.entrypoint_id`
	var rows []struct {
		ID uuid.UUID
		Stats
	}
	if err := s.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("count arrivals: %w", err)
	}
	out := make(map[uuid.UUID]Stats, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Stats
	}
	return out, nil
}

// CampaignTotals adds up a campaign's entry points (R9).
type CampaignTotals struct {
	Campaign string `json:"campaign"`
	Stats
}

// Listing is the Entry points page.
type Listing struct {
	EntryPoints []EntryPoint     `json:"entrypoints"`
	Campaigns   []CampaignTotals `json:"campaigns"`
	Period      struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	} `json:"period"`
}

// List returns the entry points with their counts for a period (R9).
func (s *Service) List(ctx context.Context, orgID uuid.UUID, p Period, includeArchived bool) (*Listing, error) {
	p = s.period(p)
	q := s.db.WithContext(ctx).Where("organization_id = ?", orgID)
	if !includeArchived {
		q = q.Where("archived_at IS NULL")
	}
	var rows []entryRow
	if err := q.Order("archived_at IS NOT NULL, lower(name), id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list entry points: %w", err)
	}
	st, err := s.stats(ctx, orgID, p, nil)
	if err != nil {
		return nil, err
	}
	out := &Listing{EntryPoints: make([]EntryPoint, 0, len(rows)), Campaigns: []CampaignTotals{}}
	out.Period.From, out.Period.To = p.From, p.To
	byCampaign := map[string]int{}
	for _, r := range rows {
		v := r.view(st[r.ID])
		out.EntryPoints = append(out.EntryPoints, v)
		if r.Campaign == "" {
			continue
		}
		i, ok := byCampaign[r.Campaign]
		if !ok {
			i = len(out.Campaigns)
			byCampaign[r.Campaign] = i
			out.Campaigns = append(out.Campaigns, CampaignTotals{Campaign: r.Campaign})
		}
		out.Campaigns[i].add(v.Stats)
	}
	return out, nil
}

// Get returns one entry point with its counts for a period.
func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID, p Period) (*EntryPoint, error) {
	row, err := s.load(ctx, s.db, orgID, id, false)
	if err != nil {
		return nil, err
	}
	st, err := s.stats(ctx, orgID, s.period(p), []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	v := row.view(st[id])
	return &v, nil
}

// ContactRef is a contact with a masked number.
type ContactRef struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	PhoneMasked string    `json:"phone_masked"`
}

// Arrival is one row of an entry point's arrivals (R10).
type Arrival struct {
	ID        uuid.UUID  `json:"id"`
	Contact   ContactRef `json:"contact"`
	ArrivedAt time.Time  `json:"arrived_at"`
	IsNew     bool       `json:"is_new"`
	OptedIn   bool       `json:"opted_in"`
	Booked    bool       `json:"booked"`
}

func maskPhone(p string) string {
	d := nonDigit.ReplaceAllString(p, "")
	if len(d) <= 5 {
		return strings.Repeat("•", len(d))
	}
	return d[:2] + strings.Repeat("•", len(d)-5) + d[len(d)-3:]
}

// firstArrival is when anyone first arrived through an entry point (or now).
func (s *Service) firstArrival(ctx context.Context, orgID, id uuid.UUID) (time.Time, error) {
	var row struct{ First *time.Time }
	if err := s.db.WithContext(ctx).Raw(`SELECT min(arrived_at) AS first FROM wl.ads_arrivals WHERE organization_id = ? AND entrypoint_id = ?`,
		orgID, id).Scan(&row).Error; err != nil {
		return time.Time{}, fmt.Errorf("first arrival: %w", err)
	}
	if row.First != nil {
		return *row.First, nil
	}
	return s.now(), nil
}

// Arrivals returns an entry point's arrivals, newest first, before a time.
func (s *Service) Arrivals(ctx context.Context, orgID, id uuid.UUID, before *time.Time) ([]Arrival, error) {
	if _, err := s.load(ctx, s.db, orgID, id, false); err != nil {
		return nil, err
	}
	// next_at only needs arrivals from the entry point's first one on.
	first, err := s.firstArrival(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	q := arrivalsSQL + `SELECT a.id, a.contact_id, a.arrived_at, a.is_new, COALESCE(c.profile_name, '') AS name,
			COALESCE(c.phone_number, '') AS phone, ` + optedInSQL + ` AS opted_in, ` + bookedSQL + ` AS booked
		FROM a
		LEFT JOIN contacts c ON c.id = a.contact_id AND c.organization_id = a.organization_id
		WHERE a.entrypoint_id = ?`
	args := []any{orgID, first, id}
	if before != nil {
		q += ` AND a.arrived_at < ?`
		args = append(args, *before)
	}
	q += ` ORDER BY a.arrived_at DESC, a.id LIMIT ?`
	args = append(args, pageSize)
	var rows []struct {
		ID        uuid.UUID
		ContactID uuid.UUID
		ArrivedAt time.Time
		IsNew     bool
		Name      string
		Phone     string
		OptedIn   bool
		Booked    bool
	}
	if err := s.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list arrivals: %w", err)
	}
	out := make([]Arrival, 0, len(rows))
	for _, r := range rows {
		out = append(out, Arrival{ID: r.ID, Contact: ContactRef{ID: r.ContactID, Name: r.Name, PhoneMasked: maskPhone(r.Phone)},
			ArrivedAt: r.ArrivedAt, IsNew: r.IsNew, OptedIn: r.OptedIn, Booked: r.Booked})
	}
	return out, nil
}

// ---------- code cache ----------

// active is what matching needs about an active entry point.
type active struct {
	ID   uuid.UUID
	Tags []string
}

type cachedCodes struct {
	codes map[string]active
	exp   time.Time
}

// codeCache keeps each organization's active codes for a minute; changes on
// this instance drop it straight away.
type codeCache struct {
	mu sync.Mutex
	m  map[uuid.UUID]cachedCodes
}

func (c *codeCache) get(org uuid.UUID) (map[string]active, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[org]
	if !ok || time.Now().After(v.exp) {
		return nil, false
	}
	return v.codes, true
}

func (c *codeCache) put(org uuid.UUID, codes map[string]active) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[uuid.UUID]cachedCodes{}
	}
	c.m[org] = cachedCodes{codes: codes, exp: time.Now().Add(codeCacheTTL)}
}

func (c *codeCache) drop(org uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, org)
}

func (s *Service) activeCodes(ctx context.Context, orgID uuid.UUID) (map[string]active, error) {
	if c, ok := s.codes.get(orgID); ok {
		return c, nil
	}
	var rows []entryRow
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND archived_at IS NULL", orgID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load codes: %w", err)
	}
	codes := make(map[string]active, len(rows))
	for _, r := range rows {
		tags := []string{r.Tag}
		if r.TagCampaign {
			if t := campaignTag(r.Campaign); t != "" {
				tags = append(tags, t)
			}
		}
		codes[r.Code] = active{ID: r.ID, Tags: tags}
	}
	s.codes.put(orgID, codes)
	return codes, nil
}

// ---------- arrivals ----------

// Incoming is the part of a message.incoming event matching needs.
type Incoming struct {
	ContactID uuid.UUID
	MessageID *uuid.UUID
	Text      string
}

// Recorded says what Record did.
type Recorded struct {
	Arrived      bool
	EntrypointID uuid.UUID
	IsNew        bool
}

var errSkip = errors.New("skip")

// Record matches a code in an incoming text and records the arrival, tagging
// the contact (R4-R7). Idempotent; sends nothing.
func (s *Service) Record(ctx context.Context, orgID uuid.UUID, in Incoming) (*Recorded, error) {
	if len([]rune(in.Text)) > maxMatchRunes {
		return &Recorded{}, nil
	}
	codes, err := s.activeCodes(ctx, orgID)
	if err != nil || len(codes) == 0 {
		return &Recorded{}, err
	}
	code, ok := firstCode(in.Text, codes)
	if !ok {
		return &Recorded{}, nil
	}
	ep := codes[code]
	res := &Recorded{EntrypointID: ep.ID}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Locking the contact serialises messages from the same person, so two
		// can't both pass the 24-hour check.
		var cs []struct {
			ID   uuid.UUID
			Tags models.JSONBArray `gorm:"type:jsonb"`
		}
		if err := tx.Raw(`SELECT id, tags FROM contacts WHERE organization_id = ? AND id = ? AND deleted_at IS NULL FOR UPDATE`,
			orgID, in.ContactID).Scan(&cs).Error; err != nil {
			return fmt.Errorf("lock contact: %w", err)
		}
		if len(cs) == 0 {
			return errSkip // not this organization's contact
		}
		at := s.now()
		if in.MessageID != nil {
			var n int64
			if err := tx.Model(&arrivalRow{}).Where("organization_id = ? AND message_id = ?", orgID, *in.MessageID).Count(&n).Error; err != nil {
				return fmt.Errorf("check message: %w", err)
			}
			if n > 0 {
				return errSkip
			}
			var times []time.Time
			if err := tx.Model(&models.Message{}).Where("organization_id = ? AND id = ?", orgID, *in.MessageID).
				Limit(1).Pluck("created_at", &times).Error; err != nil {
				return fmt.Errorf("load message: %w", err)
			}
			if len(times) == 1 {
				at = times[0]
			}
		}
		var recent int64
		if err := tx.Model(&arrivalRow{}).Where("organization_id = ? AND contact_id = ? AND entrypoint_id = ? AND arrived_at > ?",
			orgID, in.ContactID, ep.ID, at.Add(-repeatWindow)).Count(&recent).Error; err != nil {
			return fmt.Errorf("check recent arrival: %w", err)
		}
		if recent > 0 {
			return errSkip
		}
		var first struct{ At *time.Time }
		if err := tx.Raw(`SELECT min(created_at) AS at FROM messages WHERE organization_id = ? AND contact_id = ? AND direction = ?`,
			orgID, in.ContactID, models.DirectionIncoming).Scan(&first).Error; err != nil {
			return fmt.Errorf("first message: %w", err)
		}
		isNew := first.At == nil || !first.At.Before(at.Add(-newWindow))
		row := arrivalRow{OrganizationID: orgID, EntrypointID: ep.ID, ContactID: in.ContactID, MessageID: in.MessageID,
			Code: code, ArrivedAt: at, IsNew: isNew}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("record arrival: %w", err)
		}
		tags, changed := withTags(cs[0].Tags, ep.Tags)
		if changed {
			if err := tx.Model(&models.Contact{}).Where("organization_id = ? AND id = ?", orgID, in.ContactID).
				Update("tags", tags).Error; err != nil {
				return fmt.Errorf("tag contact: %w", err)
			}
		}
		res.Arrived, res.IsNew = true, isNew
		return nil
	})
	if errors.Is(err, errSkip) {
		return &Recorded{}, nil
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

// withTags adds tags that aren't there yet (case-insensitive), keeping the rest (R7).
func withTags(have models.JSONBArray, add []string) (models.JSONBArray, bool) {
	out := append(models.JSONBArray{}, have...)
	changed := false
	for _, t := range add {
		found := false
		for _, h := range out {
			if s, _ := h.(string); strings.EqualFold(s, t) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, t)
			changed = true
		}
	}
	return out, changed
}
