package booking

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	tz, err := time.LoadLocation(name)
	require.NoError(t, err)
	return tz
}

func localDay(tz *time.Location, y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, tz)
}

func clock(ts []time.Time, tz *time.Location) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.In(tz).Format("15:04")
	}
	return out
}

var (
	never  = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	future = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
)

func TestIntersect(t *testing.T) {
	got, ok := intersect(minutes{480, 720}, minutes{540, 1020})
	assert.True(t, ok)
	assert.Equal(t, minutes{540, 720}, got)
	_, ok = intersect(minutes{480, 540}, minutes{540, 600})
	assert.False(t, ok, "touching ranges don't overlap")
}

func TestFreeStarts_GridAndDuration(t *testing.T) {
	tz := zone(t, "Australia/Sydney")
	day := localDay(tz, 2027, 3, 8)
	plan := dayPlan{Day: day, Working: []minutes{{9 * 60, 10 * 60}}}

	assert.Equal(t, []string{"09:00", "09:15", "09:30", "09:45"}, clock(freeStarts(plan, tz, 15, 15, nil, never, future), tz))
	assert.Equal(t, []string{"09:00", "09:15", "09:30"}, clock(freeStarts(plan, tz, 15, 30, nil, never, future), tz),
		"a 30-minute service needs 30 free minutes")

	// A working range that doesn't start on the grid is rounded up.
	plan.Working = []minutes{{9*60 + 5, 10 * 60}}
	assert.Equal(t, []string{"09:15", "09:30", "09:45"}, clock(freeStarts(plan, tz, 15, 15, nil, never, future), tz))
}

func TestFreeStarts_BusyEarliestLatest(t *testing.T) {
	tz := zone(t, "Australia/Sydney")
	day := localDay(tz, 2027, 3, 8)
	plan := dayPlan{Day: day, Working: []minutes{{9 * 60, 10 * 60}}}
	at := func(h, m int) time.Time { return time.Date(2027, 3, 8, h, m, 0, 0, tz) }

	busy := []span{{at(9, 15), at(9, 30)}}
	assert.Equal(t, []string{"09:00", "09:30", "09:45"}, clock(freeStarts(plan, tz, 15, 15, busy, never, future), tz))
	assert.Equal(t, []string{"09:30"}, clock(freeStarts(plan, tz, 15, 30, busy, never, future), tz),
		"09:00 for 30 minutes runs into the busy 09:15; 09:45 runs past 10:00")

	assert.Equal(t, []string{"09:30", "09:45"}, clock(freeStarts(plan, tz, 15, 15, nil, at(9, 30), future), tz), "earliest")
	assert.Equal(t, []string{"09:00", "09:15"}, clock(freeStarts(plan, tz, 15, 15, nil, never, at(9, 15)), tz), "latest")
}

// Sydney skips 02:00–02:59 on the first Sunday of October.
func TestFreeStarts_SpringForwardSkipsTheMissingHour(t *testing.T) {
	tz := zone(t, "Australia/Sydney")
	day := localDay(tz, 2026, 10, 4)
	plan := dayPlan{Day: day, Working: []minutes{{60, 4 * 60}}}

	got := clock(freeStarts(plan, tz, 15, 15, nil, never, future), tz)
	assert.Equal(t, []string{"01:00", "01:15", "01:30", "01:45", "03:00", "03:15", "03:30", "03:45"}, got)
}

// Sydney repeats 02:00–02:59 on the first Sunday of April: each wall-clock
// start is offered once.
func TestFreeStarts_FallBackOffersTheRepeatedHourOnce(t *testing.T) {
	tz := zone(t, "Australia/Sydney")
	day := localDay(tz, 2027, 4, 4)
	plan := dayPlan{Day: day, Working: []minutes{{60, 4 * 60}}}

	got := freeStarts(plan, tz, 15, 15, nil, never, future)
	labels := clock(got, tz)
	assert.Len(t, got, 12, "01:00 to 03:45 on the 15-minute grid")
	assert.Contains(t, labels, "02:00")
	seen := map[string]bool{}
	for _, l := range labels {
		assert.False(t, seen[l], "offered twice: "+l)
		seen[l] = true
	}
}

func TestFreeStarts_BrisbaneAndPerthHaveNoDST(t *testing.T) {
	for _, name := range []string{"Australia/Brisbane", "Australia/Perth"} {
		tz := zone(t, name)
		plan := dayPlan{Day: localDay(tz, 2026, 10, 4), Working: []minutes{{60, 4 * 60}}}
		assert.Len(t, freeStarts(plan, tz, 15, 15, nil, never, future), 12, name)
	}
}

func TestStatusMachine(t *testing.T) {
	allowed := map[Status][]Status{
		StatusBooked:    {StatusConfirmed, StatusArrived, StatusCancelled, StatusNoShow},
		StatusConfirmed: {StatusArrived, StatusCancelled, StatusNoShow},
		StatusArrived:   {StatusCompleted},
	}
	all := []Status{StatusBooked, StatusConfirmed, StatusArrived, StatusCompleted, StatusCancelled, StatusNoShow}
	for _, from := range all {
		for _, to := range all {
			want := false
			for _, a := range allowed[from] {
				if a == to {
					want = true
				}
			}
			assert.Equal(t, want, from.CanBecome(to), "%s → %s", from, to)
		}
	}
	for _, terminal := range []Status{StatusCompleted, StatusCancelled, StatusNoShow} {
		assert.Empty(t, terminal.Next(), "%s is final", terminal)
	}
	assert.True(t, StatusConfirmed.movable())
	assert.False(t, StatusArrived.movable())
}

func TestNormaliseMobile(t *testing.T) {
	for _, in := range []string{"0412 345 678", "+61 412 345 678", "61412345678", "(04) 1234-5678"} {
		got, err := normaliseMobile(in)
		require.NoError(t, err, in)
		assert.Equal(t, "61412345678", got, in)
	}
	for _, in := range []string{"02 9876 5432", "0412 345", "+1 415 555 0100", "", "61212345678"} {
		_, err := normaliseMobile(in)
		assert.Error(t, err, in)
	}
}

func TestReferences(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		r := newReference()
		assert.Len(t, r, 6)
		assert.NotContains(t, r, "0")
		assert.NotContains(t, r, "O")
		assert.NotContains(t, r, "I")
		seen[r] = true
	}
	assert.Greater(t, len(seen), 495, "practically unique")
}
