-- The bicycle's own drag area and rolling resistance, for a rider whose
-- estimated power is worked out at their own numbers rather than the road
-- bicycle every other rider is estimated at. Nullable like every other
-- profile column: a rider who has entered one and not the other is
-- estimated at the built-in bicycle until both are set.
ALTER TABLE rider_profiles ADD COLUMN drag_area_m2 REAL;
ALTER TABLE rider_profiles ADD COLUMN rolling_resistance REAL;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (56, CAST(strftime('%s', 'now') AS INTEGER));
