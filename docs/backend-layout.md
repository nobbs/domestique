# Backend layout

The shape `internal/` should take, and what the architecture specification has
to say before any of it moves.

**What remains here has not been applied.** What is left is grouping `internal/`
by role, which needs the architecture specification revised before any code
moves.

`config.go` and `internal/route/route.go` are each one package's whole subject
and are not split.

Every claim carries the file it was read from. Sizes and counts are from the
commit this was written against and will drift; the shapes will not.

## Grouping `internal/`

`internal/` becomes four groups by role, with four packages left flat.

~~~text
internal/
├── core/            route, sync, oauth, session, schedule,
│                    elevation, surface, ridemodel, readiness
├── upstream/        veloplanner, komoot, wahoo,
│                    openmeteo, pushover, osmindex
├── adapter/         sqlite, fit, auth0
├── serve/           httpapi, webui
├── config/          runtimeconfig/
└── build/           demo/
~~~

### The membership rules

- **`core/`** — owns a rule about routes or synchronisation and performs no I/O
  of its own. This is the set the specification describes as "independent of
  HTTP, SQLite, and upstream protocols".
- **`upstream/`** — makes outbound calls to somebody else's service.
- **`adapter/`** — wraps infrastructure this service depends on but does not
  own: the database, the FIT encoding, the identity check.
- **`serve/`** — what the service exposes to a caller.

`config`, `runtimeconfig`, `build` and `demo` stay flat. Configuration is read
by every layer and belongs to none, and `build` and `demo` are not product code.

### What the specification must say first

Two edits to
[implementation-architecture.md](specs/implementation-architecture.md), in the
same change as the move and not after it:

1. The directory tree, replaced with the one above.
2. The rule forbidding a `common`, `models` or `repository` package. As written
   it forbids this grouping outright. It is rewritten to permit grouping by role
   while still refusing grouping by nothing.

### What it costs

The Go half is mechanical: **61 of 147 files** import at least one moved
package, across **110 import lines**. A rename tool plus `goimports` handles it,
and the compiler proves the result.

The non-Go half is not evenly spread, and one package carries almost all of it:

| Package | Non-Go references |
| --- | --- |
| `webui` | 86, across 11 files |
| `httpapi` | 5 |
| `route` | 1 |
| `sqlite` | 0 |

`internal/webui` is named in `.github/workflows/ci.yml`,
`.github/paths-filter.yml`, `codecov.yml`, `.gitleaks.toml`, `.mise.toml`,
`mise-tasks.toml`, `playwright.config.ts`, the e2e fixtures, `AGENTS.md`, and
both [delivery.md](specs/delivery.md) and
[implementation-architecture.md](specs/implementation-architecture.md). A missed
path in `paths-filter.yml` or `codecov.yml` fails open: the job stops running
against those files. It is the one move that cannot be proved by compiling.

`serve/webui` is therefore sequenced last and alone. Every other package in the
tree moves under compiler protection.

## Suggested order

1. The specification revision above, followed by the moves under compiler
   protection: `core/`, `upstream/`, `adapter/`, and `serve/httpapi`.
2. `serve/webui` last and on its own, with the eleven non-Go files it is named
   in. It is the only move whose correctness the compiler cannot check.
