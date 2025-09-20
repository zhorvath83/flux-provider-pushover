.PHONY: build test clean run docker-build docker-run fmt vet lint bench

# Variables
BINARY_NAME=flux-provider-pushover
DOCKER_IMAGE=flux-provider-pushover
VERSION?=dev
BUILD_DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Go build flags for optimization
LDFLAGS=-ldflags "-w -s -X main.version=${VERSION} -X main.buildDate=${BUILD_DATE}"
GOFLAGS=-trimpath

# Build the binary
build:
	CGO_ENABLED=0 go build ${GOFLAGS} ${LDFLAGS} -o ${BINARY_NAME} .

# Run tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Generate LCOV coverage report using coverco tool
test-lcov: test
	@echo "Generating LCOV coverage report..."
	@mkdir -p coverage
	@export PATH=$(shell go env GOPATH)/bin:$$PATH && \
		which gcov2lcov > /dev/null || (echo "Installing gcov2lcov..." && go install github.com/jandelgado/gcov2lcov@latest)
	@export PATH=$(shell go env GOPATH)/bin:$$PATH && \
		which coverco > /dev/null || (echo "Installing coverco..." && go install github.com/mkabdelrahman/coverco@latest)
	@export PATH=$(shell go env GOPATH)/bin:$$PATH && \
		gcov2lcov -infile=coverage.out -outfile=coverage/lcov.info
	@echo "LCOV report generated: coverage/lcov.info"
	@echo ""
	@echo "Coverage summary:"
	@export PATH=$(shell go env GOPATH)/bin:$$PATH && \
		coverco -coverage-dir=coverage -coverage-reports-format=lcov -keep-reports=true .

# Run benchmarks
bench:
	go test -bench=. -benchmem -run=^$$

# Format code
fmt:
	gofmt -w .
	go mod tidy

# Vet code
vet:
	go vet ./...

# Run static analysis (requires staticcheck)
lint:
	@which staticcheck > /dev/null || (echo "Installing staticcheck..." && go install honnef.co/go/tools/cmd/staticcheck@latest)
	staticcheck ./...

# Clean build artifacts
clean:
	rm -f ${BINARY_NAME}
	rm -f coverage.out coverage.html
	rm -rf coverage/
	rm -rf dist/

# Run locally
run: build
	./${BINARY_NAME}

# Docker multi-arch build
docker-build:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=${VERSION} \
		--build-arg BUILD_DATE=${BUILD_DATE} \
		-t ${DOCKER_IMAGE}:${VERSION} \
		-t ${DOCKER_IMAGE}:latest \
		.

# Docker build and push
docker-push:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=${VERSION} \
		--build-arg BUILD_DATE=${BUILD_DATE} \
		-t ${DOCKER_IMAGE}:${VERSION} \
		-t ${DOCKER_IMAGE}:latest \
		--push \
		.

# Run with docker
docker-run:
	docker run --rm \
		-e PUSHOVER_USER_KEY=${PUSHOVER_USER_KEY} \
		-e PUSHOVER_API_TOKEN=${PUSHOVER_API_TOKEN} \
		-p 8080:8080 \
		${DOCKER_IMAGE}:latest

# Development with hot reload (requires air)
dev:
	@which air > /dev/null || (echo "Installing air..." && go install github.com/air-verse/air@latest)
	air

# Memory profiling
profile-mem:
	go test -memprofile=mem.prof -bench=.
	go tool pprof -http=:8081 mem.prof

# CPU profiling  
profile-cpu:
	go test -cpuprofile=cpu.prof -bench=.
	go tool pprof -http=:8082 cpu.prof

# Check for security vulnerabilities
security:
	@which gosec > /dev/null || (echo "Installing gosec..." && go install github.com/securego/gosec/v2/cmd/gosec@latest)
	gosec ./...

# Full check before commit
check: fmt vet test lint security
	@echo "✅ All checks passed!"

help:
	@echo "Available targets:"
	@echo "  build         - Build the binary"
	@echo "  test          - Run tests with race detection"
	@echo "  test-coverage - Run tests and generate coverage report"
	@echo "  test-lcov     - Run tests and generate LCOV report"
	@echo "  bench         - Run benchmarks"
	@echo "  fmt           - Format code"
	@echo "  vet           - Run go vet"
	@echo "  lint          - Run static analysis"
	@echo "  clean         - Remove build artifacts"
	@echo "  run           - Build and run locally"
	@echo "  docker-build  - Build multi-arch Docker image"
	@echo "  docker-push   - Build and push Docker image"
	@echo "  docker-run    - Run with Docker"
	@echo "  dev           - Run with hot reload"
	@echo "  profile-mem   - Run memory profiling"
	@echo "  profile-cpu   - Run CPU profiling"
	@echo "  security      - Check for security issues"
	@echo "  check         - Run all checks"
	@echo "  help          - Show this help"