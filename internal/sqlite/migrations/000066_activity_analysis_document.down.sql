DELETE FROM schema_migrations WHERE version = 66;

ALTER TABLE activity_analyses DROP COLUMN document;
