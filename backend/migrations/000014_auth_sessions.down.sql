-- Reverse of 000014. Dropping sessions logs everyone out, which is the correct
-- consequence of removing the session store.

DROP TRIGGER IF EXISTS sessions_set_updated_at ON sessions;
DROP TABLE IF EXISTS sessions;
ALTER TABLE users DROP COLUMN IF EXISTS token_version;
