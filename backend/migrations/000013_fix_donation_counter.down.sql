-- Reverse of 000013: restore the INSERT-only trigger from 000010.
-- Note this reintroduces the drift defect; the down path exists for
-- completeness of the migration chain, not because reverting is advisable.

CREATE OR REPLACE FUNCTION sync_donor_donation_count() RETURNS TRIGGER AS $$
BEGIN
    UPDATE donor_profiles
       SET total_donations = total_donations + 1, updated_at = now()
     WHERE user_id = NEW.donor_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS donations_sync_counter ON donations;
CREATE TRIGGER donations_sync_counter
    AFTER INSERT ON donations
    FOR EACH ROW EXECUTE FUNCTION sync_donor_donation_count();
