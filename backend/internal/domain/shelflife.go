package domain

import (
	"fmt"
	"time"
)

// StorageRange is the temperature band a component must be held at, in Celsius.
type StorageRange struct {
	MinC float64
	MaxC float64
}

// ShelfLife is a component's configured life and storage conditions.
type ShelfLife struct {
	Component Component
	Duration  time.Duration
	Storage   StorageRange
	// Note is the operator-facing gloss from the policy row — "35 days, CPDA-1".
	// Never parsed; shown.
	Note string
}

// ShelfLife reads a component's shelf life from policy.
//
// Stored in HOURS, not days, and that is a deliberate schema decision (§12.1):
// platelets are 5 days, or 7 with bacterial testing, and a day-granular column
// cannot express the difference between components measured in days and one
// measured in hours without a second unit column to disagree with the first.
func (p *Policies) ShelfLife(c Component) (ShelfLife, error) {
	key := ShelfLifeKey(c)
	var v struct {
		Hours    *float64   `json:"hours"`
		StorageC []*float64 `json:"storage_c"`
		Note     string     `json:"note"`
	}
	if err := p.decode(key, &v); err != nil {
		return ShelfLife{}, err
	}
	if v.Hours == nil {
		return ShelfLife{}, fmt.Errorf("%w: %s needs hours", ErrPolicyMalformed, key)
	}
	if *v.Hours <= 0 {
		return ShelfLife{}, fmt.Errorf("%w: %s must be positive", ErrPolicyMalformed, key)
	}
	if len(v.StorageC) != 2 || v.StorageC[0] == nil || v.StorageC[1] == nil {
		return ShelfLife{}, fmt.Errorf("%w: %s needs storage_c as [min, max]", ErrPolicyMalformed, key)
	}
	lo, hi := *v.StorageC[0], *v.StorageC[1]
	if lo > hi {
		return ShelfLife{}, fmt.Errorf("%w: %s has storage_c reversed", ErrPolicyMalformed, key)
	}
	return ShelfLife{
		Component: c,
		Duration:  time.Duration(*v.Hours * float64(time.Hour)),
		Storage:   StorageRange{MinC: lo, MaxC: hi},
		Note:      v.Note,
	}, nil
}

// ExpiresAt is when a unit collected at `collectedAt` expires.
//
// **The addition is in absolute time, not calendar time, and that is the whole
// point of this function.** A component's life is a physical fact about a bag
// of blood: 35 days of refrigeration is 840 hours whatever the calendar does in
// between. `AddDate(0, 0, 35)` adds 35 *calendar* days, which across a spring
// daylight-saving change is 839 hours — an hour of shelf life invented — and
// across an autumn one is 841, an hour of expired product left available.
//
// `time.Time.Add` on an instant is immune to both: it advances the underlying
// instant, and any wall-clock representation of the result is derived from it
// afterwards. Leap years and leap seconds need no special handling for the same
// reason.
//
// TRD §13.3 names this as a required test, describing a DST-shifted expiry as
// "a real and embarrassing bug". It is: the unit would be issued, and nothing
// downstream would notice.
func (s ShelfLife) ExpiresAt(collectedAt time.Time) time.Time {
	return collectedAt.Add(s.Duration)
}

// RemainingAt is how much life a unit has left at `at`. Negative once expired,
// so a caller can tell "just expired" from "expired last month".
func (s ShelfLife) RemainingAt(collectedAt, at time.Time) time.Duration {
	return s.ExpiresAt(collectedAt).Sub(at)
}

// AllShelfLives reads every component's shelf life.
//
// It fails if ANY component is missing one, rather than returning what it
// found. A partial map is the shape that lets a component slip through with no
// expiry date at all, and a unit with no expiry is a unit that never expires.
func (p *Policies) AllShelfLives() (map[Component]ShelfLife, error) {
	out := make(map[Component]ShelfLife, len(Components()))
	for _, c := range Components() {
		sl, err := p.ShelfLife(c)
		if err != nil {
			return nil, err
		}
		out[c] = sl
	}
	return out, nil
}
