---
title: flux-v2-event-compatibility-fix
type: progress
permalink: flux-provider-pushover/docs/progress/flux-v2-event-compatibility-fix
tags:
- bugfix
- security-review
- forward-compatibility
- fluxcd
---

# Flux v2 Event Payload Kompatibilitás Javítás — Audit Eredmények

## Javítás Összefoglaló

A HelmRelease események elutasítása (`json: unknown field "app-version"`) miatt a relay 400-as hibát adott. A gyökérok a `Metadata` fix struct és a `DisallowUnknownFields()` hívás volt. A javítás a FluxCD Event API-nak megfelelő `map[string]string` típusra váltott, és eltávolította a szigorú JSON deszerializációt.

### Változtatások

| Fájl | Változás |
|---|---|
| `internal/types/types.go` | `ObjectReference` nevesített típus, `Metadata map[string]string`, `ReportingInstance` omitempty |
| `internal/handlers/handlers.go` | `DisallowUnknownFields()` eltávolítása |
| `internal/handlers/message.go` | Map indexelés, `app-version` opcionális megjelenítés |
| `internal/handlers/handlers_test.go` | Új tesztesetek, struct literálok frissítése |
| `internal/handlers/message_test.go` | Új tesztesetek, struct literálok frissítése |
| `internal/handlers/handlers.go` (utánkövető) | `json.Marshal` hiba kezelése (commit 8226672) |
| `internal/types/types.go` (utánkövető) | Struct mezők igazítása `gofmt` ellenőrzéshez (commit 845e312) |

### Tesztek — Mind zöld

- `go test ./...` — PASS
- `go vet ./...` — tiszta
- `go test -race ./...` — nincs race condition
- Új tesztesetek: app-version metadata, unknown top-level field, multiple metadata keys, nil metadata, empty app-version

---

## Review Megállapítások

### Kozmetikai

1. **Üres sor maradvány** (`handlers_test.go:30`): A `contains` helper törlése után üres sor maradt. Nem funkcionális hiba.
2. **Hiányzó trailing newline**: `handlers_test.go` és `message_test.go` utolsó sorából hiányzik a sor végi újsor.

### Funkcionális

3. **`ExtractAlertInfo` nem adja vissza az `app-version`-t**: A függvény csak a korábbi mezőket adja vissza. Ha valaki `ExtractAlertInfo`-ból próbálná kiolvasni az `app-version`-t, nem fogja megtalálni. Jelenleg nem hiba (csak a logolás használja), de jövőbeli API bővítésnél figyelni kell.

---

## Security Review Megállapítások

### Alacsony kockázat

1. **JSON injection az error response-ban** (`handlers.go:114`): A `fmt.Sprintf` nem védez JSON escaping-gel. Ha a Pushover hibaüzenet tartalmaz `"` vagy `\` karaktert, a JSON response sérülhet. Nincs XSS kockázat (nem böngésző), de nem valid JSON. **Javaslat**: `json.Marshal` használata.

2. **`DisallowUnknownFields()` eltávolítása**: Forward compatibility szempontból helyes, de typókkal (pl. `severety` `severity` helyett) nem jelez hibát. **Elfogadható trade-off** — a fix struct megközelítés már bizonyítottan törékeny.

3. **`map[string]string` metadata**: Bármilyen kulcs-érték pár elfogadásra kerül. Mivel az értékek csak a Pushover üzenetbe kerülnek szövegként (`fmt.Sprintf`), és a Pushover API nem interpretálja őket, **nincs injection kockázat**.

### Nincs kockázat

- Nil map hozzáférés: Go-ban biztonságos, `defaultIfEmpty` kezeli
- Request body méretkorlátozás: `MaxBytesReader` (1MB) továbbra is aktív
- Bearer token auth: érintetlen
- Nincs új külső függőség: 0 függőség megőrizve

---

## Jövőbeli Teendők

### Javasolt (a review-ból estek ki)

- [x] **JSON escaping az error response-ban**: `handlers.go:114` — `fmt.Sprintf` cserélve `json.Marshal`-ra (commit b9d6826)
- [x] **Trailing newline javítás**: `handlers_test.go` és `message_test.go` utolsó sorához újsor hozzáadva (commit b9d6826)
- [x] **Üres sor eltávolítása**: `handlers_test.go:30` üres sor törölve (commit b9d6826)
- [x] **`json.Marshal` hibakezelése**: Pushover failure ágban a `marshalErr` ellenőrzése, fallback 500-as válasszal (commit 8226672 — utánkövető security fix)
- [x] **`gofmt` struct alignment**: `FluxAlert`/`ObjectReference` mezők igazítása a `Metadata map[string]string` átállás után (commit 845e312)

### További javaslatok (nem a javítás része)

- [ ] **`ExtractAlertInfo` bővítése**: Ha a jövőben az `app-version`-t is ki kell olvasni (pl. strukturáltabb logolás), a függvény bővítendő
- [ ] **Rate limiting**: A CLAUDE.md-ben szerepel opcionális teendőként DDoS védelem
- [ ] **Circuit breaker**: Pushover API hívásokhoz (CLAUDE.md-ben szerepel)
- [x] ~~**Prometheus metrics**: Opcionális, a CLAUDE.md-ben szerepel~~ → **Elhalasztva** (YAGNI: lightweight middleware, slog + request ID elegendő)
- [ ] **Connection pooling finomhangolás**: CLAUDE.md-ben szerepel

### Elutasított megközelítések (dokumentáció)

- **`DisallowUnknownFields()` megtartása + minden ismert mező hozzáadása a struct-hoz**: Elutasítva, mert minden új Flux verzió potenciálisan új mezőt adhat hozzá, ami új relay releaset igényelne. Sérti a forward compatibility elvet.

---

## Források

- [FluxCD Event v1beta1 struct](https://github.com/fluxcd/pkg/blob/main/apis/event/v1beta1/event.go)
- [FluxCD Event metadata konstansok](https://github.com/fluxcd/pkg/blob/main/apis/event/v1beta1/metadata.go)
- [helm-controller #968 — appVersion implementáció](https://github.com/fluxcd/helm-controller/pull/968)