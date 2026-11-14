package calendly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type fakeCalendly struct {
	mu         sync.Mutex
	srv        *httptest.Server
	availCalls int
	cancelled  []string
	links      int
	status     int // forced status for every call
	offAPINext bool
}

const evA, evB, evC = "a1b2c3d4-0000-0000-0000-000000000001", "a1b2c3d4-0000-0000-0000-000000000002", "a1b2c3d4-0000-0000-0000-000000000003"

var t0 = time.Date(2027, 3, 8, 0, 0, 0, 0, time.UTC) // "now" in tests

func newFake(t *testing.T) *fakeCalendly {
	f := &fakeCalendly{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCalendly) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer good-token" {
		w.WriteHeader(401)
		return
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	api := f.srv.URL
	out := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	q := r.URL.Query()
	switch {
	case r.URL.Path == "/users/me":
		out(map[string]any{"resource": map[string]any{"uri": api + "/users/U1", "name": "Sam Lee", "timezone": "Australia/Brisbane"}})
	case r.URL.Path == "/event_types":
		if q.Get("page") == "2" {
			out(map[string]any{"collection": []any{map[string]any{"uri": api + "/event_types/E2", "name": "Follow-up", "duration": 15, "active": true}},
				"pagination": map[string]any{"next_page": ""}})
			return
		}
		out(map[string]any{"collection": []any{map[string]any{"uri": api + "/event_types/E1", "name": "Initial consult", "duration": 30, "active": true}},
			"pagination": map[string]any{"next_page": api + "/event_types?page=2"}})
	case r.URL.Path == "/event_type_available_times":
		f.availCalls++
		start, _ := time.Parse(time.RFC3339, q.Get("start_time"))
		end, _ := time.Parse(time.RFC3339, q.Get("end_time"))
		if end.Sub(start) > 7*24*time.Hour {
			w.WriteHeader(400)
			return
		}
		var c []any
		for d := start.Truncate(24 * time.Hour).Add(24*time.Hour + 23*time.Hour); d.Before(end); d = d.Add(24 * time.Hour) {
			c = append(c, map[string]any{"status": "available", "start_time": d.Format(time.RFC3339)})
		}
		out(map[string]any{"collection": c})
	case r.URL.Path == "/scheduling_links" && r.Method == http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["owner"] != api+"/event_types/E1" || body["owner_type"] != "EventType" || body["max_event_count"] != float64(1) {
			w.WriteHeader(400)
			return
		}
		f.links++
		out(map[string]any{"resource": map[string]any{"booking_url": "https://calendly.com/d/abc-123/initial-consult"}})
	case r.URL.Path == "/scheduled_events":
		next := ""
		if q.Get("page") == "" {
			next = api + "/scheduled_events?page=2"
			if f.offAPINext {
				next = "https://evil.example/scheduled_events?page=2"
			}
			out(map[string]any{"collection": []any{event(api, evA, t0.Add(26*time.Hour), "active"), event(api, evB, t0.Add(50*time.Hour), "canceled")},
				"pagination": map[string]any{"next_page": next}})
			return
		}
		out(map[string]any{"collection": []any{event(api, evC, t0.Add(-48*time.Hour), "active")}, "pagination": map[string]any{"next_page": ""}})
	case strings.HasSuffix(r.URL.Path, "/invitees"):
		id := strings.Split(r.URL.Path, "/")[2]
		inv := map[string]any{"uri": api + "/scheduled_events/" + id + "/invitees/I-" + id[len(id)-1:], "name": "Pat Kim", "status": "active",
			"reschedule_url": "https://calendly.com/reschedulings/" + id}
		switch id {
		case evA:
			inv["text_reminder_number"] = "+61 412 345 678"
		case evB:
			inv["questions_and_answers"] = []any{map[string]any{"question": "Mobile number", "answer": "0400 111 222"}}
		}
		out(map[string]any{"collection": []any{inv}, "pagination": map[string]any{"next_page": ""}})
	case strings.HasSuffix(r.URL.Path, "/cancellation") && r.Method == http.MethodPost:
		f.cancelled = append(f.cancelled, strings.Split(r.URL.Path, "/")[2])
		out(map[string]any{"resource": map[string]any{}})
	case strings.HasPrefix(r.URL.Path, "/scheduled_events/"):
		id := strings.TrimPrefix(r.URL.Path, "/scheduled_events/")
		if id != evA {
			w.WriteHeader(404)
			return
		}
		out(map[string]any{"resource": event(api, evA, t0.Add(26*time.Hour), "active")})
	default:
		w.WriteHeader(404)
	}
}

func event(api, id string, start time.Time, status string) map[string]any {
	return map[string]any{"uri": api + "/scheduled_events/" + id, "name": "Initial consult", "status": status,
		"start_time": start.Format(time.RFC3339), "end_time": start.Add(30 * time.Minute).Format(time.RFC3339),
		"event_type": api + "/event_types/E1", "updated_at": t0.Format(time.RFC3339),
		"event_memberships": []any{map[string]any{"user": api + "/users/U1"}}}
}

func newProvider(t *testing.T, f *fakeCalendly, token string) *Provider {
	t.Helper()
	c, err := NewClient(token, f.srv.URL, f.srv.Client())
	require.NoError(t, err)
	p := New(c)
	p.now = func() time.Time { return t0 }
	return p
}

func TestConnectAndCatalog(t *testing.T) {
	f := newFake(t)
	_, err := NewClient("  ", "", nil)
	assert.ErrorIs(t, err, kit.ErrInvalid)
	bad := newProvider(t, f, "wrong")
	_, err = bad.TestConnection(ctx)
	require.ErrorIs(t, err, kit.ErrInvalid)
	assert.Contains(t, err.Error(), "refused the token")

	p := newProvider(t, f, "good-token")
	msg, err := p.TestConnection(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Connected to Calendly as Sam Lee", msg)
	svcs, err := p.ListServices(ctx)
	require.NoError(t, err)
	assert.Equal(t, []provider.Service{{ID: f.srv.URL + "/event_types/E1", Name: "Initial consult", Minutes: 30},
		{ID: f.srv.URL + "/event_types/E2", Name: "Follow-up", Minutes: 15}}, svcs, "followed the next page")
	locs, err := p.ListLocations(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Australia/Brisbane", locs[0].TimeZone)
	prs, err := p.ListPractitioners(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "Sam Lee", prs[0].Name)
	assert.NoError(t, ParseSettings(nil))
	assert.ErrorIs(t, ParseSettings(json.RawMessage(`{"x":1}`)), kit.ErrInvalid)
}

func TestFreeTimesAndLinks(t *testing.T) {
	f := newFake(t)
	p := newProvider(t, f, "good-token")
	e1 := f.srv.URL + "/event_types/E1"
	ft, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: e1, LocationID: "calendly", Limit: 10})
	require.NoError(t, err)
	assert.Len(t, ft.Times, 10)
	assert.Equal(t, 2, f.availCalls, "a week at a time, until enough times")
	assert.Equal(t, 30*time.Minute, ft.Times[0].End.Sub(ft.Times[0].Start))
	assert.Equal(t, "Sam Lee", ft.Times[0].PractitionerName)
	_, err = p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "nope"})
	assert.ErrorIs(t, err, kit.ErrInvalid)

	_, err = p.Book(ctx, provider.BookInput{ServiceID: e1})
	assert.ErrorIs(t, err, provider.ErrNotSupported, "bookings happen on Calendly's page")
	link, err := p.BookingLink(ctx, e1)
	require.NoError(t, err)
	assert.Equal(t, "https://calendly.com/d/abc-123/initial-consult", link)
	assert.Equal(t, 1, f.links)
	rl, err := p.RescheduleLink(ctx, evA)
	require.NoError(t, err)
	assert.Equal(t, "https://calendly.com/reschedulings/"+evA, rl)
	_, err = p.RescheduleLink(ctx, "../../users/me")
	assert.Error(t, err, "ids are UUIDs only")
}

func TestAppointmentsCustomersAndCancel(t *testing.T) {
	f := newFake(t)
	p := newProvider(t, f, "good-token")
	from, to := t0.Add(-72*time.Hour), t0.Add(72*time.Hour)
	all, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	require.NoError(t, err)
	require.Len(t, all, 3, "followed the next page")
	byID := map[string]provider.Appointment{}
	for _, a := range all {
		byID[a.ID] = a
	}
	assert.Equal(t, provider.StatusBooked, byID[evA].Status)
	assert.Equal(t, "phone:61412345678", byID[evA].CustomerID, "the SMS reminder number, normalised")
	assert.Equal(t, provider.StatusCancelled, byID[evB].Status)
	assert.Equal(t, "phone:61400111222", byID[evB].CustomerID, "or a mobile question's answer")
	assert.Equal(t, provider.StatusCompleted, byID[evC].Status, "past events count as visits")
	assert.True(t, strings.HasPrefix(byID[evC].CustomerID, "invitee:"), "no mobile: an invitee id, short")
	assert.LessOrEqual(t, len(byID[evC].CustomerID), 64)
	assert.Equal(t, "Initial consult", byID[evA].ServiceName)

	cust, _, err := p.FindOrCreateCustomer(ctx, provider.CustomerInput{Name: "Pat", Mobile: "0412 345 678"})
	require.NoError(t, err)
	assert.Equal(t, "phone:61412345678", cust.ID)
	mine, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to, CustomerID: cust.ID})
	require.NoError(t, err)
	require.Len(t, mine, 1)
	assert.Equal(t, evA, mine[0].ID)
	got, err := p.GetCustomer(ctx, cust.ID)
	require.NoError(t, err)
	assert.Equal(t, "61412345678", got.Mobile, "reminders get the mobile")
	none, err := p.GetCustomer(ctx, byID[evC].CustomerID)
	require.NoError(t, err)
	assert.Empty(t, none.Mobile)
	_, _, err = p.FindOrCreateCustomer(ctx, provider.CustomerInput{Name: "No number"})
	assert.ErrorIs(t, err, provider.ErrNotFound)

	a, err := p.GetAppointment(ctx, evA)
	require.NoError(t, err)
	assert.Equal(t, "phone:61412345678", a.CustomerID)
	_, err = p.GetAppointment(ctx, evB)
	assert.ErrorIs(t, err, provider.ErrNotFound)
	require.NoError(t, p.Cancel(ctx, evA, ""))
	assert.Equal(t, []string{evA}, f.cancelled)

	f.offAPINext = true
	_, err = p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	assert.ErrorIs(t, err, provider.ErrInvalid, "pagination is only followed back to Calendly's API")
	f.offAPINext = false
	f.status = 429
	_, err = p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	assert.ErrorIs(t, err, kit.ErrTooMany)
}
