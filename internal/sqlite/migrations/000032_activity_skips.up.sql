-- Activities a poll could not read, kept apart from activities so a skipped
-- ride never reaches the read model with zeroed totals. Retried on a backoff
-- that lengthens with attempts and never gives up.
CREATE TABLE activity_skips (
  target_slot       TEXT    NOT NULL REFERENCES targets(slot),
  workout_id        INTEGER NOT NULL,
  attempts          INTEGER NOT NULL,
  last_attempt_unix INTEGER NOT NULL,
  observed          TEXT    NOT NULL,
  PRIMARY KEY (target_slot, workout_id)
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (32, CAST(strftime('%s', 'now') AS INTEGER));
