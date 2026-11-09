package booking

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var ctx = context.Background()

func ptr[T any](v T) *T { return &v }

var sydney, _ = time.LoadLocation("Australia/Sydney")

// syd is a Sydney wall-clock time.
func syd(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, sydney)
}

func date(s string) practice.Date {
	d, err := practice.ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

// fx is a clinic in Sydney on a 15-minute grid, "now" fixed at Mon 1 Mar 2027
// 09:00:
//   - Bondi, open Mon–Fri 08:00–17:00
//   - Dr Ada (GP) and Dr Ben (GP), and Nurse Cy (Nurse), all at Bondi
//   - Consult (15 min, GP), Long consult (30 min, GP), Dressing (15 min, Nurse)
//   - Ada works Mon 09:00–12:00, Ben Mon 09:00–10:00, Cy no hours yet
type fx struct {
	db           *gorm.DB
	svc          *Service
	ps           *practice.Service
	org          uuid.UUID
	bondi        uuid.UUID
	ada, ben, cy uuid.UUID
	consult      uuid.UUID
	long         uuid.UUID
	dressing     uuid.UUID
	patient      uuid.UUID
	staff        Actor
}

const monday = "2027-03-08" // a week after "now"

func newFx(t *testing.T) *fx {
	t.Helper()
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	ps := practice.NewService(db)
	f := &fx{db: db, ps: ps, org: org.ID, svc: NewService(db)}
	f.svc.now = func() time.Time { return syd(2027, 3, 1, 9, 0) }

	_, err := ps.SaveProfile(ctx, f.org, practice.ProfileInput{Name: "Harbour Clinic", TimeZone: "Australia/Sydney",
		BookingSlug: "c" + strings.ReplaceAll(uuid.NewString()[:8], "-", ""), SlotMinutes: 15})
	require.NoError(t, err)
	loc, err := ps.CreateLocation(ctx, f.org, practice.LocationInput{Name: "Bondi"})
	require.NoError(t, err)
	f.bondi = loc.ID
	week := []practice.Hours{}
	for d := 1; d <= 5; d++ {
		week = append(week, practice.Hours{Weekday: d, OpensAt: "08:00", ClosesAt: "17:00"})
	}
	_, err = ps.SetHours(ctx, f.org, f.bondi, week)
	require.NoError(t, err)

	pr := func(name, role string) uuid.UUID {
		p, err := ps.CreatePractitioner(ctx, f.org, practice.PractitionerInput{DisplayName: name, Role: role, LocationIDs: []uuid.UUID{f.bondi}})
		require.NoError(t, err)
		return p.ID
	}
	f.ada, f.ben, f.cy = pr("Dr Ada", "GP"), pr("Dr Ben", "GP"), pr("Nurse Cy", "Nurse")
	sv := func(name string, minutes int, role string) uuid.UUID {
		s, _, err := ps.CreateService(ctx, f.org, practice.ServiceInput{Name: name, DurationMinutes: minutes, AllowedRoles: []string{role}})
		require.NoError(t, err)
		return s.ID
	}
	f.consult, f.long, f.dressing = sv("Consult", 15, "GP"), sv("Long consult", 30, "GP"), sv("Dressing", 15, "Nurse")

	f.hours(t, f.ada, Availability{LocationID: f.bondi, Weekday: 1, StartsAt: "09:00", EndsAt: "12:00"})
	f.hours(t, f.ben, Availability{LocationID: f.bondi, Weekday: 1, StartsAt: "09:00", EndsAt: "10:00"})
	f.patient = testutil.CreateTestContact(t, db, f.org).ID
	user := testutil.CreateTestUser(t, db, f.org).ID
	f.staff = Actor{UserID: &user, Source: SourceStaff}
	return f
}

func (f *fx) hours(t *testing.T, pr uuid.UUID, blocks ...Availability) {
	t.Helper()
	_, _, err := f.svc.SetAvailability(ctx, f.org, pr, blocks)
	require.NoError(t, err)
}

func (f *fx) slots(t *testing.T, service uuid.UUID, pr *uuid.UUID, day string) []string {
	t.Helper()
	got, err := f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: service, LocationID: f.bondi, PractitionerID: pr, From: date(day), To: date(day)})
	require.NoError(t, err)
	out := make([]string, len(got))
	for i, s := range got {
		out[i] = s.StartsAt.In(sydney).Format("15:04")
	}
	return out
}

func (f *fx) book(t *testing.T, pr uuid.UUID, at time.Time) *Appointment {
	t.Helper()
	a, err := f.svc.Create(ctx, f.org, f.staff, NewAppointment{ContactID: f.patient, ServiceID: f.consult, LocationID: f.bondi, PractitionerID: pr, StartsAt: at})
	require.NoError(t, err)
	return a
}

func range15(from, to string) []string {
	start, _ := time.Parse("15:04", from)
	end, _ := time.Parse("15:04", to)
	var out []string
	for t := start; t.Before(end); t = t.Add(15 * time.Minute) {
		out = append(out, t.Format("15:04"))
	}
	return out
}

// ---------- availability (R1–R3) ----------

func TestAvailability_Validation(t *testing.T) {
	f := newFx(t)
	set := func(blocks ...Availability) error {
		_, _, err := f.svc.SetAvailability(ctx, f.org, f.cy, blocks)
		return err
	}
	a := func(day int, from, to string) Availability {
		return Availability{LocationID: f.bondi, Weekday: day, StartsAt: from, EndsAt: to}
	}
	assert.NoError(t, set(a(1, "08:00", "12:00"), a(1, "13:00", "17:00")), "two blocks a day")
	assert.ErrorIs(t, set(a(1, "08:00", "12:00"), a(1, "11:00", "13:00")), kit.ErrInvalid, "overlap")
	assert.ErrorIs(t, set(a(1, "08:05", "12:00")), kit.ErrInvalid, "off grid")
	assert.ErrorIs(t, set(a(1, "12:00", "08:00")), kit.ErrInvalid, "end before start")
	assert.ErrorIs(t, set(a(8, "08:00", "12:00")), kit.ErrInvalid, "weekday")

	other, err := f.ps.CreateLocation(ctx, f.org, practice.LocationInput{Name: "Manly"})
	require.NoError(t, err)
	err = set(Availability{LocationID: other.ID, Weekday: 2, StartsAt: "09:00", EndsAt: "10:00"})
	assert.ErrorIs(t, err, kit.ErrInvalid, "doesn't work at Manly")

	_, warnings, err := f.svc.SetAvailability(ctx, f.org, f.cy, []Availability{a(6, "09:00", "12:00"), a(2, "07:00", "09:00")})
	require.NoError(t, err)
	assert.Len(t, warnings, 2, "Saturday closed; Tuesday 07:00 before opening")

	got, err := f.svc.GetAvailability(ctx, f.org, f.cy)
	require.NoError(t, err)
	assert.Len(t, got, 2, "the week was replaced")
}

// ---------- slot search (R4–R6) ----------

func TestSlots_OnePractitionerAndAnyone(t *testing.T) {
	f := newFx(t)
	assert.Equal(t, range15("09:00", "12:00"), f.slots(t, f.consult, &f.ada, monday))
	assert.Equal(t, range15("09:00", "10:00"), f.slots(t, f.consult, &f.ben, monday))
	assert.Equal(t, range15("09:00", "11:45"), f.slots(t, f.long, &f.ada, monday), "30 minutes must fit")
	assert.Empty(t, f.slots(t, f.consult, &f.ada, "2027-03-09"), "Ada doesn't work Tuesdays")

	got, err := f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: f.consult, LocationID: f.bondi, From: date(monday), To: date(monday)})
	require.NoError(t, err)
	require.Len(t, got, 12, "anyone: each time once")
	assert.Equal(t, []uuid.UUID{f.ada, f.ben}, got[0].PractitionerIDs, "09:00: both, by name")
	assert.Equal(t, []uuid.UUID{f.ada}, got[4].PractitionerIDs, "10:00: only Ada")
	assert.Equal(t, "2027-03-08T09:00", got[0].Local)
}

func TestSlots_RolesAndInactive(t *testing.T) {
	f := newFx(t)
	_, err := f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: f.dressing, LocationID: f.bondi, PractitionerID: &f.ada, From: date(monday)})
	assert.ErrorIs(t, err, kit.ErrInvalid, "a GP doesn't do dressings")
	assert.Empty(t, f.slots(t, f.dressing, nil, monday), "the nurse has no hours")

	_, err = f.ps.UpdatePractitioner(ctx, f.org, f.ben, practice.PractitionerPatch{IsActive: ptr(false)})
	require.NoError(t, err)
	got, err := f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: f.consult, LocationID: f.bondi, From: date(monday)})
	require.NoError(t, err)
	for _, s := range got {
		assert.NotContains(t, s.PractitionerIDs, f.ben, "inactive practitioner")
	}
	_, _, err = f.ps.UpdateService(ctx, f.org, f.consult, practice.ServicePatch{IsActive: ptr(false)})
	require.NoError(t, err)
	_, err = f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: f.consult, LocationID: f.bondi, From: date(monday)})
	assert.ErrorIs(t, err, kit.ErrInvalid, "inactive service")
}

func TestSlots_ClosuresBlocksAndAppointments(t *testing.T) {
	f := newFx(t)
	_, err := f.ps.CreateClosure(ctx, f.org, practice.ClosureInput{StartsOn: monday, EndsOn: monday, Reason: "Public holiday"})
	require.NoError(t, err)
	assert.Empty(t, f.slots(t, f.consult, &f.ada, monday), "all-locations closure")
	assert.NotEmpty(t, f.slots(t, f.consult, &f.ada, "2027-03-15"), "next Monday is open")

	day := "2027-03-15"
	block, clashes, err := f.svc.CreateBlock(ctx, f.org, f.staff, f.ada, syd(2027, 3, 15, 10, 0), syd(2027, 3, 15, 11, 0), "Meeting")
	require.NoError(t, err)
	assert.Empty(t, clashes)
	want := append(range15("09:00", "10:00"), range15("11:00", "12:00")...)
	assert.Equal(t, want, f.slots(t, f.consult, &f.ada, day), "block removes 10:00–11:00")
	require.NoError(t, f.svc.DeleteBlock(ctx, f.org, block.ID))
	assert.Equal(t, range15("09:00", "12:00"), f.slots(t, f.consult, &f.ada, day), "and comes back")

	a := f.book(t, f.ada, syd(2027, 3, 15, 9, 30))
	assert.NotContains(t, f.slots(t, f.consult, &f.ada, day), "09:30", "booked")
	_, clashes, err = f.svc.CreateBlock(ctx, f.org, f.staff, f.ada, syd(2027, 3, 15, 9, 0), syd(2027, 3, 15, 10, 0), "Leave")
	require.NoError(t, err)
	require.Len(t, clashes, 1, "a block over an appointment lists it")
	assert.Equal(t, a.ID, clashes[0].ID)
}

func TestSlots_NoticeAndWindowForPatients(t *testing.T) {
	f := newFx(t)
	// "now" is Mon 1 Mar 09:00: patients need 2 hours' notice, staff don't.
	staff, err := f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: f.consult, LocationID: f.bondi, PractitionerID: &f.ada, From: date("2027-03-01")})
	require.NoError(t, err)
	assert.Equal(t, "09:00", staff[0].StartsAt.In(sydney).Format("15:04"))
	pat, err := f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: f.consult, LocationID: f.bondi, PractitionerID: &f.ada, From: date("2027-03-01"), Patient: true})
	require.NoError(t, err)
	assert.Equal(t, "11:00", pat[0].StartsAt.In(sydney).Format("15:04"))

	_, err = f.svc.SaveSettings(ctx, f.org, 0, 7)
	require.NoError(t, err)
	pat, err = f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: f.consult, LocationID: f.bondi, PractitionerID: &f.ada, From: date("2027-03-15"), Patient: true})
	require.NoError(t, err)
	assert.Empty(t, pat, "beyond the 7-day window")

	_, err = f.svc.SearchSlots(ctx, f.org, SlotQuery{ServiceID: f.consult, LocationID: f.bondi, From: date("2027-03-01"), To: date("2027-04-15")})
	assert.ErrorIs(t, err, kit.ErrInvalid, "range too long")
}

// ---------- appointments (R7–R13) ----------

func TestCreate_RulesAndHistory(t *testing.T) {
	f := newFx(t)
	a := f.book(t, f.ada, syd(2027, 3, 8, 9, 0))
	assert.Equal(t, StatusBooked, a.Status)
	assert.Len(t, a.Reference, 6)
	assert.Equal(t, "Dr Ada", a.Practitioner.Name)
	assert.Equal(t, 15, a.Duration)
	assert.Equal(t, f.patient, a.Contact.ID)

	base := NewAppointment{ContactID: f.patient, ServiceID: f.consult, LocationID: f.bondi, PractitionerID: f.ada}
	try := func(at time.Time, mod func(*NewAppointment)) error {
		in := base
		in.StartsAt = at
		in.AllowPatientOverlap = true
		if mod != nil {
			mod(&in)
		}
		_, err := f.svc.Create(ctx, f.org, f.staff, in)
		return err
	}
	err := try(syd(2027, 3, 8, 9, 0), nil)
	require.ErrorIs(t, err, kit.ErrConflict, "double booking")
	assert.Contains(t, err.Error(), "Dr Ada already has an appointment 09:00–09:15")
	assert.ErrorIs(t, try(syd(2027, 3, 8, 9, 5), nil), kit.ErrInvalid, "off the grid")
	assert.ErrorIs(t, try(syd(2027, 3, 8, 13, 0), nil), kit.ErrInvalid, "outside Ada's hours")
	assert.ErrorIs(t, try(syd(2027, 2, 22, 9, 0), nil), kit.ErrInvalid, "in the past")
	assert.NoError(t, try(syd(2027, 3, 8, 13, 0), func(in *NewAppointment) { in.OutsideHours = true }), "staff override")
	assert.ErrorIs(t, try(syd(2027, 3, 8, 13, 0), func(in *NewAppointment) { in.OutsideHours = true }), kit.ErrConflict,
		"override never allows a clash")
	assert.ErrorIs(t, try(syd(2027, 3, 8, 10, 0), func(in *NewAppointment) { in.ServiceID = f.dressing }), kit.ErrInvalid, "role")
	assert.ErrorIs(t, try(syd(2027, 3, 8, 10, 0), func(in *NewAppointment) { in.ContactID = uuid.New() }), kit.ErrNotFound, "unknown patient")
	assert.ErrorIs(t, try(syd(2027, 3, 8, 10, 0), func(in *NewAppointment) { in.Note = strings.Repeat("x", 281) }), kit.ErrInvalid, "note")

	h, err := f.svc.History(ctx, f.org, a.ID)
	require.NoError(t, err)
	require.Len(t, h, 1)
	assert.Nil(t, h[0].FromStatus)
	assert.Equal(t, StatusBooked, h[0].ToStatus)
	require.NotNil(t, h[0].ChangedBy)
	assert.Equal(t, *f.staff.UserID, h[0].ChangedBy.ID)
}

func TestCreate_PatientsCantOverrideAndNeedNotice(t *testing.T) {
	f := newFx(t)
	web := Actor{Source: SourceWeb}
	in := NewAppointment{ContactID: f.patient, ServiceID: f.consult, LocationID: f.bondi, PractitionerID: f.ada, StartsAt: syd(2027, 3, 1, 10, 0)}
	_, err := f.svc.Create(ctx, f.org, web, in)
	assert.ErrorIs(t, err, kit.ErrInvalid, "less than 2 hours' notice")
	in.StartsAt, in.OutsideHours = syd(2027, 3, 8, 13, 0), true
	_, err = f.svc.Create(ctx, f.org, web, in)
	assert.ErrorIs(t, err, kit.ErrForbidden)
	in.OutsideHours = false
	in.StartsAt = syd(2027, 3, 8, 11, 0)
	a, err := f.svc.Create(ctx, f.org, web, in)
	require.NoError(t, err)
	assert.Equal(t, SourceWeb, a.Source)
}

func TestCreate_PatientOverlap(t *testing.T) {
	f := newFx(t)
	f.book(t, f.ada, syd(2027, 3, 8, 9, 0))
	in := NewAppointment{ContactID: f.patient, ServiceID: f.consult, LocationID: f.bondi, PractitionerID: f.ben, StartsAt: syd(2027, 3, 8, 9, 0)}
	_, err := f.svc.Create(ctx, f.org, Actor{Source: SourceWeb}, in)
	assert.ErrorIs(t, err, kit.ErrConflict, "web: refused")
	_, err = f.svc.Create(ctx, f.org, f.staff, in)
	assert.ErrorIs(t, err, kit.ErrConflict, "staff: warned")
	in.AllowPatientOverlap = true
	_, err = f.svc.Create(ctx, f.org, f.staff, in)
	assert.NoError(t, err, "staff: overridden")
}

// R8: ten requests race for one slot; exactly one wins, whatever the service
// check saw.
func TestCreate_ParallelRequestsBookOnce(t *testing.T) {
	f := newFx(t)
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		contact := testutil.CreateTestContact(t, f.db, f.org).ID
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Create(ctx, f.org, f.staff, NewAppointment{ContactID: contact, ServiceID: f.consult,
				LocationID: f.bondi, PractitionerID: f.ada, StartsAt: syd(2027, 3, 8, 9, 0)})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case assert.ErrorIs(t, err, kit.ErrConflict):
			conflicts++
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, 9, conflicts)
}

func TestStatus_MachineAndHistory(t *testing.T) {
	f := newFx(t)
	a := f.book(t, f.ada, syd(2027, 3, 8, 9, 0))

	_, err := f.svc.SetStatus(ctx, f.org, f.staff, a.ID, StatusCompleted, "")
	require.ErrorIs(t, err, kit.ErrConflict)
	assert.Contains(t, err.Error(), "allowed: CONFIRMED, ARRIVED, CANCELLED, NO_SHOW")

	a, err = f.svc.SetStatus(ctx, f.org, f.staff, a.ID, StatusConfirmed, "")
	require.NoError(t, err)
	a, err = f.svc.SetStatus(ctx, f.org, f.staff, a.ID, StatusArrived, "")
	require.NoError(t, err)
	assert.Equal(t, []Status{StatusCompleted}, a.AllowedNext)
	a, err = f.svc.SetStatus(ctx, f.org, f.staff, a.ID, StatusCompleted, "")
	require.NoError(t, err)
	_, err = f.svc.SetStatus(ctx, f.org, f.staff, a.ID, StatusCancelled, "changed mind")
	assert.ErrorIs(t, err, kit.ErrConflict, "final")

	h, err := f.svc.History(ctx, f.org, a.ID)
	require.NoError(t, err)
	assert.Len(t, h, 4)
	assert.Equal(t, StatusArrived, *h[3].FromStatus)
	assert.Equal(t, StatusCompleted, h[3].ToStatus)
	assert.NotContains(t, f.slots(t, f.consult, &f.ada, monday), "09:00", "a completed appointment keeps its time")
}

func TestStatus_CancelNeedsReasonAndFreesTime(t *testing.T) {
	f := newFx(t)
	a := f.book(t, f.ada, syd(2027, 3, 8, 9, 0))
	_, err := f.svc.SetStatus(ctx, f.org, f.staff, a.ID, StatusCancelled, " ")
	assert.ErrorIs(t, err, kit.ErrInvalid)
	_, err = f.svc.SetStatus(ctx, f.org, f.staff, a.ID, StatusCancelled, "Patient unwell")
	require.NoError(t, err)
	assert.Contains(t, f.slots(t, f.consult, &f.ada, monday), "09:00")
	h, _ := f.svc.History(ctx, f.org, a.ID)
	assert.Equal(t, "Patient unwell", h[len(h)-1].Reason)
	f.book(t, f.ada, syd(2027, 3, 8, 9, 0)) // rebookable
}

func TestMove(t *testing.T) {
	f := newFx(t)
	a := f.book(t, f.ada, syd(2027, 3, 8, 9, 0))
	f.book(t, f.ada, syd(2027, 3, 8, 9, 30))

	_, err := f.svc.Move(ctx, f.org, f.staff, a.ID, MoveInput{StartsAt: ptr(syd(2027, 3, 8, 9, 30))})
	assert.ErrorIs(t, err, kit.ErrConflict, "clash")
	moved, err := f.svc.Move(ctx, f.org, f.staff, a.ID, MoveInput{StartsAt: ptr(syd(2027, 3, 8, 9, 15))})
	require.NoError(t, err, "moving next to itself doesn't clash with its old time")
	assert.Equal(t, "2027-03-08T09:15", moved.Local)
	moved, err = f.svc.Move(ctx, f.org, f.staff, a.ID, MoveInput{PractitionerID: &f.ben})
	require.NoError(t, err)
	assert.Equal(t, "Dr Ben", moved.Practitioner.Name)

	h, _ := f.svc.History(ctx, f.org, a.ID)
	require.Len(t, h, 3)
	assert.Contains(t, h[1].Changes, "starts_at")
	assert.Contains(t, h[2].Changes, "practitioner_id")

	_, err = f.svc.SetStatus(ctx, f.org, f.staff, a.ID, StatusArrived, "")
	require.NoError(t, err)
	_, err = f.svc.Move(ctx, f.org, f.staff, a.ID, MoveInput{StartsAt: ptr(syd(2027, 3, 8, 9, 45))})
	assert.ErrorIs(t, err, kit.ErrConflict, "an arrived appointment can't move")
	_, err = f.svc.Move(ctx, f.org, f.staff, a.ID, MoveInput{Note: ptr("Bring referral")})
	assert.NoError(t, err, "but its note can change")
}

func TestList(t *testing.T) {
	f := newFx(t)
	f.book(t, f.ben, syd(2027, 3, 8, 9, 15))
	f.book(t, f.ada, syd(2027, 3, 8, 9, 0))
	got, err := f.svc.List(ctx, f.org, ListQuery{From: date(monday)})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Dr Ada", got[0].Practitioner.Name, "by time")
	got, err = f.svc.List(ctx, f.org, ListQuery{From: date(monday), PractitionerID: &f.ben})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	got, err = f.svc.List(ctx, f.org, ListQuery{From: date("2027-03-09")})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// ---------- organization isolation ----------

func TestCrossOrganizationRefused(t *testing.T) {
	f := newFx(t)
	other := newFx(t)
	a := f.book(t, f.ada, syd(2027, 3, 8, 9, 0))

	_, err := other.svc.Get(ctx, other.org, a.ID)
	assert.ErrorIs(t, err, kit.ErrNotFound)
	_, err = other.svc.SetStatus(ctx, other.org, other.staff, a.ID, StatusCancelled, "x")
	assert.ErrorIs(t, err, kit.ErrNotFound)
	_, err = other.svc.Create(ctx, other.org, other.staff, NewAppointment{ContactID: other.patient, ServiceID: other.consult,
		LocationID: other.bondi, PractitionerID: f.ada, StartsAt: syd(2027, 3, 8, 10, 0)})
	assert.ErrorIs(t, err, kit.ErrNotFound, "another org's practitioner")
	_, err = other.svc.Create(ctx, other.org, other.staff, NewAppointment{ContactID: f.patient, ServiceID: other.consult,
		LocationID: other.bondi, PractitionerID: other.ada, StartsAt: syd(2027, 3, 8, 10, 0)})
	assert.ErrorIs(t, err, kit.ErrNotFound, "another org's patient")
	_, _, err = other.svc.SetAvailability(ctx, other.org, f.ada, nil)
	assert.ErrorIs(t, err, kit.ErrNotFound)
	got, err := other.svc.List(ctx, other.org, ListQuery{From: date(monday)})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// ---------- patients and settings ----------

func TestPatients(t *testing.T) {
	f := newFx(t)
	_, _, err := f.svc.FindOrCreatePatient(ctx, f.org, "Pat Lee", "0412 345 678")
	assert.ErrorIs(t, err, kit.ErrConflict, "no WhatsApp account yet")

	testutil.CreateTestWhatsAppAccount(t, f.db, f.org)
	p, existing, err := f.svc.FindOrCreatePatient(ctx, f.org, "Pat Lee", "0412 345 678")
	require.NoError(t, err)
	assert.False(t, existing)
	assert.Equal(t, "61412345678", p.Phone)
	again, existing, err := f.svc.FindOrCreatePatient(ctx, f.org, "Someone else", "+61 412 345 678")
	require.NoError(t, err)
	assert.True(t, existing)
	assert.Equal(t, p.ID, again.ID)
	assert.Equal(t, "Pat Lee", again.Name, "an existing contact keeps its name")

	byName, err := f.svc.SearchPatients(ctx, f.org, "pat")
	require.NoError(t, err)
	assert.Len(t, byName, 1)
	byPhone, err := f.svc.SearchPatients(ctx, f.org, "0412 345")
	require.NoError(t, err)
	assert.Len(t, byPhone, 1)
	_, err = f.svc.SearchPatients(ctx, f.org, "pa")
	assert.ErrorIs(t, err, kit.ErrInvalid)

	found, err := f.svc.PatientByPhone(ctx, f.org, "61412345678")
	require.NoError(t, err)
	assert.Equal(t, p.ID, found.ID)
	other := newFx(t)
	_, err = other.svc.PatientByPhone(ctx, other.org, "61412345678")
	assert.ErrorIs(t, err, kit.ErrNotFound, "scoped to the org")
	none, err := other.svc.SearchPatients(ctx, other.org, "Pat Lee")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestSettings(t *testing.T) {
	f := newFx(t)
	s, err := f.svc.GetSettings(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, DefaultMinNoticeMinutes, s.MinNoticeMinutes)
	assert.Equal(t, DefaultWindowDays, s.WindowDays)
	_, err = f.svc.SaveSettings(ctx, f.org, 60, 30)
	require.NoError(t, err)
	_, err = f.svc.SaveSettings(ctx, f.org, 60, 30) // upsert
	require.NoError(t, err)
	s, _ = f.svc.GetSettings(ctx, f.org)
	assert.Equal(t, 30, s.WindowDays)
	_, err = f.svc.SaveSettings(ctx, f.org, -1, 30)
	assert.ErrorIs(t, err, kit.ErrInvalid)
	_, err = f.svc.SaveSettings(ctx, f.org, 60, 400)
	assert.ErrorIs(t, err, kit.ErrInvalid)
}

func TestNotSetUp(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	_, err := NewService(db).SearchSlots(ctx, org.ID, SlotQuery{ServiceID: uuid.New(), LocationID: uuid.New(), From: date(monday)})
	assert.ErrorIs(t, err, kit.ErrConflict)
}
