# domestique

`domestique` mirrors one private VeloPlanner route library to one or two Wahoo
accounts. It is a single-tenant service intended to run as a Docker container on
an amd64 Tailnet host.

It also serves a read-only browser UI on the same private listener, which draws
the whole stored library on one map and gives each route a page of its own.
Basemap tiles come from a configurable keyless provider — the only request this
service's page makes outside the Tailnet — and the map follows your system's
light or dark colour scheme.

The accepted contract is in [the service specification](docs/specs/service.md).
The supporting specifications define configuration, sync safety, architecture,
and delivery.

## Development

Install the pinned local toolchain and run the routine check loop:

~~~sh
mise install
mise run quick
~~~

GitHub Actions is the authoritative gate and runs the complete validation on
every pull request. `mise run quick` is the fast local loop; `mise run check`
runs the full gate locally when an earlier answer is worth its cost. The
[delivery specification](docs/specs/delivery.md) records what the two differ by
and why.

Install the optional Git hook with `mise exec -- prek install`. Work on the
browser UI with `mise run ui-dev`, which serves it with hot reload and proxies
the API to a locally running service. See [CONTRIBUTING.md](CONTRIBUTING.md) for
the contributor workflow. Copy [`config.example.toml`](config.example.toml)
outside the repository when preparing a local deployment; it references secret
files and never embeds secret values.

## Deployment

The service image is CGO-free `linux/amd64` and embeds the browser UI, so the
API and the UI ship as one artefact. A change to the default
branch that touches an input of the image publishes it to
`ghcr.io/nobbs/domestique` from GitHub Actions, tagged `latest` and
`sha-<commit>` and carrying a BuildKit bill of materials and provenance. A
change that touches none — documentation, say — publishes nothing, so `latest`
may name an earlier commit than the branch head. A host deploys the immutable digest that run reported, never a tag.
The image is not signed; the [delivery specification](docs/specs/delivery.md)
states why and what stands in its place. Its base images are Docker Hardened
Images, so *building* it requires `docker login dhi.io`; deploying it does not,
because a host pulls rather than builds.
The host owns the private boundary: Docker publishes only `127.0.0.1:8080` and
the readiness probe's own `127.0.0.1:8081`, while host-level Tailscale Serve
provides HTTPS for the first of those and is the origin a Cloudflare Tunnel dials
by Tailscale Service name. Readiness is never served through it, so it stays a
host-local health check rather than an endpoint on the public surface. Tailscale does not run in the image.

The [Linux VM guide](docs/hetzner.md) covers the long-running host. The
[Cloudflare Access guide](docs/cloudflare-access.md) covers how the single
operator reaches it — the only way in, and without publishing a listener. The
[operator recovery runbook](docs/runbook.md) covers the uncommon moments a
running deployment asks for a person. The optional sandbox encoder check is
documented in [the FIT acceptance guide](docs/fit-sandbox-acceptance.md).
