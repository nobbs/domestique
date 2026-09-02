-- Existing rows carry a 30-day expiry nothing here re-caps to the new fixed
-- 24-hour lifetime, and their admin status predates the claim that now
-- decides it: every subject signs in fresh once this lands.
DELETE FROM web_sessions;
ALTER TABLE web_sessions ADD COLUMN admin INTEGER NOT NULL DEFAULT 0;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (29, CAST(strftime('%s', 'now') AS INTEGER));
