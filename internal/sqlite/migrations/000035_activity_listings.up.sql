-- Listings the account holds that are not stored yet, kept so a poll can drain
-- its backlog without re-reading the account's whole workout list. A stored
-- activity's row is removed, which makes the account's own total comparable to
-- the stored rows plus these.
CREATE TABLE activity_listings (
  target_slot              TEXT    NOT NULL REFERENCES targets(slot),
  workout_id               INTEGER NOT NULL,
  started_at_unix          INTEGER NOT NULL,
  workout_type_id          INTEGER NOT NULL,
  workout_type_location_id INTEGER NOT NULL,
  PRIMARY KEY (target_slot, workout_id)
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (35, CAST(strftime('%s', 'now') AS INTEGER));
