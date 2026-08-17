# syntax=docker/dockerfile:1

# golang:1.26.6-alpine
FROM --platform=$BUILDPLATFORM golang@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS build

ARG TARGETARCH
ARG TARGETOS

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
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
