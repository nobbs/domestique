DELETE FROM schema_migrations WHERE version = 65;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the reason
-- migration 041's down migration gives.
CREATE TABLE runtime_source_pre_sync (
  position INTEGER PRIMARY KEY,
  provider TEXT NOT NULL,
  base_url TEXT NOT NULL
);
INSERT INTO runtime_source_pre_sync (position, provider, base_url)
SELECT position, provider, base_url FROM runtime_source;
DROP TABLE runtime_source;
ALTER TABLE runtime_source_pre_sync RENAME TO runtime_source;
