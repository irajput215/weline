package msgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Name is the provider name.
const Name = "microsoft_365"

// Settings are an organization's tenant and mailboxes: one for everyone,
// optionally one per practitioner (Weline practitioner id → mailbox).
type Settings struct {
	Tenant                string            `json:"-"` // from the tenant_id field
	Mailbox               string            `json:"-"` // from the mailbox field
	PractitionerMailboxes map[string]string `json:"practitioner_mailboxes,omitempty"`
}

func validMailbox(m string) bool {
	a, err := mail.ParseAddress(m)
	return err == nil && a.Address == m && len(m) <= 255
}

// ParseSettings reads the fields and the settings JSON.
func ParseSettings(tenant, mailbox string, raw json.RawMessage) (Settings, error) {
	var s Settings
	if len(raw) > 0 && string(raw) != "null" {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			return s, kit.Invalid("Microsoft 365 settings: practitioner_mailboxes must map practitioner ids to mailboxes")
		}
	}
	s.Tenant, s.Mailbox = strings.TrimSpace(tenant), strings.ToLower(strings.TrimSpace(mailbox))
	if !ValidTenant(s.Tenant) {
		return s, kit.Invalid("tenant_id must be your Microsoft 365 tenant id (a GUID); approve Weline first to fill it in")
	}
	if !validMailbox(s.Mailbox) {
		return s, kit.Invalid("mailbox must be the email address whose calendar Weline books into")
	}
	for p, m := range s.PractitionerMailboxes {
		m = strings.ToLower(strings.TrimSpace(m))
		if p == "" || !validMailbox(m) {
			return s, kit.Invalid("practitioner_mailboxes: each entry needs a practitioner id and an email address")
		}
		s.PractitionerMailboxes[p] = m
	}
	return s, nil
}

func (s Settings) mailboxFor(practitionerID string) string {
	if m := s.PractitionerMailboxes[practitionerID]; m != "" {
		return m
	}
	return s.Mailbox
}

// Mailboxes are every mailbox the settings use.
func (s Settings) Mailboxes() []string {
	seen := map[string]bool{s.Mailbox: true}
	out := []string{s.Mailbox}
	for _, m := range s.PractitionerMailboxes {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Strings(out[1:])
	return out
}

// Provider is Microsoft 365 for one organization: catalog, customers and
// candidate times from Weline's own diary (base), busy times and bookings in
// the mailbox calendars.
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

// TestConnection checks every mailbox calendar can be read.
func (p *Provider) TestConnection(ctx context.Context) (string, error) {
	for _, m := range p.set.Mailboxes() {
		if _, err := p.c.GetCalendar(ctx, p.set.Tenant, m); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("Connected to Microsoft 365: %s", strings.Join(p.set.Mailboxes(), ", ")), nil
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

// FindFreeTimes drops Weline's candidate times that are busy in the mailbox.
func (p *Provider) FindFreeTimes(ctx context.Context, q provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	want := q.Limit
	if want <= 0 {
		want = 20
	}
	q.Limit = 200
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
	busy, err := p.c.Busy(ctx, p.set.Tenant, min, max, p.set.Mailboxes())
	if err != nil {
		return nil, err
	}
	out := &provider.FreeTimes{WindowDays: cand.WindowDays}
	for _, t := range cand.Times {
		if overlaps(busy[p.set.mailboxFor(t.PractitionerID)], t.Start, t.End) {
			continue
		}
		out.Times = append(out.Times, t)
		if len(out.Times) == want {
			break
		}
	}
	return out, nil
}

// Appointment ids are "<mailbox>~<event id>" (Graph ids never hold "~").
func splitID(id string) (mailbox, event string, err error) {
	i := strings.LastIndex(id, "~")
	if i <= 0 || i == len(id)-1 {
		return "", "", provider.ErrNotFound
	}
	return id[:i], id[i+1:], nil
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

func reference(id string) string {
	r := strings.ToUpper(strings.NewReplacer("-", "", "_", "", "=", "").Replace(id))
	if len(r) > 8 {
		r = r[len(r)-8:]
	}
	return r
}

func (p *Provider) view(cat *catalog, mailbox string, e *Event, l *Link) provider.Appointment {
	a := provider.Appointment{ID: mailbox + "~" + e.ID, Reference: reference(e.ID), Status: provider.StatusBooked,
		Start: e.Start.Time(), End: e.End.Time(), ServiceID: l.Service, PractitionerID: l.Practitioner,
		LocationID: l.Location, CustomerID: l.Contact, TimeZone: l.TimeZone}
	a.UpdatedAt, _ = time.Parse(time.RFC3339, e.LastModifiedDateTime)
	switch {
	case e.IsCancelled:
		a.Status = provider.StatusCancelled
	case !a.End.IsZero() && a.End.Before(p.now()):
		a.Status = provider.StatusCompleted
	}
	if cat != nil {
		a.ServiceName = cat.services[a.ServiceID].Name
		a.PractitionerName = cat.practitioners[a.PractitionerID].Name
		loc := cat.locations[a.LocationID]
		a.LocationName = loc.Name
		if a.TimeZone == "" {
			a.TimeZone = loc.TimeZone
		}
	}
	return a
}

// Book checks the time is still free in the mailbox and creates the event.
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
	mb := p.set.mailboxFor(in.PractitionerID)
	busy, err := p.c.Busy(ctx, p.set.Tenant, in.Start, end, []string{mb})
	if err != nil {
		return nil, err
	}
	if overlaps(busy[mb], in.Start, end) {
		return nil, provider.ErrSlotTaken
	}
	subject := svc.Name
	if c, err := p.base.GetCustomer(ctx, in.CustomerID); err == nil && c.Name != "" {
		subject += " – " + c.Name
	}
	link := Link{Contact: in.CustomerID, Service: in.ServiceID, Location: in.LocationID, Practitioner: in.PractitionerID, TimeZone: loc.TimeZone}
	ev, err := p.c.CreateEvent(ctx, p.set.Tenant, mb, subject, in.Start, end, link)
	if err != nil {
		return nil, err
	}
	a := p.view(cat, mb, ev, &link)
	return &a, nil
}

func (p *Provider) event(ctx context.Context, id string) (string, *Event, *Link, error) {
	mb, eventID, err := splitID(id)
	if err != nil {
		return "", nil, nil, err
	}
	known := false
	for _, m := range p.set.Mailboxes() {
		known = known || m == mb
	}
	if !known {
		return "", nil, nil, provider.ErrNotFound // only this organization's mailboxes
	}
	e, err := p.c.GetEvent(ctx, p.set.Tenant, mb, eventID)
	if err != nil {
		return "", nil, nil, err
	}
	l := e.Link()
	if l == nil {
		return "", nil, nil, provider.ErrNotFound // not one of Weline's events
	}
	return mb, e, l, nil
}

// Reschedule moves an event to a new start (same length, same mailbox).
func (p *Provider) Reschedule(ctx context.Context, id string, in provider.RescheduleInput) (*provider.Appointment, error) {
	mb, e, l, err := p.event(ctx, id)
	if err != nil {
		return nil, err
	}
	if e.IsCancelled {
		return nil, kit.Conflict("that appointment was cancelled")
	}
	if in.PractitionerID != "" && p.set.mailboxFor(in.PractitionerID) != mb {
		return nil, kit.Invalid("moving to another practitioner's calendar is done in Outlook")
	}
	if in.Start.IsZero() {
		return nil, kit.Invalid("choose a new time")
	}
	start, end := e.Start.Time(), e.End.Time()
	newEnd := in.Start.Add(end.Sub(start))
	busy, err := p.c.Busy(ctx, p.set.Tenant, in.Start, newEnd, []string{mb})
	if err != nil {
		return nil, err
	}
	var others []Interval
	for _, b := range busy[mb] {
		if !b.Start.Equal(start) || !b.End.Equal(end) { // not the event itself
			others = append(others, b)
		}
	}
	if overlaps(others, in.Start, newEnd) {
		return nil, provider.ErrSlotTaken
	}
	if err := p.c.MoveEvent(ctx, p.set.Tenant, mb, e.ID, in.Start, newEnd); err != nil {
		return nil, err
	}
	e.Start, e.End = utc(in.Start), utc(newEnd)
	cat, _ := p.catalog(ctx)
	a := p.view(cat, mb, e, l)
	return &a, nil
}

// Cancel deletes the event; it then reads as not found.
func (p *Provider) Cancel(ctx context.Context, id, _ string) error {
	mb, e, _, err := p.event(ctx, id)
	if err != nil {
		return err
	}
	return p.c.DeleteEvent(ctx, p.set.Tenant, mb, e.ID)
}

// GetAppointment reads one event.
func (p *Provider) GetAppointment(ctx context.Context, id string) (*provider.Appointment, error) {
	mb, e, l, err := p.event(ctx, id)
	if err != nil {
		return nil, err
	}
	cat, _ := p.catalog(ctx)
	a := p.view(cat, mb, e, l)
	return &a, nil
}

// ListAppointments reads Weline's events from every mailbox. Without a range,
// "changed since" reads events starting from then to a year ahead.
func (p *Provider) ListAppointments(ctx context.Context, q provider.AppointmentQuery) ([]provider.Appointment, error) {
	from, to := p.now().AddDate(0, 0, -1), p.now().AddDate(0, 3, 0)
	switch {
	case q.From != nil && q.To != nil:
		from, to = *q.From, *q.To
	case q.ChangedSince != nil:
		from, to = *q.ChangedSince, p.now().AddDate(1, 0, 0)
	}
	cat, err := p.catalog(ctx)
	if err != nil {
		return nil, err
	}
	var out []provider.Appointment
	for _, mb := range p.set.Mailboxes() {
		evs, err := p.c.CalendarView(ctx, p.set.Tenant, mb, from, to)
		if err != nil {
			return nil, err
		}
		for i := range evs {
			l := evs[i].Link()
			if l == nil || (q.CustomerID != "" && l.Contact != q.CustomerID) {
				continue
			}
			a := p.view(cat, mb, &evs[i], l)
			if q.ChangedSince != nil && !a.UpdatedAt.IsZero() && a.UpdatedAt.Before(*q.ChangedSince) {
				continue
			}
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

var _ provider.Provider = (*Provider)(nil)
