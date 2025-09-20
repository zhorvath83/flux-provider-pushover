#!/bin/bash

# runGoTests.sh - Go teszt és coverage script
# Flux Provider Pushover projekthez

set -euo pipefail

# Színes output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Projekt root meghatározása git-tel
if ! PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null); then
    echo -e "${RED}HIBA: Nem git repository vagy nincs git telepítve!${NC}"
    exit 1
fi

# Váltás a projekt root-ba
if [ "$(pwd)" != "$PROJECT_ROOT" ]; then
    echo -e "${YELLOW}Váltás a projekt root könyvtárba: $PROJECT_ROOT${NC}"
    cd "$PROJECT_ROOT" || exit 1
fi

echo -e "${BLUE}=== Flux Provider Pushover - Go Test Suite ===${NC}"
echo -e "${BLUE}Projekt root: $(pwd)${NC}"
echo ""

# Coverage könyvtár létrehozása
COVERAGE_DIR="coverage"
if [ ! -d "$COVERAGE_DIR" ]; then
    echo -e "${BLUE}Coverage könyvtár létrehozása: $COVERAGE_DIR${NC}"
    mkdir -p "$COVERAGE_DIR"
fi

# .gitignore frissítése
GITIGNORE_FILE=".gitignore"
GITIGNORE_ENTRY="coverage/"

if [ -f "$GITIGNORE_FILE" ]; then
    if ! grep -q "^${GITIGNORE_ENTRY}$" "$GITIGNORE_FILE"; then
        echo -e "${BLUE}Coverage könyvtár hozzáadása .gitignore-hoz${NC}"
        echo "$GITIGNORE_ENTRY" >> "$GITIGNORE_FILE"
    fi
else
    echo -e "${BLUE}.gitignore létrehozása és coverage könyvtár hozzáadása${NC}"
    echo "$GITIGNORE_ENTRY" > "$GITIGNORE_FILE"
fi

# Go verzió ellenőrzése
echo -e "${BLUE}Go verzió:${NC}"
go version
echo ""

# go.mod ellenőrzése
if [ ! -f "go.mod" ]; then
    echo -e "${RED}HIBA: go.mod nem található!${NC}"
    exit 1
fi

echo -e "${BLUE}Go modul információ:${NC}"
go list -m
echo ""

# Függőségek letöltése/frissítése
echo -e "${BLUE}=== Függőségek ellenőrzése ===${NC}"
go mod download
go mod tidy
echo -e "${GREEN}✓ Függőségek rendben${NC}"
echo ""

# Go vet ellenőrzés
echo -e "${BLUE}=== Go Vet ellenőrzés ===${NC}"
if go vet ./...; then
    echo -e "${GREEN}✓ Go vet ellenőrzés sikeres${NC}"
else
    echo -e "${RED}✗ Go vet hibákat talált${NC}"
    exit 1
fi
echo ""

# Go fmt ellenőrzés
echo -e "${BLUE}=== Go fmt ellenőrzés ===${NC}"
UNFORMATTED=$(gofmt -l . 2>/dev/null || true)
if [ -z "$UNFORMATTED" ]; then
    echo -e "${GREEN}✓ Minden fájl megfelelően formázott${NC}"
else
    echo -e "${YELLOW}Figyelem: Az alábbi fájlok nincsenek formázva:${NC}"
    echo "$UNFORMATTED"
    echo -e "${YELLOW}Futtatd: gofmt -w .${NC}"
fi
echo ""

# Unit tesztek futtatása race detection-nel
echo -e "${BLUE}=== Unit tesztek futtatása ===${NC}"
echo -e "${BLUE}Race detection és verbose output engedélyezve${NC}"

if go test -v -race -timeout=30s ./...; then
    echo -e "${GREEN}✓ Minden teszt sikeres${NC}"
else
    echo -e "${RED}✗ Tesztek sikertelenek${NC}"
    exit 1
fi
echo ""

# Coverage elemzés
echo -e "${BLUE}=== Test Coverage elemzés ===${NC}"

# Coverage fájlok elérési útja
COVERAGE_FILE="$COVERAGE_DIR/coverage.out"

echo -e "${BLUE}Coverage profile generálása...${NC}"
if go test -race -coverprofile="$COVERAGE_FILE" -covermode=atomic ./...; then
    echo -e "${GREEN}✓ Coverage profile létrehozva: $COVERAGE_FILE${NC}"
else
    echo -e "${RED}✗ Coverage profile generálása sikertelen${NC}"
    exit 1
fi

# Coverage százalék kiszámítása
COVERAGE_PERCENT=$(go tool cover -func="$COVERAGE_FILE" | grep "total:" | awk '{print $3}' | tr -d '%')
echo -e "${BLUE}Teljes coverage: ${COVERAGE_PERCENT}%${NC}"

# Coverage riport kategorizálása
if (( $(echo "$COVERAGE_PERCENT >= 90" | bc -l) )); then
    echo -e "${GREEN}✓ Kiváló coverage (≥90%)${NC}"
elif (( $(echo "$COVERAGE_PERCENT >= 80" | bc -l) )); then
    echo -e "${YELLOW}✓ Jó coverage (≥80%)${NC}"
elif (( $(echo "$COVERAGE_PERCENT >= 70" | bc -l) )); then
    echo -e "${YELLOW}⚠ Elfogadható coverage (≥70%)${NC}"
else
    echo -e "${RED}✗ Alacsony coverage (<70%)${NC}"
fi
echo ""

# Részletes coverage információk
echo -e "${BLUE}=== Részletes Coverage Információ ===${NC}"
go tool cover -func="$COVERAGE_FILE"
echo ""

# Benchmark tesztek (ha vannak)
echo -e "${BLUE}=== Benchmark tesztek ===${NC}"
BENCHMARK_RESULTS=$(go test -bench=. -benchmem ./... 2>/dev/null || echo "")
if [ -n "$BENCHMARK_RESULTS" ]; then
    echo "$BENCHMARK_RESULTS"
    echo -e "${GREEN}✓ Benchmark tesztek futtatva${NC}"
else
    echo -e "${YELLOW}⚠ Nincsenek benchmark tesztek${NC}"
fi
echo ""

# Memória profilozás (ha benchmark létezik)
if echo "$BENCHMARK_RESULTS" | grep -q "Benchmark"; then
    echo -e "${BLUE}=== Memória Profil Generálás ===${NC}"
    MEMPROFILE="$COVERAGE_DIR/mem.prof"
    if go test -bench=. -memprofile="$MEMPROFILE" ./... >/dev/null 2>&1; then
        echo -e "${GREEN}✓ Memória profil: $MEMPROFILE${NC}"
        echo -e "${BLUE}Elemzés: go tool pprof $MEMPROFILE${NC}"
    fi
fi

# Összefoglaló
echo -e "${BLUE}=== ÖSSZEFOGLALÓ ===${NC}"
echo -e "${GREEN}✓ Go vet ellenőrzés${NC}"
echo -e "${GREEN}✓ Unit tesztek (race detection)${NC}"
echo -e "${GREEN}✓ Coverage elemzés: ${COVERAGE_PERCENT}%${NC}"
echo -e "${GREEN}✓ Fájlok a $COVERAGE_DIR könyvtárban${NC}"

# Generált fájlok listázása
echo ""
echo -e "${BLUE}Generált fájlok ($COVERAGE_DIR/):${NC}"
ls -la "$COVERAGE_DIR/" 2>/dev/null || echo -e "${YELLOW}Üres coverage könyvtár${NC}"

echo ""
echo -e "${GREEN}=== TESZT FUTTATÁS KÉSZ ===${NC}"
