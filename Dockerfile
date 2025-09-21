# syntax=docker/dockerfile:1
# Build stage
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

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

# Final stage - scratch for absolute minimal size
FROM scratch

# Copy certificates and timezone data
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binary with non-root user permissions
COPY --from=builder --chown=65532:65532 /build/flux-provider-pushover /flux-provider-pushover

# Use non-root user
USER 65532:65532

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/flux-provider-pushover", "-health"]

# Run the binary
ENTRYPOINT ["/flux-provider-pushover"]
