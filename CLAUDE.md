# CLAUDE.md

Guidance for Claude Code (and humans) working in this repository.

## What this is

**BBank** — a Blood Bank Database Management System. Donors register and request donation
appointments; an admin reviews requests and confirms them into scheduled appointments.

- `bbank/` — **Frontend.** Next.js 16 (App Router), React 19, Tailwind 4. Uses server
  components + server actions to talk to the Go API. Path alias `@/*` → `src/*`.
- `backend/` — **Backend.** Single-file Go API (`main.go`) using `gorilla/mux` + `lib/pq`,
  raw SQL against PostgreSQL. Auto-creates tables on boot. Listens on `:8000`.
- `compose.yaml` — Docker Compose: `goapp` (8000), `db` (Postgres 18; host 5433 → container 5432),
  `frontend` (3000).

## Architecture at a glance

- API routes are under `/api/go/...` (`donors`, `requests`, `appointments`).
- Flow: signup → donor (`POST /donors`) → donor requests appointment (`POST /requests`)
  → admin confirms (`POST /requests/{id}/confirm`) which **deletes the request and creates an
  appointment**.
- Route groups: `(root)` = public (landing/login/signup), `(dashboard)` = `admin/*` and
  `donor/[id]`. There is currently **no auth guarding these** — see PROJECT_STATUS.md P0 items.

## Running locally

```bash
cp .env.example .env               # required — there are no default credentials
docker compose up --build          # db -> migrate -> goapp -> frontend
# or, piecemeal:
cd backend && go run .             # DATABASE_URL is required; the process exits 1 without it
cd bbank   && npm install && npm run dev
```

`migrate` runs to completion before `goapp` starts; the API no longer creates tables (WI-08).
To add a table, add a pair of files under `backend/migrations/`:

```bash
migrate create -ext sql -dir backend/migrations -seq <description>
migrate -path backend/migrations -database "$DATABASE_URL" up      # and `down 1` to reverse
```

If host port 5433 is taken by another project's Postgres, set `DB_HOST_PORT` in `.env`.

Backend reads `DATABASE_URL` (falls back to a localhost DSN). The frontend resolves the API base
through `src/lib/api.ts` (`API_BASE_URL`, set to `http://goapp:8000` in compose) — never hardcode
`http://localhost:8000` in a fetch call; that breaks inside Docker.

## Conventions

- **Backend layout — transitional.** Today the backend is one file (`main.go`), with handlers as
  `func(db *sql.DB) http.HandlerFunc`. **Keep matching that style for changes to the existing code.**
  This convention was correct at 3 tables and does not survive the documented 21-table scope:
  `docs/TRD.md` §4.4 formally supersedes it with a layered package structure (`cmd/`,
  `internal/domain/`, `internal/http/handlers/`, `internal/store/`, `internal/service/`,
  `internal/middleware/`, `migrations/`), adopted by strangler in Phase 1 (`WI-22`). Until that
  lands, do not start new packages ad hoc — follow the plan or the existing file, not a third thing.
- **Schema changes go through migrations, not `CREATE TABLE IF NOT EXISTS` on boot.** The current
  auto-create block in `main.go` is being replaced (`WI-08`); do not add tables to it.
- Frontend mutations use **server actions** (`'use server'`) that fetch the Go API, then
  `redirect(...?success=...|error=...)`; `components/ToastAlert.tsx` renders those query params.
- Match existing Tailwind-in-JSX style. Custom utility classes (`.card`, `.btn`, `.field`, `.badge`,
  `.blob`, `.blur-panel`, …) are hand-defined in `src/app/globals.css` — check there before using or
  renaming one. (The "undefined classes" issue previously noted here was resolved on 2026-06-11.)
- **Clinical constants are never hardcoded.** Eligibility thresholds, donation intervals and
  component shelf lives live in the `policies` table (`docs/DATABASE_SCHEMA.md` §12), not in Go or TS.

## Planning documents  ← read before non-trivial work

Six cross-referenced documents in `docs/` specify the target system. They own different things and
cite each other by ID — do not redefine another document's identifiers:

| Document | Owns |
|---|---|
| `docs/PRD.md` | Requirement IDs `FR-01`–`FR-83`, `NFR-01`–`NFR-26` |
| `docs/TRD.md` | Architecture, API surface, auth, debt register `TD-01`–`TD-23` |
| `docs/USER_JOURNEY.md` | Route paths, per-persona flows |
| `docs/UIUX_BRIEF.md` | Design tokens, component specs |
| `docs/DATABASE_SCHEMA.md` | Table, column, enum and view names |
| `docs/IMPLEMENTATION_PLAN.md` | Work items `WI-xx`, phases, sequencing |

Start from `docs/IMPLEMENTATION_PLAN.md` — it sequences everything else and cites the requirement
each work item satisfies.

## Maintaining `docs/PROJECT_STATUS.md`  ← important

`docs/PROJECT_STATUS.md` is the **single source of truth for progress**. Treat it as a living doc.

**Whenever you (Claude or a contributor) complete work in this repo, update it in the same change:**
1. Tick/untick the relevant **Feature Checklist** boxes.
2. Adjust the percentages in the **Completion Snapshot** table and the headline overall %.
3. Move resolved items out of **Weaknesses & Fixes** (and add new ones you discover).
4. Add a dated line to the **Changelog** and bump "Last updated".

Keep edits proportional to the actual change — don't inflate progress. If a task touches
security/auth, re-check the P0 list specifically.

> Optional automation: to *enforce* updates, add a `Stop` or `PostToolUse` hook in
> `.claude/settings.json` that reminds the agent to refresh PROJECT_STATUS.md after edits.
> Ask Claude to "set up a hook to keep PROJECT_STATUS.md updated" if you want this wired up.
