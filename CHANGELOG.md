# Changelog

THIS IS A DRAFT DOCUMENT AND WILL BE REFINED UPON v1.0.5 RELEASE

All notable changes to `reserve` are documented here.

The project uses **[Semantic Versioning](https://semver.org/)**. v1.0.5 is the
first publicly tagged release. Prior versions are documented under
[Development History](#development-history) for auditability.

---

## [v1.0.5] — 2026-XX-XX — First Public Release

**Phase 4 complete. Production documentation. First tagged GitHub release.**

### Added
- `README.md` rewritten as full production documentation: install, quick start,
  design philosophy, complete command reference with examples, pipeline usage
  guide, output formats, global flags, and configuration reference
- `CHANGELOG.md` introduced with full development history
- "Why reserve?" positioning section covering cross-platform portability,
  command-object model, embedded database, pipeline semantics, LLM/agentic
  workflow readiness, and Go idioms
- Table of contents with anchor links throughout README

### Notes
- No functional code changes from v1.0.4
- All Phase 4 transform, window, and analyze commands verified against live
  FRED data prior to this release
- Recommended upgrade path from any prior untagged build: `git pull && go build -o reserve .`

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
- `category get|ls|tree|series` — browse the FRED category hierarchy
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

[v1.0.5]: https://github.com/derickschaefer/reserve/releases/tag/v1.0.5
