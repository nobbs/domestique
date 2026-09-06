DELETE FROM schema_migrations WHERE version = 38;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the same reason
-- migration 029's down migration is: portability to whatever sqlite3 an
-- operator has on hand matters most on this path.
CREATE TABLE web_sessions_pre_nickname (
    token_digest    BLOB    PRIMARY KEY,
    subject         TEXT    NOT NULL,
    display         TEXT    NOT NULL,
    admin           INTEGER NOT NULL DEFAULT 0,
    created_at_unix INTEGER NOT NULL,
    renewed_at_unix INTEGER NOT NULL,
    expires_at_unix INTEGER NOT NULL
);
INSERT INTO web_sessions_pre_nickname (token_digest, subject, display, admin, created_at_unix, renewed_at_unix, expires_at_unix)
SELECT token_digest, subject, display, admin, created_at_unix, renewed_at_unix, expires_at_unix FROM web_sessions;
DROP TABLE web_sessions;
ALTER TABLE web_sessions_pre_nickname RENAME TO web_sessions;
CREATE INDEX web_sessions_expiry_index ON web_sessions(expires_at_unix);
