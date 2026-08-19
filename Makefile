GO ?= go
ACTIONLINT ?= actionlint
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
GITLEAKS ?= gitleaks
MARKDOWNLINT ?= markdownlint-cli2
SHELLCHECK ?= shellcheck
PREK ?= prek
NPM ?= npm
GOCACHE ?= $(CURDIR)/.cache/go-build

# The running service reports which public commit produced it, and only a
# trusted build input can say which one that is: a Docker build context carries
# no VCS metadata, so nothing inside the build can work it out. SOURCE_REVISION
# is empty for a local build on purpose — the service then reports no revision
# at all, which is honest, rather than one an operator might act on.
SOURCE_REVISION ?=
REVISION_FLAGS := $(if $(SOURCE_REVISION),-ldflags=-X=github.com/nobbs/domestique/internal/build.revision=$(SOURCE_REVISION))
BUILD_FLAGS := -trimpath -buildvcs=true $(REVISION_FLAGS)
BUILD_TARGET := ./cmd/domestique
BUILD_OUTPUT := build/domestique
# The release image is published for linux/amd64 alone, so that is what the
# compile check covers. BUILD_ARCH selects what `make build` writes to disk, and
# defaults to the machine's own architecture so a local build runs where it was
# built; set it explicitly to cross-compile for the deployed host from another.
# RELEASE_ARCHES is a list so that publishing a second architecture again is
# this line plus the two platform arguments in the workflow.
BUILD_ARCH ?= $(shell $(GO) env GOARCH)
RELEASE_ARCHES := amd64
UI_DIR := internal/webui/app
UI_DIST := $(UI_DIR)/dist

# Both coverage reports land under one gitignored directory so that a single
# upload step can find them without rediscovering where each toolchain likes to
# write. Vitest is pointed at the same place from vite.config.ts.
COVERAGE_DIR := $(CURDIR)/.coverage
GO_COVERPROFILE := $(COVERAGE_DIR)/go.out
# The service is what the number is about; dev/ is repository tooling, measured
# by its own tests but not part of the service's coverage.
GO_COVERPKG := ./cmd/...,./internal/...

.PHONY: fmt hygiene lint markdownlint shell-lint workflow-lint hook-check gate-check test vet mod-check vulncheck secret-scan build build-check ci-lint ci-test ci-security ci-ui quick check
.PHONY: ui-install ui-ensure ui-dev ui-typecheck ui-lint ui-format ui-test ui-audit ui-build
.PHONY: ui-browser-install ui-browser-test
.PHONY: coverage coverage-go coverage-ui
.PHONY: dev-setup dev-api demo container-smoke

export GOCACHE

fmt:
	$(GOLANGCI_LINT) fmt

# prek's own configuration over the whole tree. The installed commit hook runs
# the same hooks over staged files alone, so this is the repository-wide pass
# that the hook deliberately does not pay for.
hygiene:
	$(PREK) run --all-files

lint:
	$(GOLANGCI_LINT) run ./...

markdownlint:
	$(MARKDOWNLINT) "**/*.md"

# Both scripts run somewhere a mistake is expensive: one against the deployed
# state, the other as root on the deploying host.
shell-lint:
	$(SHELLCHECK) deploy/*.sh dev/*.sh

# actionlint shells out to shellcheck for every `run:` block, so the pinned
# shellcheck above is what lints the workflow scripts too.
workflow-lint:
	$(ACTIONLINT) .github/workflows/*.yml

# The commit hook is only worth installing while it stays fast. This asserts
# the structure that keeps it so, rather than a wall-clock number CI cannot
# measure reliably. See dev/check-hook-cost.sh.
hook-check:
	./dev/check-hook-cost.sh

# `quick` is only trustworthy while it stays a strict subset of `check`. This
# asserts that, and that the work it defers is exactly the documented set,
# so a new check cannot land in one and be forgotten in the other. See
# dev/check-local-gate.sh.
gate-check:
	./dev/check-local-gate.sh

test:
	CGO_ENABLED=0 $(GO) test -shuffle=on ./...

vet:
	CGO_ENABLED=0 $(GO) vet ./...

mod-check:
	$(GO) mod tidy -diff
	$(GO) mod verify

vulncheck:
	$(GOVULNCHECK) ./...

secret-scan:
	$(GITLEAKS) dir . --redact --no-banner

# Coverage is measured on demand and is deliberately not part of `quick`,
# `check`, or the CI gate: instrumenting the tree makes the suite slower, and
# nothing here fails on a number. It reports what a change leaves untested; it
# does not judge it.
coverage: coverage-go coverage-ui

# -coverpkg attributes coverage across the whole service, so a function
# exercised only through another package's tests is not reported as dead. The
# cost is that the per-package percentage `go test` prints on each line below
# becomes the fraction of the whole service that package's tests reached; the
# real per-package number is the summary printed after it.
coverage-go:
	mkdir -p $(COVERAGE_DIR)
	CGO_ENABLED=0 $(GO) test -shuffle=on -coverpkg=$(GO_COVERPKG) \
		-coverprofile=$(GO_COVERPROFILE) ./...
	$(GO) run ./dev/coveragesummary <$(GO_COVERPROFILE)

# Both browser UI suites, in the order the merge needs them.
#
# Vitest writes its LCOV, its JSON report and a terminal summary under
# $(COVERAGE_DIR); see the coverage block in $(UI_DIR)/vite.config.ts for what it
# measures. The second step drives the browser suite over the demo stack,
# attributes what it reached back to $(UI_DIR)/src, and rewrites the LCOV file as
# the two suites merged — so one file describes one language, rather than the map
# and the page-level components reading untested because only jsdom was asked.
#
# The second step needs the browser and the Go toolchain, and says so and stops
# rather than failing when the browser is not installed: the first half is still
# a report, and downloading a browser is not something a coverage run should do
# on someone's behalf. Run `make ui-browser-install` once to measure both halves.
coverage-ui: ui-ensure
	$(NPM) --prefix $(UI_DIR) run test:coverage
	$(NPM) --prefix $(UI_DIR) run test:coverage:browser

# Snapshots the deployed SQLite state and writes a development configuration
# that reads real VeloPlanner data but cannot reach Wahoo. See dev/setup.sh.
dev-setup:
	./dev/setup.sh

# Serves the API on :8081 against the snapshot, so it never contends with the
# deployed container for a SQLite file.
dev-api:
	DOMESTIQUE_CONFIG_FILE=$(CURDIR)/.local/dev/config.toml \
		CGO_ENABLED=0 $(GO) run $(BUILD_TARGET)

# Starts the whole demo: a throwaway configuration, a freshly seeded synthetic
# library, the API serving it, and the UI dev server in front. No account, no
# secret, and no copy of anyone's routes. See dev/demo.sh.
demo: ui-ensure
	./dev/demo.sh

ui-install:
	$(NPM) --prefix $(UI_DIR) ci

# Installs only when the tree is missing altogether, so the routine loop does
# not pay a clean `npm ci` every run. It cannot notice a lockfile that moved
# under an already-installed tree, so run `make ui-install` after changing a UI
# dependency; `ci-ui`, and therefore `check` and CI, always install from
# scratch and would catch it anyway.
ui-ensure:
	@[ -d "$(UI_DIR)/node_modules" ] || $(NPM) --prefix $(UI_DIR) ci

ui-dev:
	$(NPM) --prefix $(UI_DIR) run dev

ui-typecheck:
	$(NPM) --prefix $(UI_DIR) run typecheck

ui-lint:
	$(NPM) --prefix $(UI_DIR) run lint

ui-format:
	$(NPM) --prefix $(UI_DIR) run format

ui-test:
	$(NPM) --prefix $(UI_DIR) run test

# Downloads the browser the whole-page suite runs in. Separate from running the
# suite so that a failure names which of the two went wrong, and so a contributor
# can pay the download once.
#
# PLAYWRIGHT_INSTALL_FLAGS is the escape hatch for a host that needs --with-deps,
# which installs the browser's system libraries and needs root. CI deliberately
# does not pass it: on ubuntu-24.04 every library Chromium links is already on
# the image, so --with-deps adds nothing but fonts, off an apt mirror that has
# taken over five minutes to serve them. Reach for it on a bare container.
#
# It installs the dependency tree first so that it can be the first UI target a
# job runs, which is what the Coverage job needs: the browser has to be there
# before `make coverage` drives it.
ui-browser-install: ui-ensure
	$(NPM) --prefix $(UI_DIR) run test:browser:install -- $(PLAYWRIGHT_INSTALL_FLAGS)

# The whole page in a real browser, over `make demo`'s synthetic library: what a
# WebGL map and a cross-component interaction cannot be asserted about in jsdom.
# It starts the demo stack itself, so it needs the Go toolchain as well as the
# browser, and it reaches nothing but the service it started. See
# $(UI_DIR)/playwright.config.ts.
ui-browser-test:
	$(NPM) --prefix $(UI_DIR) run test:browser

# govulncheck does not see npm packages, so the JavaScript tree needs its own
# advisory check in the same gate.
ui-audit:
	$(NPM) --prefix $(UI_DIR) audit --omit=dev

# The bundler empties dist, which would remove the committed placeholder that
# keeps the go:embed pattern valid before a UI build. Restore it afterwards.
ui-build:
	$(NPM) --prefix $(UI_DIR) run build
	touch $(UI_DIST)/.gitkeep

build: ui-build
	mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=$(BUILD_ARCH) $(GO) build $(BUILD_FLAGS) -o $(BUILD_OUTPUT) $(BUILD_TARGET)

build-check: ui-build
	for arch in $(RELEASE_ARCHES); do \
		echo "==> linux/$$arch"; \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch $(GO) build $(BUILD_FLAGS) -o /dev/null $(BUILD_TARGET) || exit 1; \
	done

# Starts the production image under the runtime a deployment gives it and asks
# whether the service comes up: the one check that runs the built container
# rather than reasoning about it. Deliberately outside `check` and `quick`, and
# outside the ci-* groups: it needs an image, and building one needs a
# `docker login dhi.io` for the hardened base images, which a local gate must
# not require. CI runs it in the `Image` job, which already holds those
# credentials, over a natively built image. See dev/container-smoke.sh for how to
# point it at an image locally.
container-smoke:
	./dev/container-smoke.sh

# CI runs the ci-* groups below as separate jobs so that a failure names the
# area it came from, and GitHub Actions running them is what actually gates a
# merge. Keep them as the only decomposition of the full gate: `check` exists to
# run the same work locally, in one command, when you want the answer earlier.
#
# Every step of a gate goal — the ci-* groups, `check`, and `quick` — is written
# as its own target invoked with $(MAKE), never as a bare shell command and
# never as a prerequisite. That is what lets `gate-check` see the steps and
# compare the two entry points; a check written any other way would be invisible
# to it. `gate-check` enforces the convention as well as the comparison.
ci-lint:
	$(MAKE) hygiene
	$(MAKE) lint
	$(MAKE) markdownlint
	$(MAKE) shell-lint
	$(MAKE) workflow-lint
	$(MAKE) hook-check
	$(MAKE) gate-check

ci-test:
	$(MAKE) vet
	$(MAKE) test

ci-security:
	$(MAKE) mod-check
	$(MAKE) vulncheck
	$(MAKE) ui-audit
	$(MAKE) secret-scan

# Every UI target reads node_modules, so this group installs before it runs
# anything. Nothing else in the gate may assume the tree is already installed.
ci-ui: ui-install
	$(MAKE) ui-typecheck
	$(MAKE) ui-lint
	$(MAKE) ui-test
	$(MAKE) ui-browser-install
	$(MAKE) ui-browser-test

# The routine local loop. It runs every check that reads the tree as it stands
# and needs nothing beyond it, and defers five things to `check` and to GitHub
# Actions, which is the authoritative gate a merge has to pass:
#
#   build-check  rebuilds the UI bundle and compiles the published release
#                target; the slowest check in the gate whenever the build cache
#                is cold, which is every CI run
#   vulncheck    needs the network and a current Go advisory database
#   ui-audit     needs the network and a current npm advisory database
#   ui-browser-install
#                downloads a browser, which is a network fetch and a few hundred
#                megabytes on disk
#   ui-browser-test
#                runs that browser over the demo stack: minutes rather than
#                seconds, and useless without the download above
#
# Nothing is weakened by leaving them out: every one still runs in `check` and
# on every pull request. `gate-check` asserts that this list is the whole of the
# difference, so a check added to the full gate cannot silently skip this one.
#
# It reuses an installed UI tree rather than reinstalling it; see `ui-ensure`.
quick: ui-ensure
	$(MAKE) hygiene
	$(MAKE) lint
	$(MAKE) markdownlint
	$(MAKE) shell-lint
	$(MAKE) workflow-lint
	$(MAKE) hook-check
	$(MAKE) gate-check
	$(MAKE) vet
	$(MAKE) test
	$(MAKE) mod-check
	$(MAKE) secret-scan
	$(MAKE) ui-typecheck
	$(MAKE) ui-lint
	$(MAKE) ui-test

# The full gate, on demand. GitHub Actions runs this same work on every pull
# request and is what gates the merge, so running it here buys an earlier
# answer rather than a different one. `quick` is what to run while writing and
# before handing work over; reach for this when a change specifically implicates
# one of the five checks `quick` defers.
check:
	$(MAKE) ci-lint
	$(MAKE) ci-test
	$(MAKE) ci-security
	$(MAKE) ci-ui
	$(MAKE) build-check
