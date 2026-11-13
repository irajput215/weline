package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
)

// BuiltIn is Weline's own diary behind the connector interface (connectors
// R4): every call maps onto what Booking already does, so behaviour and tests
// are unchanged. It is bound to one organization.
type BuiltIn struct {
	svc   *Service
	orgID uuid.UUID
}

var _ provider.Provider = (*BuiltIn)(nil)

// Provider returns the built-in provider for an organization.
func (s *Service) Provider(orgID uuid.UUID) *BuiltIn { return &BuiltIn{svc: s, orgID: orgID} }

// BuiltInResolver resolves every organization to the built-in provider. It is
// Booking's resolver until the registry hands it Connectors' one.
func (s *Service) BuiltInResolver() provider.Resolver {
	return provider.ResolverFunc(func(_ context.Context, orgID uuid.UUID) (provider.Provider, error) {
		return s.Provider(orgID), nil
	})
}

// Name implements provider.Provider.
func (b *BuiltIn) Name() string { return provider.BuiltIn }

func parseID(id string) (uuid.UUID, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, provider.ErrNotFound
	}
	return u, nil
}

// builtinErr turns Booking's errors into the interface's: any clash when
// booking or moving (the time, the practitioner or the patient already taken)
// becomes ErrSlotTaken, as the chatbot always answered; Booking's own message
// stays in the error text for logs. Other errors pass through.
func builtinErr(err error) error {
	if errors.Is(err, kit.ErrConflict) && !errors.Is(err, errNotSetUp) {
		return fmt.Errorf("%s: %w", err.Error(), provider.ErrSlotTaken)
	}
	return err
}

// ListLocations implements provider.Provider.
func (b *BuiltIn) ListLocations(ctx context.Context) ([]provider.Location, error) {
	c, err := b.svc.clinic(ctx, b.orgID)
	if err != nil {
		return nil, err
	}
	locs, err := b.svc.practice.ActiveLocations(ctx, b.orgID)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Location, 0, len(locs))
	for _, l := range locs {
		out = append(out, provider.Location{ID: l.ID.String(), Name: l.Name, TimeZone: c.tz.String()})
	}
	return out, nil
}

// ListPractitioners implements provider.Provider.
func (b *BuiltIn) ListPractitioners(ctx context.Context, locationID string) ([]provider.Practitioner, error) {
	prs, err := b.svc.practice.ActivePractitioners(ctx, b.orgID)
	if err != nil {
		return nil, err
	}
	out := []provider.Practitioner{}
	for _, p := range prs {
		ids := make([]string, 0, len(p.LocationIDs))
		at := locationID == ""
		for _, l := range p.LocationIDs {
			ids = append(ids, l.String())
			if l.String() == locationID {
				at = true
			}
		}
		if at {
			out = append(out, provider.Practitioner{ID: p.ID.String(), Name: p.DisplayName, LocationIDs: ids})
		}
	}
	return out, nil
}

// ListServices implements provider.Provider.
func (b *BuiltIn) ListServices(ctx context.Context) ([]provider.Service, error) {
	svcs, err := b.svc.practice.ActiveServices(ctx, b.orgID)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Service, 0, len(svcs))
	for _, sv := range svcs {
		out = append(out, provider.Service{ID: sv.ID.String(), Name: sv.Name, Minutes: sv.DurationMinutes})
	}
	return out, nil
}

// FindFreeTimes implements provider.Provider: Booking's slot search in 31-day
// steps from From until Limit times are found or To (or the booking window)
// ends, each time with the first practitioner free then.
func (b *BuiltIn) FindFreeTimes(ctx context.Context, q provider.FreeTimeQuery) (*provider.FreeTimes, error) {
	c, err := b.svc.clinic(ctx, b.orgID)
	if err != nil {
		return nil, err
	}
	serviceID, err := parseID(q.ServiceID)
	if err != nil {
		return nil, err
	}
	locationID, err := parseID(q.LocationID)
	if err != nil {
		return nil, err
	}
	var practitionerID *uuid.UUID
	if q.PractitionerID != "" {
		id, err := parseID(q.PractitionerID)
		if err != nil {
			return nil, err
		}
		practitionerID = &id
	}
	limit := q.Limit
	if limit <= 0 || limit > maxSlots {
		limit = maxSlots
	}
	from := c.today(b.svc.now())
	if !q.From.IsZero() {
		from = c.today(q.From)
	}
	end := c.today(b.svc.now().AddDate(0, 0, c.settings.WindowDays))
	if !q.To.IsZero() {
		end = c.today(q.To)
	}
	var found []Slot
	for d := from; !d.After(end.Time) && len(found) < limit; d = (practice.Date{Time: d.AddDate(0, 0, maxRangeDays)}) {
		to := practice.Date{Time: d.AddDate(0, 0, maxRangeDays-1)}
		if to.After(end.Time) {
			to = end
		}
		got, err := b.svc.searchSlots(ctx, b.orgID, c, SlotQuery{ServiceID: serviceID, LocationID: locationID,
			PractitionerID: practitionerID, From: d, To: to, Patient: q.Patient, Limit: limit - len(found)})
		if err != nil {
			return nil, err
		}
		found = append(found, got...)
	}
	names := map[uuid.UUID]string{}
	prs, err := b.svc.practice.ActivePractitioners(ctx, b.orgID)
	if err != nil {
		return nil, err
	}
	for _, p := range prs {
		names[p.ID] = p.DisplayName
	}
	minutes := 0
	if svcs, err := b.svc.practice.ActiveServices(ctx, b.orgID); err == nil {
		for _, sv := range svcs {
			if sv.ID == serviceID {
				minutes = sv.DurationMinutes
			}
		}
	}
	out := &provider.FreeTimes{Times: make([]provider.FreeTime, 0, len(found)), WindowDays: c.settings.WindowDays}
	for _, sl := range found {
		pr := sl.PractitionerIDs[0]
		out.Times = append(out.Times, provider.FreeTime{Start: sl.StartsAt, End: sl.StartsAt.Add(time.Duration(minutes) * time.Minute),
			PractitionerID: pr.String(), PractitionerName: names[pr], LocationID: q.LocationID})
	}
	return out, nil
}

// FindOrCreateCustomer implements provider.Provider: the customer is the
// Weline contact itself.
func (b *BuiltIn) FindOrCreateCustomer(ctx context.Context, in provider.CustomerInput) (*provider.Customer, bool, error) {
	if in.ContactID != uuid.Nil {
		p, err := b.svc.patient(ctx, b.orgID, in.ContactID)
		if err != nil {
			return nil, false, err
		}
		return &provider.Customer{ID: p.ID.String(), Name: p.Name}, false, nil
	}
	p, existing, err := b.svc.FindOrCreatePatient(ctx, b.orgID, in.Name, in.Mobile)
	if err != nil {
		return nil, false, err
	}
	return &provider.Customer{ID: p.ID.String(), Name: p.Name}, !existing, nil
}

// Book implements provider.Provider.
func (b *BuiltIn) Book(ctx context.Context, in provider.BookInput) (*provider.Appointment, error) {
	ids := make([]uuid.UUID, 4)
	for i, s := range []string{in.CustomerID, in.ServiceID, in.LocationID, in.PractitionerID} {
		id, err := parseID(s)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	source := in.Source
	if source == "" {
		source = SourceWhatsApp
	}
	a, err := b.svc.Create(ctx, b.orgID, Actor{Source: source}, NewAppointment{ContactID: ids[0], ServiceID: ids[1],
		LocationID: ids[2], PractitionerID: ids[3], StartsAt: in.Start, Note: in.Note})
	if err != nil {
		return nil, builtinErr(err)
	}
	return b.view(ctx, a)
}

// Reschedule implements provider.Provider.
func (b *BuiltIn) Reschedule(ctx context.Context, id string, in provider.RescheduleInput) (*provider.Appointment, error) {
	aid, err := parseID(id)
	if err != nil {
		return nil, err
	}
	mv := MoveInput{}
	if !in.Start.IsZero() {
		mv.StartsAt = &in.Start
	}
	if in.PractitionerID != "" {
		p, err := parseID(in.PractitionerID)
		if err != nil {
			return nil, err
		}
		mv.PractitionerID = &p
	}
	if in.LocationID != "" {
		l, err := parseID(in.LocationID)
		if err != nil {
			return nil, err
		}
		mv.LocationID = &l
	}
	a, err := b.svc.Move(ctx, b.orgID, Actor{Source: SourceWhatsApp}, aid, mv)
	if err != nil {
		return nil, err // not builtinErr: "can't be moved" is a conflict too, and must say so
	}
	return b.view(ctx, a)
}

// Cancel implements provider.Provider.
func (b *BuiltIn) Cancel(ctx context.Context, id string, reason string) error {
	aid, err := parseID(id)
	if err != nil {
		return err
	}
	_, err = b.svc.SetStatus(ctx, b.orgID, Actor{Source: SourceWhatsApp}, aid, StatusCancelled, reason)
	return err
}

// GetAppointment implements provider.Provider.
func (b *BuiltIn) GetAppointment(ctx context.Context, id string) (*provider.Appointment, error) {
	aid, err := parseID(id)
	if err != nil {
		return nil, err
	}
	a, err := b.svc.Get(ctx, b.orgID, aid)
	if err != nil {
		return nil, err
	}
	return b.view(ctx, a)
}

// ListAppointments implements provider.Provider: changed since a time, or
// starting in a range, optionally for one contact.
func (b *BuiltIn) ListAppointments(ctx context.Context, q provider.AppointmentQuery) ([]provider.Appointment, error) {
	db := b.svc.db.WithContext(ctx).Where("organization_id = ?", b.orgID)
	switch {
	case q.ChangedSince != nil:
		db = db.Where("updated_at > ?", *q.ChangedSince)
	case q.From != nil && q.To != nil:
		db = db.Where("starts_at >= ? AND starts_at < ?", *q.From, *q.To)
	default:
		return nil, fmt.Errorf("%w: give changed since, or from and to", provider.ErrInvalid)
	}
	if q.CustomerID != "" {
		cid, err := parseID(q.CustomerID)
		if err != nil {
			return nil, err
		}
		db = db.Where("contact_id = ?", cid)
	}
	var rows []appointmentRow
	if err := db.Order("updated_at, id").Limit(500).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list appointments: %w", err)
	}
	updated := map[uuid.UUID]time.Time{}
	for _, r := range rows {
		updated[r.ID] = r.UpdatedAt
	}
	as, err := b.svc.present(ctx, b.orgID, rows)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Appointment, 0, len(as))
	for i := range as {
		v, err := b.view(ctx, &as[i])
		if err != nil {
			return nil, err
		}
		v.UpdatedAt = updated[as[i].ID]
		out = append(out, *v)
	}
	return out, nil
}

var builtinStatus = map[Status]string{
	StatusBooked: provider.StatusBooked, StatusConfirmed: provider.StatusBooked, StatusArrived: provider.StatusBooked,
	StatusCancelled: provider.StatusCancelled, StatusCompleted: provider.StatusCompleted, StatusNoShow: provider.StatusNoShow,
}

func (b *BuiltIn) view(ctx context.Context, a *Appointment) (*provider.Appointment, error) {
	c, err := b.svc.clinic(ctx, b.orgID)
	if err != nil {
		return nil, err
	}
	return &provider.Appointment{ID: a.ID.String(), Reference: a.Reference, Status: builtinStatus[a.Status],
		Start: a.StartsAt, End: a.EndsAt, ServiceID: a.Service.ID.String(), ServiceName: a.Service.Name,
		PractitionerID: a.Practitioner.ID.String(), PractitionerName: a.Practitioner.Name,
		LocationID: a.Location.ID.String(), LocationName: a.Location.Name, TimeZone: c.tz.String(),
		CustomerID: a.Contact.ID.String(), UpdatedAt: a.CreatedAt}, nil
}
