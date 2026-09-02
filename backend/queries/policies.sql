-- Every active policy row, for every key.
--
-- The whole set in one query, not a lookup per key: a decision must be made
-- against ONE snapshot. Fetching `donor_age_years` and then
-- `donation_interval_days.whole_blood` as two round trips lets an administrator
-- change a threshold in between, so the decision is made half under the old
-- policy and half under the new one — and the version stamped on it would name
-- neither.
--
-- name: ListActivePolicies :many
SELECT key, region, value, effective_from
FROM active_policies
ORDER BY key, region;
