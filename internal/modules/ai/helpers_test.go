package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var ctx = context.Background()

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	a := audit.New(nil, nil)
	require.NoError(t, kit.RunMigrations(db, a.Name(), a.Migrations()))
	require.NoError(t, kit.RunMigrations(db, Name, migrations))
	return db
}

// scriptedLLM answers the classifier with intents[text] and the answerer with
// answers[text]; it records how many calls it got.
type scriptedLLM struct {
	mu      sync.Mutex
	intents map[string]string // text -> classifier JSON
	answers map[string]string // text -> answerer JSON
	calls   int
	fail    bool
}

func (s *scriptedLLM) Complete(_ context.Context, req kit.LLMRequest) (*kit.LLMResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.fail {
		return nil, fmt.Errorf("down")
	}
	text := req.Messages[len(req.Messages)-1].Content
	if strings.Contains(req.System, "You route") {
		if j, ok := s.intents[text]; ok {
			return &kit.LLMResponse{Text: j, TokensIn: 10, TokensOut: 5}, nil
		}
		return &kit.LLMResponse{Text: `{"intent":"other","confidence":0.5}`}, nil
	}
	if j, ok := s.answers[text]; ok {
		return &kit.LLMResponse{Text: j, TokensIn: 20, TokensOut: 10}, nil
	}
	return &kit.LLMResponse{Text: `{"can_answer":false,"answer":"","sources":[]}`}, nil
}

// fakeHandover records hand-overs.
type fakeHandover struct {
	mu     sync.Mutex
	active map[uuid.UUID]bool
	notes  []string
}

func (f *fakeHandover) HasActiveTransfer(_, c uuid.UUID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active[c]
}

func (f *fakeHandover) HandOver(_, c uuid.UUID, _ *uuid.UUID, notes string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notes = append(f.notes, notes)
	f.active[c] = true
	return true, nil
}

// stubDiary is a connector provider in memory.
type stubDiary struct {
	mu        sync.Mutex
	services  []provider.Service
	locations []provider.Location
	appts     map[string]*provider.Appointment
	booked    []provider.BookInput
	next      int
}

func newDiary() *stubDiary {
	return &stubDiary{services: []provider.Service{{ID: "s1", Name: "Initial physio", Minutes: 45}, {ID: "s2", Name: "Follow-up", Minutes: 30}},
		locations: []provider.Location{{ID: "l1", Name: "Harbour St", TimeZone: "Australia/Sydney"}}, appts: map[string]*provider.Appointment{}}
}

var day = time.Date(2027, 3, 8, 22, 0, 0, 0, time.UTC) // Tue 9 Mar 9:00am Sydney

func (d *stubDiary) Name() string { return "stub" }
func (d *stubDiary) ListLocations(context.Context) ([]provider.Location, error) {
	return d.locations, nil
}
func (d *stubDiary) ListPractitioners(context.Context, string) ([]provider.Practitioner, error) {
	return nil, nil
}
func (d *stubDiary) ListServices(context.Context) ([]provider.Service, error) { return d.services, nil }
func (d *stubDiary) FindFreeTimes(_ context.Context, q provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	out := &provider.FreeTimes{WindowDays: 14}
	for i := 0; i < 7 && len(out.Times) < q.Limit; i++ {
		out.Times = append(out.Times, provider.FreeTime{Start: day.Add(time.Duration(i) * time.Hour), PractitionerID: "p1", PractitionerName: "Sam Lee", LocationID: q.LocationID})
	}
	return out, nil
}
func (d *stubDiary) FindOrCreateCustomer(_ context.Context, in provider.CustomerInput) (*provider.Customer, bool, error) {
	return &provider.Customer{ID: "cust-" + in.ContactID.String()[:8], Name: in.Name}, false, nil
}
func (d *stubDiary) GetCustomer(_ context.Context, id string) (*provider.Customer, error) {
	return &provider.Customer{ID: id, Name: "Pat Kim", Mobile: "61412345678"}, nil
}
func (d *stubDiary) Book(_ context.Context, in provider.BookInput) (*provider.Appointment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.next++
	d.booked = append(d.booked, in)
	a := &provider.Appointment{ID: fmt.Sprint(d.next), Reference: fmt.Sprintf("R%d", d.next), Status: provider.StatusBooked, Start: in.Start,
		ServiceID: in.ServiceID, ServiceName: "Initial physio", PractitionerName: "Sam Lee", LocationID: in.LocationID, LocationName: "Harbour St",
		TimeZone: "Australia/Sydney", CustomerID: in.CustomerID}
	d.appts[a.ID] = a
	return a, nil
}
func (d *stubDiary) Reschedule(_ context.Context, id string, in provider.RescheduleInput) (*provider.Appointment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.appts[id]
	if !ok {
		return nil, provider.ErrNotFound
	}
	a.Start = in.Start
	return a, nil
}
func (d *stubDiary) Cancel(_ context.Context, id string, _ string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.appts[id]
	if !ok {
		return provider.ErrNotFound
	}
	a.Status = provider.StatusCancelled
	return nil
}
func (d *stubDiary) GetAppointment(_ context.Context, id string) (*provider.Appointment, error) {
	return d.appts[id], nil
}
func (d *stubDiary) ListAppointments(_ context.Context, q provider.AppointmentQuery) ([]provider.Appointment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []provider.Appointment
	for _, a := range d.appts {
		if a.CustomerID == q.CustomerID {
			out = append(out, *a)
		}
	}
	return out, nil
}

type fakeSender struct {
	mu   sync.Mutex
	sent []string
}

func (f *fakeSender) SendText(_ context.Context, _, _ uuid.UUID, text string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, text)
	return uuid.New(), nil
}
func (f *fakeSender) SendTemplate(context.Context, uuid.UUID, uuid.UUID, string, string, map[string]string) (uuid.UUID, error) {
	return uuid.New(), nil
}

type fixture struct {
	db      *gorm.DB
	svc     *Service
	llm     *scriptedLLM
	hand    *fakeHandover
	diary   *stubDiary
	sender  *fakeSender
	org     uuid.UUID
	user    uuid.UUID
	contact uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	user := testutil.CreateTestUser(t, db, org.ID)
	f := &fixture{db: db, org: org.ID, user: user.ID, llm: &scriptedLLM{intents: map[string]string{}, answers: map[string]string{}},
		hand: &fakeHandover{active: map[uuid.UUID]bool{}}, diary: newDiary(), sender: &fakeSender{}}
	f.svc = NewService(db, nil, func(context.Context, uuid.UUID) (kit.LLM, error) { return f.llm, nil },
		provider.ResolverFunc(func(context.Context, uuid.UUID) (provider.Provider, error) { return f.diary, nil }), f.hand, f.sender, nil)
	f.svc.now = func() time.Time { return day.Add(-24 * time.Hour) }
	c := &models.Contact{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, PhoneNumber: "61412345678", ProfileName: "Pat Kim", Metadata: models.JSONB{}}
	require.NoError(t, db.Create(c).Error)
	f.contact = c.ID
	return f
}

func (f *fixture) enable(t *testing.T) {
	t.Helper()
	_, err := f.svc.SaveSettings(ctx, f.org, f.user, Settings{Enabled: true, EmergencyWords: DefaultEmergencyWords})
	require.NoError(t, err)
}

func (f *fixture) faq(t *testing.T, q, a string) uuid.UUID {
	t.Helper()
	it, err := f.svc.AddKnowledge(ctx, f.org, f.user, KnowledgeInput{Kind: "faq", Question: q, Answer: a})
	require.NoError(t, err)
	return it.SourceID
}

func event(org, contact uuid.UUID, text, typ string) kit.Event {
	b, _ := json.Marshal(handlers.MessageEventData{MessageID: uuid.NewString(), ContactID: contact.String(), MessageType: models.MessageType(typ),
		Content: text, Direction: models.DirectionIncoming})
	return kit.Event{Name: "message.incoming", OrgID: org, Timestamp: time.Now(), Data: b}
}
