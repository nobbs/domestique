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

[notifications.pushover]
base_url = "https://api.pushover.net"
application_token_file = "/run/secrets/pushover_application_token"
user_key_file = "/run/secrets/pushover_user_key"

[webui]
tile_style_url = "https://tiles.openfreemap.org/styles/bright"
tile_style_url_dark = "https://tiles.openfreemap.org/styles/dark"

[surface]
overpass_url = "https://overpass-api.de/api/interpreter"
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

`surface.overpass_url` is the OpenStreetMap Overpass endpoint the **service**
asks which ways lie along a stage, in order to classify its ground as asphalt,
paving, compacted, gravel, or unsurfaced. It must be an absolute HTTPS URL
without credentials, query, or fragment, and the default is the public instance,
which needs no account and no key.

Unlike the tile style this is not a browser concern: the service itself sends a
simplified form of each stage's shape to that endpoint, so the endpoint learns
where the operator's routes go. Nothing else is sent — no title, no identity, no
account reference — and each stage's geometry is asked about once, because the
answer is cached until the stage's content hash changes.

Setting it to an empty string disables the lookup, and stages then carry no
surface. Pointing it at a self-hosted Overpass instance keeps the shapes inside
the operator's own infrastructure.

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
