---
title: comprehensive-code-audit-2026-05-23
type: note
permalink: flux-provider-pushover/docs/roadmap/comprehensive-code-audit-2026-05-23
tags:
- audit
- security-review
- code-quality
- roadmap
- production-hardening
- detailed
---

## Implementáció Státusz
**Státusz**: ✅ KÉSZ | **Befejezés**: 2026-05-24

| ID | Elem | Státusz |
|----|------|---------|
| S1 | Timing-safe auth (crypto/subtle.ConstantTimeCompare) | ✅ |
| S2 | Pushover error szivárgás javítás (502 + generic msg) | ✅ |
| S3 | Log injection (O1 részeként megoldódik, slog auto-escape) | ✅ |
| B1 | Go verzió alignment (go.mod toolchain go1.26.0) | ✅ |
| O1 | slog strukturált logolás (JSON handler, Info/Warn/Error/With) | ✅ |
| O2 | Request ID propagáció (nano-id 20char base32, X-Request-ID) | ✅ |
| R1 | Server.Start() hibakezelés (channel error pipe, nincs os.Exit) | ✅ |
| R2 | Shutdown context propagáció (BaseContext callback + cancel) | ✅ |
| R3 | HealthCheck timeout (2s saját HTTP client) | ✅ |
| R4 | MaxHeaderBytes szétválasztás (MaxHeaderSize = 8KB) | ✅ |
| Q1-Q7 | Kódminőség javítások (mind) | ✅ |
| R5 | Retry logika (exp backoff jitter, max 2 retry) | ✅ |
| R6 | Circuit breaker (closed/open/halfOpen, stdlib) | ✅ |
| R7 | Rate limiting (per-IP token bucket, x/time/rate) | ✅ |

**Teszteredmények**: go test -race MINDEN ZÖLD, coverage 80%+
**Új függőség**: golang.org/x/time/rate (R7)
### Haladás

| ID | Elem | Státusz |
|----|------|---------|
| S1 | Timing-safe auth összehasonlítás | ⬜ Következik |
| S2 | Pushover error szivárgás javítás | ⬜ Vár |
| S3 | Log injection (O1 részeként megoldódik) | ⬜ Vár |
| B1 | Go verzió alignment | ⬜ Vár |
| O1 | slog strukturált logolás | ⬜ Vár |
| O2 | Request ID propagáció | ⬜ Vár (O1 függő) |
| R1 | Server.Start() hibakezelés | ⬜ Vár |
| R2 | Shutdown context propagáció | ⬜ Vár |
| R3 | HealthCheck timeout | ⬜ Vár |
| R4 | MaxHeaderBytes szétválasztás | ⬜ Vár |
| Q1-Q7 | Kódminőség javítások | ⬜ Vár |
| R5 | Retry logika | ⬜ Vár |
| R6 | Circuit breaker | ⬜ Vár |
| R7 | Rate limiting | ⬜ Vár |

# Átfogó Kód Audit — Flux Provider Pushover (Részletes Kidolgozás)

## Kontextus

Teljes kódbázis (cmd/, internal/, Dockerfile, CI workflow, scripts/) auditja, minden találat részletes elemzéssel: problémaelemzés, attack/failure scenario, javasolt megoldás, alternatívák, teszt stratégia, becsült effort. **Implementáció ehhez a tervhez NEM tartozik** — ez tervezési dokumentum.

- Audit dátum: 2026-05-23
- Audit alapja: \`main\` branch, HEAD = fb8ba8d
- Coverage: 81.7% (config 100%, handlers 83.9%, pushover 88.5%, server 69.2%, cmd/server 55.0%)

## Audit Hatókör

- Forrás: \`cmd/server/\`, \`internal/{config,handlers,pushover,server,types}/\` (≈2911 LoC)
- Build: \`Dockerfile\`, \`go.mod\`, \`scripts/runGoTests.sh\`
- CI: \`.github/workflows/build.yml\`
- Konfiguráció: \`.gosec.json\`, \`.dockerignore\`, \`.gitignore\`

## Erősségek (megőrzendő)

- Nulla külső függőség (\`go.sum\` üres, csak stdlib)
- Distroless nonroot base image, multi-arch (amd64/arm64), SBOM + provenance attestation
- gosec + Trivy + race detection a CI-ban
- Funkcionális config loader pattern (\`ConfigLoader\`, \`WithValidation\`)
- Pure function tervezés (\`BuildPushoverMessage\`, \`ExtractAlertInfo\`)
- Renovate automatizált dependency frissítés
- \`MaxBytesReader\` (1MB request body limit) aktív

---

# P1 — Security

## S1. Timing-unsafe Bearer token összehasonlítás

### Hely
\`internal/handlers/handlers.go:67\`

### Jelenlegi kód
\`\`\`go
if r.Header.Get("Authorization") != deps.Config.BearerToken {
    deps.Logger.Printf("Unauthorized request from %s", r.RemoteAddr)
    writeJSONResponse(w, http.StatusUnauthorized, types.ResponseUnauthorized)
    return
}
\`\`\`

### Problémaelemzés

Go nyelvi \`!=\` operátor \`string\` típuson byte-szintű, short-circuit összehasonlítást végez: amint az első eltérő byte-ot találja, kilép. Ez méréshullámot generál CPU cache és branch predictor szintjén: minél több byte egyezik a támadó által küldött token-ből a valódi token elejével, annál hosszabb a feldolgozási idő.

A különbség nanoszekundumokban mérhető lokálisan, de:
- Aggregálva (10000+ minta) statisztikailag elkülöníthető
- LAN környezetben a hálózati jitter alatti
- Multi-tenant Kubernetes namespace-en futó pod-okhoz különösen hozzáférhető

### Attack scenario

\`\`\`
1. Támadó hozzáfér a klaszter belső hálózatához (compromised pod)
2. Iteratívan próbálkozik token byte-onként
3. Próbánként 10000 request, latency átlagot mér
4. A lassabb átlag jelzi a helyes karaktert (a long match miatt)
5. 32-karakteres token: ~20M request, néhány óra alatt
\`\`\`

### Javasolt megoldás

\`crypto/subtle.ConstantTimeCompare\`:

\`\`\`go
import "crypto/subtle"

authHeader := r.Header.Get("Authorization")
if subtle.ConstantTimeCompare([]byte(authHeader), []byte(deps.Config.BearerToken)) != 1 {
    deps.Logger.Printf("Unauthorized request from %s", r.RemoteAddr)
    writeJSONResponse(w, http.StatusUnauthorized, types.ResponseUnauthorized)
    return
}
\`\`\`

**Szempontok**:
- Eltérő hosszúságú string-eknél 0-t ad vissza, időkomplexitás \`min(len(a), len(b))\` — stdlib megoldása konstans idejű
- \`[]byte\` konverzió allokál — elfogadható overhead

### Alternatívák

| Megközelítés | Pro | Kontra |
|---|---|---|
| \`subtle.ConstantTimeCompare\` (stdlib) | Nulla függőség, audit-elt | - |
| \`hmac.Equal\` | Szintén stdlib | Szemantikailag MAC-comparison |
| Pre-computed SHA-256 + compare | Rövidebb side-channel ablak | Felesleges complexity |

**Választás**: \`subtle.ConstantTimeCompare\`.

### Teszt stratégia

- Unit: helyes token PASS, rossz token UNAUTHORIZED, üres/eltérő hossz UNAUTHORIZED
- Negative: prefix-egyezés ne legyen elég (\`"Bearer test_tokenX"\` vs \`"Bearer test_token"\`)
- Benchmark: \`BenchmarkAuthCompare\` — helyes és helytelen token ideje ±1σ-n belül

### Becsült effort

- LoC <10, új import, ~30 sor új teszt
- Idő: 30 perc

### Hivatkozások

- [Go \`crypto/subtle.ConstantTimeCompare\`](https://pkg.go.dev/crypto/subtle#ConstantTimeCompare)
- [CWE-208: Observable Timing Discrepancy](https://cwe.mitre.org/data/definitions/208.html)

---

## S2. Pushover hibarészletek szivárgása a kliens felé

### Hely
\`internal/handlers/handlers.go:114-121\`

### Problémaelemzés

\`err.Error()\` a \`PushoverClient.SendMessage\` által visszaadott üzenet (\`client.go:60-66\`): \`fmt.Errorf("pushover API returned status %d: %s", resp.StatusCode, string(body))\`.

A response body (max 512 byte) közvetlenül a Pushover API válasza, tartalmazhat:
- Pushover hibakódot (\`{"errors":["application token is invalid"]}\`) — token állapotról info
- Rate limit infót
- Account-szintű korlátot
- Belső Pushover hibát

A relay ezt szó szerint továbbítja FluxCD-nek a \`details\` mezőben.

### Failure scenario

\`\`\`
1. Token rotation lekésve → Pushover 400: "application token is invalid"
2. FluxCD-nek visszamegy: {"error":"...","details":"..."}
3. FluxCD log aggregator indexeli
4. Akinek log olvasási joga van (de Pushover secret-hez nincs), megtudja a token állapotát
\`\`\`

Sérti az **Information Disclosure** alapelvet.

### Javasolt megoldás

Kétszintű elválasztás: belső log vs. külső válasz.

\`\`\`go
if err := deps.PushoverClient.SendMessage(ctx, pushoverMsg); err != nil {
    deps.Logger.Printf("Pushover send failed: %v (request_id=%s)", err, requestID)
    writeJSONResponse(w, http.StatusBadGateway, types.ResponseUpstreamError)
    return
}
\`\`\`

Változtatások:
1. Status code: 500 → 502 Bad Gateway (helyesebb — upstream hiba)
2. \`details\` mező eltávolítása
3. Pre-defined \`types.ResponseUpstreamError\` konstans
4. Request ID a log-ban (O2 függőség)

### Alternatívák

| Opció | Pro | Kontra |
|---|---|---|
| Generic message | Egyszerű, biztonságos | Operatornek logot kell olvasnia |
| Sanitized details mapping | Több info kliensnek | Karbantartási teher |
| Request ID + generic | Operator korrelálhat | Igényli O2-t |

**Választás**: generic + request ID.

### Teszt stratégia

- Unit: Pushover mock különböző hibákat ad → válaszban NE legyen \`details\`, NE Pushover-specifikus string
- Snapshot: válasz JSON shape rögzítése
- Negative: Pushover válasz speciális karakterekkel → továbbra is valid JSON

### Becsült effort

- LoC ~15, új konstans
- Status code változás (500 → 502): FluxCD retry policy-t figyelni
- Idő: 45 perc (Q3-mal együtt)

### Hivatkozások

- [OWASP — Improper Error Handling](https://owasp.org/www-community/Improper_Error_Handling)
- [CWE-209: Information Exposure Through an Error Message](https://cwe.mitre.org/data/definitions/209.html)

---

## S3. Log injection a sikerágban

### Hely
\`internal/handlers/handlers.go:130\`

### Jelenlegi kód
\`\`\`go
info := ExtractAlertInfo(&alert)
deps.Logger.Printf("Successfully sent alert to Pushover for %s/%s", info["kind"], info["name"])
\`\`\`

\`info["kind"]\` és \`info["name"]\` a request body-ból parsolódik — **user-controlled**.

### Problémaelemzés

\`log.Printf\` plain text-et ír stdout-ra. Egy CRLF-fel ellátott objektumnév hamis log sort csempészhet be — log aggregator-ban hamis ADMIN audit eseményt indexelhet.

### Attack scenario

\`\`\`
1. Támadó (vagy hibás konfig) létrehoz egy Deployment-et gonosz névvel
2. Flux esemény keletkezik, a relay loggolja
3. Hamis ADMIN audit log → incident response zavar
\`\`\`

Belső támadási felület, de **integrity** sérül.

### Javasolt megoldás

Hosszú távú (O1 része): \`slog\` JSON output — automatikusan escape-el.

\`\`\`go
slog.Info("alert sent to Pushover",
    slog.String("kind", info["kind"]),
    slog.String("name", info["name"]),
    slog.String("request_id", requestID))
\`\`\`

Rövid távú: \`strconv.Quote\`.

### Alternatívák

| Megközelítés | Pro | Kontra |
|---|---|---|
| \`slog\` JSONHandler | Strukturált, auto-escape | Új interface, refactor |
| \`strconv.Quote\` | Egy-soros | Idézőjelek a logban |
| \`strings.ReplaceAll\` egyedi | Teljes kontroll | Fragile |
| Whitelist regex | Szigorú | K8s névkonvenció pont ez |

**Választás**: O1 (slog).

### Teszt stratégia

- Unit: handler gonosz string-ekkel (CRLF, ANSI, null byte) → log output valid JSON
- Property-based: random byte sequence → log output 1 sorral nőjön

### Becsült effort

- O1-mel együtt: 0 extra LoC
- Önállóan: <5 sor

### Hivatkozások

- [OWASP — Log Injection](https://owasp.org/www-community/attacks/Log_Injection)
- [CWE-117: Improper Output Neutralization for Logs](https://cwe.mitre.org/data/definitions/117.html)

---

# P1 — Build korrektség

## B1. Go verzió eltérés go.mod ↔ Dockerfile ↔ CI

### Hely
- \`go.mod:3\`: \`go 1.22\`
- \`Dockerfile:3\`: \`golang:1.26-alpine\`
- \`.github/workflows/build.yml:104\`: \`go-version: '1.26'\`

### Problémaelemzés

A \`go\` direktíva a minimum required version. Go 1.21 óta van \`toolchain\` direktíva.

**Forgatókönyv 1**: Helyi 1.22, fejlesztő használ 1.23+ feature-t → lokál nem fordít.

**Forgatókönyv 2**: Helyi 1.26, CI 1.26 — minden zöld; új fejlesztő 1.22-vel build fail.

**Forgatókönyv 3** (jelenlegi): Code 1.22-kompatibilis, de a drift "működik" amíg valaki nem old fel egy függőséget 1.23+ igénnyel.

### Javasolt megoldás

**Opció A** (1.22 marad): minden helyen 1.22.

**Opció B** (1.26 + toolchain pin):
\`\`\`
go.mod:     go 1.22
            toolchain go1.26.0
Dockerfile: golang:1.26-alpine
CI:         go-version: '1.26'
\`\`\`

**Választás**: B.

### Teszt stratégia

- \`go vet ./...\` és \`go test ./...\` mindkét toolchain-en
- CI matrix: 1.22 + 1.26

### Becsült effort

- LoC <10, 3 fájl, idő 15 perc

---

# P2 — Megbízhatóság

## R1. \`Server.Start()\` hibakezelés anti-pattern

### Hely
\`internal/server/server.go:43-58\`

### Problémaelemzés

Három kritikus hiba:

1. **\`os.Exit(1)\` egy goroutine-ban**: Megöli a folyamatot takarítás nélkül.
2. **\`Start()\` szignatúra hazudik**: Mindig \`nil\`-t ad vissza. \`RunApp\` továbblép \`WaitForShutdown\`-ba — ami soha nem fog signal-t kapni.
3. **\`GO_TEST\` env-var test-only escape hatch**: Production kódban code smell.

### Failure scenario

\`\`\`
1. Container indul, PORT=8080 már foglalt
2. ListenAndServe hibázik
3. os.Exit(1) → container restart loop
\`\`\`

### Javasolt megoldás

Channel-alapú error propagation. \`Start()\` egy rövid select-tel megvárja az indulási hibát, ha van, vagy 100ms után OK-ot ad vissza. \`WaitForShutdown\` select-tel hallgat signal-re vagy runtime hibára.

### Alternatívák

| Megközelítés | Pro | Kontra |
|---|---|---|
| Channel error pipe | Stdlib only, testable | Új state-channel |
| \`errgroup\` | Kanonikus | Új függőség |
| Sync Start, külön Run | Egyszerű | 2 metódus |

**Választás**: Channel error pipe.

### \`GO_TEST\` törlése

Az új design-ban nem szükséges (nincs \`os.Exit\`).

### Teszt stratégia

- Unit: \`Start()\` invalid port → error
- Integration: \`RunApp\` invalid port → error main-ig

### Becsült effort

- LoC ~40, idő 1.5 óra, risk: API változás

---

## R2. Shutdown context nem propagálódik a webhook handler-be

### Hely
\`internal/handlers/handlers.go:107\`

### Problémaelemzés

\`context.Background()\` gyökér context — SIGTERM esetén a Pushover hívás nem szakad meg automatikusan.

### Failure scenario

\`\`\`
1. Pod kap SIGTERM
2. http.Server.Shutdown elindul, 30s timeout
3. Aktív request: Pushover hívás folyamatban
4. Pushover válaszol, de TCP kapcsolat bontva → nem nyugtáz
5. FluxCD timeout, retry → DUPLIKÁCIÓ
\`\`\`

### Javasolt megoldás

\`BaseContext\` callback: a Server-szintű \`context.WithCancel\` által generált ctx-et propagáljuk a request-ekbe. Shutdown-kor \`cancel()\` minden in-flight request-et leállít.

Handler: \`ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)\`

### Alternatívák

| Megközelítés | Pro | Kontra |
|---|---|---|
| \`BaseContext\` callback | Stdlib, idiomatikus | Új field a Server-ben |
| Manuális ctx injection | Explicit | Refactor |
| Globális context | Egyszerű | Anti-pattern |

**Választás**: \`BaseContext\`.

### Teszt stratégia

- Integration: slow Pushover mock (5s), shutdown T+1s → mock context cancellation-t kap, shutdown 1s-en belül

### Becsült effort

- LoC ~20, idő 1 óra

### Hivatkozások

- [Go \`http.Server.BaseContext\`](https://pkg.go.dev/net/http#Server.BaseContext)

---

## R3. \`HealthCheck\` HTTP kliens timeout nélkül

### Hely
\`internal/server/server.go:88\`

### Problémaelemzés

\`http.DefaultClient\` \`Timeout: 0\` (végtelen). Docker HEALTHCHECK 3s külső timeout elfedi, de helyileg / külső használat esetén hang.

### Javasolt megoldás

Lokális \`&http.Client{Timeout: 2*time.Second}\` + body drain connection reuse-hoz.

### Teszt stratégia

- Új teszt: nem-elérhető URL → 2s-en belül error

### Becsült effort

- LoC ~5, idő 15 perc

---

## R4. \`MaxHeaderBytes = MaxBodySize\` szemantikai keveredés

### Hely
\`internal/server/server.go:36\` + \`internal/types/types.go:48\`

### Problémaelemzés

\`MaxBodySize\` (1MB) használt \`MaxHeaderBytes\`-ként. Tipikus header: ~500 byte. 1MB header limit DoS vektor.

### Javasolt megoldás

\`\`\`go
const (
    MaxBodySize   = 1 << 20  // 1MB
    MaxHeaderSize = 1 << 13  // 8KB
)
\`\`\`

### Teszt stratégia

- Új teszt: nagy header (16KB+) → 431

### Becsült effort

- LoC <5, idő 15 perc

---

# P2 — Operability

## O1. Strukturált logolás (slog) bevezetése

### Hely
- \`cmd/server/main.go:13-19\`
- \`internal/server/server.go:18-21\` (\`Logger\` interface)
- Minden \`Printf\` / \`Println\` használat

### Problémaelemzés

CLAUDE.md követeli "Structured JSON logs stdout-ra"-t. Jelenleg plain text:
1. Nem parsolható log aggregator-okban
2. Nincs severity szint
3. Nincs request korreláció
4. Log injection-vulnerable (S3)

### Javasolt megoldás

\`log/slog\` (stdlib) + új \`Logger\` interface (\`Info\`, \`Warn\`, \`Error\`, \`With\`).

\`\`\`go
type SlogLogger struct { *slog.Logger }

func NewSlogLogger() *SlogLogger {
    handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    return &SlogLogger{Logger: slog.New(handler)}
}
\`\`\`

Output:
\`\`\`json
{"time":"2026-05-23T18:28:12Z","level":"INFO","msg":"alert sent","kind":"HelmRelease","name":"tuppr","request_id":"abc123"}
\`\`\`

### Backward compatibility

Stratégia A: Lecseréljük az interface-t (breaking change, mock-okat frissítjük).
Stratégia B: Adapter, régi interface nyugdíjazva.

**Választás**: A.

### Alternatívák

| Megközelítés | Pro | Kontra |
|---|---|---|
| \`log/slog\` (stdlib) | 0 függőség | Friss (Go 1.21+) |
| \`zerolog\` | Sebesség | Külső függőség |
| \`zap\` | Sebesség | Külső függőség |

**Választás**: \`log/slog\`.

### Teszt stratégia

- Unit: \`SlogLogger\` JSON output, mezők formátuma
- Integration: request → valid JSON log
- Snapshot: kanonikus mezőkészlet

### Becsült effort

- ~70 sor új \`logger.go\`, ~30 sor change handlers/server, ~50 sor mock frissítés
- Idő: 3 óra

### Hivatkozások

- [Go \`log/slog\`](https://pkg.go.dev/log/slog)

---

## O2. Request ID propagáció

### Probléma

Nincs lehetőség egy FluxCD webhook hívást végigkövetni a relay-en keresztül.

### Javasolt megoldás

Middleware: \`X-Request-ID\` header generálás (vagy meglévő preservation), context-be propagálás, response header echo. Logger \`With("request_id", rid)\`.

### Alternatívák

| Forma | Méret | Megjegyzés |
|---|---|---|
| UUID v4 | 36 char | Standard |
| UUID v7 | 36 char | Sortable |
| nano-id base32 | 20 char | Kompakt |
| Hex 16-byte | 32 char | Egyszerű |

**Választás**: nano-id (20 char base32) — 0 függőség.

### Teszt stratégia

- Middleware új ID-t generál header hiányában
- Megőrzi a meglévő \`X-Request-ID\`-t
- Response header tartalmazza

### Becsült effort

- ~50 sor middleware + ~20 sor handler
- Idő: 1.5 óra
- Függ: O1

---

## O3. Prometheus metrics endpoint

### Probléma

Production webhook receiver-ként hasznos lenne: request rate, status distribution, success rate, latency percentiles, in-flight count.

### Javasolt megoldás

**Opció A: Stdlib-only** (~80 sor saját Counter/Histogram + Prometheus text-format render).

**Opció B: \`prometheus/client_golang\`** — hivatalos library, 3 indirect dep.

### Alternatívák

| Opció | Pro | Kontra |
|---|---|---|
| Stdlib-only | "0 függőség" megőrződik | Reinventing wheel |
| \`prometheus/client_golang\` | Standard | Új függőség |
| OpenTelemetry metrics | Vendor-neutral | Sokkal több függőség |

**Választás**: opció B, **mérlegelés alatt**.

### Metrikák

| Név | Típus | Címke |
|---|---|---|
| \`http_requests_total\` | counter | method, path, status |
| \`http_request_duration_seconds\` | histogram | path |
| \`http_requests_in_flight\` | gauge | - |
| \`pushover_requests_total\` | counter | result |
| \`pushover_request_duration_seconds\` | histogram | - |

### Teszt stratégia

- Unit: counter Inc → érték helyes
- Integration: 100 request → \`/metrics\` body tartalmazza \`http_requests_total 100\`
- Smoke: \`promtool check metrics\` CI-ban

### Becsült effort

- Stdlib-only: ~80 sor, 4 óra
- Library-based: ~30 sor, 1 óra + go.sum változás

---

# P3 — Kódminőség

## Q1. Buggy \`contains\` helper

### Hely
\`cmd/server/main_test.go:194-201\`

### Problémaelemzés

\`strings.Contains\` stdlib pontosan ezt csinálja, audit-elve, Boyer-Moore optimalizálva. Saját implementáció felesleges duplikáció és bug potenciál.

### Javasolt megoldás

Törölni a saját helper-t, \`strings.Contains\` használata.

### Becsült effort

- -20 sor (törlés), idő 10 perc

---

## Q2. \`t.Log\` használata \`t.Error\` helyett

### Helyek
- \`internal/server/server_test.go:104-110\`
- \`internal/handlers/edge_case_test.go:55\` (\`TestServer_StartError\`)
- \`internal/handlers/edge_case_test.go:79-84\`
- \`cmd/server/main_test.go:135,154,177\`

### Problémaelemzés

\`t.Log\`-os hibakezelés "coverage theater" — coverage felmegy, de a teszt nem validál semmit.

### Javasolt megoldás

Csoportosítva:
- **A**: várt hiba → \`t.Error\`/\`t.Fatal\`
- **B**: nem-determinisztikus invariáns → teszt törlése
- **C**: jegyzet jellegű \`t.Log\` → marad, ha van mellette assertion

### Becsült effort

- ~40 sor változás (vagy ~30 sor törlés)
- Idő: 1 óra
- Coverage csökkenhet 1-2 százalékpontot

---

## Q3. Inkonzisztens error response shape

### Helyek
- \`handlers.go:117\` — \`{"error":"...","details":"..."}\`
- \`types.go:55-59\` — \`{"error":"..."}\`

### Javasolt megoldás

S2-vel együtt rendezve: minden válaszban csak \`error\`, opcionálisan \`request_id\` (O2 után).

### Teszt stratégia

- Snapshot: minden status code → minden mező rögzítve

### Becsült effort

- S2-vel együtt: 0 plusz LoC

---

## Q4. CORS handler placeholder

### Hely
\`internal/handlers/handlers.go:58-61\`

### Problémaelemzés

OPTIONS-re 200 OK, de nincs \`Access-Control-Allow-*\` header. Preflight kéréshez nem elég. FluxCD nem böngésző-alapú — nem küld OPTIONS-t.

### Javasolt megoldás

Törölni (YAGNI).

### Becsült effort

- -10 sor, idő 10 perc

---

## Q5. Nil-unsafe MockHTTPClient

### Helyek
- \`internal/pushover/client_test.go:22\`: panicol ha \`DoFunc == nil\`
- \`internal/handlers/edge_case_test.go:25-30\`: van nil-check

### Javasolt megoldás

Opció A: Közös \`internal/testutil/mocks/\` package. Opció B: inline nil-check \`client_test.go\`-ban.

**Választás**: B rövid távon, A hosszú távon.

### Becsült effort

- B: <5 sor; A: ~30 sor + refactor

---

## Q6. Üres test sub-test

### Hely
\`internal/handlers/dependencies_test.go:65-72\`

### Javasolt megoldás

Valódi route validáció: HTTP request mind a 3 path-ra, ellenőrizni hogy nincs 404.

### Becsült effort

- ~10 sor, idő 15 perc

---

## Q7. Input validáció mélysége

### Hely
\`internal/handlers/message.go:60-66\`

### Problémaelemzés

\`MaxBytesReader\` 1MB total limit, de:
- Egyetlen metadata kulcs lehet 900KB
- Pushover üzenet (1024 char Pushover limit) nincs vágva
- \`Name\`, \`Kind\`, \`Reason\` bármilyen hosszú

### Failure scenario

\`\`\`
1. Hibás konfig 500KB-os "name" mezőt küld
2. Validáció átengedi
3. BuildPushoverMessage 500KB-os string
4. Pushover 400 → relay 502 → FluxCD retry → végtelen
\`\`\`

### Javasolt megoldás

Per-mező hosszkorlátok:
- \`MaxStringFieldLen = 512\`, \`MaxMessageFieldLen = 4096\`
- \`MaxMetadataKeyLen = 128\`, \`MaxMetadataValueLen = 1024\`, \`MaxMetadataEntries = 32\`
- \`PushoverMessageMax = 1024\` — \`BuildPushoverMessage\` csonkít

### Alternatívák

| Megközelítés | Pro | Kontra |
|---|---|---|
| Hard reject | Egyszerű, defenzív | FluxCD hibát kap |
| Silent truncate | Mindig sikeres | Adatvesztés rejtett |
| Hybrid: truncate + warning log | Best-effort + observability | Komplexitás |

**Választás**: Hard reject Validate-ben + truncate BuildPushoverMessage-ben.

### Becsült effort

- ~50 sor, idő 1 óra

---

# P3 — Reziliencia

## R5. Retry logika Pushover hívásokra

### Probléma

Tranziens 5xx / hálózati hiba esetén nincs retry.

### Javasolt megoldás

Exponenciális backoff jitter-rel, max 2 retry, csak idempotens hibákra (5xx, network, 429). Konfiguráció: \`InitialDelay 200ms\`, \`MaxDelay 2s\`, \`Multiplier 2.0\`, \`MaxAttempts 3\`.

### Alternatívák

| Library | Pro | Kontra |
|---|---|---|
| Stdlib | 0 függőség | ~60 sor új kód |
| \`hashicorp/go-retryablehttp\` | Standard | Új dependency |
| \`avast/retry-go\` | Generikus | Új dependency |

**Választás**: Stdlib.

### Teszt stratégia

- Mock 5xx → 5xx → 200 ⇒ siker
- Mock 5xx ×3 ⇒ hiba
- Mock 400 ⇒ azonnali hiba
- Cancelled context ⇒ szüneteltetés

### Becsült effort

- ~80 sor + ~50 sor teszt, idő 2 óra
- Risk: latency növekedés worst-case (~600ms)

---

## R6. Circuit breaker pattern

### Probléma

Pushover outage → minden request 10s + 2 retry × backoff = ~12s. Goroutine pool kimerül, FluxCD timeout-okra fut, retry storm.

### Javasolt megoldás

Állapotgép (\`closed\` / \`open\` / \`halfOpen\`) a \`PushoverClient\` szintjén.

Paraméterek:
- \`failureThreshold = 5\` (5 hiba 30s alatt → open)
- \`timeout = 30s\` (open → half-open delay)
- \`successThreshold = 2\` (half-open-ben 2 sikeres → closed)

### Alternatívák

| Megközelítés | Pro | Kontra |
|---|---|---|
| Stdlib saját | 0 függőség | ~100 sor + jól-tesztelés |
| \`sony/gobreaker\` | Jól tesztelt | Új dependency |
| \`afex/hystrix-go\` | Hystrix port | Halott projekt |

**Választás**: Stdlib.

### Teszt stratégia

- State transition: closed → open → half-open → closed
- Concurrency: 100 goroutine, állapot konzisztens
- Timing: open state időtartam pontos

### Becsült effort

- ~120 sor + ~80 sor teszt, idő 4 óra
- Risk: subtle state machine bug-ok

---

## R7. Rate limiting

### Probléma

Webhook endpoint nyitott. 1000+ event/sec vagy spam → Pushover quota (10000/month) kimerül.

### Javasolt megoldás

Per-IP token bucket: \`rate = 10\` req/sec, \`burst = 30\`. Idle bucket cleanup 1h után.

### Alternatívák

| Forma | Pro | Kontra |
|---|---|---|
| Per-IP token bucket | Egyszerű | Spoofable header |
| Globális | Egyszerű | Egy rossz kliens kizárja a többit |
| Token-based (Bearer) | Pontosabb | Mind ugyanaz a token |
| \`golang.org/x/time/rate\` | Hivatalos | Új function dep |

**Választás**: \`x/time/rate.Limiter\`.

### Teszt stratégia

- 30 request burst → 30 OK, 31. → 429
- Várás után tokenek visszatöltődnek
- 100 goroutine → pontos count
- Idle bucket törlődik

### Becsült effort

- ~100 sor + ~50 sor teszt, idő 3 óra
- Risk: false positive — tunable

---

# Egyéb

## M1. CLAUDE.md frissítése

### Hely
\`CLAUDE.md\` projekt root

### Problémaelemzés

1. "Python alapú middleware újraimplementációja Go nyelven" — már nincs Python kontextus
2. Duplikált szekciók a fájl második felében
3. Coverage 90%+ claim, valóságban 81.7%
4. Fejlesztési jegyzetek elszórtak

### Javasolt megoldás

Refactor:
- Projekt áttekintés: aktuális Go-only state
- Architektúra: package-ek és felelősségek
- Build & deploy: linkek
- Test stratégia: aktuális coverage
- Roadmap link: \`basic-memory/docs/roadmap/\`
- TÖRÖLNI: duplikációk, elavult referenciák

Történeti narratíva → \`basic-memory/docs/decisions/\` ADR-formátumban.

### Becsült effort

- ~150 sor change, idő 1 óra

---

## M2. CHANGELOG.md hiánya

### Probléma

Tag-ek vannak (\`v0.x.y\`), de nincs ember-olvasható release note.

### Javasolt megoldás

\`CHANGELOG.md\` Keep a Changelog formátumban + \`git-cliff\` CI integrált auto-generálás.

### Alternatívák

| Eszköz | Pro | Kontra |
|---|---|---|
| \`git-cliff\` | Conventional commits-ből | Új tooling |
| \`release-please\` | Hivatalos | GitHub-specifikus |
| Kézi | Egyszerű | Elfeledődik |

**Választás**: \`git-cliff\`.

### Becsült effort

- Initial ~100 sor + CI ~30 sor, idő 2 óra

---

## M3. EXPOSE / PORT inkonzisztencia

### Hely
\`Dockerfile:48\` (\`EXPOSE 8080\`) + \`PORT\` env var

### Problémaelemzés

A \`PORT\` env override-olja a 8080-at, de \`EXPOSE\` informatív és 8080-at hirdet. Kubernetes Service \`targetPort\` ezt nem követi.

### Javasolt megoldás

**Opció A**: \`PORT\` override eltávolítása — minden 8080.

**Opció B**: \`PORT\` env megtartása + Dockerfile dokumentáció (komment).

**Opció C**: Build-time \`ARG PORT=8080\` + \`ENV PORT=${PORT} + ${PORT} + `EXPOSE` dinamikus.

**Választás**: B — minimális változás, dokumentáció elég.

### Becsült effort

- 5 sor docs, idő 10 perc

---

# Megvalósítási sorrend (függőségi gráf)

```
B1 (Go verzió) ──┐
                 │
S1 (timing-safe) ─┤
                  │
S2+Q3 (error shape) ──┐
                       │
O1 (slog) ─→ S3 (log inject solved)
       │
       └─→ O2 (request ID)
              │
              └─→ O3 (metrics) [optional]
                     
R1 (start error) ─→ R2 (shutdown ctx)
                            │
R3 (health timeout) ─┐      │
R4 (header limit) ───┴──→ végső megbízhatóság szint
                            
Q1, Q2, Q5, Q6 — függőség nélkül
Q4 (CORS törlés) — független
Q7 — S2 után célszerű

R5 (retry) ─→ R6 (circuit breaker)
R7 (rate limit) — független

M1, M2, M3 — dokumentáció, végén
```

## Javasolt MR-csoportosítás

| MR | Tartalom | LoC | Effort |
|---|---|---|---|
| MR-1 | B1 + S1 (build + security alap) | <30 | 1 óra |
| MR-2 | S2 + Q3 (error response shape) | ~40 | 1 óra |
| MR-3 | O1 (slog + S3 megoldás) | ~150 | 3 óra |
| MR-4 | O2 (request ID) | ~70 | 1.5 óra |
| MR-5 | R1 + R2 + R3 + R4 (server reliability) | ~80 | 3 óra |
| MR-6 | Q1+Q2+Q5+Q6 (test cleanup) | ~40 | 1 óra |
| MR-7 | Q4 + Q7 (CORS + input validation) | ~60 | 1 óra |
| MR-8 | R5 (retry) | ~130 | 2 óra |
| MR-9 | R6 (circuit breaker) | ~200 | 4 óra |
| MR-10 | R7 (rate limit) | ~150 | 3 óra |
| MR-11 | O3 (Prometheus metrics) | ~150 | 4 óra |
| MR-12 | M1 + M2 + M3 (docs cleanup) | ~250 | 3 óra |

**P1+P2 (MR-1—MR-5)**: ~370 LoC, ~10 óra.
**P3 reziliencia (MR-8/9/10)**: ~480 LoC, ~9 óra.
**Observability (O3)**: ~150 LoC, ~4 óra.
**Docs**: ~250 LoC, ~3 óra.
**Összes**: ~1300 LoC, ~30 óra. Realisztikusan: 1 hét időablak.

# Acceptance Criteria (közös)

- [ ] `go test ./...` PASS minden MR után
- [ ] `go test -race ./...` PASS
- [ ] `go vet ./...` tiszta
- [ ] gosec scan: nincs új high/medium severity finding
- [ ] Coverage nem csökken modulszinten (kivéve Q2 tudatos törlés)
- [ ] Új public API: unit teszt + (ha hot path) benchmark
- [ ] CI pipeline zöld

# Elutasított / elhalasztott javaslatok

- **OpenTelemetry tracing**: Túl-engineering jelenlegi forgalmi méretre. Releváns multi-tenant-nél vagy >100 req/s sustained
- **gRPC interface**: Pushover REST-only, FluxCD HTTP — nincs use case
- **Kubernetes operator integráció**: Külön repo / projekt scope
- **Multi-tenant deployment**: Komplex, külön spec szintű döntés

# Hivatkozások

- [Go `crypto/subtle.ConstantTimeCompare`](https://pkg.go.dev/crypto/subtle#ConstantTimeCompare)
- [Go `log/slog` package](https://pkg.go.dev/log/slog)
- [Pushover API limits](https://pushover.net/api#limits)
- [FluxCD Event v1beta1](https://github.com/fluxcd/pkg/blob/main/apis/event/v1beta1/event.go)
- [OWASP Cheat Sheet — Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [Keep a Changelog](https://keepachangelog.com/)
- [Conventional Commits](https://www.conventionalcommits.org/)

# Relations

- relates_to [[flux-v2-event-compatibility-fix]]
- implements [[Production Hardening]]