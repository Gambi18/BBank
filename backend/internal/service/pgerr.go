package service

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL SQLSTATE codes worth naming.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
	codeCheckViolation      = "23514"
)

// Translating constraint violations into domain outcomes is what keeps the
// handler from ever seeing a driver error.
//
// The alternative — checking for a duplicate before inserting and trusting the
// answer — is a race: between the SELECT and the INSERT another request can
// take the value. The constraint is the only thing that actually holds, so the
// pattern is to attempt the write and interpret the failure. That makes these
// helpers load-bearing rather than cosmetic.

func isUniqueViolation(err error) bool     { return sqlState(err) == codeUniqueViolation }
func isForeignKeyViolation(err error) bool { return sqlState(err) == codeForeignKeyViolation }
func isCheckViolation(err error) bool      { return sqlState(err) == codeCheckViolation }

// isUniqueViolationOn narrows a 23505 to ONE index by name.
//
// Which index fired decides what to do, and the two that can fire on an
// appointment insert want opposite handling: a seat collision is a lost race
// worth retrying, while a second appointment for the same request is final. A
// bare `isUniqueViolation` cannot tell them apart, so it would either retry a
// duplicate forever or report a full slot as a duplicate.
//
// The index name is the contract. It is created in a migration and asserted by
// `TestSeatCollisionsAreRetriedNotReported`, so renaming it without updating
// this string fails a test rather than silently turning the retry off.
func isUniqueViolationOn(err error, index string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == codeUniqueViolation && pgErr.ConstraintName == index
	}
	return false
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
