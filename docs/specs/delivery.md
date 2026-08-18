# Domestique delivery specification

**Status:** accepted v1 design, revised for trunk-based image publishing and
for a fast routine local loop with GitHub Actions as the authoritative gate

This is a subordinate specification to [the service contract](service.md). It
defines the local quality gate, GitHub Actions, container hardening, the images
it publishes, and how one reaches a host. It places no Tailnet-host
configuration or secret in this repository: the one host-side file it does carry
is a non-secret deployment script that reads every host-specific value from the
environment it runs in.

## Development toolchain and commands

The repository tracks a `.mise.toml` that pins the Go toolchain, Node.js,
`golangci-lint`, `prek`, Actionlint, Gitleaks, Markdownlint, ShellCheck, and
`govulncheck`. Mise is the source of truth for the versions used by a
developer and GitHub Actions. Bootstrap selects supported, mutually compatible
versions and records them there; no developer-global tool configuration is
required.

The repository provides these stable Make targets:

| Target | Contract |
| --- | --- |
| `make fmt` | Applies Go formatting to owned Go source. |
| `make lint` | Runs the configured `golangci-lint` checks. |
| `make test` | Runs the normal deterministic test suite with `CGO_ENABLED=0`. |
| `make build` | Compiles the Linux service binary for `BUILD_ARCH`, the machine's own architecture unless overridden, with `CGO_ENABLED=0`, after building the browser UI. |
| `make ui-build` | Builds the browser UI bundle that the binary embeds. |
| `make ui-dev` | Runs the UI dev server, proxying the API to the local service. |
| `make dev-setup` | Snapshots the deployed state into an isolated development environment. |
| `make dev-api` | Serves the API against that snapshot on `:8081`. |
| `make quick` | Runs the routine local loop: every check in `make check` except the three it defers. |
| `make check` | Runs the full gate locally, on demand. |

`make check` is the full gate. It includes
`prek run --all-files`, linting, tests, TypeScript type checking, the browser UI
lint and test suites, Go module verification, vulnerability analysis for both Go
and npm dependencies, a GitHub Actions workflow check, a shell-script check, a
worktree secret scan, a commit-hook cost check, a local-gate structure check,
and the release-target binary compilation for every published architecture.
`make fmt` applies Go formatting. A fixing `prek` hook exits non-zero after a
safe mechanical repair so the resulting change can be reviewed and staged
deliberately.

### The authoritative gate is GitHub Actions

**This revises the earlier contract that `make check` was the canonical local
and CI entry point, run in full before every hand-over.** It is a deliberate
change to where the gate is paid, not to what the gate contains: GitHub Actions
runs the complete validation on every pull request targeting the default branch,
and its aggregate check is what a merge must satisfy. No check was removed,
relaxed, or made optional in order to obtain a faster local loop.

Local validation is therefore a way to learn a result earlier, and it comes at
two depths:

- `make quick` is the routine loop. It runs everything the full gate runs except
  the three checks named below, so it stays worth running on every iteration.
- `make check` is the full gate on demand, unchanged. Run it before handing work
  over when an earlier answer than CI's is worth its cost.

`make quick` defers exactly three checks, each because it is slow or needs the
network rather than because it matters less:

| Deferred check | Why |
| --- | --- |
| `build-check` | Rebuilds the UI bundle and cross-compiles both published architectures; the slowest check in the gate whenever the build cache is cold. |
| `vulncheck` | Needs the network and a current Go advisory database. |
| `ui-audit` | Needs the network and a current npm advisory database. |

It also reuses an installed browser UI dependency tree rather than reinstalling
it, and installs one only when none is present. `make check` and CI always
install from a clean `npm ci`, so a lockfile change is still proved against a
fresh tree.

That `make quick` is a strict subset of `make check`, and that the difference is
exactly the deferred set above, is asserted rather than only documented: a check
added to the full gate fails the assertion until it is either added to the
routine loop or deferred deliberately. The routine loop may therefore be
narrower than the gate, but never different from it.

The three depths compose: the commit hook judges the staged files in about a
second, `make quick` judges the working tree, and GitHub Actions judges the
merge. Each is a strict subset of the one after it.

`prek` owns fast repository hygiene: whitespace and end-of-file checks,
private-key and accidental-large-file checks, YAML, TOML, and Markdown
validation where applicable, and Go formatting. Developers may install the
`prek` hook, but the hook is a convenience rather than a substitute for
`make check`. The project uses `prek`, never `pre-commit`.

The installed hook is bounded by what it may do, because a hook slow enough to
be bypassed protects nothing. A hook that runs a command takes its file list
from `prek`, so a commit is judged on what it stages and not on the rest of the
tree; the same configuration under `prek run --all-files` covers the repository,
which is how the gate keeps its reach. Tests, full linting, audits,
cross-compilation, image work, and the browser suites stay out of the hook and
belong to `make check` and GitHub Actions. Both properties are asserted, so a
later hook cannot quietly reintroduce the cost; wall-clock is not asserted,
because a timing assertion on a shared runner is flaky.

## Development environment

`make dev-setup` prepares an environment that shows real route data without
risking the deployment. It copies the deployed SQLite state into `.local/dev`
and writes a configuration beside it, so the development service and the
deployed container never share a database file.

That environment may read VeloPlanner, so a manual `POST /v1/sync` refreshes
real routes and geometry. It must never reach Wahoo, which is enforced in depth:
the state encryption key is a placeholder, so the stored refresh tokens cannot
be decrypted and a run therefore fails at the state step, which precedes any
Wahoo request; the Wahoo endpoints additionally point at an unroutable address;
and scheduled synchronisation is pushed a year out so a run only happens on
request. Pushover credentials are placeholders, so no notification is delivered.

This applies to the development environment only. It is not a substitute for the
sandbox acceptance check, and it never runs in CI.

Normal Go tests run without network access to VeloPlanner, Wahoo, Pushover,
Tailscale, or a secret system. The normal test command enables deterministic
test ordering variation with `-shuffle=on`. The v1 quality gate does not
require a race-detector job; the release and normal test contract remains
CGO-free.

## Linting and static analysis

`.golangci.yml` is intentionally focused rather than an indiscriminate
all-linters configuration. It enables the standard Go checks plus rules that
protect this service's boundaries:

- `errcheck`, `govet`, `staticcheck`, and `unused` for correctness;
- `gosec` for insecure API use;
- `bodyclose`, `noctx`, and `rowserrcheck` for HTTP and database resource
  handling;
- `gocritic`, `revive`, `unconvert`, and `misspell` for maintainable Go;
- `testifylint` for correct Testify assertion use in tests; and
- the formatter supported by the pinned linter version.

Any future exclusion is narrow, adjacent to the code it covers, and explains
the invariant being preserved. The project does not lower the lint level to
accommodate generated or copied personal data; such data is not committed.

Module checks verify that `go.mod` and `go.sum` are tidy and valid.
Vulnerability analysis uses the Go vulnerability database through
`govulncheck`. A finding is triaged before a release; it is not silently
suppressed in CI.

## GitHub Actions

GitHub Actions runs for pull requests **targeting** the default branch and for
pushes **to** it, and for nothing else. There is no manual trigger: every run
answers for a specific tree that a review or a merge produced. It uses the same
pinned Mise toolchain and invokes the same `ci-*` Make groups that `make check`
runs; CI does not reimplement a divergent list of shell commands. It is the
authoritative gate: it runs the complete validation for every changed path,
whatever a contributor chose to run locally.

The validation workflow must:

- use explicit minimal `permissions`; every job is read-only except the
  default-branch publish, which adds `packages: write`, and the deployment that
  follows it, which holds `id-token: write` alone. That token is a tailnet
  credential obtained by workload identity federation, not a signing identity:
  nothing is signed, and no long-lived tailnet secret is stored;
- pin third-party actions to immutable full commit SHAs;
- run without production credentials, Wahoo refresh tokens, or Docker secret
  files;
- fail when formatting, local hook hygiene, linting, tests, module checks,
  vulnerability analysis, or the CGO-free Linux builds for either published
  architecture fail;
- build the production Dockerfile and discard the result on a pull request that
  changes an input of the container build, never pushing it; and
- deploy what it published to the Tailnet host, over Tailscale SSH, passing the
  digest and nothing else; and
- aggregate every job into one required check that is green only when each
  dependency succeeded or was skipped by a path filter, so a failed publish or
  deployment cannot be mistaken for a passing run; and
- record each job's result in that required check's run summary, because a
  skipped job and a job that never ran are indistinguishable on a commit status,
  and the gate is readable only if a skip is visibly a skip.

The pull-request build and the default-branch publish are the same build split
by event: a pull request proves it, a push to the default branch publishes it,
and neither runs the other's job, so a change is built once per event. A pull
request may read the shared registry build cache; it may not write to it,
because writing needs the same permission that publishes.

The two are gated by different path filters, and deliberately so. Publishing
follows every input of the running binary, because a source change must reach a
new digest. The pull-request build follows only the container inputs — the
Dockerfile, what it copies in, and the dependency manifests that resolve inside
a build stage — because re-proving an untouched Dockerfile against changed Go
source repeats what the cross-compile check has already established. A container
break that reaches the default branch costs a red run and no new image; it costs
no availability, because a deploying host is pinned to a digest that keeps
running. The fix for such a break changes a container input by construction, so
it is proved before it merges.

A separate code-scanning workflow analyses Go and GitHub Actions changes.
It also uses immutable action pins and least privilege. Repository-native
secret scanning is enabled where available, but it is a defence in depth:
the committed fixtures, logs, examples, and test data must themselves contain
no credentials or personal routes.

No GitHub Actions workflow invokes the live VeloPlanner account, authorises a
Wahoo account, uploads a route, or sends a Pushover notification. Sandbox FIT
and Wahoo acceptance is an explicit operator-run check with separately
provisioned non-production credentials, and its output must be redacted.

## Container contract

The production image is a multi-stage build that produces a statically linked
Linux binary with `CGO_ENABLED=0`, published for `linux/amd64` and
`linux/arm64`. The build stages cross-compile from the build platform, so
neither architecture requires emulation. A first stage builds the browser UI
bundle with Node.js, which the Go stage then embeds; Node reaches no further
than that stage and is absent from the runtime image.

Every base image is a **Docker Hardened Image** from `dhi.io`, pinned by digest:
the `-dev` variants for the Node and Go build stages, which need a shell and a
toolchain, and the minimal `static` image for the runtime. They carry SBOMs,
SLSA Build Level 3 provenance, and signatures. Because the images this project
publishes are themselves unsigned, that is the strongest verifiable link in the
chain, and it is why the base images stay pinned by digest.

`dhi.io` requires `docker login dhi.io` with a Docker Hub account and personal
access token **even on the free Community tier**. It is therefore a build-time
credential dependency: the validation and publish jobs both authenticate to it,
and a machine that builds images locally must be logged in. A deploying host is
unaffected: it pulls a published GHCR image by digest and never builds, so it
needs no `dhi.io` credential.

The runtime image:

- contains the binary and only the certificate roots and runtime files it
  genuinely needs;
- runs as an unprivileged non-root user;
- has a declared persistent volume for `/var/lib/domestique`, which holds the
  SQLite database;
- accepts secret files only at runtime under `/run/secrets`, never during the
  image build;
- has no bundled Tailscale daemon, SSH service, shell requirement, or default
  credentials; and
- is usable with a read-only root filesystem plus a temporary writable mount if
  the selected runtime needs one.

The host runs the container with a loopback-only publication such as
`127.0.0.1:8080:8080`. The host's Tailscale process owns `tailscale serve`
and the identity header boundary; Tailscale is not embedded in the application
container. The compose file, static configuration, Docker secret files, and
pinned image digest are operator-managed deployment state outside Git.

The repository may provide a non-secret, clearly placeholder deployment example
later. It must not make a direct Internet port publication easy to copy, and it
must not contain an account identifier, callback hostname, token, or secret
file content.

## Published images

The project follows trunk-based development. There are no version tags and no
GitHub releases; the default branch is the only thing that publishes, and every
change reaches it through a pull request the full gate accepted.

A push to the default branch that touches an input of the image builds one GHCR
image index covering `linux/amd64` and `linux/arm64` and pushes it to
`ghcr.io/nobbs/domestique` under two tags:

| Tag | Meaning |
| --- | --- |
| `sha-<short-commit>` | The immutable name of the image built from that commit. It never moves. |
| `latest` | A pointer to the most recent published default-branch image. It is for inspection, never a deployment instruction. |

A commit that changes nothing the image is built from publishes no image, so
`latest` may name an older commit than the branch head. That is the intended
result: an unchanged source tree does not need a new digest, and the digest
already published is still the one to deploy. There is no republish trigger,
because there is nothing a republished identical tree would give a host.

The publish job:

1. builds the same CGO-free cross-compiled target that pull requests prove,
   exactly once for the commit;
2. pushes the index and records its digest in the run summary, which is where an
   operator reads the value to deploy;
3. attaches BuildKit's software bill of materials and `mode=max` provenance
   attestations to the index; and
4. runs only after linting, tests, security analysis, and the browser UI checks
   have succeeded or been skipped by a path filter.

The deployment that follows it hands the Tailnet host that same index digest and
nothing else. The host composes the reference from the repository it is
configured with, pins it, restarts the service, and restores the digest it was
running if the new one fails a health check — so an automated deployment that
goes wrong leaves the service that was already running, and CI reports the
failure. Automation therefore consumes exactly what an operator would: an
immutable digest read from the run that produced it. It has no path to a mutable
tag, another repository, or a locally built image, and the account it uses on
the host can run one command and no other.

**The images are not signed.** Sigstore keyless signing was removed together
with the tag-triggered release workflow, and GitHub artifact attestations are
not an available substitute: on a private repository they require GitHub
Enterprise Cloud, and this repository stays private. The trust argument is
therefore provenance of publication rather than a detached signature — the
package is private, only the default branch holds the permission that writes it,
the base images are Docker Hardened Images pinned by digest, and the deploying
host pins a digest it read from the run that produced it. A host must never
substitute an image from anywhere else.

A deploying host consumes only an immutable digest, never a mutable tag such as
`latest`. It needs a package-read credential for the private GHCR package and no
build credential. A machine may still build the pinned Dockerfile from a
checkout, as the macOS MVP does; such an image is a local build, not a published
artefact, and carries no provenance. Rollback means selecting an earlier
published digest and restarting the container; it does not restore SQLite state
or bypass the reauthorisation and safe-adoption rules.

Publish automation receives `contents: read` and `packages: write` and nothing
else. It receives no application secrets beyond the Docker Hardened Images pull
credential the build cannot proceed without, and does not contact live provider
accounts.

## Repository hygiene

Before the first implementation release, the repository includes:

- the MIT `LICENSE`;
- a concise `SECURITY.md` with a private reporting channel and supported
  release policy;
- `.gitignore` entries for local configuration, databases, secrets, coverage,
  and build outputs;
- an explicit `.dockerignore` that excludes Git metadata, local state,
  configuration, secret files, and test artefacts from image contexts;
- documentation for the normal local check, manual sandbox acceptance, image
  selection and inspection, the Tailnet-host deployment boundary, and the
  automated deployment's host-side prerequisites; and
- shell-script linting for every script it carries, because both of them run
  somewhere a mistake is expensive.

A published image is deployable only when the default-branch run that produced
it was green end to end, no untriaged vulnerability finding blocks it, and the
manual sandbox acceptance still covers the current Wahoo and FIT contract.
