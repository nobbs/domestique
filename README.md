# domestique

`domestique` mirrors one private VeloPlanner route library to one or two Wahoo
accounts. It is a single-tenant service intended to run as an arm64 Docker
container on a Tailnet host.

It also serves a read-only browser UI on the same private listener, which draws
one stored route stage at a time on a map. Basemap tiles come from a configurable
keyless provider — the only request this service's page makes outside the Tailnet.

The accepted v1 contract is in [the service specification](docs/specs/service.md).
The supporting specifications define configuration, sync safety, architecture,
and delivery before implementation begins.

## Development

Install the pinned local toolchain and run the complete quality gate:

~~~sh
mise install
mise exec -- make check
~~~

Install the optional Git hook with `mise exec -- prek install`. Work on the
browser UI with `mise exec -- make ui-dev`, which serves it with hot reload and
proxies the API to a locally running service. See
[CONTRIBUTING.md](CONTRIBUTING.md) for the contributor workflow. Copy
[`config.example.toml`](config.example.toml) outside the repository when
preparing a local deployment; it references secret files and never embeds
secret values.

## Deployment

The service image is CGO-free `linux/arm64` and embeds the browser UI, so the
API and the UI ship as one signed artefact. Its base images are Docker Hardened
Images, so building it requires `docker login dhi.io`; deploying it does not.
The host owns the private boundary: Docker publishes only `127.0.0.1:8080`,
while host-level Tailscale Serve provides HTTPS and the authenticated identity
header to the container. Tailscale does not run in the image.

For the current Apple-silicon Mac MVP, follow the
[macOS Docker guide](docs/macos-mvp.md). The later
[Pi deployment guide](docs/deployment.md) uses the same runtime contract with
a verified immutable image digest. The optional sandbox encoder check is
documented in [the FIT acceptance guide](docs/fit-sandbox-acceptance.md).
