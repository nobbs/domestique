DELETE FROM schema_migrations WHERE version = 29;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN: the latter needs SQLite
-- 3.35+, and a down migration is exactly the disaster-recovery path where
-- portability to whatever sqlite3 an operator has on hand matters most.
CREATE TABLE web_sessions_pre_admin (
    token_digest    BLOB    PRIMARY KEY,
    subject         TEXT    NOT NULL,
    display         TEXT    NOT NULL,
    created_at_unix INTEGER NOT NULL,
    renewed_at_unix INTEGER NOT NULL,
    expires_at_unix INTEGER NOT NULL
);
INSERT INTO web_sessions_pre_admin (token_digest, subject, display, created_at_unix, renewed_at_unix, expires_at_unix)
SELECT token_digest, subject, display, created_at_unix, renewed_at_unix, expires_at_unix FROM web_sessions;
DROP TABLE web_sessions;
ALTER TABLE web_sessions_pre_admin RENAME TO web_sessions;
CREATE INDEX web_sessions_expiry_index ON web_sessions(expires_at_unix);
