---
name: domestique-backlog
description: "Create, triage, label, and maintain the nobbs/domestique GitHub issue backlog. Use when organising issues, priorities, or automation readiness; not when implementing an issue."
---

# Domestique Backlog

Maintain a small, trustworthy issue queue for human and automated delivery.

## Read before writing

- Inspect live issues, labels, comments, linked pull requests, and repository
  instructions before creating, relabelling, closing, or deduplicating work.
- Preserve issue discussion and do not make external writes unless the user has
  asked for them. For a batch change, preview the exact issue set and label
  changes first unless the user already specified them exactly.
- Keep GitHub Issues as the source of truth. Do not create or modify a GitHub
  Project unless the user explicitly asks.

## Label policy

Use the existing type labels (`enhancement`, `bug`, `documentation`, and
`accessibility`) plus this lean taxonomy:

- Area: `area:ui`, `area:sync`, `area:integration`, `area:platform`, or
  `area:devex`. Apply only the areas that materially own the work.
- Priority: exactly one of `priority:now`, `priority:next`, or
  `priority:later`.
- Automation: `agent-ready` only when the issue can be completed without an
  unresolved decision, credentials, live provider access, third-party app
  installation, repository-policy choice, or other operator authority.
- Status: `status:in-progress` records that a delivery session has claimed the
  issue. Delivery writes and removes it against a claim comment; triage does
  not. Never add it to reserve work, and never remove it to unblock a pickup.

Never promote an issue to `priority:now` without explicit user direction.
Default speculative ideas to `priority:later`; use `priority:next` only for
deliberately selected work.

## Create and triage issues

- Keep one independently reviewable outcome per issue. Search for duplicates
  first and link rather than restate overlapping work.
- Write a concise problem statement followed by scope, acceptance criteria,
  safety or privacy constraints, and validation expectations when they matter.
- Call out required specification revisions for API, persistent-state,
  synchronisation, safety, or deployment-contract changes.
- Do not place credentials, real route data, deployment details, raw provider
  responses, or internal filesystem paths in an issue.
- Write issue and pull-request bodies **unwrapped**: one line per paragraph, one
  line per list item, no hard wrap at any column. GitHub reflows prose to the
  reader's width, so a hard-wrapped source only makes later edits awkward. Code
  fences, tables, and blockquotes keep their own line structure.
- When an issue becomes ambiguous or blocked by authority, remove neither its
  history nor its priority automatically; explain the condition and wait for a
  user decision.

## Housekeeping

Keep labels mutually consistent, remove obsolete duplicate labels only with
user approval, and verify every external write by reading it back. Treat
`status:in-progress` on an issue with no live claim comment and no open pull
request as a dead session's leftover, and clear it only with user approval. Treat a
pull request as implementation progress, not automatic closure or proof that
the issue is ready to merge.
