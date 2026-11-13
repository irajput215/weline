package cliniko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
)

// Links is the part of Connectors' link store the provider uses
// (connectors.Links satisfies it); declared here so this package doesn't
// import Connectors, which imports it.
type Links interface {
	Set(ctx context.Context, entity string, localID uuid.UUID, remoteID string) error
	Remote(ctx context.Context, entity string, localID uuid.UUID) (string, error)
	SetPhone(ctx context.Context, phone, remoteID string) error
	ByPhone(ctx context.Context, phone string) ([]string, error)
}

// Settings are the clinic's options for the Cliniko provider.
type Settings struct {
	WindowDays     int   `json:"window_days"`     // how far ahead patients can book, 1-60 (default 14)
	ShowAll        bool  `json:"show_all"`        // offer practitioners and services hidden from online bookings
	CreatePatients *bool `json:"create_patients"` // create a Cliniko patient for new people (default true)
}

// ParseSettings reads and checks settings, filling defaults.
func ParseSettings(raw json.RawMessage) (Settings, error) {
	var s Settings
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &s); err != nil {
			return s, fmt.Errorf("settings: %w", provider.ErrInvalid)
		}
	}
	if s.WindowDays == 0 {
		s.WindowDays = 14
	}
	if s.WindowDays < 1 || s.WindowDays > 60 {
		return s, fmt.Errorf("settings: window_days must be 1 to 60: %w", provider.ErrInvalid)
	}
	if s.CreatePatients == nil {
		t := true
		s.CreatePatients = &t
	}
	return s, nil
}

// Provider is Cliniko for one organization.
type Provider struct {
	c        *Client
	links    Links
	settings Settings
	cache    *timesCache
	now      func() time.Time
}

var _ provider.Provider = (*Provider)(nil)

// New returns the Cliniko provider for one organization.
func New(c *Client, links Links, settings Settings, cache *timesCache) *Provider {
	if cache == nil {
		cache = newTimesCache()
	}
	return &Provider{c: c, links: links, settings: settings, cache: cache, now: time.Now}
}

// Name implements provider.Provider.
func (p *Provider) Name() string { return provider.Cliniko }

// ---------- Cliniko records ----------

type link struct {
	Links struct {
		Self string `json:"self"`
	} `json:"links"`
}

// id is the last path segment of a links.self URL ("…/practitioners/12" → "12").
func (l link) id() string {
	s := strings.TrimSuffix(l.Links.Self, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return ""
}

type business struct {
	ID           string  `json:"id"`
	BusinessName string  `json:"business_name"`
	DisplayName  string  `json:"display_name"`
	TimeZone     string  `json:"time_zone_identifier"`
	ArchivedAt   *string `json:"archived_at"`
}

func (b business) name() string {
	if b.DisplayName != "" {
		return b.DisplayName
	}
	return b.BusinessName
}

type practitioner struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Active      bool   `json:"active"`
	Online      bool   `json:"show_in_online_bookings"`
}

func (p practitioner) name() string {
	if p.DisplayName != "" {
		return p.DisplayName
	}
	return strings.TrimSpace(p.FirstName + " " + p.LastName)
}

type appointmentType struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Minutes        int     `json:"duration_in_minutes"`
	MaxAttendees   int     `json:"max_attendees"`
	OnlinePayments bool    `json:"online_payments_enabled"`
	Online         bool    `json:"show_in_online_bookings"`
	ArchivedAt     *string `json:"archived_at"`
	LeadTimeHours  int     `json:"online_bookings_lead_time_hours"`
}

type appointment struct {
	ID              string     `json:"id"`
	StartsAt        time.Time  `json:"starts_at"`
	EndsAt          time.Time  `json:"ends_at"`
	CancelledAt     *time.Time `json:"cancelled_at"`
	DidNotArrive    bool       `json:"did_not_arrive"`
	PatientArrived  bool       `json:"patient_arrived"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Patient         link       `json:"patient"`
	Practitioner    link       `json:"practitioner"`
	AppointmentType link       `json:"appointment_type"`
	Business        link       `json:"business"`
}

// ---------- catalog ----------

func (p *Provider) businesses(ctx context.Context) ([]business, error) {
	var out []business
	err := p.c.list(ctx, "/businesses", nil, "businesses", func(raw json.RawMessage) error {
		var b business
		if err := json.Unmarshal(raw, &b); err != nil {
			return err
		}
		if b.ArchivedAt == nil {
			out = append(out, b)
		}
		return nil
	})
	return out, err
}

// ListLocations implements provider.Provider (businesses not archived).
func (p *Provider) ListLocations(ctx context.Context) ([]provider.Location, error) {
	bs, err := p.businesses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Location, 0, len(bs))
	for _, b := range bs {
		out = append(out, provider.Location{ID: b.ID, Name: b.name(), TimeZone: b.TimeZone})
	}
	return out, nil
}

func (p *Provider) practitionersAt(ctx context.Context, businessID string) ([]practitioner, error) {
	var out []practitioner
	err := p.c.list(ctx, "/businesses/"+url.PathEscape(businessID)+"/practitioners", nil, "practitioners", func(raw json.RawMessage) error {
		var pr practitioner
		if err := json.Unmarshal(raw, &pr); err != nil {
			return err
		}
		if pr.Active && (pr.Online || p.settings.ShowAll) {
			out = append(out, pr)
		}
		return nil
	})
	return out, err
}

// ListPractitioners implements provider.Provider: active practitioners shown
// in online bookings (all with show_all), with the businesses they work at.
func (p *Provider) ListPractitioners(ctx context.Context, locationID string) ([]provider.Practitioner, error) {
	bs, err := p.businesses(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]*provider.Practitioner{}
	var order []string
	for _, b := range bs {
		prs, err := p.practitionersAt(ctx, b.ID)
		if err != nil {
			return nil, err
		}
		for _, pr := range prs {
			if byID[pr.ID] == nil {
				byID[pr.ID] = &provider.Practitioner{ID: pr.ID, Name: pr.name(), LocationIDs: []string{}}
				order = append(order, pr.ID)
			}
			byID[pr.ID].LocationIDs = append(byID[pr.ID].LocationIDs, b.ID)
		}
	}
	out := []provider.Practitioner{}
	for _, id := range order {
		pr := byID[id]
		if locationID == "" || contains(pr.LocationIDs, locationID) {
			out = append(out, *pr)
		}
	}
	return out, nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (p *Provider) types(ctx context.Context) ([]appointmentType, error) {
	var out []appointmentType
	err := p.c.list(ctx, "/appointment_types", nil, "appointment_types", func(raw json.RawMessage) error {
		var t appointmentType
		if err := json.Unmarshal(raw, &t); err != nil {
			return err
		}
		// Group and paid types aren't bookable on WhatsApp (spike, "Services").
		if t.ArchivedAt == nil && t.MaxAttendees <= 1 && !t.OnlinePayments && (t.Online || p.settings.ShowAll) {
			out = append(out, t)
		}
		return nil
	})
	return out, err
}

// ListServices implements provider.Provider.
func (p *Provider) ListServices(ctx context.Context) ([]provider.Service, error) {
	ts, err := p.types(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Service, 0, len(ts))
	for _, t := range ts {
		out = append(out, provider.Service{ID: t.ID, Name: t.Name, Minutes: t.Minutes})
	}
	return out, nil
}

// ---------- free times ----------

const windowDays = 7 // Cliniko answers at most 7 days per call

type timesKey struct{ business, practitioner, typ, from string }

type timesEntry struct {
	at    time.Time
	times []time.Time
}

// timesCache keeps free times for 60 seconds (C7).
// Cache keeps free times and names for a minute; one per organization,
// kept across requests by Connectors.
type Cache = timesCache

// NewCache returns an empty cache.
func NewCache() *Cache { return newTimesCache() }

type timesCache struct {
	mu    sync.Mutex
	m     map[timesKey]timesEntry
	names *nameSet
}

func newTimesCache() *timesCache { return &timesCache{m: map[timesKey]timesEntry{}} }

const timesTTL = time.Minute

func (p *Provider) availableTimes(ctx context.Context, businessID, practitionerID, typeID string, from, to time.Time) ([]time.Time, error) {
	k := timesKey{businessID, practitionerID, typeID, from.Format("2006-01-02") + "/" + to.Format("2006-01-02")}
	p.cache.mu.Lock()
	if e, ok := p.cache.m[k]; ok && p.now().Sub(e.at) < timesTTL {
		p.cache.mu.Unlock()
		return e.times, nil
	}
	p.cache.mu.Unlock()
	var out []time.Time
	path := fmt.Sprintf("/businesses/%s/practitioners/%s/appointment_types/%s/available_times",
		url.PathEscape(businessID), url.PathEscape(practitionerID), url.PathEscape(typeID))
	q := url.Values{"from": {from.Format("2006-01-02")}, "to": {to.Format("2006-01-02")}}
	err := p.c.list(ctx, path, q, "available_times", func(raw json.RawMessage) error {
		var t struct {
			Start time.Time `json:"appointment_start"`
		}
		if err := json.Unmarshal(raw, &t); err != nil {
			return err
		}
		out = append(out, t.Start.UTC())
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.cache.mu.Lock()
	p.cache.m[k] = timesEntry{at: p.now(), times: out}
	p.cache.mu.Unlock()
	return out, nil
}

// FindFreeTimes implements provider.Provider: Cliniko's available times per
// practitioner in 7-day windows up to the clinic's window, merged and sorted,
// with the first practitioner free at each start (C3).
func (p *Provider) FindFreeTimes(ctx context.Context, q provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	types, err := p.types(ctx)
	if err != nil {
		return nil, err
	}
	var typ *appointmentType
	for i := range types {
		if types[i].ID == q.ServiceID {
			typ = &types[i]
		}
	}
	if typ == nil {
		return nil, provider.ErrNotFound
	}
	locs, err := p.ListLocations(ctx)
	if err != nil {
		return nil, err
	}
	tz := time.UTC
	found := false
	for _, l := range locs {
		if l.ID == q.LocationID {
			found = true
			if z, err := time.LoadLocation(l.TimeZone); err == nil {
				tz = z
			}
		}
	}
	if !found {
		return nil, provider.ErrNotFound
	}
	prs, err := p.practitionersAt(ctx, q.LocationID)
	if err != nil {
		return nil, err
	}
	if q.PractitionerID != "" {
		var only []practitioner
		for _, pr := range prs {
			if pr.ID == q.PractitionerID {
				only = append(only, pr)
			}
		}
		prs = only
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	now := p.now()
	earliest := now
	if q.Patient && typ.LeadTimeHours > 0 {
		earliest = now.Add(time.Duration(typ.LeadTimeHours) * time.Hour)
	}
	start := q.From
	if start.IsZero() || start.Before(now) {
		start = now
	}
	last := now.AddDate(0, 0, p.settings.WindowDays)
	if !q.To.IsZero() && q.To.Before(last) {
		last = q.To
	}
	day := func(t time.Time) time.Time {
		l := t.In(tz)
		return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, tz)
	}
	out := &provider.FreeTimes{WindowDays: p.settings.WindowDays, Times: []provider.FreeTime{}}
	for from := day(start); !from.After(day(last)) && len(out.Times) < limit; from = from.AddDate(0, 0, windowDays) {
		to := from.AddDate(0, 0, windowDays-1)
		if to.After(day(last)) {
			to = day(last)
		}
		first := map[time.Time]practitioner{}
		for _, pr := range prs {
			times, err := p.availableTimes(ctx, q.LocationID, pr.ID, typ.ID, from, to)
			if err != nil {
				return nil, err
			}
			for _, t := range times {
				if _, ok := first[t]; !ok {
					first[t] = pr
				}
			}
		}
		starts := make([]time.Time, 0, len(first))
		for t := range first {
			if !t.Before(earliest) && !t.Before(start) && !t.After(last) {
				starts = append(starts, t)
			}
		}
		sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
		for _, t := range starts {
			if len(out.Times) == limit {
				break
			}
			pr := first[t]
			out.Times = append(out.Times, provider.FreeTime{Start: t, End: t.Add(time.Duration(typ.Minutes) * time.Minute),
				PractitionerID: pr.ID, PractitionerName: pr.name(), LocationID: q.LocationID})
		}
	}
	return out, nil
}

// ---------- appointments ----------

// Book implements provider.Provider and links the appointment to the contact.
func (p *Provider) Book(ctx context.Context, in provider.BookInput) (*provider.Appointment, error) {
	body := map[string]any{"business_id": in.LocationID, "practitioner_id": in.PractitionerID, "appointment_type_id": in.ServiceID,
		"patient_id": in.CustomerID, "starts_at": in.Start.UTC().Format(time.RFC3339)}
	if in.Note != "" {
		body["notes"] = in.Note
	}
	var a appointment
	if err := p.c.do(ctx, http.MethodPost, "/individual_appointments", nil, body, &a); err != nil {
		if isConflict(err) {
			return nil, fmt.Errorf("%w: %w", err, provider.ErrSlotTaken)
		}
		return nil, err
	}
	p.forgetTimes()
	if in.ContactID != uuid.Nil && p.links != nil {
		if err := p.links.Set(ctx, "contact", in.ContactID, in.CustomerID); err != nil {
			return nil, err
		}
	}
	return p.view(ctx, a)
}

// forgetTimes drops cached free times after a change.
func (p *Provider) forgetTimes() {
	p.cache.mu.Lock()
	p.cache.m = map[timesKey]timesEntry{}
	p.cache.mu.Unlock()
}

// Reschedule implements provider.Provider.
func (p *Provider) Reschedule(ctx context.Context, id string, in provider.RescheduleInput) (*provider.Appointment, error) {
	body := map[string]any{}
	if !in.Start.IsZero() {
		body["starts_at"] = in.Start.UTC().Format(time.RFC3339)
		body["ends_at"] = nil // Cliniko recomputes it from the type
	}
	if in.PractitionerID != "" {
		body["practitioner_id"] = in.PractitionerID
	}
	if in.LocationID != "" {
		body["business_id"] = in.LocationID
	}
	var a appointment
	if err := p.c.do(ctx, http.MethodPatch, "/individual_appointments/"+url.PathEscape(id), nil, body, &a); err != nil {
		if isConflict(err) {
			return nil, fmt.Errorf("%w: %w", err, provider.ErrSlotTaken)
		}
		return nil, err
	}
	p.forgetTimes()
	return p.view(ctx, a)
}

// Cancel implements provider.Provider (reason 50, "other", with the note).
func (p *Provider) Cancel(ctx context.Context, id string, reason string) error {
	note := strings.TrimSpace(reason)
	if note == "" {
		note = "Cancelled by the patient on WhatsApp"
	}
	err := p.c.do(ctx, http.MethodPatch, "/individual_appointments/"+url.PathEscape(id)+"/cancel", nil,
		map[string]any{"cancellation_reason": 50, "cancellation_note": note}, nil)
	if err == nil {
		p.forgetTimes()
	}
	return err
}

// GetAppointment implements provider.Provider.
func (p *Provider) GetAppointment(ctx context.Context, id string) (*provider.Appointment, error) {
	var a appointment
	if err := p.c.do(ctx, http.MethodGet, "/individual_appointments/"+url.PathEscape(id), nil, nil, &a); err != nil {
		return nil, err
	}
	return p.view(ctx, a)
}

// ListAppointments implements provider.Provider.
func (p *Provider) ListAppointments(ctx context.Context, q provider.AppointmentQuery) ([]provider.Appointment, error) {
	v := url.Values{}
	switch {
	case q.ChangedSince != nil:
		v.Add("q[]", "updated_at:>"+q.ChangedSince.UTC().Format(time.RFC3339))
		v.Set("sort", "updated_at")
	case q.From != nil && q.To != nil:
		v.Add("q[]", "starts_at:>="+q.From.UTC().Format(time.RFC3339))
		v.Add("q[]", "starts_at:<"+q.To.UTC().Format(time.RFC3339))
		v.Set("sort", "starts_at")
	default:
		return nil, fmt.Errorf("give changed since, or from and to: %w", provider.ErrInvalid)
	}
	if q.CustomerID != "" {
		v.Add("q[]", "patient_id:="+q.CustomerID)
	}
	var raw []appointment
	err := p.c.list(ctx, "/individual_appointments", v, "individual_appointments", func(r json.RawMessage) error {
		var a appointment
		if err := json.Unmarshal(r, &a); err != nil {
			return err
		}
		raw = append(raw, a)
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]provider.Appointment, 0, len(raw))
	for _, a := range raw {
		v, err := p.view(ctx, a)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// view names an appointment's practitioner, service and location, from one
// lookup of each list per call.
func (p *Provider) view(ctx context.Context, a appointment) (*provider.Appointment, error) {
	status := provider.StatusBooked
	switch {
	case a.CancelledAt != nil:
		status = provider.StatusCancelled
	case a.DidNotArrive:
		status = provider.StatusNoShow
	case a.PatientArrived && a.EndsAt.Before(p.now()):
		status = provider.StatusCompleted
	}
	out := &provider.Appointment{ID: a.ID, Reference: a.ID, Status: status, Start: a.StartsAt.UTC(), End: a.EndsAt.UTC(),
		ServiceID: a.AppointmentType.id(), PractitionerID: a.Practitioner.id(), LocationID: a.Business.id(),
		CustomerID: a.Patient.id(), UpdatedAt: a.UpdatedAt.UTC()}
	names, err := p.names(ctx)
	if err != nil {
		return nil, err
	}
	out.ServiceName = names.types[out.ServiceID]
	out.PractitionerName = names.practitioners[out.PractitionerID]
	if b, ok := names.businesses[out.LocationID]; ok {
		out.LocationName, out.TimeZone = b.name(), b.TimeZone
	}
	return out, nil
}

type nameSet struct {
	types, practitioners map[string]string
	businesses           map[string]business
	at                   time.Time
}

// names caches the names of types, practitioners and businesses for a minute.
func (p *Provider) names(ctx context.Context) (*nameSet, error) {
	p.cache.mu.Lock()
	cached := p.cache.names
	p.cache.mu.Unlock()
	if cached != nil && p.now().Sub(cached.at) < timesTTL {
		return cached, nil
	}
	n := &nameSet{types: map[string]string{}, practitioners: map[string]string{}, businesses: map[string]business{}, at: p.now()}
	if err := p.c.list(ctx, "/appointment_types", nil, "appointment_types", func(raw json.RawMessage) error {
		var t appointmentType
		if err := json.Unmarshal(raw, &t); err != nil {
			return err
		}
		n.types[t.ID] = t.Name
		return nil
	}); err != nil {
		return nil, err
	}
	if err := p.c.list(ctx, "/practitioners", nil, "practitioners", func(raw json.RawMessage) error {
		var pr practitioner
		if err := json.Unmarshal(raw, &pr); err != nil {
			return err
		}
		n.practitioners[pr.ID] = pr.name()
		return nil
	}); err != nil {
		return nil, err
	}
	bs, err := p.businesses(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range bs {
		n.businesses[b.ID] = b
	}
	p.cache.mu.Lock()
	p.cache.names = n
	p.cache.mu.Unlock()
	return n, nil
}
