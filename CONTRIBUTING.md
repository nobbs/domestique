# Contributing

Domestique is a private single-tenant service. The accepted contracts in
[`docs/specs`](docs/specs) define v1 behavior; a change must not silently
weaken their access, deletion, or secret-handling rules.

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

The hook judges a commit on the files it stages: Go formatting and Markdown
lint see the staged files alone, so a commit never fails on a defect in a file
it did not touch. It deliberately runs no tests, no full linting, no audit, no
cross-compilation, no image build, and no browser suite — that work belongs to
`make check` and to GitHub Actions, and `make hook-check` fails if it appears
in `prek.toml`. Keeping the hook to roughly a second is what keeps it worth
leaving installed.

## Contributions

Keep changes focused, add regression tests for behavior changes, and avoid
network access in normal tests. Do not commit credentials, tokens, static
configuration, private route data, generated FIT files, SQLite state, or
deployment files from the Raspberry Pi.

Use Conventional Commits. Pull requests should explain any changes to the
normative specifications, safety gates, or operator action required for
deployment.
