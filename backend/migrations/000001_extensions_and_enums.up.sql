-- 000001_extensions_and_enums (WI-13, schema §11.2 step 1)
-- Extensions and the 21 enum types. Touches no data.
-- Source: docs/DATABASE_SCHEMA.md §5 and §6.0.

CREATE EXTENSION IF NOT EXISTS citext;     -- case-insensitive email
CREATE EXTENSION IF NOT EXISTS pg_trgm;    -- fuzzy donor / unit-code search (scaling rule 9)
CREATE EXTENSION IF NOT EXISTS btree_gin;  -- composite GIN over scalar + trigram columns
CREATE EXTENSION IF NOT EXISTS btree_gist; -- equality operators inside the policies EXCLUDE

-- Clinical vocabulary
CREATE TYPE blood_group    AS ENUM ('A', 'B', 'AB', 'O');
CREATE TYPE rhesus         AS ENUM ('positive', 'negative');
CREATE TYPE gender         AS ENUM ('male', 'female', 'other', 'undisclosed');
CREATE TYPE component_type AS ENUM (
    'whole_blood', 'packed_red_cells', 'fresh_frozen_plasma', 'platelets', 'cryoprecipitate'
);
CREATE TYPE donation_procedure AS ENUM (
    'whole_blood', 'apheresis_platelet', 'apheresis_plasma', 'double_red_cell'
);

-- Identity
CREATE TYPE user_role   AS ENUM (
    'donor', 'staff', 'lab_tech', 'inventory_manager', 'hospital_user', 'admin'
);
CREATE TYPE user_status AS ENUM ('pending_verification', 'active', 'suspended', 'deactivated');
CREATE TYPE hospital_status AS ENUM ('pending_approval', 'active', 'suspended');

-- Domain state machines (foundation brief §3.2 — values are exact, do not vary)
CREATE TYPE appointment_status      AS ENUM (
    'scheduled', 'checked_in', 'completed', 'no_show', 'cancelled', 'deferred'
);
CREATE TYPE donation_request_status AS ENUM (
    'pending', 'approved', 'rejected', 'cancelled', 'expired'
);
CREATE TYPE screening_outcome       AS ENUM (
    'passed', 'deferred_temporary', 'deferred_permanent'
);
CREATE TYPE deferral_type           AS ENUM ('temporary', 'permanent');
CREATE TYPE blood_unit_status       AS ENUM (
    'quarantined', 'available', 'reserved', 'issued', 'transfused',
    'expired', 'discarded', 'recalled'
);
CREATE TYPE test_result_status      AS ENUM (
    'pending', 'non_reactive', 'reactive', 'indeterminate'
);
CREATE TYPE blood_request_status    AS ENUM (
    'pending', 'approved', 'partially_fulfilled', 'fulfilled', 'rejected', 'cancelled', 'expired'
);
CREATE TYPE urgency_level           AS ENUM ('routine', 'urgent', 'emergency');
CREATE TYPE crossmatch_result       AS ENUM ('not_required', 'compatible', 'incompatible');
CREATE TYPE issuance_outcome        AS ENUM (
    'pending', 'transfused', 'returned', 'expired', 'discarded'
);

-- Infrastructure
CREATE TYPE storage_kind         AS ENUM ('fridge', 'freezer', 'platelet_agitator', 'transport_box');
CREATE TYPE notification_channel AS ENUM ('email', 'sms', 'in_app');
CREATE TYPE notification_status  AS ENUM ('queued', 'sent', 'delivered', 'failed', 'cancelled');
