# Build stage
FROM golang:1.22-alpine AS builder

# Install certificates for HTTPS connections
RUN apk add --no-cache ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o flux-provider-pushover .

# Final stage - distroless for minimal attack surface
FROM gcr.io/distroless/static:nonroot

# Copy certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /build/flux-provider-pushover /app/flux-provider-pushover

# Use non-root user (65532 is the nonroot user in distroless)
USER nonroot:nonroot

# Expose port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/app/flux-provider-pushover"]