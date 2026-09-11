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

Do not report that code fails to compile, that a symbol does not exist, or that a
language or library feature is unavailable. Nothing here runs a compiler, CI
compiles every pull request before it can merge, and every such claim made on this
repository so far has been wrong.

Judge the code against the versions this repository pins, never against older ones:
the Go toolchain in `go.mod`, everything else in `.mise.toml`. Recent Go language
changes have repeatedly been reported as errors. `new` accepting a value rather
than a type is the standing example.

## Out of scope

- The pull request description. Review the code.
- Asking for more tests or more coverage. Codecov's patch status gates that, and it
  measures what this reviewer cannot see.
- Style, naming and formatting. Linters and formatters own those and run in CI.

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
