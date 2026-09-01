-- Reverse of 000008. The deferred FK must go before blood_requests.

ALTER TABLE IF EXISTS unit_status_events
    DROP CONSTRAINT IF EXISTS unit_status_events_blood_request_fk;

DROP TABLE IF EXISTS unit_allocations;
DROP TABLE IF EXISTS issuances;
DROP TABLE IF EXISTS blood_requests;
