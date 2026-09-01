# BBank — UI/UX Brief

> **Status:** Draft v1 · **Date:** 2026-09-01 · **Owner:** Design
> **Siblings:** [PRD](./PRD.md) · [TRD](./TRD.md) · [User Journey](./USER_JOURNEY.md) · [Database Schema](./DATABASE_SCHEMA.md) · [Implementation Plan](./IMPLEMENTATION_PLAN.md) · [Project Status](./PROJECT_STATUS.md)

**Scope of this document.** How BBank looks, sounds, and behaves. Requirement IDs belong to the
PRD; table and column names belong to the schema doc; both are cited here, never redefined.
This is an **evolution of the design system that already exists** in
`bbank/src/app/globals.css` (391 lines) — not a replacement. Everything proposed here extends
the existing tokens and keeps the existing naming convention.

**The design problem in one sentence.** The current interface is a well-made marketing site
with a thin CRUD admin bolted on; the new scope turns it into a clinical system of record, and
clinical systems have different rules than marketing sites.

---

## 1. Design principles

Seven principles, in priority order. When two conflict, the higher-numbered one yields.

| # | Principle | What it means in practice | What it forbids |
|---|---|---|---|
| **1** | **A mistake here can harm a patient** | The unit code, blood group, and component type appear together on every screen that acts on a unit. Any allocation or issuance screen shows the recipient's required group and the unit's actual group side by side, and the UI states the compatibility verdict in words — not just by allowing the click. | Never infer intent. Never auto-select "the obvious unit". Never let a compatible-looking colour do the reasoning for the user. |
| **2** | **Irreversible actions must be deliberate** | Discarding a unit, recording a permanent deferral, issuing a bag, and marking a reactive test result all pass through `ConfirmDialog` with a typed confirmation of the unit code or donor name, a mandatory reason, and a statement of what cannot be undone. | No bare `onClick={destroy}`. No destructive action reachable by keyboard-repeat on a row. No toast-only "undo" for anything that writes a `unit_status_events` row. |
| **3** | **Status must be legible at a glance, and never by colour alone** | Every state is a **triplet**: colour tint + glyph + text label. A `StatusBadge` printed in greyscale, viewed by a deuteranope, or read by a screen reader conveys the same fact. | No colour-only dots. No "the red ones are the problem" legends. No hue as the sole difference between `quarantined` and `discarded`. |
| **4** | **Calm authority, not startup energy** | Quiet borders, one accent, generous whitespace, serif display used sparingly. The system should feel like a hospital records room that someone cared about — not a growth funnel. | No confetti. No streak counters. No countdown timers on the donor side. No "🔥 3 people are booking right now". |
| **5** | **Clarity over cleverness in clinical contexts** | Marketing surfaces (`/`, `/faq`, `/thank-you`) may use the serif display, gradients, blobs, and scroll reveals. Clinical consoles (`/staff`, `/lab`, `/inventory`) use flat cards, no reveals, no decorative motion, and literal labels. The two zones look related but behave differently. | `display-serif` on a screening form. `Reveal` wrapping a worklist. An icon standing in for a word on a lab result. |
| **6** | **Speed of input beats beauty of input** | The screening form is completable start-to-finish on a keyboard with no mouse. Numeric fields use `inputmode="decimal"`, autofocus chains forward on valid entry, units are printed next to the input (`g/dL`, `mmHg`, `kg`), and the whole form fits one screen at 1280×800 with no scroll. Targets on staff surfaces are ≥44 CSS px because staff wear gloves. | Multi-step wizards for a 6-field form. Custom select widgets that swallow type-ahead. Modals that trap the tab order and offer no `Esc`. |
| **7** | **Two audiences, one system** | A nervous 22-year-old on a 5-inch phone over 3G, and a phlebotomist on a 1440p desktop at 02:00. The donor path is mobile-first and warm. The staff path is desktop-first and terse. They share tokens, not tone. | Writing the whole product in one voice. Making the donor portal dense, or the staff console chatty. |

---

## 2. Brand & tone

### 2.1 Visual direction (unchanged, extended)

Warm minimalist. Off-white canvas (`#fafaf9`), white surfaces, one rose accent, hairline
borders tinted with zinc rather than pure black, a fixed film-grain overlay, and abstract
blurred blobs on marketing surfaces only. Type is **Outfit** for everything, **Instrument
Serif italic** for display emphasis, **Geist Mono** for codes and identifiers.

The direction stays. The extension is that the **rose accent stops being the only colour** —
it is now the *brand*, and a separate semantic scale carries *clinical state*. Rose must never
mean "bad" in a console, because rose already means "BBank".

### 2.2 Voice

| | **Donor-facing** | **Staff / clinical-facing** |
|---|---|---|
| Register | Warm, plain, second person. Explains the *why*. | Terse, imperative, third person about records. States the *what*. |
| Sentence length | 12–20 words | 4–10 words |
| Reading level | ~Grade 8 | Domain-literate; abbreviations allowed (`Hb`, `TTI`, `PRBC`) |
| Emotion | Reassurance | None. Precision reads as respect. |
| Numbers | Rounded, human ("about 10 minutes") | Exact, with units ("12.4 g/dL") |

### 2.3 Before / after

**Donor**

| ✗ Before | ✓ After | Why |
|---|---|---|
| "Awesome! You're all set 🎉" | "You're booked. Tuesday 12 March, 9:00 AM, Douala Centre." | Confirms the fact the person actually needs. No forced delight. |
| "Sorry, you are not eligible." | "You can't donate today — your last donation was 31 days ago, and we ask for 56 days between whole-blood donations. You'll be eligible again on 6 April." | Names the rule, gives the date, restores agency. |
| "Donation failed." | "We weren't able to complete your booking. Nothing was charged and nothing was recorded. Try again, or call us on +237 6 53 53 29 29." | Says what did *not* happen. Offers a human. |
| "Join 10,847 heroes!" | "10,847 people have registered to donate through BBank." | Fact, not flattery. See §2.4. |

**Staff**

| ✗ Before | ✓ After | Why |
|---|---|---|
| "Oops! Something went wrong saving the screening." | "Screening not saved — Hb value out of range (0.4 g/dL). Expected 5.0–25.0." | Names the field and the bound. |
| "Are you sure?" | "Discard unit **BB-2026-004412** (PRBC, O−, expires 14 Mar)? This is permanent and will be recorded against your account." | Identifies the object, the consequence, and the accountability. |
| "Unit updated." | "BB-2026-004412 → reserved. Allocated to request #REQ-8831." | States the transition, not the verb. |
| "No results" | "No units match O− / PRBC / available. 3 O− units are quarantined pending TTI." | An empty state on a clinical filter must say what *is* there. |

### 2.4 Words and patterns to avoid

- **Banned adjectives:** *awesome, amazing, incredible, effortless, seamless, magical, insane.*
- **No false urgency.** No countdown timers, no "only 2 slots left", no artificially reddened
  stock levels. Real urgency (`blood_request.urgency = emergency`, stock below the critical
  threshold) is the only urgency the UI is allowed to express, and it must be traceable to data.
- **Never gamify donation.** No streaks, leaderboards, badges-for-count, or social pressure
  mechanics. A meaningful share of the population is permanently or temporarily ineligible
  (medication, travel, low Hb, weight, a reactive result). Gamification punishes those people
  for a fact about their body. **Impact framing is allowed** ("your donation supported up to
  3 patients"); **competitive framing is not**.
- **Never imply a diagnosis.** The words *positive*, *negative*, *infected*, *HIV*, *hepatitis*
  never appear in a donor-facing notification. See §13.3.
- **Never say "rejected" to a person.** The schema value is `deferred_temporary` /
  `deferred_permanent`; the UI says "deferred" and explains it. `rejected` is reserved for
  `donation_request.status` and `blood_request.status` — objects, not people.
- **No emoji in clinical surfaces.** Marketing may use none either; the current landing page
  correctly uses icon components, not emoji. Keep it that way.

---

## 3. The existing design system — audit

Read from `bbank/src/app/globals.css`. Usage counts are actual `grep` counts across
`bbank/src`, excluding `globals.css` itself.

### 3.1 Tokens on `:root`

| Token | Value | Purpose | Assessment |
|---|---|---|---|
| `--bg` | `#fafaf9` | Page canvas | Keep |
| `--surface` | `#ffffff` | Cards, sidebar, tables | Keep |
| `--text` | `#18181b` | Primary ink — **16.96:1** on `--bg` | Keep |
| `--text-2` | `#52525b` | Secondary ink — **7.40:1** | Keep |
| `--text-3` | `#a1a1aa` | Tertiary ink — **2.45:1 on `--bg`** | ⚠️ **Fails AA.** See §3.4 |
| `--accent` | `#e11d48` (rose-600) | Brand — **4.70:1** on white | Keep for brand; ⚠️ not for small text on fill |
| `--accent-strong` | `#be123c` (rose-700) | Hover, badge text — **6.29:1** | Keep; promote to the fill colour |
| `--accent-soft` | `rgba(225,29,72,.08)` | Tints | Keep |
| `--border` | `rgba(24,24,27,.07)` | Hairlines | Keep |
| `--border-strong` | `rgba(24,24,27,.13)` | Input & ghost-button borders | ⚠️ **1.34:1** vs white — fails SC 1.4.11 for input boundaries |
| `--shadow-color` | `rgba(24,24,27,.04)` | Resting shadow | Keep |
| `--shadow-color-strong` | `rgba(24,24,27,.12)` | Hover shadow | Keep |
| `--radius` | `16px` | Card radius | Keep |
| `--ease-out` | `cubic-bezier(.22,1,.36,1)` | The house curve | Keep — extend with a second curve for exits |

`@theme inline` maps four of these into Tailwind 4 (`--color-background`, `--color-foreground`,
`--color-accent`, `--font-sans`, `--font-mono`).

### 3.2 Utility classes and real usage

| Class | Uses | Verdict |
|---|---|---|
| `.label` | 64 | Core. Extend with a required-indicator and a `for`-association lint. |
| `.field` | 30 | Core. **Missing all validation states** — see §5.4. |
| `.animate-fade-up` | 30 | Core. Correct under `prefers-reduced-motion`. |
| `.headline` | 18 | Core. |
| `.eyebrow` | 13 | Core. Marketing + dashboard headers. |
| `.display-serif` / `.text-gradient` | 11 / 11 | Marketing only. Formalise that rule. |
| `.blob` | 11 | Marketing only. Correctly `aria-hidden`. |
| `.card-hover` / `.mesh` | 9 / 9 | Keep. |
| `.live-dot` | 7 | Keep, but see §12 — it blinks forever. |
| `.btn-sm` / `.btn-ghost` | 6 / 6 | ⚠️ `.btn-sm` computes to ~34 px tall — below the 44 px staff target. |
| `.card-spot` / `.btn-lg` | 5 / 5 | Keep. |
| `.blur-panel` / `.avatar` | 5 / 4 | Keep. |
| `.skeleton` / `.nav-link` / `.pulse-ring` / `.animate-scale-in` / `.animate-float` | 3 each | Keep. Skeleton needs a system (§3.4). |
| `.marquee` / `.animate-fade-in` | 2 each | Keep. |
| `.grid-lines` / `.grain-overlay` | 1 each | Keep. |
| `.card-soft` | **0** | **Dead code.** Delete or adopt — it is a shadow-only card with no border, useful for nested panels inside a `.card`. Recommendation: adopt for `InventoryGrid` cells. |
| `.badge` + `-accent` `-green` `-muted` | 3 variants | ⚠️ **Only three states exist.** The new scope needs 8 unit statuses + 3 urgencies + 6 request statuses. |
| `.table-modern` | 3 pages | ⚠️ One density only (`0.95rem` row padding). Unusable for a 200-row unit list. |

### 3.3 What is working

- **The token layer is real.** Colours, radius, easing and shadow are already centralised;
  extending it is additive, not a rewrite.
- **`prefers-reduced-motion` is handled** at `globals.css:384–391`, including the
  `.reveal { opacity: 1; transform: none }` override, which is the part most implementations
  get wrong. Verified correct: the media-query rule and the base rule share specificity, and
  the media-query rule wins on source order.
- **A skip link exists** in `layout.tsx` and is correctly `sr-only focus:not-sr-only`.
- **Focus-visible rings exist** on `.btn` and on every `SidebarNav` link.
- **Geist Mono is already loaded** and exposed as `--font-mono` but used exactly once. It is
  the right face for `unit_code`, donor IDs, and lot numbers — an asset that costs nothing new.
- **The `ToastAlert` pattern** (server action → `redirect(?success=|error=)` → client toast)
  is consistent and already has `role="status"`.

### 3.4 What is missing for the new scope

| Gap | Why it blocks the new scope | Fixed in |
|---|---|---|
| **No semantic status scale.** One rose accent plus `badge-green`. | 8 `blood_unit.status` values, 3 `blood_request.urgency` values, 6 `blood_request.status` values, 4 `test_result.result` values. Rose cannot mean both "brand" and "discarded". | §5.1 |
| **No data-density variant.** `.table-modern` rows are `0.95rem 1rem`. | The inventory unit list is 200–2000 rows. At current density, 11 rows fit on a laptop screen. | §5.3 |
| **No form validation states.** `.field` has `:hover` and `:focus`, nothing else. | Screening captures 7 clinically bounded numeric values. Out-of-range must be visible before submit. | §5.4 |
| **No dark mode.** Not a single `prefers-color-scheme` rule. | Night shift, dimmed collection rooms. | **Decision: out of scope for v1.** See §5.7 |
| **No chart / data-viz tokens.** | The inventory dashboard is the highest-value new screen and it is fundamentally a data visualisation. | §5.6 |
| **No skeleton system.** `.skeleton`, `.skeleton-block`, `.skeleton-card` exist; one `loading.tsx` uses them. | Every console needs a loading state that matches its own layout. | §5.8 |
| **No elevation scale.** Two ad-hoc `box-shadow`s. | Modals, popovers, sticky table headers, and the command palette all need to stack predictably. | §5.5 |
| **`--text-3` fails contrast.** 2.45:1 on `--bg`. Used for `.table-modern thead th`, `.field::placeholder`, and footer links. | Table column headers carry meaning. This is a straight AA failure. | §5.2 |
| **`--border-strong` is 1.34:1.** | SC 1.4.11 requires 3:1 for the visual boundary of a form control. Every `.field` currently fails. | §5.2 |
| **`.btn-sm` is ~34 px tall.** | Passes SC 2.5.8 (24 px) but fails the product's own 44 px gloved-hands target. | §5.4 |
| **`Reveal` hides content behind JS.** `.reveal` starts at `opacity: 0`; if the `IntersectionObserver` never fires, the content is invisible. | Public marketing content should not depend on JS to be visible. | §12.4 |
| **`* { scroll-behavior: smooth }` is not disabled under reduced motion.** | Vestibular trigger. One-line fix. | §12.3 |

---

## 4. Zone model

Before the tokens, the rule that governs where they apply.

| Zone | Routes | Visual register |
|---|---|---|
| **Marketing** | `/`, `/faq`, `/thank-you`, `/eligibility`, `/centers`, `/privacy`, `/terms`, `/login`, `/signup` | Full existing expression: `mesh`, `blob`, `grid-lines`, `display-serif`, `text-gradient`, `Reveal`, `animate-float`, `pulse-ring`. |
| **Donor portal** | `/donor/*` | Reduced expression: `card`, `eyebrow`, `headline`, `animate-fade-up`. One `display-serif` moment per page maximum. No blobs, no float. |
| **Clinical console** | `/staff/*`, `/lab/*`, `/inventory/*`, `/hospital/*`, `/admin/*` | Flat. `card` without `card-hover` on data surfaces, `table-modern`, `StatusBadge`, mono for identifiers. **No `Reveal`, no `blob`, no gradient text, no serif.** Motion limited to ≤150 ms state feedback. |

Same tokens, three registers. This is the single most important structural decision in the brief.

---

## 5. Design tokens — proposed extensions

Paste-ready CSS, in the existing naming convention (spelled-out kebab-case on `:root`).
All contrast ratios below were computed against the actual hex values.

### 5.1 Semantic status palette

Each status is a **triplet** (`-bg`, `-fg`, `-line`) matching the existing `.badge-green`
shape (`background` / `color` / `1px solid border`), plus a documented **glyph** and **label**.
`-fg` on `-bg` is the ratio that matters, since that is the badge's own text.

```css
:root {
  /* ---------- blood_unit.status ---------- */
  --status-quarantined-bg:   #fffbeb;  --status-quarantined-fg:   #92400e;  --status-quarantined-line:   #fde68a;
  --status-available-bg:     #ecfdf5;  --status-available-fg:     #065f46;  --status-available-line:     #a7f3d0;
  --status-reserved-bg:      #eef2ff;  --status-reserved-fg:      #3730a3;  --status-reserved-line:      #c7d2fe;
  --status-issued-bg:        #eff6ff;  --status-issued-fg:        #1e40af;  --status-issued-line:        #bfdbfe;
  --status-transfused-bg:    #f0fdfa;  --status-transfused-fg:    #115e59;  --status-transfused-line:    #99f6e4;
  --status-expired-bg:       #f4f4f5;  --status-expired-fg:       #3f3f46;  --status-expired-line:       #e4e4e7;
  --status-discarded-bg:     #fef2f2;  --status-discarded-fg:     #991b1b;  --status-discarded-line:     #fecaca;
  --status-recalled-bg:      #fff7ed;  --status-recalled-fg:      #9a3412;  --status-recalled-line:      #fed7aa;

  /* ---------- blood_request.urgency ---------- */
  --urgency-routine-bg:      #f1f5f9;  --urgency-routine-fg:      #334155;  --urgency-routine-line:      #cbd5e1;
  --urgency-urgent-bg:       #fffbeb;  --urgency-urgent-fg:       #92400e;  --urgency-urgent-line:       #fde68a;
  --urgency-emergency-bg:    #991b1b;  --urgency-emergency-fg:    #fef2f2;  --urgency-emergency-line:    #7f1d1d;

  /* ---------- stock level (derived, not a column) ---------- */
  --level-critical-bg:       #fef2f2;  --level-critical-fg:       #991b1b;  --level-critical-line:       #fecaca;
  --level-low-bg:            #fffbeb;  --level-low-fg:            #92400e;  --level-low-line:            #fde68a;
  --level-adequate-bg:       #ecfdf5;  --level-adequate-fg:       #065f46;  --level-adequate-line:       #a7f3d0;
  --level-surplus-bg:        #eff6ff;  --level-surplus-fg:        #1e40af;  --level-surplus-line:        #bfdbfe;
}
```

| Value | fg / bg contrast | Glyph (`react-icons/fa6`) | Label | Meaning shown to the user |
|---|---|---|---|---|
| `quarantined` | **6.84:1** | `FaLock` | Quarantined | Collected, not yet released. Cannot be allocated. |
| `available` | **7.29:1** | `FaCircleCheck` | Available | Tested, released, allocatable. |
| `reserved` | **8.88:1** | `FaBookmark` | Reserved | Crossmatched and held for a specific request. |
| `issued` | **8.01:1** | `FaTruckMedical` | Issued | Handed over to the hospital. Out of stock. |
| `transfused` | **7.27:1** | `FaHeartPulse` | Transfused | Confirmed given to a patient. Chain complete. |
| `expired` | **9.50:1** | `FaHourglassEnd` | Expired | Past `expires_at`. Not an error — a fact. |
| `discarded` | **7.60:1** | `FaTrashCan` | Discarded | Destroyed. Requires a reason. Terminal. |
| `recalled` | **6.88:1** | `FaTriangleExclamation` | Recalled | **Look-back event.** Highest attention state. |
| `routine` | **9.45:1** | `FaClock` | Routine | Needed by the stated date. |
| `urgent` | **6.84:1** | `FaCircleExclamation` | Urgent | Needed within hours. |
| `emergency` | **7.60:1** | `FaTriangleExclamation` | Emergency | Solid fill — the only inverted badge in the system. |
| `critical` | **8.31:1** | `FaCircleExclamation` | Critical | Below the policy floor for this group. |
| `low` | **7.09:1** | `FaCircleMinus` | Low | Below target. |
| `adequate` | **7.68:1** | `FaCircleCheck` | Adequate | At or above target. |
| `surplus` | **8.72:1** | `FaCircleArrowUp` | Surplus | Above target — watch expiry, consider transfer. |

All pass **AA for normal text (4.5:1)** and all but two pass **AAA (7:1)**. The badge font is
`0.78rem` semibold, which counts as *normal* text under WCAG, so 4.5:1 is the correct bar.

**Colour-blind safety.** `quarantined` / `urgent`, `discarded` / `critical`, and `recalled`
sit in the amber–orange–red band and are *not* reliably separable under deuteranopia or
protanopia. This is unavoidable — the palette encodes blood. The mitigations are structural,
not chromatic:

1. **Glyph is mandatory**, never optional, and each of those five uses a different glyph shape
   (lock / triangle / minus / trash / rotate).
2. **Text label is mandatory** at every size except the compact table density, where it is
   still present as the `aria-label` and the tooltip.
3. **`quarantined` carries a 4 px diagonal hatch** on its badge background; `recalled` carries
   a 3 px solid left rule. Both are pure-luminance cues that survive greyscale.

```css
.badge-status { border-radius: 99px; border-width: 1px; border-style: solid; }
.badge-status[data-status="quarantined"] {
  background:
    repeating-linear-gradient(45deg,
      rgba(146,64,14,.10) 0 2px, transparent 2px 6px),
    var(--status-quarantined-bg);
  color: var(--status-quarantined-fg);
  border-color: var(--status-quarantined-line);
}
.badge-status[data-status="recalled"] {
  background: var(--status-recalled-bg);
  color: var(--status-recalled-fg);
  border-color: var(--status-recalled-line);
  box-shadow: inset 3px 0 0 var(--status-recalled-fg);
}
/* the remaining six follow the plain triplet pattern */
```

### 5.2 Contrast repairs to existing tokens

```css
:root {
  /* Was #a1a1aa (2.45:1 on --bg) — failed AA wherever it carried meaning.
     Keep the old value ONLY for decorative rules; introduce a legible tertiary. */
  --text-3: #71717a;          /* zinc-500 — 4.63:1 on --bg, 4.83:1 on --surface */
  --text-faint: #a1a1aa;      /* decorative only: separators, watermark numerals */

  /* Was rgba(24,24,27,.13) — 1.34:1, failed SC 1.4.11 for control boundaries. */
  --border-strong: rgba(24, 24, 27, 0.28);   /* ~3.1:1 against --surface */

  /* Small text on a rose fill needs rose-700, not rose-600. See §6.1. */
  --accent-fill: #be123c;     /* white on this = 6.29:1 */
  --accent-fill-hover: #9f1239;
}
```

`.btn-primary` changes its background from `var(--accent)` to `var(--accent-fill)`.
`--accent` remains the brand colour for icons, `.eyebrow`, `.text-gradient`, focus rings, and
the logo mark — all uses where it is not carrying small text on top of itself.

### 5.3 Data density scale

Three densities. `comfortable` is today's `.table-modern` and stays the default on donor and
admin overview surfaces. `compact` and `dense` are opt-in for clinical worklists.

```css
:root {
  --row-y-comfortable: 0.95rem;  --row-font-comfortable: 0.92rem;  --row-min-comfortable: 3.25rem;
  --row-y-compact:     0.55rem;  --row-font-compact:     0.875rem; --row-min-compact:     2.5rem;
  --row-y-dense:       0.3rem;   --row-font-dense:       0.8125rem;--row-min-dense:       2rem;

  --row-y:    var(--row-y-comfortable);
  --row-font: var(--row-font-comfortable);
  --row-min:  var(--row-min-comfortable);
}

[data-density="compact"] { --row-y: var(--row-y-compact); --row-font: var(--row-font-compact); --row-min: var(--row-min-compact); }
[data-density="dense"]   { --row-y: var(--row-y-dense);   --row-font: var(--row-font-dense);   --row-min: var(--row-min-dense); }

.table-modern            { font-size: var(--row-font); }
.table-modern tbody td   { padding-block: var(--row-y); height: var(--row-min); }
.table-modern thead th   { color: var(--text-3); }          /* now 4.63:1, was 2.45:1 */

/* Sticky header for long worklists */
.table-modern.is-sticky thead th {
  position: sticky; top: 0; z-index: 2;
  background: var(--surface);
  box-shadow: 0 1px 0 var(--border);
}
/* Zebra only at dense — at comfortable it fights the hairline borders */
[data-density="dense"] .table-modern tbody tr:nth-child(even) { background: rgba(24,24,27,.015); }
/* Row animation is a marketing flourish; kill it on clinical tables (see §12) */
[data-density="compact"] .table-modern tbody tr,
[data-density="dense"]   .table-modern tbody tr { animation: none; }

/* Numeric and identifier columns */
.cell-num  { font-variant-numeric: tabular-nums; text-align: right; }
.cell-code { font-family: var(--font-geist-mono), ui-monospace, monospace; font-size: 0.8125rem; letter-spacing: -0.02em; }
```

Rows fitting a 900 px viewport: **comfortable ≈ 15 · compact ≈ 22 · dense ≈ 30.**
Density is a per-user preference persisted in `localStorage` and applied on the console shell.

### 5.4 Form validation states & control sizing

```css
:root {
  --valid-fg:   #065f46;  --valid-line:   #10b981;  --valid-glow:   rgba(16,185,129,.12);
  --invalid-fg: #991b1b;  --invalid-line: #dc2626;  --invalid-glow: rgba(220,38,38,.12);
  --warn-fg:    #92400e;  --warn-line:    #d97706;  --warn-glow:    rgba(217,119,6,.12);

  /* Gloved-hands target. Above SC 2.5.8 (24px) by product decision. */
  --target-min: 44px;
}

.field[aria-invalid="true"],
.field.is-invalid {
  border-color: var(--invalid-line);
  box-shadow: 0 0 0 4px var(--invalid-glow);
}
.field.is-valid   { border-color: var(--valid-line); }
.field.is-warning { border-color: var(--warn-line); box-shadow: 0 0 0 4px var(--warn-glow); }
.field:disabled   { background: #fafafa; color: var(--text-3); cursor: not-allowed; }
.field:read-only  { background: #fafafa; border-style: dashed; }

/* Error text — must be referenced by aria-describedby on the input */
.field-error {
  display: flex; align-items: flex-start; gap: 0.35rem;
  margin-top: 0.35rem; font-size: 0.8rem; font-weight: 500;
  color: var(--invalid-fg);
}
.field-hint { margin-top: 0.3rem; font-size: 0.78rem; color: var(--text-3); }

/* Unit suffix printed inside the control: "12.4 [g/dL]" */
.field-group { position: relative; display: flex; align-items: center; }
.field-group .field { padding-right: 3.5rem; }
.field-group .field-unit {
  position: absolute; right: 0.9rem;
  font-size: 0.8rem; font-weight: 600; color: var(--text-3);
  pointer-events: none; user-select: none;
}

/* Required marker on .label */
.label[data-required]::after { content: " *"; color: var(--invalid-fg); font-weight: 700; }

/* Staff-surface sizing: raise every interactive element to the gloved target */
[data-zone="clinical"] .btn,
[data-zone="clinical"] .field,
[data-zone="clinical"] .btn-sm { min-height: var(--target-min); }
[data-zone="clinical"] .btn-sm { padding: 0.6rem 1.1rem; font-size: 0.9rem; }
```

### 5.5 Elevation scale

```css
:root {
  --elev-0: none;
  --elev-1: 0 1px 3px var(--shadow-color);                                   /* .card resting */
  --elev-2: 0 18px 36px -20px var(--shadow-color-strong);                    /* .card-hover  */
  --elev-3: 0 12px 28px -8px rgba(24,24,27,.14), 0 2px 6px rgba(24,24,27,.06); /* popover, dropdown */
  --elev-4: 0 32px 64px -16px rgba(24,24,27,.24), 0 4px 12px rgba(24,24,27,.08); /* modal */

  --z-base: 0;  --z-sticky: 20;  --z-nav: 50;  --z-dropdown: 60;
  --z-modal-backdrop: 80;  --z-modal: 90;  --z-toast: 100;  --z-grain: 9999;
}
```

`--z-grain: 9999` documents the existing `.grain-overlay` value. Note that the grain sits above
modals; it is `pointer-events: none` so it does not block interaction, and at `opacity: .35`
over a white modal it is visually harmless. **Leave it.** Do not add a competing overlay.

### 5.6 Data-visualisation tokens

```css
:root {
  /* Categorical series — luminance-separated so they survive greyscale.
     Ordered by lightness, not by hue. Use in this order. */
  --viz-1: #9f1239;   /* rose-800    L≈0.081  */
  --viz-2: #1e40af;   /* blue-800    L≈0.087  */
  --viz-3: #0f766e;   /* teal-700    L≈0.155  */
  --viz-4: #b45309;   /* amber-700   L≈0.159  */
  --viz-5: #6d28d9;   /* violet-700  L≈0.113  */
  --viz-6: #52525b;   /* zinc-600    L≈0.089  */

  /* Sequential ramp for stock level — one hue, five steps.
     Never use the 8-colour categorical set for the 8 blood groups. */
  --viz-ramp-0: #fef2f2;  --viz-ramp-1: #fecdd3;  --viz-ramp-2: #fb7185;
  --viz-ramp-3: #e11d48;  --viz-ramp-4: #9f1239;

  --viz-grid:  rgba(24,24,27,.06);
  --viz-axis:  var(--text-3);
  --viz-track: #f4f4f5;              /* empty portion of a bar/meter */
  --viz-target-line: rgba(24,24,27,.35);  /* dashed target marker */
}
```

**Data-viz rules, opinionated.**

1. **Never encode blood group by colour.** Eight categorical colours are unreadable and there
   is no meaningful ordering between `A+` and `B−`. Blood group is encoded by **position in a
   fixed 4×2 grid** and by the mono `BloodGroupChip` label. Colour in the inventory grid encodes
   **stock level against target**, which is ordinal and takes the sequential ramp.
2. **Measured adjacent-series contrast is 1.07:1 (rose vs amber) and 1.15:1 (teal vs indigo).**
   Hue alone does not separate them. Every multi-series chart must therefore carry **direct
   labels at the end of each series**, not a detached legend, and must vary **line dash pattern**
   (`solid / 6 2 / 2 2 / 8 2 4 2`) alongside colour.
3. **Charts need a table.** Every chart renders a visually-hidden `<table>` with the same numbers
   and links to the underlying filtered list. A chart is a shortcut, never the only access path.
4. **No animation on first paint** for any chart in the clinical zone. See §12.
5. **No pie charts.** The inventory questions are all comparisons against a target; bars and a
   grid answer them, pies do not.

### 5.7 Dark mode — decision

**Not in v1. Recorded as a deliberate decision, not an omission.**

Rationale: the whole system is built on `:root`-only tokens with no `@media (prefers-color-scheme)`
layer and a large number of hard-coded Tailwind colour utilities in JSX (`bg-rose-50`,
`text-zinc-500`, `bg-white`) that would each need a `dark:` counterpart. Retrofitting is a
multi-day sweep with a real regression surface, and no user has asked. The token structure
in §5.1 is deliberately triplet-shaped so that a future dark theme is a **single
`@media` block redefining `-bg` / `-fg` / `-line`**, not a redesign.

Prerequisite before anyone attempts it: replace hard-coded Tailwind colour utilities in JSX
with token-backed classes. Track as a separate item in the implementation plan.

### 5.8 Layout rhythm & skeletons

Tailwind 4 already owns the spacing scale. **Do not introduce a parallel numeric scale** — it
would drift. Instead, formalise a small **layout contract**:

```css
:root {
  --gutter-page:    1.5rem;   /* px-6  — page edge padding, mobile & desktop */
  --gutter-console: 3rem;     /* lg:px-12 — clinical console horizontal padding */
  --shell-max:      64rem;    /* max-w-5xl — dashboard content column */
  --shell-max-wide: 90rem;    /* max-w-[90rem] — inventory grid + tables need more */
  --shell-max-marketing: 72rem; /* max-w-6xl — landing */
  --stack-section:  5rem;     /* py-20 — marketing section rhythm */
  --stack-block:    2rem;     /* mb-8  — console block rhythm */
  --sidebar-w:      15rem;    /* w-60  — SidebarNav */
}
```

**Allowed spacing steps** (everything else needs a reason in review):
`0.25 · 0.5 · 0.75 · 1 · 1.25 · 1.5 · 2 · 2.5 · 3 · 4 · 5 · 6 rem`.

Skeleton system — extend the three existing classes into shapes that match real layouts:

```css
.skeleton-text  { height: 0.85rem; border-radius: 6px; }
.skeleton-text + .skeleton-text { margin-top: 0.5rem; }
.skeleton-text.w-3\/4 { width: 75%; }
.skeleton-chip  { height: 1.6rem; width: 3.25rem; border-radius: 99px; }
.skeleton-row   { height: var(--row-min); border-radius: 8px; }
.skeleton-tile  { height: 6.5rem; border-radius: var(--radius); }
.skeleton-grid  { display: grid; grid-template-columns: repeat(4, 1fr); gap: 0.75rem; }

/* The shimmer is decorative motion. Under reduced motion the global rule already
   collapses the duration; make the resting state a flat tone rather than a frozen gradient. */
@media (prefers-reduced-motion: reduce) { .skeleton { background: #e4e4e7; } }
```

Every route with a data fetch gets a `loading.tsx` whose skeleton **matches its own layout**
(tile grid for `/inventory`, sticky-header table for `/inventory/units`, form for `/staff/screening`).
A centred spinner is not acceptable on a console — it causes a layout jump on resolve.

---

## 6. Accessibility

**Target: WCAG 2.2 Level AA**, with two product decisions above AA (44 px targets on clinical
surfaces; 7:1 on clinical status badges wherever achievable).

### 6.1 The rose-600 finding — and the fix

`--accent: #e11d48` (rose-600) against white measures **4.70:1**
(foundation §7 records 4.6:1; both land on the same verdict).

| Usage | Ratio | AA verdict | Action |
|---|---|---|---|
| Rose-600 **text** on white/`--bg` (`.eyebrow`, icons, `.text-gradient`) | 4.70:1 | **Passes** normal text (4.5:1) | Keep as-is |
| **White text on rose-600 fill** (`.btn-primary`, logo mark, skip link, mobile nav CTA) at `0.925rem` / 600 weight | 4.70:1 | **Passes by 0.2** — no headroom, fails under any sub-pixel rendering or user contrast adjustment | **Change the fill to `--accent-fill: #be123c` (rose-700) → 6.29:1** |
| White text on rose-600 at `btn-lg` (`1rem`/600 ≈ 16 px semibold) | 4.70:1 | Still *normal* text under WCAG (large = 18.66 px bold or 24 px) | Same fix |
| Rose-600 as a **focus ring** on white | 4.70:1 | Passes SC 1.4.11 (3:1 non-text) | Keep — but see 6.4 |

**Concrete change:** `.btn-primary { background: var(--accent-fill); border-color: var(--accent-fill-hover); }`
and `:hover { background: var(--accent-fill-hover); }` (rose-800, 8.02:1). Visually this is a
barely-perceptible deepening of the button; measurably it moves from marginal-pass to
comfortable-pass. The `box-shadow` glow keeps the rose-600 rgba value and is unaffected.

The `::selection { background: var(--accent); color: #fff }` rule has the same 4.70:1 issue.
Change to `var(--accent-fill)`.

### 6.2 Never colour alone

Enforced by making it impossible in the component API: `StatusBadge` takes only a `status`
enum and renders glyph + label + tint from a single lookup table. There is no prop that
suppresses the label at any size — the compact variant hides the text *visually* but keeps it
in an `aria-label` and a `title`, and always keeps the glyph.

Applies equally to: expiry indicators (bar + "expires in 41 h" text), stock levels (ramp fill +
"Critical — 2 of 12" text), row highlighting in worklists (left rule + a status column, never a
tinted row alone), and the compatibility verdict on allocation screens (words: "Compatible —
O− red cells to an A+ recipient").

### 6.3 Keyboard path for the entire staff workflow

**Requirement: a phlebotomist can complete check-in → screening → collection with no mouse.**

| Surface | Keyboard contract |
|---|---|
| `/staff` (today's queue) | `/` focuses search. `↑ ↓` move row focus. `Enter` opens the focused appointment. `c` checks in the focused row (with confirm). Row focus is a real roving-tabindex, not a hover simulation. |
| `/staff/check-in` | Single search input autofocused. Scanning a donor barcode types into it and submits on `Enter`. Results are an `aria-activedescendant` listbox; `↓`/`Enter` select. |
| `/staff/screening/[id]` | Fields in clinical order: weight → temp → pulse → BP systolic → BP diastolic → Hb → questionnaire. `Tab` and `Enter` both advance. Out-of-range shows inline **without stealing focus**. Outcome is a radio group (`passed` / `deferred_temporary` / `deferred_permanent`) reachable by `Tab`, chosen by `← →`. `Ctrl+Enter` submits from anywhere in the form. |
| `/staff/collection/[id]` | Volume, bag lot number, adverse-reaction toggle, then submit. Bag lot accepts a scanner. |
| Any modal | Focus moves to the modal's heading on open; `Tab` cycles inside; `Esc` closes (except `ConfirmDialog` mid-typing, which requires an explicit Cancel); focus returns to the trigger on close. |
| Global | `Ctrl/⌘ K` opens a command palette scoped to the current console. `Esc` always dismisses the topmost layer. |

Every shortcut is discoverable: a `?` key opens a shortcut sheet, and the primary actions
display their shortcut in the button (`Save screening ⌃↵`).

### 6.4 Focus appearance (SC 2.4.11 / 2.4.13)

The current ring is `outline: 2px solid var(--accent); outline-offset: 3px`. On a white page
that is 4.70:1 — fine. On a rose-tinted surface (`bg-rose-50` cards, the active `SidebarNav`
item, a rose fill) it disappears. Fix with a two-tone ring that works on any ground:

```css
:where(a, button, input, select, textarea, [tabindex]):focus-visible {
  outline: 2px solid var(--accent-fill);
  outline-offset: 2px;
  box-shadow: 0 0 0 4px #fff, 0 0 0 6px var(--accent-fill);
  border-radius: inherit;
}
[data-zone="clinical"] :where(a, button, input, select, textarea, [tabindex]):focus-visible {
  outline-width: 3px;   /* thicker at arm's length under fluorescent light */
}
```

### 6.5 Clinical tables — screen readers

- `<caption>` on every data table, visually hidden, stating what the table is and how many rows
  are shown of how many total: *"Blood units — 42 of 1,208 shown, filtered by O− and available."*
- `<th scope="col">` on every header (`.table-modern thead th` currently renders bare `th`).
- The identifier column is `<th scope="row">` so a screen reader announces the unit code with
  every cell.
- Sortable headers use a real `<button>` inside the `th` with `aria-sort="ascending|descending|none"`.
- Filter changes announce into a single `aria-live="polite"` region: *"42 units match."*
  One region per table, debounced 400 ms — never one per filter chip.
- Row actions are named by object, not by verb: `aria-label="Discard unit BB-2026-004412"`,
  not `aria-label="Discard"`. A screen-reader user tabbing a 40-row table must not hear
  "Discard, Discard, Discard".
- Pagination is a `<nav aria-label="Blood units pagination">`.

### 6.6 Form error association

Every invalid control gets `aria-invalid="true"` **and** `aria-describedby` pointing at its
`.field-error` id. Errors are rendered adjacent to the field, never only in a toast — the
current `redirect(?error=)` → `ToastAlert` pattern loses the field association entirely and
must be supplemented (not replaced) with `useActionState` field-level errors on the screening,
collection, and blood-request forms.

A form-level error summary sits above the form, is focused on failed submit, and links to each
bad field. `ToastAlert`'s container should be `role="status"` for success (it already is) and
`role="alert"` for errors — a failed clinical write must interrupt.

### 6.7 Touch targets

WCAG 2.2 SC 2.5.8 requires **24×24 CSS px**. Measured today:

| Element | Approx. size | 2.5.8 (24 px) | Product target (44 px) |
|---|---|---|---|
| `.btn` | ~44×auto | Pass | Pass |
| `.btn-lg` | ~52 | Pass | Pass |
| `.btn-sm` | ~34 | Pass | **Fail** → §5.4 raises it in the clinical zone |
| `.field` | ~42 | Pass | **Marginal** → `min-height: 44px` in the clinical zone |
| `SidebarNav` link | ~36 | Pass | **Fail** → `py-2.5` + `min-height: 44px` |
| Navbar mobile toggle | ~36 | Pass | **Fail** → `p-3` |
| `ToastAlert` dismiss | ~26 | Pass (barely) | **Fail** → `p-2.5`, and the toast auto-dismisses anyway |
| `.badge` | ~26 | Pass, and it is not interactive | n/a |

Clinical surfaces additionally require **8 px minimum spacing** between adjacent destructive
and non-destructive actions in a row, and destructive actions are never the first control in
tab order within a row.

### 6.8 Reduced motion — verified state

**Verified working** (`globals.css:384–391`): the global rule collapses all animation and
transition durations and explicitly resets `.reveal` to `opacity: 1; transform: none`. The
specificity analysis holds — the media-query `.reveal` rule and the base `.reveal` rule are
both `(0,1,0)` and the media-query rule wins on source order, so revealed content is visible.
`animation-iteration-count: 1` also stops `.animate-float`, `.pulse-ring`, `.live-dot`, and
`.marquee`.

**Two gaps to fix:**

```css
/* 1. Smooth scrolling is a vestibular trigger and is currently applied unconditionally
      via `* { scroll-behavior: smooth }` at globals.css:43. */
@media (prefers-reduced-motion: reduce) { * { scroll-behavior: auto !important; } }

/* 2. Reveal is a JS-gated visibility gate. If the observer never runs, content is invisible. */
@media (scripting: none) { .reveal { opacity: 1; transform: none; } }
```

`Reveal.tsx` should additionally bail out early — check
`window.matchMedia('(prefers-reduced-motion: reduce)').matches` and add `revealed` immediately,
so no observer is created at all.

### 6.9 Other AA obligations

- `lang="en"` is set. Add `lang` switching only if a French locale ships (Douala — likely; flag
  it, do not build it now).
- The skip link exists; **add a second skip target** inside the console shell
  (`Skip to console content`) because the sidebar is 6+ links before the main region.
- All decorative imagery (`blob`, `grid-lines`, `grain-overlay`, the blood-type marquee) is
  already `aria-hidden` — verified.
- The blood-type marquee is `aria-hidden` on the whole `<section>`, which is correct.
- Timeouts: the `ToastAlert` 5 s auto-dismiss is acceptable under SC 2.2.1 only because the
  same information is available in the page state. Error toasts on clinical writes must **not**
  auto-dismiss.
- Session expiry must warn at 2 minutes with an extend option (SC 2.2.1), and must never
  silently discard an in-progress screening.

---

## 7. Information architecture

### 7.1 Sitemap

`[E]` = exists today · `[N]` = new · `[R]` = exists, renamed/repurposed
`[+]` = extension flagged beyond foundation §6, with rationale

```
/                                        [E]  Landing
├── /faq                                 [N]  FAQ accordion + FAQPage JSON-LD
├── /thank-you                           [N]  ?flow=signup | ?flow=booking
├── /eligibility                     [+][N]  Self-check before booking — cuts desk-side deferrals
├── /centers                         [+][N]  Centre locator; donation_centers already in schema
├── /login                               [E]
├── /signup                              [E]
├── /hospitals/register              [+][N]  Partner hospital onboarding (hospitals.status = pending)
├── /privacy                             [E]
├── /terms                               [E]
├── /not-found                           [E]  Verify + add dashboard-scoped variant
│
├── /donor                                    ── DONOR PORTAL (mobile-first)
│   ├── /donor/[id]                      [R]  Portal home: eligibility, next appt, history, impact
│   ├── /donor/[id]/book                 [N]  Create a donation_request
│   ├── /donor/[id]/appointments         [N]  Upcoming + past, cancel/reschedule
│   ├── /donor/[id]/donations            [N]  Donation history + outcome
│   ├── /donor/[id]/eligibility          [N]  Countdown, deferrals, policy explanation
│   └── /donor/settings                  [E]
│
├── /staff                                    ── STAFF CONSOLE (desktop-first, tablet-capable)
│   ├── /staff                           [N]  Today's queue at this centre
│   ├── /staff/check-in                  [N]  Donor lookup + arrival
│   ├── /staff/screening/[appointmentId] [N]  Vitals + questionnaire → outcome
│   ├── /staff/collection/[appointmentId][N]  Record the donation, mint units
│   └── /staff/donors                    [N]  Registry lookup (read-only + walk-in registration)
│
├── /lab                                      ── LAB CONSOLE (desktop-first)
│   ├── /lab                             [N]  Pending TTI worklist
│   ├── /lab/donations/[id]              [N]  Enter the 5-test panel
│   └── /lab/quarantine                  [N]  Everything held, oldest first
│
├── /inventory                                ── INVENTORY CONSOLE (desktop-first)
│   ├── /inventory                       [N]  THE dashboard — stock, expiry, unfillable demand
│   ├── /inventory/units                 [N]  Filterable unit register (dense table)
│   ├── /inventory/units/[unitCode]      [N]  Provenance / vein-to-vein timeline
│   ├── /inventory/processing            [N]  Whole blood → PRBC / FFP / platelets split
│   ├── /inventory/expiry                [N]  72 h / 7 d / 30 d sweeps
│   └── /inventory/storage               [N]  Locations, temperature, capacity
│
├── /hospital                                 ── HOSPITAL PORTAL (must work on a phone)
│   ├── /hospital                        [N]  My requests + fulfilment status
│   ├── /hospital/requests/new           [N]  Raise a blood_request (+ emergency fast path)
│   ├── /hospital/requests/[id]          [N]  Allocation + issuance detail
│   └── /hospital/stock                  [N]  Live availability by group/component
│
└── /admin                                    ── ADMIN
    ├── /admin                           [E]  Ops overview (repurposed from the CRUD stub)
    ├── /admin/donors                    [E]
    ├── /admin/donation-requests         [R]  was /admin/requests — vocabulary rename
    ├── /admin/appointments              [E]
    ├── /admin/blood-requests            [N]  Hospital demand queue
    ├── /admin/users                     [N]  Replaces the hardcoded admin credential (P1-3)
    ├── /admin/hospitals                 [N]  Approve partners
    ├── /admin/centers                   [N]
    ├── /admin/policies                  [N]  Eligibility rules, shelf lives, stock targets
    ├── /admin/reports                   [N]
    ├── /admin/audit                     [N]  audit_log viewer
    └── /admin/settings                  [E]
```

**Flagged additions (beyond foundation §6), with rationale.** `/eligibility` — a self-check
before booking is the cheapest possible reduction in desk-side deferrals, and it is the humane
place to explain a permanent deferral. `/centers` — `donation_centers` exists in the model and
a donor cannot book without choosing one. `/hospitals/register` — `hospitals.status` implies an
approval workflow, which implies a public entry point.

### 7.2 Navigation model for six roles

`SidebarNav` today hardcodes a two-branch ternary (`role === 'admin' ? [...] : [...]`). Six roles
cannot live in a ternary. **Proposed strategy — console-scoped shells driven by a nav manifest:**

1. **One manifest, `src/lib/nav.ts`**, exporting `NAV: Record<Role, NavSection[]>` where
   `NavSection = { label, items: { href, label, icon, match? }[] }`. `SidebarNav` becomes
   presentational and takes `sections` plus an `active` matcher. This is the whole refactor.
2. **One route group per console** (`(donor)`, `(staff)`, `(lab)`, `(inventory)`, `(hospital)`,
   `(admin)`), each with a `layout.tsx` that renders `<ConsoleShell role=… />`. Shared shell,
   different manifest. The sidebar's visual language does not change at all.
3. **Role → landing route** is a single map (`donor → /donor/[id]`, `staff → /staff`,
   `lab_tech → /lab`, `inventory_manager → /inventory`, `hospital_user → /hospital`,
   `admin → /admin`) used by login redirect, the logo link, and `proxy.ts`.
4. **`admin` sees everything.** Rather than duplicating every console into `/admin/*`, the admin
   sidebar gains a **"Consoles"** section linking into `/staff`, `/lab`, `/inventory` directly.
   One implementation, not six.
5. **Multi-role users** (a director who also screens) get a **console switcher** in the sidebar
   header — a small `<select>`-shaped control showing the active console. Never a hidden mode;
   the current console name is always visible above the nav.
6. **Centre context.** Staff, lab, and inventory are scoped to a `donation_center`. The active
   centre sits in the console header next to the switcher and is persisted per session. A user
   with one centre never sees the picker.
7. **Breadcrumbs carry depth, the sidebar carries breadth.** The sidebar is never more than one
   level; anything deeper (`/inventory/units/BB-2026-004412`) is expressed by `Breadcrumbs` (§9.3).
8. **Mobile.** The sidebar collapses to a bottom tab bar (≤4 items) on the donor portal and to a
   slide-over drawer on the clinical consoles. See §11.

---

## 8. Page inventory & layout specs

### 8.1 Full inventory

| Route | Role | Purpose | Key components | Priority | State |
|---|---|---|---|---|---|
| `/` | public | Convert to donor; route hospitals | `CTA`, `USPBar`, `Reveal`, stats, steps | P0 | Exists |
| `/faq` | public | Answer the 12 blocking questions | `FAQ`, `CTA`, JSON-LD | P0 | New |
| `/thank-you` | public | Confirm + carry a next action | `CTA`, calendar-add | P0 | New |
| `/eligibility` | public | Self-check before booking | form, `EmptyState` | P1 | New |
| `/centers` | public | Choose where to donate | card list, map link | P2 | New |
| `/login` `/signup` | public | Auth | `.field`, `.btn` | P0 | Exists |
| `/hospitals/register` | public | Partner onboarding | form | P2 | New |
| `/privacy` `/terms` | public | Legal | prose | P1 | Exists |
| `/donor/[id]` | donor | Portal home | eligibility ring, `StatusBadge`, timeline | P0 | Repurpose |
| `/donor/[id]/book` | donor | Request a slot | date picker, centre select | P0 | New |
| `/donor/[id]/appointments` | donor | Manage bookings | `DataTable` (comfortable), `ConfirmDialog` | P1 | New |
| `/donor/[id]/donations` | donor | History + impact | timeline, `BloodGroupChip` | P1 | New |
| `/donor/[id]/eligibility` | donor | Why / when | countdown, deferral explainer | P1 | New |
| `/donor/settings` | donor | Profile, contact, consent | form | P1 | Exists |
| `/staff` | staff | Today's queue | `DataTable` (compact), `StatusBadge` | **P0** | New |
| `/staff/check-in` | staff | Find + arrive a donor | search, `EmptyState` | **P0** | New |
| `/staff/screening/[id]` | staff | Vitals → outcome | validated numeric form, `ConfirmDialog` | **P0** | New |
| `/staff/collection/[id]` | staff | Record collection, mint units | form, `BloodGroupChip` | **P0** | New |
| `/staff/donors` | staff | Registry lookup | `DataTable`, walk-in form | P1 | New |
| `/lab` | lab_tech | TTI worklist | `DataTable` (dense), age indicator | **P0** | New |
| `/lab/donations/[id]` | lab_tech | Enter 5-test panel | radio matrix, `ConfirmDialog` | **P0** | New |
| `/lab/quarantine` | lab_tech | Held units, oldest first | `DataTable`, `ExpiryIndicator` | P1 | New |
| `/inventory` | inventory_manager | **The dashboard** | `InventoryGrid`, `ExpiryIndicator`, demand panel | **P0** | New |
| `/inventory/units` | inventory_manager | Unit register | `DataTable` (dense), filter rail | **P0** | New |
| `/inventory/units/[code]` | inventory_manager | Provenance | timeline, `StatusBadge`, `ConfirmDialog` | **P0** | New |
| `/inventory/processing` | inventory_manager | Component split | split form | P1 | New |
| `/inventory/expiry` | inventory_manager | 72 h / 7 d / 30 d | `DataTable`, `ExpiryIndicator` | P1 | New |
| `/inventory/storage` | inventory_manager | Locations & temps | grid, gauge | P2 | New |
| `/hospital` | hospital_user | My requests | `DataTable`, `StatusBadge` | **P0** | New |
| `/hospital/requests/new` | hospital_user | Raise a request | form + emergency path | **P0** | New |
| `/hospital/requests/[id]` | hospital_user | Fulfilment detail | timeline, allocation list | P1 | New |
| `/hospital/stock` | hospital_user | Live availability | `InventoryGrid` (read-only) | P1 | New |
| `/admin` | admin | Ops overview | tiles, `InventoryGrid` summary | P1 | Repurpose |
| `/admin/blood-requests` | admin | Demand queue | `DataTable`, approve/reject | **P0** | New |
| `/admin/users` | admin | User & role management | `DataTable`, `ConfirmDialog` | **P0** | New |
| `/admin/hospitals` | admin | Approve partners | `DataTable` | P1 | New |
| `/admin/donation-requests` | admin | Booking queue | `DataTable` | P1 | Rename |
| `/admin/appointments` `/admin/donors` | admin | Existing CRUD | `DataTable` | P1 | Exists |
| `/admin/centers` `/admin/policies` | admin | Configuration | forms | P2 | New |
| `/admin/reports` `/admin/audit` | admin | Analytics, audit trail | charts, `DataTable` (dense) | P2 | New |
| `/admin/settings` | admin | Account | form | P2 | Exists |

### 8.2 Landing page — with the CTA system and USP bar

```
┌──────────────────────────────────────────────────────────────────────────┐
│ NAVBAR (fixed, blur-panel on scroll)                                     │
│  ◆ BloodBank    Home  About  How it works  FAQ  Contact   Log in [Donate]│
└──────────────────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────────────────┐
│ HERO  .mesh + .grid-lines + 2× .blob                    pt-36 pb-20      │
│ ┌────────────────────────────────┐  ┌──────────────────────────────────┐ │
│ │ ●live  EVERY DROP COUNTS       │  │  [ hero image, .card p-2, float ]│ │
│ │                                │  │   ⌐ chip: "O− needed"            │ │
│ │ Give blood.                    │  │   ⌐ chip: "Appointment confirmed"│ │
│ │ Give someone tomorrow.  ← serif│  └──────────────────────────────────┘ │
│ │                                │                                       │
│ │ BBank tracks every unit from   │  ← COPY UPDATE: the current promise   │
│ │ the vein that gave it to the   │    ("priority matching") is not built.│
│ │ patient who receives it.       │    Say what the system now does.      │
│ │                                │                                       │
│ │ <CTA variant="primary">        │  ← ONE primary per viewport.          │
│ │   Become a donor  →            │                                       │
│ │ <CTA variant="secondary">      │                                       │
│ │   Request blood (hospitals)    │  ← the missing demand-side entry point│
│ │                                │                                       │
│ │ ●●●● Trusted by 10,847 donors  │                                       │
│ └────────────────────────────────┘                                       │
└──────────────────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────────────────┐
│ <USPBar>  bg-white, border-y, 4 columns, py-6                            │
│  ✓ Every unit    │ ⛓ Traceable   │ ▤ Live stock   │ ♡ Free health check │
│    screened &    │   vein-to-    │   for partner  │   with every        │
│    tested        │   vein        │   hospitals    │   donation          │
└──────────────────────────────────────────────────────────────────────────┘
   ↓ blood-type marquee (aria-hidden)  ·  stats  ·  how it works (3 steps)
   ↓ about  ·  FAQ teaser (3 Qs + "See all 12 →" to /faq)   ← NEW SECTION
   ↓ contact  ·  final CTA
```

**CTA placement rule, enforced in review:** exactly one `variant="primary"` per viewport
height. The landing page currently has three `btn-primary` instances competing
(hero, about-section ghost, final CTA). After the change: hero = primary "Become a donor"
+ secondary "Request blood"; about = tertiary link; final = primary "Become a donor today".
The `USPBar` and FAQ teaser carry no CTA at all.

**Copy correction (required).** "Hospitals with critical shortages can call our 24/7 line for
priority matching" describes a feature that does not exist. Replace with a link to
`/hospitals/register` and the real capability: "Partner hospitals can raise a blood request
online and see live stock."

### 8.3 Inventory dashboard — `/inventory`

**The brief: a blood bank director opens this at 06:00 and must answer three questions in under
ten seconds.** (1) What do I have? (2) What am I about to lose? (3) What can I not fill?
Everything else is secondary and lives below the fold.

```
┌───────────┬──────────────────────────────────────────────────────────────────────────┐
│ SIDEBAR   │ Inventory · Douala Centre ▾          [density ▾] [Ctrl K]  updated 06:04 │
│           ├──────────────────────────────────────────────────────────────────────────┤
│ ◆ BBank   │                                                                          │
│           │ ── 1. UNITS ON HAND ─────────────────────────────── [ Red cells ▾ ]       │
│ INVENTORY │  Colour = level vs target. Number = available units. Never colour alone.  │
│ ▸Overview │                                                                          │
│  Units    │  ┌────────┐┌────────┐┌────────┐┌────────┐                                │
│  Expiry   │  │  O−  ⊘ ││  O+  ✓ ││  A−  ⊖ ││  A+  ✓ │   ⊘ critical  ⊖ low            │
│  Process. │  │   2    ││   34   ││   6    ││   28   │   ✓ adequate  ↑ surplus         │
│  Storage  │  │ of 12  ││ of 30  ││ of 10  ││ of 25  │                                │
│           │  │▓▓░░░░░░││▓▓▓▓▓▓▓▓││▓▓▓▓▓░░░││▓▓▓▓▓▓▓▓│   +3 quarantined (O−)          │
│ ─────────  │  └────────┘└────────┘└────────┘└────────┘                                │
│ Settings  │  ┌────────┐┌────────┐┌────────┐┌────────┐                                │
│ Log out   │  │  B−  ⊖ ││  B+  ✓ ││ AB−  ⊘ ││ AB+  ⊖ │                                │
│           │  │   4    ││   19   ││   1    ││   5    │   Fixed 4×2 grid, always in     │
│           │  │ of 8   ││ of 20  ││ of 5   ││ of 8   │   this order. Position, not     │
│           │  │▓▓▓▓░░░░││▓▓▓▓▓▓▓▓││▓░░░░░░░││▓▓▓▓▓░░░│   colour, identifies the group. │
│           │  └────────┘└────────┘└────────┘└────────┘                                │
│           │                                                                          │
│           │ ── 2. EXPIRING ────────────────────  ── 3. UNFILLABLE DEMAND ──────────── │
│           │ ┌──────────────────────────────────┐ ┌──────────────────────────────────┐ │
│           │ │ ⏳ NEXT 72 HOURS          7 units│ │ ⚠ 2 requests cannot be filled    │ │
│           │ │ ────────────────────────────────│ │ ────────────────────────────────│ │
│           │ │ BB-…4412  O−  PRBC   ▓▓░ 41h   │ │ REQ-8831  ▮EMERGENCY             │ │
│           │ │ BB-…4418  A+  PLT    ▓░░ 18h   │ │ St. Mary's · AB− PRBC · 3 units  │ │
│           │ │ BB-…4420  A+  PLT    ▓░░ 18h   │ │ Have 1, need 3 · by 08:00        │ │
│           │ │ BB-…4431  B+  PRBC   ▓▓▓ 66h   │ │ [ View request ] [ Substitute ]  │ │
│           │ │ … 3 more                        │ │ ─────────────────────────────── │ │
│           │ │ [ Open expiry sweep → ]         │ │ REQ-8829  ▮urgent                │ │
│           │ │                                 │ │ Laquintinie · O− WB · 4 units    │ │
│           │ │ 7d: 23 units   30d: 61 units    │ │ Have 2, need 4 · by 18:00        │ │
│           │ └──────────────────────────────────┘ └──────────────────────────────────┘ │
│           │                                                                          │
│           │ ── BELOW THE FOLD ───────────────────────────────────────────────────────│
│           │ 4. Pending lab release  ·  5. 14-day collections vs issues (dual line)   │
│           │ 6. Storage locations & temperature  ·  7. Recent status events (dense)   │
└───────────┴──────────────────────────────────────────────────────────────────────────┘
```

**Data-viz specification**

| Element | Encoding | Rules |
|---|---|---|
| Group tile | Position = blood group (fixed `O− O+ A− A+ / B− B+ AB− AB+`). Big numeral = **available** count only. `of N` = the policy target from `policies`. | The numeral is `tabular-nums`, `text-4xl`, `font-bold`. Never animate it counting up. |
| Level | 8-segment bar filled proportionally + `--viz-ramp` tint on the tile border + glyph in the corner + the level word in the tooltip and `aria-label`. | Four cues for one fact. Colour is the *last* of the four. |
| Quarantined | A separate small line under the tile (`+3 quarantined`), **never added into the big number**. | The single most dangerous possible bug on this screen is counting unreleased units as stock. |
| Expiry bar | 3-segment `▓▓░` = a coarse position in the 72 h window, plus the exact remaining hours as text. | Under 24 h the row takes the `--level-critical` treatment and the `FaTriangleExclamation` glyph. |
| Unfillable demand | Sorted by `urgency` desc, then `needed_by` asc. Emergency renders the solid-fill `--urgency-emergency` badge. | This panel is **never** collapsed, never paginated, and never behind a tab. |
| Trend chart (below fold) | Two lines: collections and issues, 14 days. `--viz-1` solid, `--viz-2` dashed `6 2`, each **directly labelled at its right end**. | No legend. No area fill. No animation. Visually-hidden table beneath. |
| Freshness | "updated 06:04" in the header, with a manual refresh. | Stale stock is a patient-safety issue (TRD rule 2). If the cache is older than 60 s, say so. |

**Empty and degenerate states.** Zero units of a group renders the tile with a `0`, the
critical treatment, and the text "None on hand". A group with no configured target renders
`of —` and the neutral treatment, plus a link to `/admin/policies`. Never hide a group tile;
the absence of `AB−` is itself the information.

### 8.4 Staff check-in & screening — `/staff/check-in`, `/staff/screening/[id]`

Optimised for: gloved hands, a barcode scanner, a keyboard, and a person waiting in a chair.

```
CHECK-IN                                              SCREENING — Amina Njoya · 27 · F
┌─────────────────────────────────────────┐  ┌──────────────────────────────────────────┐
│ ⌕ [ Scan a card or type name / ID / ph ]│  │ ◀ Back    Appt #4412 · 09:00 · Douala    │
│   ↑ autofocused, accepts scanner input  │  │ ┌──────────────────────────────────────┐ │
│                                         │  │ │ ⚠ Last donation 41 days ago.         │ │
│ 3 results                               │  │ │   Whole-blood interval is 56 days.   │ │
│ ┌─────────────────────────────────────┐ │  │ │   Eligible from 6 April.             │ │
│ │ Amina Njoya      [A+] 27  ✓ eligible│ │  │ └──────────────────────────────────────┘ │
│ │ #D-1043 · 6 53 ** 29 · last 41d ago │ │  │   ↑ eligibility verdict ALWAYS above     │
│ ├─────────────────────────────────────┤ │  │     the form, never after submit         │
│ │ Amina N.         [O−] 34  ⊘ deferred│ │  │                                          │
│ │ #D-0881 · temporary until 12 Apr    │ │  │ VITALS            (⇥ advances in order)  │
│ ├─────────────────────────────────────┤ │  │ Weight     [  62.0  ] kg    ✓            │
│ │ Aminata Nj.      [B+] 41  ✓ eligible│ │  │ Temp       [  36.8  ] °C    ✓            │
│ └─────────────────────────────────────┘ │  │ Pulse      [   72   ] bpm   ✓            │
│                                         │  │ BP         [ 118 ]/[ 76 ] mmHg  ✓        │
│ ↓/↑ move · Enter check in · Esc clear   │  │ Haemoglobin[  12.1  ] g/dL              │
│                                         │  │            ⚠ Below 12.5 g/dL (female).   │
│ ─────────────────────────────────────── │  │              Suggests temporary deferral.│
│ Not in the registry?                    │  │                                          │
│ [ Register a walk-in donor ]            │  │ QUESTIONNAIRE   [ 0 of 11 ] ▸ expand     │
└─────────────────────────────────────────┘  │                                          │
                                             │ OUTCOME  (← → to choose)                 │
  · Search hits `pg_trgm` (TRD rule 9)       │ ( ) Passed                               │
  · No result list ever exceeds 8 rows       │ (•) Deferred — temporary                 │
  · Eligibility is computed server-side and  │ ( ) Deferred — permanent                 │
    shown BEFORE the row is actionable       │                                          │
                                             │ Defer until [ 2026-10-14 ]  auto: +56d   │
                                             │ Reason  [ Haemoglobin below threshold ▾ ]│
                                             │ Notes   [                              ] │
                                             │                                          │
                                             │        [ Cancel ]  [ Save screening ⌃↵ ] │
                                             └──────────────────────────────────────────┘
```

**Hard-to-get-wrong rules.**

- **Range validation is inline and live**, on blur, against `policies` — never only on submit.
  Out-of-range is a `.field.is-warning` with an explanatory sentence, **not** a blocker: a
  clinician may legitimately record an unusual value. Out-of-*possible*-range (Hb of 0.4) is
  `.is-invalid` and does block.
- **The system suggests, the human decides.** When Hb is below threshold, the UI pre-selects
  `deferred_temporary` and pre-fills `deferred_until` and a reason — and says so in a hint
  ("Suggested from the Hb value — change if the assessment differs"). It never auto-submits.
- **`deferred_permanent` requires a typed confirmation** through `ConfirmDialog` naming the
  donor. It ends a person's donor career; it gets more friction than anything else on the screen.
- **One screen, no scroll** at 1280×800 with the questionnaire collapsed. The questionnaire
  expands in place; it does not navigate.
- **Autosave a draft every 10 s** to `sessionStorage` keyed by appointment id, restored with a
  visible banner. A 2 a.m. mis-click must not cost a re-screen.
- **No `Reveal`, no `card-hover`, no float.** Clinical zone.

### 8.5 Unit detail / provenance — `/inventory/units/[unitCode]`

The vein-to-vein answer for one bag. Read top-to-bottom, forward in time.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Inventory / Units / BB-2026-004412                            ← Breadcrumbs  │
│                                                                              │
│  BB-2026-004412                     ⬤ Available      [ Reserve ] [ Discard ] │
│  ─────────────────                  ────────────                             │
│  ┏━━━━┓  Packed red cells · 280 mL                    ↑ StatusBadge, large    │
│  ┃ O− ┃  Collected 12 Mar 2026, 09:41                                        │
│  ┗━━━━┛  Expires  23 Apr 2026  ·  ▓▓▓▓▓▓░░ 41 days left                      │
│          Storage  Fridge 2 · Shelf B · 4.1 °C  ✓ in range                    │
│                                                                              │
│ ── PROVENANCE ───────────────────────────────────────────────────────────────│
│                                                                              │
│  ●  Donor          Amina Njoya · #D-1043 · A+ ✗ ← group mismatch guard       │
│  │                 Registered 4 Jan 2024 · 7th donation                      │
│  │                                                                           │
│  ●  Screening      12 Mar 09:12 · Passed · Hb 13.4 g/dL · BP 118/76          │
│  │                 by Etienne Fotso (staff)                    [ view ]      │
│  │                                                                           │
│  ●  Donation       12 Mar 09:41 · 450 mL whole blood · bag lot WB-2026-0338  │
│  │                 Phlebotomist Etienne Fotso · no adverse reaction          │
│  │                                                                           │
│  ●  Testing        12 Mar 16:20 · TTI panel complete                         │
│  │                 HIV 1/2 ✓ · HBsAg ✓ · HCV ✓ · Syphilis ✓ · Malaria ✓      │
│  │                 all non-reactive · by Dr. Ngo Bell (lab_tech)             │
│  │                                                                           │
│  ●  Processing     12 Mar 18:05 · split from BB-2026-004410 (whole blood)    │
│  │                 siblings: BB-…4413 (FFP) · BB-…4414 (platelets)           │
│  │                                                              [ view ]     │
│  ●  Released       12 Mar 18:11 · quarantined → available                    │
│  │                 by Marthe Ekani (inventory_manager)                       │
│  ○  now                                                                      │
│                                                                              │
│ ── STATUS EVENTS (append-only) ──────────────────────── [ density: dense ]   │
│  when              from → to              actor            reason            │
│  12 Mar 18:11      quarantined → available M. Ekani        TTI non-reactive  │
│  12 Mar 09:41      — → quarantined         E. Fotso        collected         │
└──────────────────────────────────────────────────────────────────────────────┘
```

- **The timeline never collapses.** Provenance is the point of the screen.
- **Group mismatch guard:** if `blood_units.blood_group` differs from the donor's recorded
  group, the row renders the `discarded` treatment with `✗ group mismatch — do not issue` and
  the `[ Reserve ]` action is disabled with an explanation. This is a real transcription error
  class and it is worth a dedicated affordance.
- **`[ Discard ]`** opens `ConfirmDialog` requiring the typed unit code and a reason from
  `policies`-backed list. **`[ Reserve ]`** is disabled unless `status = available`.
- The status-events table is `density="dense"` by default and is never editable.

### 8.6 Hospital blood request — `/hospital/requests/new`

```
┌──────────────────────────────────────────────────────────────────────┐
│ New blood request · St. Mary's Hospital                              │
│                                                                      │
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ ▮ EMERGENCY — need blood now                                     │ │
│ │   Two fields, then call. We start working before you finish.     │ │
│ │                              [ Start emergency request ]         │ │
│ └──────────────────────────────────────────────────────────────────┘ │
│  ↑ The fast path is FIRST on the page and works on a phone. It is    │
│    the one place in BBank where a solid-fill red panel is allowed.   │
│                                                                      │
│ ── Standard request ──────────────────────────────────────────────── │
│ Patient reference *  [ MRN-88213            ]  no patient names      │
│ Blood group *        [ AB ▾ ] [ Negative ▾ ]  ┏━━━┓ AB−             │
│ Component *          ( ) Whole blood  (•) PRBC  ( ) FFP  ( ) PLT     │
│ Units *              [ − ] [  3  ] [ + ]      ← stepper, 44px        │
│ Urgency *            ( ) Routine  (•) Urgent  ( ) Emergency          │
│ Needed by *          [ 2026-09-02 ] [ 08:00 ]                        │
│ Clinical notes       [                                      ]        │
│                                                                      │
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ ⓘ Live availability · AB− PRBC: 1 available, 2 quarantined       │ │
│ │   Your request is for 3. We will substitute compatible groups     │ │
│ │   where clinically acceptable: O−, A−, B−, AB−.                   │ │
│ └──────────────────────────────────────────────────────────────────┘ │
│  ↑ Shown BEFORE submit. A hospital must never discover a shortfall   │
│    only after the request sits in a queue.                           │
│                                                                      │
│                              [ Cancel ]  [ Submit request ]          │
└──────────────────────────────────────────────────────────────────────┘
```

**Emergency fast path.** Blood group + units + a phone number, submitted in one tap. It creates
the `blood_request` with `urgency = emergency`, immediately surfaces it on `/inventory` and
`/admin/blood-requests`, and shows the requester a confirmation screen carrying the blood bank's
direct line and a live status. Per TRD rule 3, emergency requests are **never rate-limited into
failure** — the UI must never show "too many requests" on this path. Everything else about the
request (patient reference, component, notes) is captured afterwards on the request detail page.

### 8.7 Donor portal home — `/donor/[id]`

Mobile-first. Single column at ≤640 px, two columns above.

```
┌───────────────────────────────┐   Above the fold on a 5" phone:
│  Hello, Amina        ⚙        │   the answer to "can I donate, and when?"
│                               │
│  ┌─────────────────────────┐  │   · Ring = progress through the 56-day interval.
│  │      ◜◝                 │  │     Ring + numeral + sentence. Never ring alone.
│  │    ◜  15  ◝   days      │  │   · Under reduced motion the ring renders at its
│  │    ◟ left ◞             │  │     final value with no sweep.
│  │      ◟◞                 │  │   · The sentence states the rule, not just the date.
│  │  You can donate again   │  │
│  │  on 6 April 2026.       │  │
│  │  We ask for 56 days     │  │
│  │  between donations.     │  │
│  └─────────────────────────┘  │
│                               │
│  NEXT APPOINTMENT             │
│  ┌─────────────────────────┐  │   Empty state when none:
│  │ ⬤ Scheduled             │  │   "No appointment booked. You'll be eligible on
│  │ Tue 12 Mar · 09:00      │  │    6 April — we'll remind you."  [ Book ahead ]
│  │ Douala Centre           │  │
│  │ [ Add to calendar ]     │  │   Never a dead end (principle from §9.6).
│  │ [ Reschedule ][ Cancel ]│  │
│  └─────────────────────────┘  │
│                               │
│  YOUR IMPACT                  │   · 7 donations · up to 21 patients supported
│  ┌─────────────────────────┐  │   · Framed as fact, not achievement. No badges,
│  │  7          up to 21    │  │     no streak, no leaderboard (§2.4).
│  │  donations  patients    │  │
│  └─────────────────────────┘  │
│                               │
│  HISTORY                      │   · Reverse-chronological. Each row: date, centre,
│  12 Mar 2026 · Douala · 450mL │     volume, and an outcome badge.
│  09 Nov 2025 · Douala · 450mL │   · A donation whose unit was discarded shows
│  …                            │     "Completed" — the donor is never shown a
│  [ See all 7 → ]              │     unit-level status. (§13.3)
│                               │
│  YOUR DETAILS   [A+] ✎        │
└───────────────────────────────┘
   ▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁
   [Home] [Book] [History] [Me]   ← bottom tab bar, ≤640px, 4 items, 56px tall
```

---

## 9. New component specs

All components live in `bbank/src/components/`. Server components by default; `'use client'`
only where interaction requires it (marked below).

### 9.1 `CTA`

| | |
|---|---|
| **Purpose** | One call-to-action primitive so placement and hierarchy are enforceable in review. |
| **Anatomy** | `<Link>` → `.btn` + variant class + optional leading icon + label + trailing `FaArrowRight`. |
| **Variants** | `primary` (`.btn-primary`) · `secondary` (`.btn-ghost`) · `tertiary` (text link with the `.nav-link` underline) · `danger` (solid `--invalid-line`, clinical only) |
| **Sizes** | `sm` `md` `lg` → `.btn-sm` / `.btn` / `.btn-lg` |
| **States** | rest, hover (`translateY(-1px)`), active (`scale(.98)`), focus-visible (§6.4), loading (spinner replaces the trailing icon, label unchanged, `aria-busy`, pointer-events none), disabled (`aria-disabled` + a reason in a tooltip — never a bare greyed button) |
| **API** | `{ href, label, variant?, size?, icon?, trailingIcon?, loading?, disabled?, disabledReason?, pulse?, className? }` |
| **Content** | Verb + object, 2–4 words. "Become a donor", "Request blood", "Book a slot". Never "Click here", "Learn more", "Submit". |
| **A11y** | Renders `<a>` when `href` is given, `<button>` otherwise — never a div. `pulse` (the existing `.pulse-ring`) is disabled under reduced motion by the global rule and must not be the only signal of importance. |
| **Placement rule** | **Max one `primary` per viewport height.** A section with a primary CTA carries no other primary. Hospitals always get `secondary` next to the donor `primary`. |
| **Appears in** | `/` (hero ×2, final ×1), `/faq` (footer), `/thank-you` (next action), `/eligibility`, `EmptyState` |

### 9.2 `USPBar`

| | |
|---|---|
| **Purpose** | Convert the four *new* capabilities into a scannable proof strip under the hero. |
| **Anatomy** | `<section class="border-y bg-white py-6">` → 4-col grid → each: icon tile (`w-9 h-9 rounded-xl bg-rose-50 text-rose-600`) + bold label + one supporting line. |
| **Variants** | `default` (4-up) · `compact` (icon + label only, used on `/faq` and `/thank-you`) |
| **States** | Static. No hover, no reveal — it is a fact strip, not a card grid. |
| **API** | `{ items: { icon, label, detail }[], variant? }` with a default export of the canonical four. |
| **Canonical content** | **Every unit screened & tested** — "HIV, hepatitis B & C, syphilis and malaria, before any unit is released." · **Traceable vein-to-vein** — "Every bag carries its own record, from the donor to the patient." · **Live stock for hospitals** — "Partner hospitals see real availability, not a phone call." · **Free health check with every donation** — "Blood pressure, pulse, weight and haemoglobin — yours to keep." |
| **A11y** | A `<ul>` of `<li>`; icons `aria-hidden`. At ≤640 px becomes 2×2, then a vertical list at ≤400 px — **never a horizontal scroller**, because scrolled-off proof is not proof. |
| **Appears in** | `/` (below hero), `/faq` (compact, above the accordion), `/thank-you` (compact) |

### 9.3 `Breadcrumbs` + `BreadcrumbList` JSON-LD

| | |
|---|---|
| **Purpose** | Express depth in the consoles, which the one-level sidebar cannot. |
| **Anatomy** | `<nav aria-label="Breadcrumb">` → `<ol>` → `<li>` links separated by a `/` character (`aria-hidden`), last item is plain text with `aria-current="page"`. |
| **Variants** | `default` · `truncated` (>4 levels: first · … · parent · current, with the middle in a dropdown) |
| **States** | Last crumb never a link. Loading crumb for a resolving dynamic segment renders `.skeleton-text` at the label's width, not a layout jump. |
| **API** | `{ items: { label: string, href?: string }[] }`. A `src/lib/breadcrumbs.ts` helper maps a pathname + resolved entity names to items so pages do not hand-build them. |
| **Content** | Sentence case. Use the entity's human name, not its id — "Blood units / BB-2026-004412", not "Units / 4412". The console root is always crumb 1 ("Inventory"). |
| **A11y** | Emits `BreadcrumbList` JSON-LD **only on public routes** — dashboard routes are `Disallow`ed in `robots.ts`, so structured data there is pointless and leaks route shape. |
| **Placement** | Dashboard only, top of the main region, above the `<h1>`. **Not on the landing page** — it is one level deep. |
| **Appears in** | `/inventory/units/[code]`, `/lab/donations/[id]`, `/staff/screening/[id]`, `/admin/*` detail routes, `/hospital/requests/[id]` |

### 9.4 `FAQ` accordion + `FAQPage` JSON-LD

| | |
|---|---|
| **Purpose** | Remove the twelve reasons a first-time donor closes the tab. |
| **Anatomy** | Category heading → `<h3><button aria-expanded aria-controls>` question + chevron → `<div role="region" aria-labelledby>` answer. Native `<details>`/`<summary>` is acceptable and preferred for the no-JS case; if so, style `summary::marker` away and rotate the chevron on `[open]`. |
| **Variants** | `full` (all 12, `/faq`) · `teaser` (3, on `/`, with "See all 12 →") |
| **States** | collapsed · expanded · focus-visible on the trigger · deep-linked open (`/faq#can-i-donate-if-i-have-a-tattoo` opens and scrolls that item) |
| **API** | `{ items: FaqItem[], defaultOpen?: string[], allowMultiple?: boolean }` where `FaqItem = { id, category, q, a }`. Content lives in `src/content/faq.ts` so the JSON-LD and the UI cannot drift. `'use client'` only if not using `<details>`. |
| **A11y** | One `<button>` per question, inside a heading. `aria-expanded` on the trigger, `aria-controls` → the panel id. Chevron rotation respects reduced motion via the global rule. Do not trap focus. |
| **Appears in** | `/faq`, `/` (teaser section), linked from `/eligibility` and `/thank-you` |

**The twelve questions.** Four categories, drafted for a nervous first-time donor.

| # | Category | Question | Answer |
|---|---|---|---|
| 1 | Eligibility | **Can I donate?** | If you are 18–65, weigh at least 50 kg, and feel well today, you very likely can. We check your haemoglobin, blood pressure, pulse and weight at the centre before you give — that check is free and takes about five minutes. Not sure? Take the [two-minute self-check](/eligibility). |
| 2 | Eligibility | **How often can I donate?** | Every 56 days for whole blood — that is about six times a year for men and four for women. The interval lets your body rebuild its red cells. Your portal shows the exact date you become eligible again. |
| 3 | Eligibility | **Can I donate if I have a tattoo or piercing?** | Usually yes, after a waiting period — commonly 3 to 6 months, depending on where it was done. Bring the date with you; the nurse will tell you on the day. |
| 4 | Eligibility | **I'm taking medication. Does that stop me?** | Most common medicines — contraceptives, most blood-pressure tablets, paracetamol — do not. Antibiotics mean waiting until you have finished the course and the infection has cleared. Bring the name of anything you take and we will check it with you. |
| 5 | Eligibility | **Can I donate if I've had malaria?** | Yes, after a waiting period once you have fully recovered and finished treatment — typically 3 months. If you have travelled to a high-transmission area recently, tell us; there may be a short wait. |
| 6 | Process | **What actually happens when I arrive?** | Check-in (2 min) → a private health check and a few questions (10 min) → the donation itself (8–10 min) → rest with a drink and a snack (15 min). Plan for about 45 minutes door to door. |
| 7 | Process | **Does it hurt?** | There is a sharp scratch when the needle goes in, similar to a blood test, and then nothing. Most people describe the rest as a slightly odd but painless pressure. Tell the phlebotomist if anything feels wrong — stopping is always fine. |
| 8 | Process | **How much blood do you take?** | About 450 mL — roughly 8% of your blood volume. Your body replaces the fluid within 24 hours and the red cells over the following weeks. |
| 9 | Safety | **Is it safe? Can I catch anything?** | Every needle, bag and tube is sterile, single-use, and opened in front of you. It is not possible to catch an infection by donating. |
| 10 | Safety | **What do you test my blood for?** | Every donation is tested for HIV, hepatitis B, hepatitis C, syphilis and malaria before any unit can be released. If a test shows something that needs following up, we contact you directly and privately — never by text or email — and help you get proper care. |
| 11 | After | **How will I feel afterwards?** | Most people feel completely normal. Some feel briefly light-headed. Drink extra water, eat properly, avoid heavy lifting and hard exercise for the rest of the day, and keep the dressing on for a few hours. If you feel faint, sit or lie down and tell someone. |
| 12 | After | **Where does my blood go?** | To patients at partner hospitals in your region — accident victims, people in surgery, mothers with complications during childbirth, and people living with sickle cell or cancer. One donation can be split into red cells, plasma and platelets, so it can help up to three people. You can see when your donation was used in your donor portal. |

Answer style: lead with the direct answer, then the reason, then the action. Never open with a
qualifier ("It depends…"). Never use the word "unfortunately".

### 9.5 Clinical primitives

#### `StatusBadge`

| | |
|---|---|
| **Purpose** | The single rendering path for every domain status. |
| **Anatomy** | `<span class="badge badge-status" data-status={status}>` → glyph (`aria-hidden`) + label text. |
| **Variants** | `size`: `sm` (compact/dense tables — label visually hidden, kept in `aria-label`) · `md` (default) · `lg` (page headers). `kind`: `unit` · `unitStatus` alias, `urgency`, `requestStatus`, `appointmentStatus`, `testResult`, `level`. |
| **States** | Static. Non-interactive. If a status is clickable (filter-by-status), the badge is *wrapped* in a button — the badge itself never becomes one. |
| **API** | `{ kind, status, size?, showLabel? }`. There is **no** `color` or `icon` prop — the lookup table in `src/lib/status.ts` owns colour, glyph and label per `(kind, status)`. An unknown value renders the neutral treatment plus the raw string, never nothing. |
| **A11y** | `aria-label` always carries `"{kind}: {label}"`. Glyph is `aria-hidden`. §5.1 hatch/rule patterns apply at all sizes. |
| **Appears in** | Everywhere. Replaces the ad-hoc `badge-green`/`badge-muted` usage in `admin/appointments`, `admin/requests`, `admin/donors`, `donor/[id]`. |

#### `BloodGroupChip`

| | |
|---|---|
| **Purpose** | Make ABO/Rh unmistakable and uniform across 30+ screens. |
| **Anatomy** | Mono, tabular, fixed-width box: group letters + a full-word-safe rhesus sign. |
| **Variants** | `sm` (inline in a dense row) · `md` (table cell) · `lg` (page header, boxed with a heavy border) · `muted` (unknown group → `—` with `badge-muted` and the label "Group not recorded") |
| **States** | Static. `emphasis="mismatch"` renders the `discarded` treatment with a `✗` — used only by the group-mismatch guard (§8.5). |
| **API** | `{ group: 'A'\|'B'\|'AB'\|'O'\|null, rhesus: 'positive'\|'negative'\|null, size?, emphasis? }` |
| **Content** | Always normalises to `A+ A− B+ B− AB+ AB− O+ O−` using **U+2212 MINUS SIGN**, not a hyphen — the existing `admin/donors` page already does this by hand and gets it right; centralise it. Never render `Positive`/`Negative` as words in a table. |
| **A11y** | `aria-label="Blood group A positive"` — a screen reader must never read "A plus" or "A minus" ambiguously. |
| **Appears in** | Every donor row, unit row, request row, and the inventory grid. |

#### `InventoryGrid`

| | |
|---|---|
| **Purpose** | Answer "what do I have?" in one glance. §8.3 is its layout spec. |
| **Anatomy** | Fixed 4×2 grid of `.card-soft` tiles (adopting the currently-dead class), each: group chip, level glyph, available count, target, 8-segment bar, quarantined footnote. |
| **Variants** | `interactive` (inventory manager — tiles link to a filtered `/inventory/units`) · `readonly` (hospital portal, admin overview) · `compact` (admin overview — no bar, no footnote) |
| **States** | loading (8 `.skeleton-tile`) · empty (renders all 8 tiles at zero, never an empty state) · no-target-configured (`of —`, neutral, links to `/admin/policies`) · stale (>60 s: a "stock as of HH:MM" line in the header) |
| **API** | `{ rows: InventorySummaryRow[], component: ComponentType, variant?, onSelect? }` reading the `inventory_summary` projection. |
| **A11y** | Renders as a `<table>` with a visually-hidden caption and real `<th scope="col">`, CSS-gridded for layout. This gives screen readers row/column semantics for free and satisfies "chart needs a table" (§5.6). Each tile's `aria-label`: *"O negative, packed red cells: 2 available of a target of 12. Critical. 3 quarantined."* |
| **Appears in** | `/inventory`, `/hospital/stock`, `/admin` |

#### `ExpiryIndicator`

| | |
|---|---|
| **Purpose** | Make time-to-expiry a glanceable quantity without a countdown timer's false urgency. |
| **Anatomy** | Segmented bar (`▓▓░`) + exact remaining time as text + a glyph when under 24 h. |
| **Variants** | `bar` (list rows) · `inline` (text only, dense tables) · `detail` (full bar + date + days, unit detail page) |
| **States** | `>7d` neutral · `72h–7d` `--level-low` · `<72h` `--level-low` + glyph · `<24h` `--level-critical` + `FaTriangleExclamation` · `expired` `--status-expired`, bar empty, text "Expired 3 days ago" |
| **API** | `{ expiresAt: string, collectedAt?: string, variant? }` — computed server-side from `blood_units.expires_at` so the client clock cannot disagree. |
| **Content** | Under 72 h show hours ("41 h left"); over 72 h show days ("41 days left"); expired shows elapsed. Never a live ticking clock — it is motion in a clinical path (§12) and it invents urgency between renders. |
| **A11y** | Text is the source of truth; the bar is `aria-hidden`. |

#### `DataTable`

| | |
|---|---|
| **Purpose** | One accessible, dense-capable table so 15 list screens do not each reinvent one. |
| **Anatomy** | Optional toolbar (search, filter chips, density toggle, result count) → `.table-modern` with `<caption>`, `<th scope>`, sortable header buttons → pagination `<nav>`. |
| **Variants** | density `comfortable` \| `compact` \| `dense` (§5.3); `sticky` header; `selectable` (checkbox column + bulk action bar); `linkRow` (whole row navigates — with a real `<a>` in the first cell, never a row `onClick`). |
| **States** | loading (`.skeleton-row` ×N at the current density, preserving column widths) · empty (`EmptyState` inside the table body, spanning all columns) · error (inline, with a retry, never a redirect) · filtered-empty (distinct copy naming the filters — §2.3) |
| **API** | `{ columns: Column[], rows, density?, sticky?, caption, emptyState, getRowId, sort?, onSortChange?, page?, total? }` where `Column = { key, header, align?, width?, cell, sortable?, hideBelow? }`. `hideBelow` drives the responsive collapse (§11). |
| **A11y** | Everything in §6.5. Bulk selection announces "3 of 42 selected". |
| **Appears in** | Every list route. |

#### `EmptyState`

| | |
|---|---|
| **Purpose** | Turn a zero-result screen into a next step. |
| **Anatomy** | Icon tile → heading (one sentence, states the fact) → body (one sentence, states why or what to do) → optional `CTA`. |
| **Variants** | `first-run` (nothing exists yet — encouraging) · `filtered` (results exist but not here — names the filters, offers "clear filters") · `all-clear` (a worklist that is legitimately empty — this is *good news* and should read as such) · `error` (something failed — offers retry) |
| **API** | `{ variant, icon, title, body, action?, secondaryAction? }` |
| **Content** | See §13.4 for the per-console strings. Never "No data". Never a shrug illustration. |
| **A11y** | Heading uses the correct level for its position. Inside a table it lives in a `<td colspan>` and the table's `aria-live` announces the count. |

#### `ConfirmDialog`

| | |
|---|---|
| **Purpose** | Make irreversible actions deliberate (principle 2). |
| **Anatomy** | Backdrop (`--z-modal-backdrop`, `rgba(24,24,27,.45)`) → panel (`.card`, `--elev-4`, `max-w-md`) → heading naming the object → consequence paragraph → **friction control** → reason field → `[ Cancel ]` `[ destructive action ]`. |
| **Variants** | `acknowledge` (checkbox: "I understand this cannot be undone") · `type-to-confirm` (must type the unit code or donor name — used for discard, permanent deferral, reactive result, user deletion) · `reason-required` (select from a policy-backed list + free text) — variants compose. |
| **States** | idle (destructive button disabled until friction is satisfied) · submitting (`aria-busy`, both buttons disabled, no spinner-only screen) · error (inline in the dialog, dialog stays open, typed value preserved) |
| **API** | `{ open, onOpenChange, title, object, consequence, friction, reasons?, confirmLabel, onConfirm }`. `'use client'`. Built on the native `<dialog>` element for free focus trapping and top-layer stacking. |
| **A11y** | `role="dialog" aria-modal="true"`, labelled by its heading and described by the consequence paragraph. Focus lands on the **heading**, not the destructive button. `Esc` cancels *unless* the type-to-confirm field has content (then it prompts). Focus returns to the trigger. The destructive button is last in tab order. |
| **Content** | Title names the object and the verb. Consequence is one sentence in plain language stating what becomes permanent and that it is attributed to the actor. The confirm button repeats the verb — **never** "OK" or "Yes". |
| **Appears in** | Discard unit, permanent deferral, record reactive result, cancel appointment, reject blood request, delete user, mark unit recalled. |

### 9.6 `ThankYou` page

| | |
|---|---|
| **Route** | `/thank-you?flow=signup` · `/thank-you?flow=booking&date=…&center=…` |
| **Purpose** | Confirm what just happened, say what happens next, **and carry a next action.** |
| **Anatomy** | `mesh` background → confirmation glyph → headline (`display-serif` on the emphasis word) → the fact (what was created) → **"What happens next"** as a numbered 3-step list → primary next action → secondary actions → `USPBar variant="compact"` → link to `/faq`. |
| **Variants** | `signup` — next action **"Book your first donation"**; steps: check your email → take the 2-minute self-check → book a slot. `booking` — next action **"Add to calendar"** (`.ics` download); steps: we confirm within 24 h → you get an SMS → bring an ID and eat beforehand. Shows the requested date and centre. |
| **States** | Missing/invalid `flow` param falls back to the `signup` variant with generic copy — never a blank page. |
| **Hard rule** | **No dead ends.** Every variant renders at least one primary action and one secondary path (`Back to your portal` / `Read the FAQ`). A thank-you page with only a full stop is a bug. |
| **A11y** | `<h1>` is the confirmation, not the word "Thank you" alone. Focus moves to the `<h1>` on mount so a screen-reader user hears the confirmation immediately. The `.ics` download is a real link with a filename, not a JS blob click. |
| **SEO** | `robots: { index: false }` — a confirmation page has no search value and indexing it strands people mid-flow. |

---

## 10. Metadata, SEO & sharing

### 10.1 Title template

Root layout (`src/app/layout.tsx`) — the only place a template is defined:

```ts
export const metadata: Metadata = {
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000'),
  title: {
    default: 'BBank — Donate blood, save lives',
    template: '%s · BBank',
  },
  description:
    'BBank connects donors with hospitals and tracks every unit of blood from the donor who gave it to the patient who receives it.',
  openGraph: { type: 'website', siteName: 'BBank', locale: 'en' },
  twitter: { card: 'summary_large_image' },
}
```

Today **only the root layout has any metadata** — every route below must export its own.

### 10.2 Title & description table

| Route | `title` (rendered) | Meta description | Index? |
|---|---|---|---|
| `/` | `BBank — Donate blood, save lives` | Register as a donor, book a donation, and track your impact. Every unit is screened, tested and traceable. | ✅ |
| `/faq` | `Donation FAQ · BBank` | Who can donate, what happens on the day, whether it hurts, and what we test for. Twelve honest answers. | ✅ |
| `/eligibility` | `Can I donate? · BBank` | A two-minute self-check covering age, weight, health and travel, before you book. | ✅ |
| `/centers` | `Donation centres · BBank` | Find a BBank donation centre near you, with opening hours and directions. | ✅ |
| `/thank-you` | `You're registered · BBank` / `Booking received · BBank` | — | ❌ `noindex` |
| `/login` | `Log in · BBank` | — | ❌ |
| `/signup` | `Become a donor · BBank` | Register in under a minute. Tell us your blood group and where you are — that is all we need. | ✅ |
| `/hospitals/register` | `Partner with BBank · BBank` | Register your hospital to raise blood requests and see live stock. | ✅ |
| `/privacy` | `Privacy policy · BBank` | How BBank handles your personal and health data. | ✅ |
| `/terms` | `Terms of service · BBank` | The terms that govern use of BBank. | ✅ |
| `/donor/[id]` | `Your portal · BBank` | — | ❌ |
| `/donor/[id]/book` | `Book a donation · BBank` | — | ❌ |
| `/donor/[id]/appointments` | `Your appointments · BBank` | — | ❌ |
| `/donor/[id]/donations` | `Your donations · BBank` | — | ❌ |
| `/donor/[id]/eligibility` | `Your eligibility · BBank` | — | ❌ |
| `/donor/settings` | `Settings · BBank` | — | ❌ |
| `/staff` | `Today's queue · BBank` | — | ❌ |
| `/staff/check-in` | `Check in a donor · BBank` | — | ❌ |
| `/staff/screening/[id]` | `Screening · BBank` (dynamic: `Screening — Amina Njoya · BBank`) | — | ❌ |
| `/staff/collection/[id]` | `Record collection · BBank` | — | ❌ |
| `/staff/donors` | `Donor lookup · BBank` | — | ❌ |
| `/lab` | `Testing worklist · BBank` | — | ❌ |
| `/lab/donations/[id]` | `Test results · BBank` | — | ❌ |
| `/lab/quarantine` | `Quarantine · BBank` | — | ❌ |
| `/inventory` | `Inventory · BBank` | — | ❌ |
| `/inventory/units` | `Blood units · BBank` | — | ❌ |
| `/inventory/units/[code]` | `BB-2026-004412 · BBank` (the unit code **is** the title) | — | ❌ |
| `/inventory/processing` | `Component processing · BBank` | — | ❌ |
| `/inventory/expiry` | `Expiry sweep · BBank` | — | ❌ |
| `/inventory/storage` | `Storage locations · BBank` | — | ❌ |
| `/hospital` | `Your requests · BBank` | — | ❌ |
| `/hospital/requests/new` | `New blood request · BBank` | — | ❌ |
| `/hospital/requests/[id]` | `Request REQ-8831 · BBank` | — | ❌ |
| `/hospital/stock` | `Live stock · BBank` | — | ❌ |
| `/admin` | `Overview · BBank` | — | ❌ |
| `/admin/blood-requests` | `Blood requests · BBank` | — | ❌ |
| `/admin/donation-requests` | `Donation requests · BBank` | — | ❌ |
| `/admin/appointments` | `Appointments · BBank` | — | ❌ |
| `/admin/donors` | `Donor registry · BBank` | — | ❌ |
| `/admin/users` | `Users & roles · BBank` | — | ❌ |
| `/admin/hospitals` | `Partner hospitals · BBank` | — | ❌ |
| `/admin/centers` | `Donation centres · BBank` | — | ❌ |
| `/admin/policies` | `Policies · BBank` | — | ❌ |
| `/admin/reports` | `Reports · BBank` | — | ❌ |
| `/admin/audit` | `Audit log · BBank` | — | ❌ |
| `/admin/settings` | `Settings · BBank` | — | ❌ |
| `not-found` | `Page not found · BBank` | — | ❌ |

Dashboard routes set `robots: { index: false, follow: false }` in their route-group layout so
each page does not repeat it.

### 10.3 OG image — `src/app/opengraph-image.tsx`

Generated with `ImageResponse` (Next.js `next/og`), **1200 × 630**, `runtime = 'edge'`,
`contentType = 'image/png'`, exported `alt`, `size`.

```
┌──────────────────────────────────────────────────────────────────────┐ 1200×630
│  #fafaf9 ground.                                                     │
│  Rose radial wash from the top-right corner:                         │
│    radial-gradient(700px 400px at 88% -10%, rgba(225,29,72,.13), transparent 62%)
│  Amber wash bottom-left: rgba(251,191,36,.09) — mirrors `.mesh`.     │
│                                                                      │
│   ┌──┐                                                          72px │
│   │◆ │  BloodBank            ← 40px Outfit 700, "Bank" in rose-700   │
│   └──┘  rose-700 rounded-2xl mark, white droplet glyph               │
│                                                                      │
│                                                                      │
│   Give blood.                        ← 86px Outfit 700, #18181b      │
│   Give someone tomorrow.             ← 86px Instrument Serif italic, │
│                                        rose-700 → #9f1239 gradient   │
│                                                                      │
│   Every unit screened, tested and traceable.  ← 30px, #52525b        │
│                                                                      │
│   ┏━━┓ ┏━━┓ ┏━━┓ ┏━━┓ ┏━━┓ ┏━━┓ ┏━━┓ ┏━━┓                            │
│   ┃O−┃ ┃O+┃ ┃A−┃ ┃A+┃ ┃B−┃ ┃B+┃ ┃AB−┃┃AB+┃  ← 8 mono chips, 26px,   │
│   ┗━━┛ ┗━━┛ ┗━━┛ ┗━━┛ ┗━━┛ ┗━━┛ ┗━━┛ ┗━━┛     zinc-100 fill,         │
│                                                zinc-600 text        │
│  ── 6px rose-700 rule along the bottom edge ──────────────────────── │
└──────────────────────────────────────────────────────────────────────┘
```

- **No `.grain-overlay`** — noise SVG in `ImageResponse` is expensive and reads as artefact at
  Twitter/WhatsApp thumbnail sizes.
- Fonts must be **fetched as `ArrayBuffer` and passed in `fonts:`** — `ImageResponse` cannot
  read CSS variables or `next/font`. Ship the two `.ttf`/`.woff` subsets under `src/app/_og/`.
- **Per-route overrides** where they add value: `/faq` (headline "Twelve honest answers about
  giving blood", no chip row) and `/signup` (headline "It takes about 45 minutes"). Everything
  else inherits the root image.
- Keep total generated size under ~200 KB.

### 10.4 `robots.ts` and `sitemap.ts`

```ts
// src/app/robots.ts
export default function robots(): MetadataRoute.Robots {
  const base = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000'
  return {
    rules: [{
      userAgent: '*',
      allow: '/',
      disallow: ['/admin', '/donor', '/staff', '/lab', '/inventory', '/hospital', '/api', '/thank-you'],
    }],
    sitemap: `${base}/sitemap.xml`,
  }
}
```

`sitemap.ts` lists only the public set — `/`, `/faq`, `/eligibility`, `/centers`, `/signup`,
`/login`, `/hospitals/register`, `/privacy`, `/terms` — with honest `changeFrequency` and
`priority` (`/` 1.0, `/faq` 0.8, `/signup` 0.8, rest 0.5). No dynamic entries: there is no
public dynamic content, and a sitemap that leaks donor ids would be a data-protection incident.

### 10.5 404 pages

`src/app/not-found.tsx` **already exists** and is well-built (mesh, blobs, `display-serif`,
a single primary CTA home). **Verify** three things and add one file:

1. It renders inside the root layout, so the grain overlay and skip link are present. ✅
2. It exports `metadata` with `title: 'Page not found'` — **currently missing, add it.**
3. Its CTA is `Back home`, which is right for the public zone.

**Add `src/app/(dashboard)/not-found.tsx`** — an in-app 404 that keeps the console shell so a
mistyped `/inventory/units/BB-9999` does not eject the user to the marketing site. Same visual
family, no blobs, `EmptyState variant="error"` inside the content column, and its CTA returns
to the current console root, not to `/`.

---

## 11. Responsive strategy

### 11.1 Breakpoints

Tailwind defaults, used as-is. Do not invent custom breakpoints.

| Token | Min-width | Primary meaning in BBank |
|---|---|---|
| *(base)* | 0 | Phone. Donor portal and hospital emergency path are designed here first. |
| `sm` | 640px | Large phone / small tablet portrait. |
| `md` | 768px | Tablet portrait — the collection-chair form factor. |
| `lg` | 1024px | Tablet landscape / small laptop. Sidebar appears. |
| `xl` | 1280px | The staff and lab console design target. |
| `2xl` | 1536px | Inventory dashboard at full width. |

### 11.2 Per-surface reality — honestly

| Surface | Direction | Design target | Must also work at | Hard constraints |
|---|---|---|---|---|
| **Marketing** (`/`, `/faq`, `/thank-you`) | Mobile-first | 375px | up to 1536px | Donors arrive on phones over poor networks. Hero image lazy below the fold; the `/` hero image keeps `priority`. |
| **Donor portal** | **Mobile-first** | 375px | up to 1280px | Bottom tab bar ≤640px. Everything above the fold on a 5-inch screen answers "can I donate, and when?". No horizontal scroll anywhere, ever. |
| **Staff console** | Desktop-first | 1280×800 | **down to 768px** (tablet at the collection chair) | Screening form must fit one screen at 1280×800 with no scroll, and reflow to a single column at 768px without losing the eligibility banner above the fold. Targets stay ≥44px at every width. |
| **Lab console** | Desktop-first | 1440px | down to 1024px | Dense worklists. Below 1024px, degrade to the card list (§11.3). Not a phone surface — say so and do not pretend otherwise. |
| **Inventory console** | Desktop-first | 1440–1536px | down to 1024px | The 4×2 grid becomes 2×4 at `md` and 2×4 stacked at `sm`; **it never becomes 1×8**, because the group pairs (`O−`/`O+`) must stay adjacent to be comparable. |
| **Hospital portal** | **Mobile-capable, desktop-designed** | 1280px | **down to 375px** | Emergencies happen away from a desk. `/hospital/requests/new` and its emergency fast path must be fully usable one-handed on a 375px screen. The stepper, the group select, and the submit button are all ≥44px. |
| **Admin** | Desktop-first | 1280px | down to 1024px | Configuration screens are not urgent; a phone is not a supported context. |

### 11.3 How dense clinical tables degrade

Three strategies, chosen per table, driven by the `hideBelow` column property on `DataTable`.

1. **Priority columns + horizontal scroll (default, `lg` → `md`).** Columns declare
   `hideBelow: 'md' | 'lg' | 'xl'`. Below their breakpoint they drop out of the table and into
   the expanded row detail. The identifier column is `position: sticky; left: 0` with a
   right-edge shadow so the unit code never scrolls away. The scroll container gets
   `tabindex="0"`, `role="region"`, and an `aria-label` so it is keyboard-reachable (SC 2.1.1).

2. **Card list (`< md`, for tables that cannot be usefully truncated).** Each row becomes a
   `.card` with the identifier as its heading, the `StatusBadge` top-right, and 3–5 label/value
   pairs. Row actions become a footer button row. This is the fallback for
   `/inventory/units` and `/lab` on a tablet in portrait.

3. **Summary + drill-in (`< sm`, for tables that are genuinely desktop-only).** Render a
   `EmptyState variant="filtered"`-shaped panel that states the count and the top three rows,
   with "Open on a larger screen for the full register." **This is honest, not a cop-out** —
   a 2000-row unit register on a 375px screen is not a usable surface, and pretending otherwise
   produces something worse than an explicit boundary. It applies to exactly two routes:
   `/inventory/units` and `/admin/audit`.

Never used: pinch-zoom tables, transposed row/column tables, or a `<table>` with
`display: block` and no semantics — the last of these silently destroys screen-reader
navigation and is the most common bad answer to this problem.

---

## 12. Motion

### 12.1 The existing system, kept

`--ease-out: cubic-bezier(.22,1,.36,1)` is the house curve for entrances. Add one for exits and
one for the small state changes that dominate the consoles:

```css
:root {
  --ease-out: cubic-bezier(0.22, 1, 0.36, 1);      /* entrance — existing, unchanged */
  --ease-in:  cubic-bezier(0.4, 0, 1, 1);           /* exit */
  --ease-std: cubic-bezier(0.4, 0, 0.2, 1);         /* state change, hover, colour */

  --dur-instant: 80ms;    /* colour/border on hover, checkbox, toggle */
  --dur-fast:    150ms;   /* dropdown, tooltip, row highlight */
  --dur-base:    250ms;   /* modal, drawer, toast */
  --dur-slow:    500ms;   /* page-level entrance */
  --dur-reveal:  800ms;   /* .reveal / .animate-fade-up — existing, marketing only */
}
```

### 12.2 What animates, and what must not

| Element | Duration | Curve | Zone |
|---|---|---|---|
| `.reveal`, `.animate-fade-up`, `.animate-scale-in` | 800ms / 700ms | `--ease-out` | **Marketing only** |
| `.animate-float`, `.pulse-ring`, `.marquee` | 7s / 2.4s / 36s loop | — | **Marketing only** |
| Modal / `ConfirmDialog` enter | 250ms fade + `scale(.98 → 1)` | `--ease-out` | All |
| Modal exit | 150ms fade | `--ease-in` | All |
| Toast enter/exit | 500ms (existing) | `--ease-out` | All |
| Dropdown, tooltip, popover | 150ms | `--ease-std` | All |
| Table row hover, badge, chip | 80ms colour only | `--ease-std` | All |
| Accordion (`FAQ`) panel | 250ms height | `--ease-std` | Marketing |
| Sidebar drawer (mobile) | 250ms transform | `--ease-out` | All |
| Density change on a table | **none** | — | Clinical |
| Number counting up | **never** | — | — |
| `ExpiryIndicator` | **never ticks** | — | — |

**Nothing in a clinical alert path is delayed by an animation.** Specifically:

- The eligibility verdict on the screening form, the group-mismatch guard, the unfillable-demand
  panel, an emergency `blood_request` appearing, an out-of-range validation message, and a
  `ConfirmDialog`'s consequence text **all render at full opacity on first paint**. No fade-in,
  no stagger, no `Reveal`.
- `.table-modern tbody tr { animation: fade-in .5s ease both }` currently animates **every row
  of every table**. On a 200-row unit list this is 200 concurrent animations and a half-second
  delay before the data is readable. §5.3 disables it at `compact` and `dense`; it should also
  be disabled on any table over 25 rows regardless of density.
- `.live-dot` blinks forever. Keep it on the marketing hero and the "Overview" eyebrow, but
  **never** attach it to a clinical status — a blinking element next to a patient-safety number
  is an attention tax with no information content.

### 12.3 `prefers-reduced-motion`

The existing block at `globals.css:384–391` is correct and stays. Two additions (full CSS in §6.8):
disable `scroll-behavior: smooth`, and flatten `.skeleton` to a solid tone rather than a frozen
gradient. Additionally:

- `ConfirmDialog` and drawers must **cross-fade only** (no transform) under reduced motion, not
  simply run their transform at 0.001ms — a `scale` that snaps is still a snap.
- The donor eligibility ring renders at its final value with no sweep.

### 12.4 `Reveal` hardening

`Reveal.tsx` should read `prefers-reduced-motion` before creating the observer and add
`revealed` synchronously if it matches, and the `@media (scripting: none)` fallback in §6.8
guarantees the content is visible without JS. `Reveal` must not be imported by any file under
`(dashboard)`, `staff`, `lab`, `inventory`, or `hospital` — worth a lint rule.

---

## 13. Content & microcopy guidelines

### 13.1 Rules

1. **Sentence case everywhere.** Buttons, headings, table headers, labels. No Title Case.
2. **Dates:** `Tue 12 Mar 2026` in prose and cards; `2026-03-12` in dense tables and exports.
   Times are 24-hour in clinical surfaces (`09:00`, `18:05`), 12-hour on donor surfaces
   (`9:00 AM`). Always with a timezone-stable server-rendered value.
3. **Units are always printed**, never assumed: `12.4 g/dL`, `118/76 mmHg`, `450 mL`, `62.0 kg`.
4. **Identifiers are mono and never truncated in the middle of a scan target.** In dense tables
   truncate the *prefix* (`BB-…4412`), never the discriminating suffix.
5. **Never blame the user.** "That email is already registered" — not "You entered an invalid email".
6. **Say what did not happen.** Failed writes state that nothing was recorded.
7. **Every error offers a route out** — retry, a different action, or a phone number.

### 13.2 Deferral messaging

| Context | String |
|---|---|
| Staff, selecting temporary | Temporary deferral. The donor cannot donate until the date below. They will be told the reason and the date. |
| Staff, selecting permanent | **Permanent deferral. This ends this person's eligibility to donate.** They will be told, in plain language, that they cannot donate — and offered a conversation with a clinician. Type the donor's name to confirm. |
| Donor portal, temporary | You can't donate right now. Your haemoglobin was 12.1 g/dL on 12 March, and we ask for at least 12.5 g/dL. This is common and usually temporary — iron-rich food helps. **You'll be eligible again on 14 October 2026**, and we'll remind you. |
| Donor portal, temporary (interval) | You donated on 12 March. We ask for 56 days between whole-blood donations so your body can rebuild its red cells. **You'll be eligible again on 6 April 2026.** |
| Donor portal, permanent | Based on your health check, you're not able to donate blood. We know that's disappointing — thank you for coming in. **This is about donation safety, not about your health being at risk.** Please call us on +237 6 53 53 29 29 if you'd like to talk it through with a clinician. |
| Donor email/SMS, permanent | *Never sent as a full explanation.* SMS: "Thank you for visiting BBank today. Please call us on +237 6 53 53 29 29 — we'd like to talk through your health check with you." |

The permanent-deferral copy never uses "rejected", "unfit", "disqualified", or "failed", and
never states a suspected diagnosis.

### 13.3 Reactive test result — the most sensitive screen in the system

A reactive TTI result is a **potential diagnosis of a serious infection, delivered to someone
who came in to do a good deed**. The UI's job is to route the person to a qualified human, and
otherwise to say as little as possible.

**What the lab tech sees** (`/lab/donations/[id]`, on selecting `reactive`):

> **Reactive result — this triggers a notification protocol.**
> Recording a reactive result will: discard every unit from donation `#D-4412`, place a permanent
> deferral on the donor, and **create a counselling task for the clinical team**.
> The donor will **not** be told the test name or the result by the system. Only a clinician
> may communicate this, in person or by phone.
> Type the donation reference to confirm.

**What the donor sees** — in the portal and in the SMS/email, **identically**:

> **Please contact us.**
> There's something about your recent donation that we need to discuss with you in person.
> Please call **+237 6 53 53 29 29** or come to the Douala Centre — we'll arrange a private
> appointment at a time that suits you.
> There is nothing you need to do before you call.

**What the UI deliberately does not say — enumerated, because omission here is a requirement:**

- The name of any test (HIV, HBsAg, HCV, syphilis, malaria).
- The words *positive*, *reactive*, *result*, *infection*, *disease*, *diagnosis*, *risk*.
- Anything implying the donation was refused, wasted, or "failed".
- Anything about the donor's units, their status, or that they were discarded.
- Any urgency framing ("urgently", "immediately") — urgency reads as catastrophe to the recipient.
- Any of it in an **SMS or email preview line** — the notification's subject and first 40
  characters must be safe to read on a locked screen in front of other people:
  subject `A message from BBank`, preview `Please give us a call when you can.`

Implementation consequences: `notifications` rows for this template carry no clinical payload;
the donor-facing `donations` history shows `Completed`, never the unit outcome; and the
counselling task is the only path by which the result reaches the person.

### 13.4 Empty states per console

| Console | Variant | Title | Body | Action |
|---|---|---|---|---|
| `/staff` (no appointments today) | all-clear | No donors scheduled today | Douala Centre has no appointments for 1 September. Walk-ins are still welcome. | Register a walk-in |
| `/staff/check-in` (no search match) | filtered | No donor matches "njoy" | Try a phone number or national ID, or register them as a new donor. | Register a walk-in |
| `/lab` (worklist empty) | all-clear | Nothing waiting to be tested | Every collected donation has a complete TTI panel. Nice. | View quarantine |
| `/lab/quarantine` (empty) | all-clear | No units in quarantine | Everything collected has been tested and released. | Go to inventory |
| `/inventory/units` (filtered to nothing) | filtered | No units match O− · PRBC · available | 3 O− PRBC units are quarantined pending TTI, and 1 is reserved. | Clear filters |
| `/inventory/expiry` (nothing expiring) | all-clear | Nothing expires in the next 72 hours | The next expiry is BB-2026-004431 on 18 September. | View all units |
| `/hospital` (no requests) | first-run | You haven't raised a request yet | When a patient needs blood, raise a request here and we'll match it against live stock. | New blood request |
| `/hospital/stock` (group unavailable) | filtered | No AB− red cells on hand | 2 units are quarantined pending testing. Compatible alternatives: O−, A−, B−. | Raise a request anyway |
| `/donor/[id]/donations` (never donated) | first-run | You haven't donated yet | Your first donation will appear here, along with when it was used. | Book a donation |
| `/donor/[id]/appointments` (none) | first-run | No appointments booked | You'll be eligible on 6 April 2026. You can book ahead from today. | Book ahead |
| `/admin/blood-requests` (queue empty) | all-clear | No requests waiting | Every hospital request has been approved, fulfilled or closed. | View fulfilled |
| Any table, error | error | We couldn't load this | The request to the API didn't complete. Nothing was changed. | Try again |

### 13.5 Error messages

| Situation | String |
|---|---|
| Login failed | That email and password don't match an account. Check both, or [reset your password]. |
| Signup, duplicate email | An account already exists for this email. [Log in] instead, or [reset your password]. |
| Booking, not yet eligible | You can't book for that date — you'll be eligible from 6 April 2026. Pick a later date. |
| Screening, value out of possible range | Haemoglobin must be between 5.0 and 25.0 g/dL. You entered 0.4. |
| Screening, value out of policy range | 12.1 g/dL is below the 12.5 g/dL threshold for female donors. You can still record it — choose an outcome below. |
| Allocation, unit already taken | **BB-2026-004412 was allocated to request #REQ-8829 while you were working.** Nothing was changed. Choose a different unit. |
| Allocation, incompatible | O− red cells can go to any recipient, but **A+ units cannot go to an O− recipient.** Choose a compatible unit: O−. |
| Discard, missing reason | Choose a reason. Every discard is recorded against your account. |
| API unreachable | We can't reach the BBank service right now. Nothing you entered was lost — try again in a moment. |
| Rate-limited login | Too many attempts. Try again in 5 minutes, or [reset your password]. |
| Session expiring | Your session ends in 2 minutes. [Stay signed in] — your screening draft is saved either way. |

### 13.6 Destructive-action confirmation copy

| Action | Title | Consequence | Friction | Confirm button |
|---|---|---|---|---|
| Discard a unit | Discard unit BB-2026-004412? | This unit will be permanently removed from stock and cannot be restored. The discard is recorded against your account with the reason you give. | Type `BB-2026-004412` + reason | Discard unit |
| Permanent deferral | Permanently defer Amina Njoya? | Amina will no longer be able to donate blood. She will be told, and offered a conversation with a clinician. This cannot be undone from this screen. | Type `Amina Njoya` + reason | Defer permanently |
| Record reactive result | Record a reactive result for donation #D-4412? | All units from this donation will be discarded, the donor will be permanently deferred, and a counselling task will be created. The donor will not be told the result by the system. | Type `D-4412` | Record result |
| Recall a unit | Recall unit BB-2026-004412? | This flags a look-back investigation. If the unit has been issued, the receiving hospital is notified immediately. | Acknowledge + reason | Recall unit |
| Reject a blood request | Reject request REQ-8831 from St. Mary's? | St. Mary's will be notified with your reason. Any units already reserved for this request are released back to stock. | Reason required | Reject request |
| Cancel an appointment (staff) | Cancel Amina Njoya's appointment on 12 March? | The slot is released and Amina is notified by SMS. She can book again straight away. | Acknowledge | Cancel appointment |
| Delete a user | Remove Etienne Fotso's access? | Etienne can no longer sign in. His name stays on every screening and donation he recorded — those records are never removed. | Type `Etienne Fotso` | Remove access |

Note the last one: **deletion of a user must never imply deletion of their clinical records.**
Say so in the dialog.

---

## 14. Implementation notes

### 14.1 Component library decision — **stay hand-rolled**

**Recommendation: do not adopt shadcn/ui. Extend `globals.css` and hand-roll the ~8 new
primitives, adopting Radix UI *primitives only* for the two components where accessibility is
genuinely hard.**

| Factor | Assessment |
|---|---|
| Existing system maturity | 391 lines of coherent, token-driven CSS with 15 classes in real use across 20 files. This is not a blank slate — it is a working design system with a distinct identity. |
| Cost of adopting shadcn | shadcn ships its own token layer (`--background`, `--primary`, `--ring`, `--radius`, `hsl()` triplets) that **collides** with `--bg` / `--accent` / `--radius`. Every one of the 20 existing files would need reconciling, or the app would carry two parallel token systems. |
| Visual outcome | shadcn's default look is a well-known aesthetic. The current site is warm, serif-accented, grain-textured, and *not* generic. Adopting shadcn means either fighting its defaults on every component, or losing the identity. |
| What shadcn would actually buy | Accessible `Dialog`, `Select`, `Combobox`, `Popover`, `Tabs` — i.e. Radix. That value is available **directly from `@radix-ui/react-*`** without the token layer or the CLI-generated component sprawl. |
| Risk | Low either way, but the hand-rolled path has no migration step and no dependency on a scaffolding CLI's conventions. |

**Concrete plan:**

- **Hand-roll, styled with existing + new classes:** `CTA`, `USPBar`, `Breadcrumbs`,
  `StatusBadge`, `BloodGroupChip`, `InventoryGrid`, `ExpiryIndicator`, `EmptyState`, `DataTable`,
  `ThankYou`. None of these need a library; all of them need to look like BBank.
- **Native platform elements** where they are sufficient: `<details>/<summary>` for `FAQ`,
  `<dialog>` for `ConfirmDialog` (free focus trap, top-layer, `Esc`), `<input type="date">` for
  dates. Test `<dialog>` behaviour on the target browsers before committing.
- **Add `@radix-ui/react-select` and `@radix-ui/react-popover` only if** the native `<select>`
  proves insufficient for the searchable centre/hospital pickers. Do not add them speculatively.
- **Keep `react-icons/fa6`** — already a dependency, already the icon language of the whole app.
  Centralise the status glyph mapping in `src/lib/status.ts` so an icon is never chosen at a
  call site.
- Add `@tailwindcss/forms`? **No.** `.field` already does the job and `@tailwindcss/forms`
  would reset it.

### 14.2 Files that change

| File | Change |
|---|---|
| `bbank/src/app/globals.css` | Add §5.1–§5.8 blocks. Repair `--text-3`, `--border-strong`, `.btn-primary` fill, `::selection`. Add `@media (prefers-reduced-motion)` scroll-behavior and skeleton rules. Add the `@media (scripting: none)` reveal fallback. Delete or adopt `.card-soft`. |
| `bbank/src/app/layout.tsx` | Add `metadataBase`, `title.template`, `twitter`. Bump the skip-link background to `--accent-fill`. |
| `bbank/src/app/(root)/page.tsx` | Insert `<USPBar />` under the hero. Replace the three ad-hoc `btn-primary` uses with `<CTA />` and enforce one primary per viewport. Add the FAQ teaser section. **Rewrite the "priority matching" claim** (§8.2). Add `<CTA variant="secondary">Request blood</CTA>` to the hero. |
| `bbank/src/app/(root)/layout.tsx` | Add `/faq` and `/eligibility` to the footer "Resources" column. |
| `bbank/src/components/Navbar.tsx` | Add `FAQ` to `links`. Raise the mobile toggle to `p-3`. |
| `bbank/src/components/SidebarNav.tsx` | Replace the role ternary with the `NAV` manifest from `src/lib/nav.ts`. Add the console switcher and centre context slots. Raise link `min-height` to 44px. Add the console skip target. |
| `bbank/src/components/Reveal.tsx` | Early-return on `prefers-reduced-motion`. |
| `bbank/src/components/ToastAlert.tsx` | `role="alert"` for errors; no auto-dismiss on clinical errors; dismiss button to `p-2.5`. |
| `bbank/src/app/not-found.tsx` | Add `export const metadata = { title: 'Page not found' }`. |
| `bbank/src/app/(dashboard)/admin/*` | Replace `badge-green`/`badge-muted`/`badge-accent` with `StatusBadge` and `BloodGroupChip`. Wrap tables in `DataTable`. Add `metadata` exports. |
| `bbank/src/app/loading.tsx` | Keep; add per-route `loading.tsx` matching each console's layout. |

### 14.3 New files

```
bbank/src/app/robots.ts
bbank/src/app/sitemap.ts
bbank/src/app/opengraph-image.tsx
bbank/src/app/_og/                       (font binaries for ImageResponse)
bbank/src/app/(root)/faq/page.tsx
bbank/src/app/(root)/thank-you/page.tsx
bbank/src/app/(root)/eligibility/page.tsx
bbank/src/app/(root)/centers/page.tsx
bbank/src/app/(dashboard)/not-found.tsx
bbank/src/components/CTA.tsx
bbank/src/components/USPBar.tsx
bbank/src/components/Breadcrumbs.tsx
bbank/src/components/FAQ.tsx
bbank/src/components/StatusBadge.tsx
bbank/src/components/BloodGroupChip.tsx
bbank/src/components/InventoryGrid.tsx
bbank/src/components/ExpiryIndicator.tsx
bbank/src/components/DataTable.tsx
bbank/src/components/EmptyState.tsx
bbank/src/components/ConfirmDialog.tsx
bbank/src/components/ConsoleShell.tsx
bbank/src/lib/nav.ts                     (role → sidebar manifest)
bbank/src/lib/status.ts                  (status → colour + glyph + label lookup)
bbank/src/lib/bloodGroup.ts              (normalisation, compatibility matrix)
bbank/src/lib/breadcrumbs.ts
bbank/src/content/faq.ts                 (the 12 Q&As — single source for UI + JSON-LD)
```

### 14.4 Build order (feeds the implementation plan)

1. **Token layer + contrast repairs** (§5.2, §5.1, §5.4, §6.1, §6.4, §6.8). No new pages —
   this is a `globals.css` change plus a `.btn-primary` fill swap. Everything else depends on it.
2. **`src/lib/status.ts` + `StatusBadge` + `BloodGroupChip`.** Retrofit the three existing admin
   tables immediately so the primitives are proven before the new consoles are built.
3. **`DataTable` + `EmptyState` + density scale** (§5.3). Retrofit the same three tables.
4. **Site completeness set** — `CTA`, `USPBar`, `/faq`, `/thank-you`, `robots.ts`, `sitemap.ts`,
   `opengraph-image.tsx`, per-route `metadata`, `(dashboard)/not-found.tsx`. Independent of the
   clinical work and shippable on its own.
5. **`ConsoleShell` + `nav.ts` + `Breadcrumbs`.** Unblocks all four new consoles.
6. **`ConfirmDialog`.** Blocks every destructive clinical action; build before the first one ships.
7. **Clinical screens in domain order** — staff (check-in → screening → collection), then lab,
   then inventory, then hospital. `InventoryGrid` and `ExpiryIndicator` land with the inventory
   dashboard.
8. **Accessibility verification pass** — keyboard-only walkthrough of the full staff workflow,
   axe/Lighthouse on every route, greyscale screenshot review of every status surface.

### 14.5 Definition of done for any new screen

- [ ] Exports `metadata` with a unique title from the §10.2 table.
- [ ] Has a `loading.tsx` skeleton that matches its own layout.
- [ ] Has an `EmptyState` for every zero-result path, with copy from §13.4.
- [ ] Every status renders through `StatusBadge`; every blood group through `BloodGroupChip`.
- [ ] Every destructive action goes through `ConfirmDialog` with copy from §13.6.
- [ ] Completable by keyboard alone; focus order verified; visible focus on every control.
- [ ] Readable in greyscale — no meaning lost.
- [ ] Correct zone register (§4): no `Reveal`, serif, or blob in a clinical console.
- [ ] Targets ≥44px on clinical surfaces; ≥24px everywhere.
- [ ] `prefers-reduced-motion` respected; nothing in an alert path is delayed by animation.
- [ ] `PROJECT_STATUS.md` updated in the same change.

---

*End of UI/UX Brief · Draft v1 · 2026-09-01*
