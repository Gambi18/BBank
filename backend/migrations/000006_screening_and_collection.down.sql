-- Reverse of 000006. The tables are created empty by this migration, so on a
-- clean rollback nothing is lost. Order matters: donations references appointments
-- and screenings; deferrals references screenings.

DROP TABLE IF EXISTS donations;
DROP TABLE IF EXISTS deferrals;
DROP TABLE IF EXISTS screenings;
