-- 000000_baseline — a faithful capture of the schema that main.go used to create
-- at boot (the `CREATE TABLE IF NOT EXISTS` block removed in WI-08).
--
-- Purpose: give golang-migrate a known starting point. `IF NOT EXISTS` is retained
-- *here specifically* so that an existing database — which already has these three
-- tables — reconciles to version 0 without error. Later migrations must NOT use
-- IF NOT EXISTS; silently skipping drift is exactly the failure this replaces.
--
-- This schema is superseded from 000001 onward (see docs/DATABASE_SCHEMA.md §11.2).

CREATE TABLE IF NOT EXISTS donors (
    id            SERIAL PRIMARY KEY,
    full_name     TEXT,
    email         TEXT UNIQUE,
    dob           DATE,
    gender        TEXT,
    blood_group   TEXT,
    rhesus        TEXT,
    contact       TEXT,
    address       TEXT,
    password      TEXT,
    last_donation DATE
);

CREATE TABLE IF NOT EXISTS requests (
    id            SERIAL PRIMARY KEY,
    donor_id      INTEGER REFERENCES donors(id),
    donor_name    TEXT,
    last_donation DATE,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS appointments (
    id               SERIAL PRIMARY KEY,
    request_id       INTEGER,
    donor_id         INTEGER REFERENCES donors(id),
    donor_name       TEXT,
    appointment_date DATE
);
