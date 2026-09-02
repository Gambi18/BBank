-- Reverting removes the capacity constraint entirely: over-booking becomes
-- possible again, because the previous version of the application had no notion
-- of a seat and would insert without one.

DROP TRIGGER IF EXISTS appointments_enforce_slot_capacity ON appointments;
DROP FUNCTION IF EXISTS enforce_slot_capacity();
DROP INDEX IF EXISTS appointments_slot_idx;
DROP INDEX IF EXISTS appointments_one_per_slot_seat;

ALTER TABLE appointments DROP CONSTRAINT IF EXISTS appointments_slot_seat_positive;
ALTER TABLE appointments DROP COLUMN IF EXISTS slot_seat;
