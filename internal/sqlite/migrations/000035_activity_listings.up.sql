-- The rider's account as the last full reading of it left it, so a poll can
-- tell what the account holds from what it has stored without reading the list
-- again. Comparing this against the account's own count is what notices a ride
-- added, or one deleted after it was stored; read_at_unix is when the reading
-- these rows came from was taken, which bounds how long any change that count
-- cannot see stays unnoticed.
CREATE TABLE activity_listings (
  target_slot              TEXT    NOT NULL REFERENCES targets(slot),
  workout_id               INTEGER NOT NULL,
  started_at_unix          INTEGER NOT NULL,
  workout_type_id          INTEGER NOT NULL,
  workout_type_location_id INTEGER NOT NULL,
  read_at_unix             INTEGER NOT NULL,
  PRIMARY KEY (target_slot, workout_id)
);
CREATE INDEX activity_listings_started_index ON activity_listings(target_slot, started_at_unix);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (35, CAST(strftime('%s', 'now') AS INTEGER));
