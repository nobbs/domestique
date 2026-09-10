DELETE FROM schema_migrations WHERE version = 56;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the reason
-- migration 041's down migration gives: portability to whatever sqlite3 an
-- operator has on hand matters most on this path. rider_profiles is
-- referenced by nothing, so a rebuild is safe here.
CREATE TABLE rider_profiles_pre_bicycle (
  subject                          TEXT    PRIMARY KEY,
  max_heart_rate_bpm               REAL,
  resting_heart_rate_bpm           REAL,
  threshold_heart_rate_bpm         REAL,
  functional_threshold_power_watts REAL,
  rider_mass_kg                    REAL,
  bike_mass_kg                     REAL,
  updated_at_unix                  INTEGER NOT NULL
);
INSERT INTO rider_profiles_pre_bicycle
SELECT subject, max_heart_rate_bpm, resting_heart_rate_bpm, threshold_heart_rate_bpm,
  functional_threshold_power_watts, rider_mass_kg, bike_mass_kg, updated_at_unix
FROM rider_profiles;
DROP TABLE rider_profiles;
ALTER TABLE rider_profiles_pre_bicycle RENAME TO rider_profiles;
