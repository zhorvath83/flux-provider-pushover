# Flux Provider Pushover

## Projekt Áttekintés
FluxCD webhook-ot Pushover push notification-né alakító Go middleware. Híd szerepet tölt be a FluxCD alert provider és a Pushover API között.

## Miért kell ez?
- FluxCD nem támogat natív Pushover provider-t
- A generic provider nem képes megfelelően kezelni a Pushover authentikációt
- Szükség van egy middleware-re ami fogadja a FluxCD webhook-okat és továbbítja Pushover-nek

## Architektúra

```
FluxCD → POST /webhook → Auth (ConstantTimeCompare) → Validate → Build Message → Pushover API
                                                         ↑
                                                    Rate Limiter (per-IP)
                                                    Request ID (nano-id)
                                                    Circuit Breaker
```

### Middleware lánc
`RateLimitMiddleware → RequestIDMiddleware → Router → Handler`

## Projekt Struktúra

```
flux-provider-pushover/
├── cmd/server/
│   ├── main.go              # Entry point, wiring
│   ├── main_test.go          # RunApp tesztek
│   └── integration_test.go   # Handler integrációs tesztek
├── internal/
│   ├── config/
│   │   ├── config.go        # Config loader + validator (functional pattern)
│   │   └── config_test.go
│   ├── handlers/
│   │   ├── handlers.go      # HTTP handler-ek (webhook, health, root)
│   │   ├── message.go       # BuildPushoverMessage, ValidateAlert, input validáció
│   │   ├── handlers_test.go
│   │   ├── message_test.go
│   │   ├── edge_case_test.go
│   │   └── dependencies_test.go
│   ├── pushover/
│   │   ├── client.go         # Pushover API kliens retry + circuit breaker
│   │   ├── circuitbreaker.go # Circuit breaker (closed/open/halfOpen)
│   │   ├── client_test.go
│   │   └── circuitbreaker_test.go
│   ├── server/
│   │   ├── server.go         # HTTP server, BaseContext, Start/Shutdown
│   │   ├── logger.go         # slog Logger interface + SlogLogger
│   │   ├── middleware.go     # RequestID middleware (nano-id, X-Request-ID)
│   │   ├── ratelimit.go      # Per-IP rate limiter (x/time/rate)
│   │   ├── server_test.go
│   │   ├── server_coverage_test.go
│   │   ├── middleware_test.go
│   │   └── ratelimit_test.go
│   └── types/
│       └── types.go         # Constants, structs, pre-defined responses
├── Dockerfile               # Multi-stage, multi-arch, distroless/nonroot
├── go.mod & go.sum
├── scripts/
│   └── runGoTests.sh
├── .github/workflows/
│   └── build.yml            # CI: gosec + test + build + trivy
└── basic-memory/            # Projekt knowledge graph
```

## API Struktúra

### FluxCD → Middleware
```json
{
  "severity": "error|info",
  "message": "Alert message",
  "reason": "ProgressDeadlineExceeded",
  "reportingController": "kustomize-controller",
  "metadata": {
    "revision": "main@sha1:abc123"
  },
  "involvedObject": {
    "kind": "Kustomization",
    "name": "flux-system"
  }
}
```

### Middleware → Pushover
```json
{
  "token": "PUSHOVER_API_TOKEN",
  "user": "PUSHOVER_USER_KEY",
  "title": "FluxCD",
  "message": "Formatted alert message"
}
```

### Middleware → Client (error)
```json
{"error": "Upstream service unavailable"}
```

## Biztonság

| Védelem | Implementáció |
|---------|---------------|
| Auth timing attack | `crypto/subtle.ConstantTimeCompare` |
| Error info leakage | 502 generic msg, details csak log-ban |
| Log injection | `log/slog` JSON auto-escape |
| Request body limit | `MaxBytesReader` 1MB |
| Header size limit | `MaxHeaderSize` 8KB |
| Per-field validation | MaxStringFieldLen 512, MaxMessageFieldLen 4096, metadata limits |
| Pushover message truncation | 1024 char limit |
| Rate limiting | Per-IP token bucket (10 req/s, burst 30) |
| No secrets in image | Env vars / mounted files |
| Container security | ReadOnlyRootFilesystem, RunAsNonRoot |

## Reziliencia

| Feature | Implementáció |
|---------|---------------|
| Retry | Exponenciális backoff jitter-rel, max 2 retry (5xx/429/network only) |
| Circuit breaker | closed → open (5 hiba) → halfOpen (30s) → closed (2 siker) |
| Graceful shutdown | BaseContext cancel + 30s timeout + SIGTERM handling |
| Health check timeout | 2s saját HTTP client |

## Observability

- **Structured JSON logs** stdout-ra (`log/slog` JSONHandler)
- **Request ID** `X-Request-ID` header + context + log fields
- **Log levels**: Info (success), Warn (client errors), Error (upstream failures)
- **Health endpoint** `/health` Kubernetes probe-hoz

## Env változók
- `PUSHOVER_USER_KEY`: Kötelező
- `PUSHOVER_API_TOKEN`: Kötelező (Pushover app token + Bearer auth token)
- `PORT`: Opcionális, default 8080
- `PUSHOVER_URL`: Opcionális, default `https://api.pushover.net/1/messages.json`

## Függőségek
- `golang.org/x/time/rate` — per-IP rate limiter
- Minden más: standard library

## Tesztek
- `go test ./... -race` — minden zöld
- Coverage: config 100%, handlers 86%, pushover 91%, server 81%
- `go vet ./...` — 0 probléma

## CI/CD
- GitHub Actions: gosec → test → build (multi-arch amd64/arm64) → trivy
- Docker image: GHCR, distroless/nonroot base, SBOM + provenance attestation

## Performance Célok
- <100ms response time webhook-okra
- <20MB memory használat idle
- <1s cold start