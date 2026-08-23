# Semgrep rules

This directory holds every rule `mise run semgrep` runs. There is no
`--config auto` and no `p/...` registry pack: CI never fetches a rule set at
scan time, only what is committed under [`rules/`](rules/).

## What is here and why

Semgrep is not a second gosec. `.golangci.yml` already runs gosec, staticcheck,
and friends over the Go tree, and `npm audit` covers dependency advisories for
the browser UI; duplicating either would just be baseline noise the acceptance
criteria for this check explicitly rules out. Each rule file exists to cover a
gap none of those tools reach:

- [`go-architecture.yaml`](rules/go-architecture.yaml) — two invariants from
  `AGENTS.md` and `docs/specs/implementation-architecture.md` that are
  structural (call sites, import graphs) rather than syntactic, so no linter
  already enforces them: only a `main.go` may call `os.Exit` or `log.Fatal*`,
  and an adapter package (`veloplanner`, `komoot`, `fit`, `wahoo`, `sqlite`,
  `pushover`) may not import another adapter or an application-layer package.
- [`typescript-security.yaml`](rules/typescript-security.yaml) — client-side
  injection sinks (`dangerouslySetInnerHTML`, `eval`, `document.write`) that
  Biome's recommended preset and `tsc` do not flag. Nothing in the tree uses
  any of them today.
- [`workflow-security.yaml`](rules/workflow-security.yaml) — GitHub Actions
  script injection: an attacker-controlled context value (a PR title, branch
  name, or issue/comment body) template-interpolated straight into a `run:`
  block. `actionlint` checks workflow syntax and shells out to shellcheck per
  step; neither looks for this.

## Adding or changing a rule

1. Write the rule under `rules/`, in its own file if it covers a new gap, or
   alongside the existing entries for the same one.
2. Give every rule a `message` that names the concrete risk and the fix, not
   just what pattern matched — that message is what a contributor sees at the
   call site.
3. Validate it locally before committing:

   ```sh
   mise run semgrep
   ```

   A false positive against the current tree fails that run immediately.
   Prove a true positive separately, against a throwaway fixture outside the
   repository — `semgrep --config rules/ <fixture>` — since there is
   deliberately nothing in this tree for the rules to match.
4. Keep an exclusion narrow and next to the rule it modifies, with a comment
   naming what it protects, the same standard `.golangci.yml` already holds
   its own exclusions to.

## Running it

`mise run semgrep` is part of `quick` and `ci-security`, so it runs in the
routine local loop and in the GitHub Actions security job on every pull
request. It needs no network access and no unpinned tool: the version lives in
`.mise.toml` next to every other pinned CLI.
