# macOS Docker MVP

Use this target to run Domestique on an Apple-silicon Mac before moving the
same Docker image definition and runtime-state model to a Raspberry Pi. It is
an MVP host:
the Mac must stay awake, Docker Desktop and Tailscale must remain running, and
the Mac's Tailnet device name becomes the Wahoo OAuth callback host.

The setup keeps the same trust boundary as the Pi deployment: Docker publishes
only 127.0.0.1:8080, and Tailscale Serve on the macOS host terminates HTTPS and
adds the authenticated identity header. Do not run Tailscale in the container
or use Funnel.

## Prepare local runtime files

From the repository root, copy the non-secret configuration template:

~~~sh
cp config.example.toml config.toml
mkdir -m 700 secrets
~~~

Update config.toml before starting the container:

- set access.tailnet_user_login to the exact Tailnet login;
- set wahoo.client_id and use the Wahoo sandbox or production endpoints that
  match the approved app;
- use two stable target-slot IDs; and
- leave secret paths under /run/secrets unchanged.

Create the six files below in secrets/. Each contains one value and is ignored
by Git:

- state_encryption_key — base64url encoding of exactly 32 random bytes;
- veloplanner_email and veloplanner_password;
- wahoo_client_secret;
- pushover_application_token; and
- pushover_user_key.

The state key is not recoverable. Keep it with the Docker volume for the
lifetime of this MVP instance; losing either requires both Wahoo accounts to be
authorized again.

## Start locally

~~~sh
docker compose -f compose.macos.yml up --build --detach
curl --fail http://127.0.0.1:8080/healthz
~~~

The Compose target builds the repository's pinned linux/arm64 image locally,
runs it as UID/GID 65532, drops all Linux capabilities, uses a read-only root
filesystem, and persists only /var/lib/domestique in the named
domestique-state volume.

## Expose it privately through Tailscale

After the loopback probe succeeds, configure Tailscale Serve on this Mac:

~~~sh
tailscale serve --bg 8080
tailscale serve status
~~~

For this MVP, the copied configuration sets wahoo.redirect_url to the URI
below. Set the callback URL in the Wahoo developer application to the identical
value:

~~~text
https://domestique.fluffy-sargas.ts.net/oauth/wahoo/callback
~~~

Open that URL from the configured Tailnet identity. Authorize both Wahoo slots
at /oauth/wahoo/start/{target-id}, then inspect /v1/status. Tailscale Serve
removes client-supplied identity headers and adds Tailscale-User-Login only for
Tailnet traffic, so the host's loopback-only publication remains essential.

## Operate and stop

Follow logs with docker compose -f compose.macos.yml logs --follow. A
successful initial delayed sync and each hourly run produces a Pushover
notification. Keep the Mac awake while expecting a sync or OAuth callback;
sleep pauses both the container and local Tailnet proxy.

To stop the MVP without deleting state:

~~~sh
docker compose -f compose.macos.yml down
~~~

Do not add --volumes: that removes encrypted OAuth state. The later Pi
deployment uses the same configuration and secret-file contract, but deploys a
verified immutable GHCR digest instead of building from the checkout.
