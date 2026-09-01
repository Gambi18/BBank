-- Reverse of 000012. Removes only the seeded reference rows. Operational data
-- (donations, units, requests) is never touched by this migration in either
-- direction. The MAIN placeholder center from 000003 is left alone — it is not
-- seeded here and 000005 may still point appointments at it.
--
-- This down migration is only runnable BEFORE operational data exists. Reference
-- rows are protected by ON DELETE RESTRICT precisely so that a seed rollback
-- cannot orphan a real clinical record — rolling back `test_types` while a
-- `test_results` row cites it would destroy the meaning of that result.
--
-- Rather than surfacing an opaque FK violation, fail with a message that says
-- what is actually wrong and what to do about it.

DO $$
DECLARE
    n_tests      BIGINT;
    n_units      BIGINT;
    n_allocs     BIGINT;
BEGIN
    SELECT count(*) INTO n_tests  FROM test_results;
    SELECT count(*) INTO n_units  FROM blood_units;
    SELECT count(*) INTO n_allocs FROM unit_allocations;

    IF n_tests > 0 OR n_units > 0 OR n_allocs > 0 THEN
        RAISE EXCEPTION
          'Refusing to roll back seed reference data: % test_results, % blood_units and % unit_allocations reference it. Rolling back reference data under live clinical records would orphan them. Remove the operational data first, or roll back to 000009 instead.',
          n_tests, n_units, n_allocs;
    END IF;
END $$;

DELETE FROM abo_compatibility;
DELETE FROM test_types;
DELETE FROM policies;
DELETE FROM storage_locations;
DELETE FROM donation_centers WHERE code <> 'MAIN';
