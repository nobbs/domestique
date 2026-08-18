---
name: domestique-issue-delivery
description: "Select and deliver one agent-ready GitHub issue for nobbs/domestique, from issue triage through a review-ready pull request. Use when asked to pick the next ready issue or implement a labelled Domestique issue; not for backlog administration."
---

# Domestique Issue Delivery

Deliver one focused issue without broadening its scope.

## Select one issue

- Work only on an open issue carrying `agent-ready`. Do not infer readiness from
  an issue title, priority, or lack of objections.
- Read the live issue body, labels, comments, linked work, and relevant open
  pull requests before choosing it. Do not duplicate active work.
- Prefer `priority:now`, then `priority:next`, then `priority:later`. Within a
  priority, choose the oldest clear issue unless the user names one.
- If a selected issue has unresolved product choices, requires secrets or a
  live provider, installs a third-party GitHub App, changes repository policy,
  or otherwise needs operator authority, stop and report that the label is
  inconsistent. Do not silently remove or change labels.
- If there is no eligible issue, report that result and do not broaden the
  search or invent work.

## Deliver it

1. Read the repository's current `AGENTS.md`, the full issue, and every
   applicable normative specification before changing code. The issue describes
   scope; repository instructions define the delivery contract.
2. Inspect the worktree before changing branches. Preserve unrelated work and
   use a separate worktree when that is safer.
3. Implement the smallest change that satisfies the issue acceptance criteria.
   Preserve the service's access, deletion, secret, and geometry boundaries;
   revise a specification in the same change when the contract changes.
4. Add deterministic regression coverage. Run the current required validation
   from the repository instructions; report any unrun map visual inspection or
   blocked external acceptance check plainly.
5. When the user asked for end-to-end delivery, create a focused branch, commit
   and pull request following `AGENTS.md`, link the issue with `Closes #<n>`,
   request formal Copilot review, and verify the requested-reviewer state. Do
   not merge unless the user explicitly authorises it.

## Report

State the selected issue, why it won selection, the implementation and
validation result, and any remaining operator action. Do not change backlog
labels or close the issue merely because a pull request exists.
