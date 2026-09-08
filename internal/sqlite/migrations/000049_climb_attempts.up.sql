-- How long one ride took over one of its route's sustained climbs, and what the
-- rider's sensors said while it lasted. One row per ride per climb, written by
-- the same pass that matched the ride to the route, and cascading with the ride.
--
-- What the climb *is* -- where it runs, how far it rises, how steep it averages
-- -- is not here. That belongs to the route, is found again from its stored
-- geometry whenever it is read, and would only ever disagree with itself if a
-- copy were kept beside every attempt at it. climb_index says which of the
-- route's climbs this is, in the order they are ridden.
--
-- A row exists only for a climb the ride actually rode, the way its route
-- stores it: a ride that turned back half way up, or ran the route the other
-- way round, has no row rather than a row of nothing.
--
-- Measured and estimated power sit in their own columns and are never summed or
-- ranked together. An estimate is a different kind of number.
CREATE TABLE activity_climb_attempt (
  target_slot           TEXT    NOT NULL,
  workout_id            INTEGER NOT NULL,
  climb_index           INTEGER NOT NULL,
  seconds               REAL    NOT NULL,
  heart_rate_bpm        REAL,
  power_watts           REAL,
  estimated_power_watts REAL,
  PRIMARY KEY (target_slot, workout_id, climb_index),
  -- A climb takes time to ride; a row saying otherwise is not an attempt.
  CHECK (seconds > 0),
  CHECK (climb_index >= 0),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (49, CAST(strftime('%s', 'now') AS INTEGER));
