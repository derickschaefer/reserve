# TEST.md — Reserve Test Suite

This document describes the current test surface in the `reserve` project. It is the authoritative reference for understanding what is tested, how to run it, and what each test group is verifying.

---

## Table of Contents

1. [Overview](#overview)
2. [Running the Tests](#running-the-tests)
3. [Command Tests](#command-tests)
4. [Unit Tests](#unit-tests)
   - [internal/analyze](#internalanalyze)
   - [internal/chart](#internalchart)
   - [internal/config](#internalconfig)
   - [internal/pipeline](#internalpipeline)
   - [internal/store](#internalstore)
   - [internal/transform](#internaltransform)
5. [Integration Tests](#integration-tests)
   - [tests/core_test.go](#testscore_testgo)
   - [tests/cmd_test.go](#testscmd_testgo)
6. [Benchmarks](#benchmarks)
   - [Setup](#setup)
   - [bench_test.go — v1 baseline](#bench_testgo--v1-baseline)
   - [bench_v2_test.go — explicit v2 API and parity](#bench_v2_testgo--explicit-v2-api-and-parity)
   - [Running and comparing](#running-and-comparing)
   - [Key findings](#key-findings)
7. [Test Isolation Philosophy](#test-isolation-philosophy)

---

## Overview

The test suite is organized into four layers:

| Layer | Location | Count | Network required |
|---|---|---|---|
| Command tests | `cmd/` | targeted command/unit tests | Never |
| Unit tests | `internal/*/` | 167 tests | Never |
| Integration tests | `tests/` | ~50 checks | Optional (skips gracefully) |
| Benchmarks | `tests/benchmarks/` | 20 benchmarks, 2 tests | Never (fixtures pre-fetched) |

Command tests, unit tests, and benchmarks run fully offline. Integration tests that require a live FRED API key skip automatically when no credentials are present.

---

## Running the Tests

```bash
# Preferred Makefile entry points
make test              # unit + integration
make test-unit         # cmd + internal packages
make test-integration  # tests/
make test-all          # go test -v ./...
make test-cover        # coverage report (cmd + internal + tests)

# Command tests
go test ./cmd/...

# All unit tests
go test ./internal/...

# All unit tests, verbose
go test -v ./internal/...

# Integration tests (skips live checks if no API key configured)
go test -v ./tests/

# Specific integration group
go test -v -run TestFredAPIConnectivity ./tests/
go test -v -run TestPayloadIntegrity    ./tests/
go test -v -run TestAPIClientBehaviour  ./tests/

# Everything
go test ./...

# Benchmarks (see Benchmarks section for full instructions)
go test ./tests/benchmarks/... -bench=. -benchmem -count=10
```

The `Makefile` targets are the easiest way to run the suite consistently in this repo because they also set local `GOCACHE` and `GOMODCACHE` paths under the workspace. For day-to-day work, `make test` is the default entry point; use the raw `go test` commands when you want a narrower package selection or custom flags.

---

## Command Tests

### cmd/

Command tests cover CLI-specific behavior that does not belong in the lower-level packages.

Key areas currently covered:

- `cmd/onboard_test.go`
  - topic parsing defaults and `all` expansion
  - command guide generation
  - registry coverage against the real Cobra top-level command tree
  - required-field consistency for command onboarding JSON
  - `onboard export` bundle generation and JSON validity
- `cmd/obs_test.go`
  - `obs get --from` source selection
  - default source = `live`
  - rejection of unknown sources
  - planned-but-unconfigured source handling
  - cache reads not requiring an API key
  - cache-backed observation reads from the local store
- `cmd/helpers_test.go`
  - output writer selection
  - integer ID parsing helpers

---

## Unit Tests

### internal/analyze

**File:** `internal/analyze/analyze_test.go` — 31 tests

Covers the `Summarize` and `Trend` functions which produce statistical summaries and linear/Theil-Sen trend fits over observation slices.

**Summarize tests** verify basic counts, mean, standard deviation, min/max, median (even and odd count), percentiles (p25/p75), first/last values, total change, percent change from first to last, skewness, and correct exclusion of NaN values from all calculations. Edge cases cover empty input, all-NaN input, and single-value series.

**Trend tests** verify slope direction for linear upward and downward series, flat series detection, R² range validity (0–1), slope-per-year consistency, NaN exclusion from regression inputs, minimum observation threshold enforcement (returns error below 3 non-NaN points), all-NaN input error, series ID preservation through the result, method field preservation, and the Theil-Sen estimator's resistance to outliers compared to OLS. Cross-cutting tests verify that `Summarize` followed by `Trend` agree on direction and that non-NaN count from `Summarize` matches the count used by `Trend`.

---

### internal/chart

**File:** `internal/chart/chart_test.go` — 19 tests

Covers the `Bar` (ASCII bar chart) and `Plot` (ASCII sparkline) rendering functions.

**Bar tests** verify basic rendering with known values, correct behavior when all values are NaN (no bars rendered), single-observation input, NaN filtering within a mixed series, the maximum-bars limit (100 bars), negative value handling (bars extend left), flat series (zero range), density warning emission when observations exceed the bar limit, and date format selection (annual YYYY vs monthly YYYY-MM).

**Plot tests** verify basic sparkline rendering, line count matching the requested height, title override, all-NaN input (blank output), single-observation rendering, NaN gaps (blank cells in the sparkline), flat series (single row), width parameter enforcement, and x-axis label generation.

---

### internal/config

**File:** `internal/config/config_test.go` — 24 tests

Covers configuration loading, priority layering (flag > env > file > defaults), validation, API key redaction, file writing, and the template function.

**Load resolution tests** verify that defaults apply when no config file or environment variables are present, that all fields from a config file are applied correctly, that `ConfigPath` is recorded when a config file is found, that missing config file returns no error and leaves `ConfigPath` empty, and that an invalid timeout string in the config file is ignored in favor of the default.

**Priority layering tests** verify that `FRED_API_KEY` environment variable overrides the file value, that `RESERVE_DB_PATH` environment variable sets `DBPath`, that a CLI flag value overrides both env and file, and that an empty flag value does not override a file-supplied value.

**Validate tests** verify that `Validate` passes with an API key present, fails without one, and that the error message mentions "API key" explicitly.

**RedactedAPIKey tests** verify that normal-length keys are shown as `xx****xx` (first 2, stars, last 2), that keys of 4 characters or fewer return `"****"`, and that the plaintext key is never returned.

**WriteFile and Template tests** verify a full round-trip (write then unmarshal back), that the written file has permissions `0600`, that the output is valid JSON, and that `Template()` produces a struct with correct default values for all fields.

Test isolation: every test that touches the filesystem uses `t.TempDir()` and `os.Chdir` to avoid touching the real `config.json`. `t.Setenv` handles environment variable cleanup automatically.

---

### internal/pipeline

**File:** `internal/pipeline/pipeline_test.go` — 36 tests

Covers `ReadObservations` (JSONL decode from stdin) and `WriteJSONL` (JSONL encode to stdout), plus round-trip integrity.

**Read tests** verify basic float parsing, null JSON value becoming NaN, FRED-style `"."` string becoming NaN, empty string becoming NaN, series ID extraction from the first record's `series_id` field, empty series ID when the field is absent, date parsing to `time.Time`, `value_raw` string preservation, `value_raw` defaulting to `"."` when value is null, blank line skipping, comment line skipping (`#` prefix), empty input error, blank-only input error, invalid JSON error, invalid date error, unexpected non-numeric string value error, and a large input test with 10,000 observations confirming no memory or correctness issues.

**Write tests** verify basic float output, NaN written as JSON null, date formatted as `YYYY-MM-DDTHH:MM:SSZ`, `value_raw` field preservation, one JSON object per line (newline-delimited), and empty slice producing no output.

**Round-trip tests** verify that a slice written by `WriteJSONL` and read back by `ReadObservations` produces identical values, dates, and `value_raw` strings. Additional round-trip tests cover many observations (1,000 points) and series ID preservation through the cycle.

---

### internal/store

**File:** `internal/store/store_test.go`

Covers the bbolt database abstraction: opening, key construction, series metadata CRUD, observation CRUD, database statistics, bucket clearing, and test isolation.

**Open/Close tests** verify that `Open` creates a database file at the specified path, that parent directories are created automatically if they don't exist, and that `Close` is idempotent (second call does not panic).

**ObsKey tests** verify the composite cache key builder: minimal key (series ID only), full key with all optional fields, that empty optional fields are omitted from the key, that the same parameters always produce the same key (determinism), and that different series IDs produce different keys.

**SeriesMeta tests** verify put/get round-trip, not-found returning false, `FetchedAt` being stamped on put, overwrite replacing the previous value, `ListSeriesMeta` returning all stored entries, and empty list on a fresh database.

**Obs tests** verify put/get round-trip, not-found returning false, NaN surviving the bbolt round-trip (stored as null, read back as NaN), date preservation through marshal/unmarshal, overwrite behavior, and multiple independent keys for the same series coexisting without interference.

**ListObsKeys tests** verify listing all keys, filtering by series prefix, and empty results on a fresh database.

**Stats tests** verify zero counts on a fresh database and correct row counts after writes.

**ClearBucket/ClearAll tests** verify that `ClearBucket` empties the target bucket while leaving others intact, and that `ClearAll` empties every bucket.

**Isolation test** explicitly verifies that two test database instances opened in separate temp directories do not share any state.

Test isolation: every test uses a `testDB(t)` helper that creates an isolated bbolt database in `t.TempDir()`. The production database at `~/.reserve/reserve.db` is never touched.

---

### internal/transform

**File:** `internal/transform/transform_test.go` — 77 tests

Covers all transformation operators: `PctChange`, `Diff`, `Log`, `Index`, `Normalize`, `Resample`, `Filter`, `Roll`, and composition.

**PctChange tests** cover period-1 and period-12 percentage change, NaN propagation when the denominator observation is NaN, zero denominator producing NaN, invalid period error, too-few-observations error, output length matching input length (with leading NaN padding), and date preservation.

**Diff tests** cover first and second order differences, NaN propagation, invalid order error, and too-few-observations error.

**Log tests** cover positive value transformation (verifying `ln(x)` correctness), non-positive values producing NaN with a warning appended, NaN passthrough, and output length.

**Index tests** cover basic indexing to 100 at the anchor date, anchor value becoming exactly 100, missing anchor date error, zero anchor value error (division by zero), NaN anchor value error, and NaN preservation at non-anchor positions.

**Normalize tests** cover z-score normalization (mean ≈ 0, std ≈ 1), min-max normalization (range 0–1), flat series z-score (all NaN, since std = 0), flat series min-max (all NaN, since range = 0), all-NaN input, NaN preservation for individual points, and unknown method error.

**Resample tests** cover monthly-to-annual with mean, last, and sum aggregation methods; monthly-to-quarterly; multi-year resampling; NaN values being skipped in aggregation; empty input error; unknown method error; and output dates being set to period-start (first of month/quarter/year).

**Filter tests** cover after-date filtering, before-date filtering, combined date range, minimum value threshold, maximum value threshold, drop-missing removing NaN observations, NaN retention when drop-missing is false, no-options pass-through, and all-excluded empty result.

**Roll tests** cover rolling mean (verifying window arithmetic), partial window behavior, min-periods threshold (NaN below threshold), rolling std, rolling min, rolling max, rolling sum, NaN skipping within a window, NaN window falling below min-periods, invalid window size error, min-periods exceeding window error, unknown stat error, length preservation, and date preservation.

**Composition tests** verify `PctChange` chained with `Roll` (12-month pct change then 3-period rolling mean) and `Resample` chained with `Diff` (monthly-to-quarterly then first difference).

---

## Integration Tests

Integration tests live in the `tests/` package and import internal packages directly. They are organized into named groups and produce readable pass/fail summaries. Groups that require live credentials skip automatically with a descriptive message.

### tests/core_test.go

Three test groups covering API connectivity, payload parsing, and HTTP client behavior.

**TestFredAPIConnectivity** — requires `config.json` with a valid API key; skips otherwise. Verifies DNS resolution of `api.stlouisfed.org`, `GetSeries` returning metadata without error, series ID and title being non-empty in the response, `GetObservations` returning a non-empty array, the first observation carrying a numeric (non-NaN) value, and observation dates matching `YYYY-MM-DD` format.

**TestPayloadIntegrity** — fully offline, never skips. Verifies `ParseObsValue` for numeric strings (`"305.109"`, `"0"`, `"-1.5"`), FRED missing-value sentinel (`"."`), empty string, and whitespace-padded sentinel — all six producing the correct `float64` or `NaN`. Verifies `FormatValue(NaN)` renders as `"."`. Verifies decimal display rules (whole numbers show one decimal place). Verifies config layering (`config.json` values load, env overrides file, flag overrides env) inline using sub-tests. Verifies the rate limiter allows requests at a high limit and blocks under context cancellation at a very low limit.

**TestAPIClientBehaviour** — fully offline, uses an injected mock HTTP client. Verifies `GetSeries` parses series ID and title from a mock response, propagates API error messages correctly, `GetObservations` parses numeric values and FRED `"."` sentinel as NaN, preserves `ValueRaw`, forwards `observation_start` and `observation_end` query parameters correctly, retries on HTTP 503 (succeeds on the third attempt after two transient failures), and `SearchSeries` sends the correct `search_text` parameter and returns parsed results.

---

### tests/cmd_test.go

Five integration groups covering durable CLI contracts and offline behavior around the command layer.

**TestCommandSurface** — verifies the shipped command surface through real `--help` output. Confirms expected top-level commands are present, deprecated `store` is absent, `obs get` documents `--from live|cache`, `onboard` documents command-specific and topic-based usage, and `series` advertises the supported current verbs.

**TestNewAPIEndpoints** — uses a mock HTTP client to verify `GetCategory`, `GetRelease`, `GetSource`, and `GetTag` client methods: correct endpoint paths, parameter forwarding, and response parsing.

**TestBatchConcurrency** — verifies the worker pool respects the `--concurrency` ceiling using an atomic counter to track peak simultaneous in-flight requests.

**TestPartialFailureWarnings** — verifies per-item batch failures are collected as warnings rather than aborting the whole batch.

**TestValueSemanticsOffline** — verifies offline rendering and storage semantics such as numeric preservation, null/NaN handling, CSV output for missing values, and exact cache key lookup behavior.

Command-level unit coverage in `cmd/` also includes `cmd/category_test.go`, which verifies `parseCategoryID` input handling and `walkCategoryTree` depth limiting against a mock FRED hierarchy.

---

## Benchmarks

**Location:** `tests/benchmarks/`

The benchmark suite measures `encoding/json` v1 performance against Go 1.25's experimental `encoding/json/v2` on real FRED API payloads. Fixtures are pre-fetched FRED JSON responses committed to `tests/benchmarks/fixtures/` — no network access is required at benchmark time.

### Setup

Fetch fixtures once (requires a FRED API key):

```bash
cd tests/benchmarks
chmod +x fetch_fixtures.sh
FRED_API_KEY=your_key ./fetch_fixtures.sh
```

The script fetches observation data for GDP, CPIAUCSL, UNRATE, and FEDFUNDS, plus series metadata for 10 well-known series. Commit the resulting JSON files — benchmarks run offline forever after.

```bash
git add tests/benchmarks/fixtures/
git commit -m "bench: add FRED JSON fixtures for json v1/v2 benchmarks"
```

Install `benchstat` for comparison output:

```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

---

### bench_test.go — v1 baseline

**Build tag:** none — always compiles. Uses `encoding/json` directly.

Running with `GOEXPERIMENT=jsonv2` transparently replaces the `encoding/json` internals with the v2 engine without any code change, making this file the primary vehicle for the transparent upgrade comparison.

**Shared infrastructure** (used by both test files):

- `loadFixture` — reads a named JSON file from `fixtures/`, skips the test if not present
- `toSeriesData` — converts raw `fredObsResponse` to `model.SeriesData`, parsing dates and mapping FRED `"."` to `math.NaN()`
- `loadObsFixture` — loads and parses an observation fixture; logs observation count only in regular tests, not per benchmark iteration
- `loadMetaFixtures` — loads all 10 series metadata fixtures into a `[]model.SeriesMeta` slice
- `safeObsRow` / `safeSeriesData` — JSON-safe envelope types using `*float64` (nil = missing) to avoid `encoding/json`'s refusal to marshal `math.NaN()`; mirrors the representation used by `store.PutObs` in production
- `toSafeSeriesData` — converts `model.SeriesData` → `safeSeriesData`, mapping NaN to nil pointer

**Group 1 — UnmarshalRawObs** (3 benchmarks: GDP, CPIAUCSL, UNRATE)

Decodes the raw FRED `/series/observations` HTTP response into `fredObsResponse`. Represents the API client hot path — what happens every time a new fetch is made. Uses `b.SetBytes` to report throughput in MB/s.

**Group 2 — MarshalSeriesData** (2 benchmarks: GDP, CPIAUCSL)

Marshals `safeSeriesData` to JSON bytes. Represents `store.PutObs` — the bbolt write path. Uses `*float64` to correctly handle missing values. Reports allocations with `b.ReportAllocs()`.

**Group 3 — UnmarshalSeriesData** (2 benchmarks: CPIAUCSL, UNRATE)

Unmarshals JSON bytes into `safeSeriesData`. Represents `store.GetObs` — the bbolt read path. Setup marshals the fixture data once before the timer starts.

**Group 4 — JSONLRoundTrip** (3 benchmarks: GDP, CPIAUCSL, UNRATE)

Calls `pipeline.WriteJSONL` then `pipeline.ReadObservations` in a single iteration. Represents the hot path for every pipeline command (transform, analyze, filter). Because `pipeline` uses `encoding/json` internally, `GOEXPERIMENT=jsonv2` upgrades this path transparently with zero code change.

**Group 5 — MetaBatch** (2 benchmarks: Marshal, Unmarshal)

Marshals and unmarshals a `[]model.SeriesMeta` slice of 10 series. Represents `store.ListSeriesMeta` and batch metadata writes. String-heavy payload with HTML content in the `Notes` field — which is why this group produces a byte-identity difference between v1 and v2 (see below).

---

### bench_v2_test.go — explicit v2 API and parity

**Build tag:** `//go:build goexperiment.jsonv2` — only compiles when `GOEXPERIMENT=jsonv2` is set.

Contains explicit `jsonv2.Marshal` / `jsonv2.Unmarshal` benchmarks (prefixed `BenchmarkV2`) and two correctness tests.

**Explicit v2 benchmarks** mirror Groups 1–3 and 5 from `bench_test.go` but call the `encoding/json/v2` API directly. This isolates the explicit v2 API performance from the transparent GOEXPERIMENT upgrade, allowing a three-way comparison: v1 baseline, v2 engine via GOEXPERIMENT, and v2 explicit API.

**TestMarshalByteIdentity** — compares raw bytes produced by `json.Marshal` and `jsonv2.Marshal` for each fixture, finding the exact byte position and context of the first divergence if any. Reports ✓ (identical) or ✗ (differs, with a 40-byte context window around the first difference). Covers all three observation series and the metadata batch.

**TestV1V2Parity** — verifies round-trip correctness. Each fixture is marshaled by v1, unmarshaled by v1, marshaled by v2, and unmarshaled by v2. Observation counts and values are compared element-by-element (handling nil pointers for missing values). Also cross-decodes: v1 output decoded by v2, and v2 output decoded by v1, confirming format compatibility.

---

### Running and comparing

```bash
# v1 baseline
go test ./tests/benchmarks/... -bench=. -benchmem -count=10 | tee v1.txt

# v2 engine via GOEXPERIMENT (transparent upgrade, same code)
GOEXPERIMENT=jsonv2 go test ./tests/benchmarks/... -bench=. -benchmem -count=10 | tee v2exp.txt

# parity and byte identity tests
GOEXPERIMENT=jsonv2 go test ./tests/benchmarks/... -run "TestV1V2Parity|TestMarshalByteIdentity" -v

# compare
~/go/bin/benchstat v1.txt v2exp.txt
```

---

### Key findings

Benchmarks run on real FRED data (GDP ~300 obs, CPIAUCSL ~950 obs, UNRATE ~937 obs) on a DigitalOcean instance. Results as of February 2026 with Go 1.25.

**Unmarshal: significant wins across the board**

| Benchmark | Improvement | Allocation reduction |
|---|---|---|
| UnmarshalRawObs (all series) | ~47% faster | 55–67% fewer allocs |
| UnmarshalSeriesData (store read) | ~40% faster | 41–54% fewer allocs |
| UnmarshalMetaBatch | ~51% faster | 62% fewer allocs |

**Marshal: regression on the write path**

| Benchmark | Change |
|---|---|
| MarshalSeriesData (store write) | ~47% slower |
| JSONLRoundTrip (pipeline) | 17–26% slower |
| MarshalMetaBatch | no significant change |

Marshal allocations remain at 1 across v1 and v2 — the regression is not GC pressure.

**Byte identity results (TestMarshalByteIdentity)**

| Payload | Result |
|---|---|
| GDP observation data | ✓ Byte-for-byte identical |
| CPIAUCSL observation data | ✓ Byte-for-byte identical |
| UNRATE observation data | ✓ Byte-for-byte identical |
| SeriesMeta batch (HTML in Notes field) | ✗ Differs — v2 is 45 bytes smaller |

The metadata divergence is v2 emitting raw UTF-8 (`</p>`) where v1 HTML-escapes to `\u003c/p\u003e`. V2's behavior is correct for API/CLI contexts; v1's default HTML escaping is a well-known footgun for non-browser use cases.

The observation marshal regression is a pure performance regression: v2 produces byte-identical output but takes 47% longer to produce it.

---

## Test Isolation Philosophy

Every test that touches state follows these invariants:

**Database tests** use `testDB(t)` which creates a bbolt database in `t.TempDir()` and registers `t.Cleanup(db.Close)`. The production database at `~/.reserve/reserve.db` is never opened during testing.

**Config tests** use `t.TempDir()` with `os.Chdir` to control which directory `config.Load()` searches, and `t.Setenv` for environment variables (both cleaned up automatically when the test ends).

**HTTP client tests** use `httptest.NewServer` with `defer srv.Close()`. No real network requests are made.

**Benchmark fixtures** are static files committed to the repository. `loadFixture` skips the test if a fixture is missing rather than fetching live data.

No test shares mutable state with any other test. Tests can be run in any order, in parallel within a package, or individually without affecting results.
