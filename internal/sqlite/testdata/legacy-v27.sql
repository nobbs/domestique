-- Fresh schema produced by the legacy Go migration engine at version 27 in 635d5c8.
CREATE TABLE schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at_unix INTEGER NOT NULL
);
WITH RECURSIVE versions(version) AS (
  SELECT 1
  UNION ALL
  SELECT version + 1 FROM versions WHERE version < 27
)
INSERT INTO schema_migrations (version, applied_at_unix)
SELECT version, 0 FROM versions;

CREATE TABLE targets (
  slot TEXT PRIMARY KEY,
  wahoo_user_id TEXT UNIQUE,
  refresh_token BLOB,
  authorization_state TEXT NOT NULL CHECK (authorization_state IN ('not_authorized', 'authorized', 'needs_reauthorization')),
  updated_at_unix INTEGER NOT NULL
);
CREATE TABLE oauth_transactions (
  id TEXT PRIMARY KEY,
  target_slot TEXT NOT NULL REFERENCES targets(slot),
  state_digest BLOB NOT NULL,
  code_verifier BLOB NOT NULL,
  expires_at_unix INTEGER NOT NULL,
  used_at_unix INTEGER,
  caller_login TEXT NOT NULL DEFAULT ''
);
CREATE TABLE trusted_inventory (
  target_slot TEXT PRIMARY KEY REFERENCES targets(slot),
  captured_at_unix INTEGER NOT NULL
);
CREATE TABLE sync_runs (
  id INTEGER PRIMARY KEY,
  started_at_unix INTEGER NOT NULL,
  finished_at_unix INTEGER,
  outcome TEXT NOT NULL,
  detail TEXT,
  source_stages INTEGER NOT NULL DEFAULT 0,
  created INTEGER NOT NULL DEFAULT 0,
  updated INTEGER NOT NULL DEFAULT 0,
  deleted INTEGER NOT NULL DEFAULT 0,
  phase TEXT NOT NULL DEFAULT '',
  reference TEXT NOT NULL DEFAULT ''
);
CREATE TABLE notification_state (
  kind TEXT PRIMARY KEY,
  last_sent_at_unix INTEGER NOT NULL,
  last_run_id INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE sync_schedule (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  source_enabled INTEGER NOT NULL CHECK (source_enabled IN (0, 1)),
  targets_enabled INTEGER NOT NULL CHECK (targets_enabled IN (0, 1)),
  updated_at_unix INTEGER NOT NULL
);
INSERT INTO sync_schedule VALUES (1, 1, 1, 0);
CREATE TABLE target_runs (
  target_slot TEXT PRIMARY KEY REFERENCES targets(slot),
  finished_at_unix INTEGER NOT NULL,
  outcome TEXT NOT NULL,
  detail TEXT NOT NULL
);
CREATE TABLE surface_index (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  built_at_unix INTEGER NOT NULL,
  generation TEXT NOT NULL
);
INSERT INTO surface_index VALUES (1, 0, '');

CREATE TABLE "source_stages" (
  provider TEXT NOT NULL DEFAULT 'veloplanner',
  route_id INTEGER NOT NULL,
  stage_order INTEGER NOT NULL,
  source_revision TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  PRIMARY KEY (provider, route_id, stage_order)
);
CREATE TABLE "target_stages" (
  target_slot TEXT NOT NULL REFERENCES targets(slot),
  provider TEXT NOT NULL DEFAULT 'veloplanner',
  route_id INTEGER NOT NULL,
  stage_order INTEGER NOT NULL,
  wahoo_route_id INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  source_revision TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (target_slot, provider, route_id, stage_order)
);
CREATE TABLE "trusted_inventory_stages" (
  target_slot TEXT NOT NULL REFERENCES trusted_inventory(target_slot),
  provider TEXT NOT NULL DEFAULT 'veloplanner',
  route_id INTEGER NOT NULL,
  stage_order INTEGER NOT NULL,
  wahoo_route_id INTEGER NOT NULL,
  PRIMARY KEY (target_slot, provider, route_id, stage_order)
);
CREATE TABLE "stage_geometry" (
  provider TEXT NOT NULL DEFAULT 'veloplanner',
  route_id INTEGER NOT NULL,
  stage_order INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  route_name TEXT NOT NULL,
  stage_name TEXT NOT NULL,
  point_count INTEGER NOT NULL,
  distance_metres REAL NOT NULL,
  ascent_metres REAL NOT NULL DEFAULT 0,
  max_gradient_percent REAL NOT NULL DEFAULT 0,
  min_longitude REAL NOT NULL,
  min_latitude REAL NOT NULL,
  max_longitude REAL NOT NULL,
  max_latitude REAL NOT NULL,
  coordinates BLOB NOT NULL,
  updated_at_unix INTEGER NOT NULL,
  descent_metres REAL NOT NULL DEFAULT 0,
  PRIMARY KEY (provider, route_id, stage_order)
);
CREATE TABLE "stage_surface" (
  provider TEXT NOT NULL DEFAULT 'veloplanner',
  route_id INTEGER NOT NULL,
  stage_order INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  ranges BLOB NOT NULL,
  matched_metres REAL NOT NULL,
  updated_at_unix INTEGER NOT NULL,
  index_generation TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (provider, route_id, stage_order)
);
CREATE TABLE "stage_reprocess" (
  provider TEXT NOT NULL DEFAULT 'veloplanner',
  route_id INTEGER NOT NULL,
  stage_order INTEGER NOT NULL,
  requested_at_unix INTEGER NOT NULL,
  PRIMARY KEY (provider, route_id, stage_order)
);
CREATE TABLE stage_duration (
  provider TEXT NOT NULL,
  route_id INTEGER NOT NULL,
  stage_order INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  surface_generation TEXT NOT NULL,
  coefficient_fingerprint TEXT NOT NULL,
  moving_seconds REAL,
  cumulative_seconds BLOB,
  updated_at_unix INTEGER NOT NULL,
  PRIMARY KEY (provider, route_id, stage_order)
);
CREATE TABLE runtime_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  allow_empty_source_deletion INTEGER NOT NULL CHECK (allow_empty_source_deletion IN (0, 1)),
  stale_after_seconds INTEGER NOT NULL CHECK (stale_after_seconds > 0),
  notifications_enabled INTEGER NOT NULL CHECK (notifications_enabled IN (0, 1)),
  success_policy TEXT NOT NULL CHECK (success_policy IN ('every', 'quiet', 'digest')),
  digest_interval_seconds INTEGER NOT NULL CHECK (digest_interval_seconds > 0),
  pushover_base_url TEXT NOT NULL,
  surface_rebuild_interval_seconds INTEGER NOT NULL CHECK (surface_rebuild_interval_seconds > 0),
  updated_at_unix INTEGER NOT NULL,
  wahoo_api_base_url TEXT NOT NULL DEFAULT '',
  wahoo_oauth_base_url TEXT NOT NULL DEFAULT '',
  wahoo_client_id TEXT NOT NULL DEFAULT '',
  ridemodel_coefficients_file TEXT NOT NULL DEFAULT '',
  sync_initial_delay_seconds INTEGER NOT NULL DEFAULT 60,
  timezone TEXT NOT NULL DEFAULT 'Europe/Berlin'
);
INSERT INTO runtime_settings VALUES (1, 0, 86400, 1, 'every', 86400, 'https://api.pushover.net', 604800, 0, '', '', '', '', 60, 'Europe/Berlin');
CREATE TABLE runtime_basemap (
  position INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  style_url TEXT NOT NULL,
  style_url_dark TEXT NOT NULL,
  dark_cartography INTEGER NOT NULL CHECK (dark_cartography IN (0, 1))
);
INSERT INTO runtime_basemap VALUES (0, 'Streets', 'https://tiles.openfreemap.org/styles/bright', 'https://tiles.openfreemap.org/styles/dark', 0);
CREATE TABLE runtime_surface_region (
  position INTEGER PRIMARY KEY,
  region TEXT NOT NULL
);
CREATE TABLE runtime_target (
  position INTEGER PRIMARY KEY,
  target_id TEXT NOT NULL
);
CREATE TABLE runtime_source (
  position INTEGER PRIMARY KEY,
  provider TEXT NOT NULL,
  base_url TEXT NOT NULL
);
CREATE TABLE runtime_secret (
  name TEXT PRIMARY KEY,
  value BLOB NOT NULL,
  updated_at_unix INTEGER NOT NULL
);
CREATE TABLE task_runs (
  id INTEGER PRIMARY KEY,
  task TEXT NOT NULL,
  argument TEXT NOT NULL DEFAULT '',
  started_at_unix INTEGER NOT NULL,
  finished_at_unix INTEGER NOT NULL,
  outcome TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  reference TEXT NOT NULL DEFAULT '',
  trigger TEXT NOT NULL DEFAULT ''
);
CREATE TABLE stage_enrichment_failure (
  provider TEXT NOT NULL,
  route_id INTEGER NOT NULL,
  stage_order INTEGER NOT NULL,
  pass TEXT NOT NULL,
  reason TEXT NOT NULL,
  failed_at_unix INTEGER NOT NULL,
  PRIMARY KEY (provider, route_id, stage_order, pass)
);
CREATE TABLE alert_toggle (
  task TEXT NOT NULL,
  scope TEXT NOT NULL DEFAULT '',
  alert TEXT NOT NULL,
  enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
  updated_at_unix INTEGER NOT NULL,
  PRIMARY KEY (task, scope, alert)
);
CREATE TABLE task_schedule (
  task TEXT NOT NULL PRIMARY KEY,
  enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
  updated_at_unix INTEGER NOT NULL
);
INSERT INTO task_schedule VALUES ('sync:source', 1, 0);
INSERT INTO task_schedule VALUES ('sync:target', 1, 0);

CREATE UNIQUE INDEX oauth_transactions_state_digest_index ON oauth_transactions(state_digest);
CREATE INDEX sync_runs_reference_index ON sync_runs(reference);
CREATE INDEX task_runs_task_index ON task_runs(task, finished_at_unix DESC, id DESC);
CREATE INDEX task_runs_recency_index ON task_runs(finished_at_unix DESC, id DESC);
