package practice

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAPI_ForOtherModules(t *testing.T) {
	f := newFx(t)
	p := f.withProfile(t, 15, DefaultTimeZone)
	a, b := f.location(t, "A"), f.location(t, "B")

	got, err := f.svc.ProfileBySlug(ctx, p.BookingSlug)
	require.NoError(t, err)
	assert.Equal(t, f.org, got.OrganizationID)
	_, err = f.svc.ProfileBySlug(ctx, "no-such-clinic")
	assert.True(t, IsNotSetUp(err))

	_, err = f.svc.SetHours(ctx, f.org, a.ID, []Hours{{Weekday: 1, OpensAt: "08:30", ClosesAt: "17:00"}})
	require.NoError(t, err)
	hours, err := f.svc.OpeningMinutes(ctx, f.org, a.ID)
	require.NoError(t, err)
	assert.Equal(t, map[int]MinuteRange{1: {510, 1020}}, hours)

	for _, c := range []ClosureInput{
		{LocationID: &a.ID, StartsOn: "2027-04-02", EndsOn: "2027-04-05", Reason: "Easter"},
		{StartsOn: "2027-04-25", EndsOn: "2027-04-25", Reason: "Anzac Day"},
		{LocationID: &b.ID, StartsOn: "2027-04-10", EndsOn: "2027-04-10", Reason: "B only"},
	} {
		_, err := f.svc.CreateClosure(ctx, f.org, c)
		require.NoError(t, err)
	}
	from, _ := ParseDate("2027-04-04")
	to, _ := ParseDate("2027-04-30")
	closed, err := f.svc.ClosedDays(ctx, f.org, a.ID, from, to)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"2027-04-04": true, "2027-04-05": true, "2027-04-25": true}, closed,
		"clipped to the range; all-locations closures count; other locations' don't")

	other := newFx(t)
	_, err = other.svc.GetLocation(ctx, other.org, a.ID)
	assert.Error(t, err, "scoped to the org")
}
