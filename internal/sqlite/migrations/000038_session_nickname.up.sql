-- The ID token's nickname claim, personal data of the same kind as subject.
-- Nullable: a token without the claim stores nothing rather than a guess.
ALTER TABLE web_sessions ADD COLUMN nickname TEXT;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (38, CAST(strftime('%s', 'now') AS INTEGER));
