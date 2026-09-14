-- A plan is an admin-composed route: ordered waypoints and a routing profile,
-- routed through the self-hosted engine on save. id is drawn at random from
-- the positive int63 range at creation, never assigned in sequence, so a
-- database rebuilt after state loss cannot reissue a deleted plan's identity.
CREATE TABLE plans (
  id                   INTEGER PRIMARY KEY,
  name                 TEXT    NOT NULL,
  profile              TEXT    NOT NULL CHECK (profile IN ('trekking', 'fastbike', 'gravel')),
  -- JSON array of [longitude, latitude] pairs the admin drew.
  waypoints            TEXT    NOT NULL,
  -- The routed geometry, encoded exactly as stage_geometry.coordinates.
  coordinates          BLOB    NOT NULL,
  distance_metres      REAL    NOT NULL,
  ascent_metres        REAL    NOT NULL,
  published            INTEGER NOT NULL DEFAULT 0 CHECK (published IN (0, 1)),
  -- The counter a replace or delete must present as If-Match; never leaves
  -- the service as a Wahoo-facing value.
  version              INTEGER NOT NULL,
  created_at_unix_nano INTEGER NOT NULL,
  updated_at_unix_nano INTEGER NOT NULL
);
INSERT INTO schema_migrations (version, applied_at_unix) VALUES (61, CAST(strftime('%s', 'now') AS INTEGER));
