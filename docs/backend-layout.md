# Backend layout

The shape `internal/` should take, and what the architecture specification has
to say before any of it moves.

**What remains here has not been applied.** The specification's tree, the
`internal/sqlite/store.go` split and the interface-and-type headers in
`handler.go`, `service.go` and `client.go` are done and are gone from this
document. What is left is section 3: grouping `internal/` by role, which needs
the architecture specification revised before any code moves.

`config.go` (514 lines) and what is now `internal/route/route.go` (409) were the
survey's "if they still read long afterwards" item. They do not: each is one
package's whole subject, told once, and splitting either would divide a single
topic rather than separate two.

Every claim carries the file it was read from. Sizes and counts are from the
commit this was written against and will drift; the shapes will not.

## Grouping `internal/`

`internal/` becomes four groups by role, with four packages left flat.

~~~text
internal/
├── core/            route, sync, oauth, schedule,
│                    elevation, surface, ridemodel, readiness
├── upstream/        veloplanner, komoot, wahoo,
│                    openmeteo, pushover, osmindex
├── adapter/         sqlite, fit, cfaccess
├── serve/           httpapi, webui
├── config/          runtimeconfig/
└── build/           demo/
~~~

### The membership rules

Each group needs a rule crisp enough that the next package has a home without a
debate:

- **`core/`** — owns a rule about routes or synchronisation and performs no I/O
  of its own. This is the set the specification already describes as
  "independent of HTTP, SQLite, and upstream protocols".
- **`upstream/`** — makes outbound calls to somebody else's service. Six
  packages, and the rule admits no argument about any of them.
- **`adapter/`** — wraps infrastructure this service depends on but does not
  own: the database, the FIT encoding, the identity check.
- **`serve/`** — what the service exposes to a caller.

`config`, `runtimeconfig`, `build` and `demo` stay flat. They are the residue:
configuration is read by every layer and belongs to none, and `build` and `demo`
are not product code at all. A two-member group named for what its members are
*not* would read worse than four flat entries.

### What the specification must say first

Three edits to
[implementation-architecture.md](specs/implementation-architecture.md), in the
same change as the move and not after it:

1. The directory tree, replaced with the one above — including the `api/`,
   `deploy/` and `dev/` corrections from the top of this document.
2. The sentence at line 64. As written it forbids this grouping outright, and
   its purpose — keeping out `common`, `models` and `repository` packages that
   own nothing — survives a rewrite that permits grouping by role while still
   refusing grouping by nothing.
3. ~~The responsibility table, which covers 19 packages and omits four:
   `build`, `cfaccess`, `demo` and `openmeteo`.~~ Already added, along with the
   six packages the tree itself omitted.

### What it costs

The Go half is mechanical and safe: **61 of 147 files** import at least one
moved package, across **110 import lines**. A rename tool plus `goimports`
handles it, and the compiler proves the result.

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
[implementation-architecture.md](specs/implementation-architecture.md). Nothing
there is hard, but a missed path in `paths-filter.yml` or `codecov.yml` fails
open — the job simply stops running against those files — so it is the one move
that cannot be proved by compiling.

That is why `serve/webui` is sequenced last and alone. Every other package in
the tree moves under compiler protection.

## Suggested order

1. The specification revision above — the tree is already correct and the four
   missing table rows are already added, so what remains is the line-64 rule —
   followed by the moves under compiler protection: `core/`, `upstream/`,
   `adapter/`, and `serve/httpapi`.
2. `serve/webui` last and on its own, with the eleven non-Go files it is named
   in. It is the only move whose correctness the compiler cannot check.
