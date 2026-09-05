DELETE FROM schema_migrations WHERE version = 33;

-- ALTER TABLE ... DROP COLUMN rather than a rebuild, as migration 030 explains:
-- activity_records references activities by foreign key, and a rebuild's DROP
-- TABLE would fail against any stored record inside this one transaction.
-- The partial index must go first: SQLite refuses to drop an indexed column.
DROP INDEX activities_pending_records_index;
ALTER TABLE activities DROP COLUMN records_state;
ALTER TABLE activities DROP COLUMN fit_checksum_failed;
