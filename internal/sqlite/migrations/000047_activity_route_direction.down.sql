DELETE FROM schema_migrations WHERE version = 47;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the reason migration
-- 041's down migration gives: portability to whatever sqlite3 an operator has
-- on hand matters most on this path.
CREATE TABLE activity_route_match_pre_direction (
  target_slot     TEXT    NOT NULL,
  workout_id      INTEGER NOT NULL,
  provider        TEXT,
  route_id        INTEGER,
  stage_order     INTEGER,
  route_coverage  REAL,
  ride_coverage   REAL,
  library_hash    TEXT    NOT NULL,
  matched_at_unix INTEGER NOT NULL,
  PRIMARY KEY (target_slot, workout_id),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
INSERT INTO activity_route_match_pre_direction
SELECT target_slot, workout_id, provider, route_id, stage_order,
  route_coverage, ride_coverage, library_hash, matched_at_unix
FROM activity_route_match;
DROP TABLE activity_route_match;
ALTER TABLE activity_route_match_pre_direction RENAME TO activity_route_match;
CREATE INDEX activity_route_match_route_index
  ON activity_route_match(provider, route_id, stage_order);
