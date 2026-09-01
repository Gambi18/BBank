-- 000015_user_home_center (WI-20)
--
-- Gives the `cid` claim (TRD §7.3) a source column.
--
-- The token design has always specified `cid` — "home donation_centers.id, or
-- null for donors and admins" — and the RBAC matrix (§7.6) scopes every staff
-- grant to `ctr`. But `users` carried only `hospital_id`, so `cid` was signed as
-- NULL on every token ever issued. The middleware fails closed on a NULL centre,
-- which means a `staff` account could see nothing at all rather than seeing its
-- own centre: safe, and useless. This closes the gap.
--
-- Mirrors the shape of the `hospital_id` column added in 000003, deliberately —
-- the two claims are the same idea applied to the two scoping dimensions.

ALTER TABLE users
    ADD COLUMN center_id BIGINT;

ALTER TABLE users
    ADD CONSTRAINT users_center_fk FOREIGN KEY (center_id)
        REFERENCES donation_centers(id) ON DELETE RESTRICT;

-- Who may hold a home centre, and who must.
--
--   staff                          MUST have one — every one of their matrix
--                                  cells is `ctr`-scoped, so a staff account
--                                  without a centre is an account that can do
--                                  nothing.
--   donor, admin, hospital_user    must NOT — §7.3 says null for donors and
--                                  admins, and a hospital user is scoped by
--                                  `hid` instead.
--   lab_tech, inventory_manager    MAY. They work somewhere, but the matrix
--                                  grants them cross-centre reads, so a centre
--                                  is a fact about them rather than a limit on
--                                  them.
ALTER TABLE users
    ADD CONSTRAINT users_center_matches_role
        CHECK (
            CASE role
                WHEN 'staff'         THEN center_id IS NOT NULL
                WHEN 'donor'         THEN center_id IS NULL
                WHEN 'admin'         THEN center_id IS NULL
                WHEN 'hospital_user' THEN center_id IS NULL
                ELSE TRUE
            END
        );

-- Staff listings are always "this centre's ...", so the centre leads the index.
CREATE INDEX users_center_idx ON users (center_id, role) WHERE center_id IS NOT NULL;

COMMENT ON COLUMN users.center_id IS
    'Home donation centre. Signed into the access token as the `cid` claim (TRD §7.3) and used by the RBAC middleware as the mandatory WHERE clause for every ctr-scoped grant (§7.6).';
