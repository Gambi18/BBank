-- 000009_platform (WI-15, schema §11.2 step 9)
--
-- Purely additive. `audit_log` is partitioned by month: it is append-only, it is
-- the largest table in the system by row count, and retention is enforced by
-- dropping whole partitions rather than by DELETE.
--
-- The audit log is a DOMAIN requirement, distinct from and additional to
-- application logging. It answers "who did what to which record and when", which
-- a regulator asks and a log aggregator cannot answer.
--
-- A default partition catches anything outside the declared ranges so an insert
-- can never fail for want of a partition. Creating next month's partition is an
-- operational task (Phase 5, WI-88).

-- Domain audit trail. Distinct from and additional to observability logging.
CREATE TABLE audit_log (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY,
    actor_id    BIGINT,
    actor_role  user_role,
    action      TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,
    entity_id   TEXT        NOT NULL,
    before      JSONB,
    after       JSONB,
    ip          INET,
    user_agent  TEXT,
    request_id  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE audit_log_2026m09 PARTITION OF audit_log
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE audit_log_default PARTITION OF audit_log DEFAULT;

-- Outbox for email/SMS/in-app messages. Not a clinical record.
CREATE TABLE notifications (
    id            BIGINT               GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       BIGINT               NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel       notification_channel NOT NULL,
    template      TEXT                 NOT NULL,
    payload       JSONB                NOT NULL DEFAULT '{}'::jsonb,
    status        notification_status  NOT NULL DEFAULT 'queued',
    scheduled_for TIMESTAMPTZ          NOT NULL DEFAULT now(),
    sent_at       TIMESTAMPTZ,
    attempts      SMALLINT             NOT NULL DEFAULT 0,
    failed_reason TEXT,
    created_at    TIMESTAMPTZ          NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ          NOT NULL DEFAULT now(),
    CONSTRAINT notifications_sent_sync CHECK ((status IN ('sent','delivered')) = (sent_at IS NOT NULL)),
    CONSTRAINT notifications_fail_sync CHECK (status <> 'failed' OR failed_reason IS NOT NULL),
    CONSTRAINT notifications_attempts  CHECK (attempts BETWEEN 0 AND 100)
);
