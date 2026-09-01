-- 000006_screening_and_collection (WI-15, schema §11.2 step 6)
--
-- Purely additive. Creates the three tables that record what happens to a donor
-- between arriving and leaving: the pre-donation health check, any deferral it
-- produces, and the collection event itself.
--
-- `donations` is the row that was entirely missing from this system. Without it
-- nothing recorded that a donation actually happened, which is why
-- donors.last_donation was donor-entered free text and why the 56-day interval
-- could not be enforced.

-- Pre-donation health check. Exactly one per appointment; gates whether collection happens.
CREATE TABLE screenings (
    id              BIGINT            GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    appointment_id  BIGINT            NOT NULL REFERENCES appointments(id) ON DELETE RESTRICT,
    donor_id        BIGINT            NOT NULL REFERENCES donor_profiles(user_id) ON DELETE RESTRICT,
    hemoglobin_g_dl NUMERIC(4,1)      NOT NULL,
    bp_systolic     SMALLINT          NOT NULL,
    bp_diastolic    SMALLINT          NOT NULL,
    pulse_bpm       SMALLINT          NOT NULL,
    weight_kg       NUMERIC(5,1)      NOT NULL,
    temperature_c   NUMERIC(3,1)      NOT NULL,
    questionnaire   JSONB             NOT NULL DEFAULT '{}'::jsonb,
    outcome         screening_outcome NOT NULL,
    deferred_until  DATE,
    notes           TEXT,
    screened_by     BIGINT            NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    screened_at     TIMESTAMPTZ       NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ       NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ       NOT NULL DEFAULT now(),
    CONSTRAINT screenings_appointment_key UNIQUE (appointment_id),
    CONSTRAINT screenings_hb_range     CHECK (hemoglobin_g_dl BETWEEN 3.0 AND 25.0),
    CONSTRAINT screenings_sys_range    CHECK (bp_systolic  BETWEEN 60 AND 260),
    CONSTRAINT screenings_dia_range    CHECK (bp_diastolic BETWEEN 30 AND 160),
    CONSTRAINT screenings_bp_order     CHECK (bp_systolic > bp_diastolic),
    CONSTRAINT screenings_pulse_range  CHECK (pulse_bpm BETWEEN 30 AND 200),
    CONSTRAINT screenings_weight_range CHECK (weight_kg BETWEEN 30.0 AND 300.0),
    CONSTRAINT screenings_temp_range   CHECK (temperature_c BETWEEN 30.0 AND 45.0),
    CONSTRAINT screenings_defer_sync   CHECK ((outcome = 'deferred_temporary') = (deferred_until IS NOT NULL)),
    CONSTRAINT screenings_questionnaire_object CHECK (jsonb_typeof(questionnaire) = 'object')
);

-- A period during which a donor may not donate.
CREATE TABLE deferrals (
    id           BIGINT        GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    donor_id     BIGINT        NOT NULL REFERENCES donor_profiles(user_id) ON DELETE RESTRICT,
    screening_id BIGINT        REFERENCES screenings(id) ON DELETE SET NULL,
    type         deferral_type NOT NULL,
    reason_code  TEXT,
    reason       TEXT          NOT NULL,
    starts_on    DATE          NOT NULL DEFAULT CURRENT_DATE,
    ends_on      DATE,
    lifted_at    TIMESTAMPTZ,
    lifted_by    BIGINT        REFERENCES users(id) ON DELETE SET NULL,
    created_by   BIGINT        REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CONSTRAINT deferrals_end_matches_type CHECK ((type = 'permanent') = (ends_on IS NULL)),
    CONSTRAINT deferrals_window           CHECK (ends_on IS NULL OR ends_on > starts_on),
    CONSTRAINT deferrals_lift_sync        CHECK ((lifted_at IS NULL) = (lifted_by IS NULL))
);

-- The physical collection event. The row that was entirely missing from the legacy schema.
CREATE TABLE donations (
    id                   BIGINT             GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    appointment_id       BIGINT             NOT NULL REFERENCES appointments(id) ON DELETE RESTRICT,
    donor_id             BIGINT             NOT NULL REFERENCES donor_profiles(user_id) ON DELETE RESTRICT,
    center_id            BIGINT             NOT NULL REFERENCES donation_centers(id) ON DELETE RESTRICT,
    procedure            donation_procedure NOT NULL DEFAULT 'whole_blood',
    collected_at         TIMESTAMPTZ        NOT NULL DEFAULT now(),
    volume_ml            INTEGER            NOT NULL,
    bag_lot_number       TEXT               NOT NULL,
    anticoagulant        TEXT               NOT NULL DEFAULT 'CPDA-1',
    phlebotomist_id      BIGINT             NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    had_adverse_reaction BOOLEAN            NOT NULL DEFAULT FALSE,
    adverse_reaction     TEXT,
    notes                TEXT,
    created_at           TIMESTAMPTZ        NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ        NOT NULL DEFAULT now(),
    CONSTRAINT donations_appointment_key UNIQUE (appointment_id),
    CONSTRAINT donations_volume_range    CHECK (volume_ml BETWEEN 200 AND 550),
    CONSTRAINT donations_reaction_sync   CHECK (had_adverse_reaction = FALSE OR adverse_reaction IS NOT NULL)
);
