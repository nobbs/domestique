---
name: domestique-issue-delivery
description: "Select, claim, and deliver one agent-ready GitHub issue for nobbs/domestique, from issue triage through a review-ready pull request. Use when asked to pick the next ready issue or implement a labelled Domestique issue; the claim keeps parallel sessions off the same issue. Not for backlog administration."
---

# Domestique Issue Delivery

Deliver one focused issue without broadening its scope.

## Select one issue

- Work only on an open issue carrying `agent-ready`. Do not infer readiness from
  an issue title, priority, or lack of objections.
- Read the live issue body, labels, comments, linked work, and relevant open
  pull requests before choosing it. Do not duplicate active work.
- Skip an issue that carries `status:in-progress` or already has an open linked
  pull request. Another session holds it. Reclaim a stale claim only under the
  rule below.
- Prefer `priority:now`, then `priority:next`, then `priority:later`. Within a
  priority, choose the oldest clear issue unless the user names one.
- If a selected issue has unresolved product choices, requires secrets or a
  live provider, installs a third-party GitHub App, changes repository policy,
  or otherwise needs operator authority, stop and report that the label is
  inconsistent. Do not silently remove or change labels.
- If there is no eligible issue, report that result and do not broaden the
  search or invent work.

## Claim it before working

GitHub has no atomic label write, so two sessions can read the same issue as
free within the same second and both label it. Claim through the comment log,
which is ordered and stable, and treat `status:in-progress` as the visible
marker of a claim that already succeeded.

1. Mint a claim id unique to this session: `claim:$(openssl rand -hex 4)`.
2. Post exactly one comment on the chosen issue, first line verbatim:

   ~~~text
   Delivery claim <claim-id> by domestique-issue-delivery at <UTC RFC 3339>.
   ~~~

3. Re-read every comment on the issue and collect the live claims — a claim is
   live unless a later comment releases or supersedes it. The lowest comment id
   wins, whatever the wall-clock timestamps say.
4. If another live claim won, delete your own claim comment, leave the issue
   untouched, and select the next candidate. Never delete or release another
   session's claim comment, and never take an issue you lost.
5. If you won, add `status:in-progress`, then read the label and the comment
   back before changing any code.

Comment ids are ordered only in the API, not in the web view. List them with
`gh api repos/nobbs/domestique/issues/<n>/comments --jq '.[] | [.id, .created_at]
| @tsv'`, and delete a lost claim with
`gh api --method DELETE repos/nobbs/domestique/issues/comments/<id>`.

Claim an issue the user named by number the same way. If a live claim already
holds it, report that and stop rather than working the issue in parallel.

## Hold and release the claim

- Hold the claim for the whole delivery, including while the pull request is in
  review. The issue stays open until that pull request merges, and the label is
  what keeps a second session off it in the meantime.
- Release when you stop short of delivery — you abandon the issue, hand it back
  for operator authority, or find `agent-ready` inconsistent. Post
  `Delivery claim <claim-id> released: <reason>.` and remove
  `status:in-progress`. Report the reason to the user too.
- Leave the label in place once the issue closes. Selection only considers open
  issues, so a closed issue needs no cleanup.
- A claim is stale when its comment is more than 4 hours old and the issue has
  no open linked pull request. Reclaim one by posting
  `Delivery claim <old-id> superseded by <new-id>: stale since <timestamp>, no
  open pull request.` and then claiming from step 3 above. A claim younger than
  that is contention, not a leftover: report it and choose another issue.

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

## Update a delivery pull request

- Before rebasing or resolving a conflict, inspect the selected issue's pull
  request, its intended base, the branch topology, and the worktree. Never
  overwrite unrelated local work.
- Rebase only a branch that belongs to the selected delivery when its base has
  advanced. Resolve a conflict only when the smallest semantic resolution still
  meets that issue's acceptance criteria. Stop for direction if resolving it
  would change another feature, specification, or product decision.
- After a rebase or conflict resolution, repeat the validation affected by the
  changed code before pushing. When rewriting an authorised delivery branch,
  use `--force-with-lease`, never an unguarded force push.
- Treat material follow-up commits as a new review request: re-request formal
  review through GitHub's requested-reviewer mechanism and verify it actually
  registered. Report unavailable review automation rather than substituting a
  comment or claiming it succeeded.
- Keep one issue to one pull request by default. Create or maintain a stacked
  pull request only when the user asks or the issue naturally splits into an
  ordered chain of independently reviewable layers. Propose those layers before
  creating the stack, keep it linear, update a lower layer before rebasing its
  successors, and never merge it without explicit user authorisation.

## Report

State the selected issue, why it won selection, the claim id you hold or
released, the implementation and validation result, and any remaining operator
action. Do not change backlog
labels or close the issue merely because a pull request exists.
