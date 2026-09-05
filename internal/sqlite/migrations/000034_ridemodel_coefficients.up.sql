-- The fitted coefficient pair a prediction is priced with, replacing the file an
-- operator used to name. One row: the model is the service's, not a target's.
CREATE TABLE ridemodel_coefficients (
  id                      INTEGER PRIMARY KEY CHECK (id = 1),
  seconds_per_km          REAL    NOT NULL CHECK (seconds_per_km > 0),
  seconds_per_ascent_m    REAL    NOT NULL CHECK (seconds_per_ascent_m > 0),
  calibration_cutoff_unix INTEGER,
  evaluated_rides         INTEGER NOT NULL DEFAULT 0 CHECK (evaluated_rides >= 0),
  bias_percent            REAL    NOT NULL DEFAULT 0,
  mae_percent             REAL    NOT NULL DEFAULT 0,
  p90_percent             REAL    NOT NULL DEFAULT 0,
  training_window_months  INTEGER NOT NULL DEFAULT 0,
  updated_at_unix         INTEGER NOT NULL
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (34, CAST(strftime('%s', 'now') AS INTEGER));
