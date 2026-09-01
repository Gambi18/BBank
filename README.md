# BBank

A **blood bank management system**: donors register and ask to give, staff screen and collect,
the lab tests and releases, inventory tracks every unit, and hospitals request blood against
that stock.

> **Status: mid-build, and honest about it.** What runs today is the donor-appointment slice —
> signup, a donor record, a request, an admin who confirms it into a scheduled appointment —
> plus authentication and authorization for the full system. The blood itself is specified in
> detail and not yet built: nothing records a donation, a test result, a unit, or an issue to a
> hospital. See [`docs/PROJECT_STATUS.md`](docs/PROJECT_STATUS.md) for the real numbers, and
> **[Current state](#current-state)** below for what that means if you clone this today.

---

## Contents

- [What's in the box](#whats-in-the-box)
- [Quick start](#quick-start)
- [Current state](#current-state)
- [Architecture](#architecture)
- [Working on it](#working-on-it)
- [The documentation set](#the-documentation-set)
- [Testing and CI](#testing-and-ci)
- [Troubleshooting](#troubleshooting)

---

## What's in the box

| Path | What it is |
|---|---|
| `backend/` | Go 1.26 API. Layered: `cmd/api`, `internal/{domain,service,store,http,middleware,platform}`. chi + pgx + sqlc over PostgreSQL 18. Listens on `:8000`. |
| `bbank/` | Next.js 16 frontend (App Router, React 19, Tailwind 4). Server components and server actions call the Go API server-to-server. |
| `backend/migrations/` | golang-migrate SQL. 15 migrations, 26 tables, 21 enums, 4 views, 18 triggers. |
| `docs/` | Six cross-referenced planning documents that specify the target system, plus the living status file. |
| `compose.yaml` | `db` → `migrate` → `goapp` → `frontend`. |

---

## Quick start

**Prerequisites:** Docker with Compose v2, and OpenSSL for the signing key. Nothing else —
Go and Node run inside the images.

```bash
git clone <this repo> && cd BBank
cp .env.example .env
```

Now edit `.env`. Two values have no safe default and the stack will not start without them:

```bash
# 1. A database password.
POSTGRES_PASSWORD=<something long>
# ...and put the same value into DATABASE_URL, which is composed from it.

# 2. The ES256 signing key. Generate one:
openssl ecparam -genkey -name prime256v1 -noout | openssl pkcs8 -topk8 -nocrypt
```

Paste that key into `JWT_PRIVATE_KEY` as a single line with `\n` escapes, as shown in
`.env.example`. For local HTTP development also set `COOKIE_SECURE=false`, or the browser will
discard the session cookies.

```bash
docker compose up --build
```

- Frontend → <http://localhost:3000>
- API → <http://localhost:8000> (`/healthz`, `/readyz`)
- Postgres → `localhost:5433`

`migrate` runs to completion before `goapp` starts. The API does **not** create tables at boot.

### Creating the first account

There is no seeded login, and self-signup is temporarily unavailable (see below), so create a
user directly for now:

```bash
# A bcrypt cost-12 hash of your chosen password, e.g. via any bcrypt tool.
docker compose exec db psql -U admin -d bbank -c "
  INSERT INTO users (email, password_hash, role, status)
  VALUES ('you@example.com', '<bcrypt-hash>', 'admin', 'active');"
```

Roles are `donor`, `staff`, `lab_tech`, `inventory_manager`, `hospital_user`, `admin`. A `staff`
user additionally requires `center_id` (see `users_center_matches_role`); a `donor` also needs a
`donor_profiles` row. `WI-18` replaces this with a proper invite flow.

---

## Current state

The system is being rebuilt from a 3-table appointment scheduler into the blood bank the
documents describe, one work item at a time. Some things that used to work are deliberately
switched off mid-migration rather than left half-wired:

| Flow | State |
|---|---|
| Log in / log out | **Works.** ES256 JWT, 15-minute access token, rotating 7-day refresh with reuse detection. |
| Donor record, appointments, requests | **Works,** and is authorized per-role and per-owner. |
| Admin confirms a request → appointment | **Works.** |
| **Self-signup** | **Off.** `POST /donors` went away when donors moved to the layered handlers; `WI-22` rebuilds it against `users` + `donor_profiles`. The form says so instead of losing your details. |
| **Editing a donor profile** | **Off,** same reason, same fix. |
| **Registering a donor from the admin console** | **Off,** same reason, same fix. |
| Everything downstream of the needle | **Not built.** Screening, collection, testing, inventory, hospital requests, issue and traceability are specified in `docs/` and scheduled in the implementation plan. |

Blood group, rhesus and donation counts are shown read-only wherever they appear. They are
clinical facts set by staff and the lab, not something a donor declares — that is a rule
(`TRD.md` §7.7), not an oversight.

---

## Architecture

```
browser ──► Next.js (:3000) ──► Go API (:8000) ──► PostgreSQL (:5432)
            server components      chi + pgx           26 tables
            server actions         sqlc queries        golang-migrate
```

The browser never calls the Go API directly. Server components and server actions do, over the
compose network, forwarding the caller's session cookie.

**Authentication.** The API signs an ES256 JWT; the frontend only ever holds the **public** half,
fetched from `GET /api/v1/auth/public-key`. That asymmetry is the point — with a shared secret, a
compromise of the frontend would become a token factory. ES256 also verifies on both the Node and
Edge runtimes, so the route guard can check every navigation without a network round trip.

**Authorization** lives in `backend/internal/domain/rbac.go` as data: a 22-resource × 6-role ×
5-action matrix, plus named state transitions, because "may execute" and "may *approve*" are not
the same permission. Ownership comes from the token's `sub`, `cid` and `hid` claims and never from
a request parameter. A row outside your scope returns **404, not 403** — a 403 would confirm it
exists. All 660 matrix cells are asserted over HTTP in `internal/middleware/auth_test.go`.

**The dependency rule is enforced, not merely documented:**

```
domain   ← imports nothing from this project
service  ← may import domain, store
http     ← may import service, domain, store, middleware
cmd/     ← the only thing that may import http
```

`backend/archcheck.sh` fails the build on a violation, in CI and locally.

**The strangler.** `internal/legacy/` holds the original single-file handlers and shrinks with
each migrated resource. Do not add to it. To move a resource out: add queries to
`backend/queries/<resource>.sql`, run `sqlc generate`, add a service, add a handler, mount it in
`internal/http/router.go`, then delete the legacy handler and its route. `donors` is the worked
example.

---

## Working on it

```bash
# Backend, against the compose database
cd backend && go run ./cmd/api          # DATABASE_URL is required; it exits 1 without one
go test ./... && ./archcheck.sh

# Frontend
cd bbank && npm install && npm run dev
npx tsc --noEmit && npx eslint .
```

**Migrations.** Never hand-edit `backend/internal/store/` — it is generated, and CI fails if it
is stale. Run `sqlc generate` after touching `backend/queries/`.

```bash
migrate create -ext sql -dir backend/migrations -seq <description>
migrate -path backend/migrations -database "$DATABASE_URL" up
migrate -path backend/migrations -database "$DATABASE_URL" down 1     # must work
```

A down migration is written by **reading the up migration**, never from memory — constraint names
guessed from memory are a recurring source of failures here. Every migration is verified
`up → down → up` against a real database before it lands.

**Clinical constants are never hardcoded.** Eligibility thresholds, donation intervals and
component shelf lives live in the `policies` table (`docs/DATABASE_SCHEMA.md` §12), not in Go or
TypeScript.

**Keep `docs/PROJECT_STATUS.md` current in the same change** — it is the single source of truth
for progress. Tick the checklist, adjust the percentages, move items out of _Weaknesses_, add a
dated changelog line.

---

## The documentation set

Six documents specify the target system. They own different things and cite each other by ID —
never redefine another document's identifiers.

| Document | Owns |
|---|---|
| [`docs/PRD.md`](docs/PRD.md) | Requirements `FR-01`–`FR-83`, `NFR-01`–`NFR-26` |
| [`docs/TRD.md`](docs/TRD.md) | Architecture, API surface, auth, debt register `TD-01`–`TD-23` |
| [`docs/USER_JOURNEY.md`](docs/USER_JOURNEY.md) | Route paths, per-persona flows, the vein-to-vein master flow |
| [`docs/UIUX_BRIEF.md`](docs/UIUX_BRIEF.md) | Design tokens, component specs |
| [`docs/DATABASE_SCHEMA.md`](docs/DATABASE_SCHEMA.md) | Table, column, enum and view names |
| [`docs/IMPLEMENTATION_PLAN.md`](docs/IMPLEMENTATION_PLAN.md) | Work items `WI-xx`, phases, sequencing |

**Start at the implementation plan** — it sequences everything else and cites the requirement each
work item satisfies. [`docs/PROJECT_STATUS.md`](docs/PROJECT_STATUS.md) tracks what is actually
done. [`Mistakes.md`](Mistakes.md) records mistakes made here and how to avoid repeating them;
it is worth two minutes before you start.

---

## Testing and CI

`.github/workflows/ci.yml` runs on every push:

- **Backend** — gofmt, `go vet`, build, golangci-lint, govulncheck, unit tests, the architecture
  dependency check, and a check that `sqlc generate` output is not stale.
- **Frontend** — `tsc --noEmit`, ESLint, `npm audit` (high and above).
- **Migrations** — `up → down → up` against a real PostgreSQL service.
- **Secrets** — gitleaks, plus an assertion that no raw DSN is printed at boot.

Coverage is thin and known to be: the domain and the authorization middleware are tested; service,
handler and end-to-end tests are `WI-29` and `WI-30`.

---

## Troubleshooting

**`DATABASE_URL is required`** — you skipped `cp .env.example .env`, or edited `POSTGRES_PASSWORD`
without updating the same value inside `DATABASE_URL`.

**`JWT_PRIVATE_KEY is required`** — generate one as shown above. For throwaway local runs
`ALLOW_EPHEMERAL_JWT_KEY=true` works, but every restart then invalidates every session, which is
deliberately annoying.

**Logged in, then immediately logged out again** — `COOKIE_SECURE=true` over plain HTTP. The
browser accepts the cookie and refuses to send it back. Set `COOKIE_SECURE=false` for local
development.

**Port 5433 already in use** — another project's Postgres. Set `DB_HOST_PORT` in `.env`.

**A page shows an empty list** — check `docker compose logs goapp`. A 401 means the frontend is
not forwarding a session; a 403 means the role is not permitted; a 404 on a record you can see the
id of means it is outside your scope, which is the intended answer.

**Frontend changes not appearing** — `docker compose up --build frontend`. The image is a
production build, not a dev server; use `npm run dev` for iteration.
