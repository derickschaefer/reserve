# DEVPLAN — `reserve` CLI (FRED API Client)

> Binary: `reserve` | All commands: `reserve <noun> <verb> [args] [flags]`
> FRED® is a registered trademark of the Federal Reserve Bank of St. Louis.
> The name "FRED" appears only in help text, documentation, and API references.

---

## Guiding Principles

- **Compilable at every phase.** Each phase ends with a working binary.
- **No premature persistence.** Get the API and CLI right before introducing bbolt.
- **Small, composable packages.** Each internal package has one job.
- **Uniform result envelope.** Every command returns a `Result` — renderers handle the rest.
- **tablewriter for all table output.** (`github.com/olekukonko/tablewriter`)

---

## Phase 1 — Scaffold, Config, and API Client

**Goal:** `reserve series get CPIAUCSL` and `reserve obs get CPIAUCSL` hit the live API and print a formatted table.

### 1.1 Project Bootstrap

- Initialize `go.mod` as `github.com/yourname/reserve` (confirm module path before coding)
- Directory skeleton per spec section 12
- `Makefile` with targets: `build`, `test`, `lint`, `run`
- `.gitignore` (binaries, `config.json`, `*.db`)
- `config.json` at the working directory — primary API key source in Phase 1

### 1.2 Config Loader (`internal/config/`)

Single `Config` struct. Resolution order (first non-empty wins):

| Priority | Source |
|---|---|
| 1 | `--api-key` CLI flag |
| 2 | `FRED_API_KEY` environment variable |
| 3 | `config.json` in working directory |
| _(later)_ | bbolt config bucket (Phase 3) |

`config.json` minimal schema:

```json
{
  "api_key": "YOUR_KEY_HERE",
  "default_format": "table",
  "timeout": "30s",
  "concurrency": 8,
  "rate": 5.0
}
```

- Fail fast with actionable error if `api_key` is missing from all sources.
- Never log or print the API key.

### 1.3 Core Types (`internal/model/`)

Define the canonical data model. Nothing external depends on this yet.

- `Category { ID, Name, ParentID }`
- `SeriesMeta { ID, Title, Units, Frequency, SeasonalAdj, Notes, LastUpdated, Popularity }`
- `Release { ID, Name, PressRelease, Link }`
- `Source { ID, Name, Link }`
- `Tag { Name, GroupID, Notes, Created, Popularity }`
- `Observation { Date time.Time, Value float64, ValueRaw string, RealtimeStart, RealtimeEnd }`
- `SeriesData { SeriesID string, Meta *SeriesMeta, Obs []Observation }`
- `Result { Kind, GeneratedAt, Command, Data any, Warnings []string, Stats ResultStats }`
- `ResultStats { CacheHit bool, DurationMs int64, Items int }`
- Missing observation values (`"."` or `""`) → `math.NaN()`, raw string preserved in `ValueRaw`.

### 1.4 API Client (`internal/fred/`)

HTTP client wrapping the FRED REST API.

- Configurable base URL (default: `https://api.stlouisfed.org/fred/`)
- Appends `api_key` and `file_type=json` to every request automatically
- Context-aware via `context.Context`
- Retries: exponential backoff on HTTP 429 / 5xx, max 4 attempts
- Client-side rate limiter: token bucket via `golang.org/x/time/rate`
- `--debug` logs request URLs (with API key **redacted**) and response status codes

**Endpoints wired in Phase 1:**

| Method | FRED Endpoint |
|---|---|
| `GetSeries(id)` | `series` |
| `SearchSeries(query, opts)` | `series/search` |
| `GetObservations(id, opts)` | `series/observations` |

### 1.5 Cobra Root + Global Flags (`cmd/root.go`)

Register all global persistent flags on the root command:

`--format`, `--api-key`, `--timeout`, `--concurrency`, `--rate`, `--quiet`, `--verbose`, `--debug`, `--no-cache`, `--refresh`, `--out`, `--pager`

_(Email flags wired in Phase 5)_

### 1.6 First Live Commands (`cmd/series.go`, `cmd/obs.go`)

- `reserve series get <SERIES_ID...>` — fetch and display series metadata
- `reserve series search "<query>" [--limit N]` — search series by keyword
- `reserve obs get <SERIES_ID...> [--start] [--end] [--freq] [--units] [--agg]` — fetch observations
- `reserve obs latest <SERIES_ID...>` — most recent observation

### 1.7 Renderer (`internal/render/`)

Format-aware output keyed off `--format`. All renderers accept a `Result` envelope.

| Format | Implementation |
|---|---|
| `table` | `github.com/olekukonko/tablewriter` |
| `json` | `encoding/json`, full `Result` envelope |
| `jsonl` | one JSON object per line |
| `csv` | `encoding/csv`, header row always |
| `tsv` | tab-delimited, header row always |
| `md` | Markdown table (Phase 2+) |

**Phase 1 deliverable:** `table`, `json`, `jsonl`, `csv` working.

### 1.8 Phase 1 Tests

- `TestConfigResolution` — flag > env > file precedence
- `TestObservationParsing` — `"."` and `""` → `NaN`, numeric strings → `float64`
- `TestRenderTable`, `TestRenderCSV`, `TestRenderJSON` — golden output fixtures
- Integration test with mock HTTP server: verify `GetSeries`, `GetObservations`, retry on 429

---

## Phase 2 — Full Subcommand Tree (Discover + Retrieve)

**Goal:** All discovery and retrieval commands wired end-to-end, no caching yet. The binary is fully navigable.

### 2.1 Remaining API Endpoints (`internal/fred/`)

| Group | Endpoints |
|---|---|
| Category | `GetCategory`, `GetCategoryChildren`, `GetCategorySeries` |
| Series | `GetSeriesTags`, `GetSeriesCategories`, `GetSeriesRelated` |
| Release | `ListReleases`, `GetRelease`, `GetReleaseDates`, `GetReleaseSeries` |
| Source | `ListSources`, `GetSource`, `GetSourceReleases` |
| Tag | `SearchTags`, `GetTagSeries`, `GetRelatedTags` |

### 2.2 New Commands

**`reserve category`**
- `get <CATEGORY_ID>`
- `ls <CATEGORY_ID|root>`
- `tree <CATEGORY_ID|root> [--depth N]` — recursive with depth limit
- `series <CATEGORY_ID> [--limit N] [--filter <expr>]`

**`reserve release`**
- `list`
- `get <RELEASE_ID>`
- `dates <RELEASE_ID>`
- `series <RELEASE_ID> [--limit N]`

**`reserve source`**
- `list`
- `get <SOURCE_ID>`
- `releases <SOURCE_ID>`

**`reserve tag`**
- `search "<query>" [--limit N]`
- `series <TAG...> [--all|--any] [--limit N]`
- `related <TAG> [--limit N]`

**`reserve series`** (completing Phase 1 stubs)
- `tags <SERIES_ID>`
- `categories <SERIES_ID>`
- `related <SERIES_ID> [--by tags|categories] [--limit N]`
- `describe <SERIES_ID>` — rich summary: meta + recent obs stats

**`reserve search`**
- `"<query>" [--type series|category|release|tag|source|all] [--limit N]`

**`reserve meta`**
- `series <SERIES_ID...>`
- `category <CATEGORY_ID...>`
- `release <RELEASE_ID...>`
- `tag <TAG...>`
- `source <SOURCE_ID...>`

**`reserve fetch`** (batch convenience)
- `series <SERIES_ID...> [--with-meta] [--with-obs] [--start] [--end]`
- `category <CATEGORY_ID> [--recursive] [--depth N] [--limit-series N]`
- `query "<query>" [--top N] [--with-obs]`

Batch fetch uses a worker pool with `--concurrency` workers sharing a single rate limiter.

### 2.3 Concurrency Model (`internal/app/`)

Worker pool wired here for all batch commands:

- Bounded goroutine pool (size = `--concurrency`)
- Shared `rate.Limiter` from the API client
- `sync.WaitGroup` to join
- Partial results with `Warnings` on per-item errors (unless `--strict`)
- Deterministic output: sort by `series_id` then `date`

### 2.4 `reserve config` (file-backed only)

- `config init` — write a template `config.json` in the current directory
- `config get [--show-secrets]` — print current resolved config (redact `api_key` unless flag)
- `config set <key> <value>` — update `config.json`

### 2.5 `reserve completion`

- Shell completion for bash, zsh, fish via Cobra's built-in `completion` command

### 2.6 Phase 2 Tests

- Subcommand routing: every noun/verb pair resolves without error
- Batch concurrency: mock server with per-request counters, verify concurrency ceiling respected
- Worker pool: partial failure handling, warnings attached to `Result`

---

## Phase 3 — bbolt Persistence and Cache

**Goal:** All fetched data is cached in bbolt. `--refresh` forces re-fetch. `--no-cache` bypasses reads.

### 3.1 Store Layer (`internal/store/`)

Thin bbolt wrapper. Exposes clean methods; callers never see transactions.

```
GetMetaSeries(id string) (SeriesMeta, bool, error)
PutMetaSeries(meta SeriesMeta) error
GetObs(key string) (SeriesData, bool, error)
PutObs(key string, data SeriesData) error
GetQueryCache(key string) ([]string, bool, error)
PutQueryCache(key string, ids []string) error
```

- Read-only `tx` for reads; single write `tx` batches multiple puts
- DB path: env `FRED_DB_PATH` → default `~/.reserve/reserve.db`
- DB schema version stored in `config` bucket; migration runner on startup

**Buckets:**

| Bucket | Key Pattern | Contents |
|---|---|---|
| `meta_series` | `<SERIES_ID>` | JSON: `SeriesMeta` + `fetched_at` |
| `meta_category` | `<CATEGORY_ID>` | JSON: `Category` + `fetched_at` |
| `meta_release` | `<RELEASE_ID>` | JSON: `Release` + `fetched_at` |
| `meta_source` | `<SOURCE_ID>` | JSON: `Source` + `fetched_at` |
| `meta_tag` | `<TAG_NAME>` | JSON: `Tag` + `fetched_at` |
| `obs_cache` | `series:<ID>\|start:<D>\|end:<D>\|freq:<f>\|units:<u>\|agg:<a>` | JSON: observations |
| `query_cache` | `q:<sha256(normalized_params)>` | JSON: result IDs + `fetched_at` |
| `snapshots` | `snap:<ULID>` | JSON: snapshot record |
| `config` | `<key>` | string or JSON value |

### 3.2 TTL / Cache Invalidation (`internal/cache/`)

- TTL defaults: metadata = 24h, observations = 6h, query results = 24h
- TTL overridable in `config.json` / bbolt config
- `--refresh` flag: skip cache read, overwrite on write
- `--no-cache` flag: skip cache read and write
- Eviction: check `fetched_at` on read; stale entries are re-fetched transparently

### 3.3 Config Migration to bbolt

- `reserve config set` now writes to bbolt `config` bucket (and `config.json` for backward compat)
- API key resolution adds bbolt as priority 3 (between env and `config.json`)

### 3.4 Cache Commands

- `reserve cache stats` — bucket row counts, total disk usage, oldest/newest entries
- `reserve cache clear [--all | --bucket <name>]` — delete entries from one or all buckets
- `reserve cache warm <SERIES_ID...>` — pre-fetch and store metadata + observations

### 3.5 Snapshot Commands

- `reserve snapshot save --name <name> --cmd "<exact command line>"`
- `reserve snapshot list` — table of ID, name, command, created_at
- `reserve snapshot show <ID>` — full detail
- `reserve snapshot run <ID>` — re-execute stored command via `os/exec`
- `reserve snapshot delete <ID>`
- Snapshot IDs are ULIDs (sortable, collision-resistant)

### 3.6 Phase 3 Tests

- Cache key builder determinism (same params → same key, different params → different key)
- TTL invalidation: mock `fetched_at` in the past, verify re-fetch triggered
- `--refresh` bypasses stale cache
- bbolt read-only vs. read-write transaction isolation
- Migration: v0 → v1 schema upgrade path

---

## Phase 4 — Transform and Analyze Pipeline

**Goal:** `reserve obs get GDP | reserve transform pct-change | reserve analyze summary` works end-to-end.

### 4.1 Transform Operators (`internal/transform/`)

All operators consume `[]Observation` (or JSONL/CSV from stdin) and produce `[]Observation`.

| Command | Description |
|---|---|
| `pct-change [--period N]` | `(v[t] - v[t-N]) / v[t-N] * 100` |
| `diff [--order 1\|2]` | First or second difference |
| `log` | Natural log of each value |
| `index --base 100 --at YYYY-MM-DD` | Re-index relative to anchor date |
| `normalize [--method zscore\|minmax]` | Z-score or 0–1 scaling |
| `resample --freq M\|Q\|A --method mean\|last\|sum` | Temporal downsampling |
| `filter --where <expr>` | Simple date/value predicate filter |

### 4.2 Rolling Window (`reserve window`)

- `roll --stat mean|std|min|max|sum --window N [--min-periods M]`
- Handles `NaN` values: skipped in computation, preserved in output

### 4.3 Analysis (`internal/analyze/`)

| Command | Output |
|---|---|
| `analyze summary [--window N]` | Count, mean, std, min, max, median, skew, missing% |
| `analyze trend [--method linear\|theil-sen]` | Slope, intercept, R², p-value, direction label |
| `analyze seasonality [--strength]` | Seasonal indices, strength score |
| `analyze decomposition` | Trend + seasonal + residual components (basic STL) |

### 4.4 Comparison (`reserve compare`)

- `corr <A> <B> [--lag-min N] [--lag-max N]` — cross-correlation at each lag
- `spread <A> <B>` — A minus B, with summary stats
- `beta <TARGET> --x <FACTOR...> [--window N]` — rolling beta coefficient

### 4.5 Detection (`reserve detect`)

- `anomalies [--method zscore|iqr] [--threshold X]` — flag outlier observations
- `breaks [--method chow]` — structural break detection (omit if not feasible in v1)

### 4.6 Pipeline Wiring

- All transform/analyze commands read from stdin if no `--in` file specified
- Input auto-detected: JSONL (canonical format), CSV, or TSV
- Output respects `--format` (default `jsonl` for pipeline, `table` for terminal)
- `isatty` detection: if stdout is a terminal, default to `table`; otherwise default to `jsonl`

### 4.7 Phase 4 Tests

- `TestPctChange` — known input/output pairs including edge cases (zero denominator, NaN)
- `TestDiff` — first and second order
- `TestResample` — monthly → quarterly, mean and last methods
- `TestRollMean` — window with min-periods
- `TestTrendLinear` — slope direction matches known series
- `TestAnomalyZScore` — detects injected outliers
- Golden tests: pipe `obs get → transform pct-change → analyze summary`, assert stable output

---

## Phase 5 — Advanced Features: Model, Explain, Export, Report, Email

**Goal:** Complete the spec. Ship the full acceptance criteria.

### 5.1 Model (`reserve model`)

Minimal deterministic modeling only — no external ML libraries.

- `regress <Y> --x <X...> [--window N]` — OLS regression, rolling if `--window` set
- `forecast <SERIES> --horizon N [--method naive|sma|ets]`
  - `naive`: last value carried forward
  - `sma`: simple moving average
  - `ets`: Holt's exponential smoothing (double)

### 5.2 Explain (`reserve explain`)

- `move <SERIES> [--since YYYY-MM-DD] [--top-related N]`
- Produces narrative + metrics table:
  - Last value, change since date, YoY change, annualized volatility
  - Top N correlated series (from cached observations)
  - Plain-English trend direction sentence

### 5.3 Export (`reserve export`)

- `data <SERIES_ID...> [--out file.csv]` — clean CSV/TSV export with metadata header
- `chart <SERIES_ID...> [--out file.png]` — ASCII sparkline to terminal; PNG if `--out` specified
  (PNG requires a lightweight chart library, e.g., `gonum/plot`)

### 5.4 Report (`reserve report`)

- `series <SERIES_ID...> [--template builtin] [--out file.md|html]`
  - Markdown or HTML: title, metadata table, observations table, summary stats
- `snapshot <SNAPSHOT_ID>` — re-run snapshot and wrap output in a report template

### 5.5 Email Output (`internal/notify/`)

`Notifier` interface:

```go
type Notifier interface {
    Send(ctx context.Context, msg Message) error
}
```

Implementations: `SMTPNotifier` (STARTTLS), `SendmailNotifier`.

- `--email <addr,...>` triggers send after command completes
- `--subject` overrides default subject (`reserve: <command summary>`)
- `--out` path used as attachment; otherwise body is rendered output
- Config: `FRED_EMAIL_FROM`, `FRED_EMAIL_SMTP_*` env vars or bbolt config
- Failure is non-fatal unless `--email-strict` set
- SMTP password never logged

### 5.6 `reserve explain` Tests

- Narrative generation: assert known series movements produce expected direction labels
- Report: golden `.md` output for a fixture series

---

## Dependency Manifest

| Package | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI subcommand tree |
| `github.com/olekukonko/tablewriter` | Table rendering |
| `go.etcd.io/bbolt` | Embedded key-value persistence (Phase 3+) |
| `golang.org/x/time/rate` | Token bucket rate limiter |
| `log/slog` (stdlib) | Structured logging |
| `github.com/oklog/ulid/v2` | ULID generation for snapshot IDs (Phase 3+) |
| `gonum.org/v1/plot` | Chart export PNG (Phase 5, optional) |

---

## Directory Layout

```
reserve/
├── cmd/
│   ├── root.go          # Root command, global flags, config bootstrap
│   ├── series.go
│   ├── category.go
│   ├── obs.go
│   ├── release.go
│   ├── source.go
│   ├── tag.go
│   ├── search.go
│   ├── meta.go
│   ├── fetch.go
│   ├── transform.go
│   ├── window.go
│   ├── analyze.go
│   ├── compare.go
│   ├── detect.go
│   ├── model.go
│   ├── explain.go
│   ├── export.go
│   ├── report.go
│   ├── config.go
│   ├── cache.go
│   └── snapshot.go
├── internal/
│   ├── app/             # Dependency wiring (Config, Client, Store, Renderer)
│   ├── config/          # Config loading and resolution
│   ├── fred/            # FRED API HTTP client + endpoint methods
│   ├── model/           # Canonical types: SeriesMeta, Observation, Result, …
│   ├── store/           # bbolt abstraction + migrations
│   ├── cache/           # TTL key builders and invalidation logic
│   ├── transform/       # Pipeline operators
│   ├── analyze/         # Statistical analysis
│   ├── render/          # Format renderers (table/json/jsonl/csv/tsv/md)
│   ├── notify/          # Notifier interface + SMTP/sendmail
│   └── util/            # Date parsing, hashing, error helpers
├── tests/
│   ├── integration/     # Mock HTTP server tests
│   └── golden/          # Expected output fixtures
├── config.json          # Local config (gitignored)
├── Makefile
├── go.mod
├── go.sum
└── DEVPLAN.md
```

---

## Acceptance Criteria Summary

| Phase | Done When |
|---|---|
| 1 | `reserve series get CPIAUCSL` and `reserve obs get CPIAUCSL` hit live API; table/json/csv output works |
| 2 | All discovery/retrieval subcommands navigable; batch fetch respects concurrency limit |
| 3 | Results cached in bbolt; `--refresh` and `--no-cache` work; snapshots save and re-run |
| 4 | `obs get \| transform pct-change \| analyze summary` pipeline produces stable output |
| 5 | `--email` sends output; `export chart` generates chart; `explain move` produces narrative |
