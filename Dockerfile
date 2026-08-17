# syntax=docker/dockerfile:1

# Base images are Docker Hardened Images from dhi.io, pinned by digest. Pulling
# them requires `docker login dhi.io` with a Docker Hub account and personal
# access token, including on the free Community tier.

# dhi.io/node:24-dev — the -dev variant carries npm and a shell.
FROM --platform=$BUILDPLATFORM dhi.io/node@sha256:1949e745d8b5365e45dbd7ba20a495178aa55fda7248c5c5af3928d84467d047 AS webui

USER root
WORKDIR /app

# The lockfile is copied first so a source-only change reuses the install layer.
COPY internal/webui/app/package.json internal/webui/app/package-lock.json ./
RUN npm ci

COPY internal/webui/app/ ./
RUN npm run build

# dhi.io/golang:1.26-dev — the -dev variant carries the toolchain and coreutils.
FROM --platform=$BUILDPLATFORM dhi.io/golang@sha256:b511696c1fb6929510c24d8ce66b90e7f9fc763082e5a8f73f778d7a177df93c AS build

ARG TARGETARCH
ARG TARGETOS

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
    go build -trimpath -buildvcs=true -ldflags='-s -w' \
      -o /out/domestique ./cmd/domestique \
    && install -d -m 0755 /out/etc/domestique /out/var/lib/domestique \
    && touch /out/etc/domestique/.keep /out/var/lib/domestique/.keep \
    && chown -R 65532:65532 /out/etc /out/var

# dhi.io/static:20260611-alpine — minimal runtime for a static binary. Its
# nonroot user is UID 65532, matching the ownership set above.
FROM dhi.io/static@sha256:93568eb7c673afb3ad79b15cca341469d3e02cf859caae1049aa22fe7fbce90a

LABEL org.opencontainers.image.source="https://github.com/nobbs/domestique"

COPY --from=build --chown=65532:65532 /out/domestique /usr/local/bin/domestique
COPY --from=build --chown=65532:65532 /out/etc/domestique /etc/domestique
COPY --from=build --chown=65532:65532 /out/var/lib/domestique /var/lib/domestique

USER 65532:65532
WORKDIR /var/lib/domestique
VOLUME ["/var/lib/domestique"]
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/domestique"]
