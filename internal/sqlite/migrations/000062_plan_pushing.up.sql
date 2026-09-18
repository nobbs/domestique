-- Where a plan runs along a way bicycles are refused and a rider walks, as a
-- JSON array of [start, end] pairs in metres from the plan's start. Only the
-- routing engine's answer says which ways those are, so it is kept with the
-- geometry it was measured against.
ALTER TABLE plans ADD COLUMN pushing TEXT NOT NULL DEFAULT '[]';
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (62, CAST(strftime('%s', 'now') AS INTEGER));
