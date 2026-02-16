BINARY   := reserve
VERSION  := v1.0.5
LDFLAGS  := -ldflags "-X github.com/derickschaefer/reserve/cmd.Version=$(VERSION) \
             -X github.com/derickschaefer/reserve/cmd.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)"
#GOFLAGS := -mod=vendor

# Benchmark settings
BENCH_COUNT    := 3
BENCH_FLAGS    := -benchmem -count=$(BENCH_COUNT)
BENCH_OUT_V1   := bench_v1.txt
BENCH_OUT_V2   := bench_v2exp.txt

.PHONY: build test test-all test-unit test-integration \
        test-analyze test-chart test-config test-pipeline test-store test-transform \
        test-cover bench bench-v2 bench-compare bench-parity bench-identity \
        bench-setup lint clean run install help

## ── Build ────────────────────────────────────────────────────────────────────

## build: compile the reserve binary
build:
	go build $(GOFLAGS) $(LDFLAGS) -o $(BINARY) .

## install: install to $$GOPATH/bin
install:
	go install $(GOFLAGS) $(LDFLAGS) .

## run: build and run with args  (usage: make run ARGS="series get GDP")
run: build
	./$(BINARY) $(ARGS)

## ── Unit Tests (per package) ─────────────────────────────────────────────────

## test-analyze: unit tests for internal/analyze (statistical summaries and trend fitting)
test-analyze:
	go test $(GOFLAGS) -v ./internal/analyze/...

## test-chart: unit tests for internal/chart (ASCII bar and sparkline rendering)
test-chart:
	go test $(GOFLAGS) -v ./internal/chart/...

## test-config: unit tests for internal/config (load, priority, validate, redact)
test-config:
	go test $(GOFLAGS) -v ./internal/config/...

## test-pipeline: unit tests for internal/pipeline (JSONL read/write and round-trip)
test-pipeline:
	go test $(GOFLAGS) -v ./internal/pipeline/...

## test-store: unit tests for internal/store (bbolt CRUD, keys, snapshots, isolation)
test-store:
	go test $(GOFLAGS) -v ./internal/store/...

## test-transform: unit tests for internal/transform (all transformation operators)
test-transform:
	go test $(GOFLAGS) -v ./internal/transform/...

## test-unit: run all internal package unit tests
test-unit:
	go test $(GOFLAGS) -v ./internal/...

## ── Integration Tests ────────────────────────────────────────────────────────

## test-integration: run integration tests (live checks skip if no API key configured)
test-integration:
	go test $(GOFLAGS) -v ./tests/

## test: alias for test-integration (default test target)
test: test-integration

## ── Holistic / Full Suite ────────────────────────────────────────────────────

## test-all: run every test across all packages (unit + integration)
test-all:
	go test $(GOFLAGS) -v ./...

## test-cover: run full suite with HTML coverage report
test-cover:
	go test $(GOFLAGS) -coverprofile=coverage.out ./internal/... ./tests/
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## ── Benchmarks ───────────────────────────────────────────────────────────────
## Fixtures must be fetched once before running benchmarks.
## Run: make bench-setup FRED_API_KEY=your_key

## bench-setup: fetch FRED fixtures for benchmarks (requires FRED_API_KEY)
bench-setup:
	@if [ -z "$(FRED_API_KEY)" ]; then \
		echo "Error: FRED_API_KEY is not set."; \
		echo "  Usage: make bench-setup FRED_API_KEY=your_key"; \
		exit 1; \
	fi
	cd tests/benchmarks && FRED_API_KEY=$(FRED_API_KEY) ./fetch_fixtures.sh

## bench: run v1 baseline benchmarks ($(BENCH_COUNT) iterations each)
bench:
	go test $(GOFLAGS) ./tests/benchmarks/... \
		-bench=. $(BENCH_FLAGS) | tee $(BENCH_OUT_V1)
	@echo "Results written to $(BENCH_OUT_V1)"

## bench-v2: run benchmarks with GOEXPERIMENT=jsonv2 engine ($(BENCH_COUNT) iterations each)
bench-v2:
	GOEXPERIMENT=jsonv2 go test $(GOFLAGS) ./tests/benchmarks/... \
		-bench=. $(BENCH_FLAGS) | tee $(BENCH_OUT_V2)
	@echo "Results written to $(BENCH_OUT_V2)"

## bench-compare: run v1 and v2 benchmarks then diff with benchstat
bench-compare: bench bench-v2
	@if ! command -v benchstat > /dev/null 2>&1; then \
		echo "benchstat not found. Install with:"; \
		echo "  go install golang.org/x/perf/cmd/benchstat@latest"; \
		exit 1; \
	fi
	benchstat $(BENCH_OUT_V1) $(BENCH_OUT_V2)

## bench-parity: run v1/v2 round-trip parity test (requires GOEXPERIMENT=jsonv2)
bench-parity:
	GOEXPERIMENT=jsonv2 go test $(GOFLAGS) ./tests/benchmarks/... \
		-run TestV1V2Parity -v

## bench-identity: run byte-identity comparison test (requires GOEXPERIMENT=jsonv2)
bench-identity:
	GOEXPERIMENT=jsonv2 go test $(GOFLAGS) ./tests/benchmarks/... \
		-run TestMarshalByteIdentity -v

## ── Quality ──────────────────────────────────────────────────────────────────

## lint: vet all packages
lint:
	go vet $(GOFLAGS) ./...

## ── Cleanup ──────────────────────────────────────────────────────────────────

## clean: remove build artifacts and benchmark output files
clean:
	rm -f $(BINARY) coverage.out coverage.html $(BENCH_OUT_V1) $(BENCH_OUT_V2)

## ── Help ─────────────────────────────────────────────────────────────────────

## help: show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
