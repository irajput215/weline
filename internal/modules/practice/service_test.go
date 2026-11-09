package practice

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var ctx = context.Background()

func ptr[T any](v T) *T { return &v }

// uniqueSlug avoids clashes between tests sharing the database.
func uniqueSlug() string { return "c" + strings.ReplaceAll(uuid.NewString()[:8], "-", "") }

type fx struct {
	db  *gorm.DB
	svc *Service
	org uuid.UUID
}

func newFx(t *testing.T) *fx {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	return &fx{db: db, svc: NewService(db), org: org.ID}
}

// withProfile saves a profile with the given slot length and time zone.
func (f *fx) withProfile(t *testing.T, slot int, tz string) *Profile {
	p, err := f.svc.SaveProfile(ctx, f.org, ProfileInput{Name: "Harbour Clinic", TimeZone: tz, BookingSlug: uniqueSlug(), SlotMinutes: slot})
	require.NoError(t, err)
	return p
}

func (f *fx) location(t *testing.T, name string) *Location {
	l, err := f.svc.CreateLocation(ctx, f.org, LocationInput{Name: name})
	require.NoError(t, err)
	return l
}

// ---------- profile (R1-R5) ----------

func TestProfile_CreateThenUpdate(t *testing.T) {
	f := newFx(t)
	_, err := f.svc.GetProfile(ctx, f.org)
	assert.True(t, IsNotSetUp(err))

	slug := uniqueSlug()
	p, err := f.svc.SaveProfile(ctx, f.org, ProfileInput{Name: " Harbour Clinic ", BookingSlug: slug, ABN: "51 824 753 556"})
	require.NoError(t, err)
	assert.Equal(t, "Harbour Clinic", p.Name)
	assert.Equal(t, DefaultTimeZone, p.TimeZone, "default zone")
	assert.Equal(t, DefaultSlotMinutes, p.SlotMinutes, "default slot")
	assert.Equal(t, "51824753556", p.ABN, "spaces removed")

	p2, err := f.svc.SaveProfile(ctx, f.org, ProfileInput{Name: "Harbour Clinic North", TimeZone: "Australia/Perth", BookingSlug: slug, SlotMinutes: 15})
	require.NoError(t, err)
	assert.Equal(t, p.ID, p2.ID, "second save updates the same row")
	assert.Equal(t, "Australia/Perth", p2.TimeZone)
}

func TestProfile_Validation(t *testing.T) {
	f := newFx(t)
	other := newFx(t)
	taken := uniqueSlug()
	_, err := other.svc.SaveProfile(ctx, other.org, ProfileInput{Name: "Other", BookingSlug: taken})
	require.NoError(t, err)

	cases := map[string]struct {
		in   ProfileInput
		kind error
	}{
		"no name":            {ProfileInput{BookingSlug: uniqueSlug()}, kit.ErrInvalid},
		"zone outside AU":    {ProfileInput{Name: "x", TimeZone: "Europe/London", BookingSlug: uniqueSlug()}, kit.ErrInvalid},
		"slug too short":     {ProfileInput{Name: "x", BookingSlug: "ab"}, kit.ErrInvalid},
		"slug uppercase":     {ProfileInput{Name: "x", BookingSlug: "Harbour"}, kit.ErrInvalid},
		"slug space":         {ProfileInput{Name: "x", BookingSlug: "har bour"}, kit.ErrInvalid},
		"slug double hyphen": {ProfileInput{Name: "x", BookingSlug: "har--bour"}, kit.ErrInvalid},
		"slug reserved":      {ProfileInput{Name: "x", BookingSlug: "admin"}, kit.ErrInvalid},
		"slug taken":         {ProfileInput{Name: "x", BookingSlug: taken}, kit.ErrConflict},
		"slot 7":             {ProfileInput{Name: "x", BookingSlug: uniqueSlug(), SlotMinutes: 7}, kit.ErrInvalid},
		"ABN checksum":       {ProfileInput{Name: "x", BookingSlug: uniqueSlug(), ABN: "51824753557"}, kit.ErrInvalid},
		"ABN length":         {ProfileInput{Name: "x", BookingSlug: uniqueSlug(), ABN: "5182475355"}, kit.ErrInvalid},
		"email":              {ProfileInput{Name: "x", BookingSlug: uniqueSlug(), Email: "not-an-email"}, kit.ErrInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.svc.SaveProfile(ctx, f.org, tc.in)
			assert.ErrorIs(t, err, tc.kind)
		})
	}
}

func TestProfile_SlotChangeRefusedWhenOffGrid(t *testing.T) {
	f := newFx(t)
	p := f.withProfile(t, 15, DefaultTimeZone)
	loc := f.location(t, "Main")
	_, _, err := f.svc.CreateService(ctx, f.org, ServiceInput{Name: "Standard consult", DurationMinutes: 15, AllowedRoles: []string{"GP"}})
	require.NoError(t, err)
	_, err = f.svc.SetHours(ctx, f.org, loc.ID, []Hours{{Weekday: 1, OpensAt: "08:15", ClosesAt: "17:00"}})
	require.NoError(t, err)

	_, err = f.svc.SaveProfile(ctx, f.org, ProfileInput{Name: p.Name, BookingSlug: p.BookingSlug, SlotMinutes: 30})
	require.ErrorIs(t, err, kit.ErrConflict)
	assert.Contains(t, err.Error(), "Standard consult")
	assert.Contains(t, err.Error(), "Main")

	_, err = f.svc.SaveProfile(ctx, f.org, ProfileInput{Name: p.Name, BookingSlug: p.BookingSlug, SlotMinutes: 5})
	assert.NoError(t, err, "a finer grid fits everything")
}

func TestABN(t *testing.T) {
	assert.True(t, validABN("51824753556"), "ATO example")
	assert.False(t, validABN("51824753557"))
	assert.False(t, validABN("5182475355a"))
}

// ---------- locations (R6, R7) ----------

func TestLocations(t *testing.T) {
	f := newFx(t)
	l := f.location(t, "Bondi")
	_, err := f.svc.CreateLocation(ctx, f.org, LocationInput{Name: "Bondi"})
	assert.ErrorIs(t, err, kit.ErrConflict, "duplicate name")
	_, err = f.svc.CreateLocation(ctx, f.org, LocationInput{Name: "  "})
	assert.ErrorIs(t, err, kit.ErrInvalid)

	renamed, err := f.svc.UpdateLocation(ctx, f.org, l.ID, LocationPatch{Name: ptr("Bondi Junction"), Phone: ptr("02 9000 0000")})
	require.NoError(t, err)
	assert.Equal(t, "Bondi Junction", renamed.Name)

	_, err = f.svc.UpdateLocation(ctx, f.org, l.ID, LocationPatch{IsActive: ptr(false)})
	require.NoError(t, err)
	active, _ := f.svc.ListLocations(ctx, f.org, false)
	all, _ := f.svc.ListLocations(ctx, f.org, true)
	assert.Empty(t, active)
	assert.Len(t, all, 1)

	_, err = f.svc.UpdateLocation(ctx, f.org, l.ID, LocationPatch{IsActive: ptr(true)})
	require.NoError(t, err)
	active, _ = f.svc.ListLocations(ctx, f.org, false)
	assert.Len(t, active, 1, "reactivated")
}

// ---------- practitioners (R8-R11) ----------

func TestPractitioners(t *testing.T) {
	f := newFx(t)
	a, b := f.location(t, "A"), f.location(t, "B")
	member := testutil.CreateTestUser(t, f.db, f.org, testutil.WithFullName("Dr Jane Lee"))

	p, err := f.svc.CreatePractitioner(ctx, f.org, PractitionerInput{DisplayName: "Dr Jane Lee", Role: "GP", AHPRANumber: "med 0001234567",
		UserID: &member.ID, LocationIDs: []uuid.UUID{a.ID, b.ID, a.ID}})
	require.NoError(t, err)
	assert.Equal(t, "MED0001234567", p.AHPRANumber, "normalised")
	assert.Len(t, p.LocationIDs, 2, "duplicates collapsed")
	assert.Equal(t, "Dr Jane Lee", p.UserName)

	_, err = f.svc.CreatePractitioner(ctx, f.org, PractitionerInput{DisplayName: "Copy", Role: "GP", UserID: &member.ID, LocationIDs: []uuid.UUID{a.ID}})
	assert.ErrorIs(t, err, kit.ErrConflict, "one practitioner per user")

	_, err = f.svc.CreatePractitioner(ctx, f.org, PractitionerInput{DisplayName: "No loc", Role: "GP"})
	assert.ErrorIs(t, err, kit.ErrInvalid, "needs a location")
	_, err = f.svc.CreatePractitioner(ctx, f.org, PractitionerInput{DisplayName: "Bad AHPRA", Role: "GP", AHPRANumber: "12345", LocationIDs: []uuid.UUID{a.ID}})
	assert.ErrorIs(t, err, kit.ErrInvalid)

	updated, err := f.svc.UpdatePractitioner(ctx, f.org, p.ID, PractitionerPatch{UserID: Opt[uuid.UUID]{Set: true}, LocationIDs: &[]uuid.UUID{b.ID}})
	require.NoError(t, err)
	assert.Nil(t, updated.UserID, "unlinked")
	assert.Equal(t, []uuid.UUID{b.ID}, updated.LocationIDs)

	_, err = f.svc.UpdatePractitioner(ctx, f.org, p.ID, PractitionerPatch{IsActive: ptr(false)})
	require.NoError(t, err)
	active, _ := f.svc.ActivePractitioners(ctx, f.org)
	assert.Empty(t, active)
}

func TestPractitioners_CrossOrgRefused(t *testing.T) {
	f := newFx(t)
	other := newFx(t)
	mine := f.location(t, "Mine")
	theirs := other.location(t, "Theirs")
	outsider := testutil.CreateTestUser(t, other.db, other.org)

	_, err := f.svc.CreatePractitioner(ctx, f.org, PractitionerInput{DisplayName: "X", Role: "GP", LocationIDs: []uuid.UUID{mine.ID, theirs.ID}})
	assert.ErrorIs(t, err, kit.ErrNotFound, "another org's location")

	_, err = f.svc.CreatePractitioner(ctx, f.org, PractitionerInput{DisplayName: "X", Role: "GP", UserID: &outsider.ID, LocationIDs: []uuid.UUID{mine.ID}})
	assert.ErrorIs(t, err, kit.ErrNotFound, "another org's user")

	pr, err := other.svc.CreatePractitioner(ctx, other.org, PractitionerInput{DisplayName: "Theirs", Role: "GP", LocationIDs: []uuid.UUID{theirs.ID}})
	require.NoError(t, err)
	_, err = f.svc.UpdatePractitioner(ctx, f.org, pr.ID, PractitionerPatch{DisplayName: ptr("Stolen")})
	assert.ErrorIs(t, err, kit.ErrNotFound)

	members, err := f.svc.ListMembers(ctx, f.org)
	require.NoError(t, err)
	for _, m := range members {
		assert.NotEqual(t, outsider.ID, m.ID, "members list is org-scoped")
	}
}

// ---------- services (R12, R13) ----------

func TestServices(t *testing.T) {
	f := newFx(t)
	_, _, err := f.svc.CreateService(ctx, f.org, ServiceInput{Name: "Consult", DurationMinutes: 15, AllowedRoles: []string{"GP"}})
	assert.True(t, IsNotSetUp(err), "needs the slot grid")

	f.withProfile(t, 15, DefaultTimeZone)
	loc := f.location(t, "Main")
	_, err = f.svc.CreatePractitioner(ctx, f.org, PractitionerInput{DisplayName: "Dr A", Role: "GP", LocationIDs: []uuid.UUID{loc.ID}})
	require.NoError(t, err)

	sv, warnings, err := f.svc.CreateService(ctx, f.org, ServiceInput{Name: "Long consult", DurationMinutes: 30, PriceCents: ptr(8500), AllowedRoles: []string{"gp", "Nurse", "GP"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"gp", "Nurse"}, []string(sv.AllowedRoles), "case-insensitive de-dupe")
	assert.Equal(t, []string{`no practitioner has role "Nurse" yet`}, warnings)

	for name, in := range map[string]ServiceInput{
		"off grid":   {Name: "a", DurationMinutes: 20, AllowedRoles: []string{"GP"}},
		"too short":  {Name: "b", DurationMinutes: 5, AllowedRoles: []string{"GP"}},
		"too long":   {Name: "c", DurationMinutes: 495, AllowedRoles: []string{"GP"}},
		"negative":   {Name: "d", DurationMinutes: 15, PriceCents: ptr(-1), AllowedRoles: []string{"GP"}},
		"no roles":   {Name: "e", DurationMinutes: 15},
		"blank role": {Name: "f", DurationMinutes: 15, AllowedRoles: []string{" "}},
	} {
		_, _, err := f.svc.CreateService(ctx, f.org, in)
		assert.ErrorIs(t, err, kit.ErrInvalid, name)
	}
	_, _, err = f.svc.CreateService(ctx, f.org, ServiceInput{Name: "Long consult", DurationMinutes: 15, AllowedRoles: []string{"GP"}})
	assert.ErrorIs(t, err, kit.ErrConflict, "duplicate name")

	cleared, _, err := f.svc.UpdateService(ctx, f.org, sv.ID, ServicePatch{PriceCents: Opt[int]{Set: true}, IsActive: ptr(false)})
	require.NoError(t, err)
	assert.Nil(t, cleared.PriceCents, "price cleared with null")
	assert.False(t, cleared.IsActive)
}

// ---------- hours and closures (R14, R15) ----------

func TestHours(t *testing.T) {
	f := newFx(t)
	loc := f.location(t, "Main")
	_, err := f.svc.SetHours(ctx, f.org, loc.ID, []Hours{{Weekday: 1, OpensAt: "08:00", ClosesAt: "17:00"}})
	assert.True(t, IsNotSetUp(err), "needs the slot grid")

	f.withProfile(t, 15, DefaultTimeZone)
	got, err := f.svc.SetHours(ctx, f.org, loc.ID, []Hours{{Weekday: 3, OpensAt: "08:30", ClosesAt: "12:00"}, {Weekday: 1, OpensAt: "08:00", ClosesAt: "17:00"}})
	require.NoError(t, err)
	assert.Equal(t, []Hours{{1, "08:00", "17:00"}, {3, "08:30", "12:00"}}, got, "Monday first, other days closed")

	for name, h := range map[string][]Hours{
		"weekday 0":   {{Weekday: 0, OpensAt: "08:00", ClosesAt: "09:00"}},
		"twice":       {{Weekday: 2, OpensAt: "08:00", ClosesAt: "09:00"}, {Weekday: 2, OpensAt: "10:00", ClosesAt: "11:00"}},
		"reversed":    {{Weekday: 2, OpensAt: "17:00", ClosesAt: "08:00"}},
		"off grid":    {{Weekday: 2, OpensAt: "08:10", ClosesAt: "17:00"}},
		"bad format":  {{Weekday: 2, OpensAt: "8am", ClosesAt: "17:00"}},
		"after 24:00": {{Weekday: 2, OpensAt: "08:00", ClosesAt: "24:15"}},
	} {
		_, err := f.svc.SetHours(ctx, f.org, loc.ID, h)
		assert.ErrorIs(t, err, kit.ErrInvalid, name)
	}

	got, err = f.svc.SetHours(ctx, f.org, loc.ID, nil)
	require.NoError(t, err)
	assert.Empty(t, got, "empty week closes every day")
}

func TestClosures(t *testing.T) {
	f := newFx(t)
	loc := f.location(t, "Main")
	other := newFx(t)
	theirs := other.location(t, "Theirs")

	c, err := f.svc.CreateClosure(ctx, f.org, ClosureInput{LocationID: &loc.ID, StartsOn: "2026-12-25", EndsOn: "2026-12-26", Reason: "Christmas"})
	require.NoError(t, err)
	_, err = f.svc.CreateClosure(ctx, f.org, ClosureInput{StartsOn: "2027-01-01", EndsOn: "2027-01-01", Reason: "New Year"})
	require.NoError(t, err)

	for name, in := range map[string]ClosureInput{
		"backwards": {StartsOn: "2026-12-26", EndsOn: "2026-12-25", Reason: "x"},
		"bad date":  {StartsOn: "25/12/2026", EndsOn: "2026-12-25", Reason: "x"},
		"no reason": {StartsOn: "2026-12-25", EndsOn: "2026-12-25"},
	} {
		_, err := f.svc.CreateClosure(ctx, f.org, in)
		assert.ErrorIs(t, err, kit.ErrInvalid, name)
	}
	_, err = f.svc.CreateClosure(ctx, f.org, ClosureInput{LocationID: &theirs.ID, StartsOn: "2026-12-25", EndsOn: "2026-12-25", Reason: "x"})
	assert.ErrorIs(t, err, kit.ErrNotFound, "another org's location")

	from, _ := ParseDate("2026-12-01")
	list, err := f.svc.ListClosures(ctx, f.org, from)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "Christmas", list[0].Reason)

	assert.ErrorIs(t, other.svc.DeleteClosure(ctx, other.org, c.ID), kit.ErrNotFound, "other org can't delete it")
	require.NoError(t, f.svc.DeleteClosure(ctx, f.org, c.ID))
}

// ---------- IsOpen (R18) ----------

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestIsOpen(t *testing.T) {
	f := newFx(t)
	f.withProfile(t, 15, "Australia/Sydney")
	loc := f.location(t, "Main")
	var week []Hours
	for d := 1; d <= 6; d++ { // Monday to Saturday, closed Sunday
		week = append(week, Hours{Weekday: d, OpensAt: "09:00", ClosesAt: "17:00"})
	}
	_, err := f.svc.SetHours(ctx, f.org, loc.ID, week)
	require.NoError(t, err)
	_, err = f.svc.CreateClosure(ctx, f.org, ClosureInput{StartsOn: "2026-06-10", EndsOn: "2026-06-10", Reason: "Staff day"})
	require.NoError(t, err)

	cases := []struct {
		name string
		at   string
		open bool
	}{
		{"Mon 09:00 AEST (UTC Sunday 23:00) - weekday from local date", "2026-06-07T23:00:00Z", true},
		{"Mon 08:59 AEST", "2026-06-07T22:59:00Z", false},
		{"Mon 16:59 AEST", "2026-06-08T06:59:00Z", true},
		{"Mon 17:00 AEST closing time is closed", "2026-06-08T07:00:00Z", false},
		{"Sun 10:00 AEST closed weekday", "2026-06-07T00:00:00Z", false},
		{"Wed 10:00 AEST all-locations closure", "2026-06-10T00:00:00Z", false},
		{"Sat 3 Oct 09:30 AEST (before DST)", "2026-10-02T23:30:00Z", true},
		{"Mon 5 Oct 09:30 AEDT (after DST, +11)", "2026-10-04T22:30:00Z", true},
		{"Mon 5 Oct 08:30 AEDT", "2026-10-04T21:30:00Z", false},
	}
	for _, c := range cases {
		open, err := f.svc.IsOpen(ctx, f.org, loc.ID, utc(c.at))
		require.NoError(t, err, c.name)
		assert.Equal(t, c.open, open, c.name)
	}

	_, err = f.svc.UpdateLocation(ctx, f.org, loc.ID, LocationPatch{IsActive: ptr(false)})
	require.NoError(t, err)
	open, err := f.svc.IsOpen(ctx, f.org, loc.ID, utc("2026-06-08T00:00:00Z"))
	require.NoError(t, err)
	assert.False(t, open, "inactive location is never open")
}

func TestIsOpen_PerthHasNoDST(t *testing.T) {
	f := newFx(t)
	f.withProfile(t, 30, "Australia/Perth")
	loc := f.location(t, "Perth CBD")
	_, err := f.svc.SetHours(ctx, f.org, loc.ID, []Hours{{Weekday: 1, OpensAt: "09:00", ClosesAt: "17:00"}})
	require.NoError(t, err)

	for at, want := range map[string]bool{
		"2026-10-05T01:00:00Z": true,  // Mon 09:00 AWST (+8, unchanged by eastern DST)
		"2026-10-05T00:59:00Z": false, // Mon 08:59
		"2026-06-08T01:00:00Z": true,  // winter Mon 09:00
	} {
		open, err := f.svc.IsOpen(ctx, f.org, loc.ID, utc(at))
		require.NoError(t, err)
		assert.Equal(t, want, open, at)
	}
}

func TestSetupMissing(t *testing.T) {
	f := newFx(t)
	missing, err := f.svc.SetupMissing(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, []string{"profile", "locations", "hours", "practitioners", "services"}, missing)

	f.withProfile(t, 15, DefaultTimeZone)
	loc := f.location(t, "Main")
	_, err = f.svc.SetHours(ctx, f.org, loc.ID, []Hours{{Weekday: 1, OpensAt: "09:00", ClosesAt: "17:00"}})
	require.NoError(t, err)
	missing, err = f.svc.SetupMissing(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, []string{"practitioners", "services"}, missing)

	_, err = f.svc.CreatePractitioner(ctx, f.org, PractitionerInput{DisplayName: "Dr A", Role: "GP", LocationIDs: []uuid.UUID{loc.ID}})
	require.NoError(t, err)
	_, _, err = f.svc.CreateService(ctx, f.org, ServiceInput{Name: "Consult", DurationMinutes: 15, AllowedRoles: []string{"GP"}})
	require.NoError(t, err)
	missing, err = f.svc.SetupMissing(ctx, f.org)
	require.NoError(t, err)
	// An empty list, not nil: it goes to the page as [] rather than null.
	assert.NotNil(t, missing)
	assert.Empty(t, missing)
}
