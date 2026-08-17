# domestique

`domestique` mirrors one private VeloPlanner route library to two Wahoo
accounts. It is a single-tenant service intended to run as a Docker container
on a Raspberry Pi in a Tailnet.

The accepted v1 contract is in [the service specification](docs/specs/service.md).
The supporting specifications define configuration, sync safety, architecture,
and delivery before implementation begins.

## Development

Install the pinned local toolchain and run the complete quality gate:

~~~sh
mise install
mise exec -- make check
~~~

Install the optional Git hook with `mise exec -- prek install`. See
[CONTRIBUTING.md](CONTRIBUTING.md) for the contributor workflow. Copy
[`config.example.toml`](config.example.toml) outside the repository when
preparing a local deployment; it references secret files and never embeds
secret values. The current implementation validates static configuration and
persists encrypted state, but does not start the HTTP, OAuth, or sync flows yet.
