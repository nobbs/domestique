# Domestique configuration specification

**Status:** accepted

This is a subordinate specification to [the service contract](service.md).
It defines the static configuration and secret-input contract. It does not
define a secret provider or a configuration API.

## Loading

The server uses Koanf to layer configuration in this order:

1. built-in defaults;
2. one TOML file; and
3. `DOMESTIQUE_`-prefixed environment variables.

The default TOML path is `/etc/domestique/config.toml`.
`DOMESTIQUE_CONFIG_FILE` selects another file path only; it does not override a
setting and is removed before Koanf loads the remaining environment. Nested
environment keys use a double underscore, so
`DOMESTIQUE_WAHOO__CLIENT_ID` maps to `wahoo.client_id`.

`DOMESTIQUE_IMAGE` is not a setting either. It is the image reference the
deploying host pinned, which compose already holds in order to start the
container, and it is taken out of the environment before Koanf reads the rest —
for the same reason the configuration selector is, since every remaining
`DOMESTIQUE_` variable is treated as a setting and an unknown one is fatal. Only the digest is taken from it,
so the service can report which image is running; the registry and repository in
front of the digest are deployment topology and never leave the host. Absent, as
it is in local development, the service reports no image digest at all.

Configuration is read once during startup. A configuration or static-secret
change takes effect only after a container restart. The server fails closed
before opening an HTTP listener when configuration is missing, malformed,
contains an unknown key, or fails validation.

## Non-secret configuration

The following is safe to commit:

```toml
[http]
listen_address = ":8080"
readiness_address = ":8081"

[access.cloudflare]
team_domain = "yourteam.cloudflareaccess.com"
application_aud = "the AUD tag of the Access application"
allowed_email = "you@example.com"

[state]
database_path = "/var/lib/domestique/state.db"
encryption_key_file = "/run/secrets/state_encryption_key"

[veloplanner]
base_url = "https://veloplanner.com"
email_file = "/run/secrets/veloplanner_email"
password_file = "/run/secrets/veloplanner_password"

[wahoo]
api_base_url = "https://api.sandbox.wahooligan.com"
oauth_base_url = "https://api.sandbox.wahooligan.com"
client_id = "replace-with-client-id"
client_secret_file = "/run/secrets/wahoo_client_secret"
redirect_url = "https://pi.example.ts.net/oauth/wahoo/callback"

[[wahoo.targets]]
id = "rider-a"

[[wahoo.targets]]
id = "rider-b"

[sync]
initial_delay = "1m"
interval = "1h"
max_deletions_per_target = 5
empty_source_deletion = "deny"

[notifications]
success_policy = "every"
digest_interval = "24h"

[notifications.pushover]
base_url = "https://api.pushover.net"
application_token_file = "/run/secrets/pushover_application_token"
user_key_file = "/run/secrets/pushover_user_key"

[webui]
tile_style_url = "https://tiles.openfreemap.org/styles/bright"
tile_style_url_dark = "https://tiles.openfreemap.org/styles/dark"

[surface]
regions = ["europe/germany/rheinland-pfalz", "europe/germany/hessen"]
rebuild_interval = "168h"
```

The endpoint examples are illustrative. The deployed values must match the
chosen Wahoo environment and the redirect URI registered for the application.

`webui.tile_style_url` is the MapLibre style the route map view loads. It must
be an absolute HTTPS URL. Unlike the service's own endpoints it may carry a
query string, because providers that require an API key put it there.

It is deliberately **not** a secret and is never handled as one: the browser must
know it, so it is served to the page and is visible to anyone who can reach the
UI. The default is a keyless provider, so a default deployment publishes no
credential and sends no account identity to the tile origin. An operator who
chooses a keyed provider is accepting that the key becomes visible to the UI's
single authorised user; self-hosted tiles avoid both that and the fact that the
browser reveals the area of a viewed route to the tile origin.

Changing this value changes the Content-Security-Policy the service sends, which
permits exactly the service's own origin and this one tile origin. A provider
that serves its style, tiles, sprites, and glyphs from more than one host is not
supported without widening that policy.

`webui.tile_style_url_dark` is the style the browser loads instead when it
reports a dark system colour scheme, so the map follows the same preference the
rest of the UI already follows in CSS. It is optional: an empty value leaves one
style in force under both schemes, which is what a provider publishing only one
requires.

When it is set it must be an absolute HTTPS URL **on the same origin** as
`webui.tile_style_url`, and configuration is rejected at startup when it is not.
That constraint keeps the guarantee above intact — one tile origin in the
Content-Security-Policy, and one third-party origin learning the area of a viewed
route. A dark style on a second origin would widen both, and is therefore a
deliberate revision of this contract rather than a setting.

Both styles are served to the page, which chooses between them; the service does
not resolve the colour scheme, because the preference belongs to the browser and
this response is cached for the session.

`surface.regions` names the OpenStreetMap extracts the **service** builds its
surface index from, so it can classify a stage's ground as asphalt, paving,
compacted, gravel, or unsurfaced. Each entry is a Geofabrik region path such as
`europe/germany/rheinland-pfalz`: lowercase path segments of letters, digits, and
single hyphens, and nothing else. The shape is validated at startup rather than
trusted, because a region becomes a URL under a fixed host, and a validated slug
can never introduce a host, a query, or a traversal.

The default is **no regions**, which switches surface classification off: nothing
is downloaded, no index is built, and stages carry no surface. An operator who
wants classification names the regions they actually ride. Naming more costs disk
and build time for map nobody will ever be matched against.

Unlike the tile style this is not a browser concern, and unlike the endpoint it
replaced it sends no route data anywhere: classification reads a local file. The
only outbound traffic is the scheduled download of each region's published
extract, which tells the extract host which regions this deployment cares about
and nothing about any route.

`surface.rebuild_interval` is how often the index is rebuilt. It must be a
positive Go duration and defaults to `168h`, one week — roughly the pace at which
a region's surface tagging changes enough to matter. A rebuild first fetches each
region's published checksum and stops there when every one is unchanged, so a
cadence faster than the upstream's own is cheap rather than wasteful.

The interval is time **between builds**, not time since this process started: the
service records when the last build finished and counts from there, so a
deployment restarted several times a day still rebuilds weekly rather than on
every start. A build that is already overdue when the process starts still waits
a few minutes, so a restart puts the service on its feet before it puts a
memory-hungry job behind it.

The index is written beside the state database, which is the one directory a
deployment is guaranteed to have made durable. It is named for the generation it
was built from, so a new build is written and opened beside the live one and the
superseded file is removed only once the new one is serving. A build holds
roughly half a gigabyte of heap and stages an extract of a few hundred megabytes
on disk, both of which are released when it finishes.

## Secret input

Every static secret has exactly one active input. It may use a TOML file path,
an overriding `*_FILE` environment value, or a direct environment value. File
inputs are preferred for Docker deployments; direct environment values support
a simple local setup.

| Canonical secret | TOML file-path field | Direct environment value | Environment file path |
| --- | --- | --- | --- |
| state encryption key | `state.encryption_key_file` | `DOMESTIQUE_STATE__ENCRYPTION_KEY` | `DOMESTIQUE_STATE__ENCRYPTION_KEY_FILE` |
| VeloPlanner email | `veloplanner.email_file` | `DOMESTIQUE_VELOPLANNER__EMAIL` | `DOMESTIQUE_VELOPLANNER__EMAIL_FILE` |
| VeloPlanner password | `veloplanner.password_file` | `DOMESTIQUE_VELOPLANNER__PASSWORD` | `DOMESTIQUE_VELOPLANNER__PASSWORD_FILE` |
| Wahoo client secret | `wahoo.client_secret_file` | `DOMESTIQUE_WAHOO__CLIENT_SECRET` | `DOMESTIQUE_WAHOO__CLIENT_SECRET_FILE` |
| Pushover application token | `notifications.pushover.application_token_file` | `DOMESTIQUE_NOTIFICATIONS__PUSHOVER__APPLICATION_TOKEN` | `DOMESTIQUE_NOTIFICATIONS__PUSHOVER__APPLICATION_TOKEN_FILE` |
| Pushover user key | `notifications.pushover.user_key_file` | `DOMESTIQUE_NOTIFICATIONS__PUSHOVER__USER_KEY` | `DOMESTIQUE_NOTIFICATIONS__PUSHOVER__USER_KEY_FILE` |

Literal secret fields are invalid in the TOML file. They are accepted only from
the documented direct environment variables. A `*_FILE` environment variable
overrides the corresponding TOML file path, but it must not accompany a direct
secret environment value.

A file secret must be an absolute path to a regular readable file. Text secrets
must be non-empty after one terminal line break is trimmed. The state key is a
base64url encoding of exactly 32 random bytes. The service reads every secret
once at startup, does not log a value or path, and clears direct secret
environment values from its own process environment after loading.

The application does not know which system created a file. Docker Secrets,
read-only bind mounts, and manually managed local files are equally valid.
A deployment tool such as `fnox` may provision files, but is not a runtime or
application dependency.

## Static fields

- `http.listen_address` is required. Docker maps the container port to the
  Tailnet host's `127.0.0.1` only; the application must not use the address itself as
  evidence of Tailnet identity.
- `http.readiness_address` is optional and defaults to `:8081`. It is the second
  listener, and it serves the readiness probe alone. It is validated on the same
  terms as `http.listen_address`, and its port must differ from that listener's:
  one port serving both would put readiness behind Tailscale Serve and the
  tunnel, which is the surface the probe exists to stay off. An existing
  configuration file needs no change.
- `access.cloudflare` is required in full: `team_domain`, `application_aud`, and
  `allowed_email` must all be present. It is the only gate the service has, so a
  missing or partly filled section is a startup error rather than a service that
  answers every request with a 401. None of the three is a secret — the team
  domain and the audience tag are public identifiers, and verification uses
  Cloudflare's published signing keys — so they are ordinary configuration
  values rather than secret files.
- `access.cloudflare.allowed_email` is the sole identity allowed to use normal or
  OAuth endpoints, and is the principal every authenticated request resolves to.
  The configured spelling is what reaches the OAuth service, so a flow begun by
  one request stays consumable by the next even if Access varies the case of the
  asserted address.
  `application_aud` is what confines an assertion to this one application:
  without it, a token minted for any other application of the same Cloudflare
  team would verify against the same key.
- `state.database_path` is required and must reside on the persistent Docker
  volume.
- `veloplanner.base_url` is a required absolute HTTPS URL. The authenticated
  VeloPlanner user ID is discovered, not configured. It also reaches the browser
  through `GET /v1/webui/config` as the base of a stage's link back to its
  source route, so pointing it at a different deployment moves both the
  inventory it reads and the link it offers.
- `wahoo.api_base_url` and `wahoo.oauth_base_url` are required absolute HTTPS
  URLs. `wahoo.client_id` is non-secret.
- `wahoo.redirect_url` is required HTTPS and must exactly match Wahoo's
  registered callback URI, ending in `/oauth/wahoo/callback`.
- `wahoo.targets` contains one or two entries. Each `id` is unique and stable
  across deployments; it is a configured slot, not a Wahoo user identifier.
- `sync.initial_delay` is positive. `sync.interval` equals one hour.
  `sync.max_deletions_per_target` equals `5`.
- `notifications.pushover.base_url` is an absolute HTTPS origin without a path,
  and defaults to Pushover's own, so a deployment that says nothing about it
  notifies Pushover. It is a setting rather than a compiled-in constant for the same
  reason the provider endpoints above are: a development or demo environment has
  to be able to point it at an address that goes nowhere, and the alternative is
  such an environment reaching the real service with a placeholder token. The
  path is rejected at startup rather than by the notifier, so the failure names
  the setting.
- `sync.empty_source_deletion` is `deny` or `allow`, defaulting to `deny`.
  The operator sets `allow` only for a deliberate final-library deletion and
  returns it to `deny` immediately afterward.
- `notifications.success_policy` is `every`, `quiet`, or `digest`, defaulting to
  `every`, so a deployment that says nothing about it keeps the per-run message
  it has always sent. It governs routine success alone; it can never suppress a
  failure, a blocked run, or the first success that ends one.
  [The sync lifecycle specification](sync-lifecycle.md#notifications) states what
  each policy delivers.
- `notifications.digest_interval` is a positive duration of at most seven days
  and is read only by the `digest` policy. It defaults to `24h`. The upper bound
  is the reach of the recorded run history: a longer period would silently total
  a window whose earliest runs have already been pruned.

The decoder rejects unknown fields, invalid URLs or durations, duplicate target
IDs, a target count outside one through two, invalid callback paths, unreadable secret files,
and ambiguous secret inputs before it opens an HTTP listener.

## Runtime state

Dynamic Wahoo refresh tokens are not configuration. They are encrypted in
SQLite using the supplied state key; access tokens remain in memory only. The
state database is intentionally not backed up. Changing the state key makes the
existing encrypted state unreadable, and key rotation is not a feature.

## Diagnostics and exclusions

Startup and `GET /v1/status` may report non-sensitive configuration facts:
the selected Wahoo endpoint, target slot labels, database readiness, configured
sync interval, and whether a target needs OAuth authorisation. They must not
report secret paths, secret values, client-secret material, tokens, or
VeloPlanner account identifiers.

Outside the contract:

- a secret-manager SDK, provider URI, provider credential, or secret reference
  syntax in Go or TOML;
- a native secret-resolution SDK or cgo;
- field-level environment overrides for non-secret configuration;
- secret rotation without a controlled migration; and
- a third or dynamically created Wahoo target.
