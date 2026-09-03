# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

# Base images are Docker Hardened Images from dhi.io, pinned by digest. Pulling
# them requires `docker login dhi.io` with a Docker Hub account and personal
# access token, including on the free Community tier.
#
# Each carries its tag beside the digest. The digest is what resolves — the tag
# changes nothing about which image this builds on — but it names the stream the
# digest was taken from, which is what lets the updater offer the next digest of
# that stream rather than of `latest`.
#
# The tag names the exact patch, not the major or major.minor line it belongs
# to, so that it states the same version `.mise.toml` pins rather than the
# nearest floating alias of it. A digest refresh then only ever brings a rebuild
# of that patch, and reaching the next one is a tag change — which is what puts
# it in the toolchain group a person reads.

# The -dev variant carries corepack and a shell. Corepack installs the pnpm
# version pinned by `packageManager` in package.json, so the build resolves the
# same pnpm the repository does without pinning it a second time here.
FROM --platform=$BUILDPLATFORM dhi.io/node:24.20.0-dev@sha256:35dae551ffb9790b9e0416c5359c80b1b45c345b950b0670615a307451f0005f AS webui

USER root
WORKDIR /app

RUN corepack enable pnpm

# The lockfile is copied first so a source-only change reuses the install layer.
COPY internal/webui/app/package.json internal/webui/app/pnpm-lock.yaml internal/webui/app/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY internal/webui/app/ ./
RUN pnpm run build

# The -dev variant carries the toolchain and coreutils.
FROM --platform=$BUILDPLATFORM dhi.io/golang:1.27.0-dev@sha256:e2e77e505161b120742b747aae60dbf6179bf381e9a178644ef6530b65171f79 AS build

ARG TARGETARCH
ARG TARGETOS

# The commit the image is built from, passed in by CI. It cannot be derived
# here: .dockerignore excludes .git, and a build context is not a checkout. Left
# unset — as it is for any local `docker build` — the service reports no
# revision rather than one it guessed.
ARG SOURCE_REVISION=

USER root
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# The browser UI is compiled into the binary, so it must be in place before the
# Go build resolves its embed directive. The build context never carries dist:
# .dockerignore excludes it so a stale local bundle cannot leak into the image.
COPY --from=webui /app/dist ./internal/webui/app/dist

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=true \
      -ldflags="-s -w ${SOURCE_REVISION:+-X=github.com/nobbs/domestique/internal/build.revision=${SOURCE_REVISION}}" \
      -o /out/domestique ./cmd/domestique \
    && install -d -m 0755 /out/etc/domestique /out/var/lib/domestique \
    && touch /out/etc/domestique/.keep /out/var/lib/domestique/.keep \
    && chown -R 65532:65532 /out/etc /out/var

# Minimal runtime for a static binary. Its nonroot user is UID 65532,
# matching the ownership set above.
FROM dhi.io/static:20260611-alpine@sha256:93568eb7c673afb3ad79b15cca341469d3e02cf859caae1049aa22fe7fbce90a

LABEL org.opencontainers.image.source="https://github.com/nobbs/domestique"

COPY --from=build --chown=65532:65532 /out/domestique /usr/local/bin/domestique
COPY --from=build --chown=65532:65532 /out/etc/domestique /etc/domestique
COPY --from=build --chown=65532:65532 /out/var/lib/domestique /var/lib/domestique

USER 65532:65532
WORKDIR /var/lib/domestique
VOLUME ["/var/lib/domestique"]
# The served port, and the readiness probe's own port. The host publishes both
# to its loopback address only; the readiness one is deliberately never fronted
# by Tailscale Serve, which is what keeps the probe off the authenticated public
# surface.
EXPOSE 8080 8081

ENTRYPOINT ["/usr/local/bin/domestique"]
