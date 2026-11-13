package cliniko

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

// Monday 16 Nov 2026, 08:00 in Sydney (AEDT, UTC+11).
var monday8am = time.Date(2026, 11, 15, 21, 0, 0, 0, time.UTC)

func newProvider(t *testing.T, f *fakeCliniko, raw string) (*Provider, *memLinks) {
	t.Helper()
	set, err := ParseSettings(json.RawMessage(raw))
	require.NoError(t, err)
	l := newLinks()
	p := New(f.client(t), l, set, nil)
	p.now = func() time.Time { return monday8am }
	return p, l
}

func TestParseKeyAndHeaders(t *testing.T) {
	for key, shard := range map[string]string{"abcDEF123-au3": "au3", "abcDEF123": "au1", "abc-uk1": "uk1", "abc-ca12": "ca12"} {
		got, err := ParseKey(key)
		require.NoError(t, err, key)
		assert.Equal(t, shard, got, key)
	}
	for _, bad := range []string{"", "has space", "abc-AU1", "abc-au", "abc:def", "abc-au1-x"} {
		_, err := ParseKey(bad)
		assert.True(t, errors.Is(err, kit.ErrInvalid), bad)
	}
	c, err := NewClient("abcDEF123-au3", "dev@example.test", nil, "")
	require.NoError(t, err)
	assert.Equal(t, "https://api.au3.cliniko.com/v1", c.base, "the shard picks the host")
	_, err = NewClient("abcDEF123-au3", "", nil, "")
	assert.True(t, errors.Is(err, kit.ErrNotReady), "no contact email for the User-Agent")

	f := newFake(t)
	msg, err := f.client(t).TestConnection(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Connected to Cliniko as Weline Integration", msg)
	r := f.requests[0]
	assert.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte(fakeKey+":")), r.Header.Get("Authorization"))
	assert.Equal(t, "application/json", r.Header.Get("Accept"))
	assert.Equal(t, "Weline (dev@example.test)", r.Header.Get("User-Agent"))
}

func TestErrorsAndRateLimits(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	slept := time.Duration(0)
	c.sleep = func(d time.Duration) { slept += d }

	f.status429, f.resetIn = 1, 3*time.Second
	_, err := c.TestConnection(ctx)
	require.NoError(t, err, "a near reset waits and retries once")
	assert.Greater(t, slept, time.Duration(0))

	f.status429, f.resetIn = 2, 2*time.Minute
	_, err = c.TestConnection(ctx)
	var rl *provider.RateLimited
	require.True(t, errors.As(err, &rl), "a far reset is reported with when to retry")
	assert.True(t, errors.Is(err, provider.ErrUnavailable))
	f.status429 = 0

	f.fail500 = true
	_, err = c.TestConnection(ctx)
	assert.True(t, errors.Is(err, provider.ErrUnavailable))
	assert.Equal(t, 503, kit.StatusOf(err))
	f.fail500 = false

	p, _ := newProvider(t, f, "")
	_, err = p.GetAppointment(ctx, "nope")
	assert.True(t, errors.Is(err, provider.ErrNotFound))

	unauth, err := NewClient("wrongkey-au3", "dev@example.test", f.srv.Client(), f.base())
	require.NoError(t, err)
	unauth.auth = "" // the fake answers 401 without Basic auth
	_, err = unauth.TestConnection(ctx)
	assert.ErrorIs(t, err, errKeyRefused)
	assert.NotContains(t, err.Error(), "wrongkey", "the key never appears in errors")

	err = c.do(ctx, "GET", "https://evil.example/v1/user", nil, nil, nil)
	assert.True(t, errors.Is(err, provider.ErrUnavailable), "links outside the account's API aren't followed")
}

func TestCatalog(t *testing.T) {
	f := newFake(t)
	p, _ := newProvider(t, f, "")
	locs, err := p.ListLocations(ctx)
	require.NoError(t, err)
	assert.Equal(t, []provider.Location{{ID: "11", Name: "Harbour St", TimeZone: "Australia/Sydney"},
		{ID: "13", Name: "Perth CBD", TimeZone: "Australia/Perth"}}, locs, "archived skipped; pages followed")

	prs, err := p.ListPractitioners(ctx, "")
	require.NoError(t, err)
	require.Len(t, prs, 2, "hidden from online bookings skipped")
	assert.Equal(t, provider.Practitioner{ID: "31", Name: "Sam Lee", LocationIDs: []string{"11", "13"}}, prs[0])
	assert.Equal(t, "Jo Wu", prs[1].Name)
	perth, err := p.ListPractitioners(ctx, "13")
	require.NoError(t, err)
	assert.Len(t, perth, 1)

	svcs, err := p.ListServices(ctx)
	require.NoError(t, err)
	assert.Equal(t, []provider.Service{{ID: "21", Name: "Initial physio", Minutes: 45}}, svcs, "group, paid, archived and hidden types skipped")

	all, _ := newProvider(t, f, `{"show_all": true}`)
	prs, err = all.ListPractitioners(ctx, "11")
	require.NoError(t, err)
	assert.Len(t, prs, 3)
	svcs, err = all.ListServices(ctx)
	require.NoError(t, err)
	assert.Len(t, svcs, 2)

	_, err = ParseSettings(json.RawMessage(`{"window_days": 90}`))
	assert.True(t, errors.Is(err, provider.ErrInvalid))
}

func TestFindFreeTimes(t *testing.T) {
	f := newFake(t)
	f.perPage = 100
	p, _ := newProvider(t, f, `{"window_days": 10}`)
	free, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "21", LocationID: "11", Patient: true, Limit: 5})
	require.NoError(t, err)
	assert.Equal(t, 10, free.WindowDays)
	require.Len(t, free.Times, 5)
	syd, _ := time.LoadLocation("Australia/Sydney")
	var got []string
	for _, ft := range free.Times {
		got = append(got, ft.Start.In(syd).Format("Mon 15:04")+" "+ft.PractitionerName)
	}
	assert.Equal(t, []string{"Mon 09:00 Sam Lee", "Mon 10:00 Sam Lee", "Mon 11:00 Jo Wu", "Tue 09:00 Sam Lee", "Tue 10:00 Sam Lee"}, got,
		"merged and sorted; 09:00 offered once with the first practitioner free")
	assert.Equal(t, 45*time.Minute, free.Times[0].End.Sub(free.Times[0].Start))
	calls := f.count("GET", "/available_times")
	assert.Equal(t, 2, calls, "one 7-day window per practitioner was enough for 5 times")

	_, err = p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "21", LocationID: "11", Patient: true, Limit: 5})
	require.NoError(t, err)
	assert.Equal(t, calls, f.count("GET", "/available_times"), "cached for a minute")

	jo, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "21", LocationID: "11", PractitionerID: "32", Limit: 100})
	require.NoError(t, err)
	assert.Len(t, jo.Times, 20, "10 days x 2 times; a second 7-day window was asked for")
	for _, ft := range jo.Times {
		assert.Equal(t, "32", ft.PractitionerID)
	}

	_, err = p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "22", LocationID: "11"})
	assert.True(t, errors.Is(err, provider.ErrNotFound), "group classes aren't bookable")
	_, err = p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "21", LocationID: "12"})
	assert.True(t, errors.Is(err, provider.ErrNotFound), "archived business")
}

func TestFindOrCreateCustomer(t *testing.T) {
	f := newFake(t)
	p, links := newProvider(t, f, "")
	contact := uuid.New()

	// Name search, checked against the mobile (no phone search in Cliniko).
	c, created, err := p.FindOrCreateCustomer(ctx, provider.CustomerInput{ContactID: contact, Name: "Ava Ng", Mobile: "61412345678"})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, "201", c.ID)
	assert.Equal(t, "201", links.remote[contact], "linked to the contact")
	assert.Equal(t, []string{"201"}, links.phones["61412345678"], "and indexed by phone")

	// Linked contact reused without a search.
	searches := f.count("GET", "/v1/patients")
	c, _, err = p.FindOrCreateCustomer(ctx, provider.CustomerInput{ContactID: contact, Name: "Someone Else", Mobile: "61499999999"})
	require.NoError(t, err)
	assert.Equal(t, "201", c.ID)
	assert.Equal(t, searches+1, f.count("GET", "/v1/patients"), "one GET of the linked patient, no search")

	// Phone index: a different WhatsApp name, same mobile.
	_, err = p.SyncPhones(ctx, nil)
	require.NoError(t, err)
	c, _, err = p.FindOrCreateCustomer(ctx, provider.CustomerInput{ContactID: uuid.New(), Name: "Benny", Mobile: "61400111222"})
	require.NoError(t, err)
	assert.Equal(t, "202", c.ID)

	// Same name, different mobile: not a match, so a new patient.
	c, created, err = p.FindOrCreateCustomer(ctx, provider.CustomerInput{ContactID: uuid.New(), Name: "Ava Ng", Mobile: "61411000000"})
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, "201", c.ID)
	b := f.lastBody("POST", "/v1/patients")
	assert.Equal(t, "Ava", b["first_name"])
	assert.Equal(t, "Ng", b["last_name"])
	assert.Equal(t, "+61411000000", b["patient_phone_numbers"].([]any)[0].(map[string]any)["number"])

	// A single WhatsApp name.
	_, _, err = p.FindOrCreateCustomer(ctx, provider.CustomerInput{Name: "Zed", Mobile: "61411000001"})
	require.NoError(t, err)
	assert.Equal(t, "(WhatsApp)", f.lastBody("POST", "/v1/patients")["last_name"])

	// Archived patients are never matched.
	_, created, err = p.FindOrCreateCustomer(ctx, provider.CustomerInput{Name: "Cy Old", Mobile: "61400999999"})
	require.NoError(t, err)
	assert.True(t, created)

	off, _ := newProvider(t, f, `{"create_patients": false}`)
	_, _, err = off.FindOrCreateCustomer(ctx, provider.CustomerInput{Name: "New Person", Mobile: "61411000002"})
	assert.True(t, errors.Is(err, provider.ErrNotFound), "creating patients turned off")
}

func TestBookRescheduleCancel(t *testing.T) {
	f := newFake(t)
	p, links := newProvider(t, f, "")
	contact := uuid.New()
	start := time.Date(2026, 11, 16, 22, 0, 0, 0, time.UTC) // Tue 09:00 Sydney
	a, err := p.Book(ctx, provider.BookInput{ServiceID: "21", LocationID: "11", PractitionerID: "31", Start: start,
		CustomerID: "201", ContactID: contact, Note: "Booked on WhatsApp"})
	require.NoError(t, err)
	body := f.lastBody("POST", "/individual_appointments")
	assert.Equal(t, map[string]any{"business_id": "11", "practitioner_id": "31", "appointment_type_id": "21", "patient_id": "201",
		"starts_at": "2026-11-16T22:00:00Z", "notes": "Booked on WhatsApp"}, body)
	assert.Equal(t, provider.StatusBooked, a.Status)
	assert.Equal(t, "Initial physio", a.ServiceName)
	assert.Equal(t, "Sam Lee", a.PractitionerName)
	assert.Equal(t, "Harbour St", a.LocationName)
	assert.Equal(t, "Australia/Sydney", a.TimeZone)
	assert.Equal(t, "201", a.CustomerID, "ids read from nested links")
	assert.Equal(t, a.ID, a.Reference)
	assert.Equal(t, "201", links.remote[contact])

	moved, err := p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: start.Add(time.Hour)})
	require.NoError(t, err)
	assert.True(t, moved.Start.Equal(start.Add(time.Hour)))
	patch := f.lastBody("PATCH", "/individual_appointments/"+a.ID)
	assert.Contains(t, patch, "ends_at", "ends_at null so Cliniko recomputes it")
	assert.Nil(t, patch["ends_at"])

	require.NoError(t, p.Cancel(ctx, a.ID, ""))
	assert.Equal(t, float64(50), f.lastBody("PATCH", "/cancel")["cancellation_reason"])
	assert.Equal(t, "Cancelled by the patient on WhatsApp", f.lastBody("PATCH", "/cancel")["cancellation_note"])
	got, err := p.GetAppointment(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, provider.StatusCancelled, got.Status)

	since := monday8am.Add(-time.Hour)
	list, err := p.ListAppointments(ctx, provider.AppointmentQuery{ChangedSince: &since})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Contains(t, f.requests[len(f.requests)-1].URL.RawQuery, "updated_at")

	f.bookStatus, f.bookMsg = 422, "Appointment conflicts with another appointment"
	_, err = p.Book(ctx, provider.BookInput{ServiceID: "21", LocationID: "11", PractitionerID: "31", Start: start, CustomerID: "201"})
	assert.ErrorIs(t, err, provider.ErrSlotTaken)
	f.bookMsg = "Patient can't be blank"
	_, err = p.Book(ctx, provider.BookInput{ServiceID: "21", LocationID: "11", PractitionerID: "31", Start: start})
	assert.ErrorIs(t, err, provider.ErrInvalid)
	assert.NotErrorIs(t, err, provider.ErrSlotTaken)
}

func TestSyncPhones(t *testing.T) {
	f := newFake(t)
	p, links := newProvider(t, f, "")
	res, err := p.SyncPhones(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Patients, "every page read on the first run")
	assert.Equal(t, 2, res.Phones, "archived patients' numbers skipped")
	require.NotNil(t, res.Newest)
	assert.Equal(t, []string{"201"}, links.phones["61412345678"], "local 04 numbers stored with 61")
	assert.Equal(t, []string{"202"}, links.phones["61400111222"])

	since := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	res, err = p.SyncPhones(ctx, &since)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Patients, "later runs only read patients updated since")
	q := f.requests[len(f.requests)-1].URL.Query()["q[]"]
	assert.True(t, len(q) == 1 && strings.HasPrefix(q[0], "updated_at:>2026-11-01T12:00:00Z"))
}

func TestNormalise(t *testing.T) {
	for in, want := range map[string]string{"0412 345 678": "61412345678", "+61 412 345 678": "61412345678", "0061412345678": "61412345678",
		"(02) 9999 0000": "61299990000", "+44 7700 900123": "447700900123"} {
		assert.Equal(t, want, Normalise(in), in)
	}
}
