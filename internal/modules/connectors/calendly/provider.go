package calendly

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/connectors/cliniko"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Name is the provider name.
const Name = "calendly"

const (
	locationID  = "calendly"
	windowDays  = 28
	chunk       = 7 * 24 * time.Hour // Calendly's longest availability window
	phonePrefix = "phone:"
)

// ParseSettings accepts no settings yet (an empty object).
func ParseSettings(raw json.RawMessage) error {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" || s == "{}" {
		return nil
	}
	return kit.Invalid("Calendly has no settings")
}

// Provider is Calendly for one organization (the token owner's event types
// and events).
type Provider struct {
	c   *Client
	now func() time.Time

	once sync.Once
	user *User
	err  error
}

// New builds the provider.
func New(c *Client) *Provider { return &Provider{c: c, now: time.Now} }

func (p *Provider) me(ctx context.Context) (*User, error) {
	p.once.Do(func() { p.user, p.err = p.c.Me(ctx) })
	return p.user, p.err
}

// TestConnection checks the token.
func (p *Provider) TestConnection(ctx context.Context) (string, error) {
	u, err := p.me(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Connected to Calendly as %s", u.Name), nil
}

func (p *Provider) Name() string { return Name }

func (p *Provider) tz(ctx context.Context) string {
	if u, err := p.me(ctx); err == nil && u.Timezone != "" {
		return u.Timezone
	}
	return "Australia/Sydney"
}

// ListLocations: Calendly has no sites; one place stands for "as set in Calendly".
func (p *Provider) ListLocations(ctx context.Context) ([]provider.Location, error) {
	if _, err := p.me(ctx); err != nil {
		return nil, err
	}
	return []provider.Location{{ID: locationID, Name: "As set in Calendly", TimeZone: p.tz(ctx)}}, nil
}

// ListPractitioners is the token's owner.
func (p *Provider) ListPractitioners(ctx context.Context, _ string) ([]provider.Practitioner, error) {
	u, err := p.me(ctx)
	if err != nil {
		return nil, err
	}
	return []provider.Practitioner{{ID: u.URI, Name: u.Name, LocationIDs: []string{locationID}}}, nil
}

func (p *Provider) eventTypes(ctx context.Context) ([]EventType, error) {
	u, err := p.me(ctx)
	if err != nil {
		return nil, err
	}
	return p.c.EventTypes(ctx, u.URI)
}

// ListServices are the active event types.
func (p *Provider) ListServices(ctx context.Context) ([]provider.Service, error) {
	ets, err := p.eventTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Service, 0, len(ets))
	for _, e := range ets {
		out = append(out, provider.Service{ID: e.URI, Name: e.Name, Minutes: e.Duration})
	}
	return out, nil
}

func (p *Provider) eventType(ctx context.Context, id string) (*EventType, error) {
	ets, err := p.eventTypes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range ets {
		if ets[i].URI == id {
			return &ets[i], nil
		}
	}
	return nil, kit.Invalid("choose a service")
}

// FindFreeTimes reads Calendly's availability, a week at a time.
func (p *Provider) FindFreeTimes(ctx context.Context, q provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	et, err := p.eventType(ctx, q.ServiceID)
	if err != nil {
		return nil, err
	}
	u, _ := p.me(ctx)
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	from, to := p.now().Add(time.Minute), p.now().AddDate(0, 0, windowDays)
	if !q.From.IsZero() && q.From.After(from) {
		from = q.From
	}
	if !q.To.IsZero() && q.To.Before(to) {
		to = q.To
	}
	out := &provider.FreeTimes{WindowDays: windowDays}
	for start := from; start.Before(to) && len(out.Times) < limit; start = start.Add(chunk) {
		end := start.Add(chunk)
		if end.After(to) {
			end = to
		}
		times, err := p.c.AvailableTimes(ctx, et.URI, start, end)
		if err != nil {
			return nil, err
		}
		for _, t := range times {
			out.Times = append(out.Times, provider.FreeTime{Start: t, End: t.Add(time.Duration(et.Duration) * time.Minute),
				PractitionerID: u.URI, PractitionerName: u.Name, LocationID: locationID})
			if len(out.Times) == limit {
				break
			}
		}
	}
	return out, nil
}

// FindOrCreateCustomer: Calendly has no customer records; a customer is their
// mobile number, matched with the SMS reminder number they give Calendly.
func (p *Provider) FindOrCreateCustomer(_ context.Context, in provider.CustomerInput) (*provider.Customer, bool, error) {
	d := cliniko.Normalise(in.Mobile)
	if len(d) < 8 {
		return nil, false, provider.ErrNotFound
	}
	return &provider.Customer{ID: phonePrefix + d, Name: in.Name}, false, nil
}

// GetCustomer returns the mobile for a "phone:" customer; others have none.
func (p *Provider) GetCustomer(_ context.Context, id string) (*provider.Customer, error) {
	if d, ok := strings.CutPrefix(id, phonePrefix); ok && d != "" {
		return &provider.Customer{ID: id, Mobile: d}, nil
	}
	return &provider.Customer{ID: id}, nil
}

// Book: bookings happen on Calendly's page (BookingLink).
func (p *Provider) Book(context.Context, provider.BookInput) (*provider.Appointment, error) {
	return nil, provider.ErrNotSupported
}

// Reschedule: patients reschedule on Calendly's page (RescheduleLink).
func (p *Provider) Reschedule(context.Context, string, provider.RescheduleInput) (*provider.Appointment, error) {
	return nil, provider.ErrNotSupported
}

// BookingLink implements provider.LinkBooker: a one-time Calendly link.
func (p *Provider) BookingLink(ctx context.Context, serviceID string) (string, error) {
	et, err := p.eventType(ctx, serviceID)
	if err != nil {
		return "", err
	}
	return p.c.SchedulingLink(ctx, et.URI)
}

// RescheduleLink implements provider.LinkBooker: the invitee's own link.
func (p *Provider) RescheduleLink(ctx context.Context, appointmentID string) (string, error) {
	ev, err := p.c.ScheduledEvent(ctx, p.eventURI(appointmentID))
	if err != nil {
		return "", err
	}
	invs, err := p.c.Invitees(ctx, ev.URI)
	if err != nil {
		return "", err
	}
	for _, in := range invs {
		if strings.HasPrefix(in.RescheduleURL, "https://") {
			return in.RescheduleURL, nil
		}
	}
	return "", provider.ErrNotSupported
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)

// Appointment ids are the scheduled event's UUID (the last part of its URI).
func (p *Provider) eventURI(id string) string {
	if !uuidRe.MatchString(id) {
		return p.c.api + "/invalid"
	}
	return p.c.api + "/scheduled_events/" + id
}

func lastPart(uri string) string { return uri[strings.LastIndex(uri, "/")+1:] }

// Cancel cancels the event in Calendly (Calendly tells the invitee).
func (p *Provider) Cancel(ctx context.Context, id, reason string) error {
	if reason == "" {
		reason = "Cancelled on WhatsApp"
	}
	return p.c.Cancel(ctx, p.eventURI(id), reason)
}

func mobileOf(in Invitee) string {
	if d := cliniko.Normalise(in.TextReminderNumber); len(d) >= 8 {
		return d
	}
	for _, qa := range in.Answers {
		q := strings.ToLower(qa.Question)
		if strings.Contains(q, "mobile") || strings.Contains(q, "phone") {
			if d := cliniko.Normalise(qa.Answer); len(d) >= 8 {
				return d
			}
		}
	}
	return ""
}

func (p *Provider) view(ctx context.Context, ev *ScheduledEvent, names map[string]string) (provider.Appointment, error) {
	a := provider.Appointment{ID: lastPart(ev.URI), Reference: strings.ToUpper(strings.ReplaceAll(lastPart(ev.URI), "-", ""))[:8],
		Status: provider.StatusBooked, Start: ev.StartTime, End: ev.EndTime, ServiceID: ev.EventType, ServiceName: names[ev.EventType],
		LocationID: locationID, LocationName: "As set in Calendly", TimeZone: p.tz(ctx), UpdatedAt: ev.UpdatedAt}
	if a.ServiceName == "" {
		a.ServiceName = ev.Name
	}
	if len(ev.Memberships) > 0 {
		a.PractitionerID = ev.Memberships[0].User
		if u, err := p.me(ctx); err == nil && u.URI == a.PractitionerID {
			a.PractitionerName = u.Name
		}
	}
	switch {
	case ev.Status == "canceled":
		a.Status = provider.StatusCancelled
	case ev.EndTime.Before(p.now()):
		a.Status = provider.StatusCompleted
	}
	invs, err := p.c.Invitees(ctx, ev.URI)
	if err != nil {
		return a, err
	}
	for _, in := range invs {
		if m := mobileOf(in); m != "" {
			a.CustomerID = phonePrefix + m
			break
		}
		a.CustomerID = "invitee:" + lastPart(in.URI) // no mobile: kept short, reminders skip it
	}
	return a, nil
}

func (p *Provider) names(ctx context.Context) map[string]string {
	out := map[string]string{}
	if ets, err := p.eventTypes(ctx); err == nil {
		for _, e := range ets {
			out[e.URI] = e.Name
		}
	}
	return out
}

// GetAppointment reads one event and its invitee.
func (p *Provider) GetAppointment(ctx context.Context, id string) (*provider.Appointment, error) {
	ev, err := p.c.ScheduledEvent(ctx, p.eventURI(id))
	if err != nil {
		return nil, err
	}
	a, err := p.view(ctx, ev, p.names(ctx))
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAppointments reads the owner's events; without a range, "changed since"
// reads events starting from then to a year ahead.
func (p *Provider) ListAppointments(ctx context.Context, q provider.AppointmentQuery) ([]provider.Appointment, error) {
	u, err := p.me(ctx)
	if err != nil {
		return nil, err
	}
	from, to := p.now().AddDate(0, 0, -1), p.now().AddDate(0, 3, 0)
	switch {
	case q.From != nil && q.To != nil:
		from, to = *q.From, *q.To
	case q.ChangedSince != nil:
		from, to = *q.ChangedSince, p.now().AddDate(1, 0, 0)
	}
	evs, err := p.c.ScheduledEvents(ctx, u.URI, from, to)
	if err != nil {
		return nil, err
	}
	names := p.names(ctx)
	var out []provider.Appointment
	for i := range evs {
		a, err := p.view(ctx, &evs[i], names)
		if err != nil {
			return nil, err
		}
		if q.CustomerID != "" && a.CustomerID != q.CustomerID {
			continue
		}
		if q.ChangedSince != nil && a.UpdatedAt.Before(*q.ChangedSince) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

var (
	_ provider.Provider   = (*Provider)(nil)
	_ provider.LinkBooker = (*Provider)(nil)
)
