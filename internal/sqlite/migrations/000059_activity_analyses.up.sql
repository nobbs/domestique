-- What a language model made of one ride. Keyed to the ride rather than to its
-- metrics row, which migrations rebuild: a rebuild's DROP TABLE would cascade
-- through a foreign key and discard analyses that each cost a request.
CREATE TABLE activity_analyses (
  target_slot      TEXT    NOT NULL,
  workout_id       INTEGER NOT NULL,
  text             TEXT    NOT NULL CHECK (length(text) BETWEEN 1 AND 2000),
  model            TEXT    NOT NULL,
  prompt_revision  INTEGER NOT NULL,
  analysed_at_unix INTEGER NOT NULL,
  PRIMARY KEY (target_slot, workout_id),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
-- The instant the analysis was first enabled. Only rides started after it are
-- owed one, so it is written once and never moved.
CREATE TABLE analysis_state (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  enabled_since_unix INTEGER NOT NULL
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (59, CAST(strftime('%s', 'now') AS INTEGER));
