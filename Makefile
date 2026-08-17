GO ?= go
ACTIONLINT ?= actionlint
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
GITLEAKS ?= gitleaks
MARKDOWNLINT ?= markdownlint-cli2
PREK ?= prek
NPM ?= npm
GOCACHE ?= $(CURDIR)/.cache/go-build

BUILD_FLAGS := -trimpath -buildvcs=true
BUILD_TARGET := ./cmd/domestique
BUILD_OUTPUT := build/domestique
UI_DIR := internal/webui/app
UI_DIST := $(UI_DIR)/dist

.PHONY: fmt lint markdownlint workflow-lint test vet mod-check vulncheck secret-scan build build-check ci-lint ci-test ci-security ci-ui check
.PHONY: ui-install ui-dev ui-typecheck ui-lint ui-format ui-test ui-audit ui-build
.PHONY: dev-setup dev-api

export GOCACHE

fmt:
	$(GOLANGCI_LINT) fmt

lint:
	$(GOLANGCI_LINT) run ./...

markdownlint:
	$(MARKDOWNLINT) "**/*.md"

workflow-lint:
	$(ACTIONLINT) .github/workflows/*.yml

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

# Snapshots the deployed SQLite state and writes a development configuration
# that reads real VeloPlanner data but cannot reach Wahoo. See dev/setup.sh.
dev-setup:
	./dev/setup.sh

# Serves the API on :8081 against the snapshot, so it never contends with the
# deployed container for a SQLite file.
dev-api:
	DOMESTIQUE_CONFIG_FILE=$(CURDIR)/.local/dev/config.toml \
		CGO_ENABLED=0 $(GO) run $(BUILD_TARGET)

ui-install:
	$(NPM) --prefix $(UI_DIR) ci

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
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(BUILD_FLAGS) -o $(BUILD_OUTPUT) $(BUILD_TARGET)

build-check: ui-build
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(BUILD_FLAGS) -o /dev/null $(BUILD_TARGET)

# CI runs the ci-* groups below as separate jobs so that a failure names the
# area it came from. Keep them as the only decomposition of the quality gate:
# check exists to run the same work locally, in one command.
ci-lint:
	$(PREK) run --all-files
	$(MAKE) lint
	$(MAKE) markdownlint
	$(MAKE) workflow-lint

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

check:
	$(MAKE) ci-lint
	$(MAKE) ci-test
	$(MAKE) ci-security
	$(MAKE) ci-ui
	$(MAKE) build-check
