-- WI-24: make over-booking a slot IMPOSSIBLE, not merely checked.
--
-- `FR-14`'s acceptance criterion is "two simultaneous approvals into a one-seat
-- slot produce exactly one appointment". A count-then-insert cannot deliver
-- that: both transactions count 0, both see room, both insert. The plan is
-- explicit that this must be **a constraint**, and this is it.
--
-- THE MODEL: a slot has `capacity_per_slot` numbered SEATS, and an appointment
-- occupies exactly one of them.
--
--   * `appointments.slot_seat` says which.
--   * A partial unique index makes two live appointments in the same seat
--     impossible. Whichever transaction commits second gets a 23505 and the
--     service tries the next seat.
--   * A trigger keeps the seat inside the centre's capacity, so no caller —
--     including a future one nobody has written yet, or psql — can widen a slot
--     by inventing seat 99.
--
-- Together those bound a slot at exactly `capacity_per_slot` live appointments,
-- with no lock held across the decision and no count anybody can race.
--
-- WHY SEATS RATHER THAN A COUNTING TRIGGER: a trigger that counts rows in the
-- slot is the same check-then-insert one layer down — under READ COMMITTED it
-- cannot see another transaction's uncommitted row, so two concurrent inserts
-- both count 0 and both pass. Making the constraint a UNIQUE INDEX moves the
-- decision to the one place Postgres already serialises.
--
-- `scheduled_at` IS THE SLOT START. The service snaps a requested time down to
-- the centre's `slot_minutes` grid before writing, so "the same slot" and "the
-- same `scheduled_at`" are one thing. Without that, two appointments five
-- minutes apart would be different slots and capacity would mean nothing.

-- Added NULLABLE and backfilled before anything enforces it.
--
-- `NOT NULL DEFAULT 1` would have been wrong on any database with data in it.
-- Every appointment created before this migration sits at a hardcoded 09:00
-- (migration 000005), and the only uniqueness on the table is per DONOR per day
-- (`appointments_one_active_per_donor_per_day`) — so two different donors booked
-- at one centre on one day are two rows with the same `(center_id,
-- scheduled_at)`. Giving them both seat 1 makes the unique index below
-- unbuildable, and the migration fails on the first real database it meets.
-- The test suite could not catch it: migrations run once against an empty
-- database.
ALTER TABLE appointments ADD COLUMN slot_seat SMALLINT;

-- Number the live appointments within each existing slot. Cancelled ones are
-- excluded from the index, so they can all sit at seat 1 without colliding.
WITH numbered AS (
    SELECT id, row_number() OVER (
               PARTITION BY center_id, scheduled_at ORDER BY id
           ) AS seat
    FROM appointments
    WHERE status <> 'cancelled'
)
UPDATE appointments a
   SET slot_seat = numbered.seat
  FROM numbered
 WHERE a.id = numbered.id;

UPDATE appointments SET slot_seat = 1 WHERE slot_seat IS NULL;

ALTER TABLE appointments
    ALTER COLUMN slot_seat SET NOT NULL,
    ALTER COLUMN slot_seat SET DEFAULT 1;

ALTER TABLE appointments
    ADD CONSTRAINT appointments_slot_seat_positive CHECK (slot_seat >= 1);

-- The seat is released by cancelling, and only by cancelling.
--
-- `no_show`, `completed` and `deferred` all describe a slot that WAS used: the
-- donor was expected, the chair was held, and the appointment is history. Only
-- `cancelled` means "this never occupied the slot", so only `cancelled` frees
-- the seat for somebody else. Freeing a no-show's seat would also let a past
-- slot be re-booked, which is not a thing that can happen.
CREATE UNIQUE INDEX appointments_one_per_slot_seat
    ON appointments (center_id, scheduled_at, slot_seat)
    WHERE status <> 'cancelled';

CREATE OR REPLACE FUNCTION enforce_slot_capacity() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    cap SMALLINT;
    active BOOLEAN;
BEGIN
    SELECT capacity_per_slot, is_active INTO cap, active
    FROM donation_centers WHERE id = NEW.center_id;

    IF cap IS NULL THEN
        RAISE EXCEPTION 'no donation centre with id %', NEW.center_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    IF NEW.slot_seat > cap THEN
        RAISE EXCEPTION 'seat % exceeds capacity % at centre %', NEW.slot_seat, cap, NEW.center_id
            USING ERRCODE = 'check_violation';
    END IF;

    -- A deactivated centre takes no NEW bookings (FR-14), and existing ones are
    -- untouched — "deactivating a center stops new bookings and preserves
    -- history". Enforced on INSERT only, so cancelling, completing or marking a
    -- no-show at a closed centre all still work: the history has to be
    -- finishable.
    IF TG_OP = 'INSERT' AND NOT active THEN
        RAISE EXCEPTION 'donation centre % is not accepting bookings', NEW.center_id
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;

-- On UPDATE OF the fields that decide which slot this is, as well as on INSERT:
-- rescheduling moves an appointment into a different slot, and a move that
-- over-fills the destination is the same defect arriving by a different route.
-- Created LAST, after the backfill, deliberately.
--
-- The backfill is an UPDATE of `slot_seat`, which this trigger fires on. An
-- existing slot may already hold more appointments than the centre's capacity —
-- nothing enforced one before today — and those rows would be rejected by their
-- own backfill, failing the migration to protect a rule that did not exist when
-- they were booked.
--
-- Leaving them is the right answer and matches FR-14's "preserves history": the
-- appointments people already have are kept, and the capacity governs the next
-- booking. The one consequence is that RESCHEDULING such an appointment
-- reallocates its seat in the destination slot (see RescheduleAppointment), so
-- an over-capacity legacy row cannot carry its high seat number forward.
CREATE TRIGGER appointments_enforce_slot_capacity
    BEFORE INSERT OR UPDATE OF center_id, scheduled_at, slot_seat ON appointments
    FOR EACH ROW EXECUTE FUNCTION enforce_slot_capacity();

-- Supports the "which seats are taken in this slot" read the allocator does.
CREATE INDEX appointments_slot_idx
    ON appointments (center_id, scheduled_at)
    WHERE status <> 'cancelled';
