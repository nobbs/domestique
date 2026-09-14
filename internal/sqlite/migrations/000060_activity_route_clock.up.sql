-- A ride's moving time along the route it was matched to, as a JSON object
-- {"line": fingerprint of the route line, "readings": [{"i": sample index,
-- "a": metres along, "s": moving seconds}]}. Kept on
-- the match itself, so a match replaced, deleted or cleared takes it along.
-- Absent for a ride with no match, or one that ran its route the other way.
ALTER TABLE activity_route_match ADD COLUMN route_clock TEXT;
-- Every matched ride is owed the clock this pass now derives, and a match whose
-- library hash differs is what that pass looks for.
UPDATE activity_route_match SET library_hash = '' WHERE provider IS NOT NULL;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (60, CAST(strftime('%s', 'now') AS INTEGER));
