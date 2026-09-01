-- 000012_seed_reference_data (WI-16, schema §11.2 step 12, §12)
--
-- Turns the clinical constants into DATA. No eligibility threshold, donation
-- interval or component shelf life may live in Go or TypeScript source — a
-- region that donates on a different interval changes a row here, not a binary.
--
-- Idempotent via ON CONFLICT DO NOTHING, so re-running is safe.
-- Timezone: Africa/Douala (OD-14, resolved 2026-09-01).

INSERT INTO policies (key, value, region, description) VALUES
 ('donor_age_years',                          '{"min":18,"max":65,"first_time_max":60}', '*', 'Donor age window; first-time donors capped lower'),
 ('donor_min_weight_kg',                      '{"kg":50}',                               '*', 'Minimum donor weight'),
 ('donor_min_hemoglobin_g_dl',                '{"female":12.5,"male":13.0}',             '*', 'Minimum pre-donation haemoglobin'),
 ('donor_vitals_range',                       '{"bp_systolic":{"min":90,"max":180},"bp_diastolic":{"min":50,"max":100},"pulse_bpm":{"min":50,"max":100},"temperature_c":{"max":37.5}}', '*', 'Acceptable pre-donation vitals'),
 ('donation_interval_days.whole_blood',       '{"days":56}',                             '*', 'Minimum gap between whole-blood donations'),
 ('donation_interval_days.apheresis_platelet','{"days":7}',                              '*', 'Minimum gap between platelet apheresis donations'),
 ('donations_per_year_max',                   '{"male":6,"female":4,"apheresis_platelet":24}', '*', 'Annual donation caps'),
 ('shelf_life_hours.whole_blood',             '{"hours":840,"storage_c":[1,6],"note":"35 days, CPDA-1"}',      '*', 'Whole blood shelf life'),
 ('shelf_life_hours.packed_red_cells',        '{"hours":1008,"storage_c":[1,6],"note":"42 days, SAGM/AS-1"}',  '*', 'Packed red cell shelf life'),
 ('shelf_life_hours.platelets',               '{"hours":120,"storage_c":[20,24],"note":"5 days; 7 with bacterial testing"}', '*', 'Platelet shelf life'),
 ('shelf_life_hours.fresh_frozen_plasma',     '{"hours":8760,"storage_c":[-80,-18],"note":"12 months"}',       '*', 'FFP shelf life'),
 ('shelf_life_hours.cryoprecipitate',         '{"hours":8760,"storage_c":[-80,-18],"note":"12 months"}',       '*', 'Cryoprecipitate shelf life'),
 ('expiry_alert_hours',                       '{"hours":72}',                            '*', 'Dashboard near-expiry threshold'),
 ('allocation_min_remaining_hours',           '{"hours":4}',                             '*', 'Do not allocate a unit expiring sooner than this')
ON CONFLICT DO NOTHING;

INSERT INTO abo_compatibility
    (component_class, recipient_group, recipient_rhesus, donor_group, donor_rhesus, preference_rank)
VALUES
 ('red_cells','O','negative','O','negative',1),
 ('red_cells','O','positive','O','positive',1),
 ('red_cells','O','positive','O','negative',2),
 ('red_cells','A','negative','A','negative',1),
 ('red_cells','A','negative','O','negative',2),
 ('red_cells','A','positive','A','positive',1),
 ('red_cells','A','positive','A','negative',2),
 ('red_cells','A','positive','O','positive',3),
 ('red_cells','A','positive','O','negative',4),
 ('red_cells','B','negative','B','negative',1),
 ('red_cells','B','negative','O','negative',2),
 ('red_cells','B','positive','B','positive',1),
 ('red_cells','B','positive','B','negative',2),
 ('red_cells','B','positive','O','positive',3),
 ('red_cells','B','positive','O','negative',4),
 ('red_cells','AB','negative','AB','negative',1),
 ('red_cells','AB','negative','A','negative',2),
 ('red_cells','AB','negative','B','negative',3),
 ('red_cells','AB','negative','O','negative',4),
 ('red_cells','AB','positive','AB','positive',1),
 ('red_cells','AB','positive','AB','negative',2),
 ('red_cells','AB','positive','A','positive',3),
 ('red_cells','AB','positive','B','positive',4),
 ('red_cells','AB','positive','A','negative',5),
 ('red_cells','AB','positive','B','negative',6),
 ('red_cells','AB','positive','O','positive',7),
 ('red_cells','AB','positive','O','negative',8)
ON CONFLICT DO NOTHING;

INSERT INTO test_types (code, name, is_mandatory, display_order) VALUES
 ('HIV',      'HIV 1/2 antibody-antigen',     TRUE, 1),
 ('HBSAG',    'Hepatitis B surface antigen',  TRUE, 2),
 ('HCV',      'Hepatitis C antibody',         TRUE, 3),
 ('SYPHILIS', 'Treponema pallidum antibody',  TRUE, 4),
 ('MALARIA',  'Malaria antigen',              TRUE, 5)
ON CONFLICT (code) DO NOTHING;

INSERT INTO donation_centers (code, name, address_line, city, region, phone, capacity_per_slot, timezone)
VALUES
 ('DLA-01','Douala Central Blood Centre','12 Rue Joss, Bonanjo','Douala','Littoral','+237233420001',4,'Africa/Douala'),
 ('YDE-01','Yaoundé Regional Blood Centre','Avenue Kennedy','Yaoundé','Centre','+237222230002',6,'Africa/Douala'),
 ('BUE-01','Buea Mobile Collection Unit','Molyko','Buea','South-West','+237233320003',2,'Africa/Douala')
ON CONFLICT (code) DO NOTHING;

INSERT INTO storage_locations (center_id, name, kind, temp_min_c, temp_max_c, capacity_units)
SELECT c.id, v.name, v.kind::storage_kind, v.tmin, v.tmax, v.cap
FROM donation_centers c
CROSS JOIN (VALUES
    ('Fridge A',       'fridge',            1.0,   6.0, 250),
    ('Fridge B',       'fridge',            1.0,   6.0, 250),
    ('Plasma Freezer', 'freezer',         -30.0, -18.0, 400),
    ('Agitator 1',     'platelet_agitator', 20.0, 24.0,  60)
) AS v(name, kind, tmin, tmax, cap)
WHERE c.code = 'DLA-01'
ON CONFLICT (center_id, name) DO NOTHING;
