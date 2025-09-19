# Flux Provider Pushover - Go Reimplementáció

## Projekt Áttekintés
Ez a projekt a flux-provider-pushover Python alapú middleware szolgáltatás újraimplementációja Go nyelven. A szolgáltatás híd szerepet tölt be a FluxCD és a Pushover push notification szolgáltatás között.

## Miért kell ez?
- FluxCD nem támogat natív Pushover provider-t
- A generic provider nem képes megfelelően kezelni a Pushover authentikációt
- Szükség van egy middleware-re ami fogadja a FluxCD webhook-okat és továbbítja Pushover-nek

## Technikai Követelmények

### Core funkcionalitás
1. **Webhook fogadás**: `/webhook` endpoint ami fogadja a FluxCD alert-eket
2. **Authentikáció**: Bearer token alapú auth az API token-nel
3. **Pushover integráció**: Alert továbbítás Pushover API-n keresztül
4. **Health check**: `/health` endpoint Kubernetes readiness/liveness probe-hoz

### Go implementáció szempontok
- **Minimális függőségek**: Csak a standard library + esetleg 1-2 jól megválasztott package
- **Alacsony memóriaigény**: ~10-20MB RAM használat célzott
- **Gyors indulás**: <1 másodperc boot time
- **Graceful shutdown**: SIGTERM kezelés, kapcsolatok tiszta lezárása

### Container szempontok
1. **Rootless**: Non-root user (UID 65532 vagy hasonló)
2. **Readonly filesystem**: 
   - Csak `/tmp` írható (ha kell)
   - Binary `/app` könyvtárban
3. **Multi-arch**: linux/amd64 és linux/arm64 támogatás
4. **Distroless/scratch base**: Minimális attack surface

## Projekt Struktúra

```
flux-provider-pushover/
├── main.go                 # Fő alkalmazás
├── handler.go             # HTTP handler-ek
├── pushover.go            # Pushover kliens
├── config.go              # Konfiguráció kezelés
├── Dockerfile             # Multi-stage, multi-arch build
├── go.mod & go.sum        # Go dependencies
├── .github/
│   └── workflows/
│       └── build.yml      # CI/CD pipeline
├── kubernetes/            # K8s manifest példák
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── ingress.yaml
│   └── secret.yaml
├── README.md              # Publikus dokumentáció
└── basic-memory/
    └── CLAUDE.md          # Ez a fájl


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
  "message": "Formatted alert message",
  "priority": 0,
  "timestamp": 1234567890
}
```

## Biztonsági Szempontok
1. **No secrets in image**: Minden secret env változóból vagy mounted file-ból
2. **Minimal permissions**: ReadOnlyRootFilesystem, RunAsNonRoot
3. **Network policies**: Csak kimenő kapcsolat Pushover felé
4. **Input validation**: Minden bejövő adat validálása
5. **Rate limiting**: DDoS védelem (opcionális)

## Test Stratégia
- Unit testek minden komponenshez
- Integration test mock Pushover API-val
- E2E test real FluxCD alert payload-dal
- Load testing vegeta vagy k6-tal

## Monitoring & Observability
- Structured JSON logs stdout-ra
- Health endpoint monitoring
- Opcionális: Prometheus metrics
- Opcionális: OpenTelemetry traces

## Fejlesztési Jegyzetek

### Env változók
- `PUSHOVER_USER_KEY`: Kötelező, Pushover user key
- `PUSHOVER_API_TOKEN`: Kötelező, Pushover app token + auth token
- `PORT`: Opcionális, default 8080
- `LOG_LEVEL`: Opcionális, default "info"

### Go Best Practices
- Context használata minden async művelethez
- Proper error wrapping (`fmt.Errorf` with `%w`)
- Defer használata cleanup-hoz
- No panic in production code
- Interfaces for testability

### Performance Célok
- <100ms response time webhook-okra
- <20MB memory használat idle
- <50MB memory használat terhelés alatt
- <1s cold start
- 10000+ RPS képesség (single instance)
