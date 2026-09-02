package domain_test

import (
	"errors"
	"testing"
	"time"

	"bbank/internal/domain"
)

func douala(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Africa/Douala")
	if err != nil {
		t.Skipf("no tzdata for Africa/Douala: %v", err)
	}
	return loc
}

func mustHours(t *testing.T, raw string) domain.OpeningHours {
	t.Helper()
	h, err := domain.ParseOpeningHours([]byte(raw))
	if err != nil {
		t.Fatalf("parse opening hours: %v", err)
	}
	return h
}

// Every weekday open 08:00-12:00, so a test can pick any date without its
// answer depending on which day of the week the calendar happens to land on.
const everyDayMorning = `{"mon":[["08:00","12:00"]],"tue":[["08:00","12:00"]],"wed":[["08:00","12:00"]],
                          "thu":[["08:00","12:00"]],"fri":[["08:00","12:00"]],"sat":[["08:00","12:00"]],
                          "sun":[["08:00","12:00"]]}`

func TestParseOpeningHours(t *testing.T) {
	h := mustHours(t, `{"mon":[["08:00","12:00"],["13:00","16:30"]],"sat":[["09:00","12:00"]]}`)

	if !h.Configured() {
		t.Error("hours with entries report themselves unconfigured")
	}
	mon := h["mon"]
	if len(mon) != 2 {
		t.Fatalf("monday has %d intervals, want 2 — a lunch break is the ordinary case", len(mon))
	}
	if mon[0].Start.String() != "08:00" || mon[0].End.String() != "12:00" {
		t.Errorf("first interval = %s-%s", mon[0].Start, mon[0].End)
	}
	// A day with no entry is closed, and that must not be confused with the
	// whole object being empty.
	if len(h["sun"]) != 0 {
		t.Error("sunday has intervals it was never given")
	}

	// An empty object is "nobody has configured this", which is different from
	// "closed all week" — the caller falls back rather than turning everyone
	// away.
	empty, err := domain.ParseOpeningHours([]byte(`{}`))
	if err != nil {
		t.Fatalf("an empty object was rejected: %v", err)
	}
	if empty.Configured() {
		t.Error("an empty object reports itself configured")
	}
	if nothing, err := domain.ParseOpeningHours(nil); err != nil || nothing.Configured() {
		t.Errorf("nil hours = %v, %v; want an unconfigured empty set", nothing, err)
	}
}

func TestParseOpeningHoursRejectsNonsense(t *testing.T) {
	cases := map[string]string{
		"not an object":        `[["08:00","12:00"]]`,
		"unknown day":          `{"funday":[["08:00","12:00"]]}`,
		"close before open":    `{"mon":[["17:00","09:00"]]}`,
		"close equals open":    `{"mon":[["09:00","09:00"]]}`,
		"not HH:MM":            `{"mon":[["8am","noon"]]}`,
		"one-ended interval":   `{"mon":[["08:00"]]}`,
		"three-ended":          `{"mon":[["08:00","12:00","13:00"]]}`,
		"overlapping":          `{"mon":[["08:00","12:00"],["11:00","15:00"]]}`,
		"overlapping unsorted": `{"mon":[["11:00","15:00"],["08:00","12:00"]]}`,
		"hour out of range":    `{"mon":[["25:00","26:00"]]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.ParseOpeningHours([]byte(raw)); err == nil {
				t.Fatalf("%s was accepted", name)
			} else if !errors.Is(err, domain.ErrInvalidHours) {
				t.Errorf("error = %v, want ErrInvalidHours", err)
			}
		})
	}
}

// Two abutting intervals are NOT an overlap: a centre that closes at 12:00 and
// reopens at 12:00 is open continuously, which is odd to write but not wrong.
func TestAbuttingIntervalsAreAllowed(t *testing.T) {
	if _, err := domain.ParseOpeningHours([]byte(`{"mon":[["08:00","12:00"],["12:00","16:00"]]}`)); err != nil {
		t.Errorf("abutting intervals were rejected as overlapping: %v", err)
	}
}

// The grid is measured from the START OF THE INTERVAL, not from midnight.
//
// A centre opening at 08:15 with 30-minute slots has slots at 08:15 and 08:45.
// A grid from midnight would offer 08:00 — fifteen minutes before anybody is
// there to take blood.
func TestSlotGridStartsAtOpeningNotMidnight(t *testing.T) {
	loc := douala(t)
	s := domain.Scheduling{
		SlotMinutes: 30, CapacityPer: 4, Location: loc,
		OpeningHours: mustHours(t, `{"mon":[["08:15","11:15"]],"tue":[["08:15","11:15"]],"wed":[["08:15","11:15"]],
		                             "thu":[["08:15","11:15"]],"fri":[["08:15","11:15"]],"sat":[["08:15","11:15"]],
		                             "sun":[["08:15","11:15"]]}`),
	}
	at := func(h, m int) time.Time { return time.Date(2026, time.October, 5, h, m, 0, 0, loc) }

	for _, c := range []struct {
		h, m int
		want string
	}{
		{8, 15, "08:15"},
		{8, 20, "08:15"},
		{8, 44, "08:15"},
		{8, 45, "08:45"},
		{10, 50, "10:45"},
	} {
		got, err := s.SlotStart(at(c.h, c.m))
		if err != nil {
			t.Errorf("%02d:%02d: %v", c.h, c.m, err)
			continue
		}
		if got.Format("15:04") != c.want {
			t.Errorf("%02d:%02d snapped to %s, want %s", c.h, c.m, got.Format("15:04"), c.want)
		}
	}
}

func TestSlotStartRefusesTimesOutsideOpeningHours(t *testing.T) {
	loc := douala(t)
	s := domain.Scheduling{
		SlotMinutes: 30, CapacityPer: 4, Location: loc,
		OpeningHours: mustHours(t, `{"mon":[["08:00","12:00"],["13:00","16:00"]]}`),
	}
	monday := func(h, m int) time.Time { return time.Date(2026, time.October, 5, h, m, 0, 0, loc) }
	if monday(9, 0).Weekday() != time.Monday {
		t.Fatalf("fixture drift: 2026-10-05 is a %s", monday(9, 0).Weekday())
	}

	for _, c := range []struct{ h, m int }{
		{7, 59},  // before opening
		{12, 0},  // the lunch break starts
		{12, 30}, // inside the break
		{16, 0},  // closing time itself
		{18, 0},  // after closing
	} {
		if _, err := s.SlotStart(monday(c.h, c.m)); !errors.Is(err, domain.ErrSlotOutOfHours) {
			t.Errorf("%02d:%02d = %v, want ErrSlotOutOfHours", c.h, c.m, err)
		}
	}
	// Both sides of the break are open.
	for _, c := range []struct{ h, m int }{{8, 0}, {11, 30}, {13, 0}, {15, 30}} {
		if _, err := s.SlotStart(monday(c.h, c.m)); err != nil {
			t.Errorf("%02d:%02d was refused: %v", c.h, c.m, err)
		}
	}
	// A closed day is refused whatever the time.
	sunday := time.Date(2026, time.October, 4, 9, 0, 0, 0, loc)
	if _, err := s.SlotStart(sunday); !errors.Is(err, domain.ErrSlotOutOfHours) {
		t.Errorf("sunday at a mon-only centre = %v, want ErrSlotOutOfHours", err)
	}
}

// A slot that would run past closing is not a slot.
func TestTheLastSlotMustFinishBeforeClosing(t *testing.T) {
	loc := douala(t)
	s := domain.Scheduling{
		SlotMinutes: 30, CapacityPer: 4, Location: loc,
		OpeningHours: mustHours(t, `{"mon":[["08:00","12:10"]]}`),
	}
	monday := func(h, m int) time.Time { return time.Date(2026, time.October, 5, h, m, 0, 0, loc) }

	// 11:30 finishes exactly at 12:00 and is fine.
	if got, err := s.SlotStart(monday(11, 45)); err != nil || got.Format("15:04") != "11:30" {
		t.Errorf("11:45 = %s, %v; want 11:30", got.Format("15:04"), err)
	}
	// 12:05 falls inside opening hours, but the slot it starts runs to 12:30.
	if _, err := s.SlotStart(monday(12, 5)); !errors.Is(err, domain.ErrSlotOutOfHours) {
		t.Errorf("12:05 on a day closing at 12:10 = %v, want ErrSlotOutOfHours", err)
	}

	slots := s.SlotsOn(monday(0, 0))
	if len(slots) != 8 {
		t.Errorf("%d slots between 08:00 and 12:10 at 30 minutes, want 8", len(slots))
	}
	if last := slots[len(slots)-1]; last.Format("15:04") != "11:30" {
		t.Errorf("the last slot is %s, want 11:30", last.Format("15:04"))
	}
}

func TestFirstSlotOn(t *testing.T) {
	loc := douala(t)
	s := domain.Scheduling{
		SlotMinutes: 30, CapacityPer: 4, Location: loc,
		OpeningHours: mustHours(t, `{"mon":[["08:00","12:00"]],"tue":[["13:00","17:00"]]}`),
	}

	monday := time.Date(2026, time.October, 5, 0, 0, 0, 0, loc)
	if got, err := s.FirstSlotOn(monday); err != nil || got.Format("15:04") != "08:00" {
		t.Errorf("monday first slot = %s, %v; want 08:00", got.Format("15:04"), err)
	}
	tuesday := monday.AddDate(0, 0, 1)
	if got, err := s.FirstSlotOn(tuesday); err != nil || got.Format("15:04") != "13:00" {
		t.Errorf("tuesday first slot = %s, %v; want 13:00", got.Format("15:04"), err)
	}
	// A closed day has no first slot, and says so rather than returning midnight.
	if _, err := s.FirstSlotOn(monday.AddDate(0, 0, 2)); !errors.Is(err, domain.ErrCentreClosed) {
		t.Errorf("wednesday at a mon/tue centre = %v, want ErrCentreClosed", err)
	}
	// An interval too short for one slot yields none.
	tight := domain.Scheduling{
		SlotMinutes: 60, CapacityPer: 4, Location: loc,
		OpeningHours: mustHours(t, `{"mon":[["08:00","08:30"]]}`),
	}
	if _, err := tight.FirstSlotOn(monday); !errors.Is(err, domain.ErrCentreClosed) {
		t.Errorf("a 30-minute day with 60-minute slots = %v, want ErrCentreClosed", err)
	}
}

func TestSlotsOnEnumeratesEveryInterval(t *testing.T) {
	loc := douala(t)
	s := domain.Scheduling{
		SlotMinutes: 60, CapacityPer: 4, Location: loc,
		OpeningHours: mustHours(t, `{"mon":[["08:00","11:00"],["13:00","15:00"]]}`),
	}
	got := s.SlotsOn(time.Date(2026, time.October, 5, 0, 0, 0, 0, loc))

	want := []string{"08:00", "09:00", "10:00", "13:00", "14:00"}
	if len(got) != len(want) {
		t.Fatalf("%d slots, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Format("15:04") != w {
			t.Errorf("slot %d = %s, want %s", i, got[i].Format("15:04"), w)
		}
	}
	// A closed day is an empty list, not a nil-vs-empty puzzle for the caller.
	if closed := s.SlotsOn(time.Date(2026, time.October, 4, 0, 0, 0, 0, loc)); len(closed) != 0 {
		t.Errorf("a closed day produced %d slots", len(closed))
	}
}

// A slot is a WALL-CLOCK time at the centre. Across a daylight-saving change the
// 09:00 slot is still 09:00 on the clock in the building, even though the
// instant moves — the opposite of the shelf-life rule, and for the opposite
// reason: a shelf life is a physical duration, an appointment is a time people
// turn up at.
func TestSlotsFollowTheWallClockAcrossDaylightSaving(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skipf("no tzdata for Europe/London: %v", err)
	}
	s := domain.Scheduling{
		SlotMinutes: 30, CapacityPer: 4, Location: london,
		OpeningHours: mustHours(t, everyDayMorning),
	}

	// BST starts 2026-03-29 and ends 2026-10-25.
	for _, day := range []time.Time{
		time.Date(2026, time.March, 28, 0, 0, 0, 0, london),
		time.Date(2026, time.March, 30, 0, 0, 0, 0, london),
		time.Date(2026, time.October, 24, 0, 0, 0, 0, london),
		time.Date(2026, time.October, 26, 0, 0, 0, 0, london),
	} {
		first, err := s.FirstSlotOn(day)
		if err != nil {
			t.Errorf("%s: %v", day.Format("2006-01-02"), err)
			continue
		}
		if got := first.In(london).Format("15:04"); got != "08:00" {
			t.Errorf("%s opens at %s, want 08:00 — the slot moved with the clocks",
				day.Format("2006-01-02"), got)
		}
	}
}

func TestSlotStartRejectsAnImpossibleSlotLength(t *testing.T) {
	loc := douala(t)
	for _, minutes := range []int{0, 4, 241, -30} {
		s := domain.Scheduling{
			SlotMinutes: minutes, CapacityPer: 4, Location: loc,
			OpeningHours: mustHours(t, everyDayMorning),
		}
		at := time.Date(2026, time.October, 5, 9, 0, 0, 0, loc)
		if _, err := s.SlotStart(at); !errors.Is(err, domain.ErrInvalidSlotLength) {
			t.Errorf("%d-minute slots: SlotStart = %v, want ErrInvalidSlotLength", minutes, err)
		}
		if _, err := s.FirstSlotOn(at); !errors.Is(err, domain.ErrInvalidSlotLength) {
			t.Errorf("%d-minute slots: FirstSlotOn = %v, want ErrInvalidSlotLength", minutes, err)
		}
	}
}

func TestClockRoundTrips(t *testing.T) {
	for _, s := range []string{"00:00", "08:15", "12:00", "23:59"} {
		c, err := domain.ParseClock(s)
		if err != nil {
			t.Errorf("ParseClock(%q): %v", s, err)
			continue
		}
		if c.String() != s {
			t.Errorf("ParseClock(%q).String() = %q", s, c.String())
		}
	}
	// A missing leading zero is accepted and normalised. The value comes from an
	// administrator editing a JSONB column by hand, and refusing "8:00" would
	// buy strictness nobody wants at the cost of a support call.
	if c, err := domain.ParseClock("8:00"); err != nil || c.String() != "08:00" {
		t.Errorf(`ParseClock("8:00") = %v, %v; want 08:00`, c, err)
	}
	if c, err := domain.ParseClock("  09:30  "); err != nil || c.String() != "09:30" {
		t.Errorf("ParseClock with padding = %v, %v", c, err)
	}

	for _, bad := range []string{"", "24:00", "12:60", "noon", "08-00", "0800"} {
		if _, err := domain.ParseClock(bad); err == nil {
			t.Errorf("ParseClock(%q) was accepted", bad)
		}
	}
}
