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
`DOMESTIQUE_STATE__DATABASE_PATH` maps to `state.database_path`.

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

The file is the whole of what the host hands the process, and this is all of it:

```toml
[http]
listen_address = ":8080"
readiness_address = ":8081"
browser_origin_url = "https://pi.example.ts.net"

[access.cloudflare]
team_domain = "yourteam.cloudflareaccess.com"
application_aud = "the AUD tag of the Access application"
allowed_email = "you@example.com"

[state]
database_path = "/var/lib/domestique/state.db"
encryption_key_file = "/run/secrets/state_encryption_key"
```

What the file no longer carries is as much a part of its contract as what it
does. There is no source and no source credential, no Wahoo application and no
target slot, no notification credential, no ride model, and no delay before the
first run — nor a basemap list, a surface region or rebuild cadence, a staleness
bound, an empty-source deletion gate, or a notification switch, success policy,
digest period, or Pushover origin. Every one of those is a runtime setting, and
the decoder refuses it here by name.

## Secret input

The file names exactly one secret, and it is the one every other secret is kept
under. It has one active input: a TOML file path, an overriding `*_FILE`
environment value, or a direct environment value. The file input is preferred
for Docker deployments; the direct environment value supports a simple local
setup.

| Canonical secret | TOML file-path field | Direct environment value | Environment file path |
| --- | --- | --- | --- |
| state encryption key | `state.encryption_key_file` | `DOMESTIQUE_STATE__ENCRYPTION_KEY` | `DOMESTIQUE_STATE__ENCRYPTION_KEY_FILE` |

A literal `state.encryption_key` is invalid in the TOML file. It is accepted
only from the documented direct environment variable. The `*_FILE` environment
variable overrides the TOML file path, but it must not accompany the direct
value.

A file secret must be an absolute path to a regular readable file. It must be
non-empty after one terminal line break is trimmed, and it decodes as a
base64url encoding of exactly 32 random bytes. The service reads it once at
startup, does not log the value or the path, and clears the direct secret
environment value from its own process environment after loading.

Every other credential the service holds — the Wahoo client secret, each
source's account, and the Pushover pair — is entered on the settings page and
kept encrypted under this key.
[Credentials](#credentials) below states what that means for a deployment.

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
- `http.browser_origin_url` is required, and must be an absolute HTTPS origin
  with no path. It is the address a browser reaches this service at: the public
  hostname when the Cloudflare path is deployed, because an OAuth redirect lands
  in an ordinary browser that may not be on the Tailnet, and the Tailnet URL
  otherwise.

  It is the one origin a state-changing request may come from. A request to a
  sync trigger, the schedule switch, or a stage reprocess that names any other
  origin — or none — is refused, so that an authenticated session cannot be
  driven from another site.

  The Wahoo callback is derived from it rather than configured beside it: the
  redirect URL is this service's own `/oauth/wahoo/callback` on this origin, and
  it must match the callback registered with Wahoo. Two settings that had to
  agree are one setting that cannot disagree, and the path is not an operator's
  choice in the first place — this binary serves exactly that one.

  It is a file setting rather than a runtime one because it is deployment
  topology: it is fixed by where the container is published, it gates the write
  path that the settings page itself uses, and a wrong value edited through that
  page would lock the operator out of the page that could correct it.
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
- The interval between scheduled runs and the per-target deletion limit are not
  settings in either home. They are constants — one hour and five — stated in
  the code that enforces them. Both were file keys that accepted exactly one
  value, which is a dial in name only, and a limit that exists to stop a runaway
  deletion is not one an operator has a use for.

The decoder rejects unknown fields, invalid addresses or URLs, an unreadable
secret file, and an ambiguous secret input before it opens an HTTP listener. An
unknown field includes every key that moved into the database, so a file still
carrying one fails startup with the key named rather than having it quietly
ignored.

## Runtime settings

The settings below are held in the state database rather than in the file. They
are read once at startup into a snapshot, replaced whole when the operator saves
them from the browser UI, and in force from the next request and the next run.

They live there because each one is something somebody has a reason to change on
a service that is already running: the deletion gate opened for one deliberate
run and closed again afterwards, a basemap added to the picker, notifications
quieted for a week, a region added because a holiday moved where somebody rides,
a library account whose password was rotated, a second rider's slot named. As
file keys, every one of those changes cost an edit on the host and a container
restart. The restart is not the expensive part — needing shell access on the
host to reach a switch the UI could offer is.

What is left in the file is what the host has to know before the process can
serve anything at all: where to listen, who may call, and where its state is.
Nothing that decides *what work the service does* is in there any more.

Every reader takes a copy of the snapshot for the length of one run, one
request, or one notification, so an edit lands between two units of work rather
than inside one. A run that started before a save finishes on the settings it
started with.

A save writes every setting at once. There is no partial write, because the form
holds every field and a body naming only some of them would leave the rest at
whatever the caller assumed they were. Credentials are the exception, and
[Credentials](#credentials) below says why.

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
staleness bound, a one-minute delay before the first run, notifications on with
a message per successful run, a 24-hour digest period, Pushover's own origin, a
single keyless basemap, no surface regions, and a weekly index rebuild.

The settings that name an upstream have no such default, because none of them
can be guessed: an OAuth application, a target slot name, and a library account
belong to one operator. They are seeded **unconfigured**, and a service holding
those seeds starts, passes its readiness probe, serves the settings page, and
runs nothing. A scheduled run finds nothing configured and no-ops instead of
failing, and the page says which settings are still missing rather than leaving
an operator to infer it from a quiet service. That is the state every new
deployment begins in, and it is the state an operator configures their way out
of through the browser rather than through the host.

### Credentials

The Wahoo client secret, each source's email and password, and the Pushover
application token and user key are stored in the state database, encrypted under
the state key exactly as a Wahoo refresh token is. The secret's own name is the
associated data, so a ciphertext moved from one row to another fails to open
rather than authenticating as the wrong credential. A database written under a
different key is a startup failure naming no value, not a service that silently
holds no credentials.

They are **write-only**. The settings endpoint never returns a stored value, in
any form, to any caller: it reports per credential whether one is set, and the
page offers to replace it. This is the one place the whole-object save does not
hold, and it has to be — a form that had to echo a credential back in order to
save the settings around it would put every credential the service holds into
the response to an ordinary page load.

So a save carries only the credentials that were actually typed into it. One
left untouched keeps the stored value, and one submitted empty is left alone
rather than cleared, because a blank field on a form is what an operator who
changed nothing leaves behind. A credential is removed by the deployment losing
its database, not by the page.

A submitted name that no part of the service reads is refused, naming it. Storing
a credential nothing would ever look for is a page quietly accepting a
misconfiguration.

The seven names are `wahoo.client_secret`, `veloplanner.email`,
`veloplanner.password`, `komoot.email`, `komoot.password`,
`notifications.pushover.application_token`, and
`notifications.pushover.user_key`.

### Sources

`sources` is the libraries a run reads, in the order it reads them. Each entry
carries a `provider` — `veloplanner` or `komoot` — and a `base_url`, which must
be an absolute HTTPS origin without a path, matching what the adapter itself
requires. A provider may appear at most once, because a run reads each provider
once and stores its inventory under that provider's name.

The base URL also reaches the browser through `GET /v1/webui/config`, keyed by
provider, as the base of a stage's link back to its source route, so pointing it
at a different deployment moves both the inventory that is read and the link
that is offered. The authenticated account's user ID is discovered from the
credentials, never configured.

An empty list is accepted and is what a new deployment holds: it is a service
with nothing to read yet rather than a mistake. A configured source whose email
and password have not been entered is **not** skipped — the run refuses instead,
because reading part of a library and calling it the whole inventory is what the
deletion gate exists to prevent.

### Wahoo

`wahoo.api_base_url` and `wahoo.oauth_base_url` are absolute HTTPS URLs, and
`wahoo.client_id` is non-secret. Together with the client secret they are the
OAuth application, and the service builds no Wahoo client at all until all four
are set: an unconfigured application makes runs report they are not ready rather
than fail against an application that does not exist.

`wahoo.targets` holds up to two destination slots. Each is a non-empty name,
unique within the list, and stable across deployments; it is a configured slot,
not a Wahoo user identifier, and it is the identity every stored authorization,
target stage, and recorded run already carries. Naming a slot creates its
durable record on the save rather than at the next startup, so the one-time
OAuth onboarding that follows has a row to authorise. Removing a slot from the
list keeps that record, which is what a slot renamed back would want, and
nothing reads a record whose slot is not configured.

An empty list is a deployment with nowhere to publish yet. The readiness probe
still reports ready, because a service waiting to be configured through a
browser is running exactly as deployed, and a probe that called that unhealthy
would roll the deployment back before anyone could configure it.

The endpoint values must match the chosen Wahoo environment; the callback the
application is registered with is derived from `http.browser_origin_url` and is
not a setting here.

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

`sync.initial_delay` is how long after start the first run is attempted. It is
at least one second and defaults to a minute. It is consumed once, at the start
it delays, so an edit changes the next start rather than the current one — which
makes it the one setting here that is not in force from the next run. It is
stored beside the others anyway, because the reason to change it is the reason
to change any of them: a deployment that keeps restarting while something is
being configured wants the first run pushed out of the way, and doing that
through the host's file was the friction this split removes.

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

`notifications.digest_interval` is at least a second, at most seven days, and
defaults to 24 hours. The upper bound is the reach of the recorded run history:
a longer period would silently total a window whose earliest runs have already
been pruned. It is checked whatever the policy is, because a setting an operator
will one day switch to should not turn out to have been invalid all along at the
moment they switch.

`notifications.pushover.base_url` is an absolute HTTPS origin without a path,
defaulting to Pushover's own, so a deployment that says nothing about it
notifies Pushover. It is a setting rather than a compiled-in constant for the
same reason the provider endpoints are: a development or demo environment has to
be able to point it at an address that goes nowhere, and the alternative is such
an environment reaching the real service with a placeholder token. The path is
rejected by the setting rather than by the notifier, so the failure names the
setting. The credentials sent to that origin are stored beside it, on the terms
[Credentials](#credentials) states.

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

`surface.rebuild_interval` is how often the index is rebuilt. It is at least a
second and defaults to one week — roughly the pace at which a region's surface
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

### Ride model

`ridemodel.coefficients_file` is a path on the host, and the one setting here
that names something outside the database. It names the hybrid profile the offline benchmark
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
even when the operator's file is unchanged. The service reads the file when the setting names one and computes a predicted
moving time per stage from it, in the manner surface classification is computed — see [the implementation architecture
specification's tier ownership
section](implementation-architecture.md#predicted-moving-time-is-a-deliberate-departure)
for why that placement is correct despite depending on rider mass and power.

The default is **no file**, which switches prediction off entirely: no
coefficient is loaded, no stage anywhere carries a predicted time, and the
routes endpoints omit the field rather than reporting zero. When set, the path
must be absolute, and the file is read and validated for physical plausibility —
not merely parsed — rather than trusted. A path that will not load leaves the
stages that would have carried a prediction without one; it does not substitute
a guess, and it does not stop the service, because the file lives on the host
and the setting that names it is edited from a browser that cannot see whether
it is there.

Unlike the state key, the only other file this specification names,
`coefficients_file` is **deliberately not a secret**. It carries physical constants and a rides-
calibrated correction, and no route data, no credential, and no personal
information, so it is a plain path like `surface.regions`' extracts
rather than a stored credential, and the settings endpoint serves it back the
way it serves any other setting.

## Migrating an existing deployment

A deployment upgrading into this split does not carry its file forward. Every
provider section, target slot, credential path, schedule, and ride-model path is
removed from the file, and the service starts unconfigured: it comes up, passes
readiness, and serves a settings page that names what is still missing.

The operator enters those settings once, in the browser, and the deployment is
configured from then on. The credentials have to be typed rather than moved,
because the file only ever named the paths they were read from and this service
cannot read a path that is no longer configured. There is no import step and no
transition period in which both homes are consulted: a file still carrying a
moved key fails startup naming the key, so the upgrade is found at the restart
that performs it rather than months later.

The Wahoo authorizations already in the database survive, because a slot is
identified by its name. Naming the same slots again adopts the existing rows,
and the onboarding is not repeated.

## Runtime state

Dynamic Wahoo refresh tokens are not configuration. They are encrypted in
SQLite using the supplied state key; access tokens remain in memory only. The
runtime settings above share that database and share its fate: a lost database
returns every one of them to the seed it started at, which for the settings that
name an upstream means unconfigured. The credentials among them are encrypted
under the same key as a refresh token; the rest are stored in the clear, because
none of them is a secret. The state database is intentionally not backed up.
Changing the state key makes the existing encrypted state unreadable, and key
rotation is not a feature.

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
  the settings surface — what an operator may change while the service runs is
  the list above and nothing else, and the three things the file still holds are
  exactly the three a wrong value would lock them out of the page with;
- a stored credential read back out of the settings surface in any form;
- secret rotation without a controlled migration; and
- a third or dynamically created Wahoo target.
