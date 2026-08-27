# Domestique configuration specification

**Status:** accepted

This is a subordinate specification to [the service contract](service.md).
It defines the static configuration and secret-input contract, and the runtime
settings the service holds in its own state database. It does not define a
secret provider, and the HTTP shape of the settings endpoints belongs to the
service contract rather than to this document. What is defined here is which
settings exist, which of the two homes each one lives in, and what every one of
them has to satisfy.

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

The file is read once during startup. A change to it, or to a static secret,
takes effect only after a container restart. The server fails closed before
opening an HTTP listener when configuration is missing, malformed, contains an
unknown key, or fails validation.

The file is not everything the service is configured by. The settings an
operator has a reason to change while the service is running live in the state
database instead, are edited from the browser UI, and are in force from the next
request and the next run — with no restart, and without shell access to the
host. [Runtime settings](#runtime-settings) below states which those are.

The two homes do not overlap: every setting has exactly one, and a key that
moved to the database is refused by name when it is still written in the file.
A deployment upgrading into the split therefore finds out at startup, rather
than discovering months later that the file it kept editing was being ignored.

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

[komoot]
base_url = "https://api.komoot.de"
email_file = "/run/secrets/komoot_email"
password_file = "/run/secrets/komoot_password"

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

[notifications.pushover]
application_token_file = "/run/secrets/pushover_application_token"
user_key_file = "/run/secrets/pushover_user_key"

[ridemodel]
coefficients_file = "/etc/domestique/ridemodel.toml"
```

The endpoint examples are illustrative. The deployed values must match the
chosen Wahoo environment and the redirect URI registered for the application.

What the file no longer carries is as much a part of its contract as what it
does. There is no basemap list, no surface region or rebuild cadence, no
staleness bound, no empty-source deletion gate, and no notification switch,
success policy, digest period, or Pushover origin. Every one of those is a
runtime setting, and the decoder refuses it here by name.

`ridemodel.coefficients_file` names the hybrid profile the offline benchmark
tooling ([#239](https://github.com/nobbs/domestique/issues/239)) emits:

```toml
calibration_cutoff = "2025-08-01"
mass_kg = 90.0
power_watts = 180.0
cda_m2 = 0.45
crr = 0.012
seconds_per_km = 145.3578
seconds_per_ascent_m = 3.2190
evaluated_rides = 42
bias_percent = -1.20
mae_percent = 6.80
p90_percent = 14.10
training_window_months = 12
```

`evaluated_rides`, `bias_percent`, `mae_percent` and `p90_percent` are the
profile's measured unseen-route error. They are optional — a file written
before [#217](https://github.com/nobbs/domestique/issues/217) added them
still loads — and, when present, are served alongside a stage's predicted
moving time so the browser can qualify the estimate rather than present it
as a bare number. `dev/fitter -recalibrate` prints them as part of its
copy-ready profile, computed from the same evaluation pass that prints the
human-readable `validation:` line beside them.

They describe the *procedure* that produced the profile rather than these
exact coefficients, and
[#251](https://github.com/nobbs/domestique/issues/251) is why the
distinction is worth drawing. The fitter walks a monthly origin across the
corpus; each fold calibrates on the training window behind its origin and is
scored on the unseen routes of the month after it, so no fold is ever
measured against a ride it was fitted on. Those per-fold errors are pooled
into the four fields. The coefficients themselves are then fit over the
newest window — including the rides the last folds scored — because a
profile withholding the most recent months is precisely the stale profile
the exercise exists to avoid. The single chronological split this replaced
drew its evaluation rides from across the whole corpus, which let a profile
that was badly wrong about the present still report a flattering average.

`training_window_months` is how far back that fit was allowed to reach;
`calibration_cutoff` is where it stopped, so the two together say exactly
which rides produced the file. It is optional and defaults to zero, meaning
a file written before the window existed and fitted over all history. The
window is a bound rather than a guillotine: the fitter reaches further back
when a window holds too few rides to fit from, so a quiet season narrows the
training set instead of emptying it. Twelve months is the default because it
is the shortest span covering a full year of weather and daylight —
measured on the operator's own corpus, accuracy is flat from six months to
all history, so the window buys no error today and exists so a profile can
follow a rider whose form actually moves.

The predicted moving time is a weighted average of fixed physics —
independently defensible assumptions, not values a corpus this small can
reliably identify — and a two-parameter route correction, `seconds_per_km` and
`seconds_per_ascent_m`, calibrated against the route-disjoint rolling-origin
protocol described above. The physics half carries a quarter of the blend and
the route correction the remaining three quarters, because the two are not
peers: the route half is fitted against whole moving times and so is already a
complete estimate, while the physics half is a fixed prior nobody calibrates.
Weighting them equally pulls a calibrated answer halfway toward an
uncalibrated one, which measurably worsened error on routes the fit had never
seen. The physics share is not zero because the linear half cannot tell 200 m
of climbing up a single wall from 200 m spread over rolling ground, and only
the physics half can. Mass, power, drag area and rolling resistance stay
per-file because they vary with the rider and the bike; the blend weight,
drivetrain efficiency, standard air density, and the descent cap do
not appear in the file at all — they are fixed constants a code upgrade can
change, which is why an upgrade to them still invalidates a cached prediction
even when the operator's file is unchanged. The service reads the file once at
startup and computes a predicted moving time per stage from it, in the manner
surface classification is computed — see [the implementation architecture
specification's tier ownership
section](implementation-architecture.md#predicted-moving-time-is-a-deliberate-departure)
for why that placement is correct despite depending on rider mass and power.

The default is **no file**, which switches prediction off entirely: no
coefficient is loaded, no stage anywhere carries a predicted time, and the
routes endpoints omit the field rather than reporting zero. When set, the path
must be absolute; the file itself is read and validated for physical
plausibility — not merely parsed — at startup, and a missing, malformed, or
implausible file is a startup failure rather than a silent fallback to no
prediction.

Unlike every other file this specification names, `coefficients_file` is
**deliberately not a secret**. It carries physical constants and a rides-
calibrated correction, and no route data, no credential, and no personal
information, so it is a plain path like `surface.regions`' extracts rather
than a `*_file` secret input, and it is safe to commit alongside the rest of
this example.

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
| Komoot email | `komoot.email_file` | `DOMESTIQUE_KOMOOT__EMAIL` | `DOMESTIQUE_KOMOOT__EMAIL_FILE` |
| Komoot password | `komoot.password_file` | `DOMESTIQUE_KOMOOT__PASSWORD` | `DOMESTIQUE_KOMOOT__PASSWORD_FILE` |
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

At least one of `[veloplanner]` and `[komoot]` must be configured; a
configuration with neither is refused at startup. Presence is asked of the
fully merged configuration, TOML file and environment overrides alike, so a
source named only through its `DOMESTIQUE_VELOPLANNER__*` or
`DOMESTIQUE_KOMOOT__*` environment variables — with no matching section in the
file at all — still counts. Each is an independent named section rather than
an array entry, because each provider's credential shape belongs to that
provider — VeloPlanner and Komoot happen to need the same base URL plus email
and password today, but nothing in this schema assumes a third source would.
A configuration file cannot define the same section twice: TOML itself
rejects a redefined table as a parse error naming it, before this package's
own validation runs.

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
- When `[veloplanner]` is configured, `veloplanner.base_url` is a required
  HTTPS origin without a path, matching what the adapter itself requires — a
  value with a path is refused here, at config load, rather than later when
  the client is constructed. The authenticated VeloPlanner user ID is
  discovered, not configured. It also reaches the browser through
  `GET /v1/webui/config`, keyed by provider, as the base of a stage's link
  back to its source route, so pointing it at a different deployment moves
  both the inventory it reads and the link it offers.
- When `[komoot]` is configured, `komoot.base_url` is a required HTTPS origin
  without a path, on the same terms as `veloplanner.base_url`. The
  authenticated Komoot user ID is discovered from the account's email and
  password, not configured.
- `wahoo.api_base_url` and `wahoo.oauth_base_url` are required absolute HTTPS
  URLs. `wahoo.client_id` is non-secret.
- `wahoo.redirect_url` is required HTTPS and must exactly match Wahoo's
  registered callback URI, ending in `/oauth/wahoo/callback`.
- `wahoo.targets` contains one or two entries. Each `id` is unique and stable
  across deployments; it is a configured slot, not a Wahoo user identifier.
- `sync.initial_delay` is positive. It stays a file setting because it is
  consumed once, at the start it delays: a value edited later would change
  nothing until the restart an editable setting exists to avoid.
- The interval between scheduled runs and the per-target deletion limit are not
  settings in either home. They are constants — one hour and five — stated in
  the code that enforces them. Both were file keys that accepted exactly one
  value, which is a dial in name only, and a limit that exists to stop a runaway
  deletion is not one an operator has a use for.

The decoder rejects unknown fields, invalid URLs or durations, duplicate target
IDs, a target count outside one through two, invalid callback paths, unreadable secret files,
and ambiguous secret inputs before it opens an HTTP listener. An unknown field
includes every key that moved into the database, so a file still carrying one
fails startup with the key named rather than having it quietly ignored.

## Runtime settings

The settings below are held in the state database rather than in the file. They
are read once at startup into a snapshot, replaced whole when the operator saves
them from the browser UI, and in force from the next request and the next run.

They live there because each one is something somebody has a reason to change on
a service that is already running: the deletion gate opened for one deliberate
run and closed again afterwards, a basemap added to the picker, notifications
quieted for a week, a region added because a holiday moved where somebody rides.
As file keys, every one of those changes cost an edit on the host and a
container restart. The restart is not the expensive part — needing shell access
on the host to reach a switch the UI could offer is.

Every reader takes a copy of the snapshot for the length of one run, one
request, or one notification, so an edit lands between two units of work rather
than inside one. A run that started before a save finishes on the settings it
started with.

A save writes every setting at once. There is no partial write, because the form
holds every field and a body naming only some of them would leave the rest at
whatever the caller assumed they were.

The rules below are checked in one place by both edges: the write path checks
what the browser submitted, and startup checks what the database returned. A
value that is served is therefore one that would have passed the check that let
it be written — including in a database somebody edited by hand, which fails
startup naming the setting rather than being served. Checking normalises as it
goes, so names and URLs are stored trimmed. The names used below are the ones
the service's own messages use; [the service contract](service.md) documents the
JSON names the same settings carry on the wire.

A deployment that has never opened the settings page runs on the seeded
defaults, which are the ones each rule names below: deletion denied, a 24-hour
staleness bound, notifications on with a message per successful run, a 24-hour
digest period, Pushover's own origin, a single keyless basemap, no surface
regions, and a weekly index rebuild.

### Synchronisation

The **empty-source deletion gate** decides whether a source that reported an
empty library may delete the last owned routes a target holds. It denies by
default, and allowing it stays allowed until it is turned off again: nothing
expires it, because a gate that closed itself is one an operator cannot rely on
still being open for the run they opened it for. The UI asks before turning it
on and says as much. It is the setting that most wanted this split — it exists
to be open for one deliberate final-library deletion and shut immediately
afterwards, which as a file key meant editing the host's file and restarting the
container twice around a single run.

`sync.stale_after` bounds how long the trusted source inventory may go without a
successful refresh before `GET /v1/status` reports it as stale and a
notification goes out. It is at least one second and defaults to 24 hours. The
response reports age in whole seconds, so a sub-second bound is rejected rather
than silently truncating to a zero maximum age that would flag every service as
permanently stale.
[The sync lifecycle specification](sync-lifecycle.md#trusted-inventory-freshness)
states what is measured and how the notification is rate-limited.

### Notifications

`notifications.enabled` is the switch for the whole channel, and it is on by
default. Off is not merely a quieter success policy: it suppresses a failure, a
blocked run, and a stale inventory as surely as it suppresses a routine success.
Every surface offering it has to say so, because an operator reading it as
"fewer messages" would be turning off precisely the messages notifications were
installed for.

`notifications.success_policy` is `every`, `quiet`, or `digest`, defaulting to
`every`, so a deployment that has changed nothing keeps the per-run message it
has always sent. It governs routine success alone: it can never suppress a
failure, a blocked run, or the first success that ends one — only the switch
above can.
[The sync lifecycle specification](sync-lifecycle.md#notifications) states what
each policy delivers.

`notifications.digest_interval` is positive, at most seven days, and defaults to
24 hours. The upper bound is the reach of the recorded run history: a longer
period would silently total a window whose earliest runs have already been
pruned. It is checked whatever the policy is, because a setting an operator will
one day switch to should not turn out to have been invalid all along at the
moment they switch.

`notifications.pushover.base_url` is an absolute HTTPS origin without a path,
defaulting to Pushover's own, so a deployment that says nothing about it
notifies Pushover. It is a setting rather than a compiled-in constant for the
same reason the provider endpoints are: a development or demo environment has to
be able to point it at an address that goes nowhere, and the alternative is such
an environment reaching the real service with a placeholder token. The path is
rejected by the setting rather than by the notifier, so the failure names the
setting. The credentials sent to that origin stay in the file, because a token
is a secret and secrets are not edited through a browser.

### Basemaps

`webui.basemaps` is the list of cartographies the route map view offers, in the
order they are offered. **At least one entry is required**, and the default is a
single keyless one, so a deployment that configures nothing still gets a map.
The first entry is what a browser that has never chosen one loads. The page
shows a picker only when the list holds more than one, because one cartography
is not a choice.

Each entry carries:

- `name` — the label the picker shows, and the identity a browser remembers a
  reader's choice by. Required, non-empty after trimming, and unique across the
  list; two entries sharing a name would make label and identity disagree.
  Renaming an entry therefore forgets any choice remembered under the old name.
- `style_url` — the MapLibre style to load. It must be an absolute HTTPS URL.
  Unlike the service's own endpoints it may carry a query string, because
  providers that require an API key put it there.
- `style_url_dark` — optional; the style the browser loads instead when it
  reports a dark system colour scheme, so the map follows the same preference the
  rest of the UI already follows in CSS. Omitting it leaves one style in force
  under both schemes, which is what a provider publishing only one requires. When
  set it must be an absolute HTTPS URL **on the same origin** as that entry's
  `style_url`, and a list that breaks the rule is refused on the save that
  submits it.
- `dark_cartography` — optional; `true` marks ground that is dark whatever the
  system asks for, which is what satellite imagery is. Anything the page paints
  over the map reads this rather than the colour scheme, because a route drawn in
  the dark-ground ink over light cartography — or the reverse — is the one that
  cannot be seen. It contradicts `style_url_dark`: a provider publishing a dark
  twin has light cartography to switch away from, and setting both is refused.

A style URL is deliberately **not** a secret and is never handled as one: the
browser must know it, so it is served to the page and is visible to anyone who
can reach the UI. The default is a keyless provider, so a default deployment
publishes no credential and sends no account identity to the tile origin. An
operator who chooses a keyed provider is accepting that the key becomes visible
to the UI's single authorised user; self-hosted tiles avoid both that and the
fact that the browser reveals the area of a viewed route — or, once the map's
locate button is pressed, the area around the reader's own live position — to
the tile origin. The raw position itself is never one of the values revealed:
it moves the camera and goes no further.

The list changes the Content-Security-Policy the service sends, which permits
the service's own origin plus **the origin of every configured entry**, sorted
and deduplicated. The header is composed per response from the live list, so a
basemap added on the settings page is permitted by the next response rather than
at the next restart — which is the whole reason the header is built there rather
than once at startup. Because an entry's dark style is held to that entry's own
origin, the list of origins is as long as the number of distinct providers
offered and no longer.

Naming a second provider is a deliberate widening, and it is worth being exact
about what it costs. The policy says which origins the page *may* reach; it does
not make the page reach them. Only the basemap on screen is ever requested, so
what a single provider learns — the area of a viewed route — is unchanged. What
grew is the set of providers that could be asked, and that set is exactly the one
the operator wrote down.

A provider that serves its style, tiles, sprites, and glyphs from more than one
host is still not supported, because one entry admits one origin.

The whole list is served to the page, which chooses within it; the service
resolves neither the reader's choice nor the colour scheme, because both belong
to the browser and this response is cached for the session. Saving the settings
discards that cached copy in the browser that saved them, so the picker offers
the edited list without a reload.

Weather forecasts go further than a keyless basemap: there is no key
configuration could carry in the first place. Open-Meteo's free forecast
endpoint needs none, so `internal/openmeteo`'s options hold no credential
field at all — unlike `internal/pushover`'s, which carries an application
token and a user key because Pushover requires them. The asymmetry with the
basemap list above is a decision, not an oversight — a basemap's key is the
provider's choice to require one; Open-Meteo's is not to.

### Surface classification

`surface.regions` names the OpenStreetMap extracts the **service** builds its
surface index from, so it can classify a stage's ground as asphalt, paving,
compacted, gravel, or unsurfaced. Each entry is a Geofabrik region path such as
`europe/germany/rheinland-pfalz`: lowercase path segments of letters, digits, and
single hyphens, and nothing else. The shape is validated where it is entered
rather than trusted, because a region becomes a URL under a fixed host, and a
validated slug can never introduce a host, a query, or a traversal. A blank line
and a repeat are dropped rather than refused: a repeat asks for nothing that is
not already being done, and doing it once is the useful reading of a typo.

The default is **no regions**, which switches surface classification off: nothing
is downloaded, no index is built, and stages carry no surface. An operator who
wants classification names the regions they actually ride. Naming more costs disk
and build time for map nobody will ever be matched against.

Unlike the tile style this is not a browser concern, and unlike the endpoint it
replaced it sends no route data anywhere: classification reads a local file. The
only outbound traffic is the scheduled download of each region's published
extract, which tells the extract host which regions this deployment cares about
and nothing about any route.

`surface.rebuild_interval` is how often the index is rebuilt. It must be
positive and defaults to one week — roughly the pace at which a region's surface
tagging changes enough to matter. It is required whether or not a region is
named, because the rebuild schedule is created either way and simply builds
nothing when there are no regions; a cadence of zero would be a schedule that
could not be started, and the operator naming their first region would find that
out at the restart this setting exists to avoid. A rebuild first fetches each
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

## Migrating an existing deployment

A deployment already running today's single-source `[veloplanner]` section
needs no change: it continues to be read exactly as before, and the service
keeps configuring VeloPlanner alone if that remains the only section present.
Enabling Komoot on a running deployment is purely additive — write a new
`[komoot]` section and provision its two secret files, and the matching
Docker secret entries in the deployment's compose file, without touching the
existing `[veloplanner]` section or its secrets.

## Runtime state

Dynamic Wahoo refresh tokens are not configuration. They are encrypted in
SQLite using the supplied state key; access tokens remain in memory only. The
runtime settings above share that database, stored in the clear because none of
them is a secret, and share its fate: a lost database returns every one of them
to the default it was seeded with. The
state database is intentionally not backed up. Changing the state key makes the
existing encrypted state unreadable, and key rotation is not a feature.

## Diagnostics and exclusions

Startup and `GET /v1/status` may report non-sensitive configuration facts:
the selected Wahoo endpoint, target slot labels, database readiness, the sync
interval, and whether a target needs OAuth authorisation. They must not
report secret paths, secret values, client-secret material, tokens, or any
configured source's account identifiers.

Outside the contract:

- a secret-manager SDK, provider URI, provider credential, or secret reference
  syntax in Go or TOML;
- a native secret-resolution SDK or cgo;
- field-level environment overrides for non-secret configuration;
- a secret, a listener address, or a provider endpoint edited through the
  settings surface — what an operator may change while the service runs is the
  list above and nothing else;
- secret rotation without a controlled migration; and
- a third or dynamically created Wahoo target.
