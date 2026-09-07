-- Which way round its route a ride went. It does not decide the match -- a loop
-- ridden anticlockwise is the same loop -- but a route ridden the other way is
-- not the same ride: its climbs are its descents. A consumer pooling rides over
-- a route reads this before comparing them.
--
-- Nullable, and empty for a ride whose direction could not be told, which an
-- out-and-back never can: it advances as far one way as the other.
ALTER TABLE activity_route_match ADD COLUMN direction TEXT;
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (47, CAST(strftime('%s', 'now') AS INTEGER));
