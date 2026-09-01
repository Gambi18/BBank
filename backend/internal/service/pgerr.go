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

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
