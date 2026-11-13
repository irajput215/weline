package reminders

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

var ctx = context.Background()

// diary is a connector provider in memory, with customers' mobiles.
type diary struct {
	mu        sync.Mutex
	appts     map[string]*provider.Appointment
	mobiles   map[string]string
	cancelled []string
	confirmed []string
}

func (d *diary) Name() string                                               { return "stub" }
func (d *diary) ListLocations(context.Context) ([]provider.Location, error) { return nil, nil }
func (d *diary) ListPractitioners(context.Context, string) ([]provider.Practitioner, error) {
	return nil, nil
}
func (d *diary) ListServices(context.Context) ([]provider.Service, error) { return nil, nil }
func (d *diary) FindFreeTimes(context.Context, provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	return nil, provider.ErrNotSupported
}
func (d *diary) FindOrCreateCustomer(context.Context, provider.CustomerInput) (*provider.Customer, bool, error) {
	return nil, false, provider.ErrNotSupported
}
func (d *diary) GetCustomer(_ context.Context, id string) (*provider.Customer, error) {
	m, ok := d.mobiles[id]
	if !ok {
		return nil, provider.ErrNotFound
	}
	return &provider.Customer{ID: id, Name: "Pat " + id, Mobile: m}, nil
}
func (d *diary) Book(context.Context, provider.BookInput) (*provider.Appointment, error) {
	return nil, provider.ErrNotSupported
}
func (d *diary) Reschedule(context.Context, string, provider.RescheduleInput) (*provider.Appointment, error) {
	return nil, provider.ErrNotSupported
}
func (d *diary) Cancel(_ context.Context, id string, _ string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cancelled = append(d.cancelled, id)
	if a, ok := d.appts[id]; ok {
		a.Status = provider.StatusCancelled
	}
	return nil
}
func (d *diary) Confirm(_ context.Context, id string) error {
	d.confirmed = append(d.confirmed, id)
	return nil
}
func (d *diary) GetAppointment(_ context.Context, id string) (*provider.Appointment, error) {
	return d.appts[id], nil
}
func (d *diary) ListAppointments(_ context.Context, q provider.AppointmentQuery) ([]provider.Appointment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []provider.Appointment
	for _, a := range d.appts {
		if q.From != nil && a.Start.Before(*q.From) || q.To != nil && !a.Start.Before(*q.To) {
			continue
		}
		out = append(out, *a)
	}
	return out, nil
}

type sentTemplate struct {
	contact uuid.UUID
	name    string
	params  map[string]string
}

type fakeSender struct {
	mu        sync.Mutex
	templates []sentTemplate
	texts     []string
}

func (f *fakeSender) SendText(_ context.Context, _, _ uuid.UUID, text string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texts = append(f.texts, text)
	return uuid.New(), nil
}
func (f *fakeSender) SendTemplate(_ context.Context, _, c uuid.UUID, name, _ string, p map[string]string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.templates = append(f.templates, sentTemplate{contact: c, name: name, params: p})
	return uuid.New(), nil
}

type fakeHandover struct{ notes []string }

func (f *fakeHandover) HandOver(_, _ uuid.UUID, _ *uuid.UUID, notes string) (bool, error) {
	f.notes = append(f.notes, notes)
	return true, nil
}

type fixture struct {
	db     *gorm.DB
	svc    *Service
	d      *diary
	sender *fakeSender
	hand   *fakeHandover
	org    uuid.UUID
	user   uuid.UUID
	now    time.Time
}

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	for _, m := range []kit.Module{audit.New(nil, nil), consent.New(nil, nil)} {
		require.NoError(t, kit.RunMigrations(db, m.Name(), m.Migrations()))
	}
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
	return db
}

// Monday 8 March 2027, 10:00 in Sydney (UTC+11).
var monday10 = time.Date(2027, 3, 7, 23, 0, 0, 0, time.UTC)

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	acc := testutil.CreateTestWhatsAppAccount(t, db, org.ID)
	for _, name := range []string{"appt_reminder", "recall_checkup"} {
		require.NoError(t, db.Create(&models.Template{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, WhatsAppAccount: acc.Name,
			Name: name, Language: "en", Category: "UTILITY", Status: "APPROVED", BodyContent: "Hi {{1}}"}).Error)
	}
	f := &fixture{db: db, org: org.ID, user: user.ID, d: &diary{appts: map[string]*provider.Appointment{}, mobiles: map[string]string{}},
		sender: &fakeSender{}, hand: &fakeHandover{}, now: monday10}
	f.svc = NewService(db, nil, provider.ResolverFunc(func(context.Context, uuid.UUID) (provider.Provider, error) { return f.d, nil }),
		f.sender, f.hand, nil)
	f.svc.now = func() time.Time { return f.now }
	return f
}

func (f *fixture) appt(id, customer string, start time.Time) *provider.Appointment {
	a := &provider.Appointment{ID: id, Status: provider.StatusBooked, Start: start, End: start.Add(30 * time.Minute), ServiceID: "s1",
		ServiceName: "Initial physio", PractitionerName: "Sam Lee", LocationName: "Harbour St", TimeZone: "Australia/Sydney", CustomerID: customer}
	f.d.appts[id] = a
	if _, ok := f.d.mobiles[customer]; !ok {
		f.d.mobiles[customer] = fmt.Sprintf("614000%05d", len(f.d.mobiles)+1)
	}
	return a
}

func (f *fixture) rule(t *testing.T, in Rule) Rule {
	t.Helper()
	r, err := f.svc.SaveRule(ctx, f.org, f.user, nil, in)
	require.NoError(t, err)
	return *r
}

func reminder24h() Rule {
	return Rule{Kind: KindReminder, Name: "Day before", OffsetMinutes: 1440, TemplateName: "appt_reminder", Language: "en",
		Variables: []string{"name", "service", "when", "location"}, Enabled: true}
}

func TestRules(t *testing.T) {
	f := newFixture(t)
	for _, bad := range []Rule{
		{Kind: KindReminder, Name: "x", OffsetMinutes: 10, TemplateName: "appt_reminder", Language: "en"},
		{Kind: KindRecall, Name: "x", RecallDays: 2, TemplateName: "recall_checkup", Language: "en"},
		{Kind: "nag", Name: "x", TemplateName: "appt_reminder", Language: "en"},
		{Kind: KindReminder, Name: "x", OffsetMinutes: 60, TemplateName: "not_approved", Language: "en"},
		{Kind: KindReminder, Name: "x", OffsetMinutes: 60, TemplateName: "appt_reminder", Language: "English"},
		{Kind: KindRecall, Name: "x", RecallDays: 180, TemplateName: "recall_checkup", Language: "en", Variables: []string{"when"}},
	} {
		_, err := f.svc.SaveRule(ctx, f.org, f.user, nil, bad)
		assert.Error(t, err, "%+v", bad)
	}
	r := f.rule(t, reminder24h())
	r.Name, r.OffsetMinutes = "Two days before", 2880
	got, err := f.svc.SaveRule(ctx, f.org, f.user, &r.ID, r)
	require.NoError(t, err)
	assert.Equal(t, 2880, got.OffsetMinutes)
	other := uuid.New()
	_, err = f.svc.SaveRule(ctx, other, f.user, &r.ID, r)
	assert.Error(t, err, "another organization can't change it")
	assert.Error(t, f.svc.DeleteRule(ctx, other, r.ID))
	require.NoError(t, f.svc.DeleteRule(ctx, f.org, r.ID))
}

func TestSweep_DueAlreadyCancelledMoved(t *testing.T) {
	f := newFixture(t)
	f.rule(t, reminder24h())
	tomorrow9 := monday10.Add(23 * time.Hour) // Tue 9:00: within 24 h → due
	wednesday := monday10.Add(48 * time.Hour) // not yet due
	f.appt("a1", "c1", tomorrow9)
	f.appt("a2", "c2", wednesday)
	cancelled := f.appt("a3", "c3", tomorrow9.Add(time.Hour))
	cancelled.Status = provider.StatusCancelled

	n, err := f.svc.SweepOrg(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only a1 is due; a2 later; a3 cancelled")
	require.Len(t, f.sender.templates, 1)
	assert.Equal(t, map[string]string{"1": "Pat", "2": "Initial physio", "3": "Tue 9 Mar at 9:00am", "4": "Harbour St"}, f.sender.templates[0].params)

	n, err = f.svc.SweepOrg(ctx, f.org)
	require.NoError(t, err)
	assert.Zero(t, n, "already sent: never twice")

	// a1 is moved to Wednesday afternoon: no reminder for the old time; one for the new time when due.
	f.d.appts["a1"].Start = wednesday.Add(5 * time.Hour)
	n, err = f.svc.SweepOrg(ctx, f.org)
	require.NoError(t, err)
	assert.Zero(t, n)
	f.now = monday10.Add(25 * time.Hour) // Tuesday 11:00: a2 (Wed 10:00) is now due
	n, err = f.svc.SweepOrg(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "a2 now due; moved a1 (Wed 15:00) not yet")
	f.now = monday10.Add(30 * time.Hour) // Tue 16:00: moved a1 due for its new time
	n, err = f.svc.SweepOrg(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Len(t, f.sender.templates, 3)

	// A contact was created for each customer from their mobile.
	var contacts int64
	require.NoError(t, f.db.Model(&models.Contact{}).Where("organization_id = ?", f.org).Count(&contacts).Error)
	assert.Equal(t, int64(2), contacts)
}

func TestSweep_ConsentAndNoMobile(t *testing.T) {
	f := newFixture(t)
	f.rule(t, reminder24h())
	f.appt("a1", "c1", monday10.Add(2*time.Hour))
	f.appt("a2", "nomobile", monday10.Add(3*time.Hour))
	f.d.mobiles["nomobile"] = ""
	// c1's contact exists and said STOP; the clinic stops service messages after STOP too.
	c := models.Contact{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: f.org, PhoneNumber: f.d.mobiles["c1"], MarketingOptOut: true, Metadata: models.JSONB{}}
	require.NoError(t, f.db.Create(&c).Error)
	cs := consent.NewService(f.db, nil, nil, nil)
	_, err := cs.SaveSettings(ctx, f.org, f.user, consent.SettingsPatch{ServiceAfterStop: ptr("stop")})
	require.NoError(t, err)

	_, err = f.svc.SweepOrg(ctx, f.org)
	require.NoError(t, err)
	assert.Empty(t, f.sender.templates)
	sends, err := f.svc.Sends(ctx, f.org, nil)
	require.NoError(t, err)
	reasons := map[string]string{}
	for _, s := range sends {
		reasons[s.Status+":"+s.Reason] = s.Status
	}
	assert.Contains(t, reasons, "skipped:opted_out")
	assert.Contains(t, reasons, "skipped:no_mobile")
}

func ptr[T any](v T) *T { return &v }

func event(org, contact uuid.UUID, text string) kit.Event {
	b, _ := json.Marshal(handlers.MessageEventData{MessageID: uuid.NewString(), ContactID: contact.String(), MessageType: "button", Content: text,
		Direction: models.DirectionIncoming})
	return kit.Event{Name: "message.incoming", OrgID: org, Timestamp: time.Now(), Data: b}
}

func TestReplies(t *testing.T) {
	f := newFixture(t)
	f.rule(t, reminder24h())
	f.appt("a1", "c1", monday10.Add(5*time.Hour))
	f.appt("a2", "c2", monday10.Add(6*time.Hour))
	f.appt("a3", "c3", monday10.Add(7*time.Hour))
	_, err := f.svc.SweepOrg(ctx, f.org)
	require.NoError(t, err)
	contactOf := func(customer string) uuid.UUID {
		var c models.Contact
		require.NoError(t, f.db.Where("organization_id = ? AND phone_number = ?", f.org, f.d.mobiles[customer]).First(&c).Error)
		return c.ID
	}

	d := kit.NewDispatcher(&Module{service: f.svc}, claimSpy{})
	spy = nil
	require.NoError(t, d.Dispatch(ctx, event(f.org, contactOf("c1"), "Confirm ✅")))
	assert.Equal(t, []string{"a1"}, f.d.confirmed)
	assert.Equal(t, []string{Name}, spy, "the reply is claimed so the AI agent leaves it")
	require.NoError(t, d.Dispatch(ctx, event(f.org, contactOf("c2"), "Cancel")))
	assert.Equal(t, []string{"a2"}, f.d.cancelled)
	require.NoError(t, d.Dispatch(ctx, event(f.org, contactOf("c3"), "Reschedule")))
	assert.Contains(t, f.hand.notes, "Reminder reply: wants to reschedule")
	assert.Len(t, f.sender.texts, 3)

	spy = nil
	require.NoError(t, d.Dispatch(ctx, event(f.org, contactOf("c1"), "Confirm")))
	assert.Equal(t, []string{""}, spy, "already answered: not a reminder reply any more")
	require.NoError(t, d.Dispatch(ctx, event(f.org, contactOf("c1"), "what time is it?")))
	assert.Len(t, f.d.confirmed, 1)

	sends, err := f.svc.Sends(ctx, f.org, nil)
	require.NoError(t, err)
	replies := map[string]bool{}
	for _, s := range sends {
		replies[s.Reply] = true
	}
	assert.True(t, replies["confirmed"] && replies["cancelled"] && replies["reschedule"])
}

var spy []string

type claimSpy struct{}

func (claimSpy) Events() []string { return []string{"message.incoming"} }
func (claimSpy) HandleEvent(ctx context.Context, _ kit.Event) error {
	spy = append(spy, kit.ClaimedBy(ctx))
	return nil
}

func TestRecalls(t *testing.T) {
	f := newFixture(t)
	r := f.rule(t, Rule{Kind: KindRecall, Name: "6-month check", RecallDays: 180, TemplateName: "recall_checkup", Language: "en",
		Variables: []string{"name", "service"}, Enabled: true})
	due := f.appt("old", "due1", monday10.AddDate(0, -7, 0))
	due.Status = provider.StatusCompleted
	recent := f.appt("recent", "notdue", monday10.AddDate(0, -2, 0))
	recent.Status = provider.StatusCompleted
	booked := f.appt("past", "booked", monday10.AddDate(0, -8, 0))
	booked.Status = provider.StatusCompleted
	f.appt("ahead", "booked", monday10.AddDate(0, 0, 10))
	optout := f.appt("old2", "optout", monday10.AddDate(0, -9, 0))
	optout.Status = provider.StatusCompleted
	c := models.Contact{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: f.org, PhoneNumber: f.d.mobiles["optout"], MarketingOptOut: true, Metadata: models.JSONB{}}
	require.NoError(t, f.db.Create(&c).Error)

	list, err := f.svc.RecallDue(ctx, f.org, r.ID)
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, d := range list {
		ids[d.CustomerID] = true
	}
	assert.Equal(t, map[string]bool{"due1": true, "optout": true}, ids, "recent and already-booked customers aren't due")

	counts, err := f.svc.SendRecalls(ctx, f.org, r.ID, []string{"due1", "optout", "notdue"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"sent": 1, "skipped": 1, "not_due": 1, "already": 0}, counts, "opted out of marketing: skipped")
	require.Len(t, f.sender.templates, 1)
	assert.Equal(t, map[string]string{"1": "Pat", "2": "Initial physio"}, f.sender.templates[0].params)
	counts, err = f.svc.SendRecalls(ctx, f.org, r.ID, []string{"due1"})
	require.NoError(t, err)
	assert.Equal(t, 1, counts["already"], "once per customer and month")
}

// ---------- handlers ----------

type fakeAuth struct{ allowed map[string]bool }

func (a fakeAuth) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	id, _ := r.RequestCtx.UserValue("organization_id").(uuid.UUID)
	return id, nil
}
func (a fakeAuth) HasPermission(_ uuid.UUID, res, act string, _ ...uuid.UUID) bool {
	return a.allowed[res+":"+act]
}

func TestHandlers(t *testing.T) {
	f := newFixture(t)
	nobody := fakeAuth{allowed: map[string]bool{}}
	all := fakeAuth{allowed: map[string]bool{PermRules + ":read": true, PermRules + ":create": true, PermRules + ":update": true,
		PermRules + ":delete": true, PermRecalls + ":read": true, PermRecalls + ":create": true}}
	call := func(auth kit.Auth, method, path, id string, body any, org uuid.UUID) (int, map[string]any) {
		var h fastglue.FastRequestHandler
		for _, rt := range routes(auth, f.svc) {
			if rt.method == method && rt.path == path {
				h = rt.handler
			}
		}
		require.NotNil(t, h)
		rq := testutil.NewRequest(t)
		if body != nil {
			b, _ := json.Marshal(body)
			rq.RequestCtx.Request.SetBody(b)
		}
		if id != "" {
			rq.RequestCtx.SetUserValue("id", id)
		}
		testutil.SetAuthContext(rq, org, f.user)
		require.NoError(t, h(rq))
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
		return rq.RequestCtx.Response.StatusCode(), env.Data
	}
	for _, rt := range routes(nobody, f.svc) {
		code, _ := call(nobody, rt.method, rt.path, uuid.NewString(), map[string]any{}, f.org)
		assert.Equal(t, http.StatusForbidden, code, "%s %s", rt.method, rt.path)
	}
	code, data := call(all, "POST", base+"/rules", "", reminder24h(), f.org)
	require.Equal(t, 200, code, data)
	id := data["rule"].(map[string]any)["id"].(string)
	code, data = call(all, "GET", base+"/rules", "", nil, f.org)
	require.Equal(t, 200, code)
	assert.Len(t, data["rules"], 1)
	other := testutil.CreateTestOrganization(t, f.db)
	code, data = call(all, "GET", base+"/rules", "", nil, other.ID)
	require.Equal(t, 200, code)
	assert.Empty(t, data["rules"], "another organization sees none")
	code, _ = call(all, "DELETE", base+"/rules/{id}", id, nil, other.ID)
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = call(all, "GET", base+"/recalls/{id}/due", id, nil, f.org)
	assert.Equal(t, http.StatusNotFound, code, "a reminder rule isn't a recall")
	code, _ = call(all, "GET", base+"/sends", "", nil, f.org)
	assert.Equal(t, 200, code)
}
