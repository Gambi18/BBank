-- 000003_reference_and_facilities (WI-13, schema §11.2 step 3)
-- Facilities and reference data: donation_centers, storage_locations, hospitals,
-- policies, test_types, abo_compatibility. Also adds the users.hospital_id FK,
-- which 000002 deliberately left as a bare column because hospitals did not exist yet.
--
-- Reference *rows* are seeded in 000012, not here — except the placeholder center,
-- which exists so 000005 has something to point appointments at.

-- A physical site where donations are collected.
CREATE TABLE donation_centers (
    id                BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code              TEXT        NOT NULL,
    name              TEXT        NOT NULL,
    address_line      TEXT        NOT NULL,
    city              TEXT        NOT NULL,
    region            TEXT        NOT NULL,
    phone             TEXT,
    email             CITEXT,
    latitude          NUMERIC(9,6),
    longitude         NUMERIC(9,6),
    capacity_per_slot SMALLINT    NOT NULL DEFAULT 4,
    slot_minutes      SMALLINT    NOT NULL DEFAULT 30,
    opening_hours     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    timezone          TEXT        NOT NULL DEFAULT 'Africa/Douala',
    is_active         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT donation_centers_code_key  UNIQUE (code),
    CONSTRAINT donation_centers_capacity  CHECK (capacity_per_slot BETWEEN 1 AND 100),
    CONSTRAINT donation_centers_slot_len  CHECK (slot_minutes BETWEEN 5 AND 240),
    CONSTRAINT donation_centers_lat_range CHECK (latitude  IS NULL OR latitude  BETWEEN -90  AND 90),
    CONSTRAINT donation_centers_lng_range CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)
);

-- A temperature-controlled place inside a center where units physically sit.
CREATE TABLE storage_locations (
    id             BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    center_id      BIGINT       NOT NULL REFERENCES donation_centers(id) ON DELETE RESTRICT,
    name           TEXT         NOT NULL,
    kind           storage_kind NOT NULL,
    temp_min_c     NUMERIC(4,1) NOT NULL,
    temp_max_c     NUMERIC(4,1) NOT NULL,
    capacity_units INTEGER,
    is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT storage_locations_name_key   UNIQUE (center_id, name),
    CONSTRAINT storage_locations_temp_order CHECK (temp_min_c < temp_max_c),
    CONSTRAINT storage_locations_temp_sane  CHECK (temp_min_c >= -80 AND temp_max_c <= 30),
    CONSTRAINT storage_locations_capacity   CHECK (capacity_units IS NULL OR capacity_units > 0)
);

-- A partner hospital that may raise blood_requests.
CREATE TABLE hospitals (
    id            BIGINT          GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT            NOT NULL,
    license_no    TEXT            NOT NULL,
    address_line  TEXT            NOT NULL,
    city          TEXT            NOT NULL,
    region        TEXT            NOT NULL,
    phone         TEXT,
    contact_email CITEXT          NOT NULL,
    status        hospital_status NOT NULL DEFAULT 'pending_approval',
    created_at    TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ     NOT NULL DEFAULT now(),
    CONSTRAINT hospitals_license_key UNIQUE (license_no)
);

ALTER TABLE users
    ADD CONSTRAINT users_hospital_fk FOREIGN KEY (hospital_id)
        REFERENCES hospitals(id) ON DELETE RESTRICT,
    ADD CONSTRAINT users_hospital_only_for_hospital_user
        CHECK ((role = 'hospital_user') = (hospital_id IS NOT NULL));

-- Versioned, region-scoped clinical and operational thresholds. See §12.1.
CREATE TABLE policies (
    id             BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key            TEXT        NOT NULL,
    value          JSONB       NOT NULL,
    region         TEXT        NOT NULL DEFAULT '*',
    description    TEXT,
    effective_from DATE        NOT NULL DEFAULT CURRENT_DATE,
    effective_to   DATE,
    created_by     BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT policies_window CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT policies_no_overlap EXCLUDE USING gist (
        key    WITH =,
        region WITH =,
        daterange(effective_from, effective_to) WITH &&
    )
);

-- The TTI panel. A table, not an enum, because the panel is region-configurable.
CREATE TABLE test_types (
    code          TEXT        PRIMARY KEY,
    name          TEXT        NOT NULL,
    is_mandatory  BOOLEAN     NOT NULL DEFAULT TRUE,
    region        TEXT        NOT NULL DEFAULT '*',
    display_order SMALLINT    NOT NULL DEFAULT 0,
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT test_types_code_upper CHECK (code = upper(code) AND code ~ '^[A-Z0-9_]{2,32}$')
);

-- ABO/Rh compatibility matrix. Joinable, so allocation stays a single SQL statement.
CREATE TABLE abo_compatibility (
    component_class  TEXT        NOT NULL,
    recipient_group  blood_group NOT NULL,
    recipient_rhesus rhesus      NOT NULL,
    donor_group      blood_group NOT NULL,
    donor_rhesus     rhesus      NOT NULL,
    preference_rank  SMALLINT    NOT NULL DEFAULT 100,
    PRIMARY KEY (component_class, recipient_group, recipient_rhesus, donor_group, donor_rhesus),
    CONSTRAINT abo_compatibility_class CHECK (component_class IN ('red_cells','plasma','platelets'))
);

-- Placeholder center. 000005 assigns existing appointments to it; real centers
-- arrive in 000012. Timezone is Africa/Douala (OD-14, resolved 2026-09-01).
INSERT INTO donation_centers (code, name, address_line, city, region, timezone)
VALUES ('MAIN', 'Main Donation Centre', 'To be confirmed', 'Douala', 'Littoral', 'Africa/Douala')
ON CONFLICT (code) DO NOTHING;
