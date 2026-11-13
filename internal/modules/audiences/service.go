package audiences

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/templateutil"
	"gorm.io/gorm"
)

// Service holds the audiences rules (requirements R1–R13). Every method takes
// the organization first and filters every query by it.
type Service struct {
	db      *gorm.DB
	consent *consent.Service // settings only: "marketing only to people who opted in"
	limits  kit.Limits       // plan limits (billing); nil = unlimited
}

// WithLimits meters broadcast recipients against the organization's plan.
func (s *Service) WithLimits(l kit.Limits) *Service {
	s.limits = l
	return s
}

// NewService returns the audiences service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db, consent: consent.NewService(db, nil, nil, nil)}
}

// MaxRecipients is the per-campaign limit (Q5).
const MaxRecipients = 10000

const (
	sampleSize   = 20
	nameMaxLen   = 120
	descMaxLen   = 500
	minPhoneLen  = 8
	campaignName = 255
)

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

// ---------- models ----------

// Segment is a saved set of rules (wl.audiences_segments).
type Segment struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid" json:"-"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Rules          models.JSONB   `gorm:"type:jsonb" json:"rules"`
	CreatedByID    *uuid.UUID     `gorm:"type:uuid" json:"-"`
	UpdatedByID    *uuid.UUID     `gorm:"type:uuid" json:"-"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-"`

	Count      *int       `gorm:"-" json:"count,omitempty"`
	LastUsedAt *time.Time `gorm:"-" json:"last_used_at"`
}

func (Segment) TableName() string { return "wl.audiences_segments" }

// sendRow is one created campaign (wl.audiences_sends).
type sendRow struct {
	ID               uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID   uuid.UUID    `gorm:"type:uuid"`
	Kind             string       `gorm:"column:kind"`
	SegmentID        *uuid.UUID   `gorm:"type:uuid"`
	SourceCampaignID *uuid.UUID   `gorm:"type:uuid"`
	RetargetFilter   *string      `gorm:"column:retarget_filter"`
	CampaignID       uuid.UUID    `gorm:"type:uuid"`
	RulesSnapshot    models.JSONB `gorm:"type:jsonb"`
	Variables        models.JSONB `gorm:"type:jsonb"`
	RecipientCount   int
	Excluded         models.JSONB `gorm:"type:jsonb"`
	CreatedByID      *uuid.UUID   `gorm:"type:uuid"`
	CreatedAt        time.Time
}

func (sendRow) TableName() string { return "wl.audiences_sends" }

// Excluded counts contacts left out, by reason (R6).
type Excluded struct {
	OptedOut   int `json:"opted_out"`
	NoOptIn    int `json:"no_opt_in"` // marketing template while Consent requires an opt-in
	NoNumber   int `json:"no_number"`
	Duplicates int `json:"duplicates"`
	OverLimit  int `json:"over_limit"`
}

func (e Excluded) jsonb() models.JSONB {
	return models.JSONB{"opted_out": e.OptedOut, "no_opt_in": e.NoOptIn, "no_number": e.NoNumber, "duplicates": e.Duplicates,
		"over_limit": e.OverLimit}
}

// SampleContact is a masked preview row (R2).
type SampleContact struct {
	Name        string `json:"name"`
	PhoneMasked string `json:"phone_masked"`
}

// Preview is what a set of rules reaches.
type Preview struct {
	Matched    int      `json:"matched"`    // contacts matching the rules
	Recipients int      `json:"recipients"` // left after exclusions: what a campaign would get
	Excluded   Excluded `json:"excluded"`
	// MarketingOptOut is how many would be left out with a marketing template
	// (R2): equal to Excluded.OptedOut when the preview is for one, else counted
	// among the recipients so the builder can show it before a template is chosen.
	MarketingOptOut int `json:"marketing_opt_out"`
	// NotRecorded is how many recipients have no recorded opt-in (Consent R9).
	// RequireOptIn says whether they are left out of marketing templates.
	NotRecorded  int             `json:"not_recorded"`
	RequireOptIn bool            `json:"require_opt_in"`
	Sample       []SampleContact `json:"sample"`
}

// Send is a created campaign as the history shows it.
type Send struct {
	ID               uuid.UUID      `json:"id"`
	Kind             string         `json:"kind"`
	SegmentID        *uuid.UUID     `json:"segment_id"`
	SourceCampaignID *uuid.UUID     `json:"source_campaign_id"`
	RetargetFilter   *string        `json:"retarget_filter"`
	CampaignID       uuid.UUID      `json:"campaign_id"`
	CampaignName     string         `json:"campaign_name"`
	RecipientCount   int            `json:"recipient_count"`
	Excluded         map[string]any `json:"excluded"`
	CreatedBy        *kitRef        `json:"created_by"`
	CreatedAt        time.Time      `json:"created_at"`
}

type kitRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// ---------- rules plumbing ----------

func parseRules(raw models.JSONB) (*Rules, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, kit.Invalid("invalid rules")
	}
	var r Rules
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, kit.Invalid("invalid rules")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

func rulesJSONB(r *Rules) models.JSONB {
	b, _ := json.Marshal(r)
	var out models.JSONB
	_ = json.Unmarshal(b, &out)
	return out
}

// checkCampaigns makes sure every campaign a rule names is the organization's.
func (s *Service) checkCampaigns(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) error {
	for _, id := range ids {
		var n int64
		if err := s.db.WithContext(ctx).Model(&models.BulkMessageCampaign{}).
			Where("organization_id = ? AND id = ?", orgID, id).Count(&n).Error; err != nil {
			return fmt.Errorf("check campaign: %w", err)
		}
		if n == 0 {
			return kit.NotFound("campaign not found")
		}
	}
	return nil
}

// ---------- segments (R1, R3) ----------

// SegmentInput is the segment form.
type SegmentInput struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Rules       models.JSONB `json:"rules"`
}

// SegmentPatch changes only the fields that are set.
type SegmentPatch struct {
	Name        *string      `json:"name"`
	Description *string      `json:"description"`
	Rules       models.JSONB `json:"rules"`
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", kit.Invalid("name is required")
	}
	if len([]rune(name)) > nameMaxLen {
		return "", kit.Invalid("name is too long (max %d characters)", nameMaxLen)
	}
	return name, nil
}

func cleanDesc(d string) (string, error) {
	d = strings.TrimSpace(d)
	if len([]rune(d)) > descMaxLen {
		return "", kit.Invalid("description is too long (max %d characters)", descMaxLen)
	}
	return d, nil
}

// ListSegments returns the organization's segments with live counts.
func (s *Service) ListSegments(ctx context.Context, orgID uuid.UUID) ([]Segment, error) {
	out := []Segment{}
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Order("name").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}
	for i := range out {
		if r, err := parseRules(out[i].Rules); err == nil {
			if n, err := s.count(ctx, orgID, r); err == nil {
				out[i].Count = &n
			}
		}
		var last []time.Time
		if err := s.db.WithContext(ctx).Model(&sendRow{}).Where("organization_id = ? AND segment_id = ?", orgID, out[i].ID).
			Order("created_at DESC").Limit(1).Pluck("created_at", &last).Error; err == nil && len(last) > 0 {
			out[i].LastUsedAt = &last[0]
		}
	}
	return out, nil
}

// GetSegment returns one segment.
func (s *Service) GetSegment(ctx context.Context, orgID, id uuid.UUID) (*Segment, error) {
	var found []Segment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id).Limit(1).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("load segment: %w", err)
	}
	if len(found) == 0 {
		return nil, kit.NotFound("segment not found")
	}
	return &found[0], nil
}

// CreateSegment saves a new segment.
func (s *Service) CreateSegment(ctx context.Context, orgID, userID uuid.UUID, in SegmentInput) (*Segment, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return nil, err
	}
	desc, err := cleanDesc(in.Description)
	if err != nil {
		return nil, err
	}
	r, err := parseRules(in.Rules)
	if err != nil {
		return nil, err
	}
	if err := s.checkCampaigns(ctx, orgID, r.campaignIDs()); err != nil {
		return nil, err
	}
	seg := Segment{OrganizationID: orgID, Name: name, Description: desc, Rules: rulesJSONB(r), CreatedByID: &userID, UpdatedByID: &userID}
	if err := s.db.WithContext(ctx).Create(&seg).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, kit.Conflict("a segment named %q already exists", name)
		}
		return nil, fmt.Errorf("save segment: %w", err)
	}
	return &seg, nil
}

// UpdateSegment changes a segment.
func (s *Service) UpdateSegment(ctx context.Context, orgID, userID, id uuid.UUID, in SegmentPatch) (*Segment, error) {
	seg, err := s.GetSegment(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{"updated_by_id": userID, "updated_at": time.Now()}
	if in.Name != nil {
		name, err := cleanName(*in.Name)
		if err != nil {
			return nil, err
		}
		fields["name"] = name
	}
	if in.Description != nil {
		d, err := cleanDesc(*in.Description)
		if err != nil {
			return nil, err
		}
		fields["description"] = d
	}
	if in.Rules != nil {
		r, err := parseRules(in.Rules)
		if err != nil {
			return nil, err
		}
		if err := s.checkCampaigns(ctx, orgID, r.campaignIDs()); err != nil {
			return nil, err
		}
		fields["rules"] = rulesJSONB(r)
	}
	if err := s.db.WithContext(ctx).Model(seg).Where("organization_id = ?", orgID).Updates(fields).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, kit.Conflict("a segment named %q already exists", fields["name"])
		}
		return nil, fmt.Errorf("update segment: %w", err)
	}
	return s.GetSegment(ctx, orgID, id)
}

// DeleteSegment removes a segment; its sends keep their snapshot.
func (s *Service) DeleteSegment(ctx context.Context, orgID, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id).Delete(&Segment{})
	if res.Error != nil {
		return fmt.Errorf("delete segment: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return kit.NotFound("segment not found")
	}
	return nil
}

// ---------- evaluation ----------

type contactRow struct {
	ID              uuid.UUID
	PhoneNumber     string
	ProfileName     string
	Metadata        models.JSONB `gorm:"type:jsonb"`
	MarketingOptOut bool
	OptedIn         bool // Consent has an opt-in on record
}

var nonDigits = regexp.MustCompile(`\D`)

func normalisePhone(p string) string { return nonDigits.ReplaceAllString(p, "") }

func maskPhone(p string) string {
	d := normalisePhone(p)
	if len(d) <= 5 {
		return strings.Repeat("•", len(d))
	}
	return d[:2] + strings.Repeat("•", len(d)-5) + d[len(d)-3:]
}

func (s *Service) count(ctx context.Context, orgID uuid.UUID, r *Rules) (int, error) {
	where, args := r.compile(orgID)
	var n int64
	if err := s.db.WithContext(ctx).Raw("SELECT count(*) FROM contacts c WHERE "+where, args...).Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("count contacts: %w", err)
	}
	return int(n), nil
}

// evaluate returns the matching contacts, most recently active first.
func (s *Service) evaluate(ctx context.Context, orgID uuid.UUID, r *Rules) ([]contactRow, error) {
	where, args := r.compile(orgID)
	var rows []contactRow
	err := s.db.WithContext(ctx).Raw(`SELECT c.id, c.phone_number, c.profile_name, c.metadata, c.marketing_opt_out,
			EXISTS (SELECT 1 FROM wl.consent_status cs WHERE cs.organization_id = c.organization_id AND cs.contact_id = c.id
				AND cs.marketing = 'opted_in') AS opted_in
		FROM contacts c WHERE `+where+` ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC, c.id`, args...).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("evaluate segment: %w", err)
	}
	return rows, nil
}

// pick applies the exclusions (R6, R7, R13, Consent R9) and the limit.
func pick(rows []contactRow, marketing, requireOptIn bool) ([]contactRow, Excluded) {
	var ex Excluded
	seen := map[string]bool{}
	out := make([]contactRow, 0, len(rows))
	for _, c := range rows {
		p := normalisePhone(c.PhoneNumber)
		switch {
		case len(p) < minPhoneLen:
			ex.NoNumber++
		case seen[p]:
			ex.Duplicates++
		case marketing && c.MarketingOptOut:
			seen[p] = true
			ex.OptedOut++
		case marketing && requireOptIn && !c.OptedIn:
			seen[p] = true
			ex.NoOptIn++
		case len(out) >= MaxRecipients:
			seen[p] = true
			ex.OverLimit++
		default:
			seen[p] = true
			out = append(out, c)
		}
	}
	return out, ex
}

// PreviewInput selects what to preview: rules, or a retarget of a campaign,
// optionally for a template (to count marketing opt-outs).
type PreviewInput struct {
	Rules      models.JSONB `json:"rules"`
	Retarget   *Retarget    `json:"retarget"`
	TemplateID *uuid.UUID   `json:"template_id"`
}

// Retarget is a follow-up from a past campaign (R8).
type Retarget struct {
	CampaignID uuid.UUID `json:"campaign_id"`
	Filter     string    `json:"filter"`
	Days       int       `json:"days"`
}

func (rt Retarget) rules() (*Rules, error) {
	if !allowed(RetargetFilters, rt.Filter) {
		return nil, kit.Invalid("filter must be one of %s", strings.Join(RetargetFilters, ", "))
	}
	v, _ := json.Marshal(rt.CampaignID.String())
	r := &Rules{Match: "all", Conditions: []Condition{{Field: "campaign", Op: rt.Filter, Value: v, Days: rt.Days}}}
	return r, r.Validate()
}

func (s *Service) resolve(ctx context.Context, orgID uuid.UUID, raw models.JSONB, rt *Retarget) (*Rules, error) {
	var r *Rules
	var err error
	switch {
	case rt != nil:
		r, err = rt.rules()
	case raw != nil:
		r, err = parseRules(raw)
	default:
		return nil, kit.Invalid("give rules, a segment or a retarget")
	}
	if err != nil {
		return nil, err
	}
	if err := s.checkCampaigns(ctx, orgID, r.campaignIDs()); err != nil {
		return nil, err
	}
	return r, nil
}

// Preview counts what rules or a retarget reach, with exclusions and a masked sample.
func (s *Service) Preview(ctx context.Context, orgID uuid.UUID, in PreviewInput) (*Preview, error) {
	r, err := s.resolve(ctx, orgID, in.Rules, in.Retarget)
	if err != nil {
		return nil, err
	}
	marketing := false
	if in.TemplateID != nil {
		t, err := s.template(ctx, orgID, *in.TemplateID)
		if err != nil {
			return nil, err
		}
		marketing = strings.EqualFold(t.Category, "MARKETING")
	}
	rows, err := s.evaluate(ctx, orgID, r)
	if err != nil {
		return nil, err
	}
	cs, err := s.consent.Settings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	kept, ex := pick(rows, marketing, cs.RequireOptIn)
	out := &Preview{Matched: len(rows), Recipients: len(kept), Excluded: ex, MarketingOptOut: ex.OptedOut,
		NotRecorded: ex.NoOptIn, RequireOptIn: cs.RequireOptIn, Sample: []SampleContact{}}
	for _, c := range kept {
		if !marketing && c.MarketingOptOut {
			out.MarketingOptOut++
		}
		if !c.OptedIn && !c.MarketingOptOut {
			out.NotRecorded++
		}
	}
	for i := 0; i < len(kept) && i < sampleSize; i++ {
		out.Sample = append(out.Sample, SampleContact{Name: kept[i].ProfileName, PhoneMasked: maskPhone(kept[i].PhoneNumber)})
	}
	return out, nil
}

// ---------- creating a campaign (R4–R7, R13) ----------

// VarSource fills one template variable per recipient (R5).
type VarSource struct {
	Source   string `json:"source"` // text | name | phone | metadata
	Value    string `json:"value,omitempty"`
	Key      string `json:"key,omitempty"`
	Fallback string `json:"fallback,omitempty"`
}

// Variables maps template variable names to sources, for body and header.
type Variables struct {
	Body   map[string]VarSource `json:"body"`
	Header map[string]VarSource `json:"header"`
}

// SendInput creates a draft campaign from a segment or a retarget.
type SendInput struct {
	Name            string     `json:"name"`
	WhatsAppAccount string     `json:"whatsapp_account"`
	TemplateID      uuid.UUID  `json:"template_id"`
	SegmentID       *uuid.UUID `json:"segment_id"`
	Retarget        *Retarget  `json:"retarget"`
	Variables       Variables  `json:"variables"`
	ConfirmCount    int        `json:"confirm_count"`
}

// SendResult is what was created.
type SendResult struct {
	CampaignID uuid.UUID `json:"campaign_id"`
	Recipients int       `json:"recipients"`
	Excluded   Excluded  `json:"excluded"`
}

func (s *Service) template(ctx context.Context, orgID, id uuid.UUID) (*models.Template, error) {
	var found []models.Template
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id).Limit(1).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("load template: %w", err)
	}
	if len(found) == 0 {
		return nil, kit.NotFound("template not found")
	}
	return &found[0], nil
}

// templateVars returns the template's body and header variable names.
func templateVars(t *models.Template) (body, header []string) {
	body = templateutil.ExtParamNames(t.BodyContent)
	if strings.EqualFold(t.HeaderType, "TEXT") {
		header = templateutil.ExtParamNames(t.HeaderContent)
	}
	return body, header
}

func checkVars(names []string, got map[string]VarSource, part string) error {
	for _, n := range names {
		v, ok := got[n]
		if !ok {
			return kit.Invalid("choose a value for {{%s}} in the %s", n, part)
		}
		switch v.Source {
		case "text", "name", "phone":
		case "metadata":
			if !metadataKeyRe.MatchString(v.Key) {
				return kit.Invalid("{{%s}}: choose a contact field", n)
			}
		default:
			return kit.Invalid("{{%s}}: unknown source %q", n, v.Source)
		}
	}
	return nil
}

func render(v VarSource, c contactRow) string {
	var out string
	switch v.Source {
	case "text":
		out = v.Value
	case "name":
		out = c.ProfileName
	case "phone":
		out = normalisePhone(c.PhoneNumber)
	case "metadata":
		if c.Metadata != nil {
			if x, ok := c.Metadata[v.Key]; ok && x != nil {
				out = strings.TrimSpace(fmt.Sprint(x))
			}
		}
	}
	if strings.TrimSpace(out) == "" {
		return v.Fallback
	}
	return out
}

func varsJSONB(v Variables) models.JSONB {
	b, _ := json.Marshal(v)
	var out models.JSONB
	_ = json.Unmarshal(b, &out)
	return out
}

// CreateSend turns a segment or a retarget into a draft campaign in Weline's
// Campaigns, through upstream's models (the rows Weline's import writes).
func (s *Service) CreateSend(ctx context.Context, orgID, userID uuid.UUID, in SendInput) (*SendResult, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, kit.Invalid("campaign name is required")
	}
	if len([]rune(name)) > campaignName {
		return nil, kit.Invalid("campaign name is too long")
	}
	if (in.SegmentID == nil) == (in.Retarget == nil) {
		return nil, kit.Invalid("give either a segment or a retarget")
	}
	var account []models.WhatsAppAccount
	if err := s.db.WithContext(ctx).Select("id", "name").Where("organization_id = ? AND name = ?", orgID, in.WhatsAppAccount).
		Limit(1).Find(&account).Error; err != nil {
		return nil, fmt.Errorf("load account: %w", err)
	}
	if len(account) == 0 {
		return nil, kit.NotFound("WhatsApp account not found")
	}
	t, err := s.template(ctx, orgID, in.TemplateID)
	if err != nil {
		return nil, err
	}
	if t.WhatsAppAccount != in.WhatsAppAccount {
		return nil, kit.Invalid("template %s belongs to another WhatsApp number", t.Name)
	}
	if !strings.EqualFold(t.Status, "APPROVED") {
		return nil, kit.Invalid("template %s isn't approved by Meta yet", t.Name)
	}
	bodyVars, headerVars := templateVars(t)
	if err := checkVars(bodyVars, in.Variables.Body, "message"); err != nil {
		return nil, err
	}
	if err := checkVars(headerVars, in.Variables.Header, "header"); err != nil {
		return nil, err
	}

	send := sendRow{OrganizationID: orgID, CreatedByID: &userID, Variables: varsJSONB(in.Variables)}
	var r *Rules
	if in.SegmentID != nil {
		seg, err := s.GetSegment(ctx, orgID, *in.SegmentID)
		if err != nil {
			return nil, err
		}
		if r, err = s.resolve(ctx, orgID, seg.Rules, nil); err != nil {
			return nil, err
		}
		send.Kind, send.SegmentID = "segment", &seg.ID
	} else {
		if r, err = s.resolve(ctx, orgID, nil, in.Retarget); err != nil {
			return nil, err
		}
		f := in.Retarget.Filter
		send.Kind, send.SourceCampaignID, send.RetargetFilter = "retarget", &in.Retarget.CampaignID, &f
	}
	send.RulesSnapshot = rulesJSONB(r)

	rows, err := s.evaluate(ctx, orgID, r)
	if err != nil {
		return nil, err
	}
	cs, err := s.consent.Settings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	kept, ex := pick(rows, strings.EqualFold(t.Category, "MARKETING"), cs.RequireOptIn)
	if len(kept) == 0 {
		return nil, kit.Invalid("no one to send to after exclusions")
	}
	if in.ConfirmCount != len(kept) {
		return nil, kit.Conflict("the audience changed (now %d recipients, you confirmed %d); please check again", len(kept), in.ConfirmCount)
	}
	if s.limits != nil {
		if err := s.limits.Allow(ctx, orgID, kit.MetricBroadcast, len(kept)); err != nil {
			return nil, err
		}
	}
	send.RecipientCount, send.Excluded = len(kept), ex.jsonb()

	campaign := models.BulkMessageCampaign{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		OrganizationID:  orgID,
		WhatsAppAccount: in.WhatsAppAccount,
		Name:            name,
		TemplateID:      t.ID,
		Status:          models.CampaignStatusDraft,
		TotalRecipients: len(kept),
		CreatedBy:       userID,
	}
	recipients := make([]models.BulkMessageRecipient, 0, len(kept))
	for _, c := range kept {
		body := models.JSONB{}
		for _, n := range bodyVars {
			body[n] = render(in.Variables.Body[n], c)
		}
		header := models.JSONB{}
		for _, n := range headerVars {
			header[n] = render(in.Variables.Header[n], c)
		}
		recipients = append(recipients, models.BulkMessageRecipient{
			BaseModel:      models.BaseModel{ID: uuid.New()},
			CampaignID:     campaign.ID,
			PhoneNumber:    normalisePhone(c.PhoneNumber),
			RecipientName:  c.ProfileName,
			TemplateParams: body,
			HeaderParams:   header,
			Status:         models.MessageStatusPending,
		})
	}
	send.CampaignID = campaign.ID
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&campaign).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(&recipients, 500).Error; err != nil {
			return err
		}
		return tx.Create(&send).Error
	})
	if err != nil {
		return nil, fmt.Errorf("create campaign: %w", err)
	}
	return &SendResult{CampaignID: campaign.ID, Recipients: len(kept), Excluded: ex}, nil
}

// ---------- history (R10) ----------

// ListSends returns created campaigns, newest first.
func (s *Service) ListSends(ctx context.Context, orgID uuid.UUID, segmentID, campaignID *uuid.UUID) ([]Send, error) {
	q := s.db.WithContext(ctx).Where("organization_id = ?", orgID)
	if segmentID != nil {
		q = q.Where("segment_id = ?", *segmentID)
	}
	if campaignID != nil {
		q = q.Where("campaign_id = ? OR source_campaign_id = ?", *campaignID, *campaignID)
	}
	var rows []sendRow
	if err := q.Order("created_at DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list sends: %w", err)
	}
	out := make([]Send, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	campaignIDs, userIDs := []uuid.UUID{}, []uuid.UUID{}
	for _, r := range rows {
		campaignIDs = append(campaignIDs, r.CampaignID)
		if r.CreatedByID != nil {
			userIDs = append(userIDs, *r.CreatedByID)
		}
	}
	names := map[uuid.UUID]string{}
	var cs []models.BulkMessageCampaign
	if err := s.db.WithContext(ctx).Select("id", "name").Where("organization_id = ? AND id IN ?", orgID, campaignIDs).Find(&cs).Error; err != nil {
		return nil, fmt.Errorf("load campaigns: %w", err)
	}
	for _, c := range cs {
		names[c.ID] = c.Name
	}
	users := map[uuid.UUID]string{}
	if len(userIDs) > 0 {
		var us []models.User
		if err := s.db.WithContext(ctx).Raw(`SELECT u.id, u.full_name FROM users u WHERE u.id IN ? AND (u.organization_id = ?
			OR EXISTS (SELECT 1 FROM user_organizations uo WHERE uo.user_id = u.id AND uo.organization_id = ? AND uo.deleted_at IS NULL))`,
			userIDs, orgID, orgID).Scan(&us).Error; err != nil {
			return nil, fmt.Errorf("load users: %w", err)
		}
		for _, u := range us {
			users[u.ID] = u.FullName
		}
	}
	for _, r := range rows {
		snd := Send{ID: r.ID, Kind: r.Kind, SegmentID: r.SegmentID, SourceCampaignID: r.SourceCampaignID, RetargetFilter: r.RetargetFilter,
			CampaignID: r.CampaignID, CampaignName: names[r.CampaignID], RecipientCount: r.RecipientCount,
			Excluded: map[string]any(r.Excluded), CreatedAt: r.CreatedAt}
		if r.CreatedByID != nil {
			snd.CreatedBy = &kitRef{ID: *r.CreatedByID, Name: users[*r.CreatedByID]}
		}
		out = append(out, snd)
	}
	return out, nil
}

// ---------- builder pickers ----------

// TemplateOption is an approved template the create dialog can use.
type TemplateOption struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Language        string    `json:"language"`
	Category        string    `json:"category"`
	WhatsAppAccount string    `json:"whatsapp_account"`
	HeaderType      string    `json:"header_type"`
	HeaderContent   string    `json:"header_content"`
	BodyContent     string    `json:"body_content"`
	BodyVars        []string  `json:"body_vars"`
	HeaderVars      []string  `json:"header_vars"`
}

// CampaignOption is a past campaign for rules and retargets.
type CampaignOption struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Total       int        `json:"total"`
	Sent        int        `json:"sent"`
	Delivered   int        `json:"delivered"`
	Read        int        `json:"read"`
	Failed      int        `json:"failed"`
	CompletedAt *time.Time `json:"completed_at"`
}

// Fields are the pickers for the builder and create dialog.
type Fields struct {
	Tags         []string         `json:"tags"`
	MetadataKeys []string         `json:"metadata_keys"`
	Accounts     []string         `json:"accounts"`
	Campaigns    []CampaignOption `json:"campaigns"`
	Templates    []TemplateOption `json:"templates"`
}

// Fields returns tags, contact fields, accounts, past campaigns and approved templates.
func (s *Service) Fields(ctx context.Context, orgID uuid.UUID) (*Fields, error) {
	out := &Fields{Tags: []string{}, MetadataKeys: []string{}, Accounts: []string{}, Campaigns: []CampaignOption{}, Templates: []TemplateOption{}}
	db := s.db.WithContext(ctx)
	if err := db.Model(&models.Tag{}).Where("organization_id = ?", orgID).Order("name").Pluck("name", &out.Tags).Error; err != nil {
		return nil, fmt.Errorf("load tags: %w", err)
	}
	if err := db.Raw(`SELECT k FROM contacts c, jsonb_object_keys(CASE WHEN jsonb_typeof(c.metadata) = 'object' THEN c.metadata ELSE '{}'::jsonb END) k
		WHERE c.organization_id = ? AND c.deleted_at IS NULL GROUP BY k ORDER BY count(*) DESC, k LIMIT 50`, orgID).
		Scan(&out.MetadataKeys).Error; err != nil {
		return nil, fmt.Errorf("load contact fields: %w", err)
	}
	keys := out.MetadataKeys[:0]
	for _, k := range out.MetadataKeys {
		if metadataKeyRe.MatchString(k) {
			keys = append(keys, k)
		}
	}
	out.MetadataKeys = keys
	if err := db.Model(&models.WhatsAppAccount{}).Where("organization_id = ?", orgID).Order("name").Pluck("name", &out.Accounts).Error; err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}
	var cs []models.BulkMessageCampaign
	if err := db.Where("organization_id = ? AND status <> ?", orgID, models.CampaignStatusDraft).
		Order("created_at DESC").Limit(50).Find(&cs).Error; err != nil {
		return nil, fmt.Errorf("load campaigns: %w", err)
	}
	for _, c := range cs {
		out.Campaigns = append(out.Campaigns, CampaignOption{ID: c.ID, Name: c.Name, Status: string(c.Status), Total: c.TotalRecipients,
			Sent: c.SentCount, Delivered: c.DeliveredCount, Read: c.ReadCount, Failed: c.FailedCount, CompletedAt: c.CompletedAt})
	}
	var ts []models.Template
	if err := db.Where("organization_id = ? AND upper(status) = 'APPROVED'", orgID).Order("name").Find(&ts).Error; err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	for i := range ts {
		body, header := templateVars(&ts[i])
		if body == nil {
			body = []string{}
		}
		if header == nil {
			header = []string{}
		}
		out.Templates = append(out.Templates, TemplateOption{ID: ts[i].ID, Name: ts[i].Name, Language: ts[i].Language, Category: ts[i].Category,
			WhatsAppAccount: ts[i].WhatsAppAccount, HeaderType: ts[i].HeaderType, HeaderContent: ts[i].HeaderContent,
			BodyContent: ts[i].BodyContent, BodyVars: body, HeaderVars: header})
	}
	return out, nil
}
