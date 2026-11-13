package ads

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
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var ctx = context.Background()

type fixture struct {
	db      *gorm.DB
	svc     *Service
	org     uuid.UUID
	user    uuid.UUID
	account string
	now     time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	acc := testutil.CreateTestWhatsAppAccount(t, db, org.ID)
	f := &fixture{db: db, svc: NewService(db, nil), org: org.ID, user: user.ID, account: acc.Name,
		now: time.Now().UTC().Truncate(time.Second)}
	f.svc.now = func() time.Time { return f.now }
	return f
}

var phoneSeq int64 = time.Now().UnixNano() % 1_000_000

func (f *fixture) contact(t *testing.T, org uuid.UUID) *models.Contact {
	t.Helper()
	phoneSeq++
	c := &models.Contact{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org,
		PhoneNumber: fmt.Sprintf("61498%06d", phoneSeq), ProfileName: "Pat", Tags: models.JSONBArray{"vip"}, Metadata: models.JSONB{}}
	require.NoError(t, f.db.Create(c).Error)
	return c
}

func (f *fixture) reload(t *testing.T, id uuid.UUID) models.Contact {
	t.Helper()
	var c models.Contact
	require.NoError(t, f.db.Where("id = ?", id).First(&c).Error)
	return c
}

// message stores an incoming message the way upstream does before it raises
// message.incoming, and returns its id.
func (f *fixture) message(t *testing.T, org uuid.UUID, c *models.Contact, at time.Time, text string) uuid.UUID {
	t.Helper()
	m := &models.Message{BaseModel: models.BaseModel{ID: uuid.New(), CreatedAt: at}, OrganizationID: org, WhatsAppAccount: f.account,
		ContactID: c.ID, Direction: models.DirectionIncoming, MessageType: "text", Content: text}
	require.NoError(t, f.db.Create(m).Error)
	return m.ID
}

// arrive stores a message with text at time at and records it.
func (f *fixture) arrive(t *testing.T, c *models.Contact, at time.Time, text string) *Recorded {
	t.Helper()
	id := f.message(t, f.org, c, at, text)
	r, err := f.svc.Record(ctx, f.org, Incoming{ContactID: c.ID, MessageID: &id, Text: text})
	require.NoError(t, err)
	return r
}

func input(code string) Input {
	return Input{Name: "Flu shot ad", Code: code, Channel: "meta_ad", Campaign: "Flu season 2026", WaNumber: "+61 400 000 000",
		PrefillText: "Hi! I'd like to know more [" + code + "]"}
}

func (f *fixture) entry(t *testing.T, code string) *EntryPoint {
	t.Helper()
	ep, err := f.svc.Create(ctx, f.org, f.user, input(code))
	require.NoError(t, err)
	return ep
}

func (f *fixture) arrivals(t *testing.T, contact uuid.UUID) []arrivalRow {
	t.Helper()
	var rows []arrivalRow
	require.NoError(t, f.db.Where("organization_id = ? AND contact_id = ?", f.org, contact).Order("arrived_at").Find(&rows).Error)
	return rows
}

func hasTag(c models.Contact, tag string) int {
	n := 0
	for _, t := range c.Tags {
		if s, _ := t.(string); strings.EqualFold(s, tag) {
			n++
		}
	}
	return n
}

func isKind(err, kind error) bool { return errors.Is(err, kind) }

// ---------- entry points ----------

func TestCreate_Rules(t *testing.T) {
	f := newFixture(t)
	ep := f.entry(t, "flu24")
	assert.Equal(t, "FLU24", ep.Code)
	assert.Equal(t, "61400000000", ep.WaNumber)
	assert.Equal(t, "from-flu24", ep.Tag, "default tag")
	assert.True(t, ep.TagCampaign)
	assert.Equal(t, "campaign-flu-season-2026", ep.CampaignTag)
	assert.Equal(t, "https://wa.me/61400000000?text=Hi%21%20I%27d%20like%20to%20know%20more%20%5Bflu24%5D", ep.Link,
		"spaces as %20, brackets and quotes encoded")

	for _, code := range []string{"STOP", "12345", "A1", "FL-24", "ABCDEFGHIJ123", ""} {
		_, err := f.svc.Create(ctx, f.org, f.user, input(code))
		assert.True(t, isKind(err, kit.ErrInvalid), "code %q", code)
	}
	_, err := f.svc.Create(ctx, f.org, f.user, input("FLU24"))
	assert.True(t, isKind(err, kit.ErrConflict), "code used")
	_, err = f.svc.Archive(ctx, f.org, f.user, ep.ID)
	require.NoError(t, err)
	_, err = f.svc.Create(ctx, f.org, f.user, input("FLU24"))
	assert.True(t, isKind(err, kit.ErrConflict), "archived codes aren't reused")

	other := testutil.CreateTestOrganization(t, f.db)
	_, err = f.svc.Create(ctx, other.ID, f.user, input("FLU24"))
	assert.NoError(t, err, "the same code in another organization")

	bad := []func(*Input){
		func(in *Input) { in.Name = " " },
		func(in *Input) { in.Channel = "tv" },
		func(in *Input) { in.Campaign = strings.Repeat("x", 61) },
		func(in *Input) { in.WaNumber = "0400 000 000" },
		func(in *Input) { in.WaNumber = "61 400" },
		func(in *Input) { in.WaNumber = "61400000000x" },
		func(in *Input) { in.PrefillText = "Hi there" },
		func(in *Input) { in.PrefillText = "Hi XSKI26" },
		func(in *Input) { in.PrefillText = strings.Repeat("x", 501) + " SKI26" },
		func(in *Input) { in.Tag = "from ski" },
	}
	for i, mod := range bad {
		in := input("SKI26")
		mod(&in)
		_, err := f.svc.Create(ctx, f.org, f.user, in)
		assert.True(t, isKind(err, kit.ErrInvalid), "case %d", i)
	}
	off := false
	in := input("SKI26")
	in.Tag, in.TagCampaign = "skin_check", &off
	ep2, err := f.svc.Create(ctx, f.org, f.user, in)
	require.NoError(t, err)
	assert.Equal(t, "skin_check", ep2.Tag)
	assert.Empty(t, ep2.CampaignTag)
}

func TestUpdateArchiveDelete(t *testing.T) {
	f := newFixture(t)
	ep := f.entry(t, "FLU24")
	in := input("ZZZ99") // the code in the body is ignored
	in.Name, in.Channel, in.PrefillText = "Flu flyer", "print", "Flu shots FLU24"
	got, err := f.svc.Update(ctx, f.org, f.user, ep.ID, in)
	require.NoError(t, err)
	assert.Equal(t, "FLU24", got.Code)
	assert.Equal(t, "Flu flyer", got.Name)
	assert.Equal(t, "print", got.Channel)

	in.PrefillText = "no code here"
	_, err = f.svc.Update(ctx, f.org, f.user, ep.ID, in)
	assert.True(t, isKind(err, kit.ErrInvalid), "message must keep the code")

	c := f.contact(t, f.org)
	require.True(t, f.arrive(t, c, f.now, "FLU24").Arrived)
	assert.True(t, isKind(f.svc.Delete(ctx, f.org, ep.ID), kit.ErrConflict), "has arrivals")

	_, err = f.svc.Archive(ctx, f.org, f.user, ep.ID)
	require.NoError(t, err)
	a2, err := f.svc.Archive(ctx, f.org, f.user, ep.ID)
	require.NoError(t, err, "archiving twice")
	require.NotNil(t, a2.ArchivedAt)
	in.PrefillText = "Flu FLU24"
	_, err = f.svc.Update(ctx, f.org, f.user, ep.ID, in)
	assert.True(t, isKind(err, kit.ErrConflict), "archived can't be edited")
	f.now = f.now.Add(48 * time.Hour)
	assert.False(t, f.arrive(t, c, f.now, "FLU24").Arrived, "archived codes stop matching")

	empty := f.entry(t, "SKI26")
	require.NoError(t, f.svc.Delete(ctx, f.org, empty.ID))
	f.entry(t, "SKI26") // the code is free again
	assert.True(t, isKind(f.svc.Delete(ctx, f.org, uuid.New()), kit.ErrNotFound))
}

// ---------- matching ----------

func TestRecord_Matching(t *testing.T) {
	f := newFixture(t)
	flu := f.entry(t, "FLU24")
	ski := f.entry(t, "SKI26")
	cases := []struct {
		text string
		want *uuid.UUID
	}{
		{"Hi! I'd like to know more [FLU24]", &flu.ID},
		{"flu24", &flu.ID},
		{"#Flu24 please", &flu.ID},
		{"SKI26 and FLU24", &ski.ID},
		{"FLU245", nil},
		{"XFLU24", nil},
		{"flu 24", nil},
		{strings.Repeat("a", 995) + " FLU24", nil},
	}
	for _, tc := range cases {
		c := f.contact(t, f.org)
		r := f.arrive(t, c, f.now, tc.text)
		if tc.want == nil {
			assert.False(t, r.Arrived, tc.text)
			continue
		}
		assert.True(t, r.Arrived, tc.text)
		assert.Equal(t, *tc.want, r.EntrypointID, tc.text)
		rows := f.arrivals(t, c.ID)
		require.Len(t, rows, 1)
		wantCode := map[uuid.UUID]string{flu.ID: "FLU24", ski.ID: "SKI26"}[*tc.want]
		assert.Equal(t, wantCode, rows[0].Code, "the matched code is stored, not the text")
	}
}

func TestRecord_Repeats(t *testing.T) {
	f := newFixture(t)
	flu := f.entry(t, "FLU24")
	f.entry(t, "SKI26")
	c := f.contact(t, f.org)

	id := f.message(t, f.org, c, f.now, "FLU24")
	for i := 0; i < 2; i++ {
		_, err := f.svc.Record(ctx, f.org, Incoming{ContactID: c.ID, MessageID: &id, Text: "FLU24"})
		require.NoError(t, err)
	}
	assert.Len(t, f.arrivals(t, c.ID), 1, "the same message twice")
	assert.False(t, f.arrive(t, c, f.now.Add(23*time.Hour), "FLU24 again").Arrived, "within 24 hours")
	assert.True(t, f.arrive(t, c, f.now.Add(23*time.Hour), "SKI26").Arrived, "another entry point the same day")
	r := f.arrive(t, c, f.now.Add(25*time.Hour), "FLU24")
	assert.True(t, r.Arrived, "after 24 hours")
	assert.Equal(t, flu.ID, r.EntrypointID)
	assert.Len(t, f.arrivals(t, c.ID), 3)
}

func TestRecord_ConcurrentSameContact(t *testing.T) {
	f := newFixture(t)
	f.entry(t, "FLU24")
	c := f.contact(t, f.org)
	ids := []uuid.UUID{f.message(t, f.org, c, f.now, "FLU24"), f.message(t, f.org, c, f.now, "FLU24 hello")}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			_, err := f.svc.Record(ctx, f.org, Incoming{ContactID: c.ID, MessageID: &id, Text: "FLU24"})
			assert.NoError(t, err)
		}(id)
	}
	wg.Wait()
	assert.Len(t, f.arrivals(t, c.ID), 1, "one arrival per day even when two messages race")
}

func TestRecord_IsNew(t *testing.T) {
	f := newFixture(t)
	f.entry(t, "FLU24")
	first := f.contact(t, f.org)
	assert.True(t, f.arrive(t, first, f.now, "FLU24").IsNew, "first message")

	soon := f.contact(t, f.org)
	f.message(t, f.org, soon, f.now.Add(-5*time.Minute), "hello?")
	assert.True(t, f.arrive(t, soon, f.now, "FLU24").IsNew, "first message 5 minutes earlier")

	old := f.contact(t, f.org)
	f.message(t, f.org, old, f.now.Add(-7*24*time.Hour), "hi")
	r := f.arrive(t, old, f.now, "FLU24")
	assert.True(t, r.Arrived)
	assert.False(t, r.IsNew, "wrote last week")
}

func TestRecord_TagsAndNoConsent(t *testing.T) {
	f := newFixture(t)
	f.entry(t, "FLU24")
	off := false
	in := input("SKI26")
	in.TagCampaign = &off
	_, err := f.svc.Create(ctx, f.org, f.user, in)
	require.NoError(t, err)

	c := f.contact(t, f.org)
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("tags", models.JSONBArray{"vip", "FROM-FLU24"}).Error)
	f.arrive(t, c, f.now, "FLU24")
	got := f.reload(t, c.ID)
	assert.Equal(t, 1, hasTag(got, "from-flu24"), "not added twice (case-insensitive)")
	assert.Equal(t, 1, hasTag(got, "campaign-flu-season-2026"))
	assert.Equal(t, 1, hasTag(got, "vip"), "other tags kept")

	c2 := f.contact(t, f.org)
	f.arrive(t, c2, f.now, "SKI26")
	got = f.reload(t, c2.ID)
	assert.Equal(t, 1, hasTag(got, "from-ski26"))
	assert.Equal(t, 0, hasTag(got, "campaign-flu-season-2026"), "campaign tag off")

	var n int64
	require.NoError(t, f.db.Table("wl.consent_events").Where("organization_id = ?", f.org).Count(&n).Error)
	assert.Zero(t, n, "an arrival is not consent (R8)")
	assert.False(t, got.MarketingOptOut)
}

func TestRecord_OtherOrganization(t *testing.T) {
	f := newFixture(t)
	f.entry(t, "FLU24")
	other := testutil.CreateTestOrganization(t, f.db)
	theirs := f.contact(t, other.ID)
	r, err := f.svc.Record(ctx, f.org, Incoming{ContactID: theirs.ID, Text: "FLU24"})
	require.NoError(t, err)
	assert.False(t, r.Arrived, "another org's contact")
	r, err = f.svc.Record(ctx, other.ID, Incoming{ContactID: theirs.ID, Text: "FLU24"})
	require.NoError(t, err)
	assert.False(t, r.Arrived, "another org's code")
}

func event(org uuid.UUID, d handlers.MessageEventData) kit.Event {
	b, _ := json.Marshal(d)
	return kit.Event{Name: "message.incoming", OrgID: org, Timestamp: time.Now(), Data: b}
}

func TestHandleIncoming(t *testing.T) {
	f := newFixture(t)
	f.entry(t, "FLU24")
	c := f.contact(t, f.org)
	id := f.message(t, f.org, c, f.now, "FLU24")
	base := handlers.MessageEventData{MessageID: id.String(), ContactID: c.ID.String(), MessageType: "text", Content: "Hi FLU24",
		Direction: models.DirectionIncoming}

	out := base
	out.Direction = models.DirectionOutgoing
	require.NoError(t, f.svc.HandleIncoming(ctx, event(f.org, out)))
	img := base
	img.MessageType = "image"
	require.NoError(t, f.svc.HandleIncoming(ctx, event(f.org, img)))
	assert.Empty(t, f.arrivals(t, c.ID), "outgoing and non-text ignored")

	require.NoError(t, f.svc.HandleIncoming(ctx, event(f.org, base)))
	rows := f.arrivals(t, c.ID)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].MessageID)
	assert.Equal(t, id, *rows[0].MessageID)
	assert.True(t, rows[0].ArrivedAt.Equal(f.now), "arrival time is the message's")
}

// ---------- reports ----------

// practiceRows makes the practice rows an appointment needs.
func (f *fixture) appointment(t *testing.T, c *models.Contact, createdAt time.Time, source string) {
	t.Helper()
	loc, pr, svc := uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, f.db.Exec(`INSERT INTO wl.practice_locations (id, organization_id, name) VALUES (?, ?, ?)`, loc, f.org, "Loc "+loc.String()[:8]).Error)
	require.NoError(t, f.db.Exec(`INSERT INTO wl.practice_practitioners (id, organization_id, display_name, role) VALUES (?, ?, 'Dr A', 'GP')`, pr, f.org).Error)
	require.NoError(t, f.db.Exec(`INSERT INTO wl.practice_services (id, organization_id, name, duration_minutes, allowed_roles) VALUES (?, ?, ?, 15, '["GP"]')`,
		svc, f.org, "Svc "+svc.String()[:8]).Error)
	start := createdAt.Add(72 * time.Hour)
	require.NoError(t, f.db.Exec(`INSERT INTO wl.booking_appointments (organization_id, reference, contact_id, practitioner_id, service_id,
		location_id, starts_at, ends_at, source, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.org, strings.ToUpper(uuid.NewString()[:8]), c.ID, pr, svc, loc, start, start.Add(15*time.Minute), source, createdAt).Error)
}

func (f *fixture) optIn(t *testing.T, c *models.Contact, givenAt time.Time) {
	t.Helper()
	require.NoError(t, f.db.Exec(`INSERT INTO wl.consent_events (organization_id, contact_id, action, source, given_at)
		VALUES (?, ?, 'opt_in', 'staff', ?)`, f.org, c.ID, givenAt).Error)
}

func TestStats(t *testing.T) {
	f := newFixture(t)
	flu := f.entry(t, "FLU24")
	ski := f.entry(t, "SKI26")
	in := input("NOC11")
	in.Campaign = ""
	none, err := f.svc.Create(ctx, f.org, f.user, in)
	require.NoError(t, err)
	t0 := f.now.Add(-10 * 24 * time.Hour)

	optedIn := f.contact(t, f.org) // arrives, opts in 3 days later
	f.arrive(t, optedIn, t0, "FLU24")
	f.optIn(t, optedIn, t0.Add(3*24*time.Hour))

	late := f.contact(t, f.org) // opts in 31 days after: not credited
	f.arrive(t, late, t0.Add(-40*24*time.Hour), "FLU24")
	f.optIn(t, late, t0.Add(-9*24*time.Hour))

	switched := f.contact(t, f.org) // FLU24, then SKI26, then books: credited to SKI26 only
	f.arrive(t, switched, t0, "FLU24")
	f.arrive(t, switched, t0.Add(2*24*time.Hour), "SKI26")
	f.appointment(t, switched, t0.Add(3*24*time.Hour), "whatsapp")

	cliniko := f.contact(t, f.org) // a Cliniko booking isn't counted
	f.message(t, f.org, cliniko, t0.Add(-30*24*time.Hour), "hi")
	f.arrive(t, cliniko, t0, "NOC11")
	f.appointment(t, cliniko, t0.Add(24*time.Hour), "cliniko")

	list, err := f.svc.List(ctx, f.org, Period{}, false)
	require.NoError(t, err)
	got := map[uuid.UUID]Stats{}
	for _, ep := range list.EntryPoints {
		got[ep.ID] = ep.Stats
	}
	assert.Equal(t, Stats{Conversations: 2, NewContacts: 2, OptedIn: 1, Booked: 0}, got[flu.ID], "late arrival is outside the 30 days")
	assert.Equal(t, Stats{Conversations: 1, NewContacts: 0, OptedIn: 0, Booked: 1}, got[ski.ID], "switched contact had written before")
	assert.Equal(t, Stats{Conversations: 1, NewContacts: 0, OptedIn: 0, Booked: 0}, got[none.ID])
	require.Len(t, list.Campaigns, 1, "entry points without a campaign aren't totalled")
	assert.Equal(t, "Flu season 2026", list.Campaigns[0].Campaign)
	assert.Equal(t, Stats{Conversations: 3, NewContacts: 2, OptedIn: 1, Booked: 1}, list.Campaigns[0].Stats)

	week, err := f.svc.List(ctx, f.org, Period{From: f.now.Add(-9 * 24 * time.Hour), To: f.now}, false)
	require.NoError(t, err)
	for _, ep := range week.EntryPoints {
		if ep.ID == ski.ID {
			assert.Equal(t, 1, ep.Stats.Conversations)
		} else {
			assert.Zero(t, ep.Stats.Conversations, ep.Code)
		}
	}

	_, err = f.svc.Archive(ctx, f.org, f.user, none.ID)
	require.NoError(t, err)
	list, err = f.svc.List(ctx, f.org, Period{}, false)
	require.NoError(t, err)
	assert.Len(t, list.EntryPoints, 2, "archived hidden")
	list, err = f.svc.List(ctx, f.org, Period{}, true)
	require.NoError(t, err)
	assert.Len(t, list.EntryPoints, 3)

	one, err := f.svc.Get(ctx, f.org, flu.ID, Period{})
	require.NoError(t, err)
	assert.Equal(t, 2, one.Stats.Conversations)
}

func TestArrivals(t *testing.T) {
	f := newFixture(t)
	flu := f.entry(t, "FLU24")
	var last *models.Contact
	for i := 0; i < pageSize+2; i++ {
		last = f.contact(t, f.org)
		f.arrive(t, last, f.now.Add(time.Duration(i)*time.Minute), "FLU24")
	}
	f.optIn(t, last, f.now.Add(2*time.Hour))
	page, err := f.svc.Arrivals(ctx, f.org, flu.ID, nil)
	require.NoError(t, err)
	require.Len(t, page, pageSize)
	assert.Equal(t, last.ID, page[0].Contact.ID, "newest first")
	assert.True(t, page[0].OptedIn)
	assert.True(t, page[0].IsNew)
	assert.Regexp(t, `^61•+\d{3}$`, page[0].Contact.PhoneMasked)
	before := page[len(page)-1].ArrivedAt
	rest, err := f.svc.Arrivals(ctx, f.org, flu.ID, &before)
	require.NoError(t, err)
	assert.Len(t, rest, 2)

	other := testutil.CreateTestOrganization(t, f.db)
	_, err = f.svc.Arrivals(ctx, other.ID, flu.ID, nil)
	assert.True(t, isKind(err, kit.ErrNotFound))
}
