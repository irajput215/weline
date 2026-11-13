package booking

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- built-in provider (connectors R1, R4) ----------

func TestBuiltIn_Catalog(t *testing.T) {
	f := newFx(t)
	p := f.svc.Provider(f.org)
	assert.Equal(t, provider.BuiltIn, p.Name())

	locs, err := p.ListLocations(ctx)
	require.NoError(t, err)
	require.Len(t, locs, 1)
	assert.Equal(t, provider.Location{ID: f.bondi.String(), Name: "Bondi", TimeZone: "Australia/Sydney"}, locs[0])

	prs, err := p.ListPractitioners(ctx, "")
	require.NoError(t, err)
	assert.Len(t, prs, 3)
	at, err := p.ListPractitioners(ctx, f.bondi.String())
	require.NoError(t, err)
	assert.Len(t, at, 3)
	none, err := p.ListPractitioners(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Empty(t, none)

	svcs, err := p.ListServices(ctx)
	require.NoError(t, err)
	byName := map[string]provider.Service{}
	for _, s := range svcs {
		byName[s.Name] = s
	}
	assert.Equal(t, 30, byName["Long consult"].Minutes)
	assert.Equal(t, f.consult.String(), byName["Consult"].ID)
}

func TestBuiltIn_FreeTimesMatchSlotSearch(t *testing.T) {
	f := newFx(t)
	p := f.svc.Provider(f.org)
	day := syd(2027, 3, 8, 0, 0)
	free, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: f.consult.String(), LocationID: f.bondi.String(),
		From: day, To: day, Limit: 100})
	require.NoError(t, err)
	want, err := f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: f.consult, LocationID: f.bondi, From: date(monday), To: date(monday)})
	require.NoError(t, err)
	require.Len(t, free.Times, len(want))
	for i := range want {
		assert.True(t, free.Times[i].Start.Equal(want[i].StartsAt))
		assert.Equal(t, want[i].PractitionerIDs[0].String(), free.Times[i].PractitionerID)
		assert.Equal(t, 15*time.Minute, free.Times[i].End.Sub(free.Times[i].Start))
	}
	assert.Equal(t, "Dr Ada", free.Times[0].PractitionerName)
	assert.Equal(t, 60, free.WindowDays)

	ben, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: f.consult.String(), LocationID: f.bondi.String(),
		PractitionerID: f.ben.String(), From: day, To: day, Limit: 100})
	require.NoError(t, err)
	assert.Len(t, ben.Times, 4, "Dr Ben works 09:00-10:00")

	limited, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: f.consult.String(), LocationID: f.bondi.String(), Patient: true, Limit: 3})
	require.NoError(t, err)
	assert.Len(t, limited.Times, 3, "looks ahead from today until the limit")

	_, err = p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "not-an-id", LocationID: f.bondi.String()})
	assert.ErrorIs(t, err, provider.ErrNotFound)
}

func TestBuiltIn_CustomersAndAppointments(t *testing.T) {
	f := newFx(t)
	p := f.svc.Provider(f.org)
	testutil.CreateTestWhatsAppAccount(t, f.db, f.org)

	c, created, err := p.FindOrCreateCustomer(ctx, provider.CustomerInput{ContactID: f.patient})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, f.patient.String(), c.ID, "the customer is the Weline contact")
	n, created, err := p.FindOrCreateCustomer(ctx, provider.CustomerInput{Name: "New Person", Mobile: "0400 111 222"})
	require.NoError(t, err)
	assert.True(t, created)
	again, created, err := p.FindOrCreateCustomer(ctx, provider.CustomerInput{Name: "New Person", Mobile: "+61400111222"})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, n.ID, again.ID)
	_, _, err = p.FindOrCreateCustomer(ctx, provider.CustomerInput{ContactID: uuid.New()})
	assert.True(t, errors.Is(err, kit.ErrNotFound))
	got1, err := p.GetCustomer(ctx, n.ID)
	require.NoError(t, err)
	assert.Equal(t, "61400111222", got1.Mobile)
	_, err = p.GetCustomer(ctx, uuid.NewString())
	assert.True(t, errors.Is(err, kit.ErrNotFound))

	start := syd(2027, 3, 8, 9, 0)
	in := provider.BookInput{ServiceID: f.consult.String(), LocationID: f.bondi.String(), PractitionerID: f.ada.String(),
		Start: start, CustomerID: c.ID, ContactID: f.patient}
	a, err := p.Book(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, provider.StatusBooked, a.Status)
	assert.Equal(t, "Dr Ada", a.PractitionerName)
	assert.Equal(t, "Bondi", a.LocationName)
	assert.NotEmpty(t, a.Reference)
	assert.True(t, a.Start.Equal(start))
	row, err := f.svc.Get(ctx, f.org, uuid.MustParse(a.ID))
	require.NoError(t, err)
	assert.Equal(t, SourceWhatsApp, row.Source)

	in.CustomerID = n.ID
	_, err = p.Book(ctx, in)
	assert.ErrorIs(t, err, provider.ErrSlotTaken, "Dr Ada is taken at 09:00")

	moved, err := p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: start.Add(30 * time.Minute)})
	require.NoError(t, err)
	assert.True(t, moved.Start.Equal(start.Add(30*time.Minute)))
	got, err := p.GetAppointment(ctx, a.ID)
	require.NoError(t, err)
	assert.True(t, got.Start.Equal(moved.Start))

	since := time.Now().Add(-time.Hour)
	changed, err := p.ListAppointments(ctx, provider.AppointmentQuery{ChangedSince: &since})
	require.NoError(t, err)
	require.Len(t, changed, 1)
	from, to := syd(2027, 3, 8, 0, 0), syd(2027, 3, 9, 0, 0)
	inRange, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to, CustomerID: f.patient.String()})
	require.NoError(t, err)
	assert.Len(t, inRange, 1)
	_, err = p.ListAppointments(ctx, provider.AppointmentQuery{})
	assert.ErrorIs(t, err, provider.ErrInvalid)

	require.NoError(t, p.Cancel(ctx, a.ID, "patient asked on WhatsApp"))
	got, err = p.GetAppointment(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, provider.StatusCancelled, got.Status)
	_, err = p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: start})
	assert.True(t, errors.Is(err, kit.ErrConflict))
	assert.False(t, errors.Is(err, provider.ErrSlotTaken), "a cancelled appointment can't be moved: not 'just taken'")
	_, err = p.GetAppointment(ctx, uuid.NewString())
	assert.True(t, errors.Is(err, kit.ErrNotFound))

	other := testutil.CreateTestOrganization(t, f.db)
	_, err = f.svc.Provider(other.ID).GetAppointment(ctx, a.ID)
	assert.True(t, errors.Is(err, kit.ErrNotFound) || errors.Is(err, kit.ErrConflict), "another organization's provider can't see it")
}

// ---------- the chatbot through another provider (connectors R10) ----------

// stubProvider is a diary in memory with Cliniko-style ids.
type stubProvider struct {
	booked []provider.BookInput
	taken  bool
}

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) ListLocations(context.Context) ([]provider.Location, error) {
	return []provider.Location{{ID: "11", Name: "Harbour St", TimeZone: "Australia/Perth"}}, nil
}
func (s *stubProvider) ListPractitioners(context.Context, string) ([]provider.Practitioner, error) {
	return nil, nil
}
func (s *stubProvider) ListServices(context.Context) ([]provider.Service, error) {
	return []provider.Service{{ID: "21", Name: "Initial physio", Minutes: 45}, {ID: "22", Name: "Follow-up", Minutes: 30}}, nil
}
func (s *stubProvider) FindFreeTimes(_ context.Context, q provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	if q.ServiceID != "21" || q.LocationID != "11" || !q.Patient {
		return nil, provider.ErrInvalid
	}
	at := time.Date(2027, 3, 8, 1, 0, 0, 0, time.UTC) // 09:00 in Perth
	return &provider.FreeTimes{WindowDays: 14, Times: []provider.FreeTime{
		{Start: at, PractitionerID: "31", PractitionerName: "Sam Lee", LocationID: "11"},
		{Start: at.Add(time.Hour), PractitionerID: "32", PractitionerName: "Jo Wu", LocationID: "11"},
	}}, nil
}
func (s *stubProvider) FindOrCreateCustomer(_ context.Context, in provider.CustomerInput) (*provider.Customer, bool, error) {
	return &provider.Customer{ID: "41", Name: in.Name}, false, nil
}
func (s *stubProvider) GetCustomer(context.Context, string) (*provider.Customer, error) {
	return nil, provider.ErrNotFound
}
func (s *stubProvider) Book(_ context.Context, in provider.BookInput) (*provider.Appointment, error) {
	if s.taken {
		return nil, provider.ErrSlotTaken
	}
	s.booked = append(s.booked, in)
	return &provider.Appointment{ID: "51", Reference: "51", Status: provider.StatusBooked, Start: in.Start, ServiceName: "Initial physio",
		PractitionerName: "Sam Lee", LocationName: "Harbour St", TimeZone: "Australia/Perth", CustomerID: in.CustomerID}, nil
}
func (s *stubProvider) Reschedule(context.Context, string, provider.RescheduleInput) (*provider.Appointment, error) {
	return nil, provider.ErrNotSupported
}
func (s *stubProvider) Cancel(context.Context, string, string) error { return provider.ErrNotSupported }
func (s *stubProvider) GetAppointment(context.Context, string) (*provider.Appointment, error) {
	return nil, provider.ErrNotFound
}
func (s *stubProvider) ListAppointments(context.Context, provider.AppointmentQuery) ([]provider.Appointment, error) {
	return nil, provider.ErrNotSupported
}

func TestChatbot_ThroughAnotherProvider(t *testing.T) {
	f := newFx(t)
	stub := &stubProvider{}
	active := provider.Provider(stub)
	f.svc.UseProviders(provider.ResolverFunc(func(_ context.Context, orgID uuid.UUID) (provider.Provider, error) {
		require.Equal(t, f.org, orgID)
		return active, nil
	}))
	contact := testutil.CreateTestContactWith(t, f.db, f.org, testutil.WithPhoneNumber("61412345678"))

	cat, err := f.svc.Catalog(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, 2, cat["service_count"])
	assert.Equal(t, "Initial physio", cat["service_1_label"])
	assert.Equal(t, 1, cat["location_count"])

	slots, err := f.svc.ChatbotSlots(ctx, f.org, ChatbotSlotQuery{OfferID: cat["offer_id"].(string), ServiceLabel: "Initial physio"})
	require.NoError(t, err)
	assert.Equal(t, 2, slots["slot_count"])
	assert.Equal(t, "Mon 8 Mar 09:00 Sam Lee", slots["slot_1_label"], "labels in the location's time zone (Perth)")

	_, confirmation, err := f.svc.ChatbotBook(ctx, f.org, slots["offer_id"].(string), "Mon 8 Mar 09:00 Sam Lee", "61412345678")
	require.NoError(t, err)
	assert.Equal(t, "Booked: Initial physio with Sam Lee, Mon 8 Mar 09:00 at Harbour St. Ref 51", confirmation)
	require.Len(t, stub.booked, 1)
	b := stub.booked[0]
	assert.Equal(t, "21", b.ServiceID)
	assert.Equal(t, "11", b.LocationID)
	assert.Equal(t, "31", b.PractitionerID)
	assert.Equal(t, "41", b.CustomerID)
	assert.Equal(t, contact.ID, b.ContactID)
	assert.Equal(t, provider.SourceWhatsApp, b.Source)

	stub.taken = true
	_, _, err = f.svc.ChatbotBook(ctx, f.org, slots["offer_id"].(string), "slot_2", "61412345678")
	assert.ErrorIs(t, err, provider.ErrSlotTaken)

	// The clinic switches back to built-in: the stub's offers are refused.
	active = f.svc.Provider(f.org)
	_, _, err = f.svc.ChatbotBook(ctx, f.org, slots["offer_id"].(string), "slot_1", "61412345678")
	assert.ErrorIs(t, err, errOfferExpired)
	_, err = f.svc.ChatbotSlots(ctx, f.org, ChatbotSlotQuery{OfferID: cat["offer_id"].(string), ServiceLabel: "Initial physio"})
	assert.ErrorIs(t, err, errOfferExpired)
}

func TestChatbot_ProviderUnavailable(t *testing.T) {
	f := newFx(t)
	f.svc.UseProviders(provider.ResolverFunc(func(context.Context, uuid.UUID) (provider.Provider, error) {
		return nil, provider.ErrNotConnected
	}))
	_, err := f.svc.Catalog(ctx, f.org)
	assert.ErrorIs(t, err, provider.ErrNotConnected)
	assert.Equal(t, 503, kit.StatusOf(err), "the flow's error branch")
}
