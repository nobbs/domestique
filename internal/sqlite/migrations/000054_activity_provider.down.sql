DELETE FROM schema_migrations WHERE version = 54;

ALTER TABLE activities DROP COLUMN provider;
