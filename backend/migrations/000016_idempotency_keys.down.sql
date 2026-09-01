-- Reverse of 000016. Dropping the table loses replay protection for anything
-- still within its 24h window; the endpoints themselves keep working.

DROP INDEX IF EXISTS idempotency_keys_expiry_idx;
DROP TABLE IF EXISTS idempotency_keys;
