-- Summary values support volume reports; FIT records stay in their own table so
-- those common reads never load every sensor sample. An activity belongs to the
-- target whose Wahoo account recorded it, which is also who may read it.
CREATE TABLE activities (
  target_slot              TEXT    NOT NULL REFERENCES targets(slot),
  workout_id               INTEGER NOT NULL,
  workout_type_id          INTEGER NOT NULL,
  workout_type_location_id INTEGER NOT NULL,
  started_at_unix          INTEGER NOT NULL,
  distance_metres          REAL    NOT NULL,
  moving_seconds           REAL    NOT NULL,
  elapsed_seconds          REAL    NOT NULL,
  ascent_metres            REAL    NOT NULL,
  raw_summary_json         BLOB    NOT NULL,
  updated_at_unix          INTEGER NOT NULL,
  PRIMARY KEY (target_slot, workout_id)
);
CREATE INDEX activities_started_index ON activities(target_slot, started_at_unix);
CREATE TABLE activity_records (
  target_slot         TEXT    NOT NULL,
  workout_id          INTEGER NOT NULL,
  record_index        INTEGER NOT NULL,
  recorded_at_unix    INTEGER NOT NULL,
  distance_metres     REAL,
  latitude            REAL,
  longitude           REAL,
  altitude_metres     REAL,
  cadence_rpm         REAL,
  heart_rate_bpm      REAL,
  power_watts         REAL,
  temperature_celsius REAL,
  PRIMARY KEY (target_slot, workout_id, record_index),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (31, CAST(strftime('%s', 'now') AS INTEGER));
