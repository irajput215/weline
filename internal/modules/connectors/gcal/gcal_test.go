package gcal

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
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

// ---------- a fake Google ----------

type fakeGoogle struct {
	t       *testing.T
	pub     *rsa.PublicKey
	mu      sync.Mutex
	shared  map[string]string // calendar id → summary (calendars shared with Weline)
	events  map[string]map[string]*Event
	busy    map[string][]Interval // extra busy times (events Weline didn't make)
	tokens  int
	nextID  int
	srv     *httptest.Server
	limited bool
}

func newFake(t *testing.T) (*fakeGoogle, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	f := &fakeGoogle{t: t, pub: &key.PublicKey, shared: map[string]string{"clinic@group.calendar.google.com": "Harbour Physio"},
		events: map[string]map[string]*Event{}, busy: map[string][]Interval{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	sa, _ := json.Marshal(map[string]string{"client_email": "weline@weline-prod.iam.gserviceaccount.com", "private_key": pemKey,
		"token_uri": f.srv.URL + "/token"})
	return f, string(sa)
}

func (f *fakeGoogle) checkJWT(assertion string) bool {
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(f.pub, crypto.SHA256, sum[:], sig) != nil {
		return false
	}
	b, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c map[string]any
	_ = json.Unmarshal(b, &c)
	return c["iss"] == "weline@weline-prod.iam.gserviceaccount.com" && c["scope"] == scope && c["aud"] == f.srv.URL+"/token"
}

func (f *fakeGoogle) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/token" {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || !f.checkJWT(r.Form.Get("assertion")) {
			w.WriteHeader(400)
			return
		}
		f.tokens++
		_, _ = fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
		return
	}
	if r.Header.Get("Authorization") != "Bearer tok" {
		w.WriteHeader(401)
		return
	}
	if f.limited {
		w.WriteHeader(403)
		_, _ = fmt.Fprint(w, `{"error":{"errors":[{"reason":"rateLimitExceeded"}]}}`)
		return
	}
	path := strings.TrimPrefix(r.URL.EscapedPath(), "/calendar/v3")
	if path == "/freeBusy" {
		var in struct {
			TimeMin, TimeMax string
			Items            []struct{ ID string }
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		min, _ := time.Parse(time.RFC3339, in.TimeMin)
		max, _ := time.Parse(time.RFC3339, in.TimeMax)
		out := map[string]any{}
		for _, it := range in.Items {
			if _, ok := f.shared[it.ID]; !ok {
				out[it.ID] = map[string]any{"errors": []any{map[string]string{"reason": "notFound"}}}
				continue
			}
			busy := []Interval{}
			for _, b := range f.busy[it.ID] {
				if b.Start.Before(max) && b.End.After(min) {
					busy = append(busy, b)
				}
			}
			for _, e := range f.events[it.ID] {
				s, _ := time.Parse(time.RFC3339, e.Start.DateTime)
				en, _ := time.Parse(time.RFC3339, e.End.DateTime)
				if e.Status != "cancelled" && s.Before(max) && en.After(min) {
					busy = append(busy, Interval{Start: s.UTC(), End: en.UTC()})
				}
			}
			out[it.ID] = map[string]any{"busy": busy}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"calendars": out})
		return
	}
	// /calendars/{id}[/events[/{eventId}]]
	rest := strings.TrimPrefix(path, "/calendars/")
	seg := strings.Split(rest, "/")
	cal, _ := url.PathUnescape(seg[0])
	if _, ok := f.shared[cal]; !ok {
		w.WriteHeader(404)
		_, _ = fmt.Fprint(w, `{"error":{"errors":[{"reason":"notFound"}]}}`)
		return
	}
	if f.events[cal] == nil {
		f.events[cal] = map[string]*Event{}
	}
	switch {
	case len(seg) == 1:
		_ = json.NewEncoder(w).Encode(Calendar{ID: cal, Summary: f.shared[cal], TimeZone: "Australia/Sydney"})
	case len(seg) == 2 && r.Method == http.MethodPost:
		var e Event
		_ = json.NewDecoder(r.Body).Decode(&e)
		f.nextID++
		e.ID, e.Status, e.Updated = fmt.Sprintf("ev%04d", f.nextID), "confirmed", time.Now().UTC().Format(time.RFC3339)
		f.events[cal][e.ID] = &e
		_ = json.NewEncoder(w).Encode(e)
	case len(seg) == 2 && r.Method == http.MethodGet:
		q := r.URL.Query()
		props := q["privateExtendedProperty"]
		var items []Event
		for _, e := range f.events[cal] {
			ok := true
			for _, p := range props {
				k, v, _ := strings.Cut(p, "=")
				ok = ok && e.Private(k) == v
			}
			if e.Status == "cancelled" && q.Get("showDeleted") != "true" {
				ok = false
			}
			if tm := q.Get("timeMin"); tm != "" {
				min, _ := time.Parse(time.RFC3339, tm)
				en, _ := time.Parse(time.RFC3339, e.End.DateTime)
				ok = ok && en.After(min)
			}
			if tm := q.Get("timeMax"); tm != "" {
				max, _ := time.Parse(time.RFC3339, tm)
				s, _ := time.Parse(time.RFC3339, e.Start.DateTime)
				ok = ok && s.Before(max)
			}
			if ok {
				items = append(items, *e)
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID }) // stable pages
		// Page in twos to exercise paging.
		start := 0
		if tok := q.Get("pageToken"); tok != "" {
			_, _ = fmt.Sscan(tok, &start)
		}
		end := start + 2
		next := ""
		if end < len(items) {
			next = fmt.Sprint(end)
		} else {
			end = len(items)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items[start:end], "nextPageToken": next})
	case len(seg) == 3:
		e := f.events[cal][seg[2]]
		if e == nil {
			w.WriteHeader(404)
			return
		}
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(e)
		case http.MethodDelete:
			e.Status = "cancelled"
			w.WriteHeader(204)
		case http.MethodPatch:
			b, _ := io.ReadAll(r.Body)
			var patch Event
			_ = json.Unmarshal(b, &patch)
			if patch.Start.DateTime != "" {
				e.Start, e.End = patch.Start, patch.End
			}
			if patch.ExtendedProperties != nil {
				for k, v := range patch.ExtendedProperties.Private {
					e.ExtendedProperties.Private[k] = v
				}
			}
			_ = json.NewEncoder(w).Encode(e)
		}
	}
}

// ---------- Weline's own diary, as the base ----------

type base struct {
	times []provider.FreeTime
}

var syd, _ = time.LoadLocation("Australia/Sydney")

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
	out := &provider.FreeTimes{WindowDays: 28}
	for _, t := range b.times {
		if q.PractitionerID == "" || t.PractitionerID == q.PractitionerID {
			out.Times = append(out.Times, t)
		}
	}
	return out, nil
}
func (b *base) FindOrCreateCustomer(_ context.Context, in provider.CustomerInput) (*provider.Customer, bool, error) {
	return &provider.Customer{ID: in.ContactID.String(), Name: in.Name}, false, nil
}
func (b *base) GetCustomer(_ context.Context, id string) (*provider.Customer, error) {
	return &provider.Customer{ID: id, Name: "Pat Kim", Mobile: "61412345678"}, nil
}
func (b *base) Book(context.Context, provider.BookInput) (*provider.Appointment, error) {
	panic("the calendar books, not the base")
}
func (b *base) Reschedule(context.Context, string, provider.RescheduleInput) (*provider.Appointment, error) {
	panic("not used")
}
func (b *base) Cancel(context.Context, string, string) error { panic("not used") }
func (b *base) GetAppointment(context.Context, string) (*provider.Appointment, error) {
	panic("not used")
}
func (b *base) ListAppointments(context.Context, provider.AppointmentQuery) ([]provider.Appointment, error) {
	panic("not used")
}

const clinicCal = "clinic@group.calendar.google.com"

func monday(h, m int) time.Time { return time.Date(2027, 3, 8, h, m, 0, 0, syd) }

func newProvider(t *testing.T, f *fakeGoogle, sa string, raw string) *Provider {
	t.Helper()
	c, err := NewClient(sa, Options{API: f.srv.URL + "/calendar/v3"})
	require.NoError(t, err)
	set, err := ParseSettings(clinicCal, json.RawMessage(raw))
	require.NoError(t, err)
	b := &base{}
	for _, h := range []int{9, 10, 11} {
		for _, pr := range []string{"pr1"} {
			b.times = append(b.times, provider.FreeTime{Start: monday(h, 0), End: monday(h, 30), PractitionerID: pr, LocationID: "loc1"})
		}
	}
	p := New(c, b, set)
	p.now = func() time.Time { return monday(7, 0) }
	return p
}

func TestServiceAccountAndConnection(t *testing.T) {
	f, sa := newFake(t)
	_, _, err := ParseServiceAccount(`{"client_email":"x"}`)
	assert.Error(t, err)
	p := newProvider(t, f, sa, "")
	msg, err := p.TestConnection(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Connected to Google Calendar: Harbour Physio", msg)
	_, err = p.TestConnection(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, f.tokens, "the token is reused until near expiry")

	notShared := newProvider(t, f, sa, `{"practitioner_calendars":{"pr2":"ada@example.com"}}`)
	_, err = notShared.TestConnection(ctx)
	require.ErrorIs(t, err, kit.ErrInvalid)
	assert.Contains(t, err.Error(), "share it with weline@weline-prod.iam.gserviceaccount.com")

	for _, bad := range []string{"", "has space", "a/b"} {
		_, err := ParseSettings(bad, nil)
		assert.ErrorIs(t, err, kit.ErrInvalid, bad)
	}
	_, err = ParseSettings(clinicCal, json.RawMessage(`{"other":1}`))
	assert.ErrorIs(t, err, kit.ErrInvalid, "unknown settings refused")
}

func TestFreeTimesSkipBusyAndBookingLifecycle(t *testing.T) {
	f, sa := newFake(t)
	p := newProvider(t, f, sa, "")
	// 10:00–10:30 is busy with something the business put in the calendar.
	f.busy[clinicCal] = []Interval{{Start: monday(10, 0).UTC(), End: monday(10, 30).UTC()}}
	ft, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "s1", LocationID: "loc1", Limit: 5})
	require.NoError(t, err)
	require.Len(t, ft.Times, 2)
	assert.True(t, ft.Times[0].Start.Equal(monday(9, 0)))
	assert.True(t, ft.Times[1].Start.Equal(monday(11, 0)), "10:00 is busy in the calendar")

	a, err := p.Book(ctx, provider.BookInput{ServiceID: "s1", LocationID: "loc1", PractitionerID: "pr1", Start: monday(9, 0), CustomerID: "contact-1", Source: provider.SourceWhatsApp})
	require.NoError(t, err)
	assert.Equal(t, provider.StatusBooked, a.Status)
	assert.Equal(t, "Initial physio", a.ServiceName)
	assert.Equal(t, "Sam Lee", a.PractitionerName)
	assert.Equal(t, "Harbour St", a.LocationName)
	assert.True(t, strings.HasSuffix(a.ID, "@"+clinicCal))
	ev := f.events[clinicCal][strings.Split(a.ID, "@")[0]]
	assert.Equal(t, "Initial physio – Pat Kim", ev.Summary)
	assert.Equal(t, "contact-1", ev.Private("weline_contact"))
	assert.Equal(t, "Australia/Sydney", ev.Start.TimeZone)
	assert.NotContains(t, ev.Summary+ev.Description, "61412345678", "no phone numbers in the calendar")

	_, err = p.Book(ctx, provider.BookInput{ServiceID: "s1", LocationID: "loc1", PractitionerID: "pr1", Start: monday(9, 15), CustomerID: "contact-2"})
	assert.ErrorIs(t, err, provider.ErrSlotTaken, "overlaps the booking just made")
	_, err = p.Book(ctx, provider.BookInput{ServiceID: "s1", LocationID: "loc1", PractitionerID: "pr1", Start: monday(10, 0), CustomerID: "contact-2"})
	assert.ErrorIs(t, err, provider.ErrSlotTaken, "busy in the calendar")

	ft, err = p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "s1", LocationID: "loc1", Limit: 5})
	require.NoError(t, err)
	assert.Len(t, ft.Times, 1, "9:00 is now booked")

	moved, err := p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: monday(9, 15)})
	require.NoError(t, err, "moving within its own time is fine")
	assert.True(t, moved.Start.Equal(monday(9, 15)))
	assert.True(t, moved.End.Equal(monday(9, 45)))
	_, err = p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: monday(9, 50)})
	assert.ErrorIs(t, err, provider.ErrSlotTaken, "runs into the busy 10:00")

	got, err := p.GetAppointment(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "contact-1", got.CustomerID)

	from, to := monday(0, 0), monday(23, 59)
	list, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	require.NoError(t, err)
	require.Len(t, list, 1, "only Weline's events, not the business's own")

	require.NoError(t, p.Cancel(ctx, a.ID, "patient cancelled"))
	got, err = p.GetAppointment(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, provider.StatusCancelled, got.Status)
	_, err = p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: monday(11, 0)})
	assert.ErrorIs(t, err, kit.ErrConflict)

	_, err = p.GetAppointment(ctx, "nonsense")
	assert.ErrorIs(t, err, provider.ErrNotFound)
	_, err = p.GetAppointment(ctx, "ev9999@"+clinicCal)
	assert.ErrorIs(t, err, provider.ErrNotFound)
}

func TestListAppointments_PagesCustomersAndCompleted(t *testing.T) {
	f, sa := newFake(t)
	p := newProvider(t, f, sa, "")
	for i, h := range []int{9, 10, 11} {
		_, err := p.Book(ctx, provider.BookInput{ServiceID: "s1", LocationID: "loc1", PractitionerID: "pr1", Start: monday(h, 0),
			CustomerID: fmt.Sprintf("contact-%d", i%2)})
		require.NoError(t, err)
	}
	from, to := monday(0, 0), monday(23, 59)
	all, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	require.NoError(t, err)
	assert.Len(t, all, 3, "followed the next page")
	assert.True(t, all[0].Start.Before(all[1].Start), "soonest first")
	mine, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to, CustomerID: "contact-0"})
	require.NoError(t, err)
	assert.Len(t, mine, 2)

	p.now = func() time.Time { return monday(12, 0) }
	past, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	require.NoError(t, err)
	assert.Equal(t, provider.StatusCompleted, past[0].Status, "past appointments count as visits for recalls")

	f.limited = true
	_, err = p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	assert.ErrorIs(t, err, kit.ErrTooMany)
}

func TestPractitionerCalendars(t *testing.T) {
	f, sa := newFake(t)
	f.shared["ada@example.com"] = "Ada"
	p := newProvider(t, f, sa, `{"practitioner_calendars":{"pr2":"ada@example.com"}}`)
	p.base.(*base).times = append(p.base.(*base).times, provider.FreeTime{Start: monday(9, 0), End: monday(9, 30), PractitionerID: "pr2", LocationID: "loc1"})
	f.busy["ada@example.com"] = []Interval{{Start: monday(9, 0).UTC(), End: monday(9, 30).UTC()}}
	ft, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: "s1", LocationID: "loc1", Limit: 10})
	require.NoError(t, err)
	for _, tm := range ft.Times {
		assert.False(t, tm.PractitionerID == "pr2" && tm.Start.Equal(monday(9, 0)), "Ada's own calendar is busy at 9")
	}
	a, err := p.Book(ctx, provider.BookInput{ServiceID: "s1", LocationID: "loc1", PractitionerID: "pr2", Start: monday(11, 0), CustomerID: "c"})
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(a.ID, "@ada@example.com"), "booked in Ada's calendar")
	_, err = p.Reschedule(ctx, a.ID, provider.RescheduleInput{Start: monday(11, 30), PractitionerID: "pr1"})
	assert.ErrorIs(t, err, kit.ErrInvalid, "moving between calendars is done in Google")
}
