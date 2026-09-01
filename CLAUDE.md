# CLAUDE.md

Guidance for Claude Code (and humans) working in this repository.

## What this is

**BBank** — a Blood Bank Database Management System. Donors register and request donation
appointments; an admin reviews requests and confirms them into scheduled appointments.

- `bbank/` — **Frontend.** Next.js 16 (App Router), React 19, Tailwind 4. Uses server
  components + server actions to talk to the Go API. Path alias `@/*` → `src/*`.
- `backend/` — **Backend.** Layered Go API (`cmd/api` → `internal/{domain,service,store,http}`)
  using `chi` + `pgx`/`sqlc`, with a shrinking `internal/legacy` shim. Migrations are applied by
  `golang-migrate`, never at boot. Listens on `:8000`. See **Conventions** below.
- `compose.yaml` — Docker Compose: `goapp` (8000), `db` (Postgres 18; host 5433 → container 5432),
  `frontend` (3000).

## Architecture at a glance

- **API routes are under `/api/v1/...`** (`auth`, `donors`, `donation-requests`, `appointments`)
  — canonical since `WI-21` (TRD §6.1). The old `/api/go/...` prefix still answers, but it is a
  deprecated **alias**, not a second implementation: `middleware.LegacyShim` rewrites those paths
  to `/api/v1` before routing, so the two cannot drift. Every legacy response carries
  `Deprecation: true` and `Sunset: Wed, 31 Mar 2027`. Never write new code against `/api/go/`.
  The shim can be switched off and back on at runtime via `PATCH /api/v1/admin/flags` (admin only).
- All responses are **enveloped** (TRD §6.2): `{data, page?, meta}` on success,
  `{error:{code, message, details[], request_id}}` on failure. No bare arrays, and **never** a raw
  driver error — use `internal/http/response`, not `http.Error(w, err.Error(), …)`.
- List endpoints are paginated (§6.3): `?limit=&offset=`, default 25, max 100, **clamped not
  rejected**, and `page.limit` reports the limit actually applied. Use `response.ParsePaging`.
- Flow: signup → donor → donor requests an appointment (`POST /donation-requests`)
  → staff/admin approves (`POST /donation-requests/{id}/approve`), which creates the appointment
  and sets `status='approved'`. It **does not delete the request** — the old `confirm` did, and
  that destroyed the audit chain.
- Route groups: `(root)` = public (landing/login/signup), `(dashboard)` = `admin/*`, `donor/[id]`,
  and the `staff`/`lab`/`inventory`/`hospital` areas. Guarded by `src/proxy.ts`, which verifies the
  ES256 token (`WI-19`); the API authorizes independently (`WI-20`).

## Running locally

```bash
cp .env.example .env               # required — there are no default credentials
docker compose up --build          # db -> migrate -> goapp -> frontend
# or, piecemeal:
cd backend && go run ./cmd/api     # DATABASE_URL is required; the process exits 1 without it
cd bbank   && npm install && npm run dev
```

`migrate` runs to completion before `goapp` starts; the API no longer creates tables (WI-08).
To add a table, add a pair of files under `backend/migrations/`:

```bash
migrate create -ext sql -dir backend/migrations -seq <description>
migrate -path backend/migrations -database "$DATABASE_URL" up      # and `down 1` to reverse
```

If host port 5433 is taken by another project's Postgres, set `DB_HOST_PORT` in `.env`.

Backend requires `DATABASE_URL` — there is no fallback; the process exits 1 without it (WI-07).
The frontend resolves the API base
through `src/lib/api.ts` (`API_BASE_URL`, set to `http://goapp:8000` in compose) — never hardcode
`http://localhost:8000` in a fetch call; that breaks inside Docker.

## Conventions

- **Backend is layered** (`TRD.md` §4.2, superseding the former single-file rule — see below):

  ```
  backend/
    cmd/api/          entrypoint: wire config -> pools -> services -> handlers
    cmd/migrate/      schema version check (migrations are applied by golang-migrate)
    internal/domain/  pure business logic — imports NOTHING from this project
    internal/service/ use cases, transaction boundaries; may import domain + store
    internal/store/   sqlc-generated queries (pgx). Never hand-edit; run `sqlc generate`
    internal/http/    router, handlers/, dto/, response/ — imports service
    internal/middleware/, internal/platform/
    internal/legacy/  the old single-file handlers, shrinking. Do not add to it
    migrations/       golang-migrate SQL, applied by the `migrate` compose service
    queries/          .sql input for sqlc
  ```

  **The dependency rule is enforced**: run `backend/archcheck.sh` (also a CI job).
  `domain` imports nothing from the project; nothing but `cmd/` imports `http`.
  Adding a `domain -> store` import fails the build in CI — verified, not assumed.

  *Why this replaced "the backend is intentionally one file":* that rule was correct at
  3 tables and 543 lines. At 21 tables it left business rules with nowhere to live, made
  `rows.Scan` positional over hundreds of columns, and had no seam for testing. The change
  is deliberate and is recorded in `TRD.md` §4.4.

- **Migrating a resource out of `internal/legacy`** (the strangler): add its queries to
  `queries/<resource>.sql`, run `sqlc generate`, add a service, add a handler, mount it in
  `internal/http/router.go`, then delete the legacy handler and its route. `donors` is the
  worked example — follow it.
- **Never hand-edit `internal/store/`.** It is generated. CI fails if it is stale.
- - Frontend mutations use **server actions** (`'use server'`) that fetch the Go API, then
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
