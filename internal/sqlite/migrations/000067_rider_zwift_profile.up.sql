-- One rider's Zwift FTP, read from the profile the Zwift poll already fetches
-- on every sign-in; stored nowhere else and removed with the credentials.
CREATE TABLE rider_zwift_profiles (
  subject TEXT PRIMARY KEY,
  ftp_watts REAL NOT NULL,
  read_at_unix INTEGER NOT NULL
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (67, CAST(strftime('%s', 'now') AS INTEGER));
