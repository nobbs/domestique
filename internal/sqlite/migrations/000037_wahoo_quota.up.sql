-- The last request quota a Wahoo response advertised, so a restart resumes with
-- a window it already found spent instead of going straight back at Wahoo.
-- expires_at_unix is when the observation stops describing anything: a restore
-- past it discards the row rather than honouring a window that has refilled.
CREATE TABLE wahoo_quota (
  id               INTEGER PRIMARY KEY CHECK (id = 1),
  remaining        INTEGER NOT NULL,
  reset_at_unix    INTEGER NOT NULL,
  not_before_unix  INTEGER NOT NULL,
  observed_at_unix INTEGER NOT NULL,
  expires_at_unix  INTEGER NOT NULL
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (37, CAST(strftime('%s', 'now') AS INTEGER));
