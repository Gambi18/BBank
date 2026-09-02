-- WI-26: make `donor_eligibility` agree with `domain.EvaluateEligibility`.
--
-- The view and the Go domain both answer "may this donor donate today". Two
-- implementations of one clinical rule is the shape of every defect fixed in the
-- 2026-09-02 sweep — each path guessed, and they disagreed. The split is now
-- explicit: **the database owns the facts, the domain owns the verdict**, and
-- the view remains only for list-level display. This migration removes the two
-- ways it could still contradict the domain.
--
-- 1. THE DEFERRAL BOUNDARY WAS OFF BY ONE, INSIDE THE VIEW ITSELF.
--
--    `active_deferrals` keeps a row only while `ends_on > CURRENT_DATE`, so a
--    deferral ending today is already over and the donor is eligible today. But
--    `next_eligible_on` reported `deferred_until + 1`. The view therefore told a
--    donor to come back the day AFTER the day it would itself have let them
--    book. `ends_on` is exclusive — the same half-open reading the schema uses
--    for every `daterange`, and the reading `deferrals_window
--    CHECK (ends_on > starts_on)` implies — so the `+ 1` is dropped.
--
-- 2. THE COALESCE FALLBACKS ANSWERED WITH NUMBERS NOBODY CONFIGURED.
--
--    `cfg` read each threshold as `COALESCE((SELECT ... FROM active_policies), 56)`.
--    Delete the `donation_interval_days.whole_blood` row and the view carried on
--    applying 56 days from a literal in a migration — while the API, which
--    refuses to decide without a policy (`domain.ErrPolicyMissing`), returned an
--    error for the same donor. One component silently inventing a clinical
--    constant that the other refuses to invent is the divergence this whole work
--    item exists to remove, and it also defeated `FR-20`: a threshold cannot be
--    "no eligibility threshold is hardcoded in application logic" while a
--    fallback for it sits in the schema.
--
--    Without a policy the thresholds are NULL, every comparison is NULL,
--    `is_eligible_today` is NULL rather than true, and `reason` says
--    `policy_unavailable`. "We cannot currently decide" is the honest answer, and
--    it is the same answer the API gives.
--
--    This was not caught earlier because nothing read `policies` until WI-25 —
--    and because `TRUNCATE users ... CASCADE` had been silently emptying the
--    table on every integration test, so the fallbacks were the only thing
--    keeping the view answering at all.

DROP VIEW IF EXISTS donor_eligibility;

CREATE VIEW donor_eligibility AS
WITH cfg AS (
    SELECT
        (SELECT (value ->> 'days')::int FROM active_policies
          WHERE key = 'donation_interval_days.whole_blood' AND region = '*') AS wb_interval_days,
        (SELECT (value ->> 'min')::int FROM active_policies
          WHERE key = 'donor_age_years' AND region = '*')                    AS min_age,
        (SELECT (value ->> 'max')::int FROM active_policies
          WHERE key = 'donor_age_years' AND region = '*')                    AS max_age
),
last_completed AS (
    SELECT d.donor_id,
           max(d.collected_at)                                                AS last_donated_at,
           max(d.collected_at) FILTER (WHERE d.procedure = 'whole_blood')     AS last_whole_blood_at,
           count(*) FILTER (WHERE d.collected_at > now() - INTERVAL '1 year') AS donations_last_12m
    FROM donations d
    JOIN appointments a ON a.id = d.appointment_id AND a.status = 'completed'
    GROUP BY d.donor_id
),
active_deferrals AS (
    SELECT df.donor_id,
           bool_or(df.type = 'permanent') AS permanently_deferred,
           max(df.ends_on)                AS deferred_until
    FROM deferrals df
    WHERE df.lifted_at IS NULL
      AND (df.ends_on IS NULL OR df.ends_on > CURRENT_DATE)
    GROUP BY df.donor_id
)
SELECT
    dp.user_id AS donor_id,
    dp.full_name,
    dp.blood_group,
    dp.rhesus,
    EXTRACT(YEAR FROM age(CURRENT_DATE, dp.date_of_birth))::int AS age_years,
    lc.last_donated_at,
    lc.donations_last_12m,
    ad.permanently_deferred,
    ad.deferred_until,
    -- `deferred_until` itself, not `+ 1`: `ends_on` is the first ELIGIBLE day.
    -- NULL when a threshold is unconfigured, because a date computed from a
    -- missing interval would be a date nobody chose.
    CASE WHEN cfg.wb_interval_days IS NULL THEN NULL::date ELSE GREATEST(
        COALESCE((lc.last_whole_blood_at + make_interval(days => cfg.wb_interval_days))::date, CURRENT_DATE),
        COALESCE(ad.deferred_until, CURRENT_DATE),
        CURRENT_DATE
    ) END AS next_eligible_on,
    -- NULL, not FALSE, when policy is missing: "unknown" and "ineligible" are
    -- different answers, and only one of them is honest here.
    CASE WHEN cfg.min_age IS NULL OR cfg.max_age IS NULL OR cfg.wb_interval_days IS NULL THEN NULL::boolean ELSE (
            COALESCE(ad.permanently_deferred, FALSE) = FALSE
        AND EXTRACT(YEAR FROM age(CURRENT_DATE, dp.date_of_birth)) BETWEEN cfg.min_age AND cfg.max_age
        AND COALESCE(ad.deferred_until, CURRENT_DATE) <= CURRENT_DATE
        AND COALESCE((lc.last_whole_blood_at + make_interval(days => cfg.wb_interval_days))::date, CURRENT_DATE) <= CURRENT_DATE
        AND u.status = 'active'
    ) END AS is_eligible_today,
    CASE
        WHEN cfg.min_age IS NULL OR cfg.max_age IS NULL OR cfg.wb_interval_days IS NULL
                                                      THEN 'policy_unavailable'
        WHEN u.status <> 'active'                     THEN 'account_not_active'
        WHEN COALESCE(ad.permanently_deferred, FALSE) THEN 'permanently_deferred'
        WHEN COALESCE(ad.deferred_until, CURRENT_DATE) > CURRENT_DATE THEN 'temporarily_deferred'
        WHEN EXTRACT(YEAR FROM age(CURRENT_DATE, dp.date_of_birth)) < cfg.min_age THEN 'under_age'
        WHEN EXTRACT(YEAR FROM age(CURRENT_DATE, dp.date_of_birth)) > cfg.max_age THEN 'over_age'
        WHEN COALESCE((lc.last_whole_blood_at + make_interval(days => cfg.wb_interval_days))::date, CURRENT_DATE)
             > CURRENT_DATE THEN 'interval_not_elapsed'
        ELSE 'eligible'
    END AS reason
FROM donor_profiles dp
JOIN users u                  ON u.id       = dp.user_id
CROSS JOIN cfg
LEFT JOIN last_completed lc   ON lc.donor_id = dp.user_id
LEFT JOIN active_deferrals ad ON ad.donor_id = dp.user_id;
