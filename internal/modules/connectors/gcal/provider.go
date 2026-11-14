package gcal

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Name is the provider name.
const Name = "google_calendar"

// Settings are an organization's calendars: one for everyone, and optionally
// one per practitioner (Weline practitioner id → calendar id).
type Settings struct {
	CalendarID            string            `json:"-"` // from the calendar_id field
	PractitionerCalendars map[string]string `json:"practitioner_calendars,omitempty"`
}

// ParseSettings reads the settings JSON with the calendar id field.
func ParseSettings(calendarID string, raw json.RawMessage) (Settings, error) {
	var s Settings
	if len(raw) > 0 && string(raw) != "null" {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			return s, kit.Invalid("Google Calendar settings: %s", "practitioner_calendars must map practitioner ids to calendar ids")
		}
	}
	s.CalendarID = strings.TrimSpace(calendarID)
	if s.CalendarID == "" || len(s.CalendarID) > 255 || strings.ContainsAny(s.CalendarID, " /?#") {
		return s, kit.Invalid("enter the calendar id (Google Calendar → Settings → Integrate calendar → Calendar ID)")
	}
	for p, c := range s.PractitionerCalendars {
		if p == "" || strings.TrimSpace(c) == "" || strings.ContainsAny(c, " /?#") {
			return s, kit.Invalid("practitioner_calendars: each entry needs a practitioner id and a calendar id")
		}
	}
	return s, nil
}

func (s Settings) calendarFor(practitionerID string) string {
	if c := s.PractitionerCalendars[practitionerID]; c != "" {
		return c
	}
	return s.CalendarID
}

// Calendars are every calendar the settings use.
func (s Settings) Calendars() []string { return s.calendars() }

func (s Settings) calendars() []string {
	seen := map[string]bool{s.CalendarID: true}
	out := []string{s.CalendarID}
	for _, c := range s.PractitionerCalendars {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out[1:])
	return out
}

// Provider is Google Calendar for one organization. The catalog (services,
// locations, practitioners, opening hours) and customers (Weline contacts)
// come from Weline's own diary (base); the calendar holds busy times and
// the bookings.
type Provider struct {
	c    *Client
	base provider.Provider
	set  Settings
	now  func() time.Time
}

// New builds the provider.
func New(c *Client, base provider.Provider, set Settings) *Provider {
	return &Provider{c: c, base: base, set: set, now: time.Now}
}

// TestConnection checks every configured calendar is visible.
func (p *Provider) TestConnection(ctx context.Context) (string, error) {
	names := []string{}
	for _, id := range p.set.calendars() {
		cal, err := p.c.GetCalendar(ctx, id)
		if err != nil {
			return "", err
		}
		names = append(names, cal.Summary)
	}
	return fmt.Sprintf("Connected to Google Calendar: %s", strings.Join(names, ", ")), nil
}

func (p *Provider) Name() string { return Name }
func (p *Provider) ListLocations(ctx context.Context) ([]provider.Location, error) {
	return p.base.ListLocations(ctx)
}
func (p *Provider) ListPractitioners(ctx context.Context, loc string) ([]provider.Practitioner, error) {
	return p.base.ListPractitioners(ctx, loc)
}
func (p *Provider) ListServices(ctx context.Context) ([]provider.Service, error) {
	return p.base.ListServices(ctx)
}
func (p *Provider) FindOrCreateCustomer(ctx context.Context, in provider.CustomerInput) (*provider.Customer, bool, error) {
	return p.base.FindOrCreateCustomer(ctx, in)
}
func (p *Provider) GetCustomer(ctx context.Context, id string) (*provider.Customer, error) {
	return p.base.GetCustomer(ctx, id)
}

func overlaps(busy []Interval, start, end time.Time) bool {
	for _, b := range busy {
		if start.Before(b.End) && end.After(b.Start) {
			return true
		}
	}
	return false
}

// FindFreeTimes takes Weline's candidate times (opening hours, staff,
// service length) and drops those busy in the practitioner's calendar.
func (p *Provider) FindFreeTimes(ctx context.Context, q provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	want := q.Limit
	if want <= 0 {
		want = 20
	}
	q.Limit = 200 // ask for more: some will be busy in the calendar
	cand, err := p.base.FindFreeTimes(ctx, q)
	if err != nil || len(cand.Times) == 0 {
		return cand, err
	}
	min, max := cand.Times[0].Start, cand.Times[0].End
	for _, t := range cand.Times {
		if t.Start.Before(min) {
			min = t.Start
		}
		if t.End.After(max) {
			max = t.End
		}
	}
	busy, err := p.c.FreeBusy(ctx, min, max, p.set.calendars())
	if err != nil {
		return nil, err
	}
	out := &provider.FreeTimes{WindowDays: cand.WindowDays}
	for _, t := range cand.Times {
		if overlaps(busy[p.set.calendarFor(t.PractitionerID)], t.Start, t.End) {
			continue
		}
		out.Times = append(out.Times, t)
		if len(out.Times) == want {
			break
		}
	}
	return out, nil
}

// Appointment ids are "<event id>@<calendar id>" (event ids never hold "@").
func splitID(id string) (event, calendar string, err error) {
	event, calendar, ok := strings.Cut(id, "@")
	if !ok || event == "" || calendar == "" {
		return "", "", provider.ErrNotFound
	}
	return event, calendar, nil
}

type catalog struct {
	services      map[string]provider.Service
	locations     map[string]provider.Location
	practitioners map[string]provider.Practitioner
}

func (p *Provider) catalog(ctx context.Context) (*catalog, error) {
	c := &catalog{services: map[string]provider.Service{}, locations: map[string]provider.Location{}, practitioners: map[string]provider.Practitioner{}}
	svcs, err := p.base.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range svcs {
		c.services[s.ID] = s
	}
	locs, err := p.base.ListLocations(ctx)
	if err != nil {
		return nil, err
	}
	for _, l := range locs {
		c.locations[l.ID] = l
	}
	prs, err := p.base.ListPractitioners(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, pr := range prs {
		c.practitioners[pr.ID] = pr
	}
	return c, nil
}

func reference(eventID string) string {
	r := strings.ToUpper(eventID)
	if len(r) > 8 {
		r = r[:8]
	}
	return r
}

// view turns an event into an appointment.
func (p *Provider) view(cat *catalog, calendar string, e *Event) provider.Appointment {
	a := provider.Appointment{ID: e.ID + "@" + calendar, Reference: reference(e.ID), Status: provider.StatusBooked,
		ServiceID: e.Private("weline_service"), PractitionerID: e.Private("weline_practitioner"),
		LocationID: e.Private("weline_location"), CustomerID: e.Private("weline_contact"), TimeZone: e.Start.TimeZone}
	a.Start, _ = time.Parse(time.RFC3339, e.Start.DateTime)
	a.End, _ = time.Parse(time.RFC3339, e.End.DateTime)
	a.UpdatedAt, _ = time.Parse(time.RFC3339, e.Updated)
	switch {
	case e.Status == "cancelled":
		a.Status = provider.StatusCancelled
	case !a.End.IsZero() && a.End.Before(p.now()):
		a.Status = provider.StatusCompleted
	}
	if cat != nil {
		a.ServiceName = cat.services[a.ServiceID].Name
		a.PractitionerName = cat.practitioners[a.PractitionerID].Name
		l := cat.locations[a.LocationID]
		a.LocationName = l.Name
		if a.TimeZone == "" {
			a.TimeZone = l.TimeZone
		}
	}
	return a
}

// Book checks the time is still free in the calendar and creates the event.
func (p *Provider) Book(ctx context.Context, in provider.BookInput) (*provider.Appointment, error) {
	cat, err := p.catalog(ctx)
	if err != nil {
		return nil, err
	}
	svc, ok := cat.services[in.ServiceID]
	if !ok || svc.Minutes <= 0 {
		return nil, kit.Invalid("choose a service")
	}
	loc, ok := cat.locations[in.LocationID]
	if !ok {
		return nil, kit.Invalid("choose a location")
	}
	if in.CustomerID == "" {
		return nil, kit.Invalid("who is the appointment for?")
	}
	end := in.Start.Add(time.Duration(svc.Minutes) * time.Minute)
	cal := p.set.calendarFor(in.PractitionerID)
	busy, err := p.c.FreeBusy(ctx, in.Start, end, []string{cal})
	if err != nil {
		return nil, err
	}
	if overlaps(busy[cal], in.Start, end) {
		return nil, provider.ErrSlotTaken
	}
	customer := ""
	if c, err := p.base.GetCustomer(ctx, in.CustomerID); err == nil {
		customer = c.Name
	}
	tz := loc.TimeZone
	summary := svc.Name
	if customer != "" {
		summary += " – " + customer
	}
	ev := &Event{Summary: summary, Description: "Booked on WhatsApp through Weline.",
		Start: EventTime{DateTime: in.Start.Format(time.RFC3339), TimeZone: tz}, End: EventTime{DateTime: end.Format(time.RFC3339), TimeZone: tz}}
	ev.ExtendedProperties = &struct {
		Private map[string]string `json:"private,omitempty"`
	}{Private: map[string]string{"weline": "1", "weline_contact": in.CustomerID, "weline_service": in.ServiceID,
		"weline_location": in.LocationID, "weline_practitioner": in.PractitionerID, "weline_source": in.Source}}
	got, err := p.c.InsertEvent(ctx, cal, ev)
	if err != nil {
		return nil, err
	}
	a := p.view(cat, cal, got)
	return &a, nil
}

// Reschedule moves an event to a new start (same length, same calendar).
func (p *Provider) Reschedule(ctx context.Context, id string, in provider.RescheduleInput) (*provider.Appointment, error) {
	eventID, cal, err := splitID(id)
	if err != nil {
		return nil, err
	}
	cur, err := p.c.GetEvent(ctx, cal, eventID)
	if err != nil {
		return nil, err
	}
	if cur.Status == "cancelled" {
		return nil, kit.Conflict("that appointment was cancelled")
	}
	if in.PractitionerID != "" && p.set.calendarFor(in.PractitionerID) != cal {
		return nil, kit.Invalid("moving to another practitioner's calendar is done in Google Calendar")
	}
	start, _ := time.Parse(time.RFC3339, cur.Start.DateTime)
	end, _ := time.Parse(time.RFC3339, cur.End.DateTime)
	if in.Start.IsZero() || start.IsZero() {
		return nil, kit.Invalid("choose a new time")
	}
	newEnd := in.Start.Add(end.Sub(start))
	busy, err := p.c.FreeBusy(ctx, in.Start, newEnd, []string{cal})
	if err != nil {
		return nil, err
	}
	var others []Interval
	for _, b := range busy[cal] {
		if !b.Start.Equal(start) || !b.End.Equal(end) { // not the event itself
			others = append(others, b)
		}
	}
	if overlaps(others, in.Start, newEnd) {
		return nil, provider.ErrSlotTaken
	}
	patch := map[string]any{"start": EventTime{DateTime: in.Start.Format(time.RFC3339), TimeZone: cur.Start.TimeZone},
		"end": EventTime{DateTime: newEnd.Format(time.RFC3339), TimeZone: cur.End.TimeZone}}
	if in.PractitionerID != "" {
		patch["extendedProperties"] = map[string]any{"private": map[string]string{"weline_practitioner": in.PractitionerID}}
	}
	got, err := p.c.PatchEvent(ctx, cal, eventID, patch)
	if err != nil {
		return nil, err
	}
	cat, _ := p.catalog(ctx)
	a := p.view(cat, cal, got)
	return &a, nil
}

// Cancel deletes the event (Google keeps it as cancelled).
func (p *Provider) Cancel(ctx context.Context, id, _ string) error {
	eventID, cal, err := splitID(id)
	if err != nil {
		return err
	}
	return p.c.DeleteEvent(ctx, cal, eventID)
}

// GetAppointment reads one event.
func (p *Provider) GetAppointment(ctx context.Context, id string) (*provider.Appointment, error) {
	eventID, cal, err := splitID(id)
	if err != nil {
		return nil, err
	}
	e, err := p.c.GetEvent(ctx, cal, eventID)
	if err != nil {
		return nil, err
	}
	if e.Private("weline") != "1" {
		return nil, provider.ErrNotFound // not one of Weline's events
	}
	cat, _ := p.catalog(ctx)
	a := p.view(cat, cal, e)
	return &a, nil
}

// ListAppointments reads Weline's events from every configured calendar.
func (p *Provider) ListAppointments(ctx context.Context, q provider.AppointmentQuery) ([]provider.Appointment, error) {
	cat, err := p.catalog(ctx)
	if err != nil {
		return nil, err
	}
	var out []provider.Appointment
	for _, cal := range p.set.calendars() {
		evs, err := p.c.ListEvents(ctx, cal, EventQuery{From: q.From, To: q.To, UpdatedSince: q.ChangedSince, Contact: q.CustomerID})
		if err != nil {
			return nil, err
		}
		for i := range evs {
			if evs[i].Start.DateTime == "" { // all-day events aren't appointments
				continue
			}
			out = append(out, p.view(cat, cal, &evs[i]))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

var _ provider.Provider = (*Provider)(nil)
