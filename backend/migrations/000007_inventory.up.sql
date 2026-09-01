-- 000007_inventory (WI-15, schema §11.2 step 7)
--
-- Purely additive. The blood unit is the central entity of the whole system —
-- not the donor. A unit is one bag of one component, individually identified,
-- with its own expiry and its own status.
--
-- `unit_status_events` is the append-only traceability ledger. Every status
-- transition writes a row; nothing silently UPDATEs. This is what makes the
-- vein-to-vein chain reconstructable, which is a regulatory requirement, not a
-- nicety — if a transfusion reaction occurs you must be able to walk backwards
-- from the recipient to the donor.
--
-- unit_status_events.blood_request_id is declared bare here; its FK arrives in
-- 000008 once blood_requests exists.

-- One bag of one component. The central entity of the system.
CREATE TABLE blood_units (
    id                  BIGINT            GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    unit_code           TEXT              NOT NULL,
    donation_id         BIGINT            NOT NULL REFERENCES donations(id) ON DELETE RESTRICT,
    parent_unit_id      BIGINT            REFERENCES blood_units(id) ON DELETE RESTRICT,
    component_type      component_type    NOT NULL,
    blood_group         blood_group       NOT NULL,
    rhesus              rhesus            NOT NULL,
    volume_ml           INTEGER           NOT NULL,
    collected_at        TIMESTAMPTZ       NOT NULL,
    processed_at        TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ       NOT NULL,
    status              blood_unit_status NOT NULL DEFAULT 'quarantined',
    storage_location_id BIGINT            REFERENCES storage_locations(id) ON DELETE RESTRICT,
    discard_reason      TEXT,
    version             INTEGER           NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ       NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ       NOT NULL DEFAULT now(),
    CONSTRAINT blood_units_code_key    UNIQUE (unit_code),
    CONSTRAINT blood_units_code_format CHECK (unit_code ~ '^[A-Z0-9]{3,8}-[0-9]{4}-[0-9]{6}-[A-Z]{2,4}$'),
    CONSTRAINT blood_units_volume      CHECK (volume_ml BETWEEN 15 AND 550),
    CONSTRAINT blood_units_expiry      CHECK (expires_at > collected_at),
    CONSTRAINT blood_units_processed   CHECK (processed_at IS NULL OR processed_at >= collected_at),
    CONSTRAINT blood_units_not_own_parent CHECK (parent_unit_id IS DISTINCT FROM id),
    CONSTRAINT blood_units_discard_reason
        CHECK (status NOT IN ('discarded','recalled') OR discard_reason IS NOT NULL)
);

-- Append-only ledger of every blood_units.status transition. The traceability record.
CREATE TABLE unit_status_events (
    id               BIGINT            GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    unit_id          BIGINT            NOT NULL REFERENCES blood_units(id) ON DELETE RESTRICT,
    from_status      blood_unit_status,
    to_status        blood_unit_status NOT NULL,
    reason           TEXT,
    actor_id         BIGINT            REFERENCES users(id) ON DELETE SET NULL,
    blood_request_id BIGINT,                    -- FK added in 000008, once blood_requests exists
    occurred_at      TIMESTAMPTZ       NOT NULL DEFAULT now(),
    CONSTRAINT unit_status_events_real_change CHECK (from_status IS DISTINCT FROM to_status)
);

-- One TTI test result. Repeats supersede rather than overwrite.
CREATE TABLE test_results (
    id          BIGINT             GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    donation_id BIGINT             NOT NULL REFERENCES donations(id) ON DELETE RESTRICT,
    test_type   TEXT               NOT NULL REFERENCES test_types(code) ON DELETE RESTRICT,
    result      test_result_status NOT NULL DEFAULT 'pending',
    is_current  BOOLEAN            NOT NULL DEFAULT TRUE,
    tested_at   TIMESTAMPTZ,
    tested_by   BIGINT             REFERENCES users(id) ON DELETE RESTRICT,
    instrument  TEXT,
    kit_lot_no  TEXT,
    remarks     TEXT,
    repeat_of   BIGINT             REFERENCES test_results(id) ON DELETE RESTRICT,
    created_at  TIMESTAMPTZ        NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ        NOT NULL DEFAULT now(),
    CONSTRAINT test_results_pending_sync   CHECK ((result = 'pending') = (tested_at IS NULL)),
    CONSTRAINT test_results_tester_sync    CHECK ((tested_at IS NULL) = (tested_by IS NULL)),
    CONSTRAINT test_results_not_own_repeat CHECK (repeat_of IS DISTINCT FROM id)
);
