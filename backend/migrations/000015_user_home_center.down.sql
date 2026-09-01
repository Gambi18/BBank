-- Reverse of 000015_user_home_center.
--
-- Constraint and index names are copied from the up migration rather than
-- recalled — guessing them is how a down migration fails at the worst moment.

DROP INDEX IF EXISTS users_center_idx;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_center_matches_role;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_center_fk;

ALTER TABLE users
    DROP COLUMN IF EXISTS center_id;
