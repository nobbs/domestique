# Contributing

Domestique is a private single-tenant service developed in the open. The
accepted contracts in [`docs/specs`](docs/specs) define v1 behavior; a change
must not silently weaken their access, deletion, or secret-handling rules.

## Local checks

Install the project-local tools and run the full gate:

~~~sh
mise install
mise exec -- make check
~~~

Install the optional local Git hook with:

~~~sh
mise exec -- prek install
~~~

`make fmt` applies Go formatting. The Git hook may also make safe whitespace
repairs and exits non-zero so they can be reviewed and staged deliberately.

## Contributions

Keep changes focused, add regression tests for behavior changes, and avoid
network access in normal tests. Do not commit credentials, tokens, static
configuration, private route data, generated FIT files, SQLite state, or
deployment files from the Raspberry Pi.

Use Conventional Commits. Pull requests should explain any changes to the
normative specifications, safety gates, or operator action required for
deployment.
