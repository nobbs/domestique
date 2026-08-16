# Domestique delivery specification

**Status:** accepted v1 design

This is a subordinate specification to [the service contract](service.md). It
defines the local quality gate, GitHub Actions, container hardening, and release
artifacts. It does not place Raspberry Pi configuration or secrets in this
repository.

## Development toolchain and commands

The repository tracks a `.mise.toml` that pins the Go toolchain,
`golangci-lint`, `prek`, Actionlint, Gitleaks, Markdownlint, and
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
| `make build` | Compiles the Linux ARM64 service binary with `CGO_ENABLED=0`. |
| `make check` | Runs every repository check required before review or merge. |

`make check` is the canonical local and CI entry point. It includes
`prek run --all-files`, linting, tests, Go module verification, vulnerability
analysis, a GitHub Actions workflow check, a worktree secret scan, and the
release-target binary compilation. `make fmt` applies Go formatting. A fixing
`prek` hook exits non-zero after a safe mechanical repair so the resulting
change can be reviewed and staged deliberately.

`prek` owns fast repository hygiene: whitespace and end-of-file checks,
private-key and accidental-large-file checks, YAML, TOML, and Markdown
validation where applicable, and Go formatting. Developers may install the
`prek` hook, but the hook is a convenience rather than a substitute for
`make check`. The project uses `prek`, never `pre-commit`.

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
  and
- the formatter supported by the pinned linter version.

Any future exclusion is narrow, adjacent to the code it covers, and explains
the invariant being preserved. The project does not lower the lint level to
accommodate generated or copied personal data; such data is not committed.

Module checks verify that `go.mod` and `go.sum` are tidy and valid.
Vulnerability analysis uses the Go vulnerability database through
`govulncheck`. A finding is triaged before a release; it is not silently
suppressed in CI.

## GitHub Actions

GitHub Actions runs for pull requests and changes to the default branch. It
uses the same pinned Mise toolchain and invokes `make check`; CI does not
reimplement a divergent list of shell commands.

The validation workflow must:

- use explicit minimal `permissions`, normally read-only;
- pin third-party actions to immutable full commit SHAs;
- run without production credentials, Wahoo refresh tokens, or Docker secret
  files;
- fail when formatting, local hook hygiene, linting, tests, module checks,
  vulnerability analysis, or the CGO-free Linux ARM64 build fail; and
- build the production Dockerfile without pushing once that Dockerfile exists.

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
Linux ARM64 binary with `CGO_ENABLED=0`. The runtime image:

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

The Raspberry Pi runs the container with a loopback-only publication such as
`127.0.0.1:8080:8080`. The host's Tailscale process owns `tailscale serve`
and the identity header boundary; Tailscale is not embedded in the application
container. The Pi's compose file, static configuration, Docker secret files,
and pinned image digest are operator-managed deployment state outside this
public repository.

The repository may provide a non-secret, clearly placeholder deployment example
later. It must not make a direct Internet port publication easy to copy, and it
must not contain an account identifier, callback hostname, token, or secret
file content.

## Release artifacts

A version tag creates a GHCR image for `linux/arm64`. It is an immutable
release artifact, not a mutable deployment instruction. The release workflow:

1. builds the same CGO-free target verified in CI;
2. publishes the image and records its digest;
3. creates a software bill of materials and provenance attestation;
4. signs the image with GitHub OIDC-backed keyless signing; and
5. attaches the digest and verification instructions to the release.

The Pi deploys only a verified immutable digest, never a mutable tag such as
`latest`. It verifies the signature and provenance before a new digest is
accepted. Rollback means selecting an earlier verified digest and restarting
the container; it does not restore SQLite state or bypass the reauthorisation
and safe-adoption rules.

Release automation receives only the permissions required to publish GHCR,
attest, sign, and create the release. It receives no application secrets and
does not contact live provider accounts.

## Public repository hygiene

Before the first implementation release, the repository includes:

- the MIT `LICENSE`;
- a concise `SECURITY.md` with a private reporting channel and supported
  release policy;
- `.gitignore` entries for local configuration, databases, secrets, coverage,
  and build outputs;
- an explicit `.dockerignore` that excludes Git metadata, local state,
  configuration, secret files, and test artefacts from image contexts; and
- documentation for the normal local check, manual sandbox acceptance, image
  verification, and Pi deployment boundary.

A release is eligible only when the default-branch CI is green, its image is
signed and attested, no untriaged vulnerability finding blocks it, and the
manual sandbox acceptance still covers the current Wahoo and FIT contract.
