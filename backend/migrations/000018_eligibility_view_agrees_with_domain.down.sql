-- Restores the `donor_eligibility` view exactly as migration 000010 created it,
-- COALESCE fallbacks and `deferred_until + 1` included.
--
-- Reverting reinstates both defects. That is what a down migration is for: it
-- returns the schema to the state the previous version's code expects, and the
-- previous version's code expects a view that answers even with no policy rows.

DROP VIEW IF EXISTS donor_eligibility;

CREATE VIEW donor_eligibility AS
WITH cfg AS (
    SELECT
        COALESCE((SELECT (value ->> 'days')::int FROM active_policies
                   WHERE key = 'donation_interval_days.whole_blood' AND region = '*'), 56)      AS wb_interval_days,
        COALESCE((SELECT (value ->> 'days')::int FROM active_policies
                   WHERE key = 'donation_interval_days.apheresis_platelet' AND region = '*'), 7) AS plt_interval_days,
        COALESCE((SELECT (value ->> 'min')::int FROM active_policies
                   WHERE key = 'donor_age_years' AND region = '*'), 18)                          AS min_age,
        COALESCE((SELECT (value ->> 'max')::int FROM active_policies
                   WHERE key = 'donor_age_years' AND region = '*'), 65)                          AS max_age,
        COALESCE((SELECT (value ->> 'kg')::numeric FROM active_policies
                   WHERE key = 'donor_min_weight_kg' AND region = '*'), 50)                      AS min_weight_kg
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
    GREATEST(
        COALESCE((lc.last_whole_blood_at + make_interval(days => cfg.wb_interval_days))::date, CURRENT_DATE),
        COALESCE(ad.deferred_until + 1, CURRENT_DATE),
        CURRENT_DATE
    ) AS next_eligible_on,
    (
            COALESCE(ad.permanently_deferred, FALSE) = FALSE
        AND EXTRACT(YEAR FROM age(CURRENT_DATE, dp.date_of_birth)) BETWEEN cfg.min_age AND cfg.max_age
        AND COALESCE(ad.deferred_until, CURRENT_DATE - 1) < CURRENT_DATE
        AND COALESCE((lc.last_whole_blood_at + make_interval(days => cfg.wb_interval_days))::date, CURRENT_DATE) <= CURRENT_DATE
        AND u.status = 'active'
    ) AS is_eligible_today,
    CASE
        WHEN u.status <> 'active'                     THEN 'account_not_active'
        WHEN COALESCE(ad.permanently_deferred, FALSE) THEN 'permanently_deferred'
        WHEN COALESCE(ad.deferred_until, CURRENT_DATE - 1) >= CURRENT_DATE THEN 'temporarily_deferred'
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
