# Naming drift

Where the code disagrees with [the glossary](glossary.md), and what the fix
would be.

**What remains here has not been applied.** An entry is deleted as its rename
lands, so this file is the outstanding gap. The renames are listed so a later
change can execute them deliberately, in an order that keeps the wire, the
database and the interface in step.

Every claim below carries the file it was read from. Line numbers are from the
commit this document was written against and will drift; the identifiers will
not.

## 11. Phases are called halves in prose

`SyncPhase` is `"source" | "targets"` — singular for one, plural for the other —
while the prose around it calls a phase a *half*. No identifier uses "half",
and the comments mix the two: `internal/sync/reporter.go:61` reads "phase names
the half being run right now", and `:277` reads "The phase is a parameter so
each call".

**Proposed:** *phase* in both the comments and the identifiers. The enum keeps
`"source" | "targets"`. The asymmetry reflects the domain — one configured
source section, up to two target slots — and `SyncPhases` holds exactly one
`SyncPhaseRun` for each, so nothing reads the plural as a count. Grammatical
consistency here would break `api/openapi.yaml`, `SYNC_PHASES` and Go's
`PhaseSource`/`PhaseTargets` for a cosmetic gain.

## 14. `Route` collides with react-router

`react-router` exports `Route` and `Routes`; `api/types.ts` exports a domain
`Route`. No file imports both today, but `features/routes/` is named for the
domain concept while `App.tsx`'s `<Routes>` is the router's.

**Proposed:** nothing. The collision is inert, and neither side is renamed.

## 15. `stage` still names the unit inside the packages that consume it

The type, the wire and the specifications say *route*. *Stage* is kept where the
glossary keeps it: the provider adapters, the SQLite schema and its methods, and
`stageOrder`. The consuming packages still use *stage* for the value they are
handed. `internal/sync`'s `Encoder` reads
`Encode(ctx context.Context, stage route.Route)`, and `internal/surface` and
`internal/ridemodel` name locals the same way, while the type they hold is
`route.Route`.

| Package | Non-test | Test |
| --- | --- | --- |
| `sync` | 92 | 108 |
| `surface` | 40 | 32 |
| `ridemodel` | 40 | 47 |
| `httpapi` | 39 | 79 |
| `route` | 6 | 28 |
| `fit` | 5 | 13 |

None of it crosses a boundary; these are parameter names, locals and prose.
`internal/elevation` is done, and `sync.Processor` moved with it, so the
interface and its one implementation agree.

**Proposed:** *route* for the value, in identifiers and in prose, across all
six. *Stage* survives only in `stageOrder`, in the storage and provider names
the glossary blesses, and in the frozen `:stage:` segment of a Wahoo external
ID. It is done as one pass: a package half converted reads worse than one not
converted at all. It is done away from a rename that also moves the wire, so the
compiler proves the whole thing.

## Suggested order

1. Item 15, which the compiler checks end to end and touches no contract.
2. Item 11, a prose fix in Go comments, whenever somebody separates the
   sync-phase sense of *half* from the ordinary English one the rest of the
   tree uses.
