-- A ride's weather gains a length for each of its rows: 3600 seconds for an
-- hour read from the reanalysis, 900 for a quarter hour read from the recent
-- forecast. Every existing row was read hourly, so 3600 is the right default
-- for it. hour_unix and activity_weather_reads.hours keep their names rather
-- than being renamed to match: every migration here must stay additive so the
-- previous release's binary can still read and write what it already did, and
-- what each already stores has not changed meaning, only what it is called.
ALTER TABLE activity_weather ADD COLUMN step_seconds INTEGER NOT NULL DEFAULT 3600;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (44, CAST(strftime('%s', 'now') AS INTEGER));
