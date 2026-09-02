-- Nullable, and left NULL on existing rows: a target authorized before this
-- migration has no subject to attribute it to, and guessing one would be
-- wrong more often than leaving it for an operator to assign by hand.
ALTER TABLE targets ADD COLUMN owner_subject TEXT;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (30, CAST(strftime('%s', 'now') AS INTEGER));
