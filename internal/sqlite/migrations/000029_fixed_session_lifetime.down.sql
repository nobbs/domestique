DELETE FROM schema_migrations WHERE version = 29;
ALTER TABLE web_sessions DROP COLUMN admin;
