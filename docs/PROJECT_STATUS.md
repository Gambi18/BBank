# BBank — Project Status

> **Living document.** Update this file whenever a feature lands, a bug is fixed, or scope changes.
> Last updated: **2026-09-02** · Branch: `oc-redesign-skill-refactor`
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

**Overall: ~77% of legacy scope · ~10% of documented scope.** The area table below still measures
the legacy scope; it will be rebaselined against the new scope at the `WI-06` boundary
(`IMPLEMENTATION_PLAN.md` §3), at which point several percentages deliberately go *down*.

> **The app works end-to-end again.** `WI-19` closed the migration window `WI-20` opened: the
> frontend now sends the ES256 token the API has required since `WI-20`, and the donor and admin
> pages render real data instead of 401ing. Verified against the running stack — login, the full
> six-role route matrix, token tampering, refresh rotation and logout revocation (see the
> changelog entry for the evidence).

| Area                     | Status | Notes |
|--------------------------|:------:|-------|
| Backend API (CRUD)       |  96%   | **`internal/legacy` is deleted — the strangler finished** (`WI-22`). Every endpoint is layered (`cmd/api`, `internal/{domain,service,store,http}`), with one pgx pool instead of two. Approving a request is now a **status transition, not a DELETE**. `WI-21`: `/api/v1` is canonical and enveloped, `/api/go` is a rewriting alias with `Deprecation`/`Sunset`, list endpoints are bounded, and idempotency storage is live . The 2026-09-02 sweep made the donor update a real **PATCH** (it had been writing NULL over every omitted field, erasing emergency contacts), stopped two ordinary inputs — an invalid rhesus and an over-`int32` `?limit=` — from returning 500, and started the two retention sweeps the schema documents but nothing ran |
| Frontend UI / pages      |  97%   | Design refinement pass: Outfit font, tinted shadows, grain overlay, staggered layouts, richer empty/loading states, custom 404, legal pages, OG meta, skip-to-content. `WI-22` restored every switched-off write path — signup (with auto-login), donor profile edit, admin add-donor — and added the reject UI, whose reason list is **read from the API** rather than hardcoded. Four role areas (`/staff`, `/lab`, `/inventory`, `/hospital`) are honest placeholders. The 2026-09-02 sweep fixed what the admin console **showed**: appointment status was a hardcoded green "Confirmed", so cancelled and missed appointments read as confirmed; `/donor/{id}` listed the caller's own appointments under another donor's heading; and the dashboard counted one 25-row page instead of `page.total` |
| Auth & Security          |  98%   | **ES256 JWT + rotating refresh** (`WI-17`) now **verified on every request**, with the full TRD §7.6 permission matrix and §7.7 ownership enforced as middleware (`WI-20`). Ownership comes from the `sub`/`cid`/`hid` claims — never from a query parameter. **The frontend now verifies the same token** (`WI-19`): `proxy.ts` on the Edge runtime, server components on Node, one `jose` module for both. **`WI-18` closes the last credential gap**: the first admin is bootstrapped from an env-supplied one-time *invitation*, never a literal, and invite / suspend / reactivate / role-change are live. A suspended account stops working on its **next request** — verified over HTTP with a live token. The 2026-09-02 sweep closed five holes in that work: the login lockout was **counted but never applied**, so `NFR-12` was unreachable; TRD §5.1's bcrypt cost reached only the dummy hash, making the timing guard a louder oracle than none; a suspended account's invitation stayed live; and the one-time invite token was stored both in the replay table and in a redirect URL |
| Data model / DB          |  97%   | **Schema complete.** Migrations `000000`–`000016` verified `up → down -all → up` on a fresh database: 26 tables, 21 enums, 4 views, 82 indexes, 18 triggers, 43 FKs, 14 policy rows, plus `000016` `idempotency_keys` (`WI-21`). Remaining: drop-legacy-donors, deliberately deferred to `WI-37` |
| DevOps / Docker          |  92%   | + golang-migrate, `migrate` compose service, env-injected secrets, server timeouts, graceful shutdown, structured logs, `/healthz` + `/readyz` |
| Testing                  |  73%   | CI skeleton (gofmt, vet, build, golangci-lint, govulncheck, tsc, eslint, npm audit, gitleaks, migrate up/down/up). Unit tests for the domain (ABO, blood groups, seed cross-check, RBAC matrix + transitions) and the authorization middleware — **all 660 matrix cells asserted over HTTP, granted and denied**. `WI-21` adds the legacy-path rewrite table, the runtime shim toggle, pagination clamping, and idempotency replay/reuse/in-flight/5xx-release. `WI-22` adds the donation-request state machine and the FR-09 rejection vocabulary — including a test asserting no reason describes the *person* rather than the request. **`WI-29` adds the integration harness**: real PostgreSQL 18 via `testcontainers`, migrations applied with the same `golang-migrate` production uses, **34 tests** covering the approve/reject lifecycle, genuine concurrency (8 simultaneous approvals → exactly one appointment), refresh-token reuse revoking a family, idempotency claim/replay/release, and the legacy `requests` → `donation_requests` migration against a fixture containing the rows the old `confirm` deleted. Coverage gated in CI: **domain 99% (gate 90%), service 72% (gate 70%)**, verified non-vacuous. **`WI-30` adds the HTTP-layer regression suite**: 18 tests named for the requirement or defect each guards, driving the *real* router against a real database — the `WI-02` ownership regression with no `donor_id` parameter, the §7.6 matrix as mounted, auth boundaries, `TD-15` driver-error leakage, `TD-17` pagination bounds, the shim toggle, and `A8`. It caught a live authorization leak on its first run. **The 2026-09-02 correctness sweep adds 11 more**, one per defect, and — because two of them turned out to prove nothing — establishes the discipline as a step, not an intention: every guard was deleted and its test watched go red. The admin-demotion race test passed with the row lock removed until it was rewritten to run eight concurrent demotions. **`WI-25`/`WI-26` add the clinical suites**: TRD §13.3's eligibility boundary table one case per boundary (17/18/65/66, first-time 60/61, weight 49.9/50/50.1, Hb ±0.1 at both thresholds, the 56-day interval at 54/55/56/57), shelf life per component with **the DST and leap-year cases §13.3 calls "a real and embarrassing bug"**, and the `FR-19` gate driven through the service. `TestTheViewAndTheDomainAgree` runs eight donor fixtures through both the SQL view and the Go domain and fails if they ever differ. Coverage: domain 96.2%, service 77.3%. Remaining: browser E2E, and `FR-19` at check-in and collection (`WI-39`, `WI-44`) |
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
- [x] Add a donor from the admin console (`WI-22`)
- [x] Reject a request from the UI, with a reason from a controlled list (`WI-22`)
- [ ] Delete / deactivate a donor from the UI (`WI-31`)

### Backend
- [x] Donors: GET list, GET one, POST, PUT, DELETE
- [x] Requests: GET list, GET one, POST, POST confirm (transactional)
- [x] Appointments: GET list (with `donor_id` filter), GET one
- [x] Auth endpoint (`POST /login`, bcrypt verify)
- [x] Password hashing (bcrypt) + no password in any response
- [x] Basic input validation (required fields, email normalization)
- [x] Canonical `/api/v1` prefix; `/api/go` a deprecated rewriting alias (`WI-21`)
- [x] Response envelopes everywhere — no bare arrays, no raw driver errors (`WI-21`)
- [x] Bounded list endpoints: `?limit=&offset=`, clamped, applied limit reported (`WI-21`)
- [x] Idempotency storage + replay middleware (`WI-21`; enforcement in `WI-77`)
- [x] `internal/legacy` deleted — every endpoint layered (`WI-22`)
- [x] Donation-request state machine; decided states are terminal (`WI-22`)
- [x] Requests: reject (coded reason) and cancel endpoints (`WI-22`)
- [x] Donors: create (public self-registration) and update (`WI-22`)
- [x] Appointments: cancel + reschedule, and the idempotent no-show sweep (`WI-23`)
- [x] Donor update is a real PATCH — an omitted field keeps its stored value
- [x] Clinical thresholds read from `active_policies`, none in Go source (`WI-25`, gated in CI)
- [x] Eligibility domain: every failing criterion named, with a plain-language reason (`WI-25`)
- [x] Component shelf lives from policy, DST- and leap-year-safe (`WI-25`)
- [x] **`FR-19` deferral enforcement on booking**, server-side, with the eligible date (`WI-26`)
- [x] Permanent-deferral override: admin only, reason required, logged (`WI-26`)
- [ ] `FR-19` at check-in and collection (`WI-39`, `WI-44`)
- [x] Donation centres: CRUD, opening hours, per-slot capacity (`WI-24`)
- [x] **Over-booking impossible under concurrent approvals** — a constraint, not a check (`WI-24`)
- [x] Deactivating a centre stops new bookings and preserves history (`WI-24`)
- [x] A donor can choose a centre and a preferred date when booking (`WI-24`)
- [ ] Donation intervals for `apheresis_plasma` and `double_red_cell` — clinical values needed
- [ ] Session/JWT issuance from the backend (currently the frontend owns the cookie)

### Cross-cutting
- [x] Authentication & session management (httpOnly cookie)
- [x] Route protection / authorization (admin vs donor vs owner) — Next `proxy.ts`
- [x] Password hashing
- [x] Environment-based API base URL
- [x] Custom 404 page
- [x] Privacy policy and terms of service pages
- [x] Signed session token — ES256 JWT, backend-issued (`WI-17`)
- [x] Backend verifies it on every request — `Authenticate` middleware (`WI-20`)
- [x] RBAC: the TRD §7.6 matrix enforced as middleware, deny by default (`WI-20`)
- [x] Ownership derived from the token, not from `?donor_id=` (§7.7, closes A14 properly) (`WI-20`)
- [x] `users.center_id` → the `cid` claim, so `ctr`-scoped grants actually resolve (`WI-20`)
- [x] Frontend verifies it — `jose` ES256 in `proxy.ts` and `session.ts` (`WI-19`)
- [x] Sign-out revokes the refresh family server-side, not just the cookie (`WI-19`)
- [x] Silent access-token refresh with rotation, and a loop guard (`WI-19`)
- [x] First automated tests (domain: ABO compatibility, blood-group parsing, seed cross-check)
- [x] Architecture dependency rule enforced in CI (`archcheck.sh`)
- [x] Integration harness: real Postgres 18 + real migrations (`WI-29`)
- [x] Coverage gates in CI — domain ≥ 90%, service ≥ 70% (`WI-29`)
- [x] No credential literal anywhere; first admin bootstrapped by invitation (`WI-18`)
- [x] Invite / suspend / reactivate / change role, with a last-admin guard (`WI-18`)
- [x] HTTP-layer regression suite, named per requirement, verified non-vacuous (`WI-30`)
- [x] Account lockout after repeated failed logins (`NFR-12`; the per-IP limiter is `WI-28`)
- [x] One-time invitation token never in a URL, a log, or the replay table
- [x] Retention sweeps: expired idempotency keys and sessions are deleted, not merely expired
- [x] Concurrency proven, not assumed: simultaneous approvals, duplicate signups, key claims
- [ ] Automated tests (handler, browser E2E) — `WI-30`
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
0b. ~~**Authorization bypass in `getAppointment`.**~~ → ownership is unconditional; owner 200 / non-owner 404 / no-param 400, verified.
    **Fully closed by `WI-20` (2026-09-01):** the check now compares against the token's `sub`
    claim, not against `?donor_id=`. `WI-02` had made the check unconditional but still trusted a
    caller-supplied value, so a donor could still assert someone else's identity. `backend/main.go:42-46` gates the ownership check on
    `if donorId != ""`, so it only runs when the *caller volunteers* `?donor_id=`. Omitting the
    parameter returns **any** donor's appointment. The guard is opt-in by the attacker.
    _Fix:_ derive identity from the session, never from a query parameter. (`WI-02`)
0c. ~~**Connection pool unbounded.**~~ → 25/25/30m/5m per TRD §11.3. No `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime`
    anywhere; the API can exhaust Postgres connections under load. (`WI-04`)
0d. ~~**`.gitignore` excludes `CLAUDE.md` and all of `docs/`.**~~ → both tracked; three credential-bearing files untracked. `git ls-files docs/` returns nothing —
    the designated single source of truth for progress, and the entire planning set, are untracked
    and invisible to anyone cloning the repo. _Decision required._ (`WI-06`)

### ✅ Resolved 2026-09-01 (`WI-19` — frontend session migration)

- ~~**The *frontend* session cookie is still plain JSON** (`bb_session` = `{role,id}`), forgeable.~~
  → `session.ts` and `proxy.ts` now verify an ES256 signature against `/api/v1/auth/public-key`.
  Flipping one byte of the cookie, forging `role: admin` into the payload, and an `alg: none`
  token were each verified to end at the login page. The `/api/go/*` 401 window is closed.
- ~~**Logout deleted the cookie but left the refresh family live.**~~ → Found while verifying
  `WI-19`, fixed in the same change. Logout was a server action, and a server action posts to the
  page's own URL — where the `Path=/api/v1/auth/refresh` cookie is never sent. The revocation call
  was therefore skipped for want of a token that could not arrive, and a stolen refresh token
  outlived the sign-out by up to 7 days while the user was told they had been signed out.
  Sign-out is now a route handler *inside* that path, and revocation is asserted in the database.

### P1 — Remaining
1. **Invitations are not emailed** (`WI-79`). The one-time link is returned to the inviting admin
   to pass on by hand. A deliberate stopping point: inventing a mail path here would be a second,
   unaudited way to send credentials.
2. **A codebase audit on 2026-09-02 found 3 high and 9 medium issues** — see the changelog entry.
   They are being worked in severity order.

_Resolved since this list was written:_ open CORS (now an explicit allowlist with `Vary: Origin`).

### P2 — Polish & DX
5. **No automated tests / CI / lint gate.** A manual E2E smoke was run; add Go handler tests + a Playwright happy-path.
6. **Admin can't edit/delete donors or reject requests from the UI** (backend partially supports it).

---

## Suggested Next Steps (sequenced)

1. **A decision, not a work item: the two missing donation intervals.** `apheresis_plasma` and
   `double_red_cell` are members of the `donation_procedure` enum with no `donation_interval_days.*`
   policy row, so booking either is refused with a 422 naming the gap. Adding them is one `INSERT`
   — but the values are clinical (commonly 14 and 112 days) and want a clinician's sign-off rather
   than a plausible-looking guess in a migration.
2. `WI-31` — admin donor edit/delete, password resets, and a confirmation dialog on destructive
   actions. `WI-18` shipped the operational minimum of `/admin/users`; this makes it complete.
3. `WI-27` — the audit log. `WI-26`'s permanent-deferral override currently writes a structured log
   line because there is nowhere better; `FR-19` asks for an audit record, and `WI-27` is where the
   `audit_log` row replaces it.
4. `WI-39` / `WI-44` — `FR-19` at check-in and collection. The gate exists and is called from one
   place; these are the other two the requirement names.
5. `WI-77` — turn idempotency from recorded to **required** on the endpoints §6.5 marks `Idem`.
   The storage and replay path landed in `WI-21`; the frontend stopped minting per-request keys in
   the 2026-09-02 sweep, so what remains is minting one per form render and setting the flag.
6. `WI-79` — email the invitation instead of handing the admin a link to copy.

**Done since this list was last written:** `WI-23` (appointment cancel/reschedule/no-show sweep),
`WI-30`'s HTTP layer, `WI-25`/`WI-26` (the policy resolver and the `FR-19` booking gate), and `WI-24`
(donation centres and slot capacity).

---

## Changelog
- **2026-09-02** — **`WI-24`: donation centres, and slot capacity as a constraint rather than a
  check.**

  **Over-booking is now impossible, not merely checked.** `FR-14`'s acceptance criterion is "two
  simultaneous approvals into a one-seat slot produce exactly one appointment", and a count-then-
  insert cannot deliver it: both transactions count zero, both see room, both insert. Migration
  `000019` gives a slot numbered **seats** — `appointments.slot_seat` plus a partial unique index on
  `(center_id, scheduled_at, slot_seat)` — so the second committer gets a 23505 and the service
  tries the next seat. A trigger keeps the seat inside the centre's capacity, so no caller,
  including psql, can widen a slot by inventing seat 99. Together they bound a slot at exactly
  `capacity_per_slot` with no lock held across the decision.
  A counting trigger would have been the same check-then-insert one layer down — under READ
  COMMITTED it cannot see another transaction's uncommitted row. Making the constraint a unique
  index moves the decision to the one place Postgres already serialises.
  The retry is at the **transaction** level, because a unique violation aborts the transaction it
  happens in; rolling back also undoes the status transition, which is what makes retrying safe.

  **`scheduled_at` is now the slot start.** Every appointment used to be created at a hardcoded
  09:00, so "capacity per slot" would have meant four donors per centre per day, all at nine. A
  requested time is snapped **down** to the centre's grid — measured from each opening interval's
  start, not from midnight, because a centre opening at 08:15 has slots at 08:15 and 08:45 and a
  midnight grid would offer 08:00, fifteen minutes before anybody is there. Down rather than to the
  nearest, so a request for 09:29 books the 09:00 slot it was for. A time outside opening hours is
  refused rather than relocated: silently moving a booking hides the mistake from whoever made it.

  **`opening_hours` had a column and no definition.** It has one now, in `DATABASE_SCHEMA.md` §6.2:
  a list of `[open, close]` pairs per weekday, because a lunch break is the ordinary case. Three
  states that are all different — `{}` means nobody has configured hours and bookings fall back to
  the historical 09:00; a missing day key means **closed**; an empty list also means closed. A
  centre that has filled nothing in keeps working, because an unset column is an administrative gap
  and not a decision to shut.

  **Centre CRUD** at `/api/v1/centers`, with `GET` public per TRD §6.5 — the directory is no PHI,
  and a donor deciding whether to sign up needs to know where they would go. `code` is not
  editable: it is printed on labels and quoted in reports, and a centre needing a different code is
  a different centre. Deactivating removes a centre from the public directory, refuses new requests
  **and** new approvals, and leaves every existing appointment alone and finishable — closing a
  centre must not strand the people already booked into it. Lowering capacity below what a slot
  holds is allowed and cancels nobody: an administrator editing a number is not a scheduling
  decision the system should make on their behalf.

  **Two defects found while building it, both in code written earlier today.**
  `DonationRequestService.Create` never checked the centre at all — only approval did — so a donor
  could raise a request against a closed centre and be told "we will confirm a date soon" for a date
  that could never come, while staff inherited a queue they could only reject.
  And the test harness had the `policies` problem again in a second form: `donation_centers`
  *survives* `TRUNCATE` but is *mutated* by tests, so one deactivation test made three unrelated
  tests fail with "that centre is not currently taking bookings" — in a different file from the
  cause. `Truncate` now restores both reference tables from a snapshot captured before the first
  truncate; centres are repaired in place rather than reinserted, because `storage_locations`
  references them `ON DELETE RESTRICT`.

  Coverage: domain 96.5%, service 75.8%. Migration `000019` round-trips. Still open and deliberately
  not guessed at: `apheresis_plasma` and `double_red_cell` have no seeded donation interval, so
  booking either is refused with a 422 naming the gap — those are clinical constants, and inventing
  two of them in passing is not a thing to do. The `/admin/centers` console is `WI-89`'s.

  **Follow-up, same day:** the donor booking form gained a centre select and a preferred date, both
  optional — a donor who just wants to give blood presses the button and the API picks the main
  centre a week out, and requiring either would put two decisions in front of the commonest action
  on that page. The select is hidden when there is only one centre, because a choice of one is not a
  choice. Adding a `min` date to the input turned out to be a lint error (`Date.now` during a server
  render), and chasing it found something better: **nothing refused a booking for a past date.** The
  eligibility gate answers "were you eligible last Tuesday?" perfectly happily, so the request would
  have sat in the queue for a day staff cannot schedule. Refused in the service now, compared on the
  date so a same-day walk-in still books.

  **The review found twelve defects in `WI-24`, two of them critical, and I had independently caught
  two of those twelve while it ran.** Worth listing, because none of them broke a test:
  the migration added `slot_seat NOT NULL DEFAULT 1` and then a unique index over it, which is
  **unbuildable on any database with two appointments at one centre on one day** — every legacy row
  sits at 09:00, so the migration would have failed and the API would never have started;
  `SlotFor` inferred "the caller named a date" from an instant reading midnight in the centre's
  timezone, but the handler produces midnight **UTC**, so at UTC+1 every approval at a centre with
  configured hours returned 409 — the feature broke the moment an administrator used it;
  `Reschedule` wrote the caller's raw timestamp and kept the old seat, so an off-grid time became a
  slot no capacity rule could see, a taken seat became a 500, and a since-reduced capacity became
  another 500;
  the FR-14 closed-centre guard was skipped on the commonest request shape of all — a donor posting
  `{}`, whose centre the insert defaults to `MAIN`;
  `SlotsOn` had no fallback where `SlotFor` had one, so the shipped centre offered no slots while
  booking at it worked; `{"timezone":""}` silently relocated a centre to UTC because
  `LoadLocation("")` returns UTC without error; `?date=` resolved to the previous day west of UTC;
  `{"mon": []}` collapsed to "unconfigured" and reopened a centre the administrator had closed;
  `SlotsOn` could loop for ever on a non-positive slot length; `is_active` was dropped on create;
  and centre pagination had no unique tiebreaker.
  Every one is fixed with a test named for the defect, and the five substantive ones were each shown
  to fail with their fix removed.
- **2026-09-02** — **`WI-25` and `WI-26`: clinical policy becomes data, and `FR-19`'s deferral gate
  goes in. Plus the empty `policies` table nothing had noticed.**

  **`WI-25` — the policy resolver.** Every clinical threshold the system applies — age band, weight,
  haemoglobin by sex, vitals, donation intervals, annual caps, shelf lives, expiry windows — is now
  read from `active_policies` at decision time. `internal/domain/policy.go` names the keys and
  nothing else; `eligibility.go` and `shelflife.go` decide with whatever the rows say.
  **A missing policy is an error, not a default.** `ErrPolicyMissing` stops the decision, because a
  deleted threshold that silently falls back to a number in the source is a deleted threshold nobody
  can see — the precise failure `FR-20` exists to prevent. `backend/noclinical.sh` gates it in CI:
  planting `const x = 56` fails the build, the same number in a comment does not.
  Each decision carries a **policy version** — a fingerprint of the rows that produced it — so two
  decisions made under different numbers are distinguishable afterwards (`FR-68`).

  **`WI-26` — the `FR-19` booking gate.** An active deferral or an open interval window now blocks a
  donation request **in the service**, not the handler: the acceptance criterion is "the block
  cannot be bypassed by calling the API directly", and a check in one handler is bypassed by the
  next caller of the same method. The refusal carries every failing criterion with a sentence
  written for the donor and the date it clears (`FR-08`, `FR-17`) — `409` with `next_eligible_on`,
  an additive field recorded in TRD §6.2. A permanent deferral is overridable **only by an admin,
  only with a reason**, and the override clears that one criterion and nothing else.
  The gate decides for the date being **booked**, not for today: a donor whose interval elapses next
  Tuesday is booking for next Tuesday.

  **`policies` was empty in every integration test, and had been since `WI-29`.**
  `policies.created_by` references `users`, and `TRUNCATE users … CASCADE` propagates to every table
  with a foreign key pointing at it — cascade truncation ignores the `ON DELETE SET NULL` that
  column declares. So the harness emptied the clinical policy table before every test while its own
  comment claimed it left seed data intact. Nothing caught it because nothing read policy until now:
  the seed cross-check tests parse the migration *file*, and `donor_eligibility` COALESCEd a
  hardcoded fallback for each threshold, so it kept answering with 56 days and 18 years from an
  empty table. `Truncate` now restores the seed, captured from the freshly migrated database rather
  than copied into Go.

  **Migration `000018` makes the view and the domain agree.** Two of them: the view's deferral
  boundary was off by one *against itself* — it filtered deferrals with `ends_on > CURRENT_DATE`, so
  a deferral ending today was already over, and then reported `next_eligible_on = deferred_until + 1`,
  telling a donor to come back the day after the day it would itself have let them book. And the
  COALESCE fallbacks are gone: with no policy row the view now answers `is_eligible_today = NULL`
  and `reason = 'policy_unavailable'`, the same "cannot currently decide" the API gives, instead of
  quietly applying a constant from a migration. `TestTheViewAndTheDomainAgree` drives eight donor
  fixtures through both paths and fails if they ever differ.

  **The review found eleven defects in this work, and seven were real bugs in it.** Worth recording
  what they were, because five of them had passing tests over them:
  the gate evaluated a `procedure` the insert never stored, so `{"procedure":"apheresis_platelet"}`
  passed the 7-day interval and filed a **whole-blood** request; approval created the appointment for
  a staff-chosen date with **no eligibility check at all**, so typing an earlier date scheduled a
  donation three weeks inside the interval; the gate defaulted an absent date to today while the
  insert defaulted it to a week out, refusing bookings that were fine; an override on a refusal with
  no deferral in it discarded every real criterion; a non-admin's override was never validated for a
  donor who happened to be eligible; a procedure with no configured interval reached the client as a
  **500**; and a region-scoped policy row would have applied one region's thresholds to every
  decision everywhere. Each is fixed with a test named for the defect, and each of those tests was
  shown to fail with its fix removed.

  **And the approval re-check deadlocked the connection pool on its first run.** It called the
  pool-wide eligibility service from inside the approving transaction, so every goroutine held a
  transaction and a `FOR UPDATE` lock while waiting for a second connection another goroutine was
  holding; eight concurrent approvals hung until the suite timed out at ten minutes. The eligibility
  read now runs on the caller's own transaction, and the policy snapshot is resolved before the
  transaction opens. A named test drives twelve concurrent approvals with a sixty-second deadline.

  Coverage: domain 96.2%, service 77.3%, both gates green. Not done here, and deliberately:
  `apheresis_plasma` and `double_red_cell` have no seeded interval — inventing two clinical
  constants is not a thing to do in passing, so booking either is refused with a 422 naming the gap,
  and `WI-24` adds the rows. Check-in and collection get the same gate in `WI-39` and `WI-44`.
- **2026-09-02** — **A correctness sweep: 17 defects across the work of the last five items, each
  fixed with a named test that fails without the fix.**
  No new feature. An audit of everything `WI-18` through `WI-23` touched, on the principle that the
  cheapest defect is the one found before the next work item is built on top of it. Grouped by what
  they would have cost:

  **Security.** `RecordFailedLogin` incremented the counter and never set `locked_until`, so the
  `ErrAccountLocked` branch, the 423 status and the frontend's "temporarily locked" message were all
  **unreachable** — online password guessing was unbounded (`NFR-12`, and the reason `WI-28` is now
  partly done). `BcryptCost` — TRD §5.1's cost 12 — was applied **only** to the dummy hash on the
  user-not-found path, while every real password took the library default of 10; that inverted the
  defence it exists for, making "no such account" cost ~4× a real comparison and turning the timing
  guard into a *louder* enumeration oracle than none. Suspending an account left its outstanding
  invitation live, so an admin who thought better of an invite could still have the invitee activate
  a working account; `AcceptInvite` also never checked the account's status. Both are now closed
  independently, and the test pins each separately — with only the combined assertion, deleting
  either left the suite green. The invitation token was stored in the **idempotency replay table**
  as a response body: a one-time credential at rest, which is exactly what hashing it everywhere
  else was meant to prevent (`WithoutResponseBody`). And the frontend put that same token in a
  **redirect query string** — the address bar, browser history, the reverse-proxy access log, and
  the `Referer` of anything the page then loaded. It now travels in a short-lived HttpOnly cookie.

  **Data loss.** `UpdateDonorProfile` was a full-row `UPDATE` of 12 columns, so every save wrote
  NULL over any field the caller omitted — and `/donor/settings` sends neither `national_id` nor
  either emergency contact. **Saving a phone number erased the person to call in an emergency.** The
  query now `COALESCE`s each column, which is what PATCH means, and the domain check runs against
  the *resulting* record rather than a half-empty struct.

  **Authorization.** §7.6 grants staff `CRU` on `donor_profiles` scoped `ctr`, but `donor_profiles`
  has no centre column — deliberately, since a donor may attend any centre — so `ctr` was
  unimplementable and the two code paths **guessed differently**: `list` fell through to the entire
  registry while `resolveOwned` asked `Permits` for a row with a nil centre, which `ScopeCenter`
  always rejects. Staff could see every donor at once and none of them individually. TRD §6.5
  settles it — registry search, `center_id` an optional filter — and both paths now read one
  function, default-deny, so a scope added later inherits nothing by omission.

  **Correctness under concurrency.** `ensureNotLastAdmin` counted admins outside any transaction, so
  concurrent demotions all saw a safe number and all succeeded, leaving **zero admins** — a state
  with no recovery short of SQL. The count and the write now share a transaction and the count holds
  a row lock on every active admin.

  **Crashes from ordinary input.** Rhesus was validated for *presence* but never for *value*, so
  `{"blood_group":"A","rhesus":"x"}` reached Postgres as an invalid enum and came back a 500 instead
  of a 422. `ParsePaging` compared `int32(n) > MaxLimit`, converting **before** comparing, so
  `?limit=2147483748` wrapped to a negative int32, skipped the clamp entirely and reached Postgres
  as `LIMIT -2147483548`.

  **Unbounded growth.** Two sweeps the schema documents and nothing ran: `idempotency_keys` has a
  24-hour TTL (§6.4) and `DeleteExpiredSessions` had no caller. Both tables grew forever, the first
  while retaining stored response bodies.

  **Things shown wrong.** Both appointment tables hardcoded a green "Confirmed" badge, so a
  **cancelled or missed appointment displayed as confirmed** — the API has returned `status` all
  along and neither page read it. `/donor/{id}` listed appointments with no donor filter, rendering
  the *caller's* appointments under that donor's heading; for an admin, that was every appointment
  in the system. The admin dashboard counted `items.length` — one page, capped at 25 by §6.3 — so a
  busy inbox would have read "25 pending" forever. And `apiClient` minted a fresh
  `crypto.randomUUID()` per request as the `Idempotency-Key`, which **deduplicates nothing**: a
  double-submitted form produced two different keys and two rows, while filling the replay table
  with entries nothing would ever match. A key must identify the user's *intent*, so it has to be
  minted where the intent is — `WI-77` wires that through.

  **Every fix has a test, and every test was shown to fail without its fix.** That check was worth
  doing: the first version of the admin-demotion race test **passed with the row lock removed** —
  two goroutines never overlapped, so it proved nothing. It now runs eight concurrent demotions,
  which empties the admin role reliably without the lock. Two of the invitation guards were likewise
  unpinned, each satisfied by the other. `internal/domain` 98.5%, `internal/service` 75.3%,
  both gates green; the generated store is unchanged and in sync.
- **2026-09-02** — **`WI-23`: appointment cancel, reschedule, and the no-show sweep.**
  `FR-11` and `FR-13`. Both mutations lock the row before reading its status, so the check and the
  write cannot straddle another transaction, and both are authorized against the **locked** row —
  the row that was checked is the row that gets written.
  **Cancel and reschedule stop at check-in, deliberately.** Once a donor is checked in the
  appointment is a clinical encounter in progress, and moving it would rewrite when that encounter
  happened; once it has passed, the honest record is `no_show` or `completed`, not a quietly
  relocated slot. `check_in` itself is absent from the state machine until `WI-39`, which must pair
  it with the `FR-19` deferral block — pre-granting it here would let that requirement be skipped.
  **The sweep creates no deferral**, which is the acceptance criterion and also the point: missing
  an appointment is administrative, and recording it as a clinical deferral would put a mark on a
  donor's record that no clinician made — one that `donor_eligibility` would then act on to block
  their next booking. It is idempotent by construction (it matches only `scheduled` rows past their
  grace window), so it needs no leader election and a second replica running it is harmless. The
  grace period is 6 hours: a donor stuck in traffic is a donation, not an absence.
  `scheduled_at` is taken as RFC3339 rather than a bare date, so the caller states the offset
  instead of the server guessing one — the deployment timezone is still an open question (schema Q6).
- **2026-09-02** — **`WI-30` (partial): the HTTP regression suite — and the authorization leak it
  found on its first run.**
  `WI-29` proved the rules are *implemented*. Only an HTTP test proves they are *mounted*, and that
  is the gap three consecutive work items had been closing by hand. `WI-20`'s own changelog states
  the risk — "a matrix that passes its unit tests can still be mounted on the wrong routes" — and
  answered it once, manually, against a running stack. These 18 tests answer it on every CI run,
  driving the **real router** with the real middleware chain against a real database.
  **It immediately earned itself.** `GET /api/v1/users` returned **every account** — email, role,
  status, last login — to any authenticated **donor**. The §7.6 matrix was correct: a donor holds
  `R` on `users` scoped `own`, so they may read *their own* record. `UserHandler.list` (added in
  `WI-18`, one commit earlier) checked the permission and never consulted the **scope**. Holding a
  permission and being allowed to see every row are different questions, and it answered only the
  first. `GET /api/v1/users/{id}` had the same hole — any donor could read any account by id.
  Both are fixed: the list narrows to self for any scope short of `all`, written as a default-deny
  switch so a scope added later is refused rather than silently inheriting the unfiltered list, and
  the single read goes through `resolveOwned`, answering **404 rather than 403** because a 403
  confirms the account exists. Verified non-vacuous: reintroducing the defect makes the guard fail
  with a message naming what leaked.
  **Each test is named for what it guards**, per the acceptance criterion — `TestWI02_…`,
  `TestFR66_…`, `TestTD15_…` — so a failure says which requirement broke rather than that something
  did. Covered: anonymous refused on all 16 guarded routes and the public ones still public; the
  matrix as mounted; a donor refused approval of their own request; **the `WI-02` regression at the
  level the defect lived at** — an appointment read with *no* `donor_id` parameter at all; the
  filter that can narrow a scope but never widen one; cross-centre 404s; auth boundaries; a
  suspended user's live token dying on its next request; no driver error in any of 12 error paths;
  pagination clamped with the applied limit reported; the shim toggling 200 → 410 → 200; and
  approving not deleting the request.
  **A weak test of my own, caught and fixed.** The token-tampering check flipped the *last*
  character of the signature — but an ES256 signature is 64 bytes in 86 base64url characters, so
  the final character has 4 unused bits and several spellings decode identically. The test passed
  while proving nothing. It now flips a byte in the middle and asserts the token actually changed.
  Backend overall coverage 44% → 59%; `go test -short` still passes with no Docker.
  **Not done:** `FR-19` deferral enforcement, which `WI-30` also calls for — it needs `WI-26`, which
  needs `WI-25`. Browser journeys (TRD §13.5) remain manual.
- **2026-09-02** — **`WI-18` complete: the hardcoded admin is gone, and there is a supported way to
  create every role.**
  The credential literal stopped *working* at `WI-17`, when a session became something only the API
  can sign. What remained was worse than cosmetic: five of the six roles could be created only with
  hand-written SQL, so the honest options were a backdoor or a DBA. This is the third one.
  **The first admin is an invitation, not a password.** `BOOTSTRAP_ADMIN_EMAIL` creates the account
  with **no password anyone knows** and prints a one-time token to the startup log — and only when
  no active admin exists, so leaving the variable set re-opens nothing on later boots. Verified
  both ways against the running stack: with an admin present it logs "skipping"; with none it
  minted an invitation, the account refused login until accepted (403), accepting it worked (204),
  replaying the token failed (400), and the new admin then logged in.
  **An invited account has a password nobody knows, not a placeholder.** `users.password_hash` has
  a CHECK requiring bcrypt/argon2 shape, so a sentinel string is impossible — and a *shared*
  sentinel would be a backdoor into every invitation that is never completed. Each one gets a
  discarded random 32-byte secret. Only the SHA-256 of the invite token is stored, as with refresh
  tokens (`WI-17`), so a database disclosure hands over nothing usable.
  **The user row is created at invite time, not at acceptance.** Email collisions surface in front
  of the admin sending the invitation rather than later; an invited-but-not-joined account is
  visible in the console, so "did I invite them?" is answerable; and the role/centre CHECK is
  validated immediately, so `staff` with no centre is a **422 naming the field** instead of a
  constraint violation reaching the caller as a 500.
  **The FR-66 criterion, verified over HTTP with a live token:** an admin suspends a staff account,
  and that account's *existing, unexpired* access token returns **401 on its very next request** —
  not at its next login. Two things make it true: `VerifyAccessToken` re-reads status on every
  request, and `SetStatus` bumps `token_version` and revokes the refresh families, so there is
  nothing left to refresh into. Role changes do the same, because the role is baked into the signed
  token and a demoted admin must not keep admin authority until it expires.
  **Two guards on the operation that can lock everybody out.** The last active admin cannot be
  suspended or demoted, and no admin can suspend or demote *themselves* — recovering from either
  needs another admin, and there is no path back from zero short of SQL.
  **Frontend:** a public `/accept-invite` page where an invitee sets their own password, and
  `/admin/users` — list with role/status/search filters, invite, suspend, reactivate, change role.
  `WI-31` adds password resets and a confirmation dialog; this is the operational minimum.
  Verified in the browser end to end: an admin invited a `lab_tech`, the invitee set a password
  from the emailed-style link, logged in, landed on `/lab`, and was redirected away from `/admin`,
  `/admin/users`, `/staff` and `/inventory`.
  **The coverage gate did its job.** Adding `UserService` dropped `internal/service` to 68.6% and
  CI would have failed; the missing list/bootstrap tests brought it to 74.1%. That is the gate
  working as designed rather than a number being massaged — `WI-29` built it two commits ago and it
  caught the very next change.
  Also fixed while in the console: the new page used `badge-success`/`badge-danger`, which do not
  exist in `globals.css` (the defined variants are `badge-green`, `badge-accent`, `badge-muted`).
  An undefined utility class renders as unstyled text — the trap `CLAUDE.md` warns about.
- **2026-09-02** — **`WI-29` complete: the test harness, and the first defect it caught.**
  Integration tests now run against **real PostgreSQL 18** through `testcontainers`, with the
  schema built by the same `golang-migrate` and the same migration files the `migrate` compose
  service uses — so "the schema the tests ran against" and "the schema production gets" are one
  artefact rather than two that agree until they don't.
  **A mock could not have tested any of this.** Half of what this system relies on *is* the
  database: partial unique indexes, CHECK constraints, enum types, `FOR UPDATE` row locks,
  `ON CONFLICT DO NOTHING`. A mocked test of the approval race proves nothing about the property it
  claims to test, because the property belongs to Postgres.
  **The concurrency claims are now proven rather than asserted.** Eight goroutines approving one
  request produce **exactly one appointment** and seven clean 409s; six concurrent signups on one
  email produce one account and five conflicts; ten simultaneous idempotency claims leave exactly
  one holder. Each of these was previously a comment explaining why the code was correct.
  **The migration test the plan asked for.** `WI-29`'s acceptance criteria call for the
  `requests` → `donation_requests` migration to be tested "against a fixture including the rows the
  old `confirm` deleted". It now is: a database is stood up at the baseline schema, seeded with a
  live request, an appointment linked to it, and **an appointment orphaned by a delete that already
  happened** — then migrated forward. The test asserts both appointments survive, the orphan keeps a
  NULL link rather than a dangling one, and the loss is **recorded in `migration_rejects`**, because
  a quarantine table nobody writes to is the same as no quarantine table.
  **Auth is tested at the service layer for the first time.** Refresh-token reuse revoking the
  entire family — the security property `WI-17` built and only ever checked by hand — is now a
  named test, along with `token_version` invalidating live access tokens, logout revocation, and
  login refusing to distinguish "wrong password" from "no such account" (`NFR-12`).
  **The defect it caught, honestly scoped:** `domain.AgeYears` decided whether a birthday had
  passed by comparing `YearDay()`, which is off by one whenever a leap day falls between the two
  dates — someone born 2000-06-15 read as 25 on the day they turned 26. It is **latent, not live**:
  nothing calls `AgeYears` yet, because the eligibility band is currently computed only by the
  `donor_eligibility` view. It would have surfaced when `WI-25`/`WI-26` wire the Go eligibility
  domain, as Go and SQL disagreeing about one donor, on a birthday, at exactly the 18-year boundary
  the policy cares about. Fixed, and `TestAgeYearsAgreesWithPostgres` now checks Go against
  `EXTRACT(YEAR FROM age(...))` across 84 date pairs — testing the two implementations against
  *each other* rather than each against a hand-written expectation, which a shared misunderstanding
  would pass.
  **Coverage is gated, and the gate has been seen to fail.** `backend/coverage.sh` enforces
  `internal/domain` ≥ 90% (now 99%) and `internal/service` ≥ 70% (now 72%), measured per package
  with `-coverpkg` pointed at itself — a repo-wide `-coverpkg` spreads every package's statements
  across every profile and collapses each number to a fraction of the truth. Raising a threshold
  above actual coverage was confirmed to exit 1, following this repo's rule that a gate you have
  not watched fail is not known to work. `internal/store` and `internal/http` are deliberately
  ungated: generated code and wiring, where a threshold buys tests written to move a number.
  **CI runs them as a separate job** so a Docker or image-pull problem reads as infrastructure
  rather than as broken code, and `go test -short` still passes with no Docker at all, which keeps
  the fast gates usable on a laptop.
- **2026-09-02** — **`WI-22` complete: `internal/legacy` is deleted, and every switched-off write
  path is live again.**
  The strangler finished. Donation requests and appointments moved into
  handlers → service → store, and the package that held the original single-file handlers is gone
  — along with the second `database/sql` connection pool it needed, so the API now runs on one pgx
  pool with one place to configure limits.
  **Authorization stopped being string handling.** The legacy code built its scope filter by
  concatenating `" AND r.donor_id = $1"` onto a query. It was correct, but a missing clause was a
  wider result set rather than a compile error. The scope is now an explicit `service.Scope`, nil
  meaning "not narrowed", decided once in `resolveScope` — and `?donor_id=` is a separate field
  from the token-derived owner, so a filter can narrow a result set and never widen one. Defect
  A14 is now a shape rather than a comment.
  **Approve is atomic and locked.** `GetDonationRequestForUpdate` takes a `FOR UPDATE` row lock
  before the status is checked, and ownership is evaluated on the locked row inside the same
  transaction. Without the lock, two staff approving at once both read `pending`, both pass the
  check, and the second insert dies on the UNIQUE over `appointments.donation_request_id` — a 500
  for what is really a 409. Verified: approving twice returns **409 `request is approved`**, and
  the request row survives as `approved` with a linked appointment (the `FR-09` acceptance
  criterion, checked in the database).
  **`FR-09`'s controlled vocabulary lives in `internal/domain`,** because the column is
  `rejection_reason TEXT` — the schema enforces that a rejected request *has* a reason, and the
  domain enforces that it is one we recognise. Free text is allowed only *alongside* a code, never
  instead of one, or the fulfilment report (`FR-61`) degenerates into prose nobody can aggregate.
  The list is **served from the API** (`GET /donation-requests/rejection-reasons`) so the dropdown
  cannot drift from what the server accepts. Every value describes the **request, never the
  person**: UI/UX §4 reserves "rejected" for requests, and a donor who cannot give today is
  *deferred*, a different concept with its own table. There is a test asserting no reason contains
  a word like "unsuitable" or "ineligible", because that failure would surface in a notification
  to a human being.
  **Decided states are terminal.** `pending` may go to approved, rejected, cancelled or expired;
  nothing comes back. Re-opening a decided request would let an appointment exist against a row
  that later reads `rejected`, and the audit chain would stop explaining itself.
  **Blood group is not self-reportable** (`FR-21`). Self-registration ignores it even when sent;
  staff and admin may set it. That asymmetry is the fix for defect D7 — the original system let
  people type their own blood type, which is how `"O+"`, `"o"` and `" A "` ended up in one column.
  A donor editing their profile also cannot blank the recorded value: the service carries it
  forward rather than erasing it because a form omitted the field.
  **Every stubbed form is live:** signup (which now signs the new donor straight in rather than
  ending at a login screen), donor profile edit, admin add-donor, and the new reject UI. No
  `fieldset disabled` remains anywhere in the app.
  **A bug I introduced and caught:** the donor-create path defaulted gender to `"unknown"`, which
  is not a value of the `gender` enum (`male | female | other | undisclosed`). Every signup failed
  with a generic 500. `domain.ParseGender` now owns the permitted set, `GenderUnstated` names the
  default so nobody has to invent a spelling, and an unrecognised value is a 422 naming the field.
  Logged in `Mistakes.md` as the fourth instance of this repo's most-repeated error — guessing an
  identifier instead of reading it.
  **Verified end-to-end against the running stack:** both prefixes still serve; a donor's request
  is created from the token with an empty body; a second one is **409**, not a constraint 500; a
  donor approving their own request is 403; an unlisted rejection reason and a noteless `other` are
  both 422; approving twice and rejecting an approved request are both 409; staff at centre 1 see
  3 of 4 requests and get **404** reading or approving centre 2's; a donor's `?donor_id=` is
  ignored; self-registration is 201 then logs in, a duplicate email is 409, and a self-claimed
  blood group is discarded; a donor editing another donor is 404. In the browser: signup →
  auto-login → dashboard, request an appointment, reject it from the admin console with a coded
  reason, and edit a profile — each confirmed in the database. Fixtures removed afterwards.
  **Note on the environment:** the host disk hit 100% mid-session and produced browser
  `ERR_INSUFFICIENT_RESOURCES` failures unrelated to the code. 10.2 GB of reclaimable Docker
  **build cache** was pruned (never images or volumes — volumes hold the databases).
- **2026-09-01** — **`WI-21` complete: `/api/v1` is canonical, everything is enveloped, and the
  deprecated prefix is an alias rather than a second API.**
  `/api/go/` no longer has handlers of its own. `middleware.LegacyShim` rewrites those paths to
  `/api/v1` **before chi routes**, so there is exactly one implementation of each endpoint and the
  two spellings cannot drift — the failure mode of the obvious alternative, two parallel route
  sets, where the legacy copy is the one nobody re-reads when a rule changes. Every legacy response
  carries `Deprecation: true`, `Sunset: Wed, 31 Mar 2027` and a `Link: rel="successor-version"`.
  **It is deliberately not a blanket prefix swap.** Only the endpoints in TRD §6.1 are aliased;
  anything else under `/api/go/` is a 404. A general alias would silently expose every *future* v1
  endpoint under a deprecated name with no deprecation clock of its own, which is precisely how a
  prefix scheduled for deletion becomes permanent.
  **The flag is genuinely runtime, because the alternative does not work.** The acceptance
  criterion is that the shim toggles without a redeploy, and an environment variable cannot do
  that — changing one means a restart. So `platform.Flags` holds an `atomic.Bool`, seeded from
  `LEGACY_API_SHIM` and owned thereafter by `PATCH /api/v1/admin/flags`. The point is the
  experiment the shim exists to enable: turn it off, watch, turn it back on in seconds if something
  screams. Make that a redeploy and nobody ever runs it. Switched off, the path answers **410
  Gone**, not 404 — retired by policy, not missing by accident, and the difference tells an
  integrator whether to fix their path or their calendar. The endpoint is gated by a new
  `RequireRole`, not by the §7.6 matrix: a feature flag is not a clinical resource, and the matrix
  denies resources it does not know.
  **No endpoint returns a raw driver error any more.** The 18 `http.Error(w, err.Error(), 500)`
  sites in `internal/legacy` are gone (`TD-15`). `response` now carries `meta{request_id,
  server_time}` on success as well as failure, structured `details[{field, issue}]`, and helpers
  for the codes §6.2 distinguishes — notably **409 for a domain refusal** versus 422 for a
  validation failure, because a well-formed request the world said no to is not a client error and
  clients branch on that.
  **`legacy` shrank rather than grew.** The five donor handlers went: they stopped being routed
  when `WI-11` moved donors to the layered path, so they were dead code that still leaked driver
  errors at whoever wired them back up. `POST /api/go/login` went too — it checked a password and
  returned a donor object *without issuing a session*, which after `WI-17` is not a login at all.
  The shim now points that path at the real auth handler, so an old caller gets a genuine ES256
  session. This is a **deliberate deviation from TRD §6.1**, which preserves the old response body:
  `WI-19` already migrated the only client, so there is nobody left to keep compatible, and a
  second password-checking code path is a liability, not a courtesy.
  **A live pagination bug found and fixed.** `GET /donors?limit=5000` answered `page.limit: 5000`
  while returning 100 rows: `normalise()` clamped a **copy** of the params inside the service, and
  the handler echoed back what the caller had asked for. A client's paging loop would advance by
  5000 and skip 4,900 records with no error to notice. Parsing and clamping now happen once, up
  front, in `response.ParsePaging`, and the value reported is the value applied. Over-limit is
  clamped rather than rejected (§6.3), but `limit=0` is a 400 — reading it as "no limit" is exactly
  how an unbounded scan comes back after being closed.
  **Idempotency storage is live** (migration `000016`, schema §14A). A row is claimed *before* the
  handler runs via `INSERT ... ON CONFLICT DO NOTHING RETURNING` — one atomic statement, so two
  simultaneous retries race and exactly one proceeds; a `SELECT`-then-`INSERT` would leave a window
  where both conclude they are the original. Uniqueness is `(actor_id, idem_key)`, so one user
  cannot burn another's key or be handed their stored response. A **5xx is not stored**: the claim
  is released, because freezing a failure against the key for 24 hours would punish the honest
  client for retrying correctly. `WI-21` records and replays; `WI-77` flips `required` on.
  **The frontend moved to canonical paths in the same change**, so the app we ship does not depend
  on a prefix we intend to delete. `/api/go/*` now has no first-party consumer at all.
  **Verified end-to-end against the running stack:** both prefixes serve; `Deprecation`/`Sunset`
  present on legacy and absent on canonical; the query string survives the rewrite; the flag
  switched the shim off (410) and back on (200) in a live process with no restart, while canonical
  paths were unaffected; a donor got 403 and an anonymous caller 401 on the flags endpoint;
  `?limit=5000` reported the clamped 100; four error paths returned codes with no `pq:`/`pgx`/
  `relation` text; two POSTs with one `Idempotency-Key` created **one** row and the second came
  back `Idempotent-Replay: true`, while the same key with a different body was 422; approve
  returned 201, left the request row present as `approved`, and created the linked appointment;
  a donor approving was 403. Migration `000016` verified `up → down → up`. Frontend `tsc`,
  `eslint`, `next build` and backend `go build`/`vet`/`test`/`archcheck.sh` all clean.
  Fixtures removed afterwards.
  **Deferred deliberately:** cursor/keyset pagination (§6.3's default). It matters for sets that
  grow while being paged — `blood_units`, `audit_log` — and neither has an endpoint before Phase 2.
  Offset pagination is correct for the bounded lists that exist today, and retrofitting cursors
  onto a moving table later is a smaller job than inventing them for tables that do not exist.
- **2026-09-01** — **`WI-19` complete: the frontend verifies the token, and the app works again.**
  `session.ts` and `proxy.ts` no longer `JSON.parse` a cookie the browser could edit by hand; both
  verify an ES256 signature through one runtime-agnostic module (`lib/jwt.ts`), which is what
  **closes `OD-16`**: `jose` uses Web Crypto, so the *same* code verifies on the Edge runtime in
  `proxy.ts` and on Node in server components. That is the property ES256 was chosen for in
  TRD `Q7`, now demonstrated rather than assumed. The frontend holds only the public half.
  **The `WI-20` migration window is closed.** `lib/apiClient.ts` is the single way this app talks
  to the API: it re-attaches the session cookie to every server-to-server call (the omission that
  made every page 401), unwraps both the `{data, page}` envelope and the bare arrays the legacy
  handlers still return, and stamps an `Idempotency-Key` on mutations so `WI-77` has something to
  switch on. `apiListOrEmpty` absorbs only `ApiError` — never Next.js's `redirect()`/`notFound()`
  control flow, which a blanket catch had been swallowing — and deliberately **re-throws 401**,
  because rendering "0 donors" to someone whose session expired is a lie.
  **A defect found in this work item's own code, fixed here.** Logout was a server action, and a
  server action posts to the page's own URL — a path `bb_rt` (`Path=/api/v1/auth/refresh`) is never
  sent to. So it read `undefined`, skipped the revocation call for want of a token that could not
  arrive, deleted the cookies, and told the user they had been signed out while the refresh family
  stayed valid for its full 7 days. Sign-out is now a route handler nested *inside* the refresh
  path — the only place on this origin that receives the cookie — and `clearSession()` was deleted
  rather than left as a helper that does the easy half. Verified in the database: the family is
  `revoked_reason = 'logout'` after clicking the button, and was demonstrably not before.
  **Reads and writes are split by module, not by convention.** Everything in a `'use server'` file
  becomes a callable POST endpoint, so `lib/data/*` holds the reads and `lib/actions/*` the
  mutations. `requestAppointment` no longer sends a `donor_id` at all — the API takes it from the
  `sub` claim, so a donor cannot raise a request in someone else's name.
  **Verified end-to-end against the running stack**, three roles and six areas: anonymous → login
  for every guarded prefix; a donor refused `/admin` and `/staff` and redirected to their own
  record; **`staff` refused `/admin`** and `/lab`; a donor opening another donor's page bounced to
  their own. Tampering: one flipped signature byte, a payload forged to `role: admin`, and an
  `alg: none` token each ended at the login page. Refresh: an expired token routed to the refresh
  handler, rotated to a genuinely different token, and returned to the page it came from — with
  `_r=1` breaking the loop when it still fails, and `next=//evil.example` rewritten to `/`.
  A live donor page rendered real profile data, and "Request appointment" wrote a `pending` row
  owned by the caller's own id. Frontend `tsc`, `eslint` and `next build` clean; backend `go build`,
  `go vet` and `archcheck.sh` unchanged and passing. Fixtures removed afterwards.
  **Not done here, deliberately:** signup and donor create/edit have had no endpoint since `WI-11`;
  the forms now say so instead of posting into a 404 (`WI-22`). `/staff`, `/lab`, `/inventory` and
  `/hospital` are placeholders naming the work item that fills them, so a real account lands
  somewhere honest instead of a 404.
- **2026-09-01** — **`WI-20` complete: RBAC middleware and ownership from the token.**
  The TRD §7.6 permission matrix now lives in `internal/domain/rbac.go` as **data** — 22 resources ×
  6 roles × 5 actions — rather than as `if role == "admin"` scattered through handlers. `domain`
  imports nothing, so the entire matrix is unit-testable with no database, server or token.
  **`X` is checked twice.** The matrix writes a donor's cell as `X-cancel` and staff's as
  `X-approve/reject`: both hold X. A single Execute check would therefore let a donor **approve
  their own donation request**, deleting the review step the application exists to perform. So
  `CanExecute(role, resource, transition)` gates the named transition, and a transition nobody has
  declared is denied rather than assumed harmless.
  **Ownership (§7.7) is now derived from the verified token.** `middleware.Permits` evaluates the
  granted scope — `own` / `ctr` / `hosp` / `agg` — against the row's owning columns, and a
  violation answers **404, not 403**, because a 403 confirms the record exists.
  **This finishes what `WI-02` could only patch.** That hotfix made the appointment ownership check
  unconditional but still compared against `?donor_id=`, a value the caller supplied — a bypass fix,
  not authorization. The comparison is now against the `sub` claim, so asserting somebody else's
  identity is not something a request can express. `getAppointment` no longer reads `donor_id` at
  all; on list endpoints it survives only as a filter for callers already scoped wider than one
  donor, and it can narrow a result set but never widen it.
  **Migration `000015` gives the `cid` claim a source column.** `users` had `hospital_id` but no
  `center_id`, so `cid` had been signed as `null` on every token since `WI-17` while the matrix
  scoped **every** staff grant to `ctr`. The middleware fails closed on a null centre, which meant a
  staff account would have been able to see nothing at all. The column is asymmetric with
  `hospital_id` on purpose: mandatory for `staff`, forbidden for donors/admins/hospital users,
  optional for `lab_tech` and `inventory_manager`.
  Two further defects fixed while in there: a donor's own-scope donor listing returned **500**
  instead of an empty list for a user with no profile row, and `createRequest` took `donor_id`
  **from the request body** and looked the name up in the pre-migration `donors` table rather than
  in `donor_profiles`, the table the foreign key actually references.
  **Tests:** the matrix is asserted over real HTTP through the middleware — **165 granted and 495
  denied cells, all 660** — plus structural invariants (the audit log is read-only for every role
  including admin; a hospital user gets aggregate inventory only; an unknown role or resource
  denies), transition rules, and the §7.7 scope table.
  **Verified end-to-end against the running stack** with five throwaway accounts (two donors, two
  staff at different centres, one admin), since a matrix that passes its unit tests can still be
  mounted on the wrong routes: anonymous → 401 everywhere; a donor reading another donor → 404;
  a donor's list narrowed to itself; staff seeing only their own centre's requests and appointments;
  a donor approving their own request → 403; staff approving another centre's request → 404;
  `?donor_id=16` from donor B → 404; and posting `{"donor_id":17}` as donor A creating a row for
  **16**. Migration `000015` verified `up → down → up`, and both halves of the role/centre
  constraint verified to reject. Fixtures removed afterwards.
  **Consequence for the frontend:** `/api/go/*` now requires a token, and `bbank/` still sets the
  old unsigned `bb_session` cookie. **The donor and admin pages will 401 until `WI-19` lands** —
  that is the migration window the plan sequences, not a regression.
- **2026-09-01** — **`WI-17` complete: ES256 JWT + rotating refresh tokens.**
  Migration `000014` adds `sessions` (refresh-token families, SHA-256 hashed — the token itself is
  never stored) and `users.token_version`. `POST /api/v1/auth/{login,refresh,logout}` plus
  `GET /api/v1/auth/public-key`.
  **ES256, not HS256, for a specific reason:** `proxy.ts` must *verify* tokens, and with HS256 the
  verifying key is also the signing key — a frontend compromise would become an admin-token
  factory. ES256 gives the frontend only the public half, and P-256 works in both the Node and Edge
  runtimes that `jose` uses.
  **Verified end-to-end against the running stack:** wrong password → 401 without revealing which
  factor failed; correct password → `bb_at` (Lax, `/`) and `bb_rt` (Strict, `/api/v1/auth/refresh`)
  both HttpOnly; refresh rotates to a genuinely different token; **replaying the old token revoked
  the entire family** — both the stolen and the legitimate token — and logged
  `security.refresh_reuse` with the user and family id; logout → 204 then 401; the public-key
  endpoint contains no private material.
  **8 crypto unit tests**, including tampered-token rejection, foreign-key rejection, issuer and
  audience checks, and the **alg-confusion bypass** (`alg:none` must never verify).
  Login spends the bcrypt cost even when the account does not exist, so timing cannot be used to
  enumerate users.
  Two defects caught during the work: `000014` initially attached the `set_updated_at` trigger to a
  table with no `updated_at` column (caught by testing an UPDATE before committing), and the config
  now refuses to start without `JWT_PRIVATE_KEY` unless `ALLOW_EPHEMERAL_JWT_KEY=true` — an
  ephemeral key silently invalidates every session on restart.
  **Not yet wired:** `VerifyAccessToken` (with the `token_version` check) is implemented and tested
  but nothing calls it per-request — that is `WI-20`'s RBAC middleware.
- **2026-09-01** — **`WI-11` + `WI-12` complete: the backend is layered.**
  `main.go` (786 lines) is gone. New structure: `cmd/api`, `cmd/migrate`,
  `internal/{domain,service,store,http/{handlers,dto,response},middleware,platform,legacy}`.
  Adopted **chi**, **pgx/v5** + **sqlc** (retiring `lib/pq` for new code), and moved `donors` to
  the layered path as the strangler pilot — with pagination and search, closing the unbounded
  list scan (`TD-17`). Everything else still serves from `internal/legacy`, unchanged, so nothing
  broke mid-refactor. `CLAUDE.md` rewritten (`WI-12`): the single-file convention is formally
  replaced, with the rationale recorded.
  **First domain tests**: ABO compatibility (directionality, universal donor/recipient, no
  Rh-positive unit to an Rh-negative recipient) and legacy free-text blood-group parsing. Plus a
  test that parses `000012_seed_reference_data.up.sql` and asserts the Go matrix and the SQL seed
  agree on all 27 pairs — divergence there is a patient-safety failure, not a build failure.
  **Dependency rule enforced** by `backend/archcheck.sh` in CI, verified non-vacuous by adding a
  `domain → store` import and watching it fail.
  **Two defects found and fixed:**
  (1) `donations_sync_counter` fired `AFTER INSERT` only, so `total_donations` incremented but
  never decremented — observed drifting to 1 against 0 real donations. Migration `000013` handles
  INSERT/UPDATE/DELETE and reconciles existing drift. (`drop_legacy_donors` moves to `000014`.)
  (2) **`WI-14` had silently broken the legacy API** — it renamed `requests` and dropped
  `appointments.donor_name`/`appointment_date`, and the handlers still queried the old shape;
  both endpoints were returning 500. Fixed the shim, and **replaced its hard `DELETE` with the
  `status='approved'` transition** rather than let it keep destroying the audit chain until
  `WI-22`. Verified end-to-end: approving a request now leaves the row present, reviewed, and
  linked from the appointment.
- **2026-09-01** — **`WI-16` complete; the database schema is finished** (migrations `000010`–`000012`).
  7 trigger functions + 11 triggers, 4 views, 38 indexes, and the reference seed.
  **The clinical constants are now data, not code**: 14 `policies` rows carry the 56-day interval,
  18–65 age band, 50 kg minimum, Hb 12.5/13.0 by sex, and component shelf lives (PRBC 42d, platelets
  5d, FFP/cryo 12mo). The `abo_compatibility` matrix seeds to exactly 27 rows, matching spec
  (O−:1 … AB+:8). 5 mandatory TTI types.
  **Verified functionally on real data, not asserted:**
  `guard_unit_release` refuses an untested unit (*"5 mandatory TTI test(s) missing"*), still refuses
  with one reactive result, and releases only on a full non-reactive panel — consistent with `OD-18`
  (no override, ever). `unit_status_events` auto-appends every transition and **rejects both UPDATE
  and DELETE**; a `blood_units` status change cannot happen without a ledger row. `donor_eligibility`
  computes `next_eligible_on` from a real `donations` row (+56 days exactly), flags the legacy
  future-dated donor as `under_age`, and correctly ignores `legacy_last_donation`.
  Full `up → down -all → up` cycle passes on a fresh PostgreSQL 18 database.
  **Two design decisions recorded in the migrations:** `CREATE INDEX CONCURRENTLY` is deliberately
  *not* used in `000011` — every indexed table is empty at that point, so it would buy nothing and
  cost transactional safety; and `000012.down` now refuses with a named error when operational data
  references the seed, rather than surfacing an opaque FK violation.
- **2026-09-01** — **`WI-15` complete** (migrations `000006`–`000009`, all purely additive).
  `000006` screening/deferrals/**donations** — the row that was entirely missing, and the reason
  `donors.last_donation` was donor-entered free text and the 56-day interval could not be enforced.
  `000007` `blood_units` + the append-only `unit_status_events` ledger + `test_results`.
  `000008` the demand side that did not exist at all — `blood_requests`, `issuances`,
  `unit_allocations` — plus the `unit_status_events.blood_request_id` FK that `000007` deliberately
  deferred. `000009` month-partitioned `audit_log` + `notifications`.
  **All 21 target tables now exist**: 26 base tables counting the two audit partitions,
  `migration_rejects` and legacy `donors`; 21 enums, 43 FKs, 265 CHECK constraints.
  Verified `up → down → up` (26 → 13 → 26 tables, version 9, not dirty) with the `WI-14` data
  surviving intact. Audit-log partition routing tested functionally: a September timestamp lands in
  `audit_log_2026m09` and an out-of-range 2027 timestamp falls through to `audit_log_default`
  rather than failing the insert.
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
