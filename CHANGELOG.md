# Changelog

All notable changes to `reserve` are documented here.

The project uses **[Semantic Versioning](https://semver.org/)**. `v1.1.7` is the
current release, and `v1.0.5` was the first publicly tagged release. Prior versions are documented under
[Development History](#development-history) for auditability.

---

### v1.1.7 — 2026-05-22 — Snippet Library Foundation (Soft Launch), Deterministic Batch Concurrency Tests, and ASCII Chart Precision

**Added**

- New `snippet` command family for reusable local command chains:
  - `reserve snippet set <NAME> --cmd "..." [--desc "..."]`
  - `reserve snippet list`
  - `reserve snippet get <NAME>`
  - `reserve snippet run <NAME>`
  - `reserve snippet delete <NAME>` (`rm`/`remove` aliases)
- Filesystem-backed snippet library infrastructure for local and shared command catalogs
- Snippet command onboarding guide entry and examples in onboard command metadata
- New integration `Snippet Contracts` section in `tests/cmd_test.go` covering snippet help surface and core CLI contracts

**Changed**

- Snippet `set` now supports `--desc` so list output can serve as a compact pipeline catalog
- Snippet list output now prioritizes `NAME` + `DESCRIPTION` (with command preview fallback when description is missing)
- `batchGetObs` and `batchGetSeries` now have deterministic `testing/synctest` coverage for:
  - concurrency ceilings
  - ordering guarantees
  - warning aggregation
  - fallback concurrency behavior
- `TestBatchConcurrency` integration test no longer relies on wall-clock `time.Sleep`; it now uses deterministic request gating

**Fixed**

- Horizontal ASCII `chart bar` value labels now render as fixed two-decimal values (e.g. `59.00`) to keep numeric columns visually aligned
- `transform` pipeline commands now preserve citation metadata from upstream JSONL input so source attribution is retained in transformed outputs (including resample workflows)

---

### v1.1.6 — 2026-05-14 — Release Source Arrays, Agent Onboarding Metadata, and Per-Series Citations

**Added**

- `release get --format json` and `meta release --format json` now include full associated release source arrays under `sources`
- `SeriesMeta` now carries `source_names` for multi-source citation-aware workflows
- Onboarding JSON now includes explicit audience metadata:
  - `primary_audience: "AI agents and LLMs interfacing with reserve CLI for economic data workflows."`
  - `intended_for_humans: false`
  - `content_type: "agent_onboarding"`

**Changed**

- `reserve obs latest` and multi-series table-mode `reserve obs get` now render citations as a per-series footer block:
  - header: `Sources by series:`
  - one line per series in table order: `- <SERIES_ID>: <source(s)> via FRED`
- Single-series citation behavior remains concise:
  - one source: `Source: ... via FRED`
  - multiple sources: `Sources: ... via FRED`
- Citation-source display names are now normalized for readability when upstream source tags are entirely lowercase (display-only normalization; raw source tags are preserved)

**Fixed**

- Removed nested/awkward combined footer strings such as `Sources: ...; Sources: ...` in mixed-source multi-series outputs
- `release` metadata hydration now pulls `release/sources` so associated institutions are no longer dropped from JSON output

---

### v1.1.5 — 2026-05-10 — Alias Notes, Obs Latest Citation Consolidation, and Alias Reliability

**Added**

- `reserve alias set <ALIAS> <SERIES_ID> --note "..."` for optional user-authored alias notes
- Alias table/list output now includes a `NOTE` column
- `reserve alias get` now prints the stored note when present
- Integration coverage group `TestAliasContracts` with styled output banners/check summaries

**Changed**

- Go toolchain baseline updated to `go 1.26.3` in `go.mod` (from `1.26.1`)
- `series_aliases` config shape now stores structured alias entries:
  - `series_id` (required)
  - `note` (optional)
- `reserve obs latest` table-output citation footer now consolidates multi-series sources into one compact line:
  - Single source: `Source: <provider> via FRED`
  - Multi-source: `Sources: <provider A> via FRED; <provider B> via FRED`
- `reserve alias delete` now removes aliases from whichever config file actually owns the alias (local or user), avoiding merged-config visibility/delete mismatches
- Reserved alias name detection now derives from the live root command tree (instead of a stale hardcoded list)

**Fixed**

- Alias collision checks now fail closed on transient upstream errors (timeout/502/503/504/DNS), preventing accidental alias-to-real-series collisions during outages
- Alias parsing now tolerates legacy string-form `series_aliases` entries in existing configs while writing canonical structured entries
- Added coverage for `obs latest` citation footer consolidation:
  - multi-source output uses one deduplicated `Sources:` line
  - single-source output keeps the `Source:` label
- `reserve obs get --format json` now encodes missing observation values (`NaN`) as JSON `null` instead of failing with `json: unsupported value: NaN`

---

### v1.1.4 — 2026-04-29 — Multi-Series Summary Analysis and Sharper LLM Onboarding

**Added**

- New `reserve analyze summary --by-series` mode to summarize a multi-series JSONL stream by `series_id` in one command
- Grouped JSONL observation reading support in the pipeline layer for multi-series summary workflows

**Changed**

- Bare `reserve onboard` now emits a concise routing brief instead of a full command-library dump
- Onboarding content now promotes batched `reserve obs get <ID...>` usage much more explicitly for shared-window comparisons
- `obs`, `analyze`, pipeline examples, and gotchas now teach the preferred multi-series workflow: batched `obs get` plus `analyze summary --by-series`
- Topic-oriented onboarding was trimmed to avoid duplicating the command catalog in the base program payload

---

### v1.1.3 — 2026-04-24 — Self-Update, Cache Visibility, and Release Plumbing

**Added**

- New `reserve update apply` command for Phase 1 self-updates on macOS and Linux
- `reserve update apply --dry-run` for end-to-end validation of the live release path without replacing the installed binary
- `reserve update apply --force` to exercise the update path even when the current version already matches the latest manifest version
- New `reserve cache path` command to print the active local database path

**Changed**

- Windows update flow now resolves and prints the exact release asset URL for manual download instead of attempting in-place self-update
- `reserve cache stats` now reports schema version and on-disk database size alongside per-bucket row and byte counts

---

### v1.1.2 — 2026-04-11 — Config Discovery, Update Checks, and Release Build Hygiene

**Changed**

- Release builds now strip symbol and DWARF debug information (`-s -w`) starting in `v1.1.1`, reducing distributed binary size without changing runtime behavior
- `Makefile` now separates development builds (`make build`) from stripped production builds (`make build-release`)
- Config discovery now supports per-user config locations on Linux, macOS, and Windows
- Local `./config.json` now overrides the per-user config file when both are present
- `reserve config init` now creates the user config file and parent directories automatically
- `reserve config set` now updates a local override when present, otherwise the user config file

**Added**

- Unit coverage for user-config discovery, local-over-user precedence, and config directory creation
- New `reserve update check` command for remote version checks, release highlights, and static update instructions via a lightweight manifest
- Checked-in `release-manifest.json` plus Cloudflare distribution publishing for `release.json`

---

### v1.0.6 — 2026-02-28 — Maintenance, Value Semantics, and Keyless Signing

**Changed**

- Go toolchain baseline updated to `go 1.25.7` in `go.mod` (from `1.25.5`)
- `Makefile` test flow updated: `make test` now runs unit (`./cmd`, `./internal/...`) plus integration (`./tests/`)
- `Makefile` test/lint/bench targets now use local `GOCACHE=$(CURDIR)/.gocache` for stable runs in sandboxed/restricted environments
- `make test` output styling improved with phase banners and final suite summary

**Fixed**

- `obs latest` non-table output now preserves numeric value payloads (no zero-value regression in JSON/JSONL/CSV)
- Store key lookup boundary handling now enforces exact series matching (`GDP` no longer collides with `GDPDEF`)
- Snapshot ID generation now produces ULID-format IDs consistently
- `fetch` command invocation state now avoids cross-run flag leakage
- `llm` default topic aligned to `toc`
- `parseIntID` messaging aligned with accepted non-negative integer semantics

**Added**

- New command-level unit tests (`cmd/*_test.go`) for:
  - output writer behavior
  - LLM topic parsing defaults
  - `obs latest` value round-trip semantics
  - snapshot ULID format/uniqueness/sortability
- New integration test group `TestValueSemanticsOffline` with styled output and explicit value-fidelity assertions across JSON/JSONL/CSV and store key boundaries
- Keyless release-signing workflow at `.github/workflows/release-keyless.yml`:
  - builds cross-platform release archives on `v*` tags
  - publishes `SHA256SUMS` + `SHA256SUMS.sig` + `SHA256SUMS.pem`
  - signs checksums using Sigstore/cosign keyless OIDC identity
- Release verification documentation added at `docs/release-security.md`

---

### v1.0.5 — 2026-02-16 — LLM Onboarding, Version Command, Test Suites

**Added**

README File:
- `README.md` rewritten as full production documentation: install, quick start,
  design philosophy, complete command reference with examples, pipeline usage
  guide, output formats, global flags, and configuration reference
- `CHANGELOG.md` introduced with full development history
- "Why reserve?" positioning section covering cross-platform portability,
  command-object model, embedded database, pipeline semantics, LLM/agentic
  workflow readiness, and Go idioms
- Table of contents with anchor links throughout README

`reserve llm` (`cmd/llm.go`):
- Machine-readable context document for LLM onboarding
- `--topic toc` (default) — table of contents and interaction guide; the handshake
- `--topic commands` — full command reference: all nouns, verbs, flags, output formats
- `--topic pipeline` — stdin/stdout semantics, JSONL format, operator chaining
- `--topic data-model` — core types, NaN conventions, Result envelope, JSONL row schema
- `--topic examples` — verified end-to-end examples with confirmed FRED output values
- `--topic gotchas` — sharp edges: format flag requirement, window vs transform, multi-series limitation, vintage revisions, government shutdown gap
- `--topic version` — build metadata for provenance and reproducibility
- `--topic all` — full document for large context windows
- Comma-separated multi-topic: `reserve llm --topic pipeline,gotchas,examples`
- `--format jsonl` for single-line audit stream output
- `SetEscapeHTML(false)` on all encoders — clean `<>` output, no `\u003c` escaping
- Bare `reserve llm` defaults to `--topic toc` (handshake, not firehose)

`reserve version` (`cmd/version.go`):
- Plain text output (default): version, Go version, OS/arch, build time
- `--format json` — structured output for tooling
- `--format jsonl` — single-line output for audit streams and pipeline provenance
- `BuildTime` variable injected at build time via `-ldflags`

Makefile (`Makefile`):
- `VERSION := v1.0.5` — single source of truth for version string
- `LDFLAGS` — injects `Version` and `BuildTime` into binary at build time
- `make build` and `make install` both stamp the binary automatically

Test suites (`internal/*/suite_test.go`):
- `TestSuiteAnalyze` — 30 tests grouped under visual banner with ✅/❌ per sub-test
- `TestSuiteChart` — 19 tests
- `TestSuiteConfig` — 20 tests
- `TestSuitePipeline` — 26 tests
- `TestSuiteStore` — 34 tests
- `TestSuiteTransform` — 64 tests
- All suites use `t.Run` return value for pass/fail tracking — no logic duplication
- Individual `TestXxx` functions remain runnable in isolation via `-run`

Shared test helper (`internal/testutil/testutil.go`):
- `Result` struct with `Pass`, `Fail`, `Check`, `Summary` methods
- `Banner` function — consistent separator/emoji style matching integration tests
- `CheckPass`, `CheckFail`, `Divider`, `Separator` constants
- Single import across all six suite files: `tu "github.com/derickschaefer/reserve/internal/testutil"`

Test file renames (`tests/`):
- `reserve_test.go` → `core_test.go` — core integration tests
- `phase2_test.go` → `cmd_test.go` — command routing and concurrency tests

**Changed**

`cmd/root.go`:
- Quick start section now includes `reserve version` and `reserve llm`
- `--out` help text now says `<filename>` instead of `file` for clarity
- Removed unused `--pager` flag

`README.md` late adds:
- Added `version` and `llm` to Table of Contents
- Added `### version` command reference section
- Added `### llm` command reference section with full topic listing and workflow
- Added LLM-Assisted Analysis section to Pipeline Usage

`TEST.md`:
- Updated integration test file references from old names to `core_test.go` and `cmd_test.go`

---

## Development History

The following versions were development iterations. They were never tagged in
Git and carry no GitHub Release artifacts. They are recorded here for
transparency and to document the build-up of functionality.

---

### v1.0.4 — 2026-02-13 — Transform and Analysis Pipeline

Phase 4: stdin/stdout pipeline operators, rolling windows, statistical analysis.

**Added**

`transform` commands (`internal/transform/`, `cmd/transform.go`):
- `transform pct-change [--period N]` — period-over-period % change; use `--period 12` for YoY on monthly data
- `transform diff [--order 1|2]` — first or second difference
- `transform log` — natural log; non-positive values become NaN with a warning
- `transform index --base N --at YYYY-MM-DD` — re-scale to base at anchor date
- `transform normalize [--method zscore|minmax]` — z-score or 0–1 min-max scaling
- `transform resample --freq monthly|quarterly|annual --method mean|last|sum` — temporal downsampling
- `transform filter [--after] [--before] [--min] [--max] [--drop-missing]` — date-range and value-bound filtering

`window` command:
- `window roll --stat mean|std|min|max|sum --window N [--min-periods M]` — rolling window statistics; NaN-aware

`analyze` commands (`internal/analyze/`, `cmd/analyze.go`):
- `analyze summary` — count, missing%, mean, std, min/p25/median/p75/max, skew, first/last, total change
- `analyze trend [--method linear|theil-sen]` — direction, slope per year, intercept, R²

Pipeline I/O (`internal/pipeline/`):
- `ReadObservations` — parse JSONL from stdin; auto-detect series ID
- `WriteJSONL` — emit observations as JSONL
- `IsTTY` — auto-select table vs. jsonl based on terminal detection

**Notes**
- All operators are purely stdin/stdout; no database access required
- NaN values propagate correctly through all transform and analyze operators
- `store get --format jsonl` is the primary pipeline data source

---

### v1.0.3 — 2026-02-12 — bbolt Persistence, Cache, and Snapshots

Phase 3: local database, TTL-aware caching, snapshot system.

**Added**

Store layer (`internal/store/`):
- bbolt-backed storage for series metadata and observations
- Buckets: `meta_series`, `obs_cache`, `query_cache`, `snapshots`, `config`
- Observation cache keys encode series ID, start, end, frequency, units, aggregation
- DB schema versioning with migration runner on startup
- Custom JSON serialization handling IEEE 754 NaN as JSON `null`

`fetch` command: `fetch series <SERIES_ID...> --store` — batch fetch with concurrent workers, write to bbolt

`store` command:
- `store list` — all accumulated series with fetch timestamps and observation set counts
- `store get <SERIES_ID>` — read stored observations in any output format

`cache` command:
- `cache stats` — bucket row counts and DB file size
- `cache clear [--all | --bucket <n>]` — selective or full data wipe

`snapshot` command:
- `snapshot save --name <n> --cmd "<cmd>"` — persist a command line with a ULID key
- `snapshot list`, `snapshot show <ID>`, `snapshot run <ID>`, `snapshot delete <ID>`

**Changed**
- `app.Deps` extended with `Store *store.Store`
- Config resolution extended: `RESERVE_DB_PATH` env var and `db_path` config key

---

### v1.0.2 — 2026-02-11 — Full Subcommand Tree

Phase 2: all discovery and retrieval commands wired end-to-end.

**Added**

New API endpoints (`internal/fred/`):
- `GetCategory`, `GetCategoryChildren`, `GetCategorySeries`
- `GetSeriesTags`, `GetSeriesCategories`
- `ListReleases`, `GetRelease`, `GetReleaseDates`, `GetReleaseSeries`
- `ListSources`, `GetSource`, `GetSourceReleases`
- `SearchTags`, `GetTagSeries`, `GetRelatedTags`

New commands:
- `category get|list|tree|series` — browse the FRED category hierarchy
- `release list|get|dates|series` — scheduled data releases
- `source list|get|releases` — data source institutions
- `tag search|series|related` — tag exploration
- `search "<query>" [--type ...]` — global full-text search
- `meta series|category|release|tag|source` — batch metadata retrieval
- `fetch series|category|query` — batch convenience fetch with worker pool
- `config init|get|set` — configuration file management
- `completion` — shell completion for bash, zsh, fish via Cobra

**Changed**
- Batch commands use a bounded goroutine pool (size = `--concurrency`) sharing a single rate limiter
- Output sorted deterministically by `series_id` then `date`
- Partial batch failures collected as `Warnings` in the `Result` envelope

---

### v1.0.1 — 2026-02-10 — Scaffold, Config, API Client, Core Commands

Phase 1: working binary that hits the live FRED API.

**Added**

Project structure:
- `go.mod` as `github.com/derickschaefer/reserve`
- Directory layout: `cmd/`, `internal/`, `tests/`
- `Makefile` with `build`, `test`, `lint`, `run`, `install` targets
- `.gitignore` for binaries, `config.json`, `*.db`

Config loader (`internal/config/`):
- Resolution chain: `--api-key` flag → `FRED_API_KEY` env → `config.json`
- Fail-fast with actionable error if API key is absent
- `RedactedAPIKey()` for safe logging
- `config init`, `config get [--show-secrets]`, `config set`

Core types (`internal/model/`):
- `Category`, `SeriesMeta`, `Release`, `Source`, `Tag`
- `Observation` — `Value float64` (NaN for missing), `ValueRaw string`
- `SeriesData`, `Result`, `ResultStats`

API client (`internal/fred/`):
- Context-aware HTTP client; `api_key` and `file_type=json` on every request
- Exponential backoff retry on HTTP 429 / 5xx (max 4 attempts)
- Client-side token-bucket rate limiter (`golang.org/x/time/rate`)
- `--debug` logs request URLs with API key redacted
- Endpoints: `GetSeries`, `SearchSeries`, `GetObservations`, `GetLatestObservation`

Commands:
- `series get <SERIES_ID...>`, `series search`, `series tags`, `series categories`
- `obs get <SERIES_ID...>` with `--start`, `--end`, `--freq`, `--units`, `--agg`, `--limit`
- `obs latest <SERIES_ID...>`

Renderer (`internal/render/`):
- `table`, `json`, `jsonl`, `csv`, `tsv`, `md`
- `PrintFooter` — warnings and timing stats in `--verbose` mode

Tests (`tests/reserve_test.go`):
- Group 1: live FRED API connectivity
- Group 2: payload integrity (NaN parsing, value formatting, date round-trips, config precedence, rate limiter)
- Group 3: API client behaviour via mock HTTP server (success, error propagation, retry, params)
- Group 4: email connectivity (placeholder, Phase 5)

---

### v1.0.0 — 2026-02-09 — Initial Scaffold

Project bootstrap: compilable binary with no commands.

**Added**
- `main.go` entry point calling `cmd.Execute()`
- `cmd/root.go` — Cobra root command, all global persistent flags registered
- `go.mod` module declaration
- `Makefile`, `.gitignore`, `LICENSE` (MIT), `README.md` stub
- `DEVPLAN.md` — full phased development specification

---

[v1.0.6]: https://github.com/derickschaefer/reserve/releases/tag/v1.0.6
[v1.0.5]: https://github.com/derickschaefer/reserve/releases/tag/v1.0.5
