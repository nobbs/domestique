# Contributing

Domestique is a private single-tenant service. The accepted contracts in
[`docs/specs`](docs/specs) define v1 behavior; a change must not silently
weaken their access, deletion, or secret-handling rules.

## Local checks

GitHub Actions is the authoritative gate: it runs the complete validation for
every changed path on every pull request, and its aggregate check is what a
merge must satisfy. Local checks exist to give you that answer sooner.

Install the project-local tools and run the routine loop:

~~~sh
mise install
mise exec -- make quick
~~~

`make quick` runs everything the full gate runs except three checks it defers —
`build-check`, which cross-compiles both published architectures, and
`vulncheck` and `ui-audit`, which need the network and a current advisory
database. That difference is asserted, not just documented, so a check added to
the gate cannot quietly drop out of the routine loop.

Run the full gate yourself with:

~~~sh
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

## Coverage

Coverage is a report here, not a gate: neither `quick`, nor `check`, nor GitHub
Actions fails on a percentage. The number exists so that a change can be read
with its untested parts visible.

On a pull request, CI measures both languages and publishes them to Codecov
under two flags, `go` and `ui`, so that a shortfall names the language it came
from. The statuses it posts are informational and the upload does not gate the
required check, so a Codecov outage, or a pull request from a fork that has no
upload token, costs a report rather than a merge.

Locally:

~~~sh
mise exec -- make coverage
~~~

That writes a Go coverage profile to `.coverage/go.out` and the browser UI's
LCOV report to `.coverage/ui/lcov.info`, and prints a summary for each. Nothing
is committed — `/.coverage/` is gitignored. `make coverage-go` and
`make coverage-ui` run one side alone.

The Go profile is collected with `-coverpkg` over `./cmd/...` and
`./internal/...`, so a function exercised only through another package's tests
counts as covered rather than dead. The same flag makes the percentage `go test`
prints on each of its own lines meaningless — that one is the fraction of the
whole service one package's tests reached — so read the per-package summary
printed after the run, which `dev/coveragesummary` produces from the merged
profile.

Four things are deliberately not measured:

| Not measured | Why |
| --- | --- |
| `dev/` | Repository tooling rather than the service. It has its own tests, and they run in the normal suite. |
| `src/**/*.test.{ts,tsx}` and `src/test/` | The tests and their harness are the measurement, not its subject. |
| `src/**/*.d.ts` | Type-only declarations emit no runtime statement to reach. |
| `src/main.tsx` | It mounts React onto a real document and does nothing else, so a test of it would assert the framework rather than this UI. |

Everything else is measured, including code that no unit test can reach today.
The map components need WebGL, and the page-level components are assembled
rather than unit-tested, so both report low — which is the honest answer.
Browser-level coverage for them is tracked as its own work; excluding them would
hide a real gap behind a comfortable number.

### Making coverage enforceable

Nothing here blocks a merge yet, deliberately: a gate switched on the same day
the first report arrives cannot be told apart from a broken upload. Turning one
on is three deliberate steps, and `codecov.yml` and the CI workflow are written
so that each is a small edit rather than a redesign.

1. Choose what is measured. A **patch** target — the lines a change adds or
   alters — is the one worth enforcing. A project-total target fails on the
   legitimate deletion of well-covered code and on a refactor that moves
   statements between packages, which teaches contributors to route around it
   rather than to write a test.
2. Drop `informational: true` from the status it should apply to in
   `codecov.yml`. Each flag has its own project and patch status, so Go and the
   UI can be enforced separately and at different levels.
3. Make the verdict blocking. The branch ruleset requires exactly one status
   context, `Required`, which reports what the `Coverage` job did but does not
   fail with it; the comment above its validation step says what to add. The
   alternative is to require Codecov's own status contexts in the ruleset
   instead, which needs a repository-settings change rather than a repository
   one.

Whatever the rule ends up being, it needs an exception path and somewhere to
record one. A gate with no legitimate override gets bypassed by an
administrator merge instead, which is worse than no gate.

## Contributions

Keep changes focused, add regression tests for behavior changes, and avoid
network access in normal tests. Do not commit credentials, tokens, static
configuration, private route data, generated FIT files, SQLite state, or
deployment files from the Raspberry Pi.

Use Conventional Commits. Pull requests should explain any changes to the
normative specifications, safety gates, or operator action required for
deployment.
