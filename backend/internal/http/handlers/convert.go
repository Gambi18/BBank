package handlers

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// pgtype -> JSON-friendly strings. Dates render as YYYY-MM-DD and timestamps as
// RFC3339; a NULL becomes an absent field rather than a zero value, so a client
// can tell "unknown" from "1970".

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func tsPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.Format("2006-01-02T15:04:05Z07:00")
	return &s
}

// The donor_eligibility view computes its date columns through CASE expressions,
// which sqlc cannot type, so they arrive as interface{}. Convert defensively
// rather than asserting a single concrete type: pgx may hand back time.Time or
// a pgtype value depending on the expression.

func anyDatePtr(v any) *string {
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		s := t.Format("2006-01-02")
		return &s
	case pgtype.Date:
		return datePtr(t)
	case string:
		if t == "" {
			return nil
		}
		return &t
	}
	return nil
}

// anyBoolPtr reads `is_eligible_today`, which became a CASE expression in
// migration 000018 and so arrives as interface{} like the date columns above.
//
// nil is meaningful and is NOT false: it means the view could not decide,
// because a clinical threshold has no active policy row. Collapsing that to
// "ineligible" would state a clinical conclusion the system did not reach.
func anyBoolPtr(v any) *bool {
	switch b := v.(type) {
	case nil:
		return nil
	case bool:
		return &b
	case *bool:
		return b
	case pgtype.Bool:
		if !b.Valid {
			return nil
		}
		return &b.Bool
	}
	return nil
}

func anyTimePtr(v any) *string {
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		s := t.Format("2006-01-02T15:04:05Z07:00")
		return &s
	case pgtype.Timestamptz:
		return tsPtr(t)
	case string:
		if t == "" {
			return nil
		}
		return &t
	}
	return nil
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefBool(p *bool) bool { return p != nil && *p }
