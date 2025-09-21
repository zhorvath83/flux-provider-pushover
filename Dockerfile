# syntax=docker/dockerfile:1
# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

# Build arguments for multi-arch
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG BUILD_DATE

# Install certificates for HTTPS connections
RUN apk add --no-cache ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better cache utilization
COPY go.mod go.sum* ./

# Download dependencies (cached if go.mod/go.sum unchanged)
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build the binary with optimizations
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags="-w -s -extldflags '-static' -X main.version=${VERSION} -X main.buildDate=${BUILD_DATE}" \
    -o flux-provider-pushover ./cmd/server

# Final stage - distroless for minimal size with better compatibility
# Using pinned version with SHA256 digest for reproducibility and Renovate support
# renovate: datasource=docker depName=gcr.io/distroless/static
FROM gcr.io/distroless/static:nonroot@sha256:e8a4044e0b4ae4257efa45fc026c0bc30ad320d43bd4c1a7d5271bd241e386d0

# Copy binary (distroless nonroot already runs as user 65532)
COPY --from=builder /build/flux-provider-pushover /flux-provider-pushover

# Expose port
EXPOSE 8080

# Required environment variables (must be set at runtime):
# PUSHOVER_USER_KEY    - Your Pushover user key (required)
# PUSHOVER_API_TOKEN   - Your Pushover application API token (required)
# 
# Optional environment variables:
# PORT                 - Server port (default: ":8080")
# PUSHOVER_URL        - Pushover API URL (default: "https://api.pushover.net/1/messages.json")

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/flux-provider-pushover", "-health"]

# Run the binary
ENTRYPOINT ["/flux-provider-pushover"]
