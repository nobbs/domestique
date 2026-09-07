-- What one ride's samples and one rider's profile say about how hard it was,
-- worked out once and kept beside the ride it describes. Keyed as activities
-- is, and cascading with it: a metric outliving its ride describes nothing.
--
-- The four profile values it was computed against are on the row so a rider who
-- changes their profile leaves rows that are recognisably stale, rather than
-- rows nothing can tell apart from current ones. Every derived column is
-- nullable: a ride carries the sensors it carries, and a profile holds what the
-- rider entered, so each figure is present or absent on its own.
CREATE TABLE activity_metrics (
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
  PRIMARY KEY (target_slot, workout_id),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (40, CAST(strftime('%s', 'now') AS INTEGER));
