DELETE FROM schema_migrations WHERE version = 30;

-- ALTER TABLE ... DROP COLUMN, not a rebuild: unlike web_sessions (migration
-- 029), oauth_transactions and target_stages reference targets(slot) by
-- foreign key, and golang-migrate runs this file inside one transaction —
-- PRAGMA foreign_keys is a no-op mid-transaction, so a rebuild's DROP TABLE
-- would fail against any real target with recorded history. owner_subject
-- itself carries no constraint DROP COLUMN refuses (not a key, not indexed,
-- not referenced), so this is safe on its own terms.
ALTER TABLE targets DROP COLUMN owner_subject;
