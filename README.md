# reserve

A command-line tool for exploring, retrieving, and analyzing economic data from the
Federal Reserve Bank of St. Louis FRED® API.

> FRED® is a registered trademark of the Federal Reserve Bank of St. Louis.  
> Data sourced from FRED®, Federal Reserve Bank of St. Louis; https://fred.stlouisfed.org/  
> This project is not affiliated with or endorsed by the Federal Reserve Bank of St. Louis.

---

## Table of Contents

- [Why reserve?](#why-reserve)
- [Install](#install)
- [Release Integrity](#release-integrity)
- [Quick Start](#quick-start)
- [Design Philosophy](#design-philosophy)
- [The Command Model](#the-command-model)
- [Command Reference](#command-reference)
  - [series](#series) — discover and inspect data series
  - [obs](#obs) — retrieve observations
  - [category](#category) — browse the data hierarchy
  - [release](#release) — data releases
  - [source](#source) — data source institutions
  - [tag](#tag) — search by tag
  - [search](#search) — global full-text search
  - [meta](#meta) — batch metadata retrieval
  - [fetch](#fetch) — accumulate data locally
  - [transform](#transform) — pipeline operators
  - [window](#window) — rolling statistics
  - [analyze](#analyze) — statistical analysis
  - [cache](#cache) — manage local database
  - [config](#config) — configuration management
  - [version](#version) — binary version and build info
  - [onboard](#onboard) — machine-readable onboarding context
- [Pipeline Usage](#pipeline-usage)
- [Output Formats](#output-formats)
- [Global Flags](#global-flags)
- [Configuration](#configuration)
- [Changelog](#changelog)
- [License](#license)

---

## Why reserve?

The FRED® API is one of the richest free economic data sources in the world — 800,000+ series, updated continuously. But most tools that wrap it are platform-locked, dependency-heavy, or require a running database server just to get started.

`reserve` takes a different approach:

- **Cross-platform, zero-dependency binary.** Written in Go, `reserve` compiles to a single static executable with no runtime, no interpreter, and no external libraries to install. Run the same binary on Linux x86-64, Windows, ARM servers, and Apple Silicon — natively, without emulation.

- **Command-object model.** Every subcommand is a first-class object with a defined input schema, validation, and a uniform `Result` envelope. Commands compose cleanly, behave predictably, and are trivial to extend. No monolithic scripts, no implicit globals.

- **Embedded database — no server required.** Observation data is persisted in a single embedded database file that is created on the fly. The actual embedded database is [bbolt](https://github.com/etcd-io/bbolt), a proven embedded key-value store used in production systems. It can scale to hold tens of millions of datapoints with ease. No Postgres, no SQL Server, no running process. If your use cases require data to be centralized in a server or cloud data platform such as Snowflake, comma-separated value outputs are supported throughout reserve.

- **Pipeline-ready for large data environments.** `reserve` speaks JSONL on stdin/stdout — the lingua franca of Unix data pipelines. Chain transforms and analyses with `|`, redirect to files, or feed downstream tools. Every operator is NaN-aware and handles FRED's missing-value conventions correctly at scale. CSV formatting is also supported for importing data into other data stores or spreadsheets.

- **LLM and agentic workflow ready.** JSONL is the native input format for modern AI pipelines. Pipe `reserve` output directly into LLM tool-call chains, vector embedding workflows, or agentic analysis frameworks — economic time series, transformed and structured, exactly where your model expects it.

- **Built-in rate limiting and retry logic.** The API client enforces a configurable token-bucket rate limiter and exponential backoff on transient failures — the right defaults for shared financial data environments where API quotas matter.

- **Idiomatic Go semantics throughout.** Structured logging via `slog`, context cancellation on every HTTP call, bounded concurrency with `sync.WaitGroup` and semaphores, deterministic output ordering, and clean separation between packages. The codebase is readable, testable, and auditable.

- **Small, fast, and self-contained.** The compiled binary is under 15 MB. Cold start is measured in milliseconds. Analysis on years of monthly data runs in memory without paging. The right tool for automated pipelines, cron jobs, and production data workflows — not just interactive exploration.

---

## Install

```bash
curl -fsSL https://download.reservecli.dev/install.sh | sh
```

Pinned version:

```bash
curl -fsSL https://download.reservecli.dev/install.sh | sh -s v1.1.1
```

Windows PowerShell:

```powershell
irm https://download.reservecli.dev/install.ps1 | iex
```

These installers download from your configured `download.reservecli.dev` distribution endpoint. Publishing to that endpoint is a manual release step. The release payload now includes a root-level `release.json` manifest that powers `reserve update check`.

From source:

```bash
git clone https://github.com/derickschaefer/reserve
cd reserve
make build
```

Requires Go 1.25.7+ for source builds.

For distributed release binaries, use the stripped release target introduced in `v1.1.1`:

```bash
make build-release
```

`make build` keeps symbols for local debugging. `make build-release` applies Go linker flags `-s -w` to remove the symbol table and DWARF debug information for smaller shipped binaries.

---

## Release Integrity

Release artifacts are signed with **keyless Sigstore/cosign signatures** from
GitHub Actions OIDC.

- Workflow: `.github/workflows/release-keyless.yml`
- Verification guide: [`docs/release-security.md`](docs/release-security.md)

Quick verification (after downloading release assets from your distribution endpoint):

```bash
sha256sum -c SHA256SUMS
cosign verify-blob \
  --certificate SHA256SUMS.pem \
  --signature SHA256SUMS.sig \
  --certificate-identity-regexp '^https://github.com/derickschaefer/reserve/\.github/workflows/release-keyless\.yml@refs/tags/v[0-9].*$' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  SHA256SUMS
```

---

## Quick Start

**1. Get a free API key**

Register at https://fred.stlouisfed.org/docs/api/api_key.html

**2. Configure**

```bash
reserve config init
reserve config set api_key YOUR_KEY_HERE
```

Or set via environment variable:

```bash
export FRED_API_KEY=YOUR_KEY_HERE
```

**3. Explore live data**

```bash
reserve series search "unemployment rate" --limit 5
reserve series get UNRATE
reserve obs get UNRATE --start 2020-01-01
reserve obs latest GDP UNRATE CPIAUCSL
```

**4. Accumulate data locally for analysis**

```bash
reserve fetch series GDP CPIAUCSL UNRATE FEDFUNDS --start 2010-01-01 --store
reserve obs get GDP --from cache
```

**5. Run the analysis pipeline**

```bash
# Quarter-over-quarter GDP growth with summary statistics
reserve obs get GDP --from cache --format jsonl | reserve transform pct-change | reserve analyze summary

# Long-run unemployment trend
reserve obs get UNRATE --from cache --format jsonl | reserve analyze trend

# Annual CPI averages
reserve obs get CPIAUCSL --from cache --format jsonl | reserve transform resample --freq annual --method mean
```

---

## Design Philosophy

`reserve` operates in two distinct modes:

**Live mode** — Discovery and retrieval commands (`series`, `obs`, `category`, `search`, etc.) hit the FRED API directly by default. `obs get` uses `--from live` implicitly when no source is specified.

**Analysis mode** — You explicitly accumulate data into a local [bbolt](https://github.com/etcd-io/bbolt) database using `fetch --store`, then read it back with `obs get --from cache`. That makes analysis fast, reproducible, and offline-capable.

The pipeline is Unix-native. Commands that produce observations write JSONL to stdout; transform and analyze commands read JSONL from stdin. Chain them with `|`. When stdout is a terminal, output defaults to a formatted table. When piped, it defaults to JSONL.

---

## The Command Model

`reserve` uses a pragmatic command model that is worth understanding before you explore the full command reference.

Most commands follow a **noun-verb** pattern: the top-level command names a resource, and its subcommands are operations on that resource. This maps naturally onto the structure of the FRED API and the local data model.
```
reserve series get UNRATE            # noun: series  / verb: get
reserve category tree root           # noun: category / verb: tree
reserve release list                 # noun: release  / verb: list
reserve obs get CPIAUCSL --from cache # noun: obs     / verb: get
reserve config set api_key XYZ      # noun: config   / verb: set
```

`obs` is still a normal FRED wrapper noun. It is slightly unusual only because the name is abbreviated; conceptually it belongs alongside `series`, `category`, `release`, `source`, `tag`, and `meta`.

`onboard` is different: it is a support/meta command that emits machine-readable documentation for agents, LLMs, and advanced users.
```
reserve obs get UNRATE --start 2020-01-01   # noun: obs (observations)
reserve onboard --topic pipeline            # onboarding context for agents and advanced users
```

**Pipeline operators** — `transform`, `window`, `analyze`, and `chart` — are pure verbs. They have no resource noun because they do not target a named entity. They operate on whatever JSONL stream arrives on stdin. The data source is implicit, so there is nothing meaningful to name.
```
... | reserve transform pct-change --period 12
... | reserve window roll --stat mean --window 6
... | reserve analyze trend
... | reserve chart plot
```

Finally, `fetch` and `search` are action-oriented commands rather than resource nouns. `fetch` performs batch acquisition across supported entity types, while `search` performs full-text query across supported global entities such as series and tags.
```
reserve fetch series GDP UNRATE CPIAUCSL --store
reserve search "yield curve"
```

In summary:

| Class | Pattern | Examples |
|---|---|---|
| FRED API wrappers | noun verb | `obs`, `series`, `category`, `release`, `source`, `tag`, `meta` |
| Local state operations | noun verb | `cache`, `config` |
| Support / meta commands | noun verb | `onboard` |
| Pipeline operators | verb only | `transform`, `window`, `analyze`, `chart` |
| Batch acquisition | verb noun | `fetch` |
| Standalone query | verb only | `search` |
| Utility | standalone | `version`, `completion`, `help` |

The noun-verb commands follow consistent flag conventions and produce the same `Result` envelope. The pipeline operators follow consistent stdin/stdout JSONL semantics. Within each class, behavior is uniform and predictable.

## Command Reference

### series

Discover and inspect FRED data series. Use `series get` for immediate metadata lookup on known IDs; use `fetch` when you want batch acquisition workflows.

```bash
reserve series get <SERIES_ID...>           # metadata for one or more series
reserve series search "<query>" [--limit N] # full-text search
reserve series tags <SERIES_ID>             # tags applied to a series
reserve series categories <SERIES_ID>       # categories a series belongs to
```

Examples:

```bash
reserve series get CPIAUCSL
reserve series get GDP UNRATE CPIAUCSL --format json
reserve series search "consumer price index" --limit 10
reserve series tags UNRATE
reserve series categories GDP
```

---

### obs

Fetch time series observations from a selected source. The default source is live FRED API access. Use `obs get` for immediate retrieval on stdout; use `fetch series --store` when you want to build or refresh a reusable local dataset first.

```bash
reserve obs get <SERIES_ID...> [flags]
reserve obs latest <SERIES_ID...>
```

Flags for `obs get`:

```
--start YYYY-MM-DD   start date
--end   YYYY-MM-DD   end date
--freq  daily|weekly|monthly|quarterly|annual
--units lin|chg|ch1|pch|pc1|pca|cch|cca|log
--agg   avg|sum|eop
--from  live|cache    data origin (default: live)
--limit N            max observations (0 = all)
```

Units reference: `lin` = levels, `pch` = % change, `pc1` = % change from year ago, `log` = natural log.

Examples:

```bash
reserve obs get UNRATE --start 2020-01-01 --end 2024-12-31
reserve obs get GDP --from cache
reserve obs get GDP --from cache --format jsonl
reserve obs get CPIAUCSL --freq monthly --units pc1    # year-over-year % change
reserve obs get GDP CPIAUCSL --format csv --out data.csv
reserve obs latest GDP UNRATE CPIAUCSL FEDFUNDS
```

---

### category

Browse the FRED category hierarchy.

```bash
reserve category get <CATEGORY_ID>
reserve category list <CATEGORY_ID|root>
reserve category tree <CATEGORY_ID|root> [--depth N]
reserve category series <CATEGORY_ID> [--limit N]
```

Examples:

```bash
reserve category list root               # top-level categories
reserve category tree 32991 --depth 2  # subtree with depth limit
reserve category series 32991          # series within a category
```

---

### release

Explore scheduled FRED data releases.

```bash
reserve release list
reserve release get <RELEASE_ID>
reserve release dates <RELEASE_ID>
reserve release series <RELEASE_ID> [--limit N]
```

---

### source

Explore the institutions that provide data to FRED.

```bash
reserve source list
reserve source get <SOURCE_ID>
reserve source releases <SOURCE_ID>
```

---

### tag

Search and explore FRED tags.

```bash
reserve tag search "<query>" [--limit N]
reserve tag series <TAG...> [--limit N]
reserve tag related <TAG> [--limit N]
```

---

### search

Global full-text search across supported global entity types.

```bash
reserve search "<query>" [--type series|tag|all] [--limit N]
```

Examples:

```bash
reserve search "consumer price index" --type series --limit 10
reserve search "employment" --type all
```

---

### meta

Batch metadata retrieval for any entity type.

```bash
reserve meta series <SERIES_ID...>
reserve meta category <CATEGORY_ID...>
reserve meta release <RELEASE_ID...>
reserve meta tag <TAG...>
reserve meta source <SOURCE_ID...>
```

---

### fetch

Fetch metadata or observations from the FRED API in batch. Use `series get` or `obs get` when you want data immediately on stdout; use `fetch`, especially `fetch series --store`, when you want to acquire and persist a reusable local working set.

If you need several series, prefer one multi-series `fetch series` call over many one-off fetches. reserve already performs bounded concurrent retrieval with a shared rate limiter for the batch.

```bash
reserve fetch series <SERIES_ID...> [--start YYYY-MM-DD] [--end YYYY-MM-DD] [--store]
reserve fetch category <CATEGORY_ID|root>
reserve fetch query "<query>" [--limit N]
```

For `fetch series`:

```
--store              fetch observations and persist them to the local database; also stores series metadata
--start YYYY-MM-DD   start date for fetched observations
--end   YYYY-MM-DD   end date for fetched observations
```

Examples:

```bash
# Build a local dataset with four core macro series from 2010 onward
reserve fetch series GDP CPIAUCSL UNRATE FEDFUNDS --start 2010-01-01 --store

# Refresh one cached series from a known start date
reserve fetch series GDP --start 2010-01-01 --store
```

Stored data is written to `~/.reserve/reserve.db` by default (override with `db_path` in `config.json` or the `RESERVE_DB_PATH` environment variable). There is no automatic expiry on stored observations.

---

### transform

Pipeline operators. Each reads JSONL from stdin, applies a transformation, and writes JSONL to stdout.

```bash
reserve transform pct-change [--period N]
reserve transform diff [--order 1|2]
reserve transform log
reserve transform index --base 100 --at YYYY-MM-DD
reserve transform normalize [--method zscore|minmax]
reserve transform resample --freq monthly|quarterly|annual --method mean|last|sum
reserve transform filter [--after YYYY-MM-DD] [--before YYYY-MM-DD] \
                         [--min N] [--max N] [--drop-missing]
```

| Operator | Description |
|---|---|
| `pct-change` | `(v[t] − v[t-N]) / |v[t-N]| × 100`. Default period=1 (period-over-period). Use `--period 12` for year-over-year on monthly data. |
| `diff` | First difference `v[t] − v[t-1]`, or second difference with `--order 2`. |
| `log` | Natural log of each value. Non-positive inputs produce NaN with a warning. |
| `index` | Re-scales the series so the value at `--at` equals `--base` (default 100). |
| `normalize` | Z-score standardization (`zscore`) or min-max scaling to 0–1 (`minmax`). |
| `resample` | Downsample to a lower frequency. `mean` averages the period, `last` takes the final value, `sum` accumulates. |
| `filter` | Retain observations within a date range or value bounds. `--drop-missing` removes NaN rows. |

Examples:

```bash
# Quarter-over-quarter GDP growth rate
reserve obs get GDP --from cache --format jsonl | reserve transform pct-change

# Year-over-year CPI inflation (monthly data)
reserve obs get CPIAUCSL --from cache --format jsonl | reserve transform pct-change --period 12

# Index GDP to 100 at the start of 2010
reserve obs get GDP --from cache --format jsonl | reserve transform index --base 100 --at 2010-01-01

# Annual average CPI
reserve obs get CPIAUCSL --from cache --format jsonl | reserve transform resample --freq annual --method mean

# Post-2020 observations only
reserve obs get UNRATE --from cache --format jsonl | reserve transform filter --after 2020-01-01
```

---

### window

Rolling window statistics over a JSONL stream.

```bash
reserve window roll --stat mean|std|min|max|sum --window N [--min-periods M]
```

NaN values are excluded from window computations. If fewer than `--min-periods` valid values exist in a window, the output for that period is NaN.

Examples:

```bash
# 12-month rolling average unemployment rate
reserve obs get UNRATE --from cache --format jsonl | reserve window roll --stat mean --window 12

# 4-quarter rolling standard deviation of GDP growth
reserve obs get GDP --from cache --format jsonl | reserve transform pct-change \
  | reserve window roll --stat std --window 4
```

---

### analyze

Statistical analysis on a JSONL stream. Results print to the terminal (table or JSON).

```bash
reserve analyze summary               # descriptive statistics
reserve analyze trend [--method linear|theil-sen]
```

**`analyze summary`** produces:

| Field | Description |
|---|---|
| count | total observations |
| missing | NaN count and percentage |
| mean, std | mean and standard deviation |
| min, p25, median, p75, max | five-number summary |
| skew | Fisher-Pearson skewness coefficient |
| first, last | boundary non-NaN values |
| change, change_pct | absolute and percentage change over the full series |

**`analyze trend`** produces:

| Field | Description |
|---|---|
| direction | `up`, `down`, or `flat` |
| slope_per_year | trend slope in original units per year |
| slope_per_day | slope in original units per day |
| intercept | regression intercept |
| r2 | coefficient of determination (0–1) |
| method | `linear` (OLS) or `theil-sen` (robust) |

Examples:

```bash
reserve obs get UNRATE --from cache --format jsonl | reserve analyze summary
reserve obs get GDP --from cache --format jsonl | reserve transform pct-change | reserve analyze summary
reserve obs get UNRATE --from cache --format jsonl | reserve analyze trend
reserve obs get UNRATE --from cache --format jsonl | reserve analyze trend --method theil-sen
```

---

### cache

Manage the local embedded key-value cache database (bbolt).

```bash
reserve cache stats                         # bucket row counts and DB size
reserve cache inventory                     # per-series local coverage, date ranges, and gaps
reserve cache clear --all                   # wipe all data
reserve cache clear --bucket obs            # wipe observations only
reserve cache clear --bucket series_meta    # wipe metadata only
reserve cache clear --series GDP            # wipe cached observation sets for one series
reserve cache compact                       # reclaim disk space after clearing
```

`cache inventory` gives a higher-level view of what you have locally: one row per cached series with merged date coverage, point counts, gap counts, frequency, and whether metadata is present. It ends with a small rule-based action summary so you can quickly see whether the next step is metadata enrichment, range refill, or no action at all. Daily series display `GAPS` as `n/a` because weekends and market holidays make gap counting misleading for that cadence.

When `obs get --from cache` encounters multiple cached observation sets for the same series and no exact date/parameter filter is provided, reserve now chooses a canonical local set by widest coverage and warns which range was selected. Likewise, storing a second observation set for the same series emits a warning so the cache does not silently drift into multiple competing local variants.

`cache clear --series <ID>` removes all cached observation sets for one series while leaving its stored metadata intact. This is the preferred cleanup level when you want to rebuild one local series without wiping the entire observations bucket.

For disciplined local-cache workflows, prefer live reads for ad hoc questions, use `cache inventory` before storing additional variants of a series, and treat `cache clear --series` as a deliberate rebuild step rather than an automatic cleanup action.

`cache clear` removes entries from one bucket or all buckets. bbolt does not shrink the database file automatically — freed pages are returned to an internal freelist and reused on future writes. The file footprint does not decrease until you run `compact`.

`cache compact` rewrites the database to a new file, recovering all space freed by prior clears. The operation is safe: live data is copied to a temporary file first, then the original is atomically replaced.

```bash
# Typical maintenance workflow after a large clear:
reserve cache clear --all
reserve cache compact
```

---

### config

Manage `config.json` in the user config directory, with optional local `./config.json` overrides.

```bash
reserve config init                    # create a template config.json in your user config directory
reserve config get [--show-secrets]    # print resolved configuration (key redacted by default)
reserve config set <key> <value>       # update a single value
```

Valid keys: `api_key`, `default_format`, `timeout`, `concurrency`, `rate`, `base_url`, `db_path`.

---

### version

Print the reserve version string and build metadata.

```bash
reserve version                  # plain text — grep/awk friendly
reserve version --format json    # structured output
reserve version --format jsonl   # single line for audit streams
```

Plain text output:

```bash 
reserve v1.0.9
go      go1.25.7
os      linux/amd64
built   2026-02-28T18:42:00Z
```

---

### update

Check a lightweight remote release manifest for newer versions and short release notes.

```bash
reserve update check                # human-readable update status
reserve update check --format json  # structured output for scripts
reserve update check --format jsonl # single-line audit/event output
```

When a new release is available, `reserve` prints the current version, latest version, a short summary, release highlights, and static update instructions from the remote manifest.

---

### onboard

Emit a machine-readable onboarding document for agents, LLMs, and advanced users.
It is designed to be pasted directly into Claude, ChatGPT, or another agent session
so the model gets authoritative reserve semantics without guessing at commands or flags.

```bash
reserve onboard                          # full-program onboarding
reserve onboard obs                      # command-specific onboarding
reserve onboard --topic pipeline         # single topic
reserve onboard --topic pipeline,gotchas # comma-separated topics
reserve onboard --topic all              # full document (large context windows)
reserve onboard --topic all | pbcopy     # copy to clipboard
reserve onboard export ./onboard         # write program.json + per-command docs
```

**Topics:**

| Topic | Contents |
|---|---|
| `toc` | Topic index and interaction guide (default) |
| `commands` | Full command reference: nouns, verbs, flags, formats |
| `pipeline` | stdin/stdout semantics, JSONL format, operator chaining |
| `data-model` | Core types, NaN handling, Result envelope |
| `examples` | Verified end-to-end examples with confirmed output values |
| `gotchas` | Sharp edges, missing data, known limitations |
| `version` | Build metadata for provenance |
| `all` | Everything — for large context windows |

**Workflow:**
```bash
# Step 1 — handshake: let the AI tell you what it needs
reserve onboard --topic toc
# paste into your LLM session

# Step 2 — surgical context: paste only what the AI requested
reserve onboard --topic pipeline,data-model,gotchas
# paste → LLM confirms ready

# Step 3 — ask your question
```

**Project export:**
```bash
reserve onboard export ./onboard
# writes program.json plus command docs like obs.json, series.json, and config.json
```

**Suggested prompt:**
```
I am about to explore macro-economic data from the FRED API.
Use this as your reference for a CLI called reserve.
It contains the authoritative command reference, pipeline
semantics, verified examples, and known gotchas.
Tell me when you are ready.
```

Output is always JSON with HTML escaping disabled — `<` and `>` render
literally, not as `\u003c` / `\u003e`.

---


## Pipeline Usage

Any command that produces observations can be piped into a transform or analyze command:

```bash
# Full macro pipeline: fetch → cache read → transform → analyze
reserve obs get GDP --from cache --format jsonl \
  | reserve transform pct-change \
  | reserve analyze summary

# Post-COVID unemployment: filter → rolling average
reserve obs get UNRATE --from cache --format jsonl \
  | reserve transform filter --after 2020-01-01 \
  | reserve window roll --stat mean --window 12

# Inflation signal: resample monthly CPI to annual → trend
reserve obs get CPIAUCSL --from cache --format jsonl \
  | reserve transform resample --freq annual --method mean \
  | reserve analyze trend

# Year-over-year unemployment change → CSV file
reserve obs get UNRATE --from cache --format jsonl \
  | reserve transform pct-change --period 12 \
  | reserve transform filter --drop-missing \
  > unrate_yoy.csv
```

**NaN handling:** FRED missing values (reported as `"."`) become `NaN` internally. Transforms skip NaN inputs in calculations and propagate NaN to outputs where appropriate. `analyze summary` counts and reports missing values explicitly.

**Format auto-detection:** Pipeline commands default to `jsonl` when piped, `table` when output is a terminal. Override with `--format` on any command.

---

## Output Formats

All commands accept `--format`:

| Format | Description |
|---|---|
| `table` | Human-readable aligned table (default for terminal output) |
| `json` | Full result envelope as pretty-printed JSON |
| `jsonl` | One JSON object per line (default for piped output) |
| `csv` | Comma-separated with header row |
| `tsv` | Tab-separated with header row |
| `md` | Markdown table |

Write to a file with `--out`:

```bash
reserve obs get GDP --format csv --out gdp.csv
reserve obs get CPIAUCSL --from cache --format jsonl --out cpi.jsonl
```

---

## Global Flags

These flags are available on every command:

```
--format table|json|jsonl|csv|tsv|md    output format
--out <path>                            write command output to file (renderer-backed commands)
--api-key <key>                         override API key for this invocation only
--timeout <duration>                    HTTP request timeout (default: 30s)
--concurrency <n>                       parallel requests for batch operations (default: 8)
--rate <n>                              API requests/sec client-side limit (default: 5.0)
--verbose                               show timing and cache stats after output
--debug                                 log HTTP requests (API key redacted)
--quiet                                 suppress all non-error output
--no-cache                              bypass local database reads
--refresh                               force re-fetch and overwrite cached entries
```

---

## Configuration

`reserve config init` creates `config.json` in your user config directory:

- Linux: `~/.config/reserve/config.json`
- macOS: `~/Library/Application Support/reserve/config.json`
- Windows: `%AppData%\reserve\config.json`

If a local `./config.json` exists in the current working directory, it overrides the user config file for that shell location.

This makes ad hoc project overrides easy in a very Unix-style way: keep your personal API key and defaults in the user config file, then drop a local `./config.json` into a test or project directory when you want to override only selected settings such as `default_format` or `db_path`.

Example local override:

```json
{
  "default_format": "json",
  "db_path": "/tmp/reserve-local.db"
}
```

In that directory, `reserve` will still inherit any missing values such as `api_key` from your user config, while honoring the local overrides for the fields you set.

Example file contents:

```json
{
  "api_key":        "YOUR_KEY_HERE",
  "default_format": "table",
  "timeout":        "30s",
  "concurrency":    8,
  "rate":           5.0,
  "db_path":        ""
}
```

**API key resolution order** (first non-empty wins):

1. `--api-key` CLI flag
2. `FRED_API_KEY` environment variable
3. `api_key` in local `./config.json`
4. `api_key` in user `config.json`

**Database path resolution order:**

1. `RESERVE_DB_PATH` environment variable
2. `db_path` in local `./config.json`
3. `db_path` in user `config.json`
4. Default: `~/.reserve/reserve.db`

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for the full version history.

---

## License

MIT — see [LICENSE](LICENSE).

FRED® is a registered trademark of the Federal Reserve Bank of St. Louis.  
This project is not affiliated with or endorsed by the Federal Reserve Bank of St. Louis.
