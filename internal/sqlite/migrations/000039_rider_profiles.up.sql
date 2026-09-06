-- One rider's own body and equipment, keyed by the subject a session is issued
-- for rather than by a target: a rider has these numbers whether or not they
-- have connected an account. Every column is nullable, so a parameter the rider
-- has not entered is absent rather than a zero a calculation would believe.
CREATE TABLE rider_profiles (
  subject                          TEXT    PRIMARY KEY,
  max_heart_rate_bpm               REAL,
  resting_heart_rate_bpm           REAL,
  threshold_heart_rate_bpm         REAL,
  functional_threshold_power_watts REAL,
  rider_mass_kg                    REAL,
  bike_mass_kg                     REAL,
  updated_at_unix                  INTEGER NOT NULL
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (39, CAST(strftime('%s', 'now') AS INTEGER));
