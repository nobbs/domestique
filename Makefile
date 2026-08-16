GO ?= go
ACTIONLINT ?= actionlint
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
GITLEAKS ?= gitleaks
MARKDOWNLINT ?= markdownlint-cli2
PREK ?= prek
GOCACHE ?= $(CURDIR)/.cache/go-build

BUILD_FLAGS := -trimpath -buildvcs=true
BUILD_TARGET := ./cmd/domestique
BUILD_OUTPUT := build/domestique

.PHONY: fmt lint markdownlint workflow-lint test vet mod-check vulncheck secret-scan build build-check check

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

build:
	mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(BUILD_FLAGS) -o $(BUILD_OUTPUT) $(BUILD_TARGET)

build-check:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(BUILD_FLAGS) -o /dev/null $(BUILD_TARGET)

check:
	$(PREK) run --all-files
	$(MAKE) lint
	$(MAKE) markdownlint
	$(MAKE) workflow-lint
	$(MAKE) vet
	$(MAKE) test
	$(MAKE) mod-check
	$(MAKE) vulncheck
	$(MAKE) secret-scan
	$(MAKE) build-check
