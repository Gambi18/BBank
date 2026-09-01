-- Reverse of 000017. Dropping invites strands any user still in
-- 'pending_verification': they have no usable password and no way to set one.
-- That is the correct consequence of removing the invite mechanism, not a bug —
-- an admin must reactivate them by another route before running this.

DROP INDEX IF EXISTS user_invites_expiry_idx;
DROP INDEX IF EXISTS user_invites_one_open_idx;
DROP TABLE IF EXISTS user_invites;
