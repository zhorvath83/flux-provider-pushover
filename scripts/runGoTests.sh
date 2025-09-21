#!/bin/bash

# runGoTests.sh - Go teszt és coverage script
# Flux Provider Pushover projekthez

set -euo pipefail

# CI környezet detektálás
IS_CI="${CI:-false}"
GITHUB_STEP_SUMMARY="${GITHUB_STEP_SUMMARY:-}"
GITHUB_ENV="${GITHUB_ENV:-}"

# Színes output (csak ha nem CI)
if [ "$IS_CI" = "true" ]; then
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
else
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
fi

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

# Coverage könyvtár létrehozása (csak lokálisan)
COVERAGE_DIR="coverage"
if [ "$IS_CI" != "true" ]; then
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
if go vet ./cmd/... ./internal/...; then
    echo -e "${GREEN}✓ Go vet ellenőrzés sikeres${NC}"
else
    echo -e "${RED}✗ Go vet hibákat talált${NC}"
    exit 1
fi
echo ""

# Go fmt ellenőrzés
echo -e "${BLUE}=== Go fmt ellenőrzés ===${NC}"
UNFORMATTED=$(gofmt -l cmd/ internal/ 2>/dev/null || gofmt -l . 2>/dev/null || true)
if [ -z "$UNFORMATTED" ]; then
    echo -e "${GREEN}✓ Minden fájl megfelelően formázott${NC}"
else
    echo -e "${YELLOW}Figyelem: Az alábbi fájlok nincsenek formázva:${NC}"
    echo "$UNFORMATTED"
    if [ "$IS_CI" = "true" ]; then
        echo -e "${RED}CI-ban a formázatlan kód hibát jelent!${NC}"
        gofmt -d cmd/ internal/ 2>/dev/null || gofmt -d . 2>/dev/null
        exit 1
    else
        echo -e "${YELLOW}Futtatd: gofmt -w .${NC}"
    fi
fi
echo ""

# Unit tesztek futtatása race detection-nel
echo -e "${BLUE}=== Unit tesztek futtatása ===${NC}"
echo -e "${BLUE}Race detection és verbose output engedélyezve${NC}"

# Tesztelendő csomagok meghatározása
TEST_PACKAGES="./cmd/... ./internal/..."
if [ ! -d "cmd" ] && [ ! -d "internal" ]; then
    TEST_PACKAGES="./..."
fi

if go test -v -race -timeout=30s $TEST_PACKAGES; then
    echo -e "${GREEN}✓ Minden teszt sikeres${NC}"
else
    echo -e "${RED}✗ Tesztek sikertelenek${NC}"
    exit 1
fi
echo ""

# Coverage elemzés
echo -e "${BLUE}=== Test Coverage elemzés ===${NC}"

# Coverage fájlok elérési útja
if [ "$IS_CI" = "true" ]; then
    COVERAGE_FILE="coverage.out"
else
    COVERAGE_FILE="$COVERAGE_DIR/coverage.out"
fi

echo -e "${BLUE}Coverage profile generálása...${NC}"
if go test -race -coverprofile="$COVERAGE_FILE" -covermode=atomic $TEST_PACKAGES; then
    echo -e "${GREEN}✓ Coverage profile létrehozva: $COVERAGE_FILE${NC}"
else
    echo -e "${RED}✗ Coverage profile generálása sikertelen${NC}"
    exit 1
fi

# Coverage százalék kiszámítása
COVERAGE_PERCENT=$(go tool cover -func="$COVERAGE_FILE" | grep "total:" | awk '{print $3}' | tr -d '%')
echo -e "${BLUE}Teljes coverage: ${COVERAGE_PERCENT}%${NC}"

# CI-specifikus funkciók
if [ "$IS_CI" = "true" ] && [ -n "$GITHUB_ENV" ]; then
    echo "COVERAGE=${COVERAGE_PERCENT}%" >> "$GITHUB_ENV"
fi

if [ "$IS_CI" = "true" ] && [ -n "$GITHUB_STEP_SUMMARY" ]; then
    echo "## 📊 Test Coverage: ${COVERAGE_PERCENT}%" >> "$GITHUB_STEP_SUMMARY"
    echo "" >> "$GITHUB_STEP_SUMMARY"
    echo "### Coverage Details" >> "$GITHUB_STEP_SUMMARY"
    echo '```' >> "$GITHUB_STEP_SUMMARY"
    go tool cover -func="$COVERAGE_FILE" >> "$GITHUB_STEP_SUMMARY"
    echo '```' >> "$GITHUB_STEP_SUMMARY"
    
    # Coverage badge színezés
    COVERAGE_NUM=$(echo "$COVERAGE_PERCENT" | cut -d. -f1)
    if [ "$COVERAGE_NUM" -ge 80 ]; then
        COLOR="brightgreen"
        BADGE="✅ Kiváló"
    elif [ "$COVERAGE_NUM" -ge 60 ]; then
        COLOR="yellow"
        BADGE="⚠️ Jó"
    else
        COLOR="red"
        BADGE="❌ Alacsony"
    fi
    echo "" >> "$GITHUB_STEP_SUMMARY"
    echo "**Coverage státusz: $BADGE ($COLOR)**" >> "$GITHUB_STEP_SUMMARY"
fi

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

# Először csak benchmark-only futtatás (ahogy a CI csinálja)
echo -e "${BLUE}Benchmark-only futtatás...${NC}"
BENCHMARK_RESULTS=$(go test -bench=. -benchmem -run=^$ $TEST_PACKAGES 2>&1 || echo "")

if echo "$BENCHMARK_RESULTS" | grep -q "Benchmark"; then
    echo "$BENCHMARK_RESULTS"
    echo -e "${GREEN}✓ Benchmark tesztek futtatva${NC}"
    
    # CI-ban a benchmark eredményeket is hozzáadjuk a summary-hoz
    if [ "$IS_CI" = "true" ] && [ -n "$GITHUB_STEP_SUMMARY" ]; then
        echo "" >> "$GITHUB_STEP_SUMMARY"
        echo "### 🚀 Benchmark Results" >> "$GITHUB_STEP_SUMMARY"
        echo '```' >> "$GITHUB_STEP_SUMMARY"
        echo "$BENCHMARK_RESULTS" | grep -E "^(Benchmark|ok|PASS)" >> "$GITHUB_STEP_SUMMARY"
        echo '```' >> "$GITHUB_STEP_SUMMARY"
    fi
else
    echo -e "${YELLOW}⚠ Nincsenek benchmark tesztek${NC}"
fi
echo ""

# Memória profilozás (ha benchmark létezik és nem CI)
if echo "$BENCHMARK_RESULTS" | grep -q "Benchmark" && [ "$IS_CI" != "true" ]; then
    echo -e "${BLUE}=== Memória Profil Generálás ===${NC}"
    MEMPROFILE="$COVERAGE_DIR/mem.prof"
    if go test -bench=. -memprofile="$MEMPROFILE" $TEST_PACKAGES >/dev/null 2>&1; then
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

# Generált fájlok listázása (csak lokálisan)
if [ "$IS_CI" != "true" ]; then
    echo ""
    echo -e "${BLUE}Generált fájlok ($COVERAGE_DIR/):${NC}"
    ls -la "$COVERAGE_DIR/" 2>/dev/null || echo -e "${YELLOW}Üres coverage könyvtár${NC}"
fi

echo ""
echo -e "${GREEN}=== TESZT FUTTATÁS KÉSZ ===${NC}"
