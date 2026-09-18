DELETE FROM schema_migrations WHERE version = 62;

ALTER TABLE plans DROP COLUMN pushing;
