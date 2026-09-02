package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Scheduling is a donation centre's booking configuration, as the domain sees
// it. The database columns are on `donation_centers` (schema §6.2).
type Scheduling struct {
	SlotMinutes  int
	CapacityPer  int
	Location     *time.Location
	OpeningHours OpeningHours
}

var (
	ErrCentreClosed      = errors.New("the centre is not open then")
	ErrSlotOutOfHours    = errors.New("that time is outside the centre's opening hours")
	ErrInvalidSlotLength = errors.New("slot length must be between 5 and 240 minutes")
	ErrInvalidHours      = errors.New("opening hours are not in the expected shape")
)

// OpeningHours is a centre's week, keyed by lowercase three-letter weekday.
//
// The shape of `donation_centers.opening_hours`, which the schema declares as
// JSONB and — until now — left undefined:
//
//	{"mon": [["08:00","12:00"], ["13:00","16:30"]], "sat": [["08:00","12:00"]]}
//
// A LIST of intervals per day, not one pair, because a centre that closes for
// lunch is the ordinary case and a single open/close would either book donors
// into the break or shorten the day to avoid it. **A day with no entry is
// closed** — absence means shut, so a centre that has configured nothing takes
// no bookings by that route, rather than being treated as open around the clock.
type OpeningHours map[string][]Interval

// Interval is a half-open span of the local clock: open at Start, shut at End.
type Interval struct {
	Start Clock
	End   Clock
}

// Clock is a wall-clock time of day in minutes from local midnight.
//
// Minutes-from-midnight rather than a `time.Time`, because opening hours are a
// property of the CENTRE and not of any particular date: "we open at 08:00" is
// true on the day the clocks change too, and attaching it to a date would make
// it silently shift by an hour twice a year.
type Clock int

func (c Clock) String() string { return fmt.Sprintf("%02d:%02d", int(c)/60, int(c)%60) }

// ParseClock reads "HH:MM".
func ParseClock(s string) (Clock, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not HH:MM", ErrInvalidHours, s)
	}
	return Clock(t.Hour()*60 + t.Minute()), nil
}

var weekdayKeys = map[time.Weekday]string{
	time.Monday: "mon", time.Tuesday: "tue", time.Wednesday: "wed",
	time.Thursday: "thu", time.Friday: "fri", time.Saturday: "sat", time.Sunday: "sun",
}

// ParseOpeningHours reads the JSONB column.
//
// An empty or absent object is NOT an error and NOT "closed all week": it is a
// centre that has not configured hours, and `Configured` reports false so a
// caller can fall back rather than turning every donor away. Distinguishing
// "shut on Sunday" from "nobody has filled this in yet" matters, because the
// second is an administrative gap and the first is a fact about the centre.
func ParseOpeningHours(raw []byte) (OpeningHours, error) {
	if len(raw) == 0 {
		return OpeningHours{}, nil
	}
	var byDay map[string][][]string
	if err := json.Unmarshal(raw, &byDay); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidHours, err)
	}

	out := make(OpeningHours, len(byDay))
	for day, spans := range byDay {
		key := strings.ToLower(strings.TrimSpace(day))
		if !knownDay(key) {
			return nil, fmt.Errorf("%w: %q is not a weekday key (mon…sun)", ErrInvalidHours, day)
		}
		for _, span := range spans {
			if len(span) != 2 {
				return nil, fmt.Errorf("%w: %s has an interval that is not [open, close]", ErrInvalidHours, key)
			}
			start, err := ParseClock(span[0])
			if err != nil {
				return nil, err
			}
			end, err := ParseClock(span[1])
			if err != nil {
				return nil, err
			}
			if end <= start {
				return nil, fmt.Errorf("%w: %s closes at %s, before it opens at %s", ErrInvalidHours, key, end, start)
			}
			out[key] = append(out[key], Interval{Start: start, End: end})
		}
		sort.Slice(out[key], func(i, j int) bool { return out[key][i].Start < out[key][j].Start })

		// Overlapping intervals would make the same minute belong to two spans,
		// and the slot grid is measured from an interval's start — so a donor
		// could be offered two different "first slots" for one morning.
		for i := 1; i < len(out[key]); i++ {
			if out[key][i].Start < out[key][i-1].End {
				return nil, fmt.Errorf("%w: %s has overlapping intervals", ErrInvalidHours, key)
			}
		}
	}
	return out, nil
}

func knownDay(key string) bool {
	for _, k := range weekdayKeys {
		if k == key {
			return true
		}
	}
	return false
}

// Configured reports whether anybody has set hours for this centre.
func (h OpeningHours) Configured() bool { return len(h) > 0 }

// On returns the intervals for the weekday of t, in the centre's location.
func (h OpeningHours) On(t time.Time) []Interval { return h[weekdayKeys[t.Weekday()]] }

// SlotStart snaps a requested time DOWN to the centre's slot grid.
//
// The grid is measured from the start of the opening interval the time falls
// in, not from midnight. That distinction is real: a centre opening at 08:15
// with 30-minute slots has slots at 08:15 and 08:45, and a grid measured from
// midnight would offer 08:00 — fifteen minutes before anyone is there.
//
// Snapping DOWN rather than to the nearest boundary means a request for 09:29
// books the 09:00 slot, which is the slot that request was for. Rounding up
// would quietly move a donor to a later appointment than they asked for.
//
// Requests outside every interval are refused rather than snapped to the nearest
// open slot: a booking at a time the centre is shut is a mistake somewhere, and
// silently relocating it hides the mistake from whoever made it.
func (s Scheduling) SlotStart(t time.Time) (time.Time, error) {
	if s.SlotMinutes < 5 || s.SlotMinutes > 240 {
		return time.Time{}, fmt.Errorf("%w: got %d", ErrInvalidSlotLength, s.SlotMinutes)
	}
	local := t.In(s.Location)
	minutes := Clock(local.Hour()*60 + local.Minute())

	for _, iv := range s.OpeningHours.On(local) {
		if minutes < iv.Start || minutes >= iv.End {
			continue
		}
		offset := int(minutes-iv.Start) / s.SlotMinutes * s.SlotMinutes
		start := int(iv.Start) + offset
		// A slot that would run past closing is not a slot. The last one starts
		// early enough to finish.
		if start+s.SlotMinutes > int(iv.End) {
			return time.Time{}, fmt.Errorf("%w: the last %d-minute slot before %s has already started",
				ErrSlotOutOfHours, s.SlotMinutes, iv.End)
		}
		return atClock(local, Clock(start)), nil
	}
	return time.Time{}, fmt.Errorf("%w: %s", ErrSlotOutOfHours, local.Format("Monday 15:04"))
}

// FirstSlotOn is the first bookable slot of a given day, for a caller who named
// a date and no time.
func (s Scheduling) FirstSlotOn(day time.Time) (time.Time, error) {
	if s.SlotMinutes < 5 || s.SlotMinutes > 240 {
		return time.Time{}, fmt.Errorf("%w: got %d", ErrInvalidSlotLength, s.SlotMinutes)
	}
	local := day.In(s.Location)
	for _, iv := range s.OpeningHours.On(local) {
		if int(iv.Start)+s.SlotMinutes <= int(iv.End) {
			return atClock(local, iv.Start), nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: %s", ErrCentreClosed, local.Format("Monday 2 January"))
}

// SlotsOn lists every bookable slot start on a day. For a booking UI, and for
// the capacity view.
func (s Scheduling) SlotsOn(day time.Time) []time.Time {
	local := day.In(s.Location)
	var out []time.Time
	for _, iv := range s.OpeningHours.On(local) {
		for start := int(iv.Start); start+s.SlotMinutes <= int(iv.End); start += s.SlotMinutes {
			out = append(out, atClock(local, Clock(start)))
		}
	}
	return out
}

// atClock rebuilds an instant at a wall-clock time on the same local day.
//
// `time.Date` resolves the daylight-saving cases for us: on a spring-forward day
// a time inside the skipped hour normalises forward, and on a fall-back day the
// first of the two occurrences is chosen. Either is a defensible answer, and
// both are better than arithmetic on an instant, which would put the slot an
// hour away from the wall clock the centre actually runs on.
func atClock(local time.Time, c Clock) time.Time {
	y, m, d := local.Date()
	return time.Date(y, m, d, int(c)/60, int(c)%60, 0, 0, local.Location())
}
