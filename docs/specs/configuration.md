# Domestique configuration specification

**Status:** accepted

This specification is subordinate to [the service contract](service.md). It
defines the static configuration and secret-input contract, and the runtime
settings the service holds in its own state database. It does not define a
secret provider, and the HTTP shape of the settings endpoints belongs to the
service contract. What is defined here is which settings exist, which of the two
homes each one lives in, and what every one of them has to satisfy.

## Loading

The server uses Koanf to layer configuration in this order:

1. built-in defaults;
2. one TOML file; and
3. `DOMESTIQUE_`-prefixed environment variables.

The default TOML path is `/etc/domestique/config.toml`.
`DOMESTIQUE_CONFIG_FILE` selects another file path only; it does not override a
setting and is removed before Koanf loads the remaining environment. Nested
environment keys use a double underscore, so
`DOMESTIQUE_STATE__DATABASE_PATH` maps to `state.database_path`.

`DOMESTIQUE_IMAGE` is not a setting. It is the image reference the deploying
host pinned, and it is taken out of the environment before Koanf reads the rest:
every remaining `DOMESTIQUE_` variable is treated as a setting, and an unknown
one is fatal. Only the digest is taken from it, so the service can report which
image is running. The registry and repository in front of the digest are
deployment topology and never leave the host. Absent, as in local development,
the service reports no image digest.

The file is read once during startup. A change to it, or to a static secret,
takes effect only after a container restart. The server fails closed before
opening an HTTP listener when configuration is missing, malformed, contains an
unknown key, or fails validation.

The file is not everything the service is configured by. The settings an
operator has a reason to change while the service is running live in the state
database instead, are edited from the browser UI, and are in force from the next
request and the next run, with no restart and without shell access to the host.
[Runtime settings](#runtime-settings) below states which those are.

The two homes do not overlap: every setting has exactly one, and a key that
belongs in the database is refused by name when it is written in the file.

## Non-secret configuration

The file is the whole of what the host hands the process, and this is all of it:

```toml
[http]
listen_address = ":8080"
readiness_address = ":8081"
browser_origin_url = "https://domestique.example.com"

[auth.auth0]
domain = "yourtenant.eu.auth0.com"
client_id = "the application's client ID"
client_secret_file = "/run/secrets/auth0_client_secret"

[state]
database_path = "/var/lib/domestique/state.db"
encryption_key_file = "/run/secrets/state_encryption_key"

[log]
level = "info"
```

What the file does not carry is as much a part of its contract as what it does.
There is no source and no source credential, no Wahoo application and no target
slot, no notification credential, no ride model coefficient, and no delay before the first
run — nor a basemap list, a surface region or rebuild cadence, a staleness
bound, an empty-source deletion gate, or a notification switch or Pushover
origin. Every one of those is a runtime setting, and the decoder refuses it here
by name.

### Timezone

`timezone` is an IANA zone name this service can load, defaulting to
`Europe/Berlin`. It is what a scheduled time of day means and what hour a
forecast describes — one zone for the whole service, because a run happens once
and has to happen at a time somebody chose.

A zone that cannot be loaded is refused where it is entered and at startup: a
calendar schedule reading it would have no answer to when it is next due. The
zone database travels inside the binary, so the choice does not depend on what
the runtime image carries.

## Secret input

The file names exactly two secrets. Each has one active input: a TOML
file path, an overriding `*_FILE` environment value, or a direct environment
value. The file input is preferred for Docker deployments; the direct
environment value supports a simple local setup.

| Canonical secret | TOML file-path field | Direct environment value | Environment file path |
| --- | --- | --- | --- |
| state encryption key | `state.encryption_key_file` | `DOMESTIQUE_STATE__ENCRYPTION_KEY` | `DOMESTIQUE_STATE__ENCRYPTION_KEY_FILE` |
| Auth0 client secret | `auth.auth0.client_secret_file` | `DOMESTIQUE_AUTH__AUTH0__CLIENT_SECRET` | `DOMESTIQUE_AUTH__AUTH0__CLIENT_SECRET_FILE` |

A literal `state.encryption_key` or `auth.auth0.client_secret` is invalid in
the TOML file. Each is accepted only from its documented direct environment
variable. A `*_FILE` environment variable overrides the matching TOML file
path, but it must not accompany the direct value.

A file secret must be an absolute path to a regular readable file, non-empty
after one terminal line break is trimmed. The state encryption key is
additionally validated as a base64url encoding of exactly 32 random bytes;
the Auth0 client secret carries no such shape requirement, being opaque to
this service and checked only by Auth0 on exchange. The service reads each
once at startup, does not log the value or the path, and clears the direct
secret environment value from its own process environment after loading.

Every other credential the service holds — the Wahoo client secret, each
source's account, and the Pushover pair — is entered on the settings page and
kept encrypted under the state key.
[Credentials](#credentials) below states what that means for a deployment.
The Auth0 client secret is not among them: it gates the settings page itself,
the same reason `http.browser_origin_url` is a file setting rather than a
runtime one, so it cannot be a settings-page credential.

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
  terms as `http.listen_address`, and its port must differ from that listener's.
  One port serving both would put readiness behind the reverse proxy along with
  everything else.
- `log.level` is optional and defaults to `info`; the accepted values are
  `debug`, `info`, `warn` and `error`. The service writes one JSON object per
  line to stderr. Successful requests are not logged; a failed answer is
  reported at `error`, a slow one at `warn`, and a refused one at `debug`,
  which the default level does not admit. Each carries its method, status,
  `duration_ms` in milliseconds and the path cut to its first segment, or its
  first two under `/v1`, `/auth`, `/oauth`, `/settings` and `/admin`; never a
  route ID, query string or subject. Like every other field it may be set as
  `DOMESTIQUE_LOG__LEVEL`, which is the intended way to raise it for one
  restart.
- `http.browser_origin_url` is required, and must be an absolute HTTPS origin
  with no path. It is the address a browser reaches this service at, behind the
  reverse proxy.

  It is the one origin a state-changing request may come from. A request to a
  sync trigger, the schedule switch, a route reprocess, sign-in start, or
  sign-out that names any other origin, or none, is refused.

  The Wahoo callback and the Auth0 callback are both derived from it rather than
  configured beside it: the redirect URLs are this service's own
  `/oauth/wahoo/callback` and `/auth/callback` on this origin, and each must
  match the callback registered with its own provider. Neither path is an
  operator's choice; this binary serves exactly those two.

  It is a file setting rather than a runtime one. It is deployment topology,
  fixed by where the container is published, it gates the write path the
  settings page itself uses, and a wrong value edited through that page would
  lock the operator out of the page that could correct it.
- `auth.auth0` is required in full: `domain` and `client_id` must both be
  present, alongside a client secret (below). It is the only gate the service
  has, and a missing or partly filled section is a startup error, because a
  service that cannot verify a session cannot authenticate anyone.
  - `domain` is the tenant host, `host[:port]` with no scheme and no path.
  - `client_id` and `domain` are not secrets: the tenant host and the client
    ID are public identifiers, and verification uses Auth0's published signing
    keys, so they are ordinary configuration values rather than secret files.
- Who may sign in, and who among them holds cross-subject rights, is not a
  file setting: a post-login Action in the tenant asserts two namespaced ID
  token claims, and this service does no more than read them. The `sub` claim
  itself is still matched exactly, with no normalisation, because it is an
  opaque provider-issued identifier rather than a human-typed address — there
  is simply nothing left in this file to match it against.
- `state.database_path` is required and must reside on the persistent Docker
  volume.
- The interval between scheduled runs and the per-target deletion limit are not
  settings in either home. They are constants — one hour and five — stated in
  the code that enforces them.

The decoder rejects unknown fields, invalid addresses or URLs, an unreadable
secret file, and an ambiguous secret input before it opens an HTTP listener. An
unknown field includes every key that belongs in the database, so a file
carrying one fails startup with the key named.

## Runtime settings

The settings below are held in the state database rather than in the file. They
are read once at startup into a snapshot, replaced whole when the operator saves
them from the browser UI, and in force from the next request and the next run.

What is left in the file is what the host has to know before the process can
serve anything: where to listen, who may call, and where its state is. Nothing
that decides *what work the service does* is in the file.

Every reader takes a copy of the snapshot for the length of one run, one
request, or one notification, so an edit lands between two units of work rather
than inside one. A run that started before a save finishes on the settings it
started with.

A save writes every setting at once. There is no partial write. Credentials are
the exception, on the terms [Credentials](#credentials) states.

The rules below are checked in one place by both edges: the write path checks
what the browser submitted, and startup checks what the database returned. A
value that is served is therefore one that would have passed the check that let
it be written, including in a database edited by hand, which fails startup
naming the setting. Checking normalises as it goes, so names and URLs are stored
trimmed. The names used below are the ones the service's own messages use;
[the service contract](service.md) documents the JSON names the same settings
carry on the wire.

A deployment that has never opened the settings page runs on the seeded
defaults, which are the ones each rule names below: deletion denied, a 24-hour
staleness bound, a one-minute delay before the first run, notifications on with
nothing yet ruled on in the alert matrix, Pushover's own origin, a single
keyless basemap, no surface regions, and a weekly index rebuild.

The settings that name an upstream have no default. An OAuth application and a
library account belong to one operator. They are seeded **unconfigured**, and a
service holding those seeds starts, passes its readiness probe, serves the
settings page, and runs nothing. A scheduled run finds nothing configured and
no-ops rather than failing, and the page names which settings are still
missing. That is the state every new deployment begins in. A target is not
among these settings at all: it belongs to the subject who created it by
connecting, not to an operator, and none exists until someone does.

### Credentials

The Wahoo client secret, each source's email and password, and the Pushover
application token and user key are stored in the state database, encrypted under
the state key exactly as a Wahoo refresh token is. The secret's own name is the
associated data, so a ciphertext moved from one row to another fails to open
rather than authenticating as the wrong credential. A database written under a
different key is a startup failure naming no value.

They are **write-only**. The settings endpoint never returns a stored value, in
any form, to any caller: it reports per credential whether one is set, and the
page offers to replace it. This is the one place the whole-object save does not
hold.

A save carries only the credentials that were actually typed into it. One left
untouched keeps the stored value, and one submitted empty is left alone rather
than cleared. A credential is removed by the deployment losing its database, not
by the page.

A submitted name that no part of the service reads is refused, naming it.

The seven names are `wahoo.client_secret`, `veloplanner.email`,
`veloplanner.password`, `komoot.email`, `komoot.password`,
`notifications.pushover.application_token`, and
`notifications.pushover.user_key`.

### Sources

`sources` is the libraries a run reads, in the order it reads them. Each entry
carries a `provider` — `veloplanner` or `komoot` — and a `base_url`, which must
be an absolute HTTPS origin without a path, matching what the adapter itself
requires. A provider may appear at most once; a run reads each provider once and
stores its inventory under that provider's name.

The base URL also reaches the browser through `GET /v1/webui/config`, keyed by
provider, as the base of a route's link back to its source route. Pointing it at
a different deployment moves both the inventory that is read and the link that
is offered. The authenticated account's user ID is discovered from the
credentials, never configured.

An empty list is accepted and is what a new deployment holds. A configured
source whose email and password have not been entered is **not** skipped: the
run refuses instead.

### Wahoo

`wahoo.api_base_url` and `wahoo.oauth_base_url` are absolute HTTPS origins
without a path, matching what the adapter itself requires, and
`wahoo.client_id` is non-secret. Together with the client secret they are the
OAuth application. The service builds no Wahoo client until all four are set,
and an unconfigured application makes runs report they are not ready.

Which destinations exist is not a setting: each target is a signed-in
subject's own value, created the moment that subject starts their first Wahoo
authorisation, not entered on this page. It is the identity every stored
authorization, target route, and recorded run carries. A deployment with no
target yet — because no application is configured, or because nobody has
connected — still passes its readiness probe.

The endpoint values must match the chosen Wahoo environment. The callback the
application is registered with is derived from `http.browser_origin_url` and is
not a setting here.

The OAuth scopes are fixed by the binary: `routes_read`, `routes_write`,
`user_read`, `workouts_read`, and `offline_data`. A scope change needs every
configured target to authorise again; no operator setting can silently expand it.
It also needs the Wahoo Cloud application to list the new scope before the image
that requests it is deployed: Wahoo refuses an authorisation naming a scope the
application is not registered for, returning the rider to the callback with
`error=invalid_scope` and no code, before any consent screen is shown.

### Synchronisation

The **empty-source deletion gate** decides whether a source that reported an
empty library may delete the last owned routes a target holds. It denies by
default. Allowing it stays allowed until it is turned off again; nothing expires
it. The UI asks before turning it on and says as much.

`sync.stale_after` bounds how long the trusted source inventory may go without a
successful refresh before `GET /v1/status` reports it as stale and a
notification goes out. It is at least one second and defaults to 24 hours. The
response reports age in whole seconds, and a sub-second bound is rejected.
[The sync lifecycle specification](sync-lifecycle.md#trusted-inventory-freshness)
states what is measured and how the notification is rate-limited.

`sync.initial_delay` is how long after start the first run is attempted. It is
at least one second and defaults to a minute. It is consumed once, at the start
it delays, so an edit changes the next start rather than the current one. It is
the one setting here that is not in force from the next run.

### Notifications

`notifications.enabled` is the switch for the whole channel, and it is on by
default. Off is not a quieter setting: it suppresses a failure, a blocked run,
and a stale task as surely as it suppresses a routine success. Every surface
offering it states that.

Which of them are sent is not a setting here. It is a decision per task and per
reason, held in the alert matrix; see
[the task layer specification](task-layer.md#the-alert-matrix).

`notifications.pushover.base_url` is an absolute HTTPS origin without a path,
defaulting to Pushover's own. It is a setting rather than a compiled-in
constant, so a development or demo environment can point it at an address that
goes nowhere. A path is rejected by the setting rather than by the notifier, so
the failure names the setting. The credentials sent to that origin are stored
beside it, on the terms [Credentials](#credentials) states.

### Basemaps

`webui.basemaps` is the list of cartographies the route map view offers, in the
order they are offered. **At least one entry is required**, and the default is a
single keyless one. The first entry is what a browser that has never chosen one
loads. The page shows a picker only when the list holds more than one.

Each entry carries:

- `name` — the label the picker shows, and the identity a browser remembers a
  reader's choice by. Required, non-empty after trimming, and unique across the
  list. Renaming an entry forgets any choice remembered under the previous name.
- `style_url` — the MapLibre style to load. It must be an absolute HTTPS URL.
  Unlike the service's own endpoints it may carry a query string; providers that
  require an API key put it there.
- `style_url_dark` — optional; the style the browser loads instead when it
  reports a dark system colour scheme. Omitting it leaves one style in force
  under both schemes. When set it must be an absolute HTTPS URL **on the same
  origin** as that entry's `style_url`, and a list that breaks the rule is
  refused on the save that submits it.
- `dark_cartography` — optional; `true` marks ground that is dark whatever the
  system asks for, which is what satellite imagery is. Anything the page paints
  over the map reads this rather than the colour scheme. It contradicts
  `style_url_dark`, and setting both is refused.

A style URL is **not** a secret and is never handled as one: the browser must
know it, so it is served to the page and is visible to anyone who can reach the
UI. The default is a keyless provider, so a default deployment publishes no
credential and sends no account identity to the tile origin. An operator who
chooses a keyed provider accepts that the key becomes visible to the UI's single
authorised user. Self-hosted tiles avoid that and avoid the browser revealing
the area of a viewed route — or, once the map's locate button is pressed, the
area around the reader's own live position — to the tile origin. The raw
position is never revealed: it moves the camera and goes no further.

The list changes the Content-Security-Policy the service sends to a caller whose
identity has been established, which permits the service's own origin plus **the
origin of every configured entry**, sorted and deduplicated. An answer served
before an identity exists — a build artefact, a sign-in page, or a refused
request — names no tile origin at all, because the header travels with it and
would otherwise say which provider a deployment uses to whoever asked. The header is composed per response from the live list, so a
basemap added on the settings page is permitted by the next response rather than
at the next restart. An entry's dark style is held to that entry's own origin,
so the configured origins are as many as the distinct providers offered and no
more.

A provider is free to serve its style from one host and its glyphs, its sprite,
or its tiles from others, and that is common: a style URL alone does not say
which hosts the page will need. Those hosts are named in the style document
rather than in the settings, so **the service reads each configured style** and
permits what it names as well: its `glyphs`, its `sprite`, and each source's
tiles — including the tiles named by a source's TileJSON, one level deep, since
a provider may serve those from a host its TileJSON URL does not name. A
reference that does not resolve to HTTPS is not permitted, because the page
could not reach it in any case.

Each style is read on the save that configures it and again on a periodic
refresh, never while a response is being composed: the policy is built from what
was last read. A style that cannot be read keeps the hosts it was last seen to
name, so a provider being briefly unreachable does not blank a map that works.
A basemap removed from the list stops being permitted with the next response,
without waiting for a refresh.

The policy names which origins the page *may* reach; it does not make the page
reach them. Only the basemap on screen is ever requested.

The whole list is served to the page, which chooses within it. The service
resolves neither the reader's choice nor the colour scheme; both belong to the
browser, and this response is cached for the session. Saving the settings
discards that cached copy in the browser that saved them, so the picker offers
the edited list without a reload.

Weather forecasts carry no key at all. Open-Meteo's free forecast endpoint needs
none, so `internal/openmeteo`'s options hold no credential field, unlike
`internal/pushover`'s, which carries an application token and a user key.

### Surface classification

`surface.regions` names the OpenStreetMap extracts the **service** builds its
surface index from, and classification reports a route's ground as asphalt,
paving, compacted, gravel, or unsurfaced. Each entry is a Geofabrik region path
such as `europe/germany/rheinland-pfalz`: lowercase path segments of letters,
digits, and single hyphens, and nothing else. The shape is validated where it is
entered; a region becomes a URL under a fixed host, and a validated slug can
introduce no host, query, or traversal. A blank line and a repeat are dropped
rather than refused.

The default is **no regions**, which switches surface classification off:
nothing is downloaded, no index is built, and routes carry no surface. Each
named region costs disk and build time.

This is not a browser concern, and it sends no route data anywhere:
classification reads a local file. The only outbound traffic is the scheduled
download of each region's published extract, which tells the extract host which
regions this deployment cares about and nothing about any route.

`surface.rebuild_interval` is how often the index is rebuilt. It is at least a
second and defaults to one week. It is required whether or not a region is
named; the rebuild schedule is created either way and builds nothing when there
are no regions, and a cadence of zero would be a schedule that could not be
started. A rebuild first fetches each region's published checksum and stops
there when every one is unchanged.

The interval is time **between builds**, not time since this process started:
the service records when the last build finished and counts from there, so a
deployment restarted several times a day still rebuilds weekly. A build that is
already overdue when the process starts still waits a few minutes.

The index is written beside the state database, which is the one directory a
deployment is guaranteed to have made durable. It is named for the generation it
was built from, so a new build is written and opened beside the live one, and
the replaced file is removed only once the new one is serving. A build holds
roughly half a gigabyte of heap and stages an extract of a few hundred megabytes
on disk, both of which are released when it finishes.

### Ride model

The coefficient pair a predicted moving time is priced with — `seconds_per_km`
and `seconds_per_ascent_m` — lives as one row in the state database. There is
no operator setting: an operator names no file and edits no value, and the
settings page shows the pair read-only alongside where it came from.

A built-in default pair applies until the first calibration replaces it, so
prediction works from the first deploy rather than waiting on a fit. The weekly
calibration replaces it from the rider's own recorded activities, and leaves
whichever pair is in force alone whenever it refuses to fit. The row
also records the calibration's cutoff date, the rides it was measured over and
the resulting error, and the window the fit reached back over.

`evaluated_rides`, `bias_percent`, `mae_percent` and `p90_percent` are the
pair's measured unseen-route error. They are optional — the built-in pair
carries none — and when present they are served alongside a route's predicted
moving time, so the browser can qualify the estimate rather than present it as
a bare number.

Those four describe the *procedure* that produced the pair rather than these
exact coefficients. The fit walks a monthly origin across the corpus; each fold
calibrates on the training window behind its origin and is scored on the unseen
routes of the month after it, so no fold is measured against a ride it was
fitted on. Those per-fold errors are pooled into the four fields. The
coefficients themselves are fit over the newest window, including the rides the
last folds scored.

Not every stored ride prices a pace. The corpus is the rides ridden outdoors
under the rider's own power; an indoor trainer reports a virtual distance over
no real ground, and a motor-assisted ride is not the rider's own pace, so
neither calibrates a model that predicts outdoor route times. Both are still
stored and still served — they are cycling, just not calibration input.

`training_window_months` is how far back that fit really reached;
`calibration_cutoff` is where it stopped. The two together state which rides
produced the pair. Zero months means a fit over all history, which only a pair
predating the window can carry. The weekly calibration reaches back twelve
months — the shortest span covering a full year of weather and daylight, and
short enough that a rider whose form has durably changed is priced as they ride
now rather than against every season pooled together. The window is a bound
rather than a guillotine: one holding too few rides to fit from extends back to
the minimum number of rides instead, so a rider who has ridden for less than a
year, or only occasionally, still gets a fit. A pair fitted from an extended
window records the months it really reached, not the twelve it asked for.

The equation itself is versioned in code rather than stored beside the pair,
and a change to it invalidates a cached prediction even when the pair is
unchanged. A pair that could not have come from a real fit — either term zero
or negative — is refused rather than predicted with, which leaves the routes
that would have carried a prediction without one. It does not substitute a
guess, and it does not stop the service.

[The implementation architecture
specification](implementation-architecture.md#predicted-moving-time) states
where that computation belongs.

## Runtime state

Dynamic Wahoo refresh tokens are not configuration. They are encrypted in
SQLite using the supplied state key; access tokens remain in memory only. The
runtime settings above share that database and share its fate: a lost database
returns every one of them to the seed it started at, which for the settings that
name an upstream means unconfigured. The credentials among them are encrypted
under the same key as a refresh token; the rest are stored in the clear, none of
them being a secret. The state database is intentionally not backed up. Changing
the state key makes the existing encrypted state unreadable, and key rotation is
not a feature.

The request quota Wahoo last advertised is runtime state of the same kind. It is
stored with the instant it expires, so a restart resumes with a window it already
found spent; a reading past its expiry is discarded rather than honoured, and a
lost database only means the next request finds the quota out again.

Browser sessions are the same kind of runtime state: `web_sessions` lives in
the same database and shares its fate. A lost database signs every subject
out, the same way it returns every runtime setting to its seed — a sign-in
problem rather than a recovery one, and nothing this specification treats as
data loss.

A session row also carries the ID token's `nickname` claim, when the sign-in
supplied one, beside the subject — personal data of the same kind as the
subject itself, held only as a display label on the settings page and never
as a key: ownership and admin comparisons never read it, and no lookup goes
the other way from a nickname to a subject.

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
- a listener address, the browser origin, or the identity gate edited through
  the settings surface. What an operator may change while the service runs is
  the list above and nothing else, and the three things the file holds are
  exactly the three a wrong value would lock them out of the page with;
- a stored credential read back out of the settings surface in any form; and
- secret rotation without a controlled migration.
