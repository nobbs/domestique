DELETE FROM schema_migrations WHERE version = 46;

DROP INDEX IF EXISTS activity_route_match_route_index;
DROP TABLE IF EXISTS activity_route_match;
