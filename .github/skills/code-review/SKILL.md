---
name: code-review
description: How to review a pull request in this repository — what counts as a finding, which contracts and safety gates this codebase holds, and which claims this reviewer cannot verify and must not make.
---

# Code review

## Report a finding once

One defect gets one comment, on the most specific line it applies to. Where the
same mistake repeats across files, say so in that one comment and name the other
locations. Seven comments making a single point cost seven rounds to resolve and
bury whatever else was found.

## Claims this review cannot verify

Do not report that code fails to compile, or that a language or library feature is
unavailable, on the strength of your own reading of the language. Nothing here runs
a compiler, and every such claim made on this repository so far has been wrong. A
missing reference you established by searching the repository is different: say
where you looked and report it.

CI compiles the tree when a change touches Go. It is path-filtered, so a
documentation or UI-only pull request skips those jobs entirely and nothing there
will catch a Go mistake.

Judge the code against the versions this repository pins, never against older ones.
`.mise.toml` pins the toolchains and command-line tools, `go.mod` the language
version the module requires and its dependency versions, and the UI's
`package.json` and lockfile the versions of everything it imports. Recent Go language
changes have repeatedly been reported as errors here. `new` accepting a value
rather than a type is the standing example.

## Out of scope

- Wording in the pull request description. Review the code. The exception is a
  description that contradicts the change where a specification or a safety gate is
  involved: say so, because that misleads every later reader.
- Asking for more coverage. Codecov's patch status gates that and measures what this
  reviewer cannot see. A behaviour change carrying no regression test is still worth
  reporting, because `AGENTS.md` requires one.
- Mechanical style and formatting. Linters and formatters own those and run in CI.
  Domain naming is not mechanical: `docs/glossary.md` fixes each domain word's one
  meaning and `docs/naming-drift.md` records where the code disagrees, and no linter
  can judge either. A new name that contradicts the glossary is worth reporting.

## What is worth reporting

Correctness, security, concurrency, and efficiency serious enough to matter in
production, roughly in that order.

Two classes deserve a deliberate look, because they are what actually ships here:

- Comments and documentation asserting behaviour the code does not have. This is
  the most common real defect in this repository. Re-read the code under every
  claim a comment makes.
- Both ends of a bound. Wherever a value is bounded, ask about the other end: a
  response bounded above but not below, a clamp that admits zero, a cap with no
  floor.

## This repository's contracts

`docs/specs` is normative. Where code contradicts a spec, the spec is correct and
the code is the defect; name the spec and the line.

`AGENTS.md` lists the safety rules that must not be weakened: ownership before
deletion, the deletion gates, a failed source inventory never being destructive,
secrets staying out of logs and errors, admin-only endpoints, geometry served only
by its own endpoint, refresh tokens encrypted at rest. A change that quietly
weakens one of these is the most serious thing to find here.

When a change alters one field's type, shape or meaning, check its siblings in
`api/openapi.yaml` and in the generated clients. A type migrated in one model and
not in the ones beside it has shipped here before.
