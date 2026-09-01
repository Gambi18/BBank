// Command migrate reports and verifies schema state.
//
// Migrations themselves are applied by the golang-migrate CLI (the `migrate`
// service in compose.yaml, and the migrations job in CI). This binary exists so
// the application image can *check* what schema it is talking to without being
// able to change it — the API connects as a role that should not hold DDL rights.
//
// Usage:
//
//	migrate version          print the current schema version, exit non-zero if dirty
//	migrate require <n>      exit non-zero unless the schema is exactly version n
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"bbank/internal/platform"

	"github.com/jackc/pgx/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate version | migrate require <n>")
		os.Exit(2)
	}

	cfg, err := platform.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	version, dirty, err := schemaVersion(ctx, conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read schema version: %v\n", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("schema version %d (dirty=%t)\n", version, dirty)
		if dirty {
			fmt.Fprintln(os.Stderr, "schema is DIRTY — a migration failed partway; force to the last good version and re-run")
			os.Exit(1)
		}
	case "require":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: migrate require <n>")
			os.Exit(2)
		}
		want, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "not a version number: %q\n", os.Args[2])
			os.Exit(2)
		}
		if dirty {
			fmt.Fprintf(os.Stderr, "schema is DIRTY at version %d\n", version)
			os.Exit(1)
		}
		if version != want {
			fmt.Fprintf(os.Stderr, "schema is at version %d, this build requires %d — refusing to start\n", version, want)
			os.Exit(1)
		}
		fmt.Printf("schema version %d as required\n", version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

func schemaVersion(ctx context.Context, conn *pgx.Conn) (int64, bool, error) {
	var version int64
	var dirty bool
	err := conn.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty)
	if errors.Is(err, pgx.ErrNoRows) {
		return -1, false, nil // no migrations applied yet
	}
	return version, dirty, err
}
