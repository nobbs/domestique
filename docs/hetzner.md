# Linux VM deployment

Domestique runs as one Docker container on a small always-on Linux VM in the
Tailnet. This guide covers a host that **pulls a published image by digest**.
That is the same runtime contract as the [Pi deployment](deployment.md); the two
guides differ only in how the host itself is prepared. GitHub Actions publishes
the image from the default branch, so this host never builds one.

Building here was the earlier arrangement, and it is where this VM's memory
pressure came from: a two-platform `npm ci` and Go build does not fit alongside
the running service on two vCPUs and 4 GB. A host that only pulls carries none
of that load and needs no `dhi.io` credential.

The compose file, configuration, secret files, and pinned digest stay on the
host, outside this repository.

## Trust boundary

```mermaid
flowchart LR
  browser["Your Tailnet browser"] --> serve["Host Tailscale Serve\nHTTPS + identity header"]
  serve --> loopback["127.0.0.1:8080"]
  loopback --> app["Domestique container\nread-only root filesystem"]
  app --> state["Named Docker volume\nSQLite state"]
  app --> providers["VeloPlanner, Wahoo, Pushover"]
```

This is the same boundary the other host guides describe, and it is the reason
the listener stays private at all. On a VM with a public IP it deserves extra
care: the container must publish to `127.0.0.1` only, never to `0.0.0.0`.
Confirm it after every start with `ss -tlnp`. Do not use Tailscale Funnel, and
do not put a general-purpose reverse proxy in front of the service. The
application authenticates every request by verifying a Cloudflare Access
assertion, so a proxy cannot hand over the API by forwarding a header — but it
can still expose an unauthenticated listener to the Internet.

Reaching the service from outside the Tailnet has one supported form, described
in [the Cloudflare Access guide](cloudflare-access.md). It satisfies the rule
above rather than bending it: the proxy's origin is the Tailscale Service name,
so Tailscale Serve stays in the path and strips the client-supplied header, and
the application independently verifies a signed Cloudflare Access assertion. Any
other proxy, or that same proxy pointed at `127.0.0.1`, hands over the API.

## Prepare the host

Install Docker Engine with the Compose plugin from Docker's own apt repository,
and Tailscale from its apt repository. Then join the Tailnet with the tag that
your policy allows to host the service:

```sh
tailscale up --advertise-tags=tag:<service-tag>
```

Tag assignment needs an auth key or an interactive login, and the tag must
appear in the Tailnet policy's `tagOwners`. Verify with `tailscale status --json`
that the node carries the tag before continuing.

## Prepare deployment state

Create a directory owned by the operator, for example `/srv/domestique`:

```text
/srv/domestique/
├── compose.yml   # docs/compose.example.yml, unmodified
├── .env          # DOMESTIQUE_IMAGE=ghcr.io/nobbs/domestique@sha256:<digest>
├── config.toml   # config.example.toml with every placeholder replaced
├── secrets/      # the six secret files
└── src/          # optional checkout, for development against real data
```

Copy [`config.example.toml`](../config.example.toml) to `config.toml` and
replace all placeholders. `wahoo.redirect_url` must be the HTTPS URL this host
serves and must end in `/oauth/wahoo/callback`.

Create the six files in `secrets/`, each containing exactly one value:
`state_encryption_key` (base64url of 32 random bytes), `veloplanner_email`,
`veloplanner_password`, `wahoo_client_secret`, `pushover_application_token`,
and `pushover_user_key`.

**File ownership matters on Linux.** Compose bind-mounts each secret into the
container, where the unprivileged runtime user reads it directly, so the files
must be readable by UID `65532`. Docker Desktop hides this on macOS; a Linux
host does not:

```sh
chown 65532:65532 secrets/*
chmod 0400 secrets/*
chmod 0444 config.toml
```

Keep the state encryption key together with the `domestique-state` volume.
Losing either requires Wahoo reauthorisation and safe route adoption; there is
intentionally no state backup or key-rotation workflow in v1.

## Pull a published image

Every default-branch change that touches an input of the image publishes one
multi-architecture image to `ghcr.io/nobbs/domestique`. The package is private,
so the host needs a **classic** personal access token scoped to `read:packages`
— a fine-grained token cannot authenticate to ghcr.io:

```sh
read -rs GHCR_TOKEN
printf '%s' "$GHCR_TOKEN" | docker login ghcr.io -u <github-user> --password-stdin
unset GHCR_TOKEN
chmod 600 ~/.docker/config.json
```

Take the short commit from the run that published the image, and resolve the
**index** digest:

```sh
docker buildx imagetools inspect ghcr.io/nobbs/domestique:sha-<short-commit> \
  --format '{{.Manifest.Digest}}'
```

Confirm it matches the digest that run reported in its summary, then write the
value into `.env`, replacing any earlier one:

```text
DOMESTIQUE_IMAGE=ghcr.io/nobbs/domestique@sha256:<digest>
```

Pin that index digest — not a per-architecture manifest digest, and not a tag.
Reading a digest out of `docker images --digests` after a pull can hand you the
manifest for this host's architecture alone, which silently undoes the
multi-architecture image. `latest` moves, so it names an image to look at, never
one to deploy.

The image is not signed; [the delivery specification](specs/delivery.md) states
why and what stands in its place.
[The Pi guide](deployment.md#select-an-image) carries the same commands together
with the provenance and bill-of-materials inspection.

## Run the container

Copy [`compose.example.yml`](compose.example.yml) to `compose.yml`, then start
it from the deployment directory:

```sh
docker compose --env-file .env up -d
curl --fail http://127.0.0.1:8080/healthz
ss -tlnp | grep 8080
```

The last command must show `127.0.0.1:8080` and nothing bound to a public
address. The service writes no log line on a healthy start — it logs only
errors — so a running container with no restarts and a passing health probe is
the expected quiet result.

## Publish it through Tailscale

Configure the host as the proxy for the managed Tailnet Service:

```sh
tailscale serve --service=svc:<service> --https=443 127.0.0.1:8080
tailscale serve advertise svc:<service>
tailscale serve status
```

A **new** host advertising an existing service needs admin approval before the
service name resolves. Until it is approved, `tailscale serve status` shows the
configuration but the Tailnet DNS name does not resolve and `Self.Services`
stays empty in `tailscale status --json`. Approve the host for the service in
the admin console, then re-check.

Put the exact served URL plus `/oauth/wahoo/callback` into both
`wahoo.redirect_url` and the Wahoo application's registered callback. Ensure
the Tailnet policy permits only the configured user to reach the host.

Open the service URL in the configured user's browser, then authorise each fixed
target slot from `/oauth/wahoo/start/<target-id>` and check `/v1/status` after
each one. A tagged device is not a member identity and cannot complete the
protected OAuth flow.

## Move an existing deployment to this host

Keeping the same Tailnet service name means the served URL, and therefore
`wahoo.redirect_url`, does not change, so no Wahoo slot needs reauthorising if
the state and its key travel together.

1. Stop the container on the old host with `docker compose down` — **never**
   `-v`. A clean stop checkpoints the SQLite WAL into the database file.
2. Stream the state volume to the new host, and copy `config.toml` and the six
   secret files across:

   ```sh
   docker run --rm -v <project>_domestique-state:/s:ro alpine tar -cf - -C /s . \
     | ssh <host> 'docker run --rm -i -v <project>_domestique-state:/s alpine \
         sh -c "tar -xf - -C /s && chown -R 65532:65532 /s"'
   ```

3. Apply the ownership rules above to the copied secrets, then start the
   container and confirm `/v1/status` reports every target as `authorized`.
   That is the proof the encryption key and database arrived intact; a target
   that reports otherwise means the key and the database no longer match, and
   the run should be stopped rather than left to reconcile against half the
   state.
4. Withdraw the service from the old host with `tailscale serve drain` followed
   by `tailscale serve clear`, then advertise it here. Only one host should
   advertise the service, because two would run the sync loop against the same
   Wahoo accounts.

Leave the old host's volume in place until the new host has completed a sync;
it is the rollback path.

## Update and rollback

To update, resolve the digest of a newer published image as above, point
`DOMESTIQUE_IMAGE` at it, and run `docker compose --env-file .env up -d`. The
named state volume stays in place. Rollback is the same operation with a digest
this host ran before; it never restores old SQLite state.

Keep the previous digest written down. A digest stays pullable after its tag has
moved on, so the rollback path survives even once the local copy is gone — but
only if the value was recorded somewhere.

A host that used to build has disk to reclaim. Once a published image has
completed a sync, remove the stale `domestique:<git-sha>` images with
`docker image rm`, then run `docker builder prune -af` to clear the BuildKit
cache those builds accumulated. Leave the `alpine` image in place; the migration
recipe above uses it. Never run `docker system prune --volumes`.

Do not run `docker compose down -v`, which deletes the state volume.
