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

# Optional stage: the guard on its own (`docker build --target guard`). The guard holds the Docker socket, so this
# image is the static binary and nothing else: no shell, no package manager, no certificates (the guard never
# dials out). It sits before the runtime stage so the default build (the last stage) is unchanged. Use `command: ["guard", ...]` as with the main image, and
# `healthcheck: test: ["CMD", "/app/crowsnest", "healthcheck", "http://127.0.0.1:2375/_ping"]`.
FROM scratch AS guard
COPY --from=build /out/crowsnest /app/crowsnest
USER 1000:1000
WORKDIR /app
EXPOSE 2375
ENTRYPOINT ["/app/crowsnest"]
CMD ["guard", "--listen", ":2375"]

# Stage 2: minimal runtime image.
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
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
