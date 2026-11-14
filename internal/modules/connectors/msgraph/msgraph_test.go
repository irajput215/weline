package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
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

const (
	tenant  = "11111111-2222-3333-4444-555555555555"
	other   = "99999999-2222-3333-4444-555555555555" // a tenant that never approved Weline
	mailbox = "bookings@harbourphysio.com.au"
)

type fakeMS struct {
	mu        sync.Mutex
	srv       *httptest.Server
	approved  map[string]bool
	mailboxes map[string]bool
	events    map[string]map[string]*Event // mailbox → id → event
	busy      map[string][]Interval
	tokens    int
	nextID    int
	throttle  bool
}

func newFake(t *testing.T) *fakeMS {
	f := &fakeMS{approved: map[string]bool{tenant: true}, mailboxes: map[string]bool{mailbox: true},
		events: map[string]map[string]*Event{}, busy: map[string][]Interval{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func writeJSON(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }

func (f *fakeMS) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/login/") {
		parts := strings.Split(r.URL.Path, "/") // /login/{tenant}/oauth2/v2.0/token
		_ = r.ParseForm()
		if !f.approved[parts[2]] || r.Form.Get("client_secret") != "s3cret" || r.Form.Get("grant_type") != "client_credentials" {
			w.WriteHeader(400)
			return
		}
		f.tokens++
		writeJSON(w, map[string]any{"access_token": "tok-" + parts[2], "expires_in": 3600})
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok-") || r.Header.Get("Prefer") != `outlook.timezone="UTC"` {
		w.WriteHeader(401)
		return
	}
	if f.throttle {
		w.WriteHeader(429)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/graph/v1.0/users/")
	seg := strings.SplitN(p, "/", 2)
	mb, _ := url.PathUnescape(seg[0])
	if !f.mailboxes[mb] {
		w.WriteHeader(404)
		return
	}
	if f.events[mb] == nil {
		f.events[mb] = map[string]*Event{}
	}
	rest := ""
	if len(seg) > 1 {
		rest = seg[1]
	}
	switch {
	case rest == "calendar":
		writeJSON(w, Calendar{Name: "Calendar"})
	case rest == "calendar/getSchedule":
		var in struct {
			Schedules []string
			StartTime DateTime
			EndTime   DateTime
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		min, max := in.StartTime.Time(), in.EndTime.Time()
		var vals []any
		for _, s := range in.Schedules {
			if !f.mailboxes[s] {
				vals = append(vals, map[string]any{"scheduleId": s, "error": map[string]string{"message": "not found"}})
				continue
			}
			items := []any{}
			add := func(st string, a, b time.Time) {
				if a.Before(max) && b.After(min) {
					items = append(items, map[string]any{"status": st, "start": utc(a), "end": utc(b)})
				}
			}
			for _, b := range f.busy[s] {
				add("busy", b.Start, b.End)
			}
			for _, e := range f.events[s] {
				add("busy", e.Start.Time(), e.End.Time())
			}
			add("free", min, max) // free blocks are ignored
			vals = append(vals, map[string]any{"scheduleId": s, "scheduleItems": items})
		}
		writeJSON(w, map[string]any{"value": vals})
	case rest == "events" && r.Method == http.MethodPost:
		var e Event
		_ = json.NewDecoder(r.Body).Decode(&e)
		f.nextID++
		e.ID = fmt.Sprintf("AAMk-%04d=", f.nextID)
		e.LastModifiedDateTime = time.Now().UTC().Format(time.RFC3339)
		f.events[mb][e.ID] = &e
		cp := e
		cp.Properties = nil // Graph doesn't echo extended properties on create
		writeJSON(w, cp)
	case strings.HasPrefix(rest, "events/"):
		id, _ := url.PathUnescape(strings.TrimPrefix(rest, "events/"))
		e := f.events[mb][id]
		if e == nil {
			w.WriteHeader(404)
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, e)
		case http.MethodPatch:
			var patch Event
			_ = json.NewDecoder(r.Body).Decode(&patch)
			e.Start, e.End = patch.Start, patch.End
			writeJSON(w, e)
		case http.MethodDelete:
			delete(f.events[mb], id)
			w.WriteHeader(204)
		}
	case rest == "calendarView":
		q := r.URL.Query()
		from, _ := time.Parse(time.RFC3339, q.Get("startDateTime"))
		to, _ := time.Parse(time.RFC3339, q.Get("endDateTime"))
		var items []Event
		for _, e := range f.events[mb] {
			if e.Start.Time().Before(to) && e.End.Time().After(from) {
				items = append(items, *e)
			}
		}
		items = append(items, Event{ID: "theirs", Subject: "Dentist", Start: utc(from.Add(time.Hour)), End: utc(from.Add(2 * time.Hour))})
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		skip := 0
		_, _ = fmt.Sscan(q.Get("$skip"), &skip)
		end, next := skip+2, ""
		if end < len(items) {
			nq := r.URL.Query()
			nq.Set("$skip", fmt.Sprint(end))
			next = f.srv.URL + r.URL.Path + "?" + nq.Encode()
		} else {
			end = len(items)
		}
		writeJSON(w, map[string]any{"value": items[skip:end], "@odata.nextLink": next})
	default:
		w.WriteHeader(404)
	}
}

type base struct{ times []provider.FreeTime }

func (b *base) Name() string { return provider.BuiltIn }
func (b *base) ListLocations(context.Context) ([]provider.Location, error) {
	return []provider.Location{{ID: "loc1", Name: "Harbour St", TimeZone: "Australia/Sydney"}}, nil
}
func (b *base) ListPractitioners(context.Context, string) ([]provider.Practitioner, error) {
	return []provider.Practitioner{{ID: "pr1", Name: "Sam Lee"}, {ID: "pr2", Name: "Ada Ng"}}, nil
}
func (b *base) ListServices(context.Context) ([]provider.Service, error) {
	return []provider.Service{{ID: "s1", Name: "Initial physio", Minutes: 30}}, nil
}
func (b *base) FindFreeTimes(_ context.Context, q provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	return &provider.FreeTimes{Times: b.times, WindowDays: 28}, nil
}
func (b *base) FindOrCreateCustomer(_ context.Context, in provider.CustomerInput) (*provider.Customer, bool, error) {
	return &provider.Customer{ID: in.ContactID.String()}, false, nil
}
func (b *base) GetCustomer(_ context.Context, id string) (*provider.Customer, error) {
	return &provider.Customer{ID: id, Name: "Pat Kim", Mobile: "61412345678"}, nil
}
func (b *base) Book(context.Context, provider.BookInput) (*provider.Appointment, error) { panic("no") }
func (b *base) Reschedule(context.Context, string, provider.RescheduleInput) (*provider.Appointment, error) {
	panic("no")
}
func (b *base) Cancel(context.Context, string, string) error { panic("no") }
func (b *base) GetAppointment(context.Context, string) (*provider.Appointment, error) {
	panic("no")
}
func (b *base) ListAppointments(context.Context, provider.AppointmentQuery) ([]provider.Appointment, error) {
	panic("no")
}

var syd, _ = time.LoadLocation("Australia/Sydney")

func monday(h, m int) time.Time { return time.Date(2027, 3, 8, h, m, 0, 0, syd) }

func newProvider(t *testing.T, f *fakeMS, ten, raw string) *Provider {
	t.Helper()
	c, err := NewClient(App{ClientID: "weline-app", ClientSecret: "s3cret"}, Options{Login: f.srv.URL + "/login", API: f.srv.URL + "/graph/v1.0"})
	require.NoError(t, err)
	set, err := ParseSettings(ten, mailbox, json.RawMessage(raw))
	require.NoError(t, err)
	b := &base{}
	for _, h := range []int{9, 10, 11} {
		b.times = append(b.times, provider.FreeTime{Start: monday(h, 0), End: monday(h, 30), PractitionerID: "pr1", LocationID: "loc1"})
	}
	p := New(c, b, set)
	p.now = func() time.Time { return monday(7, 0) }
	return p
}

func TestConsentConnectionAndSettings(t *testing.T) {
	f := newFake(t)
	_, err := NewClient(App{ClientID: "x"}, Options{})
	assert.Error(t, err)
	p := newProvider(t, f, tenant, "")
	link := p.c.ConsentURL("https://weline.test/x/connectors/microsoft/consent", "state123")
	assert.Contains(t, link, "/organizations/v2.0/adminconsent?")
	assert.Contains(t, link, "client_id=weline-app")
	assert.Contains(t, link, "state=state123")

	msg, err := p.TestConnection(ctx)
	require.NoError(t, err)
	assert.Contains(t, msg, mailbox)
	_, _ = p.TestConnection(ctx)
	assert.Equal(t, 1, f.tokens, "one token per tenant, reused")

	notApproved := newProvider(t, f, other, "")
	_, err = notApproved.TestConnection(ctx)
	require.ErrorIs(t, err, kit.ErrInvalid)
	assert.Contains(t, err.Error(), "approved Weline")

	noMailbox := newProvider(t, f, tenant, `{"practitioner_mailboxes":{"pr2":"ada@harbourphysio.com.au"}}`)
	_, err = noMailbox.TestConnection(ctx)
	assert.ErrorIs(t, err, kit.ErrInvalid)

	for name, args := range map[string][2]string{"tenant": {"not-a-guid", mailbox}, "mailbox": {tenant, "Bookings <x@y.z>"}} {
		_, err := ParseSettings(args[0], args[1], nil)
		assert.ErrorIs(t, err, kit.ErrInvalid, name)
	}
	_, err = ParseSettings(tenant, mailbox, json.RawMessage(`{"x":1}`))
	assert.ErrorIs(t, err, kit.ErrInvalid)
	s, err := ParseSettings(tenant, " Bookings@HarbourPhysio.com.au ", nil)
	require.NoError(t, err)
	assert.Equal(t, mailbox, s.Mailbox, "mailboxes compared in lower case")
}

func TestFreeTimesAndBookingLifecycle(t *testing.T) {
	f := newFake(t)
	p := newProvider(t, f, tenant, "")
	f.busy[mailbox] = []Interval{{Start: monday(10, 0).UTC(), End: monday(10, 30).UTC()}}
	ft, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "s1", LocationID: "loc1", Limit: 5})
	require.NoError(t, err)
	require.Len(t, ft.Times, 2)
	assert.True(t, ft.Times[1].Start.Equal(monday(11, 0)), "10:00 is busy in Outlook")

	a, err := p.Book(ctx, provider.BookInput{ServiceID: "s1", LocationID: "loc1", PractitionerID: "pr1", Start: monday(9, 0), CustomerID: "contact-1"})
	require.NoError(t, err)
	assert.Equal(t, "Initial physio", a.ServiceName)
	assert.Equal(t, "Australia/Sydney", a.TimeZone)
	assert.True(t, strings.HasPrefix(a.ID, mailbox+"~AAMk-"))
	var stored *Event
	for _, e := range f.events[mailbox] {
		stored = e
	}
	assert.Equal(t, "Initial physio – Pat Kim", stored.Subject)
	assert.Equal(t, "UTC", stored.Start.TimeZone)
	assert.True(t, stored.Start.Time().Equal(monday(9, 0)))
	require.NotNil(t, stored.Link())
	assert.Equal(t, "contact-1", stored.Link().Contact)
	b, _ := json.Marshal(stored)
	assert.NotContains(t, string(b), "61412345678", "no phone numbers in Outlook")

	_, err = p.Book(ctx, provider.BookInput{ServiceID: "s1", LocationID: "loc1", PractitionerID: "pr1", Start: monday(9, 15), CustomerID: "c2"})
	assert.ErrorIs(t, err, provider.ErrSlotTaken)

	moved, err := p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: monday(9, 15)})
	require.NoError(t, err)
	assert.True(t, moved.End.Equal(monday(9, 45)))
	_, err = p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: monday(9, 50)})
	assert.ErrorIs(t, err, provider.ErrSlotTaken)
	_, err = p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: monday(11, 0), PractitionerID: "pr2"})
	require.NoError(t, err, "pr2 uses the same mailbox here")

	got, err := p.GetAppointment(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "contact-1", got.CustomerID)
	_, err = p.GetAppointment(ctx, "someone@else.com~AAMk-0001=")
	assert.ErrorIs(t, err, provider.ErrNotFound, "only this organization's mailboxes")
	_, err = p.GetAppointment(ctx, "garbage")
	assert.ErrorIs(t, err, provider.ErrNotFound)

	require.NoError(t, p.Cancel(ctx, a.ID, "patient"))
	_, err = p.GetAppointment(ctx, a.ID)
	assert.ErrorIs(t, err, provider.ErrNotFound, "cancelled events are removed from Outlook")
}

func TestListAppointments(t *testing.T) {
	f := newFake(t)
	p := newProvider(t, f, tenant, "")
	for i, h := range []int{9, 10, 11} {
		_, err := p.Book(ctx, provider.BookInput{ServiceID: "s1", LocationID: "loc1", PractitionerID: "pr1", Start: monday(h, 0), CustomerID: fmt.Sprintf("c%d", i%2)})
		require.NoError(t, err)
	}
	from, to := monday(0, 0), monday(23, 0)
	all, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	require.NoError(t, err)
	assert.Len(t, all, 3, "Weline's events only, across pages; the business's own 'Dentist' left out")
	assert.True(t, all[0].Start.Before(all[2].Start))
	mine, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to, CustomerID: "c0"})
	require.NoError(t, err)
	assert.Len(t, mine, 2)
	p.now = func() time.Time { return monday(12, 0) }
	past, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	require.NoError(t, err)
	assert.Equal(t, provider.StatusCompleted, past[0].Status)

	f.throttle = true
	_, err = p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	assert.ErrorIs(t, err, kit.ErrTooMany)
}
