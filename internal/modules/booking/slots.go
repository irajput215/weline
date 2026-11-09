package booking

import (
	"sort"
	"time"
)

// The slot engine is pure: it takes everything already loaded and computes free
// start times, so it can be tested without a database (design.md, "Slot search").

// span is a half-open instant interval [Start, End).
type span struct{ Start, End time.Time }

func (a span) overlaps(b span) bool { return a.Start.Before(b.End) && b.Start.Before(a.End) }

// minutes is a half-open local wall-clock range in minutes after midnight.
type minutes struct{ Start, End int }

// intersect returns the overlap of two minute ranges, ok=false when empty.
func intersect(a, b minutes) (minutes, bool) {
	s, e := max(a.Start, b.Start), min(a.End, b.End)
	return minutes{s, e}, e > s
}

// wallClock returns the instant of a local date and minute in tz, and false
// when that wall-clock time doesn't exist (the hour skipped when daylight
// saving starts). Minute 1440 is midnight at the end of the day.
func wallClock(day time.Time, minute int, tz *time.Location) (time.Time, bool) {
	t := time.Date(day.Year(), day.Month(), day.Day(), minute/60, minute%60, 0, 0, tz)
	if minute == 1440 {
		return t, true
	}
	return t, t.Hour() == minute/60 && t.Minute() == minute%60
}

// isoWeekday is 1 = Monday … 7 = Sunday.
func isoWeekday(t time.Time) int {
	if w := int(t.Weekday()); w != 0 {
		return w
	}
	return 7
}

// dayPlan is one practitioner's working time on one local date.
type dayPlan struct {
	Day     time.Time // local midnight (date only)
	Working []minutes // availability ∩ opening hours, in local minutes
}

// freeStarts returns the start instants on the slot grid where a service of
// the given length fits inside a working range and overlaps nothing busy, and
// is not before earliest nor after latest.
func freeStarts(p dayPlan, tz *time.Location, grid, duration int, busy []span, earliest, latest time.Time) []time.Time {
	var out []time.Time
	seen := map[int64]bool{}
	for _, w := range p.Working {
		first := (w.Start + grid - 1) / grid * grid // round up to the grid
		for m := first; m+duration <= w.End; m += grid {
			start, ok := wallClock(p.Day, m, tz)
			if !ok {
				continue // skipped hour (daylight saving starts)
			}
			// The end must still fall inside the working range in real time:
			// on a short day an instant duration can run past the local end.
			endLimit, _ := wallClock(p.Day, w.End, tz)
			s := span{start, start.Add(time.Duration(duration) * time.Minute)}
			if s.End.After(endLimit) || start.Before(earliest) || start.After(latest) {
				continue
			}
			if seen[start.Unix()] {
				continue // repeated hour (daylight saving ends): offer it once
			}
			free := true
			for _, b := range busy {
				if s.overlaps(b) {
					free = false
					break
				}
			}
			if free {
				seen[start.Unix()] = true
				out = append(out, start)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}
