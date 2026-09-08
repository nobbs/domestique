-- Which library route each stored ride was ridden on, worked out from the ride's
-- own track and kept beside it. Keyed as activities is, and cascading with it.
--
-- A ride that matched nothing still has a row, with the route columns absent:
-- "asked and rode none of them" is an answer, and without it every derivation
-- would re-match the rides that never match anything.
--
-- library_hash is what the match was measured against — the geometry of every
-- route in the library at the time. A route edited, added or removed changes it,
-- and every ride is then owed a fresh match, which is the only way a ride can
-- come to belong to a route whose line moved under it.
CREATE TABLE activity_route_match (
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
  -- A match is all of its parts or none of them, so a row naming a route always
  -- carries the coverage that justified naming it. Direction stands apart: a
  -- ride can be on a route with no telling which way round it went.
  CHECK ((provider IS NULL AND route_id IS NULL AND stage_order IS NULL
          AND route_coverage IS NULL AND ride_coverage IS NULL)
      OR (provider IS NOT NULL AND route_id IS NOT NULL AND stage_order IS NOT NULL
          AND route_coverage IS NOT NULL AND ride_coverage IS NOT NULL)),
  FOREIGN KEY (target_slot, workout_id) REFERENCES activities(target_slot, workout_id) ON DELETE CASCADE
);
-- The route page reads this the other way round, asking which rides a route has.
CREATE INDEX activity_route_match_route_index
  ON activity_route_match(provider, route_id, stage_order);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (46, CAST(strftime('%s', 'now') AS INTEGER));
