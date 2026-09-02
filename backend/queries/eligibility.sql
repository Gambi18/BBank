-- The FACTS an eligibility decision needs. Not the decision itself.
--
-- The `donor_eligibility` view (schema §8.3) also answers "is this donor
-- eligible", by reimplementing the age band, the interval and the deferral rules
-- in SQL with COALESCEd fallbacks. Two implementations of one clinical rule is
-- the shape of the last four defects in this repository — two paths that each
-- guessed, and disagreed. So the split is explicit: **the database owns the
-- facts, `domain.EvaluateEligibility` owns the verdict.** The view stays for
-- list-level display, and `TestTheViewAndTheDomainAgree` fails if they diverge.
--
-- `last_donation_at` is the last donation OF THE SAME PROCEDURE, because the
-- interval policy is per procedure: a platelet donor's whole-blood donation last
-- month must not push their next apheresis out by 56 days.
--
-- `donations_last_12m` counts the same procedure too, for the same reason —
-- `donations_per_year_max` states a separate figure for `apheresis_platelet`, so
-- counting every procedure against the whole-blood cap would refuse a platelet
-- donor for donations that cap never covered. (The view counts all procedures;
-- it is not applying a cap, only reporting a total.)
--
-- name: GetDonorEligibilityFacts :one
WITH completed AS (
    SELECT d.procedure, d.collected_at
    FROM donations d
    JOIN appointments a ON a.id = d.appointment_id AND a.status = 'completed'
    WHERE d.donor_id = sqlc.arg('donor_id')
),
of_procedure AS (
    SELECT max(collected_at)::timestamptz AS last_at,
           count(*) FILTER (WHERE collected_at > now() - INTERVAL '1 year') AS in_last_12m
    FROM completed
    WHERE procedure = sqlc.arg('procedure')::donation_procedure
),
ever AS (
    SELECT count(*) AS total FROM completed
),
deferral AS (
    SELECT bool_or(df.type = 'permanent') AS permanently_deferred,
           max(df.ends_on)::date          AS deferred_until
    FROM deferrals df
    WHERE df.donor_id = sqlc.arg('donor_id')
      AND df.lifted_at IS NULL
      AND (df.ends_on IS NULL OR df.ends_on > CURRENT_DATE)
)
SELECT
    dp.user_id                                  AS donor_id,
    dp.date_of_birth,
    dp.gender,
    (u.status = 'active')                       AS account_active,
    -- A donor with no completed donation of ANY procedure is a first-time
    -- donor, which is what the `first_time_max` age cap means.
    (ev.total = 0)                              AS first_time,
    op.last_at                                  AS last_donation_at,
    COALESCE(op.in_last_12m, 0)::int            AS donations_last_12m,
    COALESCE(df.permanently_deferred, FALSE)    AS permanently_deferred,
    df.deferred_until
FROM donor_profiles dp
JOIN users u ON u.id = dp.user_id
CROSS JOIN of_procedure op
CROSS JOIN ever ev
CROSS JOIN deferral df
WHERE dp.user_id = sqlc.arg('donor_id');

-- The view's own verdict, for the agreement test and for list-level display.
--
-- name: GetDonorEligibilityView :one
SELECT donor_id, age_years, last_donated_at, donations_last_12m,
       permanently_deferred, deferred_until, next_eligible_on, is_eligible_today, reason
FROM donor_eligibility WHERE donor_id = sqlc.arg('donor_id');
