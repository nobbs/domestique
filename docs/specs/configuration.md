# Domestique configuration specification

**Status:** accepted v1 design

This is a subordinate specification to [the service contract](service.md).
It defines the static configuration and secret-file contract. It does not define
a secret manager, environment-variable override system, or a configuration API.

## Loading

The server reads one TOML file from `DOMESTIQUE_CONFIG_FILE`; the default
container path is `/etc/domestique/config.toml`. This environment variable
selects a file only. It never supplies a setting or a secret value.

Configuration is read once during startup. A configuration or static-secret
change takes effect only after a container restart. The server fails closed
before opening an HTTP listener when the file is missing, malformed, contains an
unknown key, or fails validation.

## Example

The following is a safe-to-commit example. Every value ending in `_file` is a
path inside the container, not a secret value or a provider-specific reference.

```toml
[http]
listen_address = ":8080"

[access]
tailnet_user_login = "you@example.ts.net"

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
application_token_file = "/run/secrets/pushover_application_token"
user_key_file = "/run/secrets/pushover_user_key"
```

The endpoint examples are illustrative. The deployed values must match the
chosen Wahoo environment and the redirect URI registered for the application.

## Fields

### HTTP and access

- `http.listen_address` is required. In Docker, the runtime maps this
  container port to the Pi's `127.0.0.1` only. The application must not rely on
  the address itself for Tailnet identity.
- `access.tailnet_user_login` is required. It is the sole authenticated
  Tailnet login permitted to reach normal and OAuth endpoints.
- Tailscale Serve is deployment configuration, not an application setting. It
  must be the only remote ingress and must remove untrusted forwarded identity
  headers before forwarding the authenticated identity.

### State

- `state.database_path` is required and must reside on the persistent Docker
  volume.
- `state.encryption_key_file` is required. Its file content is a base64url
  encoding of exactly 32 random bytes; surrounding ASCII whitespace is ignored.
- The database is not a backup. Changing the encryption key makes existing
  OAuth state and refresh tokens unreadable. Key rotation is not a v1 feature.

### VeloPlanner

- `veloplanner.base_url` is required and must be an absolute HTTPS URL.
- `veloplanner.email_file` and `veloplanner.password_file` are required.
  They provide the one private source account's login values.
- The authenticated VeloPlanner user ID is discovered by the source adapter. It
  is not configured manually.

### Wahoo

- `wahoo.api_base_url` and `wahoo.oauth_base_url` are required absolute HTTPS
  URLs. They support the approved sandbox now and a later Wahoo environment
  without baking either host into the service contract.
- `wahoo.client_id` is required and non-secret.
- `wahoo.client_secret_file` is required.
- `wahoo.redirect_url` is required, must be HTTPS, and must exactly match the
  registered Wahoo callback URI. It must end in `/oauth/wahoo/callback`.
- `wahoo.targets` contains exactly two entries. Each `id` is required, unique,
  and stable across deployments. It is a configuration slot, not a Wahoo user
  identifier. OAuth associates each slot with a distinct Wahoo account.

### Sync

- `sync.initial_delay` is required and must be positive. It is the delay before
  the first healthy-start sync.
- `sync.interval` is required and must equal one hour in v1.
- `sync.max_deletions_per_target` is required and must equal `5` in v1.
  The limit applies independently to each Wahoo target.
- `sync.empty_source_deletion` must be either `deny` or `allow`; its default
  is `deny`. `deny` blocks deletion when a previously populated source
  inventory becomes empty. The operator sets `allow` in a static config
  deployment only when deliberately removing the final source routes, then
  returns it to `deny`.

### Pushover

- `notifications.pushover.application_token_file` and
  `notifications.pushover.user_key_file` are required.
- Their files authorise delivery only; message content is governed by the
  notification privacy rules in the service specification.

## Secret-file rules

Secret-file fields are the only way static secrets enter the process. A
secret-file value:

- must be an absolute path to a regular, readable file;
- is read once at startup and is never logged;
- must be non-empty after trimming a single terminal line break; and
- must not be an `op://`, `env:`, URL, command, template expression, or
  reference to another secret.

Docker Secrets are the preferred delivery mechanism. Bind-mounted read-only
files are equally valid. A deployment tool such as `fnox` may create either;
the service does not include, invoke, or configure it.

The state encryption key is the only binary secret and follows its own encoding
rule above. Other secret files are UTF-8 text. A secret that cannot be decoded
or validated causes startup to fail without identifying its value.

## Validation and diagnostics

The configuration decoder must reject unknown fields rather than silently
ignoring typos. It validates all paths, URL schemes, durations, target count,
target uniqueness, secret-file presence, and the redirect callback suffix before
the service starts.

Startup and `GET /v1/status` may report non-sensitive configuration facts:
the selected Wahoo endpoint, target slot labels, database readiness, configured
sync interval, and whether a target still needs OAuth authorisation. They must
not report secret paths, secret values, client secret material, tokens, or
VeloPlanner account identifiers.

## Explicit exclusions

The following are outside the v1 configuration contract:

- loading values from 1Password, Vault, cloud secret stores, or `fnox`;
- field-level environment variables;
- command-line flags;
- secret rotation without a controlled migration;
- a third or dynamically created Wahoo target; and
- automatic approval of a destructive empty-source inventory.
