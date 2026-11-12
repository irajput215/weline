package audiences

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fixture struct {
	db      *gorm.DB
	svc     *Service
	org     uuid.UUID
	user    uuid.UUID
	account string
	tmpl    *models.Template // MARKETING, body "Hi {{name}}, your {{1}}"
	util    *models.Template // UTILITY, no variables
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	acc := testutil.CreateTestWhatsAppAccount(t, db, org.ID)
	tmpl := testutil.CreateTestTemplate(t, db, org.ID, acc.Name)
	require.NoError(t, db.Model(tmpl).Update("body_content", "Hi {{name}}, your {{1}}").Error)
	tmpl.BodyContent = "Hi {{name}}, your {{1}}"
	util := testutil.CreateTestTemplate(t, db, org.ID, acc.Name)
	require.NoError(t, db.Model(util).Updates(map[string]any{"category": "UTILITY", "body_content": "Your clinic is closed today"}).Error)
	return &fixture{db: db, svc: NewService(db), org: org.ID, user: user.ID, account: acc.Name, tmpl: tmpl, util: util}
}

type contactOpt func(*models.Contact)

func tags(t ...string) contactOpt {
	return func(c *models.Contact) {
		c.Tags = models.JSONBArray{}
		for _, x := range t {
			c.Tags = append(c.Tags, x)
		}
	}
}
func meta(m models.JSONB) contactOpt { return func(c *models.Contact) { c.Metadata = m } }
func phone(p string) contactOpt      { return func(c *models.Contact) { c.PhoneNumber = p } }
func named(n string) contactOpt      { return func(c *models.Contact) { c.ProfileName = n } }
func optedOut() contactOpt           { return func(c *models.Contact) { c.MarketingOptOut = true } }

func (f *fixture) contact(t *testing.T, org uuid.UUID, opts ...contactOpt) *models.Contact {
	t.Helper()
	c := &models.Contact{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org,
		PhoneNumber: "+6140" + digitsOf(uuid.New()), ProfileName: "Pat", Tags: models.JSONBArray{}, Metadata: models.JSONB{}}
	for _, o := range opts {
		o(c)
	}
	require.NoError(t, f.db.Create(c).Error)
	return c
}

func digitsOf(u uuid.UUID) string {
	out := []byte{}
	for _, b := range u[:7] {
		out = append(out, '0'+b%10)
	}
	return string(out)
}

func rulesOf(t *testing.T, match string, conds ...map[string]any) models.JSONB {
	t.Helper()
	cs := []any{}
	for _, c := range conds {
		cs = append(cs, c)
	}
	return models.JSONB{"match": match, "conditions": cs}
}

func tagRule(tag string) map[string]any {
	return map[string]any{"field": "tag", "op": "has", "value": tag}
}

func isKind(err, kind error) bool { return errors.Is(err, kind) }

var ctx = context.Background()

// ---------- rules ----------

func TestRules_Validate(t *testing.T) {
	bad := []Rules{
		{Match: "some", Conditions: []Condition{{Field: "tag", Op: "has", Value: json.RawMessage(`"vip"`)}}},
		{Match: "all"},
		{Conditions: []Condition{{Field: "email", Op: "eq", Value: json.RawMessage(`"x"`)}}},
		{Conditions: []Condition{{Field: "tag", Op: "eq", Value: json.RawMessage(`"x"`)}}},
		{Conditions: []Condition{{Field: "tag", Op: "has", Value: json.RawMessage(`""`)}}},
		{Conditions: []Condition{{Field: "metadata", Op: "eq", Key: "a'); DROP TABLE contacts;--", Value: json.RawMessage(`"x"`)}}},
		{Conditions: []Condition{{Field: "last_inbound", Op: "within_days", Value: json.RawMessage(`0`)}}},
		{Conditions: []Condition{{Field: "last_inbound", Op: "within_days", Value: json.RawMessage(`366`)}}},
		{Conditions: []Condition{{Field: "campaign", Op: "read", Value: json.RawMessage(`"not-a-uuid"`)}}},
	}
	for i, r := range bad {
		assert.True(t, isKind(r.Validate(), kit.ErrInvalid), "case %d", i)
	}
	many := Rules{}
	for i := 0; i < maxConditions+1; i++ {
		many.Conditions = append(many.Conditions, Condition{Field: "tag", Op: "has", Value: json.RawMessage(`"x"`)})
	}
	assert.True(t, isKind(many.Validate(), kit.ErrInvalid))

	ok := Rules{Conditions: []Condition{{Field: "metadata", Op: "exists", Key: "suburb"}}}
	require.NoError(t, ok.Validate())
	assert.Equal(t, "all", ok.Match)
}

func TestMaskPhone(t *testing.T) {
	assert.Equal(t, "61••••••678", maskPhone("+61 412 345 678"))
	assert.NotContains(t, maskPhone("+61412345678"), "12345")
}

// ---------- segments ----------

func TestSegments_CRUDAndIsolation(t *testing.T) {
	f := newFixture(t)
	other := testutil.CreateTestOrganization(t, f.db).ID

	seg, err := f.svc.CreateSegment(ctx, f.org, f.user, SegmentInput{Name: " Physio ", Rules: rulesOf(t, "all", tagRule("physio"))})
	require.NoError(t, err)
	assert.Equal(t, "Physio", seg.Name)

	_, err = f.svc.CreateSegment(ctx, f.org, f.user, SegmentInput{Name: "Physio", Rules: rulesOf(t, "all", tagRule("x"))})
	assert.True(t, isKind(err, kit.ErrConflict), "duplicate name")
	_, err = f.svc.CreateSegment(ctx, other, f.user, SegmentInput{Name: "Physio", Rules: rulesOf(t, "all", tagRule("x"))})
	assert.NoError(t, err, "another organization may reuse the name")
	_, err = f.svc.CreateSegment(ctx, f.org, f.user, SegmentInput{Name: "", Rules: rulesOf(t, "all", tagRule("x"))})
	assert.True(t, isKind(err, kit.ErrInvalid))
	_, err = f.svc.CreateSegment(ctx, f.org, f.user, SegmentInput{Name: "Bad", Rules: models.JSONB{"conditions": "nope"}})
	assert.True(t, isKind(err, kit.ErrInvalid))

	f.contact(t, f.org, tags("physio"))
	f.contact(t, other, tags("physio"))
	list, err := f.svc.ListSegments(ctx, f.org)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NotNil(t, list[0].Count)
	assert.Equal(t, 1, *list[0].Count, "counts only this organization's contacts")

	_, err = f.svc.GetSegment(ctx, other, seg.ID)
	assert.True(t, isKind(err, kit.ErrNotFound), "another organization can't read it")
	newName := "Physio patients"
	_, err = f.svc.UpdateSegment(ctx, other, f.user, seg.ID, SegmentPatch{Name: &newName})
	assert.True(t, isKind(err, kit.ErrNotFound), "or change it")
	assert.True(t, isKind(f.svc.DeleteSegment(ctx, other, seg.ID), kit.ErrNotFound), "or delete it")

	upd, err := f.svc.UpdateSegment(ctx, f.org, f.user, seg.ID, SegmentPatch{Name: &newName})
	require.NoError(t, err)
	assert.Equal(t, newName, upd.Name)

	require.NoError(t, f.svc.DeleteSegment(ctx, f.org, seg.ID))
	_, err = f.svc.GetSegment(ctx, f.org, seg.ID)
	assert.True(t, isKind(err, kit.ErrNotFound))
	_, err = f.svc.CreateSegment(ctx, f.org, f.user, SegmentInput{Name: newName, Rules: rulesOf(t, "all", tagRule("x"))})
	assert.NoError(t, err, "a deleted segment's name can be reused")
}

func TestSegments_RefuseAnotherOrgsCampaign(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	c := f.campaign(t, other)
	_, err := f.svc.CreateSegment(ctx, f.org, f.user, SegmentInput{Name: "Readers",
		Rules: rulesOf(t, "all", map[string]any{"field": "campaign", "op": "read", "value": c.ID.String()})})
	assert.True(t, isKind(err, kit.ErrNotFound))
}

// ---------- preview ----------

func TestPreview_RulesAndExclusions(t *testing.T) {
	f := newFixture(t)
	other := testutil.CreateTestOrganization(t, f.db).ID
	f.contact(t, f.org, tags("physio"), named("Alice"), phone("+61 400 000 001"))
	f.contact(t, f.org, tags("physio"), phone("0400000001"), named("Alice again")) // different digits: kept
	f.contact(t, f.org, tags("physio"), phone("+61400000001"))                     // duplicate of the first
	f.contact(t, f.org, tags("physio"), phone("123"))                              // no usable number
	f.contact(t, f.org, tags("physio"), optedOut())
	f.contact(t, f.org, tags("massage"))
	f.contact(t, other, tags("physio"))

	in := PreviewInput{Rules: rulesOf(t, "all", tagRule("physio"))}
	p, err := f.svc.Preview(ctx, f.org, in)
	require.NoError(t, err)
	assert.Equal(t, 5, p.Matched)
	assert.Equal(t, 3, p.Recipients, "opt-outs count only for marketing templates")
	assert.Equal(t, Excluded{Duplicates: 1, NoNumber: 1}, p.Excluded)
	assert.Equal(t, 1, p.MarketingOptOut, "shown before a template is chosen (R2)")
	require.Len(t, p.Sample, 3)
	for _, s := range p.Sample {
		assert.Contains(t, s.PhoneMasked, "•")
	}

	in.TemplateID = &f.tmpl.ID
	p, err = f.svc.Preview(ctx, f.org, in)
	require.NoError(t, err)
	assert.Equal(t, 2, p.Recipients)
	assert.Equal(t, 1, p.Excluded.OptedOut)
	assert.Equal(t, 1, p.MarketingOptOut)

	in.TemplateID = &f.util.ID
	p, err = f.svc.Preview(ctx, f.org, in)
	require.NoError(t, err)
	assert.Equal(t, 3, p.Recipients, "utility templates still reach opted-out contacts")

	foreign := testutil.CreateTestTemplate(t, f.db, other, "x")
	in.TemplateID = &foreign.ID
	_, err = f.svc.Preview(ctx, f.org, in)
	assert.True(t, isKind(err, kit.ErrNotFound), "another organization's template")
}

func TestPreview_Fields(t *testing.T) {
	f := newFixture(t)
	f.contact(t, f.org, meta(models.JSONB{"suburb": "Newtown", "plan": "Gold"}))
	f.contact(t, f.org, meta(models.JSONB{"suburb": "newtown"}))
	f.contact(t, f.org, meta(models.JSONB{"suburb": "Glebe 100%_off"}))
	f.contact(t, f.org)

	count := func(conds ...map[string]any) int {
		t.Helper()
		p, err := f.svc.Preview(ctx, f.org, PreviewInput{Rules: rulesOf(t, "all", conds...)})
		require.NoError(t, err)
		return p.Matched
	}
	assert.Equal(t, 2, count(map[string]any{"field": "metadata", "op": "eq", "key": "suburb", "value": "NEWTOWN"}))
	assert.Equal(t, 2, count(map[string]any{"field": "metadata", "op": "neq", "key": "suburb", "value": "newtown"}))
	assert.Equal(t, 1, count(map[string]any{"field": "metadata", "op": "contains", "key": "suburb", "value": "100%_"}), "LIKE wildcards are literal")
	assert.Equal(t, 0, count(map[string]any{"field": "metadata", "op": "contains", "key": "suburb", "value": "new%town"}))
	assert.Equal(t, 1, count(map[string]any{"field": "metadata", "op": "exists", "key": "plan"}))
	assert.Equal(t, 3, count(map[string]any{"field": "metadata", "op": "not_exists", "key": "plan"}))
	assert.Equal(t, 0, count(map[string]any{"field": "metadata", "op": "eq", "key": "suburb", "value": "x' OR '1'='1"}), "values are parameters")
	assert.Equal(t, 4, count(map[string]any{"field": "created", "op": "within_days", "value": 1}))
	assert.Equal(t, 4, count(map[string]any{"field": "last_inbound", "op": "not_within_days", "value": 30}), "never wrote counts as not recent")
	assert.Equal(t, 0, count(map[string]any{"field": "whatsapp_account", "op": "eq", "value": f.account}))
	assert.Equal(t, 3, count(map[string]any{"field": "metadata", "op": "exists", "key": "suburb"}, map[string]any{"field": "created", "op": "within_days", "value": 1}))
	p, err := f.svc.Preview(ctx, f.org, PreviewInput{Rules: rulesOf(t, "any",
		map[string]any{"field": "metadata", "op": "exists", "key": "plan"},
		map[string]any{"field": "metadata", "op": "eq", "key": "suburb", "value": "glebe 100%_off"})})
	require.NoError(t, err)
	assert.Equal(t, 2, p.Matched, "match any")
}

// ---------- campaigns, retargets and sends ----------

func (f *fixture) campaign(t *testing.T, owner *fixture) *models.BulkMessageCampaign {
	t.Helper()
	c := &models.BulkMessageCampaign{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: owner.org, WhatsAppAccount: owner.account,
		Name: "Winter", TemplateID: owner.tmpl.ID, Status: models.CampaignStatusCompleted, CreatedBy: owner.user}
	require.NoError(t, f.db.Create(c).Error)
	return c
}

func (f *fixture) recipient(t *testing.T, c *models.BulkMessageCampaign, contact *models.Contact, status models.MessageStatus, at time.Time) {
	t.Helper()
	r := &models.BulkMessageRecipient{BaseModel: models.BaseModel{ID: uuid.New()}, CampaignID: c.ID, PhoneNumber: normalisePhone(contact.PhoneNumber),
		Status: status, SentAt: &at}
	if status == models.MessageStatusRead {
		r.ReadAt = &at
	}
	require.NoError(t, f.db.Create(r).Error)
}

func (f *fixture) reply(t *testing.T, contact *models.Contact, at time.Time) {
	t.Helper()
	m := &models.Message{BaseModel: models.BaseModel{ID: uuid.New(), CreatedAt: at}, OrganizationID: f.org, WhatsAppAccount: f.account,
		ContactID: contact.ID, Direction: models.DirectionIncoming, MessageType: "text"}
	require.NoError(t, f.db.Create(m).Error)
}

func TestRetarget(t *testing.T) {
	f := newFixture(t)
	c := f.campaign(t, f)
	sent := time.Now().Add(-48 * time.Hour)
	failed := f.contact(t, f.org)
	delivered := f.contact(t, f.org)
	readSilent := f.contact(t, f.org)
	readReplied := f.contact(t, f.org)
	f.contact(t, f.org) // not in the campaign
	f.recipient(t, c, failed, models.MessageStatusFailed, sent)
	f.recipient(t, c, delivered, models.MessageStatusDelivered, sent)
	f.recipient(t, c, readSilent, models.MessageStatusRead, sent)
	f.recipient(t, c, readReplied, models.MessageStatusRead, sent)
	f.reply(t, readReplied, sent.Add(time.Hour))
	f.reply(t, readSilent, sent.Add(-time.Hour)) // before the campaign: not a reply

	cases := map[string]int{"not_received": 1, "not_read": 1, "read_no_reply": 1, "replied": 1}
	for filter, want := range cases {
		p, err := f.svc.Preview(ctx, f.org, PreviewInput{Retarget: &Retarget{CampaignID: c.ID, Filter: filter}})
		require.NoError(t, err, filter)
		assert.Equal(t, want, p.Matched, filter)
	}
	_, err := f.svc.Preview(ctx, f.org, PreviewInput{Retarget: &Retarget{CampaignID: c.ID, Filter: "read"}})
	assert.True(t, isKind(err, kit.ErrInvalid), "only the retarget filters")

	other := newFixture(t)
	_, err = other.svc.Preview(ctx, other.org, PreviewInput{Retarget: &Retarget{CampaignID: c.ID, Filter: "not_read"}})
	assert.True(t, isKind(err, kit.ErrNotFound), "another organization's campaign")
}

func sendVars() Variables {
	return Variables{Body: map[string]VarSource{
		"name": {Source: "name", Fallback: "there"},
		"1":    {Source: "metadata", Key: "plan", Fallback: "membership"},
	}}
}

func TestCreateSend(t *testing.T) {
	f := newFixture(t)
	f.contact(t, f.org, tags("vip"), named("Alice"), meta(models.JSONB{"plan": "Gold"}))
	f.contact(t, f.org, tags("vip"), named(""))
	f.contact(t, f.org, tags("vip"), optedOut())
	seg, err := f.svc.CreateSegment(ctx, f.org, f.user, SegmentInput{Name: "VIP", Rules: rulesOf(t, "all", tagRule("vip"))})
	require.NoError(t, err)

	in := SendInput{Name: "VIP winter", WhatsAppAccount: f.account, TemplateID: f.tmpl.ID, SegmentID: &seg.ID, Variables: sendVars(), ConfirmCount: 3}
	_, err = f.svc.CreateSend(ctx, f.org, f.user, in)
	assert.True(t, isKind(err, kit.ErrConflict), "the confirmed count must match what would be sent")

	missing := in
	missing.Variables = Variables{Body: map[string]VarSource{"name": {Source: "name"}}}
	_, err = f.svc.CreateSend(ctx, f.org, f.user, missing)
	assert.True(t, isKind(err, kit.ErrInvalid), "every variable needs a value")

	badKey := in
	badKey.Variables = Variables{Body: map[string]VarSource{"name": {Source: "name"}, "1": {Source: "metadata", Key: "x;drop"}}}
	_, err = f.svc.CreateSend(ctx, f.org, f.user, badKey)
	assert.True(t, isKind(err, kit.ErrInvalid))

	both := in
	both.Retarget = &Retarget{CampaignID: uuid.New(), Filter: "not_read"}
	_, err = f.svc.CreateSend(ctx, f.org, f.user, both)
	assert.True(t, isKind(err, kit.ErrInvalid), "segment or retarget, not both")

	require.NoError(t, f.db.Model(&models.Template{}).Where("id = ?", f.util.ID).Update("status", "PENDING").Error)
	pending := in
	pending.TemplateID, pending.Variables = f.util.ID, Variables{}
	_, err = f.svc.CreateSend(ctx, f.org, f.user, pending)
	assert.True(t, isKind(err, kit.ErrInvalid), "templates must be approved")

	acc2 := testutil.CreateTestWhatsAppAccount(t, f.db, f.org)
	wrongAcc := in
	wrongAcc.WhatsAppAccount = acc2.Name
	_, err = f.svc.CreateSend(ctx, f.org, f.user, wrongAcc)
	assert.True(t, isKind(err, kit.ErrInvalid), "template from another number")

	other := newFixture(t)
	_, err = other.svc.CreateSend(ctx, other.org, other.user, SendInput{Name: "x", WhatsAppAccount: other.account, TemplateID: other.tmpl.ID,
		SegmentID: &seg.ID, Variables: sendVars(), ConfirmCount: 2})
	assert.True(t, isKind(err, kit.ErrNotFound), "another organization's segment")

	in.ConfirmCount = 2
	res, err := f.svc.CreateSend(ctx, f.org, f.user, in)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Recipients)
	assert.Equal(t, 1, res.Excluded.OptedOut)

	var camp models.BulkMessageCampaign
	require.NoError(t, f.db.Where("organization_id = ? AND id = ?", f.org, res.CampaignID).First(&camp).Error)
	assert.Equal(t, models.CampaignStatusDraft, camp.Status)
	assert.Equal(t, 2, camp.TotalRecipients)
	assert.Equal(t, f.user, camp.CreatedBy)

	var recs []models.BulkMessageRecipient
	require.NoError(t, f.db.Where("campaign_id = ?", camp.ID).Order("recipient_name DESC").Find(&recs).Error)
	require.Len(t, recs, 2)
	assert.Equal(t, "Alice", recs[0].TemplateParams["name"])
	assert.Equal(t, "Gold", recs[0].TemplateParams["1"])
	assert.Equal(t, "there", recs[1].TemplateParams["name"], "fallback for an empty name")
	assert.Equal(t, "membership", recs[1].TemplateParams["1"], "fallback for a missing field")
	assert.Equal(t, models.MessageStatusPending, recs[0].Status)
	assert.NotContains(t, recs[0].PhoneNumber, "+")

	sends, err := f.svc.ListSends(ctx, f.org, &seg.ID, nil)
	require.NoError(t, err)
	require.Len(t, sends, 1)
	assert.Equal(t, "VIP winter", sends[0].CampaignName)
	assert.Equal(t, 2, sends[0].RecipientCount)
	require.NotNil(t, sends[0].CreatedBy)
	assert.NotEmpty(t, sends[0].CreatedBy.Name)

	none, err := other.svc.ListSends(ctx, other.org, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, none, "history is per organization")

	segs, err := f.svc.ListSegments(ctx, f.org)
	require.NoError(t, err)
	assert.NotNil(t, segs[0].LastUsedAt)
}

func TestCreateSend_Retarget(t *testing.T) {
	f := newFixture(t)
	c := f.campaign(t, f)
	silent := f.contact(t, f.org)
	f.recipient(t, c, silent, models.MessageStatusDelivered, time.Now().Add(-48*time.Hour))
	res, err := f.svc.CreateSend(ctx, f.org, f.user, SendInput{Name: "Winter follow-up", WhatsAppAccount: f.account, TemplateID: f.util.ID,
		Retarget: &Retarget{CampaignID: c.ID, Filter: "not_read"}, ConfirmCount: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Recipients)
	sends, err := f.svc.ListSends(ctx, f.org, nil, &c.ID)
	require.NoError(t, err)
	require.Len(t, sends, 1)
	assert.Equal(t, "retarget", sends[0].Kind)
	require.NotNil(t, sends[0].RetargetFilter)
	assert.Equal(t, "not_read", *sends[0].RetargetFilter)
}

func TestFields(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	require.NoError(t, f.db.Create(&models.Tag{OrganizationID: f.org, Name: "physio"}).Error)
	require.NoError(t, f.db.Create(&models.Tag{OrganizationID: other.org, Name: "secret-tag"}).Error)
	f.contact(t, f.org, meta(models.JSONB{"suburb": "Newtown", "bad key!": "x"}))
	other.contact(t, other.org, meta(models.JSONB{"other_org_field": "x"}))
	f.campaign(t, f)

	out, err := f.svc.Fields(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, []string{"physio"}, out.Tags)
	assert.Equal(t, []string{"suburb"}, out.MetadataKeys, "unsafe keys and other organizations' fields are left out")
	assert.Equal(t, []string{f.account}, out.Accounts)
	require.Len(t, out.Campaigns, 1)
	require.Len(t, out.Templates, 2)
	for _, tp := range out.Templates {
		if tp.ID == f.tmpl.ID {
			assert.ElementsMatch(t, []string{"name", "1"}, tp.BodyVars)
		}
	}
}

func TestPick_Limit(t *testing.T) {
	rows := make([]contactRow, 0, MaxRecipients+5)
	for i := 0; i < MaxRecipients+5; i++ {
		rows = append(rows, contactRow{PhoneNumber: "+61" + strconv.Itoa(400000000+i)})
	}
	kept, ex := pick(rows, false, false)
	assert.Len(t, kept, MaxRecipients)
	assert.Equal(t, 5, ex.OverLimit)
}

// Consent (step 2.2, R9): the consent rule and "marketing only to people who opted in".
func TestConsentRuleAndRequireOptIn(t *testing.T) {
	f := newFixture(t)
	cs := consent.NewService(f.db, nil, nil, nil)
	in := f.contact(t, f.org, tags("flu"))
	out := f.contact(t, f.org, tags("flu"))
	unknown := f.contact(t, f.org, tags("flu"))
	metaOut := f.contact(t, f.org, tags("flu"), optedOut()) // Meta's flag, no record yet
	_, err := cs.Record(ctx, f.org, f.user, in.ID, consent.StaffInput{Action: consent.ActionOptIn, Evidence: "form"})
	require.NoError(t, err)
	_, err = cs.Record(ctx, f.org, f.user, out.ID, consent.StaffInput{Action: consent.ActionOptOut})
	require.NoError(t, err)

	ids := func(op string) []uuid.UUID {
		r, err := parseRules(rulesOf(t, "all", tagRule("flu"), map[string]any{"field": "consent", "op": op}))
		require.NoError(t, err)
		rows, err := f.svc.evaluate(ctx, f.org, r)
		require.NoError(t, err)
		out := []uuid.UUID{}
		for _, c := range rows {
			out = append(out, c.ID)
		}
		return out
	}
	assert.ElementsMatch(t, []uuid.UUID{in.ID}, ids("opted_in"))
	assert.ElementsMatch(t, []uuid.UUID{out.ID, metaOut.ID}, ids("opted_out"))
	assert.ElementsMatch(t, []uuid.UUID{unknown.ID}, ids("not_recorded"))
	_, err = parseRules(rulesOf(t, "all", map[string]any{"field": "consent", "op": "maybe"}))
	assert.True(t, isKind(err, kit.ErrInvalid))

	flu := PreviewInput{Rules: rulesOf(t, "all", tagRule("flu")), TemplateID: &f.tmpl.ID}
	p, err := f.svc.Preview(ctx, f.org, flu)
	require.NoError(t, err)
	assert.Equal(t, 2, p.Recipients, "opt-in not required: in and unknown")
	assert.Equal(t, 1, p.NotRecorded)
	assert.False(t, p.RequireOptIn)

	yes := true
	_, err = cs.SaveSettings(ctx, f.org, f.user, consent.SettingsPatch{RequireOptIn: &yes})
	require.NoError(t, err)
	p, err = f.svc.Preview(ctx, f.org, flu)
	require.NoError(t, err)
	assert.Equal(t, 1, p.Recipients, "only the opted-in contact")
	assert.Equal(t, 1, p.Excluded.NoOptIn)
	assert.Equal(t, 2, p.Excluded.OptedOut)
	assert.True(t, p.RequireOptIn)

	flu.TemplateID = &f.util.ID
	p, err = f.svc.Preview(ctx, f.org, flu)
	require.NoError(t, err)
	assert.Equal(t, 4, p.Recipients, "utility templates aren't marketing")
}
