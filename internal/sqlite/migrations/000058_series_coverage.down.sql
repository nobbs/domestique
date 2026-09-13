DELETE FROM schema_migrations WHERE version = 58;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the reason
-- migration 041's down migration gives: portability to whatever sqlite3 an
-- operator has on hand matters most on this path.
CREATE TABLE activity_metrics_pre_series_coverage (
  target_slot                TEXT    NOT NULL,
  workout_id                 INTEGER NOT NULL,
  zone_1_seconds             REAL,
  zone_2_seconds             REAL,
  zone_3_seconds             REAL,
  zone_4_seconds             REAL,
  zone_5_seconds             REAL,
  trimp                      REAL,
  heart_rate_tss             REAL,
  normalized_power_watts     REAL,
  intensity_factor           REAL,
  power_tss                  REAL,
  input_max_heart_rate       REAL    NOT NULL,
  input_resting_heart_rate   REAL    NOT NULL,
  input_threshold_heart_rate REAL    NOT NULL,
  input_threshold_power      REAL    NOT NULL,
  computed_at_unix           INTEGER NOT NULL,
  estimated_power_watts      REAL,
  input_total_mass           REAL    NOT NULL DEFAULT 0,
  average_heart_rate_bpm     REAL,
  max_heart_rate_bpm         REAL,
  average_cadence_rpm        REAL,
  average_power_watts        REAL,
  derivation_version         INTEGER NOT NULL DEFAULT 0,
  estimate_autocorrelation        REAL,
  estimate_delta_watts_per_second REAL,
  estimate_clip_bias_watts        REAL,
  decoupling_percent              REAL,
  heat_drift_heart_rate_bpm       REAL,
  heat_drift_temperature_celsius  REAL,
  heat_drift_samples              INTEGER,
  best_power_5s               REAL,
  best_power_30s              REAL,
  best_power_60s              REAL,
  best_power_300s             REAL,
  best_power_1200s            REAL,
  best_power_3600s            REAL,
  max_speed_kmh               REAL,
  estimated_pedalling_share   REAL,
  input_drag_area             REAL    NOT NULL DEFAULT 0,
  input_rolling_resistance    REAL    NOT NULL DEFAULT 0,
  PRIMARY KEY (target_slot, workout_id),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO activity_metrics_pre_series_coverage
SELECT target_slot, workout_id, zone_1_seconds, zone_2_seconds, zone_3_seconds, zone_4_seconds,
  zone_5_seconds, trimp, heart_rate_tss, normalized_power_watts, intensity_factor, power_tss,
  input_max_heart_rate, input_resting_heart_rate, input_threshold_heart_rate, input_threshold_power,
  computed_at_unix, estimated_power_watts, input_total_mass, average_heart_rate_bpm, max_heart_rate_bpm,
  average_cadence_rpm, average_power_watts, derivation_version, estimate_autocorrelation,
  estimate_delta_watts_per_second, estimate_clip_bias_watts, decoupling_percent, heat_drift_heart_rate_bpm,
  heat_drift_temperature_celsius, heat_drift_samples, best_power_5s, best_power_30s, best_power_60s,
  best_power_300s, best_power_1200s, best_power_3600s, max_speed_kmh, estimated_pedalling_share,
  input_drag_area, input_rolling_resistance
FROM activity_metrics;
DROP TABLE activity_metrics;
ALTER TABLE activity_metrics_pre_series_coverage RENAME TO activity_metrics;
