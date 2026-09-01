# BBank — Product Requirement Document

| | |
|---|---|
| **Product** | BBank — Blood Bank Management System |
| **Status** | **Draft v1** |
| **Date** | 2026-09-01 |
| **Owner** | Product — BBank core team (Charles Tima) |
| **Audience** | Blood bank leadership, clinical staff, partner hospitals, engineering |
| **Supersedes** | Nothing. This is the first product definition for BBank. |

### Related documents

| Document | What it owns |
|---|---|
| [`./TRD.md`](./TRD.md) | Architecture, API surface, auth model, scaling, security controls, CI/CD |
| [`./USER_JOURNEY.md`](./USER_JOURNEY.md) | End-to-end flows per persona, screen-by-screen steps, edge cases |
| [`./UIUX_BRIEF.md`](./UIUX_BRIEF.md) | Design principles, token system, page inventory, component specs |
| [`./DATABASE_SCHEMA.md`](./DATABASE_SCHEMA.md) | Full DDL, enums, indexes, migration path, seed data |
| [`./IMPLEMENTATION_PLAN.md`](./IMPLEMENTATION_PLAN.md) | Phased, dependency-ordered delivery plan with estimates |

> **ID ownership.** This document owns requirement identifiers. Functional requirements are
> `FR-01` … `FR-83`. Non-functional requirements are `NFR-01` … `NFR-26`. Sibling documents
> cite these IDs; they never renumber or redefine them. Table and column names are owned by
> [`DATABASE_SCHEMA.md`](./DATABASE_SCHEMA.md) and are not restated here.

---

## 1. Executive summary

BBank today is a donor sign-up portal with an appointment calendar attached. It tracks
**people and calendar slots, and never tracks blood.** Nothing in the system records that a
donation actually happened, what volume was collected, whether the collection was screened for
transfusion-transmissible infections, which fridge the bag is sitting in, when it expires, or
which patient received it. There are three tables — donors, requests, appointments — and the
chain of custody that a blood bank exists to maintain simply is not modelled.

There is also no demand side at all. The public landing page tells hospitals with critical
shortages to call a 24/7 line for "priority matching"; no matching, no hospital account, no
request queue and no stock view exist anywhere in the code. The word *request* is currently used
backwards: in BBank it means a donor asking for an appointment, whereas in every real blood bank
a request is a clinician asking for units. That ambiguity is not cosmetic — it is the reason the
demand side was never built.

**BBank becomes a vein-to-vein traceability and inventory system for a blood bank.** From the
moment a donor registers, through screening, collection, laboratory testing, component
separation, cold-chain storage, compatibility-matched allocation and issuance, to the recorded
outcome of the transfusion — every step produces an immutable event against a uniquely
identified blood unit. A director can answer "how many units of O− do we have that are still
good on Friday?" and, if a transfusion reaction occurs, can walk backwards from the recipient to
the donor in minutes rather than through a paper ledger.

The existing donor portal is not thrown away; it is the front door and it stays. What is added
is the operational spine behind it, six real roles instead of one hardcoded admin credential,
a hospital-facing demand channel, and a public site that only promises what the system can
actually do. This is honest-scope work: the current codebase is portfolio-grade, and the plan
sequences the boring correct thing first — a real data model, real authorisation, real audit —
before anything resembling a high-availability deployment.

---

## 2. Problem statement

Blood is a perishable, unmanufacturable, individually-traceable product. A blood bank is
therefore not a registry of willing people; it is a cold-chain inventory operation with a
regulatory obligation to prove where every millilitre came from and went. Software that models
only donors and appointments leaves the actual work on paper.

### 2.1 The operational pain

| Pain | What it looks like in practice | What it costs |
|---|---|---|
| **Wastage from expiry** | Platelets live 5 days. Nobody knows on day 4 which bags are about to time out, so they time out. | Discarded units are donations thrown away, plus the recruitment cost of replacing them. Typical unmanaged wastage sits in double digits by percentage. |
| **Stockouts during emergencies** | A trauma case needs 4 units of O− at 02:00. The on-call surgeon phones around three facilities because no one can see stock without walking to a fridge. | Delayed transfusion. This is the failure mode with a body count. |
| **Paper-based traceability** | A transfusion reaction is reported. Reconstructing donor → donation → unit → recipient means pulling ledgers and hoping the handwriting is legible. | Regulatory exposure, unbounded investigation time, inability to recall sibling components from the same donation. |
| **Donors who don't return** | A donor is eligible again after 56 days. Nobody tells them. They meant to come back and didn't. | Repeat donors are far cheaper and safer than new ones; losing them means permanent recruitment spend. |
| **Untracked screening and testing** | Screening happens verbally; test results live in a lab notebook. Nothing systemically prevents an untested unit being handed over. | The single worst outcome this system could enable. |
| **Double-issuing one bag** | Two staff members allocate the same physical unit to two patients because the "system" is a whiteboard. | A unit that exists on paper but not in the fridge, discovered at the worst moment. |

### 2.2 Why now

The current system has already earned donor trust with a working sign-up and booking flow. That
is the hard part of adoption. Adding the operational spine now — while the user base is small
and the data is cheap to migrate — is far less expensive than retro-fitting traceability onto a
system with two years of untracked collections behind it.

---

## 3. Goals & non-goals

### 3.1 Goals

| # | Goal | Why it matters |
|---|---|---|
| G1 | Model the full vein-to-vein chain as first-class, append-only events | Traceability is the product; everything else is a view over it |
| G2 | Make blood inventory visible and accurate in real time, by group, component and expiry | Removes the phone-around and the whiteboard |
| G3 | Build the demand side: hospitals raise requests, staff fulfil them under compatibility rules | Closes the loop the landing page already promises |
| G4 | Make it structurally impossible to release an untested unit or issue one without a crossmatch | Patient safety expressed as a system constraint, not a policy poster |
| G5 | Cut wastage by surfacing expiry early and issuing first-expiry-first-out | Directly measurable |
| G6 | Retain donors through eligibility-aware, automated re-engagement | The cheapest supply is a returning donor |
| G7 | Replace the single hardcoded admin with six real, least-privilege roles and a complete audit trail | Prerequisite for handling health data at all |
| G8 | Make the public site tell the truth about capability, and convert visitors into booked donors | Marketing claims currently exceed the code |

### 3.2 Non-goals

These are explicitly **out of scope**, in v1 and in most cases beyond it. Requests to add them
should be treated as new product decisions, not backlog items.

| Not building | Reasoning |
|---|---|
| **A hospital EMR / patient record system** | BBank stores a *patient reference* on a blood request, not a clinical record. Diagnoses, medications and notes belong to the hospital's system. |
| **A replacement Laboratory Information System** | BBank records TTI results and their consequences. It does not drive analysers, manage reagent lots, or run QC statistics. If a LIS exists, BBank integrates with it later (see §13). |
| **National / regional blood registry integration** | v1 is single-organisation. Cross-organisation stock sharing and national reporting formats are deferred until a real integration partner and specification exist. |
| **Donor payment, incentives or rewards** | Paid donation carries known safety and ethical problems and would change the regulatory posture of the whole product. BBank supports non-remunerated donation only. |
| **Mobile native applications** | The web app is responsive and installable. Native apps are not justified before the operational core is proven. |
| **Apheresis / plateletpheresis workflow** | Modelled as a component type and an interval policy so the data model does not exclude it, but the procedure workflow is deferred pending §13 confirmation. |
| **Blood typing decisions made by software** | BBank applies ABO/Rh compatibility rules to *suggest* and to *block*. A qualified human confirms the group and signs the crossmatch. |
| **Billing, invoicing and cost recovery** | Out of scope entirely for v1–v2. |

---

## 4. Target users & personas

Six roles exist in BBank: `donor`, `staff`, `lab_tech`, `inventory_manager`, `hospital_user`,
`admin`. One persona per role.

---

### 4.1 `donor` — Awa Njoya, 29, secondary-school teacher, Bonabéri

**Context.** Donated twice, both times after a colleague's road accident appeal on WhatsApp.
She is O+, healthy, and willing — but she has no idea when she is next allowed to give, and her
only interaction with the blood bank is a signup form she filled in 14 months ago. She books on
her phone, usually on 3G, during a break between lessons.

| | |
|---|---|
| **Goals** | Give blood when it is actually useful; know she is eligible before travelling; not waste an afternoon |
| **Frustrations** | Turned away once for low haemoglobin with no explanation and no follow-up; never told when she became eligible again; no record of her own donation history |
| **Key tasks** | Register · complete profile · check eligibility · book a slot · reschedule · view history · read the FAQ before a first donation |
| **Success looks like** | A push/SMS on day 56 saying "you're eligible again, here are Saturday slots", two taps to book, a confirmation page that tells her what to eat beforehand |

---

### 4.2 `staff` — Clarisse Fotso, 34, phlebotomist & front desk, Douala Central donation centre

**Context.** Runs the busiest morning window in the centre: 20 walk-ins and booked donors
between 08:00 and 11:00, alone at the desk for the first hour. She checks people in, takes
vitals, runs the questionnaire, and draws. Every screen she has to wait on costs her a donor's
patience.

| | |
|---|---|
| **Goals** | Move the queue without cutting a screening corner; find a returning donor by name or phone in one search; record a collection in under a minute |
| **Frustrations** | Duplicate donor records because someone typed the name differently; deferrals recorded on paper so a deferred donor books again next week; no way to tell whether the person in front of her is the person in the record |
| **Key tasks** | Check in · search donors · record vitals & Hb · complete questionnaire · pass or defer · record the collection, volume, bag lot and phlebotomist · log adverse reactions · print unit labels |
| **Success looks like** | Search by partial name, national ID or phone returns the right donor in under a second; the system refuses a booking from a donor still inside a deferral window before Clarisse ever sees them |

---

### 4.3 `lab_tech` — Blaise Ekani, 41, laboratory technician

**Context.** Receives the day's collections as quarantined units. Runs the mandatory TTI panel —
HIV 1/2, HBsAg, HCV, syphilis, malaria — plus ABO/Rh confirmation. His entries are the gate
between a bag of blood and a patient's arm, and he is acutely aware that a mistyped result is
the worst thing he can do.

| | |
|---|---|
| **Goals** | Enter a panel quickly and correctly; never have a unit escape quarantine untested; handle reactive results with the confidentiality they demand |
| **Frustrations** | Results in a notebook that inventory staff cannot see, so units are chased by phone; no structured way to mark a result indeterminate and require a repeat; no automatic donor deferral when a result comes back reactive |
| **Key tasks** | Enter TTI results per donation · confirm ABO/Rh · release units to `available` · quarantine or discard reactive units · trigger confidential donor notification and permanent deferral · order repeat testing |
| **Success looks like** | A worklist of donations awaiting testing; releasing a unit is impossible until every panel line is `non_reactive`; a reactive result discards the unit, permanently defers the donor, and writes an audit entry without him doing five separate things |

---

### 4.4 `inventory_manager` — Sandrine Etoundi, 38, stock controller

**Context.** Owns the fridges, the freezers and the platelet agitator. Splits whole blood into
packed cells, plasma and platelets, assigns storage, and is the person who has to explain to the
director why 40 units expired last month.

| | |
|---|---|
| **Goals** | Know today what expires this week; keep every component in the right temperature class; discard with a reason that survives an inspection |
| **Frustrations** | Expiry dates calculated by hand per component; no view of what is expiring in 72 hours; discards recorded as a line in a notebook with no authorisation trail; stock counts that disagree with the fridge |
| **Key tasks** | Split donations into components · assign storage locations · run and act on the expiring-soon list · record discards with reason · reconcile physical counts · watch low-stock thresholds by group |
| **Success looks like** | A dashboard whose first row is "expiring within 72h", grouped by component and blood group, that she can act on before it becomes wastage; a discard flow that captures reason, actor and timestamp automatically |

---

### 4.5 `hospital_user` — Dr Fabrice Nkeng, 45, on-call surgeon, Laquintinie Hospital

**Context.** It is 02:10. A road-traffic polytrauma is in theatre and he needs **4 units of O−**,
now. Today that means calling three numbers and hoping. He has no login, no stock view, and no
way to leave a request that will still be there when the day staff arrive.

| | |
|---|---|
| **Goals** | See whether the units exist before he commits to a plan; raise an emergency request that is impossible to miss; know the moment it is fulfilled and who is bringing it |
| **Frustrations** | The public site promises "priority matching" that does not exist; no acknowledgement that a request was received; no partial-fulfilment concept, so "we have 2 of the 4" turns into another phone call |
| **Key tasks** | Raise a blood request with patient reference, group, component, units and urgency · view live availability · track status · confirm receipt · record the transfusion outcome or return unused units |
| **Success looks like** | An emergency request that is never rate-limited into failure, is acknowledged within seconds, alerts on-duty staff by SMS, and shows him a fulfilment status he can refresh from theatre |

---

### 4.6 `admin` — Dr Marthe Ondoa, blood bank director

**Context.** Accountable to the health authority for everything the bank does. Signs off
discards, answers inspection questions, decides where the mobile drive goes next month, and
carries the risk if a transfused unit turns out to have been untested.

| | |
|---|---|
| **Goals** | Prove traceability on demand; see wastage and fill rate without building a spreadsheet; grant the minimum access each person needs |
| **Frustrations** | One shared admin credential; no audit of who changed what; monthly reporting assembled by hand from three sources; no evidence trail when something goes wrong |
| **Key tasks** | Manage users & roles · configure eligibility and shelf-life policy · review audit log · run wastage, fill-rate and retention reports · approve recalls · export regulatory returns |
| **Success looks like** | An inspector asks about a specific unit code and she reconstructs donor, screening, test results, storage, allocation and recipient reference on one screen in under two minutes |

---

## 5. Jobs to be done

### Donor

- When I want to give blood but don't know if I'm allowed yet, I want to see my next eligible date and why, so I can plan instead of guessing.
- When I become eligible again, I want to be told, so I don't drift away without meaning to.
- When I'm deferred at the desk, I want to know the reason and when it lifts, so I don't feel rejected and give up.
- When I've booked, I want a confirmation that tells me what to bring and eat, so I'm not turned away for something avoidable.
- When I've donated, I want to see my history and impact, so it feels like something I did rather than something that happened to me.

### Staff

- When a donor walks up to the desk, I want to find their record from a partial name or phone number, so the queue keeps moving.
- When I finish screening, I want the eligibility decision computed for me against current policy, so I'm not doing arithmetic on thresholds under pressure.
- When I defer someone, I want the system to enforce it at booking, so a deferred donor never appears in tomorrow's list.
- When I complete a collection, I want units created and labelled automatically in quarantine, so I never hand an unlabelled bag to the lab.

### Lab technician

- When a batch of collections arrives, I want a worklist of donations awaiting testing, so nothing sits untested by accident.
- When I record a full non-reactive panel, I want the units released in one action, so release is a decision and not a chore.
- When a result is reactive, I want the unit discarded, the donor deferred and a confidential notification raised as one atomic step, so no part of that is forgotten.

### Inventory manager

- When I start the day, I want the units expiring within 72 hours in front of me, so I can move them before they're waste.
- When I split a donation, I want each component's expiry calculated from its own shelf life, so I never guess a date.
- When I discard a unit, I want reason, authorisation and timestamp captured automatically, so an inspection is a query rather than a search.

### Hospital user

- When I need units at 2am, I want to raise an emergency request that reaches a human immediately, so I'm not phoning around.
- When I'm planning a scheduled surgery, I want to see availability by group and component, so I can book theatre with confidence.
- When only part of my request can be met, I want to know that explicitly and immediately, so I can source the rest elsewhere.
- When units arrive, I want to confirm receipt and later record the outcome, so the bank's records and mine agree.

### Admin

- When an inspector asks about a unit, I want its complete history on one screen, so I answer in minutes.
- When I review the month, I want wastage, fill rate and time-to-fulfilment computed, so I'm managing on evidence.
- When a member of staff leaves, I want to revoke their access in one action and still keep their historical audit trail, so accountability survives turnover.

---

## 6. Scope

Phases map to [`IMPLEMENTATION_PLAN.md`](./IMPLEMENTATION_PLAN.md).

| Capability | v1 (Phases 1–2) | v2 (Phase 3) | Later (Phase 4+) |
|---|:---:|:---:|:---:|
| Real roles & authorisation, signed sessions | ✅ | | |
| Donor registration, profile, history | ✅ | | |
| Donation requests → appointments (non-destructive status transitions) | ✅ | | |
| Donation centers & slot capacity | ✅ | | |
| Screening, vitals, questionnaire, deferrals | ✅ | | |
| Collection / donation event | ✅ | | |
| Blood units, unit codes, append-only status events | ✅ | | |
| TTI testing & release gate | ✅ | | |
| Component processing & per-component expiry | ✅ | | |
| Inventory summary, expiry sweep, discards | ✅ | | |
| Hospitals, hospital users, blood requests | ✅ | | |
| Compatibility-aware allocation, crossmatch, FEFO issuance | ✅ | | |
| Audit log | ✅ | | |
| Public site completeness (FAQ, thank-you, CTA, SEO, breadcrumbs, OG, USP) | ✅ | | |
| Email/SMS notifications & reminders | ✅ | | |
| Urgent-need donor broadcast | | ✅ | |
| Reporting & analytics suite, regulatory export | | ✅ | |
| Object storage for consent forms, lab PDFs, delivery notes | | ✅ | |
| Redis cache & rate limiting | | ✅ | |
| Donor-facing mobile-installable experience (PWA) | | ✅ | |
| Recall / lookback workflow | | ✅ | |
| Inter-center stock transfer | | | ✅ |
| Read replicas, load balancing, circuit breakers | | | ✅ |
| Full OpenTelemetry / Grafana observability | | | ✅ |
| Dedicated search index (beyond Postgres trigram) | | | ✅ |
| Localisation (French/English) rollout | | | ✅ |
| LIS / national registry integration | | | ✅ |

---

## 7. Functional requirements

Priorities: **P0** = v1 cannot ship without it · **P1** = v1 is materially incomplete without it ·
**P2** = valuable, schedulable later.

### 7.1 Donor management — FR-01 … FR-07

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-01** | **Donor self-registration.** A public visitor creates a donor account with email, password and core identity fields, becoming a `donor` role user with an attached donor profile. | P0 | • A duplicate email is rejected with a field-level message, never a server error<br>• The password is stored only as a hash and appears in no response body<br>• A successful signup lands on the thank-you page (FR-76), not back on a form |
| **FR-02** | **Donor profile completion.** A donor can supply and later edit date of birth, gender, blood group and rhesus, contact number, address, national ID and an emergency contact. | P0 | • Profile completeness is shown as a percentage with the missing fields named<br>• Blood group and rhesus are selected from fixed lists, never free text<br>• Date of birth outside 16–100 years is rejected at entry |
| **FR-03** | **Authentication & session.** All roles authenticate with email and password against a signed, tamper-evident session. | P0 | • A modified session cookie is rejected and the user is signed out<br>• Sessions expire after inactivity and on explicit logout<br>• Failed logins are rate limited and logged without revealing which factor was wrong |
| **FR-04** | **Donor history & eligibility status.** A donor sees their donations, appointments, deferrals and a computed next-eligible date with the reason. | P0 | • Next-eligible date reflects the later of the interval rule and any active deferral<br>• A deferral shows type, reason category and end date<br>• Historic donations show date, center and component(s) produced — never another donor's data |
| **FR-05** | **Donor lookup.** Staff search donors by partial name, phone, email, national ID or donor code. | P0 | • Partial and misspelt names still return the record (fuzzy match)<br>• Results show blood group, last donation and eligibility state at a glance<br>• Results are limited to the searcher's permitted centers |
| **FR-06** | **Duplicate prevention & merge.** The system flags likely duplicate donor records and lets an admin merge them. | P1 | • Registration warns on a matching national ID or phone before creating a second record<br>• A merge preserves both records' donations, screenings and deferrals under the surviving donor<br>• The merge is recorded in the audit log with both original identifiers |
| **FR-07** | **Consent & data rights.** Donors give explicit consent at registration and can request an export or erasure of their personal data. | P1 | • Consent version and timestamp are stored with the profile<br>• Export returns the donor's own data in a portable format<br>• Erasure anonymises the donor record while preserving the clinical chain against the unit (see FR-69) |

### 7.2 Appointment & scheduling — FR-08 … FR-14

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-08** | **Donation request submission.** A donor requests a donation by choosing a center and a preferred date, creating a `pending` donation request. | P0 | • A donor inside an active deferral or interval window is blocked with the date they become eligible<br>• A donor cannot hold more than one open request at a time<br>• The donor is returned to the thank-you page stating what happens next |
| **FR-09** | **Donation request review.** Staff approve or reject a pending donation request with a reason; approval creates a scheduled appointment. | P0 | • Approval and rejection are **status transitions, never row deletion**<br>• Rejection requires a reason from a controlled list plus optional note<br>• Approval and appointment creation succeed or fail together |
| **FR-10** | **Appointment scheduling & capacity.** Appointments are scheduled against a center's operating hours and per-slot capacity. | P0 | • A slot at capacity cannot be over-booked, including under concurrent approvals<br>• Slots outside the center's hours are not offered<br>• The donor sees the confirmed date, time and center address |
| **FR-11** | **Cancellation & rescheduling.** Donors and staff can cancel or reschedule an appointment before check-in. | P0 | • Cancellation frees the slot immediately<br>• The donor is notified of any staff-initiated change<br>• Cancelled appointments remain visible in history with their reason |
| **FR-12** | **Check-in.** Staff move an appointment to `checked_in`, recording the time and verifying donor identity. | P0 | • Check-in is only possible on the day of the appointment (with an override that is audited)<br>• Check-in blocks if the donor has an active deferral, showing why<br>• `checked_in_at` is stored and drives the screening worklist |
| **FR-13** | **No-show handling.** Appointments not checked in by end of day are marked `no_show`. | P1 | • The sweep runs daily and is idempotent<br>• Repeated no-shows are visible on the donor record<br>• A no-show does not create a deferral |
| **FR-14** | **Donation center management.** Admins create and edit donation centers with address, region, contact, operating hours and per-slot capacity. | P0 | • Deactivating a center stops new bookings but preserves history<br>• Capacity changes never invalidate already-confirmed appointments<br>• Centers appear on the public site with accurate hours |

### 7.3 Screening & eligibility — FR-15 … FR-20

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-15** | **Pre-donation questionnaire.** Staff complete a structured health and risk questionnaire per attendance, stored with the screening. | P0 | • Required questions cannot be skipped to reach an outcome<br>• The questionnaire version used is recorded with the answers<br>• Answers are visible only to `staff`, `lab_tech` and `admin` |
| **FR-16** | **Vitals & haemoglobin capture.** Staff record haemoglobin, blood pressure, pulse, weight and temperature. | P0 | • Values outside physiologically plausible ranges are rejected at entry<br>• Units of measure are fixed and displayed<br>• Missing haemoglobin blocks a `passed` outcome |
| **FR-17** | **Automated eligibility evaluation.** The system evaluates vitals, age, interval and deferral history against active policy and proposes an outcome. | P0 | • Each failing criterion is named individually, not as a single "ineligible"<br>• Staff can override a computed outcome only with a reason, which is audited<br>• The policy version used is stored on the screening |
| **FR-18** | **Deferral recording.** A `deferred_temporary` or `deferred_permanent` outcome creates a deferral with reason, start and end date. | P0 | • A temporary deferral requires an end date; a permanent one must not have one<br>• The linked appointment moves to `deferred`, not `completed`<br>• The donor is notified with the reason and the date it lifts |
| **FR-19** | **Deferral enforcement.** Active deferrals block booking, check-in and collection. | P0 | • The block is enforced server-side, not only in the UI<br>• A permanent deferral cannot be bypassed by any role except `admin`, with audit<br>• The donor sees a plain-language explanation, not an error code |
| **FR-20** | **Eligibility policy configuration.** Admins edit eligibility thresholds and intervals as versioned, effective-dated policy records. | P0 | • No eligibility threshold is hardcoded in application logic<br>• Editing a policy does not retroactively alter past screening decisions<br>• Every policy change is audited with before/after values |

### 7.4 Collection — FR-21 … FR-25

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-21** | **Record a donation.** Staff record the collection event against a checked-in, screened-and-passed appointment: volume, collection time, bag lot number and phlebotomist. | P0 | • A donation cannot be created without a passing screening on the same appointment<br>• The appointment moves to `completed` in the same transaction<br>• Bag lot number is mandatory and searchable |
| **FR-22** | **Unit code generation & labelling.** Each collection produces one or more uniquely coded blood units with printable, scannable labels. | P0 | • Unit codes are globally unique and never reused<br>• A label carries the code, group, component, collection date and expiry<br>• Codes are resolvable by scan or manual entry |
| **FR-23** | **Adverse reaction recording.** Staff record any donor adverse reaction during or after collection, with severity and action taken. | P1 | • A recorded severe reaction prompts a deferral decision before the record closes<br>• Reactions are visible on the donor record for future screenings<br>• Reaction data is included in safety reporting (FR-64) |
| **FR-24** | **Units enter quarantine.** All units created from a donation start with status `quarantined` and are invisible to allocation. | P0 | • No quarantined unit can be allocated, reserved or issued by any role<br>• Unit creation writes the first entry in the unit's status event history<br>• Quarantined stock is reported separately from available stock |
| **FR-25** | **Donor counters & next eligibility.** A completed donation updates the donor's total donations and recomputes the next eligible date from the applicable interval policy. | P0 | • Totals and next-eligible date update within the same transaction as the donation<br>• Annual donation caps by gender are enforced at the next booking attempt<br>• The donor sees the new date on their dashboard immediately |

### 7.5 Laboratory testing — FR-26 … FR-30

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-26** | **TTI panel result entry.** Lab technicians record each mandatory test — HIV 1/2, HBsAg, HCV, syphilis, malaria — per donation with result, timestamp and tester. | P0 | • The required panel is driven by configurable policy, not code<br>• Each result records who entered it and when<br>• A donation with any `pending` panel line appears on the outstanding worklist |
| **FR-27** | **ABO/Rh confirmation.** The lab confirms the unit's blood group and rhesus independently of the donor's self-declared group. | P0 | • A discrepancy with the declared group raises an alert and blocks release<br>• The confirmed group, not the declared one, is used for allocation<br>• The donor profile is updated only after a resolved discrepancy, with audit |
| **FR-28** | **Release gate.** A unit moves from `quarantined` to `available` only when every mandatory test is `non_reactive` and the group is confirmed. | P0 | • Release is refused, server-side, if any panel line is missing, `pending`, `reactive` or `indeterminate`<br>• Release writes a status event with actor and reason<br>• There is no UI path, admin included, that skips this check |
| **FR-29** | **Reactive result handling.** A `reactive` result discards all units from that donation, applies a donor deferral and raises a confidential donor notification, as one action. | P0 | • All sibling units from the donation are discarded together<br>• The donor deferral type follows policy for the specific marker<br>• The notification never states the result in the message body; it directs the donor to a confidential channel |
| **FR-30** | **Indeterminate results & repeat testing.** An `indeterminate` result holds the unit in quarantine and flags the donation for repeat testing. | P1 | • The unit cannot be released or discarded while a repeat is outstanding<br>• Repeat results are stored as additional records; the original is never overwritten<br>• Donations held beyond a configurable window are escalated to the lab lead |

### 7.6 Component processing — FR-31 … FR-34

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-31** | **Component separation.** An inventory manager splits a whole-blood unit into packed red cells, fresh frozen plasma, platelets and/or cryoprecipitate, creating child units linked to the parent. | P1 | • Each child unit gets its own code and its own status history<br>• The parent unit is closed out and cannot be issued after separation<br>• Every child inherits the donation's traceability chain |
| **FR-32** | **Per-component expiry.** Expiry is computed from the collection time and the component's configured shelf life. | P0 | • Shelf lives are policy rows, not constants in code<br>• Expiry is recomputed if a component type is corrected, with audit<br>• Expiry is shown wherever a unit is shown |
| **FR-33** | **Storage assignment & temperature class.** Every unit is assigned a storage location whose temperature range matches its component's requirement. | P0 | • Assigning a component to an incompatible storage class is refused<br>• Moving a unit writes a status event capturing origin and destination<br>• Each storage location shows current occupancy |
| **FR-34** | **Processing loss.** Volume or units lost during separation are recorded as discards with a processing reason. | P2 | • Processing discards are distinguishable from expiry and reactive discards in reports<br>• The reason is selected from a controlled list<br>• The loss appears in the wastage metric with its own category |

### 7.7 Inventory management — FR-35 … FR-43

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-35** | **Live inventory summary.** A single view shows units on hand by blood group, rhesus, component type and status, including units expiring within 72 hours. | P0 | • Availability figures reflect committed transactions with no stale reads<br>• Quarantined, available, reserved and issued are never conflated in a single "in stock" figure<br>• The view is filterable by center |
| **FR-36** | **Unit lookup.** Any authorised user can retrieve a unit's full history by scanning or typing its code. | P0 | • The history shows donation, donor reference, screening, test results, storage moves, allocation and issuance<br>• Donor identity is masked for roles without donor-data permission<br>• Lookup completes within the NFR-03 target |
| **FR-37** | **Append-only status events.** Every blood unit status change writes an immutable event with from-status, to-status, reason, actor and timestamp. | P0 | • No code path updates a unit's status without writing an event<br>• Events cannot be edited or deleted by any role<br>• A unit's current status is always reconstructible from its event history |
| **FR-38** | **Expiry sweep.** A scheduled job moves units past their expiry to `expired` and removes them from availability. | P0 | • The sweep is idempotent and safe to re-run<br>• An expired unit can never be allocated or issued<br>• Each expiry writes a status event attributed to the system actor |
| **FR-39** | **Expiring-soon alerts.** Units within a configurable window (default 72 hours) of expiry are surfaced to inventory managers and admins. | P0 | • The window is configurable per component type<br>• The alert list is the first thing on the inventory dashboard<br>• A daily digest is sent to inventory managers |
| **FR-40** | **Discard with reason and authorisation.** Units are discarded only with a controlled reason and an identified authorising actor. | P0 | • Discard reasons come from a fixed list (expired, reactive, processing loss, breakage, temperature excursion, returned unusable)<br>• The discard writes a status event and an audit entry<br>• Discarded units cannot be resurrected; a correction is a new, audited event |
| **FR-41** | **Recall & lookback.** An admin can recall units traced from a donor, a donation or a bag lot, including components already issued. | P1 | • Recall marks matching units `recalled` and removes them from availability instantly<br>• Units already issued produce an alert listing the receiving hospital and request<br>• A recall produces an exportable report |
| **FR-42** | **Low-stock thresholds.** Configurable minimum levels per blood group and component raise alerts when breached. | P1 | • Thresholds are set per center and per component<br>• A breach notifies inventory managers and admins<br>• Current level versus threshold is visible on the dashboard |
| **FR-43** | **Inter-center transfer.** Units can be transferred between centers with a status trail on both sides. | P2 | • A unit is unavailable for allocation while in transit<br>• Both origin and destination record the movement<br>• Transfers appear in each center's stock movement report |

### 7.8 Demand & fulfilment — FR-44 … FR-53

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-44** | **Hospital registration & verification.** Admins register partner hospitals with name, licence number, address and contacts, and set their status. | P0 | • Only an `active` hospital can raise blood requests<br>• Licence number is stored and shown on requests<br>• Suspending a hospital blocks new requests without deleting history |
| **FR-45** | **Hospital user accounts.** Clinicians hold `hospital_user` accounts scoped to exactly one hospital. | P0 | • A hospital user sees only their own hospital's requests<br>• Accounts are invited by an admin, not self-registered<br>• Deactivating an account preserves the requests it raised |
| **FR-46** | **Raise a blood request.** A hospital user requests units by blood group, rhesus, component type, quantity, urgency and needed-by date, with a patient reference. | P0 | • The patient reference is an opaque hospital identifier, never patient identity<br>• Urgency is one of routine, urgent, emergency<br>• The request is acknowledged on screen and by notification within seconds |
| **FR-47** | **Emergency lane.** Requests with `emergency` urgency bypass throttling, alert on-duty staff immediately, and are never silently queued. | P0 | • An emergency request is never rejected by rate limiting; excess load queues and alerts instead<br>• On-duty staff receive an out-of-band alert (SMS) within one minute<br>• Emergency requests sort above all others in every staff view |
| **FR-48** | **Request triage.** Staff approve, partially approve or reject a blood request with a reason. | P0 | • Rejection requires a reason and notifies the requester<br>• Status moves through the defined values only; no ad-hoc states<br>• Every transition is timestamped and attributed |
| **FR-49** | **Compatibility-aware allocation.** When allocating units, the system offers only ABO/Rh-compatible, available, non-expired units, ordered first-expiry-first-out. | P0 | • Incompatible units are not offered and cannot be forced through the UI<br>• Suggested order is strictly by earliest expiry<br>• Two staff allocating simultaneously can never reserve the same unit |
| **FR-50** | **Crossmatch record.** No unit is issued without a recorded crossmatch result against the request. | P0 | • Issuance is refused server-side without a compatible crossmatch record<br>• The crossmatch stores result, performer and timestamp<br>• An incompatible crossmatch returns the unit to `available` with an event |
| **FR-51** | **Issuance & delivery note.** Issuing units moves them to `issued`, produces a delivery note and records issuer and receiver. | P0 | • Issued units leave availability immediately<br>• The delivery note lists every unit code, group and component<br>• Receiver identity is captured at handover |
| **FR-52** | **Fulfilment outcome.** Requests record full or partial fulfilment; issued units record a final outcome of transfused, returned or discarded. | P0 | • Partial fulfilment is an explicit state, communicated to the requester with the shortfall<br>• A returned unit is assessed and either restored to available or discarded, with a reason<br>• Outcome is required to close a request |
| **FR-53** | **Availability visibility for hospitals.** Hospital users see current availability by group and component, without donor-identifying data. | P1 | • The view exposes counts only, never unit codes or donor data<br>• Figures carry an explicit "as at" timestamp<br>• Availability is scoped to centers that serve that hospital |

### 7.9 Notifications — FR-54 … FR-58

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-54** | **Appointment notifications.** Donors receive confirmation on booking and a reminder before the appointment. | P0 | • Reminder lead time is configurable (default 24 hours)<br>• Notifications include date, time, center address and preparation advice<br>• Every send is recorded with channel, template and delivery status |
| **FR-55** | **Eligibility-restored nudge.** Donors are contacted when they become eligible again. | P1 | • The nudge fires on the eligibility date, not before<br>• It is suppressed for permanently deferred donors<br>• It links directly to booking, pre-filled with the donor's usual center |
| **FR-56** | **Urgent-need broadcast.** Staff can broadcast an urgent need to compatible, currently eligible donors near a center. | P1 | • Recipients are filtered by compatibility, eligibility and center proximity<br>• A donor can be included in at most one broadcast per configurable cooldown<br>• The broadcast records how many were contacted and how many booked |
| **FR-57** | **Clinical notifications.** Deferral and reactive-result notifications direct the donor to a confidential channel rather than stating clinical findings. | P0 | • No message body contains a test result or diagnosis<br>• Clinical notifications are logged as sent without storing the finding in the payload<br>• Delivery failures escalate to staff for a phone follow-up |
| **FR-58** | **Preferences & opt-out.** Donors choose channels (email, SMS) and can opt out of non-essential messages. | P1 | • Opt-out is honoured for nudges and broadcasts<br>• Transactional messages (appointment changes, deferrals) are not opt-outable<br>• Every message carries an unsubscribe path where legally required |

### 7.10 Reporting & analytics — FR-59 … FR-64

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-59** | **Role-appropriate dashboards.** Each role lands on a dashboard showing the work and figures relevant to it. | P0 | • No dashboard exposes data the role cannot access<br>• Every figure states its period and refresh time<br>• Dashboards load within the NFR-04 target |
| **FR-60** | **Wastage report.** Discards and expiries by reason, component, group and period, expressed as a percentage of units produced. | P1 | • Wastage is broken down by reason category<br>• The report can be filtered by center and date range<br>• Figures reconcile with the unit status event history |
| **FR-61** | **Fulfilment report.** Fill rate, partial-fulfilment rate, rejection reasons and time-to-fulfilment by urgency. | P1 | • Emergency time-to-fulfilment is reported separately<br>• Unfulfilled requests are listed with their reasons<br>• Median and 90th percentile are both shown |
| **FR-62** | **Donor report.** New versus returning donors, return rate, deferral rate by reason, and no-show rate. | P1 | • Return rate is measured over a configurable window<br>• Deferral reasons are grouped into clinically meaningful categories<br>• Individual donors are identifiable only to roles with donor-data permission |
| **FR-63** | **Regulatory export.** Admins export collection, testing, issuance and wastage data for a period in a machine-readable format. | P1 | • Export contents and period are recorded in the audit log<br>• Exports exclude direct donor identifiers unless explicitly requested and justified<br>• Format is confirmed against the applicable authority (see §13) |
| **FR-64** | **Safety & quality report.** Adverse donor reactions, transfusion outcomes, discrepancies and repeat-test rates. | P2 | • Trends are visible over time, not just as period totals<br>• Each entry links to the underlying record<br>• The report is exportable |

### 7.11 Administration & audit — FR-65 … FR-71

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-65** | **Role-based access control.** Every action is authorised against the six roles, enforced on the server. | P0 | • No privileged operation is reachable by URL manipulation<br>• The hardcoded admin credential is removed entirely<br>• Denied actions return an authorisation error and are logged |
| **FR-66** | **User management.** Admins invite users, assign roles, suspend and reactivate accounts, and trigger password resets. | P0 | • A suspended user's session is invalidated immediately<br>• Role changes take effect on the next request, not the next login<br>• Users are never hard-deleted while they own historical records |
| **FR-67** | **Audit log.** Every create, update, state transition and privileged read of clinical or personal data is recorded with actor, action, entity, before/after and request context. | P0 | • The log is append-only and unmodifiable through the application<br>• Admins can filter by actor, entity type, entity and date range<br>• Sensitive values are redacted in the stored payload where they are not needed |
| **FR-68** | **Clinical policy administration.** Admins manage eligibility thresholds, donation intervals, shelf lives, TTI panel composition and expiry windows as versioned, effective-dated records. | P0 | • Changing a policy never rewrites decisions already made under the previous version<br>• Every change is audited with before/after<br>• Policy is scoped by region to support future multi-region operation |
| **FR-69** | **Data retention & anonymisation.** Personal data is retained per policy and anonymised at end of life while preserving clinical traceability. | P1 | • Anonymisation removes direct identifiers but keeps the donation-to-unit chain intact<br>• Retention periods are configurable per data category<br>• Each anonymisation run is audited |
| **FR-70** | **System configuration.** Admins manage centers, storage locations, hospitals, notification templates and system-wide settings from the UI. | P1 | • No operational configuration requires a code deploy<br>• Changes are audited<br>• Invalid configuration (for example a storage range that suits no component) is rejected at entry |
| **FR-71** | **Break-glass access.** In a defined emergency an admin can override a block, with a mandatory reason and heightened audit. | P2 | • Every break-glass use produces an immediate alert to other admins<br>• The reason is mandatory and free text is retained verbatim<br>• Break-glass cannot bypass the release gate (FR-28) or the crossmatch gate (FR-50) |

### 7.12 Public website — FR-72 … FR-83

| ID | Requirement | Pri | Acceptance criteria |
|---|---|:---:|---|
| **FR-72** | **Claims alignment.** Every public claim maps to a shipped capability; unshipped claims are removed or reframed. | P0 | • The "24/7 priority matching" claim is either backed by FR-46/FR-47 or removed<br>• Headline statistics are either live figures or explicitly labelled as illustrative<br>• Legal and contact details on the page are real and reachable |
| **FR-73** | **USP bar.** A horizontal strip beneath the hero states four differentiators grounded in real capability: every unit screened and tested; traceable vein-to-vein; live stock for hospitals; free health check with every donation. | P1 | • Renders as a single row on desktop and a scrollable or stacked row on mobile<br>• Each item is a short, factual claim with no marketing superlatives<br>• Items are readable at AA contrast against the page background |
| **FR-74** | **CTA system.** A single reusable call-to-action component with a documented placement rule: at most one primary CTA per viewport, "Donate blood" primary and "Request blood" secondary. | P1 | • All public CTAs render through the one component<br>• No viewport contains two competing primary CTAs<br>• Every CTA has an accessible name that makes sense out of context |
| **FR-75** | **FAQ page.** A public FAQ at `/faq` with roughly twelve questions across Eligibility, The process, Safety and After donation, in an accordion, marked up with FAQ structured data. | P1 | • Answers are individually linkable and expanded state is keyboard operable<br>• Content is derived from the actual eligibility policy, not invented<br>• Structured data validates against a schema testing tool |
| **FR-76** | **Thank-you page.** A `/thank-you` page shown after signup and after booking, confirming what happens next and carrying a next action. | P1 | • The page states the confirmed date where one exists<br>• It offers add-to-calendar and share actions<br>• It is never a dead end — at least one onward link is always present |
| **FR-77** | **Unique page titles & metadata.** Every route declares its own title and description, under a root title template. | P1 | • No two routes share a title<br>• Every public route has a description under 160 characters<br>• Dashboard routes are excluded from indexing |
| **FR-78** | **Social sharing image.** A dynamic 1200×630 sharing image is generated for the site, with per-route variants where they add value. | P2 | • Sharing previews render correctly on at least one major social platform and one messaging app<br>• The image includes the product name and a legible one-line proposition<br>• Generation does not block page rendering |
| **FR-79** | **Crawler directives & sitemap.** A robots file allows public routes and disallows the dashboard, donor area and API; a sitemap lists public routes. | P1 | • Admin, donor and API paths are disallowed<br>• The sitemap contains only publicly reachable routes<br>• Both are served at their conventional paths |
| **FR-80** | **Custom 404 pages.** A branded public not-found page, plus a dashboard-scoped one that retains the sidebar and navigation. | P1 | • The public 404 offers navigation to the landing page, FAQ and booking<br>• The dashboard 404 keeps the user in the authenticated shell<br>• Both return the correct HTTP status |
| **FR-81** | **Breadcrumbs.** Dashboard routes deeper than one level display breadcrumbs with structured data. | P2 | • Breadcrumbs reflect the actual hierarchy, not the URL string alone<br>• The current page is present but not a link<br>• Breadcrumbs are absent on one-level pages, including the landing page |
| **FR-82** | **Public eligibility self-check.** An anonymous visitor answers a short set of questions and learns whether they are likely eligible, before registering. | P2 | • The result is clearly labelled as indicative, not a clinical decision<br>• No answers are stored against an identifiable person unless the visitor registers<br>• The eligible path leads directly to registration |
| **FR-83** | **Hospital partner enquiry.** A public route for hospitals to request partner access, feeding an admin queue. | P2 | • Submissions create an admin task, not an email into the void<br>• The form captures hospital name, licence number and a verifiable contact<br>• The submitter receives an acknowledgement with expected response time |

---

## 8. Non-functional requirements

### 8.1 Performance — NFR-01 … NFR-04

| ID | Requirement | Target |
|---|---|---|
| **NFR-01** | Public page load on a mid-range Android device over 3G | Largest Contentful Paint ≤ 2.5 s; Interaction to Next Paint ≤ 200 ms; Cumulative Layout Shift ≤ 0.1 |
| **NFR-02** | API response time for standard read and write operations | p95 ≤ 300 ms, p99 ≤ 800 ms at expected v1 load |
| **NFR-03** | Inventory availability query and unit lookup by code | p95 ≤ 500 ms, computed from committed data with no stale cache |
| **NFR-04** | Dashboard and report rendering | Dashboards ≤ 1.5 s; reports over a 12-month range ≤ 5 s, or asynchronous with a progress state |

### 8.2 Availability & continuity — NFR-05 … NFR-07

| ID | Requirement | Target |
|---|---|---|
| **NFR-05** | Service availability | v1: 99.0% during operating hours, single instance, honest about it. v2: 99.5%. Phase 4 with redundancy: 99.9% |
| **NFR-06** | Backup and recovery | Automated daily encrypted database backups with point-in-time recovery; RPO ≤ 15 minutes, RTO ≤ 4 hours; restore tested quarterly |
| **NFR-07** | Degraded operation | If the API is unreachable, staff can still record collections on a documented paper fallback and reconcile afterwards; the reconciliation path is part of the product, not an afterthought |

### 8.3 Security & health-data handling — NFR-08 … NFR-14

| ID | Requirement | Target |
|---|---|---|
| **NFR-08** | Sessions are signed or encrypted and tamper-evident; passwords are hashed with a modern adaptive algorithm | A modified session token is rejected; no plaintext or reversibly-encoded credential exists anywhere |
| **NFR-09** | All traffic uses TLS 1.2 or above; HSTS enabled; no mixed content | External security header scan passes with no high findings |
| **NFR-10** | Health data at rest is encrypted (volume or column level); uploaded documents live in private storage accessed by short-lived signed URLs | No health-data object is publicly retrievable; signed URL lifetime ≤ 15 minutes |
| **NFR-11** | Authorisation is enforced on the server for every request; the client is never the authority | An authenticated user of one role cannot reach another role's data by any URL, parameter or ID manipulation |
| **NFR-12** | Abuse controls: login attempts, registration and donation requests are rate limited; cross-origin access is restricted to known origins | Login limited to 5 attempts per minute per IP; wildcard CORS removed; **emergency blood requests are queued and alerted, never rate-limited into failure** |
| **NFR-13** | Secrets are supplied by environment or a secret manager, never committed; rotation is possible without a code change | No credential appears in the repository history going forward; rotation documented |
| **NFR-14** | Dependencies are scanned for known vulnerabilities on every build; critical findings block release | Automated scan in CI; critical and high findings triaged within 7 days |

### 8.4 Privacy & retention — NFR-15 … NFR-17

| ID | Requirement | Target |
|---|---|---|
| **NFR-15** | Data minimisation: only data with an operational or clinical purpose is collected, and each field's purpose is documented | A field inventory exists mapping every personal data field to a purpose and a retention period |
| **NFR-16** | Retention: clinical traceability records are retained for the period the applicable authority requires (see §13; assume 30 years pending confirmation); marketing and behavioural data far shorter | Retention is enforced by an automated job, not by intention |
| **NFR-17** | Data subject rights: access, correction, export and erasure requests are actionable within one calendar month | Each request type has a documented, tested procedure; erasure preserves clinical traceability via anonymisation (FR-69) |

### 8.5 Auditability & traceability — NFR-18 … NFR-19

| ID | Requirement | Target |
|---|---|---|
| **NFR-18** | The audit log captures actor, action, entity, before/after state, timestamp, IP and user agent for every mutation and every privileged read of personal or clinical data; it is append-only | A synthetic tampering attempt through the application fails; audit coverage is verified by test |
| **NFR-19** | Vein-to-vein reconstruction: from a recipient reference or a unit code, the full chain back to the donor is retrievable in a single query path | A trained user completes a full trace in under 2 minutes; the trace is exportable |

### 8.6 Accessibility — NFR-20 … NFR-21

| ID | Requirement | Target |
|---|---|---|
| **NFR-20** | The product conforms to WCAG 2.2 Level AA | Automated checks pass on every route with zero critical violations; manual audit of the booking, screening and issuance flows passes. **Known open issue:** the rose accent `#e11d48` used as a background with white text measures 4.6:1 and fails AA for small text — a compliant accent-on-fill pairing must be chosen (see [`UIUX_BRIEF.md`](./UIUX_BRIEF.md)) |
| **NFR-21** | Every flow is completable by keyboard alone and announced correctly by a screen reader; focus is always visible; forms have programmatic labels and error associations | Keyboard-only and screen-reader passes on signup, booking, screening, testing and issuance |

### 8.7 Compatibility, localisation & scale — NFR-22 … NFR-26

| ID | Requirement | Target |
|---|---|---|
| **NFR-22** | Browser and device support: current and previous major versions of Chrome, Firefox, Safari and Edge; Android 10+ and iOS 15+; usable from 360 px width to 1920 px | Staff screens are usable on a 1366×768 desk monitor; donor screens are designed mobile-first |
| **NFR-23** | Localisation readiness: no user-facing string is hardcoded in a component; dates, numbers and units are formatted by locale; the UI tolerates 30% text expansion | French and English are the first target locales given the deployment context; v1 ships one locale but is structurally ready for the second |
| **NFR-24** | Scalability triggers are defined rather than assumed. Cache and rate limiting when dashboard load or login abuse justifies it; background queue when notification volume justifies it; read replicas when reporting contends with collection writes; a dedicated search index only beyond 100,000 donors | Each trigger has a named metric and threshold in [`TRD.md`](./TRD.md); no infrastructure is added before its trigger fires. **Inventory availability is never read from a replica** |
| **NFR-25** | Observability: structured JSON logs with correlation IDs from day one; metrics and traces from Phase 3. The audit log is a domain requirement and is separate from and additional to observability | Any request can be reconstructed from logs by correlation ID; alerting exists for failed jobs, failed notifications and expiry-sweep failures |
| **NFR-26** | Maintainability: automated tests cover every clinical safety gate; CI runs build, lint, test and vulnerability scan on every change | The release gate (FR-28), crossmatch gate (FR-50), deferral enforcement (FR-19) and allocation concurrency (FR-49) each have explicit regression tests |

---

## 9. Key product rules

These are clinical rules expressed as product behaviour. **Every one of them must be stored as
configurable, versioned, region-scoped policy — not as a constant in application code.** A rule
that is hardcoded cannot be corrected when the regulator changes it, and cannot be shown to an
inspector as evidence of what was in force on a given date.

| Rule | Default | Enforced by |
|---|---|---|
| **Minimum donation interval** | 56 days for whole blood; 7 days for apheresis platelets | FR-08, FR-19, FR-25 |
| **Annual donation cap** | 6 per year (male), 4 per year (female) for whole blood; 24 per year apheresis platelets | FR-25 |
| **Age range** | 18–65; first-time donors commonly capped at 60 | FR-17 |
| **Minimum weight** | 50 kg | FR-17 |
| **Haemoglobin threshold** | ≥ 12.5 g/dL female, ≥ 13.0 g/dL male | FR-16, FR-17 |
| **Vitals ranges** | BP within 90/50–180/100; pulse 50–100 bpm; temperature ≤ 37.5 °C | FR-16, FR-17 |
| **Mandatory TTI clearance before release** | HIV 1/2, HBsAg, HCV, syphilis, malaria — **all** must be non-reactive | FR-26, FR-28 |
| **Reactive result consequence** | All units from the donation discarded; donor deferred per marker; confidential notification | FR-29 |
| **Component shelf life from collection** | Whole blood 35 d (1–6 °C) · packed red cells 42 d (1–6 °C) · platelets 5 d (20–24 °C, agitated; 7 d with bacterial testing) · fresh frozen plasma 12 months (≤ −18 °C) · cryoprecipitate 12 months (≤ −18 °C) | FR-32, FR-33 |
| **FEFO issuance** | Units are always offered and issued earliest-expiry-first | FR-49 |
| **No issue without crossmatch** | An issuance is impossible without a recorded compatible crossmatch against the request | FR-50 |
| **ABO/Rh red-cell compatibility** | O− universal donor; AB+ universal recipient. Recipient → acceptable donor groups: O−:{O−} · O+:{O−,O+} · A−:{O−,A−} · A+:{O−,O+,A−,A+} · B−:{O−,B−} · B+:{O−,O+,B−,B+} · AB−:{O−,A−,B−,AB−} · AB+:{all}. Plasma compatibility is inverted — AB is the universal plasma donor | FR-49 |
| **Status changes are events, not overwrites** | Every blood unit status transition appends an immutable event | FR-37 |
| **A unit is allocated exactly once** | Concurrent allocation of the same physical unit is impossible | FR-49 |
| **Non-remunerated donation only** | No payment or material incentive is offered or recorded | §3.2 |

---

## 10. Success metrics

### 10.1 Leading indicators

Move first, and tell you whether the system is being used properly.

| Metric | Definition | Target |
|---|---|---|
| Screening capture rate | Screenings recorded in BBank ÷ completed appointments | ≥ 98% within 3 months of launch |
| Time from collection to release | Median hours from donation to unit status `available` | ≤ 24 h; 95% within 48 h |
| Untested-unit escapes | Units reaching `available` without a complete non-reactive panel | **Exactly 0. Any occurrence is a P0 incident.** |
| Inventory accuracy | Units in the system matching a physical count, per audit | ≥ 99% |
| Booking completion rate | Signups that go on to book within 30 days | ≥ 40% |
| Reminder deliverability | Notifications delivered ÷ notifications sent | ≥ 95% |

### 10.2 Lagging indicators

The outcomes the product exists to change.

| Metric | Definition | Target |
|---|---|---|
| **Wastage rate** | Units expired or discarded (excluding reactive) ÷ units produced | Reduce by at least a third against the pre-BBank baseline within 12 months |
| **Fill rate** | Blood requests fully fulfilled ÷ blood requests approved | ≥ 90% routine; ≥ 95% urgent |
| **Emergency time-to-fulfilment** | Median minutes from emergency request to units issued | ≤ 60 minutes median, ≤ 120 minutes at the 90th percentile |
| **Donor return rate** | Donors giving again within 12 months of a prior donation | ≥ 45% |
| **Screening deferral rate** | Deferrals ÷ screenings, split temporary and permanent | Tracked, not targeted — a sudden shift signals a policy, staffing or data problem |
| **Stockout days** | Days per month with zero available units of any group/component combination | ≤ 1 per month per combination |
| **Traceability response time** | Time to produce a complete vein-to-vein trace on request | ≤ 2 minutes |

### 10.3 Anti-metrics

Deliberately **not** optimised, and watched to make sure they are not being gamed: number of
donors registered (registration without donation is a vanity figure), and appointments booked
(booking without attendance is worse than no booking).

---

## 11. Risks & mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|:---:|:---:|---|
| R1 | **Clinical harm from an untested or mismatched unit reaching a patient** | Low | Catastrophic | Release gate (FR-28) and crossmatch gate (FR-50) enforced server-side with no override path; explicit regression tests (NFR-26); append-only status events (FR-37) so any failure is reconstructable |
| R2 | **Regulatory non-compliance** — the applicable authority's requirements are not yet confirmed (§13) | High | High | Treat regulatory scope as a blocking open question before go-live; build policy as configurable data (FR-68) so requirements can be met without a code change; retention and export designed to be parameterised (NFR-16, FR-63) |
| R3 | **Health-data breach** | Medium | Severe | Signed sessions, server-side authorisation, encryption in transit and at rest, private object storage with short-lived URLs, complete audit log, dependency scanning (NFR-08 … NFR-14, NFR-18) |
| R4 | **Double-issuing one physical unit** under concurrent allocation | Medium | Severe | Row-level locking or optimistic concurrency on allocation, specified in [`TRD.md`](./TRD.md); explicit concurrency test (FR-49, NFR-26) |
| R5 | **Staff adoption fails** — the desk reverts to paper because the software is slower than a pen | High | High | Design the screening and collection screens against Clarisse's 08:00–11:00 window; sub-second donor search (FR-05, NFR-03); collection recordable in under a minute; pilot at one center before rollout; keep a documented paper fallback and reconciliation path (NFR-07) |
| R6 | **Scope is large relative to the team** — this is one developer's project today, and the full vein-to-vein chain is a multi-quarter build | High | High | Strict phasing (§6); Phase 1–2 delivers a complete, safe, narrow chain rather than a broad, unsafe one; defer every Phase 3+ infrastructure rule until its named trigger fires (NFR-24); no work starts on reporting, caching or observability tooling before the domain model is correct |
| R7 | **Data migration from the legacy three tables loses or mangles history** | Medium | Medium | Migration is a named deliverable with a dry run and a reversible plan in [`DATABASE_SCHEMA.md`](./DATABASE_SCHEMA.md); the `requests` → `donation_requests` rename is executed once, deliberately, with the destructive confirm-and-delete behaviour replaced by a status transition |
| R8 | **Hospital partners don't adopt the demand side**, leaving BBank supply-only | Medium | High | Onboard one hospital as a design partner before building FR-44 … FR-53; keep the emergency path phone-compatible so BBank augments rather than replaces the 02:00 call; make availability visibility (FR-53) the adoption hook |
| R9 | **Notification costs and deliverability** — SMS in the deployment region has real per-message cost and variable delivery | Medium | Medium | Email-first with SMS reserved for time-critical clinical and emergency messages; per-message delivery logging (FR-54); cost modelled before the urgent-need broadcast (FR-56) ships |
| R10 | **Public claims outrun capability again** | Medium | Medium | FR-72 makes claims alignment a shipped requirement with acceptance criteria; a claims review is part of the release checklist for any change to the public site |

---

## 12. Decisions taken in this document

Flagged explicitly because they extend the foundation brief rather than restate it.

1. **Non-remunerated donation only** is a product constraint, not merely an unbuilt feature.
2. **Apheresis** is represented in the data model (component type, interval policy) but its
   procedural workflow is out of v1 scope, pending §13.
3. **A paper fallback and reconciliation path (NFR-07)** is part of the product. A blood bank
   cannot stop working because a server is down.
4. **Registration and appointment counts are anti-metrics** (§10.3), not success metrics.
5. **Dark mode is not in scope for v1.** The current design system is light-only and there is no
   evidence of demand; see [`UIUX_BRIEF.md`](./UIUX_BRIEF.md).

---

## 13. Open questions

These require a stakeholder answer. Several block design decisions already in flight.

| # | Question | Blocks | Who must answer |
|---|---|---|---|
| Q1 | **Which regulatory regime applies?** The deployment context appears to be Cameroon (Douala). Confirm the national blood transfusion authority, its record-retention period, its mandatory TTI panel, and its reporting format. | FR-26, FR-63, NFR-16, R2 | Blood bank director / legal |
| Q2 | **Is there an existing Laboratory Information System?** If so, do TTI results flow into BBank by integration, or are they keyed in? Integration changes FR-26 substantially. | FR-26, FR-27 | Laboratory lead |
| Q3 | **Does the bank perform apheresis?** If yes, the procedure workflow moves from deferred into a scoped phase. | §3.2, FR-21 | Clinical lead |
| Q4 | **SMS provider and budget.** Which gateway, at what per-message cost, with what delivery reporting? This determines whether SMS is the default channel or the exception. | FR-54, FR-56, FR-57, R9 | Director / operations |
| Q5 | **How many donation centers and storage locations exist today**, and does the bank operate mobile drives? Mobile collection changes the center model. | FR-14, FR-33 | Operations |
| Q6 | **Which hospitals are realistic launch partners**, and who at each would be the design partner for the demand side? | FR-44 … FR-53, R8 | Director |
| Q7 | **Who signs off a discard, and who signs off a recall?** Confirm the authorising role and whether a countersignature is required. | FR-40, FR-41 | Quality lead |
| Q8 | **What is the current wastage and fill-rate baseline?** Without it the §10.2 targets are unanchored. | §10.2 | Operations |
| Q9 | **What is the deferral notification norm?** Specifically, how are reactive results communicated to donors today, through what confidential channel, and by whom? | FR-29, FR-57 | Clinical lead |
| Q10 | **Which languages must the UI support at launch**, and is French required for staff-facing screens specifically? | NFR-23 | Director |
| Q11 | **Is patient-level data ever to be stored**, or does the patient reference stay opaque? This materially changes the privacy posture. | FR-46, NFR-15 | Legal / director |
| Q12 | **Who owns production operations** — backups, restores, incident response — once this is handling real donations? | NFR-06, NFR-07 | Director |

---

*Draft v1 · 2026-09-01 · Requirement IDs FR-01 … FR-83 and NFR-01 … NFR-26 are stable and citable.*
