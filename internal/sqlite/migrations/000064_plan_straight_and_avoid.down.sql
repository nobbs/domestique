DELETE FROM schema_migrations WHERE version = 64;

-- A rebuild rather than ALTER TABLE ... DROP COLUMN, for the reason
-- migration 041's down migration gives.
CREATE TABLE plans_pre_avoid (
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
  updated_at_unix_nano INTEGER NOT NULL,
  pushing              TEXT    NOT NULL DEFAULT '[]',
  turns                TEXT    NOT NULL DEFAULT '[]',
  cues                 INTEGER NOT NULL DEFAULT 0
);
INSERT INTO plans_pre_avoid (id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
  published, version, created_at_unix_nano, updated_at_unix_nano, pushing, turns, cues)
SELECT id, name, profile, waypoints, coordinates, distance_metres, ascent_metres,
  published, version, created_at_unix_nano, updated_at_unix_nano, pushing, turns, cues FROM plans;
DROP TABLE plans;
ALTER TABLE plans_pre_avoid RENAME TO plans;
