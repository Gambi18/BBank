-- 000011_indexes (WI-16, schema §11.2 step 11, §7)
--
-- 38 indexes covering the real query patterns: inventory availability by
-- group+component+status, units nearing expiry, fuzzy donor lookup at the
-- check-in desk (pg_trgm), audit lookups, and the booking/flow paths.
--
-- NOTE ON `CONCURRENTLY`. Schema §11.2 recommends CREATE INDEX CONCURRENTLY,
-- which cannot run inside a transaction. That advice is right for adding an index
-- to an already-large table in production. It is deliberately NOT used here: at
-- this point in the sequence every indexed table was created empty by
-- 000006-000009, so CONCURRENTLY would buy nothing and would cost this migration
-- its transactional safety and its clean rollback. Use CONCURRENTLY in a later,
-- standalone migration when adding an index to a populated table.

-- Q: "Show me available O-negative packed red cells, oldest first."  (FEFO allocation, §10)
CREATE INDEX blood_units_availability_idx
    ON blood_units (blood_group, rhesus, component_type, expires_at)
    WHERE status = 'available';

-- Q: "What expires in the next 72 hours?"  (dashboard tile + nightly expiry sweep)
CREATE INDEX blood_units_expiring_idx
    ON blood_units (expires_at)
    WHERE status IN ('available','reserved','quarantined');

CREATE INDEX blood_units_donation_idx ON blood_units (donation_id);
CREATE INDEX blood_units_parent_idx   ON blood_units (parent_unit_id) WHERE parent_unit_id IS NOT NULL;
CREATE INDEX blood_units_location_idx ON blood_units (storage_location_id)
    WHERE status IN ('quarantined','available','reserved');

CREATE INDEX donor_profiles_name_trgm_idx ON donor_profiles USING gin (full_name  gin_trgm_ops);
CREATE INDEX donor_profiles_nid_trgm_idx  ON donor_profiles USING gin (national_id gin_trgm_ops);
CREATE INDEX donor_profiles_phone_idx     ON donor_profiles (contact_phone);
CREATE INDEX donor_profiles_abo_idx       ON donor_profiles (blood_group, rhesus) WHERE blood_group IS NOT NULL;
CREATE INDEX blood_units_code_trgm_idx    ON blood_units USING gin (unit_code gin_trgm_ops);

-- Q: "Today's schedule at Douala Central."
CREATE INDEX appointments_center_day_idx ON appointments (center_id, scheduled_at)
    WHERE status IN ('scheduled','checked_in');
CREATE INDEX appointments_donor_idx      ON appointments (donor_id, scheduled_at DESC);

-- Invariant: a donor may have at most one open donation request at a time.
CREATE UNIQUE INDEX donation_requests_one_open_per_donor
    ON donation_requests (donor_id) WHERE status = 'pending';

-- Invariant: one active appointment per donor per calendar day.
CREATE UNIQUE INDEX appointments_one_active_per_donor_per_day
    ON appointments (donor_id, ((scheduled_at AT TIME ZONE 'UTC')::date))
    WHERE status IN ('scheduled','checked_in');

-- Q: admin review queue.
CREATE INDEX donation_requests_queue_idx ON donation_requests (center_id, preferred_date)
    WHERE status = 'pending';

CREATE INDEX donations_donor_recent_idx  ON donations (donor_id, collected_at DESC);
CREATE INDEX donations_center_day_idx    ON donations (center_id, collected_at DESC);
CREATE INDEX screenings_donor_idx        ON screenings (donor_id, screened_at DESC);
CREATE INDEX deferrals_active_idx        ON deferrals (donor_id) WHERE lifted_at IS NULL;
CREATE INDEX policies_lookup_idx         ON policies (key, region, effective_from DESC);

CREATE INDEX test_results_donation_idx ON test_results (donation_id);

-- Q: "Which donations are still blocking unit release?"  (lab worklist)
CREATE INDEX test_results_open_idx ON test_results (donation_id)
    WHERE is_current AND result IN ('pending','indeterminate');

-- Q: "Full status history for unit X."  (traceability, the vein-to-vein walk)
CREATE INDEX unit_status_events_unit_idx   ON unit_status_events (unit_id, occurred_at DESC);
CREATE INDEX unit_status_events_recent_idx ON unit_status_events (occurred_at DESC);

-- Q: hospital's own request list.
CREATE INDEX blood_requests_queue_idx ON blood_requests (hospital_id, status, needed_by);

-- Q: "Open requests, most urgent first."  (the inventory manager's primary screen)
CREATE INDEX blood_requests_open_idx  ON blood_requests (urgency, needed_by)
    WHERE status IN ('pending','approved','partially_fulfilled');

-- Q: "Which open requests could this newly-released unit satisfy?"
CREATE INDEX blood_requests_match_idx ON blood_requests (blood_group, rhesus, component_type)
    WHERE status IN ('pending','approved','partially_fulfilled');

CREATE INDEX unit_allocations_request_idx ON unit_allocations (blood_request_id);
CREATE INDEX issuances_request_idx        ON issuances (blood_request_id, issued_at DESC);

-- THE safety constraint: a unit can have at most one live allocation. See §10.
CREATE UNIQUE INDEX unit_allocations_one_live_per_unit
    ON unit_allocations (unit_id) WHERE released_at IS NULL;

-- Exactly one current result per donation per test type; repeats set is_current = FALSE.
CREATE UNIQUE INDEX test_results_one_current_per_donation_type
    ON test_results (donation_id, test_type) WHERE is_current;

-- Q: "Everything that ever happened to blood_unit 4471."  (incident reconstruction)
CREATE INDEX audit_log_entity_idx    ON audit_log (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_log_actor_idx     ON audit_log (actor_id, created_at DESC);
CREATE INDEX audit_log_after_gin_idx ON audit_log USING gin (after jsonb_path_ops);

-- Q: outbox poll — "what is due to send?"
CREATE INDEX notifications_outbox_idx ON notifications (scheduled_for) WHERE status = 'queued';
CREATE INDEX notifications_user_idx   ON notifications (user_id, created_at DESC);

CREATE INDEX users_role_status_idx ON users (role, status);
CREATE INDEX users_hospital_idx    ON users (hospital_id) WHERE hospital_id IS NOT NULL;
