package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"bbank/internal/domain"
	"bbank/internal/store"

	"github.com/jackc/pgx/v5"
)

// CenterService owns donation centres and their booking configuration (WI-24).
type CenterService struct {
	q *store.Queries
}

func NewCenterService(q *store.Queries) *CenterService { return &CenterService{q: q} }

// ErrCenterInactive is a centre that exists and is not taking bookings.
//
// Distinct from ErrNotFound on purpose. `FR-14` requires deactivating a centre
// to "stop new bookings and preserve history", and a donor whose usual centre
// answers "no such centre" will go looking for a bug — where "that centre is not
// currently taking bookings" tells them to pick another one.
var ErrCenterInactive = errors.New("that centre is not currently taking bookings")

// Scheduling reads a centre's booking configuration into the domain's shape.
func (s *CenterService) Scheduling(ctx context.Context, centerID int64) (domain.Scheduling, bool, error) {
	return s.SchedulingWith(ctx, s.q, centerID)
}

// SchedulingWith reads it on the CALLER'S queries.
//
// The same rule as EligibilityService.EvaluateWith: a caller inside a
// transaction must not reach for a second pool connection while holding a row
// lock, or N concurrent callers deadlock a pool of fewer than N connections.
// That already happened once here, in WI-26's approval gate.
func (s *CenterService) SchedulingWith(ctx context.Context, q *store.Queries, centerID int64) (domain.Scheduling, bool, error) {
	row, err := q.GetCenterScheduling(ctx, centerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Scheduling{}, false, fmt.Errorf("%w: no donation centre with that id", ErrNotFound)
		}
		return domain.Scheduling{}, false, fmt.Errorf("read centre %d: %w", centerID, err)
	}

	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		// A centre configured with a timezone this deployment's tzdata does not
		// know cannot be scheduled at all: every slot would be an hour or more
		// from the wall clock the staff there are reading. Refusing is the only
		// answer that does not book somebody at the wrong time.
		return domain.Scheduling{}, false, fmt.Errorf("%w: centre %s has an unknown timezone %q",
			ErrInvalid, row.Code, row.Timezone)
	}

	hours, err := domain.ParseOpeningHours(row.OpeningHours)
	if err != nil {
		return domain.Scheduling{}, false, fmt.Errorf("%w: centre %s: %s", ErrInvalid, row.Code, err)
	}

	return domain.Scheduling{
		SlotMinutes:  int(row.SlotMinutes),
		CapacityPer:  int(row.CapacityPerSlot),
		Location:     loc,
		OpeningHours: hours,
	}, row.IsActive, nil
}

// LegacyOpeningClock is the time an appointment lands at when the centre has no
// opening hours configured.
//
// 09:00 local, which is exactly what migration 000005 hardcoded into
// `CreateAppointmentForRequest` and what every existing appointment therefore
// sits at. Keeping it means `WI-24` does not silently move the appointments
// already in the database, and a centre that fills in its hours stops using it.
//
// It is NOT a clinical constant — no policy row governs when a building opens —
// so it stays in Go rather than in `policies`.
const LegacyOpeningClock = 9 * 60

// LegacyClosingClock ends the fallback day at 17:00 local.
//
// The fallback has to be a WORKING DAY, not a single slot. A one-slot fallback
// looked tidy — approvals land at 09:00 exactly as they always did — but it also
// made 09:00 the only bookable minute, so rescheduling an appointment to the
// afternoon at a centre nobody had configured came back "outside opening
// hours". That is a capability the system had before `WI-24` and has no reason
// to lose to a default.
//
// 09:00-17:00 is an operational guess, and an explicitly replaceable one: the
// moment an administrator sets real hours, none of this runs. It is not a
// clinical constant, so it belongs in Go rather than in `policies`.
const LegacyClosingClock = 17 * 60

// BookingTime is when a caller wants an appointment, and whether they named a
// DAY or an INSTANT.
//
// The distinction has to be carried explicitly, because it cannot be recovered
// from the value. `time.Parse("2006-01-02", "2026-09-03")` returns midnight
// **UTC**, and the first version of this inferred "the caller named a bare date"
// from `at.In(centre.Location)` reading 00:00 — which for Africa/Douala (UTC+1)
// is 01:00, so the inference never fired. Every approval at a centre with real
// opening hours fell through to "01:00 is outside opening hours" and returned a
// 409. The feature was unusable the moment an administrator configured hours,
// and no test saw it because the seeded centre has none.
type BookingTime struct {
	At time.Time
	// DateOnly says the caller named a calendar date and no time. The date is
	// read from `At`'s Y/M/D and rebuilt in the CENTRE's location, so a centre
	// west of UTC does not get yesterday's slots.
	DateOnly bool
}

// OnDate is a caller who named a day.
func OnDate(t time.Time) BookingTime { return BookingTime{At: t, DateOnly: true} }

// AtTime is a caller who named an instant.
func AtTime(t time.Time) BookingTime { return BookingTime{At: t} }

// effective applies the legacy fallback, so every caller sees the SAME slots.
//
// In one place, deliberately. `SlotFor` fell back to 09:00 while `SlotsOn`
// returned nothing, so on the shipped database a booking UI showed "no slots on
// any date" while approvals at that same centre succeeded at 09:00 — the two
// halves of one feature disagreeing, which is the failure this codebase keeps
// re-learning. Synthesising the fallback as real opening hours means there is
// only one code path to be right.
func effective(sched domain.Scheduling) domain.Scheduling {
	if sched.OpeningHours.Configured() {
		return sched
	}
	open, close := domain.Clock(LegacyOpeningClock), domain.Clock(LegacyClosingClock)
	hours := make(domain.OpeningHours, 7)
	for _, day := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
		hours[day] = []domain.Interval{{Start: open, End: close}}
	}
	sched.OpeningHours = hours
	return sched
}

// SlotFor resolves the instant an appointment should be booked at.
func (s *CenterService) SlotFor(ctx context.Context, centerID int64, when BookingTime) (time.Time, error) {
	return s.SlotForWith(ctx, s.q, centerID, when)
}

// SlotForWith resolves it on the caller's queries. See SchedulingWith.
func (s *CenterService) SlotForWith(ctx context.Context, q *store.Queries, centerID int64, when BookingTime) (time.Time, error) {
	sched, active, err := s.SchedulingWith(ctx, q, centerID)
	if err != nil {
		return time.Time{}, err
	}
	if !active {
		return time.Time{}, fmt.Errorf("%w: %s", ErrConflict, ErrCenterInactive)
	}
	sched = effective(sched)

	if when.DateOnly {
		// The calendar date the caller wrote, rebuilt at the centre's midnight.
		y, m, d := when.At.Date()
		day := time.Date(y, m, d, 0, 0, 0, 0, sched.Location)
		slot, err := sched.FirstSlotOn(day)
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: %s", ErrConflict, err.Error())
		}
		return slot, nil
	}

	slot, err := sched.SlotStart(when.At.In(sched.Location))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s", ErrConflict, err.Error())
	}
	return slot, nil
}

// LocalDay rebuilds a calendar date at a centre's midnight, for a caller that
// parsed `?date=YYYY-MM-DD` and therefore holds midnight UTC.
func (s *CenterService) LocalDay(sched domain.Scheduling, date time.Time) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, sched.Location)
}

type ListCenterParams struct {
	ActiveOnly bool
	Region     *string
	Limit      int32
	Offset     int32
}

func (s *CenterService) List(ctx context.Context, p ListCenterParams) ([]store.ListCentersRow, int64, error) {
	var activeOnly *bool
	if p.ActiveOnly {
		activeOnly = &p.ActiveOnly
	}
	rows, err := s.q.ListCenters(ctx, store.ListCentersParams{
		ActiveOnly: activeOnly, Region: p.Region, Limit: p.Limit, Offset: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list centres: %w", err)
	}
	var total int64
	if len(rows) > 0 {
		total = rows[0].Total
	}
	return rows, total, nil
}

func (s *CenterService) Get(ctx context.Context, id int64) (store.GetCenterRow, error) {
	row, err := s.q.GetCenter(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("get centre %d: %w", id, err)
	}
	return row, nil
}

type CenterInput struct {
	Code            string
	Name            string
	AddressLine     string
	City            string
	Region          string
	Phone           *string
	Email           *string
	CapacityPerSlot *int16
	SlotMinutes     *int16
	OpeningHours    []byte
	Timezone        *string
	IsActive        *bool
}

func (s *CenterService) Create(ctx context.Context, in CenterInput) (store.CreateCenterRow, error) {
	var zero store.CreateCenterRow
	if err := in.validate(true); err != nil {
		return zero, err
	}

	row, err := s.q.CreateCenter(ctx, store.CreateCenterParams{
		Code: strings.ToUpper(strings.TrimSpace(in.Code)), Name: strings.TrimSpace(in.Name),
		AddressLine: in.AddressLine, City: in.City, Region: in.Region,
		Phone: in.Phone, Email: in.Email,
		CapacityPerSlot: in.CapacityPerSlot, SlotMinutes: in.SlotMinutes,
		OpeningHours: in.OpeningHours, Timezone: in.Timezone, IsActive: in.IsActive,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return zero, fmt.Errorf("%w: a centre with that code already exists", ErrConflict)
		}
		if isCheckViolation(err) {
			return zero, fmt.Errorf("%w: capacity must be 1-100 and a slot 5-240 minutes", ErrInvalid)
		}
		return zero, fmt.Errorf("create centre: %w", err)
	}
	return row, nil
}

func (s *CenterService) Update(ctx context.Context, id int64, in CenterInput) (store.UpdateCenterRow, error) {
	var zero store.UpdateCenterRow
	if err := in.validate(false); err != nil {
		return zero, err
	}

	row, err := s.q.UpdateCenter(ctx, store.UpdateCenterParams{
		ID: id, Name: emptyToNil(in.Name), AddressLine: emptyToNil(in.AddressLine),
		City: emptyToNil(in.City), Region: emptyToNil(in.Region),
		Phone: in.Phone, Email: in.Email,
		CapacityPerSlot: in.CapacityPerSlot, SlotMinutes: in.SlotMinutes,
		OpeningHours: in.OpeningHours, Timezone: in.Timezone, IsActive: in.IsActive,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, ErrNotFound
	}
	if err != nil {
		if isCheckViolation(err) {
			return zero, fmt.Errorf("%w: capacity must be 1-100 and a slot 5-240 minutes", ErrInvalid)
		}
		return zero, fmt.Errorf("update centre %d: %w", id, err)
	}
	return row, nil
}

// validate checks what the database cannot.
//
// Lowering capacity below what a slot already holds is deliberately ALLOWED:
// `FR-14` says deactivating preserves history, and the same reasoning applies to
// a capacity cut — the appointments already booked keep their seats, and the
// reduced figure governs the next booking. Cancelling somebody's appointment
// because an administrator edited a number would be the system making a clinical
// scheduling decision on its own.
func (in CenterInput) validate(creating bool) error {
	if creating {
		for field, v := range map[string]string{
			"code": in.Code, "name": in.Name, "address_line": in.AddressLine,
			"city": in.City, "region": in.Region,
		} {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("%w: %s is required", ErrInvalid, field)
			}
		}
	}
	if in.Timezone != nil {
		// Blank is rejected rather than ignored. `time.LoadLocation("")` returns
		// UTC with no error, and `COALESCE(''::text, timezone)` writes the empty
		// string rather than leaving the column alone — so `{"timezone": ""}`
		// silently relocated a centre to UTC and shifted every slot by its real
		// offset, with no diagnostic anywhere.
		if strings.TrimSpace(*in.Timezone) == "" {
			return fmt.Errorf("%w: timezone cannot be blank", ErrInvalid)
		}
		if _, err := time.LoadLocation(*in.Timezone); err != nil {
			return fmt.Errorf("%w: %q is not a known timezone", ErrInvalid, *in.Timezone)
		}
	}
	if len(in.OpeningHours) > 0 {
		if _, err := domain.ParseOpeningHours(in.OpeningHours); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
		}
	}
	return nil
}

func emptyToNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	t := strings.TrimSpace(s)
	return &t
}

// SlotOccupancy reports how full each slot is on one local day at a centre.
func (s *CenterService) SlotOccupancy(ctx context.Context, centerID int64, day time.Time) ([]store.SlotOccupancyOnRow, domain.Scheduling, error) {
	sched, _, err := s.Scheduling(ctx, centerID)
	if err != nil {
		return nil, sched, err
	}
	// The same effective hours booking uses, so the slots a UI offers are the
	// slots an approval will accept.
	sched = effective(sched)
	// The caller parsed `?date=` into midnight UTC, so the day is taken from its
	// Y/M/D and rebuilt at the centre's midnight. Using the instant would give a
	// centre west of UTC the previous day's slots.
	from := s.LocalDay(sched, day)

	rows, err := s.q.SlotOccupancyOn(ctx, store.SlotOccupancyOnParams{
		CenterID: centerID,
		FromAt:   pgTime(from),
		ToAt:     pgTime(from.AddDate(0, 0, 1)),
	})
	if err != nil {
		return nil, sched, fmt.Errorf("slot occupancy: %w", err)
	}
	return rows, sched, nil
}
