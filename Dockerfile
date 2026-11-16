# Base images are pinned by digest; Dependabot (docker ecosystem) proposes
# updates. The release workflow scans the result with Trivy before pushing,
# and a fixable HIGH/CRITICAL stops the release. There is no docker CLI or
# compose in the image: the panel talks to Docker through the Engine API.

# Stage 1: build the Go binary on the build machine's native platform and
# cross-compile for the target (no QEMU emulation of the Go toolchain).
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/crowsnest ./cmd/crowsnest

# Stage 2: minimal runtime image.
FROM alpine:3.24.4@sha256:93b41ef40b1677fce8fcc64ff4af09daddfacf5040afde618bc46c1e2d1cc7d4
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates tzdata
COPY --from=build /out/crowsnest /app/crowsnest

# Non-root user. Docker socket/proxy access is granted at runtime via
# `group_add` in compose.yaml, matching the host's docker group GID —
# not baked into the image.
RUN adduser -D -u 1000 appuser
USER appuser
WORKDIR /app

EXPOSE 5000

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/app/crowsnest", "healthcheck"]

ENTRYPOINT ["/app/crowsnest"]
CMD ["serve"]
