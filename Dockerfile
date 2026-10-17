# syntax=docker/dockerfile:1.7
# SMemories API image (T-022, docs/container.md): the three binaries on a distroless non-root base.
# Both bases are pinned by digest; Dependabot's docker ecosystem keeps them current (.github/dependabot.yml).
# The Go patch must be the newest of the go.mod minor (board L-10, govulncheck reports older stdlib patches).

# golang:1.26-bookworm (1.26.9 when pinned), multi-arch index. Cross-compiles on the build host: no QEMU.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm@sha256:d9c68c2c51161e12fd77e4c6320687c9cd86e1af1e3ad6e6cd63ff970641453c AS build
ARG TARGETOS
ARG TARGETARCH
# Not secrets: the release label and the git commit, set by `make image` and CI.
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    pkg=github.com/danyaa666/smemories; \
    for b in smemories-api smemories-migrate smemories-media-backfill; do \
      CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
        -ldflags "-s -w -X $pkg/internal/version.Version=$VERSION -X $pkg/internal/version.Commit=$COMMIT" \
        -o /out/$b ./cmd/$b; \
    done

# gcr.io/distroless/static-debian12:nonroot (CA certificates, tzdata, user 65532; no shell, no package manager).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="smemories" \
      org.opencontainers.image.source="https://github.com/danyaa666/smemories" \
      org.opencontainers.image.revision="$COMMIT" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.licenses="NOASSERTION"
COPY --from=build /out/ /usr/local/bin/
USER 65532:65532
EXPOSE 8080
# The API is the default command. The one-off tasks override the command, not an entrypoint (ECS RunTask can only
# override the command): `smemories-migrate up`, `smemories-media-backfill --dry-run`.
CMD ["/usr/local/bin/smemories-api"]
