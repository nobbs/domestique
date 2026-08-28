# Naming drift

Where the code disagrees with [the glossary](glossary.md), why, and what the fix
would be.

**What remains here has not been applied.** An entry is deleted as its rename
lands, so this file is the outstanding gap rather than a history — the reasoning
for an applied one is in the pull request that applied it. The renames are
listed so a later change can execute them deliberately, in an order that keeps
the wire, the database and the interface in step, rather than one file at a time
as each is noticed.

Everything the survey found has been applied except item 11 below. The
frontend-only renames went first, then the interface wording, then `route`
versus `source route` across the wire, the Go tree and the specifications.

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
`"source" | "targets"`: the asymmetry reflects the domain — one configured
source section, up to two target slots — and `SyncPhases` holds exactly one
`SyncPhaseRun` for each, so nothing reads the plural as a count. Making it
grammatically consistent would break `api/openapi.yaml`, `SYNC_PHASES` and Go's
`PhaseSource`/`PhaseTargets` for a cosmetic gain.

## 14. `Route` collides with react-router

`react-router` exports `Route` and `Routes`; `api/types.ts` exports a domain
`Route`. No file imports both today, but `features/routes/` is named for the
domain concept while `App.tsx`'s `<Routes>` is the router's.

**Proposed:** nothing. The collision is real but inert, and renaming either side
costs more than it saves. Noted so the next person to hit it knows it was
considered.

## Suggested order

Item 11 is a prose fix in Go comments and touches nothing else, so it can go on
its own whenever somebody separates the sync-phase sense of *half* from the
ordinary English one the rest of the tree uses.
