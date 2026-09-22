-- The structured answer prompt revision 3 asks for, alongside the summary
-- text every revision has written. NULL for a row written before it.
ALTER TABLE activity_analyses ADD COLUMN document TEXT;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (66, CAST(strftime('%s', 'now') AS INTEGER));
