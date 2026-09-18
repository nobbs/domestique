DELETE FROM schema_migrations WHERE version = 62;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the reason
-- migration 041's down migration gives.
CREATE TABLE plans_pre_pushing (
  id                   INTEGER PRIMARY KEY,
  name                 TEXT    NOT NULL,
  profile              TEXT    NOT NULL CHECK (profile IN ('trekking', 'fastbike', 'gravel')),
  waypoints            TEXT    NOT NULL,
  coordinates          BLOB    NOT NULL,
  distance_metres      REAL    NOT NULL,
  ascent_metres        REAL    NOT NULL,
  published            INTEGER NOT NULL DEFAULT 0 CHECK (published IN (0, 1)),
  version              INTEGER NOT NULL,
  created_at_unix_nano INTEGER NOT NULL,
  updated_at_unix_nano INTEGER NOT NULL
);
INSERT INTO plans_pre_pushing (id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
  published, version, created_at_unix_nano, updated_at_unix_nano)
SELECT id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
  published, version, created_at_unix_nano, updated_at_unix_nano FROM plans;
DROP TABLE plans;
ALTER TABLE plans_pre_pushing RENAME TO plans;
