package cliniko

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeCliniko serves hand-made fixtures shaped like Cliniko's API (string
// ids, UTC times, links.next paging, nested link objects), from the spike.
// Replace with recorded trial responses once a trial key exists.
type fakeCliniko struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	requests []*http.Request
	bodies   []map[string]any
	// state
	patients     []map[string]any
	appointments map[string]map[string]any
	nextID       int
	// behaviour
	status429  int // answer this many 429s first
	resetIn    time.Duration
	bookStatus int    // answer POST /individual_appointments with this status
	bookMsg    string // and this message
	fail500    bool
	perPage    int
}

const fakeKey = "MS0xLWl4SzYYtestkey-au3"

func newFake(t *testing.T) *fakeCliniko {
	f := &fakeCliniko{t: t, appointments: map[string]map[string]any{}, nextID: 900, perPage: 2}
	f.patients = []map[string]any{
		{"id": "201", "first_name": "Ava", "last_name": "Ng", "archived_at": nil, "updated_at": "2026-11-01T00:00:00Z",
			"patient_phone_numbers": []any{map[string]any{"number": "0412 345 678", "phone_type": "Mobile"}}},
		{"id": "202", "first_name": "Ben", "last_name": "Ode", "archived_at": nil, "updated_at": "2026-11-02T00:00:00Z",
			"patient_phone_numbers": []any{map[string]any{"number": "+61 400 111 222", "phone_type": "Mobile"}}},
		{"id": "203", "first_name": "Cy", "last_name": "Old", "archived_at": "2026-01-01T00:00:00Z", "updated_at": "2026-11-03T00:00:00Z",
			"patient_phone_numbers": []any{map[string]any{"number": "0400 999 999", "phone_type": "Mobile"}}},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCliniko) base() string { return f.srv.URL + "/v1" }

func (f *fakeCliniko) client(t *testing.T) *Client {
	c, err := NewClient(fakeKey, "dev@example.test", f.srv.Client(), f.base())
	if err != nil {
		t.Fatal(err)
	}
	c.sleep = func(time.Duration) {}
	return c
}

func (f *fakeCliniko) self(kind, id string) map[string]any {
	return map[string]any{"links": map[string]any{"self": f.base() + "/" + kind + "/" + id}}
}

func (f *fakeCliniko) json(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// paged answers a list, perPage at a time, with links.next.
func (f *fakeCliniko) paged(w http.ResponseWriter, r *http.Request, key string, items []any) {
	pageN, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if pageN < 1 {
		pageN = 1
	}
	from := (pageN - 1) * f.perPage
	to := from + f.perPage
	if from > len(items) {
		from = len(items)
	}
	if to > len(items) {
		to = len(items)
	}
	links := map[string]any{"self": r.URL.String()}
	if to < len(items) {
		q := r.URL.Query()
		q.Set("page", strconv.Itoa(pageN+1))
		links["next"] = f.base() + strings.TrimPrefix(r.URL.Path, "/v1") + "?" + q.Encode()
	}
	f.json(w, 200, map[string]any{key: items[from:to], "total_entries": len(items), "links": links})
}

func (f *fakeCliniko) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Clone(context.Background()))
	var body map[string]any
	if b, _ := io.ReadAll(r.Body); len(b) > 0 {
		_ = json.Unmarshal(b, &body)
	}
	f.bodies = append(f.bodies, body)
	if r.Header.Get("Authorization") == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
		f.json(w, 401, map[string]any{"message": "Unauthorized"})
		return
	}
	if f.status429 > 0 {
		f.status429--
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(f.resetIn).Unix(), 10))
		f.json(w, 429, map[string]any{"message": "Too many requests"})
		return
	}
	if f.fail500 {
		f.json(w, 500, map[string]any{"message": "boom"})
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/v1")
	switch {
	case p == "/user":
		f.json(w, 200, map[string]any{"first_name": "Weline", "last_name": "Integration"})
	case p == "/businesses":
		f.paged(w, r, "businesses", []any{
			map[string]any{"id": "11", "business_name": "Harbour Physio Pty Ltd", "display_name": "Harbour St", "time_zone_identifier": "Australia/Sydney", "archived_at": nil},
			map[string]any{"id": "12", "business_name": "Old Site", "time_zone_identifier": "Australia/Sydney", "archived_at": "2025-01-01T00:00:00Z"},
			map[string]any{"id": "13", "business_name": "Perth", "display_name": "Perth CBD", "time_zone_identifier": "Australia/Perth", "archived_at": nil},
		})
	case p == "/businesses/11/practitioners":
		f.paged(w, r, "practitioners", []any{
			map[string]any{"id": "31", "display_name": "Sam Lee", "active": true, "show_in_online_bookings": true},
			map[string]any{"id": "32", "first_name": "Jo", "last_name": "Wu", "active": true, "show_in_online_bookings": true},
			map[string]any{"id": "33", "display_name": "Hidden Hal", "active": true, "show_in_online_bookings": false},
		})
	case p == "/businesses/13/practitioners":
		f.paged(w, r, "practitioners", []any{map[string]any{"id": "31", "display_name": "Sam Lee", "active": true, "show_in_online_bookings": true}})
	case p == "/practitioners":
		f.paged(w, r, "practitioners", []any{
			map[string]any{"id": "31", "display_name": "Sam Lee"}, map[string]any{"id": "32", "first_name": "Jo", "last_name": "Wu"},
		})
	case p == "/appointment_types":
		f.paged(w, r, "appointment_types", []any{
			map[string]any{"id": "21", "name": "Initial physio", "duration_in_minutes": 45, "max_attendees": 1, "show_in_online_bookings": true},
			map[string]any{"id": "22", "name": "Pilates class", "duration_in_minutes": 60, "max_attendees": 8, "show_in_online_bookings": true},
			map[string]any{"id": "23", "name": "Paid assessment", "duration_in_minutes": 30, "max_attendees": 1, "online_payments_enabled": true, "show_in_online_bookings": true},
			map[string]any{"id": "24", "name": "Retired", "duration_in_minutes": 30, "max_attendees": 1, "archived_at": "2025-01-01T00:00:00Z", "show_in_online_bookings": true},
			map[string]any{"id": "25", "name": "Staff only", "duration_in_minutes": 30, "max_attendees": 1, "show_in_online_bookings": false},
		})
	case strings.HasSuffix(p, "/available_times"):
		parts := strings.Split(p, "/") // /businesses/11/practitioners/31/appointment_types/21/available_times
		pr := parts[4]
		from, _ := time.Parse("2006-01-02", r.URL.Query().Get("from"))
		var items []any
		for d := 0; d < 7; d++ {
			day := from.AddDate(0, 0, d)
			// Sam (31) at 09:00 and 10:00 Sydney, Jo (32) at 09:00 and 11:00 (UTC+11 in November).
			hours := map[string][]int{"31": {9, 10}, "32": {9, 11}}[pr]
			for _, h := range hours {
				t := time.Date(day.Year(), day.Month(), day.Day(), h-11, 0, 0, 0, time.UTC)
				items = append(items, map[string]any{"appointment_start": t.Format(time.RFC3339)})
			}
		}
		f.paged(w, r, "available_times", items)
	case p == "/patients" && r.Method == http.MethodGet:
		var items []any
		qs := r.URL.Query()["q[]"]
		for _, pt := range f.patients {
			ok := true
			for _, q := range qs {
				switch {
				case strings.HasPrefix(q, "first_name:="):
					ok = ok && pt["first_name"] == strings.TrimPrefix(q, "first_name:=")
				case strings.HasPrefix(q, "last_name:="):
					ok = ok && pt["last_name"] == strings.TrimPrefix(q, "last_name:=")
				case strings.HasPrefix(q, "updated_at:>"):
					since, _ := time.Parse(time.RFC3339, strings.TrimPrefix(q, "updated_at:>"))
					up, _ := time.Parse(time.RFC3339, pt["updated_at"].(string))
					ok = ok && up.After(since)
				}
			}
			if ok {
				items = append(items, pt)
			}
		}
		f.paged(w, r, "patients", items)
	case p == "/patients" && r.Method == http.MethodPost:
		f.nextID++
		pt := map[string]any{"id": strconv.Itoa(f.nextID), "first_name": body["first_name"], "last_name": body["last_name"],
			"archived_at": nil, "updated_at": time.Now().UTC().Format(time.RFC3339), "patient_phone_numbers": body["patient_phone_numbers"]}
		f.patients = append(f.patients, pt)
		f.json(w, 201, pt)
	case strings.HasPrefix(p, "/patients/"):
		id := strings.TrimPrefix(p, "/patients/")
		for _, pt := range f.patients {
			if pt["id"] == id {
				f.json(w, 200, pt)
				return
			}
		}
		f.json(w, 404, map[string]any{"message": "Not found"})
	case p == "/individual_appointments" && r.Method == http.MethodPost:
		if f.bookStatus != 0 {
			f.json(w, f.bookStatus, map[string]any{"message": f.bookMsg})
			return
		}
		f.nextID++
		id := strconv.Itoa(f.nextID)
		start, _ := time.Parse(time.RFC3339, body["starts_at"].(string))
		a := map[string]any{"id": id, "starts_at": start.Format(time.RFC3339), "ends_at": start.Add(45 * time.Minute).Format(time.RFC3339),
			"cancelled_at": nil, "updated_at": time.Now().UTC().Format(time.RFC3339),
			"patient": f.self("patients", fmt.Sprint(body["patient_id"])), "practitioner": f.self("practitioners", fmt.Sprint(body["practitioner_id"])),
			"appointment_type": f.self("appointment_types", fmt.Sprint(body["appointment_type_id"])), "business": f.self("businesses", fmt.Sprint(body["business_id"]))}
		f.appointments[id] = a
		f.json(w, 201, a)
	case p == "/individual_appointments" && r.Method == http.MethodGet:
		var items []any
		for _, a := range f.appointments {
			items = append(items, a)
		}
		f.paged(w, r, "individual_appointments", items)
	case strings.HasSuffix(p, "/cancel") && r.Method == http.MethodPatch:
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/individual_appointments/"), "/cancel")
		a, ok := f.appointments[id]
		if !ok {
			f.json(w, 404, map[string]any{"message": "Not found"})
			return
		}
		a["cancelled_at"] = time.Now().UTC().Format(time.RFC3339)
		w.WriteHeader(204)
	case strings.HasPrefix(p, "/individual_appointments/"):
		id := strings.TrimPrefix(p, "/individual_appointments/")
		a, ok := f.appointments[id]
		if !ok {
			f.json(w, 404, map[string]any{"message": "Not found"})
			return
		}
		if r.Method == http.MethodPatch {
			if s, ok := body["starts_at"].(string); ok {
				t, _ := time.Parse(time.RFC3339, s)
				a["starts_at"], a["ends_at"] = t.Format(time.RFC3339), t.Add(45*time.Minute).Format(time.RFC3339)
			}
		}
		f.json(w, 200, a)
	default:
		f.json(w, 404, map[string]any{"message": "Not found"})
	}
}

func (f *fakeCliniko) count(method, pathPart string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r.Method == method && strings.Contains(r.URL.Path, pathPart) {
			n++
		}
	}
	return n
}

func (f *fakeCliniko) lastBody(method, pathPart string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Method == method && strings.Contains(f.requests[i].URL.Path, pathPart) {
			return f.bodies[i]
		}
	}
	return nil
}

// memLinks is an in-memory Links for unit tests.
type memLinks struct {
	mu     sync.Mutex
	remote map[uuid.UUID]string
	phones map[string][]string
}

func newLinks() *memLinks {
	return &memLinks{remote: map[uuid.UUID]string{}, phones: map[string][]string{}}
}

func (l *memLinks) Set(_ context.Context, _ string, local uuid.UUID, remote string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.remote[local] = remote
	return nil
}
func (l *memLinks) Remote(_ context.Context, _ string, local uuid.UUID) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.remote[local], nil
}
func (l *memLinks) SetPhone(_ context.Context, phone, remote string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.phones[phone] {
		if r == remote {
			return nil
		}
	}
	l.phones[phone] = append(l.phones[phone], remote)
	return nil
}
func (l *memLinks) ByPhone(_ context.Context, phone string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.phones[phone], nil
}
