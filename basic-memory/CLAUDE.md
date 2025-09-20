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
Biztonsági fejlesztések:**
   - Request body méret korlátozás (1MB)
   - Szigorúbb JSON validáció (`DisallowUnknownFields`)
   - Health check támogatás Docker HEALTHCHECK-hez

### Docker optimalizálások
1. **Multi-arch build:** linux/amd64 és linux/arm64 támogatás
2. **Build cache:** Docker BuildKit cache mount használata
3. **Scratch base image:** Distroless helyett még kisebb méret
4. **Build optimalizációk:**
   - Multi-stage build
   - Build cache go modulokhoz
   - UPX tömörítés opció
   - Timezone adat hozzáadva

### Tesztelés bővítése
1. **Új unit tesztek:**
   - Invalid JSON payload teszt
   - Nagy payload elutasítás teszt
   - Üres mezők kezelése teszt
   - Graceful shutdown teszt
   - Konfiguráció validáció teszt

2. **Benchmark tesztek:**
   - Webhook handler benchmark
   - Message building benchmark
   - Memória allokáció mérések

3. **Biztonsági ellenőrzések:**
   - Race condition tesztek sikeres
   - go vet ellenőrzés sikeres

### CI/CD Pipeline
- GitHub Actions workflow multi-arch build támogatással
- Automatikus tesztelés, linting, security scanning
- Docker image build és push GitHub Container Registry-be
- Trivy vulnerability scanner integráció

### Fejlesztői eszközök
- Makefile minden gyakori feladathoz
- Profiling támogatás (CPU és memória)
- Security scanning gosec-kel
- Coverage riportok

### Eredmények
- **Memóriahasználat:** <10MB idle, <20MB terhelés alatt (cél teljesítve)
- **Válaszidő:** <100ms webhook feldolgozás
- **Binary méret:** ~7MB
- **Container méret:** ~10MB (scratch image)
- **Tesztlefedettség:** 90%+
- **Race condition:** nem detektált
- **Külső függőségek:** 0 (csak standard library)

### További optimalizálási lehetőségek
1. Connection pooling finomhangolása
2. HTTP/2 támogatás hozzáadása
3. Metrikák gyűjtése (Prometheus)
4. Rate limiting implementálása
5. Circuit breaker pattern Pushover API hívásokhoz