CREATE TABLE login_transactions (
    state_digest    BLOB    PRIMARY KEY,
    nonce           TEXT    NOT NULL,
    code_verifier   TEXT    NOT NULL,
    expires_at_unix INTEGER NOT NULL
);
CREATE TABLE web_sessions (
    token_digest    BLOB    PRIMARY KEY,
    subject         TEXT    NOT NULL,
    display         TEXT    NOT NULL,
    created_at_unix INTEGER NOT NULL,
    renewed_at_unix INTEGER NOT NULL,
    expires_at_unix INTEGER NOT NULL
);
CREATE INDEX web_sessions_expiry_index ON web_sessions(expires_at_unix);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (28, CAST(strftime('%s', 'now') AS INTEGER));
