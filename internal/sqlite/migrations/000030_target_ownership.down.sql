DELETE FROM schema_migrations WHERE version = 30;

-- ALTER TABLE ... DROP COLUMN, not a rebuild: unlike web_sessions (migration
-- 029), oauth_transactions and target_stages reference targets(slot) by
-- foreign key, and golang-migrate runs this file inside one transaction —
-- PRAGMA foreign_keys is a no-op mid-transaction, so a rebuild's DROP TABLE
-- would fail against any real target with recorded history. owner_subject
-- itself carries no constraint DROP COLUMN refuses (not a key, not indexed,
-- not referenced), so this is safe on its own terms. DROP COLUMN itself needs
-- SQLite 3.35+, which this service's pinned modernc.org/sqlite always bundles
-- — never a system libsqlite3 an operator's host could hold an older one of.
ALTER TABLE targets DROP COLUMN owner_subject;
