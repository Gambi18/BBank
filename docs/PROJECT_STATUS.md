# BBank — Project Status

> **Living document.** Update this file whenever a feature lands, a bug is fixed, or scope changes.
> Last updated: **2026-09-01** · Branch: `oc-redesign-skill-refactor`
>
> How to keep it current: tick the checkboxes, adjust the % in the **Completion Snapshot**,
> and move items between _Weaknesses_ and _Done_. See `CLAUDE.md` → "Maintaining this file".

BBank is a **Blood Bank Database Management System**: a portal where donors register, request
donation appointments, and an admin confirms them into scheduled appointments.

- **Frontend:** Next.js 16 (App Router, React 19, server components + server actions), Tailwind 4 — `bbank/`
- **Backend:** Go 1.26 (`gorilla/mux`, `lib/pq`, `bcrypt`), raw SQL over PostgreSQL 18 — `backend/`
- **Infra:** Docker Compose (`goapp`, `db`, `frontend`) — `compose.yaml`

---

## Completion Snapshot

> **Scope was redefined on 2026-09-01.** The percentages below are now reported against two
> different rulers, because reporting one number would be misleading:
>
> - **~77% of the *original* scope** — a donor appointment scheduler. That system works end-to-end.
> - **~8% of the *documented* scope** — a blood bank management system, as specified across the six
>   planning documents added on 2026-09-01.
>
> The gap is not polish. The system tracks **people and calendar slots, never blood**: nothing
> records that a donation happened, what was collected, whether it was screened, where it is stored,
> when it expires, or who received it. There is no demand side — no hospitals, no blood requests, no
> inventory. Of the 15 steps in the vein-to-vein master flow (`USER_JOURNEY.md` §3), **1 exists
> correctly, 2 partially, and 12 not at all.**

**Overall: ~77% of legacy scope · ~8% of documented scope.** The area table below still measures the
legacy scope; it will be rebaselined against the new scope at the `WI-06` boundary
(`IMPLEMENTATION_PLAN.md` §3), at which point several percentages deliberately go *down*.

| Area                     | Status | Notes |
|--------------------------|:------:|-------|
| Backend API (CRUD)       |  85%   | CRUD + `POST /login`, bcrypt, validation, transactional confirm. Missing: appt/request delete endpoints |
| Frontend UI / pages      |  97%   | Design refinement pass: Outfit font, tinted shadows, grain overlay, staggered layouts, richer empty/loading states, custom 404, legal pages, OG meta, skip-to-content |
| Auth & Security          |  70%   | + CORS allowlist, ownership bypass closed, secret hygiene, fail-fast config. TODO: sign the cookie (P1-1), real roles (P1-3) |
| Data model / DB          |  84%   | Migrations `000000`–`000005` applied and round-tripped against real data. Identity model, facilities/reference tables, `requests`→`donation_requests`, appointments upgraded to real timestamps + status. Remaining: `000006`–`000012` (`WI-15`, `WI-16`) |
| DevOps / Docker          |  92%   | + golang-migrate, `migrate` compose service, env-injected secrets, server timeouts, graceful shutdown, structured logs, `/healthz` + `/readyz` |
| Testing                  |  15%   | CI skeleton (gofmt, vet, build, golangci-lint, govulncheck, tsc, eslint, npm audit, gitleaks, migrate up/down/up). No unit tests yet — `WI-29` |
| Documentation            |  90%   | Full planning set (8,100+ lines): PRD, TRD, User Journey, UI/UX Brief, DB Schema, Implementation Plan — all cross-referenced by FR/NFR/WI ID |

### Planning documents (added 2026-09-01)

| Document | Lines | Owns |
|---|--:|---|
| [`PRD.md`](./PRD.md) | 634 | Requirement IDs `FR-01`–`FR-83`, `NFR-01`–`NFR-26` |
| [`TRD.md`](./TRD.md) | 1,854 | Architecture, API surface, auth design, debt register `TD-01`–`TD-23` |
| [`USER_JOURNEY.md`](./USER_JOURNEY.md) | 1,305 | Route inventory, per-persona flows, the vein-to-vein master flow |
| [`UIUX_BRIEF.md`](./UIUX_BRIEF.md) | 1,831 | Design tokens, component specs, accessibility findings |
| [`DATABASE_SCHEMA.md`](./DATABASE_SCHEMA.md) | 2,398 | Table/enum/view names, DDL, migration path (validated on PostgreSQL 18) |
| [`IMPLEMENTATION_PLAN.md`](./IMPLEMENTATION_PLAN.md) | 973 | Work items `WI-01`–`WI-102`, `WI-34a`, `WI-S01`–`WI-S13`; phases and sequencing |

**Cross-document rules:** requirement IDs are owned by the PRD; table and column names by the
schema doc; route paths by the user journey. Other documents cite, never redefine.

---

## Feature Checklist

### Donor flow
- [x] Landing page (hero / stats / about / contact) — modernized + animated
- [x] Signup (creates donor via API, sets session, redirects)
- [x] Login (real `POST /login` w/ bcrypt verify; hardcoded admin still supported)
- [x] Donor detail page (personal + health info, appointment list)
- [x] Request appointment (server action → `POST /requests`)
- [x] Donor profile edit (`/donor/settings` — fills the previously-blank fields)
- [x] Logout (sidebar + settings)
- [x] Donor settings page

### Admin flow
- [x] Dashboard (counts of appointments & requests, add-donor form)
- [x] Donors list table
- [x] Requests list + confirm → creates appointment
- [x] Appointments list
- [x] Admin settings page (with logout)
- [ ] Edit / delete donor from UI (backend supports it; no UI)
- [ ] Reject / delete a request from UI

### Backend
- [x] Donors: GET list, GET one, POST, PUT, DELETE
- [x] Requests: GET list, GET one, POST, POST confirm (transactional)
- [x] Appointments: GET list (with `donor_id` filter), GET one
- [x] Auth endpoint (`POST /login`, bcrypt verify)
- [x] Password hashing (bcrypt) + no password in any response
- [x] Basic input validation (required fields, email normalization)
- [ ] Appointments: update / delete / cancel
- [ ] Requests: delete / reject endpoint
- [ ] Session/JWT issuance from the backend (currently the frontend owns the cookie)

### Cross-cutting
- [x] Authentication & session management (httpOnly cookie)
- [x] Route protection / authorization (admin vs donor vs owner) — Next `proxy.ts`
- [x] Password hashing
- [x] Environment-based API base URL
- [x] Custom 404 page
- [x] Privacy policy and terms of service pages
- [ ] Signed/encrypted session cookie (currently plain JSON — forgeable)
- [ ] Automated tests (backend + frontend)
- [x] CI skeleton (lint, vet, build, vuln + secret scan, migrations up/down/up)
- [x] Database migrations (`golang-migrate`; boot-time DDL removed)
- [x] CORS locked to an allowlist
- [x] Secrets injected from env, no committed credentials, `.env.example`
- [x] Structured JSON logging with correlation IDs
- [x] HTTP server timeouts + graceful shutdown
- [x] `/healthz` and `/readyz` probes

---

## Weaknesses & Fixes

Ordered by severity. **P0 = blocks production, P1 = important, P2 = polish.**

### ✅ Resolved (2026-06-11)
- ~~Plaintext passwords~~ → bcrypt hashing on create/update; verified.
- ~~`GET /donors` leaks passwords + login fetches all donors~~ → `password` omitted from all
  responses; dedicated `POST /login` does a server-side bcrypt check.
- ~~No auth / sessions / route protection~~ → httpOnly cookie session set on login/signup,
  `proxy.ts` guards `/admin` (admin-only) and `/donor` (auth + ownership by URL id), logout added.
- ~~Hardcoded `http://localhost:8000` ×12~~ → centralized in `src/lib/api.ts` (`API_BASE_URL` env);
  `compose.yaml` sets `http://goapp:8000` for the frontend container.
- ~~`confirmRequest` not transactional~~ → wrapped in a DB transaction (insert appt → delete request → commit).
- ~~Settings links 404~~ → `/donor/settings` (profile edit) and `/admin/settings` pages added.
- ~~Signup leaves 6 fields blank with no way to fill them~~ → donor profile-edit page.
- ~~No server-side validation~~ → required-field + email-normalization checks in Go.
- ~~Wrong DSN fallback~~ → corrected to host-mapped `localhost:5433` for local `go run`.
- ~~Undefined Tailwind utility classes~~ → defined in `globals.css` (+ animation system).
- ~~Dead UI controls~~ → Donate→/signup, Learn More→#about, contact form wired to a server action.
- ~~`donor/[id]` params typing~~ → typed as `Promise<{ id: string }>`.
- ~~Unstyled `error.tsx` / oversized base font~~ → styled error page; base font set to 1rem.

### ✅ Resolved 2026-09-01 (Phase 0 — `WI-01`…`WI-10`, verified against a running stack)

All four P0 findings below are fixed and covered by an acceptance check. Original text kept for the record:

0a. ~~**Database password printed to logs.**~~ → `safeDSN()` redacts it; CI gate added and tested. `backend/main.go:71` — `fmt.Println("Connected using DSN:", dsn)`
    and the DSN embeds the credentials. Every container boot leaks them.
    _Fix:_ redact before logging, or drop the line. (`IMPLEMENTATION_PLAN.md` `WI-01`)
0b. ~~**Authorization bypass in `getAppointment`.**~~ → ownership is unconditional; owner 200 / non-owner 404 / no-param 400, verified. `backend/main.go:42-46` gates the ownership check on
    `if donorId != ""`, so it only runs when the *caller volunteers* `?donor_id=`. Omitting the
    parameter returns **any** donor's appointment. The guard is opt-in by the attacker.
    _Fix:_ derive identity from the session, never from a query parameter. (`WI-02`)
0c. ~~**Connection pool unbounded.**~~ → 25/25/30m/5m per TRD §11.3. No `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime`
    anywhere; the API can exhaust Postgres connections under load. (`WI-04`)
0d. ~~**`.gitignore` excludes `CLAUDE.md` and all of `docs/`.**~~ → both tracked; three credential-bearing files untracked. `git ls-files docs/` returns nothing —
    the designated single source of truth for progress, and the entire planning set, are untracked
    and invisible to anyone cloning the repo. _Decision required._ (`WI-06`)

### P1 — Remaining
1. **Session cookie is plain JSON** (`{role,id}`) — not signed/encrypted, so it's forgeable.
   _Fix:_ sign with an HMAC secret or switch to a JWT / encrypted cookie (e.g. `jose`).
2. **Open CORS (`Access-Control-Allow-Origin: *`).**
   _Fix:_ restrict to the frontend origin.
3. **Admin is still a hardcoded credential** in the login action.
   _Fix:_ promote admin to a real `donors` row with a `role` column.
4. **No appointment/request delete or cancel** (backend + UI).

### P2 — Polish & DX
5. **No automated tests / CI / lint gate.** A manual E2E smoke was run; add Go handler tests + a Playwright happy-path.
6. **Admin can't edit/delete donors or reject requests from the UI** (backend partially supports it).

---

## Suggested Next Steps (sequenced)

1. Sign/encrypt the session cookie (HMAC secret or JWT) — closes the last big auth gap.
2. Add a `role` column; replace the hardcoded admin login.
3. Lock CORS to the frontend origin.
4. Add appointment/request cancel + admin donor edit/delete UI.
5. First automated test pass (Go handler tests + one Playwright E2E), then CI.

---

## Changelog
- **2026-09-01** — **`WI-14` complete** (migrations `000004`–`000005`). `requests` →
  `donation_requests` (the old name meant the opposite of what it means in a real blood bank;
  `blood_requests` is reserved for the hospital demand side). `appointments` gains
  `scheduled_at TIMESTAMPTZ`, `center_id`, `status`, check-in/completion timestamps, and real FKs;
  the bare `DATE` column is gone, so a no-show is finally representable (defect D3).
  **Verified against real data, with `up → down → up`:** timestamps convert correctly
  (`09:00 Africa/Douala` → `08:00Z`); rollback recovers `appointment_date` from `scheduled_at`
  and re-derives `donor_name` by join.
  **The hard-`DELETE` damage is now measured, not just described.** `requests` was already empty —
  every approved request had been destroyed by `confirmRequest`. All 4 legacy appointments had a
  dangling `donation_request_id`; the link back to "who asked and when" was never persisted and is
  not recoverable. The quarantine ledger records all of it.
  **Defect found and fixed beyond the spec:** schema §11.5 quarantines a dangling
  `donation_request_id` but *not* a dangling `donor_id`. Appointment 1 belonged to the donor
  `000002` quarantined, so the documented migration would have aborted on `appointments_donor_fk`.
  Added an orphan guard to both `000004` and `000005` that quarantines the row with its full
  payload before removing it — the donor still exists in legacy `donors`, so the pair stays
  reconstructable. Result: 4 appointments → 3 migrated, 1 quarantined, 6 rows in the ledger.
- **2026-09-01** — **Phase 1 started; `WI-13` complete** (migrations `000001`–`000003`).
  Two blocking decisions answered by the project owner and recorded: **`OD-14` deployment
  timezone = `Africa/Douala`** (WAT, UTC+1, no DST), and **`OD-18` = no TTI override, ever** —
  `USER_JOURNEY.md` §5.4 corrected so it no longer contradicts `FR-28`/`FR-71`.
  Migrations landed: `000001` (4 extensions, 21 enum types), `000002` (`users`, `donor_profiles`,
  `migration_rejects` + backfill from `donors`), `000003` (`donation_centers`, `storage_locations`,
  `hospitals`, `policies`, `test_types`, `abo_compatibility`, the `users.hospital_id` FK, and a
  `MAIN` placeholder center on `Africa/Douala`).
  **Verified against the live database holding real donor rows**, not an empty schema:
  5 legacy donors → 4 `users` + 1 quarantined (`missing_password`), reconciling exactly; donor ids
  preserved as user ids (2,3,4,5) so `requests`/`appointments` FKs stay valid; free-text legacy
  values normalised (`'M'`→`male`, `'+'`→`positive`); `up → down → up` round trip clean with
  `donors` untouched throughout.
  **Two defects found and fixed in the documented migration:** (1) the schema doc's prose promised
  that rows with an unrecognised password hash are quarantined, but the SQL only tested NULL/empty —
  a non-empty plaintext password would have aborted the whole migration on `users_hash_not_plain`;
  the quarantine predicate now checks hash *and* email format, so the reject filter and the users
  filter agree exactly. (2) Added a pre-flight guard for case-only duplicate emails (`donors.email`
  is `TEXT`, `users.email` is `CITEXT`), **tested by injecting a duplicate**, with the operator
  recovery procedure (`force` → resolve → `up`) documented in the migration itself.
- **2026-09-01** — **Phase 0 complete (`WI-01`…`WI-10`).** Security and platform hygiene, verified
  against a running stack (12/12 acceptance checks): DSN redacted in logs (`safeDSN`); the
  `getAppointment` ownership bypass closed — owner `200`, non-owner `404`, missing param `400`,
  each confirmed with real data; CORS moved from `*` to an env allowlist with `Vary: Origin`;
  connection pool bounded (25/25/30m/5m); `http.Server` timeouts + 20s graceful drain;
  `golang-migrate` adopted with a `000000_baseline` and a `migrate` compose service — the 38-line
  boot-time `CREATE TABLE` block is deleted; `log`/`fmt` replaced with `log/slog` JSON plus
  request IDs; `/healthz` and `/readyz` added; fail-fast config (exit 1, named error, no fallback
  DSN); `.env.example` added and `compose.yaml` credentials env-injected.
  **`.gitignore`:** `docs/` and `CLAUDE.md` un-ignored; `backend/backend.log` (which contained a
  committed plaintext DSN), the tracked `backend/bbank` binary, and `.vscode/settings.json`
  (five database passwords, including a superuser credential for an unrelated project) untracked.
  **These remain in git history — rotation is still required.**
  CI skeleton added; its no-credential-in-logs gate was validated by reintroducing the defect.
  Compose DB host port parameterised (`DB_HOST_PORT`) after 5433 was found occupied locally.
- **2026-09-01** — **Documentation pass: full planning set added** (~8,100 lines across six documents,
  all cross-referenced and ID-linked). `docs/PRD.md` (`FR-01`–`FR-83`, `NFR-01`–`NFR-26`),
  `docs/TRD.md`, `docs/USER_JOURNEY.md`, `docs/UIUX_BRIEF.md`, `docs/DATABASE_SCHEMA.md`,
  `docs/IMPLEMENTATION_PLAN.md` (116 work items, every FR/NFR covered). Core finding: the system
  models donors and appointments but never blood — no collection, screening, testing, components,
  inventory, expiry, hospitals, blood requests, issuance or traceability. Scope redefined
  accordingly; see the dual-ruler note in the Completion Snapshot.
  **Verified, not asserted:** the schema doc's DDL was executed against a live `postgres:18`
  container — 21 tables + 2 audit partitions, 21 enums, 4 views, 18 triggers, 43 FKs, 263 CHECK
  constraints, clean. The TTI release gate was functionally tested: an untested unit is blocked
  (*"5 mandatory TTI test(s) missing or not non_reactive"*), a single reactive result blocks release,
  an all-non-reactive panel releases the unit, and `unit_status_events` records both transitions
  automatically. Three new P0 security findings recorded above, each confirmed against source.
  Also added `Mistakes.md` at the repo root per the global instruction.
- **2026-06-24** — Frontend redesign pass: swapped Geist Sans for Outfit (more character), tinted shadows/borders with warm hue instead of pure black, added grain overlay, squircle avatars, card-soft variant. Staggered "How It Works" layout (2+1 instead of 3 equal columns), organic stats numbers (10,847 etc.), body width constrained with max-w-prose, richer stat cards with background overlays. Custom 404 page, privacy/terms pages, root loading.tsx with skeleton, improved dashboard empty states (icon + heading + description). Skip-to-content link, OG meta tags, focus-visible rings on sidebar, legal links in footer, social icons replaced with contact info. Non-generic placeholder names throughout. Fixed ToastAlert lint error.
- **2026-06-11** — Docker startup reliability fixes (from live `compose up` failure logs):
  (1) `goapp` was racing Postgres — `db.Ping()` failed during DB init, `log.Fatal` exited the
  container, and the frontend then got `EAI_AGAIN goapp` (DNS for a dead container). Fixed with a
  30×2s connection-retry loop in `main.go`, `depends_on: db: condition: service_healthy`, and
  `restart: unless-stopped` on goapp + frontend. (2) Healthcheck `pg_isready -U admin` spammed
  `FATAL: database "admin" does not exist` every 5s (dbname defaults to username) — now `-d bbank`.
  Verified: full stack up, frontend container resolves goapp and fetches donors, 0 FATALs in db logs.
- **2026-06-11** — Theme flip: dark → **light, abstract-minimalist**. Removed the glassmorphism
  effect in favour of a simple `.blur-panel` (translucent white + `blur(10px)`) used by the navbar,
  hero chips and toast. All tokens in `globals.css` flipped to a white/stone canvas with quiet
  black-alpha borders, flat white cards with minimal shadows, light badge/table/avatar/field
  variants, and soft abstract `.blob` shapes + faint color washes replacing the heavy dark mesh
  gradients. Swept every page/component (navbar, footer, landing, auth, sidebar, all dashboard
  pages, settings, error, toast) from dark zinc/rose classes to light equivalents. Verified: clean
  `next build`, live render shows light canvas + blur panels, `/admin` guard still 307s to login.
- **2026-06-11** — Complete UI/UX reinvention to a contemporary product-design standard. New
  dark-first editorial design system in `globals.css` (layered canvas tokens, rose accent,
  Geist + Instrument Serif display pairing, `.card/.btn/.field/.badge/.table-modern/.avatar`
  primitives, spotlight-edge hovers, mesh + grid hero backdrops). New client primitives:
  `Reveal` (IntersectionObserver scroll reveals), scroll-aware `Navbar` with mobile menu,
  `SidebarNav` (Linear-style dashboard rail with active-route states). Landing page recomposed
  (hero with floating chips, blood-type marquee, stats, how-it-works, about, contact, final CTA,
  proper footer); auth pages as split-screen brand panels; all dashboard pages rebuilt with stat
  cards, avatar tables, status badges, profile-completion prompt; restyled toast + error pages.
  All server actions, sessions and route guards untouched. Verified: clean `next build` (12 routes),
  live render of `/`, `/login`, and `/admin` guard redirect (307 → login).
- **2026-06-11** — Hardening + UI/UX overhaul. Backend: bcrypt hashing, `POST /login`, password
  stripping, server-side validation, transactional `confirmRequest`, fixed DSN fallback. Frontend:
  centralized API base (`lib/api.ts`), cookie sessions (`lib/session.ts`), route guard (`proxy.ts`),
  logout, donor/admin settings pages, profile edit, `params` Promise fix; full visual refresh with a
  CSS design system + animations (`globals.css`), defined previously-undefined utility classes, wired
  dead controls, styled error page. Infra: `API_BASE_URL` in compose, pinned Turbopack root.
  Verified via clean `next build`, `go vet`, and an end-to-end API smoke test (signup→login→request→confirm).
- **2026-06-11** — Initial status assessment created from full codebase review (3 commits in history).
