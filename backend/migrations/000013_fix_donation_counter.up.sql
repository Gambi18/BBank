-- 000013_fix_donation_counter
--
-- Defect found in 000010 during WI-11: `donations_sync_counter` fires AFTER
-- INSERT only, so donor_profiles.total_donations increments but never decrements.
-- Any DELETE of a donation, or any UPDATE that moves a donation to a different
-- donor, leaves the counter permanently wrong — and a denormalised counter that
-- can silently drift is worse than no counter, because it looks authoritative.
--
-- Observed for real: deleting a test donation left total_donations = 1 against
-- 0 actual donations.
--
-- Fixed by handling INSERT, UPDATE and DELETE, and by recomputing every existing
-- counter from the donations table so the drift already present is corrected.
--
-- This is a new migration rather than an edit to 000010, because 000010 has been
-- applied. Editing an applied migration means anyone who already ran it never
-- receives the fix.

CREATE OR REPLACE FUNCTION sync_donor_donation_count() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE donor_profiles
           SET total_donations = total_donations + 1, updated_at = now()
         WHERE user_id = NEW.donor_id;
        RETURN NEW;

    ELSIF TG_OP = 'DELETE' THEN
        UPDATE donor_profiles
           SET total_donations = GREATEST(total_donations - 1, 0), updated_at = now()
         WHERE user_id = OLD.donor_id;
        RETURN OLD;

    ELSIF TG_OP = 'UPDATE' AND NEW.donor_id IS DISTINCT FROM OLD.donor_id THEN
        -- A donation reassigned to a different donor must move the count.
        UPDATE donor_profiles
           SET total_donations = GREATEST(total_donations - 1, 0), updated_at = now()
         WHERE user_id = OLD.donor_id;
        UPDATE donor_profiles
           SET total_donations = total_donations + 1, updated_at = now()
         WHERE user_id = NEW.donor_id;
        RETURN NEW;
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS donations_sync_counter ON donations;
CREATE TRIGGER donations_sync_counter
    AFTER INSERT OR UPDATE OR DELETE ON donations
    FOR EACH ROW EXECUTE FUNCTION sync_donor_donation_count();

-- Correct the drift that already exists.
UPDATE donor_profiles p
   SET total_donations = COALESCE(d.n, 0), updated_at = now()
  FROM (SELECT user_id, (SELECT count(*) FROM donations WHERE donor_id = user_id) AS n
          FROM donor_profiles) d
 WHERE p.user_id = d.user_id
   AND p.total_donations IS DISTINCT FROM COALESCE(d.n, 0);
