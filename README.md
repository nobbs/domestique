# domestique

`domestique` mirrors one private VeloPlanner route library to one or two Wahoo
accounts. It is a single-tenant service intended to run as a Docker container on
an amd64 or arm64 Tailnet host.

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

The service image is CGO-free `linux/amd64` and `linux/arm64`, and embeds the
browser UI, so the API and the UI ship as one artefact. Every change to the
default branch publishes it to `ghcr.io/nobbs/domestique` from GitHub Actions,
tagged `latest` and `sha-<commit>` and carrying a BuildKit bill of materials and
provenance. A host deploys the immutable digest that run reported, never a tag.
The image is not signed; the [delivery specification](docs/specs/delivery.md)
states why and what stands in its place. Its base images are Docker Hardened
Images, so *building* it requires `docker login dhi.io`; deploying it does not,
because a host pulls rather than builds.
The host owns the private boundary: Docker publishes only `127.0.0.1:8080`,
while host-level Tailscale Serve provides HTTPS and is the origin a Cloudflare
Tunnel dials by Tailscale Service name. Tailscale does not run in the image.

The [Linux VM guide](docs/hetzner.md) covers the long-running host. The
[Cloudflare Access guide](docs/cloudflare-access.md) covers how the single
operator reaches it — the only way in, and without publishing a listener. The
[Pi deployment guide](docs/deployment.md) applies the same runtime contract to
smaller hardware, and the [macOS Docker guide](docs/macos-mvp.md) covers the
Apple-silicon MVP host, which still builds from the checkout. The optional
sandbox encoder check is documented in
[the FIT acceptance guide](docs/fit-sandbox-acceptance.md).
