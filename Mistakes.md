# Mistakes Log

Tracks mistakes made while working in this repo — cause, course, solution. Keep entries brief.
**Check this file before starting a task** to avoid repeating a known error.

Format: `### YYYY-MM-DD — Short title` then **Cause / Course / Solution**.

---

### 2026-09-01 — File created
This file was created per the global instruction to maintain a mistakes log.

### 2026-09-01 — Debug line leaked the DB password, and the log was committed
**Cause:** `fmt.Println("Connected using DSN:", dsn)` was added to `main.go` as a temporary
debugging aid (the trailing comment even said so). The DSN embeds the password. `backend/backend.log`
then captured it and was committed to git.
**Course:** every container boot printed `postgres://admin:<password>@db:5433/bbank` to stdout;
line 1 of the tracked `backend.log` is that exact string.
**Solution:** `safeDSN()` redacts the password before logging (WI-01); `backend.log` and the compiled
binary are untracked and gitignored; a CI gate greps for any print/log call passing a DSN-bearing
identifier without `safeDSN()`. **The gate was tested by reintroducing the defect** — a rule you
haven't seen fail is not known to work.
**Prevention:** never log a whole connection string, config struct, or request body. Log named,
chosen fields.

### 2026-09-01 — An ownership check that the caller could opt out of
**Cause:** `getAppointment` guarded ownership with `if donorId != ""` — the check only ran when the
caller supplied `?donor_id=`. The parameter was treated as *the thing to verify against* rather than
*the claim being made*.
**Course:** `GET /api/go/appointments/{id}` with no query parameter returned any donor's appointment.
**Solution:** ownership is unconditional; a missing or non-integer `donor_id` is a 400, a mismatch is
a **404** (403 would confirm the record exists). Still an interim fix — identity must come from a
verified session (WI-17/WI-20), not a parameter.
**Prevention:** a conditional around an authorization check is a bug unless the condition is
"is this caller an admin". Derive identity from the session, never from caller-supplied input.

### 2026-09-01 — Wrote test fixtures from guessed column names
**Cause:** wrote INSERT statements for a 21-table schema from memory instead of introspecting first.
**Course:** five failed iterations on `dob` vs `date_of_birth`, missing NOT NULLs, a unit code that
failed its format CHECK, and a password that failed the bcrypt-shape CHECK.
**Solution:** query `pg_attribute` for the real columns before writing the fixture.
**Prevention:** for an unfamiliar schema, introspect first. Each guess costs a full round trip.

---

## Known traps in this repo (pre-emptive — not mistakes, but things that have bitten before)

These are drawn from the existing `docs/PROJECT_STATUS.md` changelog, where they were
already hit once and fixed. Do not reintroduce them.

- **`goapp` racing Postgres on boot.** `db.Ping()` fails during DB init and `log.Fatal`
  kills the container; the frontend then gets `EAI_AGAIN goapp`. Fixed with a 30×2s retry
  loop in `main.go` + `depends_on: condition: service_healthy`. Don't remove either.
- **Postgres healthcheck without `-d bbank`.** `pg_isready -U admin` defaults dbname to the
  username and spams `FATAL: database "admin" does not exist` every 5s.
- **`localhost` in frontend fetches.** Breaks inside Docker — services can't reach each
  other via localhost. Always go through `src/lib/api.ts` (`API_BASE_URL`).
- **Shell: parenthesised route-group paths.** `bbank/src/app/(dashboard)/...` must be quoted
  or escaped in zsh, or `cd`/`cat` fails with "no such file or directory".
- **Undefined Tailwind utility classes.** Custom classes (`.card`, `.btn`, `.field`, `.badge`,
  `.blob`, `.mesh`, `.blur-panel`…) are hand-defined in `src/app/globals.css`, not Tailwind
  built-ins. Check that file before using or renaming one.

### 2026-09-01 — Guessed a constraint name in a down migration
**Cause:** wrote `DROP CONSTRAINT users_hospital_role_chk` in `000003.down.sql` from memory; the
actual name in the up migration is `users_hospital_only_for_hospital_user`.
**Course:** would have made the down migration fail — caught before running, by diffing the down
against the up rather than assuming.
**Solution:** read the constraint names out of the up migration.
**Prevention:** a down migration must be written by reading the up migration, never from memory.
Same root cause as the fixture-guessing entry above: guessing identifiers instead of reading them.

### 2026-09-01 — Shell variable used as a command (zsh)
**Cause:** `MIG='docker run ...'` then `$MIG down 3`. zsh does not word-split unquoted parameter
expansions the way bash does, so it looked for a single file with the whole string as its name.
**Course:** the down/up round-trip silently did not run; the follow-up assertions then reported on a
stale database and looked like they passed.
**Solution:** use a shell function, not a variable, for a reusable command.
**Prevention:** a step that appears to succeed without producing its expected output has not run.
Check the step's own output, not just the assertion after it.
