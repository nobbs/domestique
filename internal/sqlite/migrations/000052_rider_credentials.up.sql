-- One rider's own credential, keyed by the subject a session is issued for,
-- not by the deployment. Never read back to any caller; the value is sealed
-- under the state key with the subject and the name as associated data.
CREATE TABLE rider_credentials (
  subject         TEXT    NOT NULL,
  name            TEXT    NOT NULL,
  value           BLOB    NOT NULL,
  updated_at_unix INTEGER NOT NULL,
  PRIMARY KEY (subject, name)
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (52, CAST(strftime('%s', 'now') AS INTEGER));
