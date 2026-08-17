# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26.6-alpine AS build

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

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/nobbs/domestique"

COPY --from=build --chown=65532:65532 /out/domestique /usr/local/bin/domestique
COPY --from=build --chown=65532:65532 /out/etc/domestique /etc/domestique
COPY --from=build --chown=65532:65532 /out/var/lib/domestique /var/lib/domestique

USER 65532:65532
WORKDIR /var/lib/domestique
VOLUME ["/var/lib/domestique"]
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/domestique"]
