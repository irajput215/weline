package consent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var ctx = context.Background()

// fakeSender records replies instead of calling Meta.
type fakeSender struct {
	mu   sync.Mutex
	sent []string
	fail bool
}

func (f *fakeSender) SendText(_ context.Context, _, _ uuid.UUID, text string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return uuid.Nil, kit.ErrSendFailed
	}
	f.sent = append(f.sent, text)
	return uuid.New(), nil
}

func (f *fakeSender) SendTemplate(context.Context, uuid.UUID, uuid.UUID, string, string, map[string]string) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

type fixture struct {
	db     *gorm.DB
	rdb    *redis.Client
	svc    *Service
	sender *fakeSender
	org    uuid.UUID
	user   uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	rdb := testutil.SetupTestRedis(t)
	s := &fakeSender{}
	return &fixture{db: db, rdb: rdb, svc: NewService(db, rdb, s, nil), sender: s, org: org.ID, user: user.ID}
}

var phoneSeq int64 = time.Now().UnixNano() % 1_000_000

func (f *fixture) contact(t *testing.T, org uuid.UUID, opts ...func(*models.Contact)) *models.Contact {
	t.Helper()
	phoneSeq++
	c := &models.Contact{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org,
		PhoneNumber: fmt.Sprintf("61499%06d", phoneSeq), ProfileName: "Pat", Tags: models.JSONBArray{"vip"}, Metadata: models.JSONB{}}
	for _, o := range opts {
		o(c)
	}
	require.NoError(t, f.db.Create(c).Error)
	return c
}

func (f *fixture) reload(t *testing.T, id uuid.UUID) models.Contact {
	t.Helper()
	var c models.Contact
	require.NoError(t, f.db.Where("id = ?", id).First(&c).Error)
	return c
}

func (f *fixture) events(t *testing.T, contact uuid.UUID) []eventRow {
	t.Helper()
	var evs []eventRow
	require.NoError(t, f.db.Where("organization_id = ? AND contact_id = ?", f.org, contact).Order("recorded_at, id").Find(&evs).Error)
	return evs
}

func incoming(org, contact uuid.UUID, typ, text string) kit.Event {
	b, _ := json.Marshal(handlers.MessageEventData{MessageID: uuid.NewString(), ContactID: contact.String(),
		MessageType: models.MessageType(typ), Content: text, Direction: models.DirectionIncoming})
	return kit.Event{Name: "message.incoming", OrgID: org, Timestamp: time.Now(), Data: b}
}

func hasTag(c models.Contact, tag string) bool {
	for _, t := range c.Tags {
		if s, _ := t.(string); strings.EqualFold(s, tag) {
			return true
		}
	}
	return false
}

func isKind(err, kind error) bool { return errors.Is(err, kind) }

// ---------- matching ----------

func TestMatch(t *testing.T) {
	st := Defaults()
	st.OptInPhrases = []string{normalise("Yes, send me updates")}
	cases := map[string]string{
		"STOP": ActionOptOut, "stop.": ActionOptOut, " Stop messages ": ActionOptOut, "UNSUBSCRIBE!": ActionOptOut,
		"opt-out": ActionOptOut, "START": ActionOptIn, "Yes please 🙂": ActionOptIn, "yes, send me updates": ActionOptIn,
		"please don't stop my appointment": "", "cancel": "", "stop " + strings.Repeat("x", 60): "", "": "", "STOPP": "",
	}
	for text, want := range cases {
		got, _, _ := match(st, text)
		assert.Equal(t, want, got, "%q", text)
	}
	_, src, kw := match(st, "Yes, send me updates")
	assert.Equal(t, SourceButton, src)
	assert.Equal(t, "YES SEND ME UPDATES", kw)
}

// ---------- keywords end to end ----------

func TestStopAndStart(t *testing.T) {
	f := newFixture(t)
	c := f.contact(t, f.org)

	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "text", "Stop!")))
	got := f.reload(t, c.ID)
	assert.True(t, got.MarketingOptOut, "Weline's flag set")
	assert.True(t, hasTag(got, "opted-out"))
	assert.True(t, hasTag(got, "vip"), "other tags kept")
	evs := f.events(t, c.ID)
	require.Len(t, evs, 1)
	assert.Equal(t, ActionOptOut, evs[0].Action)
	assert.Equal(t, SourceKeyword, evs[0].Source)
	assert.Equal(t, "STOP", evs[0].Keyword, "the keyword, not the message")
	assert.NotNil(t, evs[0].MessageID)
	assert.Nil(t, evs[0].Previous)
	require.Equal(t, 1, f.sender.count())
	assert.Contains(t, f.sender.sent[0], "Reply START")
	assert.NotContains(t, f.sender.sent[0], "{business}")

	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "text", "STOP")))
	assert.Len(t, f.events(t, c.ID), 1, "repeating STOP records nothing")
	assert.Equal(t, 1, f.sender.count(), "and sends nothing")

	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "text", "start")))
	got = f.reload(t, c.ID)
	assert.False(t, got.MarketingOptOut)
	assert.False(t, hasTag(got, "opted-out"))
	evs = f.events(t, c.ID)
	require.Len(t, evs, 2)
	assert.Equal(t, ActionOptIn, evs[1].Action)
	require.NotNil(t, evs[1].Previous)
	assert.Equal(t, OptedOut, *evs[1].Previous)
	assert.Equal(t, 2, f.sender.count())

	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "text", "please stop calling me at work")))
	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "image", "STOP")))
	assert.Len(t, f.events(t, c.ID), 2, "ordinary messages and media are ignored")
}

func TestButtonPhraseAndCustomLists(t *testing.T) {
	f := newFixture(t)
	c := f.contact(t, f.org)
	phrases := []string{"Yes, send me updates"}
	out := []string{"Remove me"}
	_, err := f.svc.SaveSettings(ctx, f.org, f.user, SettingsPatch{OptInPhrases: &phrases, OptOutKeywords: &out})
	require.NoError(t, err)

	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "button", "Yes, send me updates")))
	evs := f.events(t, c.ID)
	require.Len(t, evs, 1)
	assert.Equal(t, SourceButton, evs[0].Source)

	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "text", "STOP")))
	assert.Len(t, f.events(t, c.ID), 1, "STOP was replaced by the custom list")
	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "text", "remove me")))
	assert.True(t, f.reload(t, c.ID).MarketingOptOut)

	both := []string{"STOP", "remove me"}
	_, err = f.svc.SaveSettings(ctx, f.org, f.user, SettingsPatch{OptInKeywords: &both})
	assert.True(t, isKind(err, kit.ErrInvalid), "a word in both lists")
	empty := []string{" ", "!!"}
	_, err = f.svc.SaveSettings(ctx, f.org, f.user, SettingsPatch{OptOutKeywords: &empty})
	assert.True(t, isKind(err, kit.ErrInvalid), "at least one opt-out keyword")
	bad := "sometimes"
	_, err = f.svc.SaveSettings(ctx, f.org, f.user, SettingsPatch{ServiceAfterStop: &bad})
	assert.True(t, isKind(err, kit.ErrInvalid))
}

func TestReplyFailureKeepsChange(t *testing.T) {
	f := newFixture(t)
	f.sender.fail = true
	c := f.contact(t, f.org)
	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "text", "STOP")))
	assert.True(t, f.reload(t, c.ID).MarketingOptOut)
}

func TestIncomingForAnotherOrgsContact(t *testing.T) {
	f := newFixture(t)
	other := testutil.CreateTestOrganization(t, f.db).ID
	theirs := f.contact(t, other)
	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, theirs.ID, "text", "STOP")))
	assert.False(t, f.reload(t, theirs.ID).MarketingOptOut, "an event for org A can't touch org B's contact")
	assert.Zero(t, f.sender.count())
}

func TestDuplicateEventChangesOnce(t *testing.T) {
	f := newFixture(t)
	c := f.contact(t, f.org)
	e := incoming(f.org, c.ID, "text", "STOP")
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, f.svc.HandleIncoming(ctx, e))
		}()
	}
	wg.Wait()
	assert.Len(t, f.events(t, c.ID), 1)
	assert.Equal(t, 1, f.sender.count())
}

// ---------- Meta's signal ----------

func TestSweepRecordsMetaChanges(t *testing.T) {
	f := newFixture(t)
	if f.rdb != nil {
		require.NoError(t, f.rdb.Del(ctx, sweepLock).Err())
	}
	c := f.contact(t, f.org)
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("marketing_opt_out", true).Error) // Meta "stop"

	n, err := f.svc.Sweep(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)
	evs := f.events(t, c.ID)
	require.Len(t, evs, 1)
	assert.Equal(t, SourceMeta, evs[0].Source)
	assert.Equal(t, ActionOptOut, evs[0].Action)
	assert.True(t, hasTag(f.reload(t, c.ID), "opted-out"))

	if f.rdb != nil {
		n, err = f.svc.Sweep(ctx)
		require.NoError(t, err)
		assert.Zero(t, n, "the lock stops a second sweep within 5 minutes")
		require.NoError(t, f.rdb.Del(ctx, sweepLock).Err())
	}
	_, err = f.svc.Sweep(ctx)
	require.NoError(t, err)
	assert.Len(t, f.events(t, c.ID), 1, "nothing new to record")

	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("marketing_opt_out", false).Error) // Meta "resume"
	if f.rdb != nil {
		require.NoError(t, f.rdb.Del(ctx, sweepLock).Err())
	}
	_, err = f.svc.Sweep(ctx)
	require.NoError(t, err)
	evs = f.events(t, c.ID)
	require.Len(t, evs, 2)
	assert.Equal(t, ActionOptIn, evs[1].Action)
	assert.Equal(t, SourceMeta, evs[1].Source)
}

func TestStartAfterUnsweptMetaStopRecordsBoth(t *testing.T) {
	f := newFixture(t)
	c := f.contact(t, f.org)
	_, err := f.svc.Record(ctx, f.org, f.user, c.ID, StaffInput{Action: ActionOptIn, Evidence: "form"})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("marketing_opt_out", true).Error)

	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "text", "START")))
	evs := f.events(t, c.ID)
	require.Len(t, evs, 3)
	assert.Equal(t, []string{SourceStaff, SourceMeta, SourceKeyword}, []string{evs[0].Source, evs[1].Source, evs[2].Source})
	assert.False(t, f.reload(t, c.ID).MarketingOptOut)
	assert.Equal(t, 1, f.sender.count(), "START was a real change")
}

// ---------- staff and checks ----------

func TestRecordByStaff(t *testing.T) {
	f := newFixture(t)
	c := f.contact(t, f.org)
	_, err := f.svc.Record(ctx, f.org, f.user, c.ID, StaffInput{Action: ActionOptIn})
	assert.True(t, isKind(err, kit.ErrInvalid), "opt-in needs evidence")
	_, err = f.svc.Record(ctx, f.org, f.user, c.ID, StaffInput{Action: ActionOptIn, Evidence: "form", GivenAt: time.Now().AddDate(0, 0, 3).Format("2006-01-02")})
	assert.True(t, isKind(err, kit.ErrInvalid), "future date")
	_, err = f.svc.Record(ctx, f.org, f.user, c.ID, StaffInput{Action: ActionOptIn, Evidence: "form", GivenAt: "2001-01-01"})
	assert.True(t, isKind(err, kit.ErrInvalid), "more than 10 years ago")
	_, err = f.svc.Record(ctx, f.org, f.user, c.ID, StaffInput{Action: "maybe"})
	assert.True(t, isKind(err, kit.ErrInvalid))

	res, err := f.svc.Record(ctx, f.org, f.user, c.ID, StaffInput{Action: ActionOptIn, Evidence: "signed new patient form", GivenAt: "2026-03-03"})
	require.NoError(t, err)
	assert.True(t, res.Changed)
	evs := f.events(t, c.ID)
	require.Len(t, evs, 1)
	assert.Equal(t, SourceStaff, evs[0].Source)
	require.NotNil(t, evs[0].RecordedByID)
	assert.Equal(t, f.user, *evs[0].RecordedByID)
	assert.Equal(t, "2026-03-03", evs[0].GivenAt.UTC().Format("2006-01-02"))

	res, err = f.svc.Record(ctx, f.org, f.user, c.ID, StaffInput{Action: ActionOptIn, Evidence: "also ticked online form"})
	require.NoError(t, err)
	assert.False(t, res.Changed)
	assert.True(t, res.Recorded, "extra proof is kept")

	res, err = f.svc.Record(ctx, f.org, f.user, c.ID, StaffInput{Action: ActionOptOut})
	require.NoError(t, err, "opt-out needs no note")
	assert.True(t, res.Changed)
	assert.Zero(t, f.sender.count(), "staff changes send no WhatsApp reply")

	other := testutil.CreateTestOrganization(t, f.db).ID
	theirs := f.contact(t, other)
	_, err = f.svc.Record(ctx, f.org, f.user, theirs.ID, StaffInput{Action: ActionOptOut})
	assert.True(t, isKind(err, kit.ErrNotFound))
}

func TestAllowed(t *testing.T) {
	f := newFixture(t)
	unknown := f.contact(t, f.org)
	in := f.contact(t, f.org)
	out := f.contact(t, f.org)
	_, err := f.svc.Record(ctx, f.org, f.user, in.ID, StaffInput{Action: ActionOptIn, Evidence: "form"})
	require.NoError(t, err)
	_, err = f.svc.Record(ctx, f.org, f.user, out.ID, StaffInput{Action: ActionOptOut})
	require.NoError(t, err)

	allowed := func(c *models.Contact, p Purpose) bool {
		ok, _, err := f.svc.Allowed(ctx, f.org, c.ID, p)
		require.NoError(t, err)
		return ok
	}
	assert.True(t, allowed(unknown, PurposeMarketing), "opt-in not required by default")
	assert.False(t, allowed(out, PurposeMarketing))
	assert.True(t, allowed(out, PurposeService), "STOP covers marketing only by default")

	yes, stop := true, ServiceStop
	_, err = f.svc.SaveSettings(ctx, f.org, f.user, SettingsPatch{RequireOptIn: &yes, ServiceAfterStop: &stop})
	require.NoError(t, err)
	assert.False(t, allowed(unknown, PurposeMarketing))
	_, reason, _ := f.svc.Allowed(ctx, f.org, unknown.ID, PurposeMarketing)
	assert.Equal(t, ReasonNoOptIn, reason)
	assert.True(t, allowed(in, PurposeMarketing))
	assert.False(t, allowed(out, PurposeService))
	assert.True(t, allowed(unknown, PurposeService))
}

func TestNeverInferred(t *testing.T) {
	f := newFixture(t)
	c := f.contact(t, f.org)
	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, c.ID, "text", "Hi, can I book for Tuesday?")))
	got, err := f.svc.Contact(ctx, f.org, c.ID)
	require.NoError(t, err)
	assert.Equal(t, NotRecorded, got.Marketing)
	assert.Empty(t, got.Events)
}

func TestSummaryEventsSearch(t *testing.T) {
	f := newFixture(t)
	a := f.contact(t, f.org, func(c *models.Contact) { c.ProfileName = "Alice Nguyen" })
	b := f.contact(t, f.org, func(c *models.Contact) { c.ProfileName = "Ben" })
	f.contact(t, f.org)
	other := testutil.CreateTestOrganization(t, f.db).ID
	f.contact(t, other, func(c *models.Contact) { c.ProfileName = "Alice Elsewhere" })
	_, err := f.svc.Record(ctx, f.org, f.user, a.ID, StaffInput{Action: ActionOptIn, Evidence: "form"})
	require.NoError(t, err)
	require.NoError(t, f.svc.HandleIncoming(ctx, incoming(f.org, b.ID, "text", "STOP")))

	sum, err := f.svc.Summary(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, Summary{OptedIn: 1, OptedOut: 1, NotRecorded: 1, TotalContacts: 3}, *sum)

	evs, err := f.svc.Events(ctx, f.org, EventFilter{})
	require.NoError(t, err)
	require.Len(t, evs, 2)
	assert.Equal(t, "Ben", evs[0].Contact.Name, "newest first")
	assert.Contains(t, evs[0].Contact.PhoneMasked, "•")
	require.NotNil(t, evs[1].RecordedBy)
	assert.NotEmpty(t, evs[1].RecordedBy.Name)
	evs, err = f.svc.Events(ctx, f.org, EventFilter{Source: SourceKeyword})
	require.NoError(t, err)
	assert.Len(t, evs, 1)
	_, err = f.svc.Events(ctx, f.org, EventFilter{Source: "rumour"})
	assert.True(t, isKind(err, kit.ErrInvalid))
	none, err := f.svc.Events(ctx, other, EventFilter{})
	require.NoError(t, err)
	assert.Empty(t, none)

	found, err := f.svc.Search(ctx, f.org, "alice")
	require.NoError(t, err)
	require.Len(t, found, 1, "another organization's Alice isn't found")
	assert.Equal(t, OptedIn, found[0].Marketing)
	found, err = f.svc.Search(ctx, f.org, b.PhoneNumber[len(b.PhoneNumber)-6:])
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, OptedOut, found[0].Marketing)
	_, err = f.svc.Search(ctx, f.org, "a")
	assert.True(t, isKind(err, kit.ErrInvalid))
}

// ---------- imports ----------

func TestImport(t *testing.T) {
	f := newFixture(t)
	if f.rdb == nil {
		t.Skip("TEST_REDIS_URL not set")
	}
	matched := f.contact(t, f.org)
	already := f.contact(t, f.org)
	laterOut := f.contact(t, f.org)
	other := testutil.CreateTestOrganization(t, f.db).ID
	theirs := f.contact(t, other)
	_, err := f.svc.Record(ctx, f.org, f.user, already.ID, StaffInput{Action: ActionOptIn, Evidence: "form"})
	require.NoError(t, err)
	_, err = f.svc.Record(ctx, f.org, f.user, laterOut.ID, StaffInput{Action: ActionOptOut, GivenAt: "2026-06-01"})
	require.NoError(t, err)

	local := "0" + matched.PhoneNumber[2:] // as typed in Australia
	csv := "Phone,Given_At,Note\n" +
		local + ",2026-03-01,paper form\n" +
		already.PhoneNumber + ",2026-03-01,\n" +
		laterOut.PhoneNumber + ",2026-03-01,\n" +
		theirs.PhoneNumber + ",2026-03-01,\n" +
		"61400000000,2026-03-01,\n" +
		"12,2026-03-01,\n" +
		matched.PhoneNumber + ",2026-02-01,dup\n" +
		"61411111111,next year,\n"
	p, err := f.svc.PreviewImport(ctx, f.org, []byte(csv))
	require.NoError(t, err)
	assert.Equal(t, 8, p.RowsTotal)
	assert.Equal(t, 1, p.WillApply)
	assert.Equal(t, 1, p.AlreadyOptedIn)
	assert.Equal(t, 1, p.OptedOutLater)
	assert.Equal(t, 2, p.UnknownNumbers, "another org's number counts as unknown")
	assert.Equal(t, 3, p.Invalid)

	_, err = f.svc.ApplyImport(ctx, f.org, f.user, ImportInput{Token: p.Token, Evidence: "", ConfirmCount: 1})
	assert.True(t, isKind(err, kit.ErrInvalid), "proof required")
	_, err = f.svc.ApplyImport(ctx, f.org, f.user, ImportInput{Token: p.Token, Evidence: "forms", ConfirmCount: 2})
	assert.True(t, isKind(err, kit.ErrConflict))
	_, err = f.svc.ApplyImport(ctx, other, f.user, ImportInput{Token: p.Token, Evidence: "forms", ConfirmCount: 1})
	assert.True(t, isKind(err, kit.ErrGone), "a token is per organization")

	res, err := f.svc.ApplyImport(ctx, f.org, f.user, ImportInput{Token: p.Token, Filename: "forms.csv", Evidence: "paper forms 2026", ConfirmCount: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Applied)
	evs := f.events(t, matched.ID)
	require.Len(t, evs, 1)
	assert.Equal(t, SourceImport, evs[0].Source)
	assert.Equal(t, "paper form", evs[0].Evidence)
	require.NotNil(t, evs[0].ImportID)
	var n int64
	require.NoError(t, f.db.Model(&models.Contact{}).Where("phone_number = ?", "61400000000").Count(&n).Error)
	assert.Zero(t, n, "unknown numbers aren't created")
	assert.False(t, f.reload(t, theirs.ID).MarketingOptOut)

	_, err = f.svc.ApplyImport(ctx, f.org, f.user, ImportInput{Token: p.Token, Evidence: "again", ConfirmCount: 1})
	assert.True(t, isKind(err, kit.ErrGone), "a token is used once")

	_, err = f.svc.PreviewImport(ctx, f.org, []byte("number,date\n1,2"))
	assert.True(t, isKind(err, kit.ErrInvalid), "columns named")
	_, err = f.svc.PreviewImport(ctx, f.org, []byte(strings.Repeat("x", maxImportBytes+1)))
	assert.True(t, isKind(err, kit.ErrInvalid), "1 MB limit")
	big := "phone,given_at\n" + strings.Repeat("61400000001,2026-01-01\n", maxImportRows+1)
	_, err = f.svc.PreviewImport(ctx, f.org, []byte(big))
	assert.True(t, isKind(err, kit.ErrInvalid), "row limit")
}

func TestNormalise(t *testing.T) {
	assert.Equal(t, "STOP MESSAGES", normalise("  stop,   messages!! "))
	assert.Equal(t, "OPT OUT", normalise("opt-out"))
	assert.Equal(t, "YES PLEASE", normalise("Yes please 🙂"))
	ph, ok := importPhone("0412 345 678")
	assert.True(t, ok)
	assert.Equal(t, "61412345678", ph)
	_, ok = importPhone("123")
	assert.False(t, ok)
}
