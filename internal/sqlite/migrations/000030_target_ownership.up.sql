-- Nullable, and left NULL on existing rows: at migration time a target
-- authorized before this column existed has no subject to attribute it to.
-- It is claimed automatically the moment a subject whose own value matches
-- its slot connects (EnsureTargetOwner), or by an operator assigning it by
-- hand before then — never guessed at from anything else.
ALTER TABLE targets ADD COLUMN owner_subject TEXT;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (30, CAST(strftime('%s', 'now') AS INTEGER));
