-- 000010_views_and_triggers (WI-16, schema §11.2 step 10, §8 and §9)
--
-- Functions and triggers first, then the views. This deviates from the document's
-- section order (§8 views, then §9 triggers) for a reason: triggers reference no
-- view, but a view could reference a function, so functions-first is the safe
-- direction. Verified: no trigger function references any view.
--
-- The two that matter most:
--   * guard_unit_release  — a unit cannot reach 'available' until every mandatory
--     TTI test is non_reactive. Enforced here, in the database, not only in Go.
--     OD-18 (resolved 2026-09-01) confirms there is NO override path, ever.
--   * donor_eligibility   — computes the next eligible date from real donations
--     plus active deferrals plus policy. This is what replaces the donor-entered
--     `last_donation` field as the source of truth.

CREATE OR REPLACE FUNCTION current_actor_id() RETURNS BIGINT
LANGUAGE plpgsql STABLE AS $$
DECLARE v TEXT;
BEGIN
    v := current_setting('bbank.actor_id', true);
    IF v IS NULL OR v = '' THEN RETURN NULL; END IF;
    RETURN v::bigint;
END;
$$;

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER users_set_updated_at             BEFORE UPDATE ON users             FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER donor_profiles_set_updated_at    BEFORE UPDATE ON donor_profiles    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER donation_requests_set_updated_at BEFORE UPDATE ON donation_requests FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER appointments_set_updated_at      BEFORE UPDATE ON appointments      FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER blood_requests_set_updated_at    BEFORE UPDATE ON blood_requests    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- ...and the same for the remaining mutable tables.

CREATE OR REPLACE FUNCTION log_unit_status_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO unit_status_events (unit_id, from_status, to_status, reason, actor_id)
        VALUES (NEW.id, NULL, NEW.status, 'unit created', current_actor_id());
        RETURN NEW;
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status THEN
        INSERT INTO unit_status_events (unit_id, from_status, to_status, reason, actor_id)
        VALUES (NEW.id, OLD.status, NEW.status,
                COALESCE(NEW.discard_reason, current_setting('bbank.transition_reason', true)),
                current_actor_id());
    END IF;
    RETURN NEW;
END;
$$;

-- Refuse to release a unit while any mandatory TTI test is missing, pending,
-- indeterminate or reactive.
CREATE OR REPLACE FUNCTION guard_unit_release() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE unresolved INT;
BEGIN
    IF NEW.status = 'available' AND OLD.status IS DISTINCT FROM 'available' THEN
        SELECT count(*) INTO unresolved
        FROM test_types tt
        LEFT JOIN test_results tr
               ON tr.test_type   = tt.code
              AND tr.donation_id = NEW.donation_id
              AND tr.is_current
        WHERE tt.is_mandatory AND tt.is_active
          AND (tr.id IS NULL OR tr.result <> 'non_reactive');

        IF unresolved > 0 THEN
            RAISE EXCEPTION
              'unit % cannot be released: % mandatory TTI test(s) missing or not non_reactive',
              NEW.unit_code, unresolved
              USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION bump_unit_version() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    NEW.version    := OLD.version + 1;
    RETURN NEW;
END;
$$;

CREATE TRIGGER blood_units_guard_release BEFORE UPDATE OF status ON blood_units
    FOR EACH ROW EXECUTE FUNCTION guard_unit_release();
CREATE TRIGGER blood_units_bump_version  BEFORE UPDATE ON blood_units
    FOR EACH ROW EXECUTE FUNCTION bump_unit_version();
CREATE TRIGGER blood_units_log_status    AFTER INSERT OR UPDATE OF status ON blood_units
    FOR EACH ROW EXECUTE FUNCTION log_unit_status_change();

CREATE OR REPLACE FUNCTION sync_donor_donation_count() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE donor_profiles
       SET total_donations = total_donations + 1, updated_at = now()
     WHERE user_id = NEW.donor_id;
    RETURN NEW;
END;
$$;

CREATE TRIGGER donations_sync_counter AFTER INSERT ON donations
    FOR EACH ROW EXECUTE FUNCTION sync_donor_donation_count();

CREATE OR REPLACE FUNCTION forbid_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only; % is not permitted', TG_TABLE_NAME, TG_OP
      USING ERRCODE = 'insufficient_privilege';
END;
$$;

CREATE TRIGGER unit_status_events_append_only BEFORE UPDATE OR DELETE ON unit_status_events
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER audit_log_append_only BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE VIEW active_policies AS
SELECT DISTINCT ON (p.key, p.region)
       p.key, p.region, p.value, p.effective_from
FROM policies p
WHERE p.effective_from <= CURRENT_DATE
  AND (p.effective_to IS NULL OR p.effective_to > CURRENT_DATE)
ORDER BY p.key, p.region, p.effective_from DESC;

CREATE VIEW inventory_summary AS
SELECT
    bu.blood_group,
    bu.rhesus,
    bu.component_type,
    count(*) FILTER (WHERE bu.status = 'available')   AS units_available,
    count(*) FILTER (WHERE bu.status = 'reserved')    AS units_reserved,
    count(*) FILTER (WHERE bu.status = 'quarantined') AS units_quarantined,
    count(*) FILTER (WHERE bu.status = 'available'
                       AND bu.expires_at <= now() + INTERVAL '72 hours') AS expiring_within_72h,
    count(*) FILTER (WHERE bu.status = 'available'
                       AND bu.expires_at <= now() + INTERVAL '7 days')   AS expiring_within_7d,
    min(bu.expires_at) FILTER (WHERE bu.status = 'available')            AS next_expiry_at,
    sum(bu.volume_ml)  FILTER (WHERE bu.status = 'available')            AS volume_ml_available
FROM blood_units bu
WHERE bu.status IN ('quarantined','available','reserved')
GROUP BY bu.blood_group, bu.rhesus, bu.component_type;

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

CREATE VIEW unit_provenance AS
WITH RECURSIVE lineage AS (
    SELECT bu.id AS unit_id, bu.id AS ancestor_id, 0 AS depth, bu.parent_unit_id
    FROM blood_units bu
  UNION ALL
    SELECT l.unit_id, p.id, l.depth + 1, p.parent_unit_id
    FROM lineage l
    JOIN blood_units p ON p.id = l.parent_unit_id
),
root AS (
    SELECT DISTINCT ON (unit_id) unit_id, ancestor_id AS root_unit_id, depth AS split_depth
    FROM lineage
    ORDER BY unit_id, depth DESC
)
SELECT
    bu.id AS unit_id, bu.unit_code, bu.component_type, bu.blood_group, bu.rhesus,
    bu.status AS unit_status, bu.expires_at,
    r.root_unit_id, r.split_depth,
    don.id AS donation_id, don.collected_at, don.volume_ml AS donation_volume_ml, don.bag_lot_number,
    dp.user_id AS donor_id, dp.full_name AS donor_name,
    scr.id AS screening_id, scr.hemoglobin_g_dl, scr.outcome AS screening_outcome,
    ctr.name AS center_name,
    (SELECT bool_and(tr.result = 'non_reactive')
       FROM test_results tr WHERE tr.donation_id = don.id AND tr.is_current) AS all_tests_non_reactive,
    (SELECT jsonb_object_agg(tr.test_type, tr.result)
       FROM test_results tr WHERE tr.donation_id = don.id AND tr.is_current) AS test_panel,
    ua.blood_request_id, ua.allocated_at, ua.crossmatch_result,
    iss.issued_at, iss.received_by_name, iss.outcome AS issuance_outcome,
    hosp.name AS hospital_name, br.patient_ref
FROM blood_units bu
JOIN root r               ON r.unit_id  = bu.id
JOIN blood_units rbu      ON rbu.id     = r.root_unit_id
JOIN donations don        ON don.id     = rbu.donation_id
JOIN donor_profiles dp    ON dp.user_id = don.donor_id
JOIN donation_centers ctr ON ctr.id     = don.center_id
LEFT JOIN screenings scr      ON scr.appointment_id = don.appointment_id
LEFT JOIN unit_allocations ua ON ua.unit_id = bu.id AND ua.released_at IS NULL
LEFT JOIN issuances iss       ON iss.id     = ua.issuance_id
LEFT JOIN blood_requests br   ON br.id      = ua.blood_request_id
LEFT JOIN hospitals hosp      ON hosp.id    = br.hospital_id;
