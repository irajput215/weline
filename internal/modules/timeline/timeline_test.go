package timeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/ads"
	"github.com/shridarpatil/whatomate/internal/modules/ai"
	"github.com/shridarpatil/whatomate/internal/modules/audiences"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"github.com/shridarpatil/whatomate/internal/modules/reminders"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

var ctx = context.Background()

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	for _, m := range []kit.Module{audit.New(nil, nil), practice.New(nil, nil), booking.New(nil, nil), consent.New(nil, nil), audiences.New(nil, nil),
		ads.New(nil, nil), reminders.New(nil, nil), ai.New(nil, nil)} {
		require.NoError(t, kit.RunMigrations(db, m.Name(), m.Migrations()))
	}
	bk := booking.New(nil, nil) // booking depends on practice: leave the shared database as other packages expect
	t.Cleanup(func() { _ = kit.RollbackLast(db, bk.Name(), bk.Migrations()) })
	return db
}

// diary is a connected system with one upcoming appointment for the contact;
// it must never be asked to create a customer.
type diary struct {
	err       error
	customers []string
}

func (d *diary) Name() string                                               { return "stub" }
func (d *diary) ListLocations(context.Context) ([]provider.Location, error) { return nil, nil }
func (d *diary) ListPractitioners(context.Context, string) ([]provider.Practitioner, error) {
	return nil, nil
}
func (d *diary) ListServices(context.Context) ([]provider.Service, error) { return nil, nil }
func (d *diary) FindFreeTimes(context.Context, provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	return nil, nil
}
func (d *diary) FindOrCreateCustomer(context.Context, provider.CustomerInput) (*provider.Customer, bool, error) {
	panic("the timeline must not create customers")
}
func (d *diary) FindCustomer(_ context.Context, in provider.CustomerInput) (*provider.Customer, error) {
	return &provider.Customer{ID: "cust-" + in.Mobile}, nil
}
func (d *diary) GetCustomer(context.Context, string) (*provider.Customer, error) { return nil, nil }
func (d *diary) Book(context.Context, provider.BookInput) (*provider.Appointment, error) {
	return nil, nil
}
func (d *diary) Reschedule(context.Context, string, provider.RescheduleInput) (*provider.Appointment, error) {
	return nil, nil
}
func (d *diary) Cancel(context.Context, string, string) error { return nil }
func (d *diary) GetAppointment(context.Context, string) (*provider.Appointment, error) {
	return nil, nil
}
func (d *diary) ListAppointments(_ context.Context, q provider.AppointmentQuery) ([]provider.Appointment, error) {
	if d.err != nil {
		return nil, d.err
	}
	d.customers = append(d.customers, q.CustomerID)
	start := time.Now().Add(48 * time.Hour)
	if q.To != nil && !start.Before(*q.To) {
		return nil, nil // outside the range asked for
	}
	return []provider.Appointment{{ID: "a1", Status: provider.StatusBooked, Start: start, ServiceName: "Initial physio",
		PractitionerName: "Sam Lee", LocationName: "Harbour St", TimeZone: "Australia/Sydney"}}, nil
}

type fixture struct {
	db      *gorm.DB
	svc     *Service
	d       *diary
	org     uuid.UUID
	user    uuid.UUID
	contact *models.Contact
	acc     string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	f := &fixture{db: db, org: org.ID, user: testutil.CreateTestUser(t, db, org.ID).ID, d: &diary{}}
	f.acc = testutil.CreateTestWhatsAppAccount(t, db, org.ID).Name
	f.contact = testutil.CreateTestContactWith(t, db, org.ID, testutil.WithPhoneNumber("614"+fmt.Sprint(time.Now().UnixNano())[10:18]))
	f.svc = NewService(db, provider.ResolverFunc(func(context.Context, uuid.UUID) (provider.Provider, error) { return f.d, nil }))
	return f
}

func (f *fixture) message(t *testing.T, contact uuid.UUID, dir models.Direction, at time.Time) {
	t.Helper()
	m := models.Message{BaseModel: models.BaseModel{ID: uuid.New(), CreatedAt: at}, OrganizationID: f.org, WhatsAppAccount: f.acc, ContactID: contact,
		Direction: dir, MessageType: "text", Content: "secret message text", Status: "read"}
	require.NoError(t, f.db.Create(&m).Error)
}

func (f *fixture) seed(t *testing.T) {
	t.Helper()
	now := time.Now().UTC()
	c := f.contact.ID
	f.message(t, c, models.DirectionIncoming, now.Add(-10*time.Hour))
	f.message(t, c, models.DirectionOutgoing, now.Add(-9*time.Hour))
	tmpl := models.Template{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: f.org, WhatsAppAccount: f.acc, Name: "promo", Language: "en", BodyContent: "x"}
	require.NoError(t, f.db.Create(&tmpl).Error)
	camp := models.BulkMessageCampaign{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: f.org, WhatsAppAccount: f.acc, Name: "Winter offer", TemplateID: tmpl.ID, CreatedBy: f.user}
	require.NoError(t, f.db.Create(&camp).Error)
	sent := now.Add(-8 * time.Hour)
	require.NoError(t, f.db.Create(&models.BulkMessageRecipient{BaseModel: models.BaseModel{ID: uuid.New()}, CampaignID: camp.ID,
		PhoneNumber: f.contact.PhoneNumber, Status: "delivered", SentAt: &sent}).Error)
	var rule struct{ ID uuid.UUID }
	require.NoError(t, f.db.Raw(`INSERT INTO wl.reminder_rules (organization_id, kind, name, offset_minutes, template_name, language, enabled)
		VALUES (?, 'reminder', 'Day before', 1440, 'promo', 'en', true) RETURNING id`, f.org).Scan(&rule).Error)
	ruleID := rule.ID
	require.NoError(t, f.db.Exec(`INSERT INTO wl.reminder_sends (organization_id, rule_id, provider, target_id, start_at, contact_id, status, reply, created_at)
		VALUES (?, ?, 'stub', 'a0', ?, ?, 'sent', 'confirmed', ?)`, f.org, ruleID, now.Add(-6*time.Hour), c, now.Add(-7*time.Hour)).Error)
	require.NoError(t, f.db.Exec(`INSERT INTO wl.consent_events (organization_id, contact_id, action, source, given_at, evidence)
		VALUES (?, ?, 'opt_in', 'staff', ?, 'secret evidence note')`, f.org, c, now.Add(-5*time.Hour)).Error)
	var entry struct{ ID uuid.UUID }
	require.NoError(t, f.db.Raw(`INSERT INTO wl.ads_entrypoints (organization_id, name, code, channel, wa_number, prefill_text, tag)
		VALUES (?, 'Facebook winter', 'FB123', 'meta_ad', '61400000000', 'Hi FB123', 'fb') RETURNING id`, f.org).Scan(&entry).Error)
	ep := entry.ID
	require.NoError(t, f.db.Exec(`INSERT INTO wl.ads_arrivals (organization_id, entrypoint_id, contact_id, code, arrived_at) VALUES (?, ?, ?, 'FB123', ?)`,
		f.org, ep, c, now.Add(-4*time.Hour)).Error)
	require.NoError(t, f.db.Exec(`INSERT INTO wl.ai_runs (organization_id, contact_id, message_id, intent, outcome, created_at) VALUES (?, ?, ?, 'question', 'answered', ?)`,
		f.org, c, uuid.New(), now.Add(-3*time.Hour)).Error)
	require.NoError(t, f.db.Create(&models.AgentTransfer{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: f.org, ContactID: c,
		WhatsAppAccount: f.acc, PhoneNumber: f.contact.PhoneNumber, Status: "active", Source: "manual", Notes: "secret handover note",
		TransferredAt: now.Add(-2 * time.Hour)}).Error)
}

func TestTimeline_AllSourcesNoTextAndIsolated(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	// Another contact in the organization, and another organization.
	other := testutil.CreateTestContact(t, f.db, f.org)
	f.message(t, other.ID, models.DirectionIncoming, time.Now().Add(-time.Hour))

	tl, err := f.svc.Get(ctx, f.org, f.contact.ID, nil)
	require.NoError(t, err)
	kinds := []string{}
	for _, it := range tl.Items {
		kinds = append(kinds, it.Kind)
	}
	assert.Equal(t, []string{KindAppointment, KindHandover, KindAI, KindAdArrival, KindConsent, KindReminder, KindBroadcast, KindMessageOut, KindMessageIn}, kinds,
		"every source, newest first (the upcoming appointment on top)")
	assert.False(t, tl.AppointmentsUnavailable)
	assert.Equal(t, []string{"cust-" + f.contact.PhoneNumber}, f.d.customers, "found, never created, in the connected system")
	byKind := map[string]Item{}
	for _, it := range tl.Items {
		byKind[it.Kind] = it
	}
	assert.Equal(t, "Winter offer", byKind[KindBroadcast].Fields["campaign"])
	assert.Equal(t, "confirmed", byKind[KindReminder].Fields["reply"])
	assert.Equal(t, "Facebook winter", byKind[KindAdArrival].Fields["entry_point"])
	assert.Equal(t, "Initial physio", byKind[KindAppointment].Fields["service"])
	b, _ := json.Marshal(tl)
	assert.NotContains(t, string(b), "secret", "no message text, notes or evidence")
	assert.NotContains(t, string(b), f.contact.PhoneNumber, "the number is masked")
	assert.Len(t, tl.Items, 9, "the other contact's message isn't here")

	otherOrg := testutil.CreateTestOrganization(t, f.db).ID
	_, err = f.svc.Get(ctx, otherOrg, f.contact.ID, nil)
	assert.ErrorIs(t, err, kit.ErrNotFound, "another organization can't see this contact")

	f.d.err = provider.ErrUnavailable
	tl, err = f.svc.Get(ctx, f.org, f.contact.ID, nil)
	require.NoError(t, err)
	assert.True(t, tl.AppointmentsUnavailable, "the rest still shows when the connected system is down")
	assert.Len(t, tl.Items, 8)
}

func TestTimeline_Paging(t *testing.T) {
	f := newFixture(t)
	start := time.Now().UTC().Add(-200 * time.Hour)
	for i := 0; i < Page+20; i++ {
		f.message(t, f.contact.ID, models.DirectionIncoming, start.Add(time.Duration(i)*time.Hour))
	}
	first, err := f.svc.Get(ctx, f.org, f.contact.ID, nil)
	require.NoError(t, err)
	require.Len(t, first.Items, Page)
	require.NotNil(t, first.NextBefore)
	second, err := f.svc.Get(ctx, f.org, f.contact.ID, first.NextBefore)
	require.NoError(t, err)
	assert.Len(t, second.Items, 21, "the rest: 120 messages + the upcoming appointment = 121 items, 100 on the first page")
	assert.True(t, second.Items[0].At.Before(*first.NextBefore))
}

func TestSearch(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", f.contact.ID).Update("profile_name", "Pat Kim").Error)
	got, err := f.svc.Search(ctx, f.org, "pat")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, Mask(f.contact.PhoneNumber), got[0].PhoneMasked)
	assert.NotEqual(t, f.contact.PhoneNumber, got[0].PhoneMasked)
	got, err = f.svc.Search(ctx, f.org, f.contact.PhoneNumber[len(f.contact.PhoneNumber)-6:])
	require.NoError(t, err)
	assert.Len(t, got, 1, "by part of the number")
	got, err = f.svc.Search(ctx, f.org, "%_")
	require.NoError(t, err)
	assert.Empty(t, got, "wildcards matched literally")
	_, err = f.svc.Search(ctx, f.org, "p")
	assert.ErrorIs(t, err, kit.ErrInvalid)
	otherOrg := testutil.CreateTestOrganization(t, f.db).ID
	got, err = f.svc.Search(ctx, otherOrg, "pat")
	require.NoError(t, err)
	assert.Empty(t, got, "another organization finds nobody here")
	assert.Equal(t, "61••••••678", Mask("61412345678"))
}

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
	call := func(auth kit.Auth, path, id, query string) (int, map[string]any) {
		var fn fastglue.FastRequestHandler
		for _, rt := range routes(auth, f.svc) {
			if rt.path == path {
				fn = rt.handler
			}
		}
		require.NotNil(t, fn)
		rq := testutil.NewRequest(t)
		rq.RequestCtx.Request.SetRequestURI(path + "?" + query)
		if id != "" {
			rq.RequestCtx.SetUserValue("id", id)
		}
		testutil.SetAuthContext(rq, f.org, f.user)
		require.NoError(t, fn(rq))
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
		return rq.RequestCtx.Response.StatusCode(), env.Data
	}
	nobody := fakeAuth{allowed: map[string]bool{}}
	for _, rt := range routes(nobody, f.svc) {
		code, _ := call(nobody, rt.path, f.contact.ID.String(), "q=pat")
		assert.Equal(t, http.StatusForbidden, code, rt.path)
	}
	reader := fakeAuth{allowed: map[string]bool{PermContacts + ":read": true}}
	code, data := call(reader, base+"/contacts/{id}", f.contact.ID.String(), "")
	require.Equal(t, 200, code)
	assert.Equal(t, f.contact.ID.String(), data["contact"].(map[string]any)["id"])
	code, _ = call(reader, base+"/contacts/{id}", "nope", "")
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = call(reader, base+"/contacts/{id}", f.contact.ID.String(), "before=yesterday")
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = call(reader, base+"/contacts", "", "q=x")
	assert.Equal(t, http.StatusBadRequest, code)
}
