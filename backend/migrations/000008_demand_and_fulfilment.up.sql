-- 000008_demand_and_fulfilment (WI-15, schema §11.2 step 8)
--
-- Purely additive. This is the demand side, which did not exist in any form: no
-- hospital could ask for blood, and the landing page's promise of "priority
-- matching" for hospitals described a feature with no code behind it.
--
-- Note the vocabulary boundary. `blood_requests` is a HOSPITAL asking for units.
-- `donation_requests` (000004) is a DONOR asking for a slot. The legacy `requests`
-- table meant the latter while using the name of the former.

-- A hospital asking for units. The demand side that did not exist at all.
CREATE TABLE blood_requests (
    id                BIGINT               GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    hospital_id       BIGINT               NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    requested_by      BIGINT               NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    patient_ref       TEXT                 NOT NULL,
    patient_age_years SMALLINT,
    blood_group       blood_group          NOT NULL,
    rhesus            rhesus               NOT NULL,
    component_type    component_type       NOT NULL,
    units_requested   SMALLINT             NOT NULL,
    units_fulfilled   SMALLINT             NOT NULL DEFAULT 0,
    urgency           urgency_level        NOT NULL DEFAULT 'routine',
    needed_by         TIMESTAMPTZ          NOT NULL,
    indication        TEXT,
    status            blood_request_status NOT NULL DEFAULT 'pending',
    reviewed_by       BIGINT               REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at       TIMESTAMPTZ,
    rejection_reason  TEXT,
    created_at        TIMESTAMPTZ          NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ          NOT NULL DEFAULT now(),
    CONSTRAINT blood_requests_units_range  CHECK (units_requested BETWEEN 1 AND 50),
    CONSTRAINT blood_requests_fulfil_range CHECK (units_fulfilled BETWEEN 0 AND units_requested),
    CONSTRAINT blood_requests_fulfilled_complete
        CHECK (status <> 'fulfilled' OR units_fulfilled = units_requested),
    CONSTRAINT blood_requests_reject_needs_reason
        CHECK (status <> 'rejected' OR rejection_reason IS NOT NULL),
    CONSTRAINT blood_requests_age CHECK (patient_age_years IS NULL OR patient_age_years BETWEEN 0 AND 130)
);

-- Handing units over to a hospital against a blood_request.
CREATE TABLE issuances (
    id                  BIGINT           GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    blood_request_id    BIGINT           NOT NULL REFERENCES blood_requests(id) ON DELETE RESTRICT,
    issued_at           TIMESTAMPTZ      NOT NULL DEFAULT now(),
    issued_by           BIGINT           NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    received_by_name    TEXT             NOT NULL,
    received_by_role    TEXT,
    delivery_note_url   TEXT,
    transport_temp_c    NUMERIC(4,1),
    outcome             issuance_outcome NOT NULL DEFAULT 'pending',
    outcome_recorded_at TIMESTAMPTZ,
    outcome_notes       TEXT,
    created_at          TIMESTAMPTZ      NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ      NOT NULL DEFAULT now(),
    CONSTRAINT issuances_outcome_sync CHECK ((outcome = 'pending') = (outcome_recorded_at IS NULL)),
    CONSTRAINT issuances_temp_range   CHECK (transport_temp_c IS NULL OR transport_temp_c BETWEEN -80 AND 30)
);

-- The reservation of one specific unit against one specific request.
CREATE TABLE unit_allocations (
    id                BIGINT            GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    blood_request_id  BIGINT            NOT NULL REFERENCES blood_requests(id) ON DELETE RESTRICT,
    unit_id           BIGINT            NOT NULL REFERENCES blood_units(id) ON DELETE RESTRICT,
    issuance_id       BIGINT            REFERENCES issuances(id) ON DELETE RESTRICT,
    crossmatch_result crossmatch_result NOT NULL DEFAULT 'not_required',
    crossmatch_at     TIMESTAMPTZ,
    crossmatch_by     BIGINT            REFERENCES users(id) ON DELETE RESTRICT,
    allocated_at      TIMESTAMPTZ       NOT NULL DEFAULT now(),
    allocated_by      BIGINT            NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    released_at       TIMESTAMPTZ,
    release_reason    TEXT,
    created_at        TIMESTAMPTZ       NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ       NOT NULL DEFAULT now(),
    CONSTRAINT unit_allocations_release_sync CHECK ((released_at IS NULL) = (release_reason IS NULL)),
    CONSTRAINT unit_allocations_xm_sync      CHECK ((crossmatch_at IS NULL) = (crossmatch_by IS NULL)),
    CONSTRAINT unit_allocations_issued_needs_xm
        CHECK (issuance_id IS NULL OR crossmatch_result <> 'incompatible'),
    CONSTRAINT unit_allocations_issued_not_released
        CHECK (issuance_id IS NULL OR released_at IS NULL)
);

-- The FK 000007 deferred, now that blood_requests exists.
ALTER TABLE unit_status_events
    ADD CONSTRAINT unit_status_events_blood_request_fk
        FOREIGN KEY (blood_request_id) REFERENCES blood_requests(id) ON DELETE SET NULL;
