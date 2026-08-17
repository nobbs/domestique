# syntax=docker/dockerfile:1

# node:24.19.0-alpine
FROM --platform=$BUILDPLATFORM node@sha256:d32cdf619f63fe0471182d08996dd516c6275bb5fd31ae06e55a570bd9e1ad43 AS webui

WORKDIR /app

# The lockfile is copied first so a source-only change reuses the install layer.
COPY internal/webui/app/package.json internal/webui/app/package-lock.json ./
RUN npm ci

COPY internal/webui/app/ ./
RUN npm run build

# golang:1.26.6-alpine
FROM --platform=$BUILDPLATFORM golang@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS build

ARG TARGETARCH
ARG TARGETOS

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

# gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12@sha256:1b7b9f0f0e0a1d2155f531db587cc48ec26aaf97ab64364225f5bf18a054e66a

LABEL org.opencontainers.image.source="https://github.com/nobbs/domestique"

COPY --from=build --chown=65532:65532 /out/domestique /usr/local/bin/domestique
COPY --from=build --chown=65532:65532 /out/etc/domestique /etc/domestique
COPY --from=build --chown=65532:65532 /out/var/lib/domestique /var/lib/domestique

USER 65532:65532
WORKDIR /var/lib/domestique
VOLUME ["/var/lib/domestique"]
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/domestique"]
