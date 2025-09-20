# Flux Provider Pushover

A lightweight Go-based webhook bridge between FluxCD and Pushover notification service.

## Features

- 🚀 **Ultra-lightweight**: ~5-20MB memory footprint
- ⚡ **Fast**: <100ms webhook response time, <1s cold start
- 🔒 **Secure**: Bearer token authentication, rootless container, minimal attack surface
- 🌍 **Multi-arch**: Supports linux/amd64 and linux/arm64
- 📊 **Production-ready**: Graceful shutdown, health checks, comprehensive tests
- 🎯 **Simple**: No external dependencies, uses only Go standard library

## Performance

- Memory usage: ~5-10MB idle, <50MB under load
- Response time: <100ms for webhook processing
- Throughput: 10,000+ RPS on a single instance
- Binary size: ~7MB
- Container size: ~10MB (scratch-based)

## Installation

### Using Docker

```bash
docker run -d \
  -e PUSHOVER_USER_KEY=your_user_key \
  -e PUSHOVER_API_TOKEN=your_api_token \
  -p 8080:8080 \
  ghcr.io/zhorvath83/flux-provider-pushover:latest
```

### Using Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: flux-provider-pushover
spec:
  replicas: 1
  selector:
    matchLabels:
      app: flux-provider-pushover
  template:
    metadata:
      labels:
        app: flux-provider-pushover
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        fsGroup: 65532
      containers:
      - name: flux-provider-pushover
        image: ghcr.io/zhorvath83/flux-provider-pushover:latest
        ports:
        - containerPort: 8080
        env:
        - name: PUSHOVER_USER_KEY
          valueFrom:
            secretKeyRef:
              name: pushover-credentials
              key: user-key
        - name: PUSHOVER_API_TOKEN
          valueFrom:
            secretKeyRef:
              name: pushover-credentials
              key: api-token
        resources:
          requests:
            memory: "20Mi"
            cpu: "10m"
          limits:
            memory: "50Mi"
            cpu: "100m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 2
          periodSeconds: 10
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
```

## FluxCD Configuration

Configure FluxCD to send alerts to this provider:

```yaml
apiVersion: notification.toolkit.fluxcd.io/v1beta1
kind: Provider
metadata:
  name: pushover
  namespace: flux-system
spec:
  type: generic
  address: http://flux-provider-pushover.flux-system:8080/webhook
  secretRef:
    name: pushover-token

---
apiVersion: v1
kind: Secret
metadata:
  name: pushover-token
  namespace: flux-system
stringData:
  token: your_api_token

---
apiVersion: notification.toolkit.fluxcd.io/v1beta1
kind: Alert
metadata:
  name: all-alerts
  namespace: flux-system
spec:
  providerRef:
    name: pushover
  eventSeverity: info
  eventSources:
  - kind: GitRepository
    name: '*'
  - kind: HelmRelease
    name: '*'
  - kind: Kustomization
    name: '*'
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `PUSHOVER_USER_KEY` | Yes | Your Pushover user key |
| `PUSHOVER_API_TOKEN` | Yes | Your Pushover application token |
| `PORT` | No | Server port (default: 8080) |
| `LOG_LEVEL` | No | Log level (default: info) |

## API Endpoints

- `GET /health` - Health check endpoint
- `POST /webhook` - FluxCD webhook endpoint (requires Bearer token authentication)
- `GET /` - Returns error message directing to use /webhook

## Development

### Prerequisites

- Go 1.22+
- Docker (optional, for container builds)
- Make (optional, for using Makefile)

### Building

```bash
# Build binary
make build

# Run tests
make test

# Run benchmarks
make bench

# Build Docker image
make docker-build

# Run all checks before commit
make check
```

### Testing

```bash
# Run unit tests with race detection
go test -v -race ./...

# Run benchmarks
go test -bench=. -benchmem

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Security

- **Authentication**: Bearer token required for webhook endpoint
- **Container**: Runs as non-root user (UID 65532)
- **Filesystem**: Read-only root filesystem
- **Network**: No outbound connections except to Pushover API
- **Dependencies**: Zero external Go dependencies
- **Base image**: Distroless/scratch for minimal attack surface

## Monitoring

The service exposes a `/health` endpoint for monitoring:

```bash
curl http://localhost:8080/health
# Returns: healthy
```
