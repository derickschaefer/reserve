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
- [Quick Start](#quick-start)
- [Design Philosophy](#design-philosophy)
- [The Command Model](#the-command-model)
- [Command Reference](#command-reference)
  - [series](#series) — discover and inspect data series
  - [obs](#obs) — fetch live observations
  - [category](#category) — browse the data hierarchy
  - [release](#release) — data releases
  - [source](#source) — data source institutions
  - [tag](#tag) — search by tag
  - [search](#search) — global full-text search
  - [meta](#meta) — batch metadata retrieval
  - [fetch](#fetch) — accumulate data locally
  - [store](#store) — inspect local data
  - [transform](#transform) — pipeline operators
  - [window](#window) — rolling statistics
  - [analyze](#analyze) — statistical analysis
  - [cache](#cache) — manage local database
  - [snapshot](#snapshot) — reproducible workflows
  - [config](#config) — configuration management
  - [version](#version) — binary version and build info
  - [llm](#llm) — LLM onboarding context
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

- **Embedded database — no server required.** Observation data is persisted in a single file, embedded database that is created on the fly.  Th actual embedded database is [bbolt](https://github.com/etcd-io/bbolt), a proven embedded key-value store used in production systems.  It can scale to hold ten's of millions of datapoints with ease. No Postgres, no SQL Server, no running process. However, if your use cases require data to be centralized in a server or cloud data platform (e.g. Snowflake), comma-seperated value outputs are supported throughout reserve.

- **Pipeline-ready for large data environments.** `reserve` speaks JSONL on stdin/stdout — the lingua franca of Unix data pipelines. Chain transforms and analyses with `|`, redirect to files, or feed downstream tools. Every operator is NaN-aware and handles FRED's missing-value conventions correctly at scale. CSV formating is also supported for importing data into other data stores or spreadsheets.

- **LLM and agentic workflow ready.** JSONL is the native input format for modern AI pipelines. Pipe `reserve` output directly into LLM tool-call chains, vector embedding workflows, or agentic analysis frameworks — economic time series, transformed and structured, exactly where your model expects it.

- **Built-in rate limiting and retry logic.** The API client enforces a configurable token-bucket rate limiter and exponential backoff on transient failures — the right defaults for shared financial data environments where API quotas matter.

- **Idiomatic Go semantics throughout.** Structured logging via `slog`, context cancellation on every HTTP call, bounded concurrency with `sync.WaitGroup` and semaphores, deterministic output ordering, and clean separation between packages. The codebase is readable, testable, and auditable.

- **Small, fast, and self-contained.** The compiled binary is under 15 MB. Cold start is measured in milliseconds. Analysis on years of monthly data runs in memory without paging. The right tool for automated pipelines, cron jobs, and production data workflows — not just interactive exploration.

---

## Install

```bash
git clone https://github.com/derickschaefer/reserve
cd reserve
go build -o reserve .
```

Or install directly:

```bash
go install github.com/derickschaefer/reserve@latest
```

Requires Go 1.21+.

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
reserve store list
```

**5. Run the analysis pipeline**

```bash
# Quarter-over-quarter GDP growth with summary statistics
reserve store get GDP --format jsonl | reserve transform pct-change | reserve analyze summary

# Long-run unemployment trend
reserve store get UNRATE --format jsonl | reserve analyze trend

# Annual CPI averages
reserve store get CPIAUCSL --format jsonl | reserve transform resample --freq annual --method mean
```

---

## Design Philosophy

`reserve` operates in two distinct modes:

**Live mode** — Discovery and retrieval commands (`series`, `obs`, `category`, `search`, etc.) hit the FRED API directly. No caching layer, always fresh data.

**Analysis mode** — You explicitly accumulate data into a local [bbolt](https://github.com/etcd-io/bbolt) database using `fetch --store`. Transform and analyze commands operate on that local dataset, making analysis fast, reproducible, and offline-capable.

The pipeline is Unix-native. Commands that produce observations write JSONL to stdout; transform and analyze commands read JSONL from stdin. Chain them with `|`. When stdout is a terminal, output defaults to a formatted table. When piped, it defaults to JSONL.

---

## The Command Model

`reserve` uses a pragmatic command model that is worth understanding before you explore the full command reference.

Most commands follow a **noun-verb** pattern: the top-level command names a resource, and its subcommands are operations on that resource. This maps naturally onto the structure of the FRED API and the local data store.
```
reserve series get UNRATE            # noun: series  / verb: get
reserve category tree root           # noun: category / verb: tree
reserve release list                 # noun: release  / verb: list
reserve store get CPIAUCSL           # noun: store    / verb: get
reserve config set api_key XYZ      # noun: config   / verb: set
```

Two top-level commands are nouns by acronym rather than by entity type: `obs` (observations) and `llm` (LLM onboarding context). They follow the same noun-verb structure; the noun is just abbreviated.
```
reserve obs get UNRATE --start 2020-01-01   # noun: obs (observations)
reserve llm --topic pipeline                # noun: llm (machine-readable context)
```

**Pipeline operators** — `transform`, `window`, `analyze`, and `chart` — are pure verbs. They have no resource noun because they do not target a named entity. They operate on whatever JSONL stream arrives on stdin. The data source is implicit, so there is nothing meaningful to name.
```
... | reserve transform pct-change --period 12
... | reserve window roll --stat mean --window 6
... | reserve analyze trend
... | reserve chart
```

Finally, two commands are standalone action verbs with no natural noun: `fetch` and `search`. `fetch` performs a batch accumulation across entity types; `search` performs a global full-text query across all FRED entity types. Neither belongs to a single resource, so no noun prefix applies.
```
reserve fetch series GDP UNRATE CPIAUCSL --store
reserve search "yield curve"
```

In summary:

| Class | Pattern | Examples |
|---|---|---|
| FRED API wrappers | noun verb | `series`, `category`, `release`, `source`, `tag`, `meta` |
| Local store operations | noun verb | `store`, `cache`, `config`, `snapshot` |
| Abbreviated nouns | noun verb | `obs`, `llm` |
| Pipeline operators | verb only | `transform`, `window`, `analyze`, `chart` |
| Cross-cutting actions | verb only | `fetch`, `search` |
| Utility | standalone | `version`, `completion`, `help` |

The noun-verb commands follow consistent flag conventions and produce the same `Result` envelope. The pipeline operators follow consistent stdin/stdout JSONL semantics. Within each class, behavior is uniform and predictable.

## Command Reference

### series

Discover and inspect FRED data series.

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

Fetch time series observations live from the FRED API.

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
--limit N            max observations (0 = all)
```

Units reference: `lin` = levels, `pch` = % change, `pc1` = % change from year ago, `log` = natural log.

Examples:

```bash
reserve obs get UNRATE --start 2020-01-01 --end 2024-12-31
reserve obs get CPIAUCSL --freq monthly --units pc1    # year-over-year % change
reserve obs get GDP CPIAUCSL --format csv --out data.csv
reserve obs latest GDP UNRATE CPIAUCSL FEDFUNDS
```

---

### category

Browse the FRED category hierarchy.

```bash
reserve category get <CATEGORY_ID>
reserve category ls <CATEGORY_ID|root>
reserve category tree <CATEGORY_ID|root> [--depth N]
reserve category series <CATEGORY_ID> [--limit N]
```

Examples:

```bash
reserve category ls root               # top-level categories
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

Global full-text search across all FRED entity types.

```bash
reserve search "<query>" [--type series|category|release|tag|source|all] [--limit N]
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

Fetch data from the FRED API and accumulate it in the local database.

```bash
reserve fetch series <SERIES_ID...> [--start YYYY-MM-DD] [--end YYYY-MM-DD] --store
```

Flags:

```
--store              write fetched observations to the local database
--start YYYY-MM-DD   start date for observations
--end   YYYY-MM-DD   end date for observations
```

Examples:

```bash
# Build a local dataset with four core macro series from 2010 onward
reserve fetch series GDP CPIAUCSL UNRATE FEDFUNDS --start 2010-01-01 --store

# Update an existing series with the latest data
reserve fetch series GDP --start 2010-01-01 --store
```

Data is stored in `~/.reserve/reserve.db` by default (override with `db_path` in `config.json` or the `RESERVE_DB_PATH` environment variable). You own the data — there is no automatic expiry on fetched observations.

---

### store

Inspect data you have accumulated locally.

```bash
reserve store list                     # all series in the database
reserve store get <SERIES_ID>          # read stored observations as a table
reserve store get GDP --format jsonl   # emit as JSONL for pipeline input
reserve store get CPIAUCSL --format csv --out cpi.csv
```

`store get` supports all output formats and is the primary data source for the analysis pipeline.

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
reserve store get GDP --format jsonl | reserve transform pct-change

# Year-over-year CPI inflation (monthly data)
reserve store get CPIAUCSL --format jsonl | reserve transform pct-change --period 12

# Index GDP to 100 at the start of 2010
reserve store get GDP --format jsonl | reserve transform index --base 100 --at 2010-01-01

# Annual average CPI
reserve store get CPIAUCSL --format jsonl | reserve transform resample --freq annual --method mean

# Post-2020 observations only
reserve store get UNRATE --format jsonl | reserve transform filter --after 2020-01-01
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
reserve store get UNRATE --format jsonl | reserve window roll --stat mean --window 12

# 4-quarter rolling standard deviation of GDP growth
reserve store get GDP --format jsonl | reserve transform pct-change \
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
reserve store get UNRATE --format jsonl | reserve analyze summary
reserve store get GDP --format jsonl | reserve transform pct-change | reserve analyze summary
reserve store get UNRATE --format jsonl | reserve analyze trend
reserve store get UNRATE --format jsonl | reserve analyze trend --method theil-sen
```

---

### cache

Manage the local bbolt database.

```bash
reserve cache stats                         # bucket row counts and DB size
reserve cache clear --all                   # wipe all data
reserve cache clear --bucket obs            # wipe observations only
reserve cache clear --bucket series_meta    # wipe metadata only
reserve cache compact                       # reclaim disk space after clearing
```

`cache clear` removes entries from one bucket or all buckets. bbolt does not shrink the database file automatically — freed pages are returned to an internal freelist and reused on future writes. The file footprint does not decrease until you run `compact`.

`cache compact` rewrites the database to a new file, recovering all space freed by prior clears. The operation is safe: live data is copied to a temporary file first, then the original is atomically replaced.

```bash
# Typical maintenance workflow after a large clear:
reserve cache clear --all
reserve cache compact
```

---

### snapshot

Save and replay exact command lines for reproducible workflows.

```bash
reserve snapshot save --name <name> --cmd "<command>"
reserve snapshot list
reserve snapshot show <ID>
reserve snapshot run <ID>
reserve snapshot delete <ID>
```

Snapshot IDs are ULIDs — lexicographically sortable and collision-resistant.

Examples:

```bash
reserve snapshot save --name "gdp-qoq" \
  --cmd "store get GDP --format jsonl | transform pct-change | analyze summary"

reserve snapshot list
reserve snapshot run 01JABCDEF0000000000000000
```

---

### config

Manage `config.json` in the current working directory.

```bash
reserve config init                    # create a template config.json
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
reserve v1.0.5
go      go1.25.5
os      linux/amd64
built   2026-02-16T18:42:00Z
```

---

### llm

Emit a machine-readable context document for LLM onboarding. Designed to be
pasted directly into a Claude, ChatGPT, or any LLM session to give the AI
authoritative knowledge of reserve's commands, pipeline semantics, data model,
verified examples, and known gotchas — without requiring the LLM to crawl
documentation or guess at flag names.

```bash
reserve llm                              # table of contents — start here
reserve llm --topic pipeline             # single topic
reserve llm --topic pipeline,gotchas     # comma-separated topics
reserve llm --topic all                  # full document (large context windows)
reserve llm --topic all | pbcopy         # copy to clipboard
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
reserve llm --topic toc
# paste into your LLM session

# Step 2 — surgical context: paste only what the AI requested
reserve llm --topic pipeline,data-model,gotchas
# paste → LLM confirms ready

# Step 3 — ask your question
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
# Full macro pipeline: fetch → transform → analyze
reserve store get GDP --format jsonl \
  | reserve transform pct-change \
  | reserve analyze summary

# Post-COVID unemployment: filter → rolling average
reserve store get UNRATE --format jsonl \
  | reserve transform filter --after 2020-01-01 \
  | reserve window roll --stat mean --window 12

# Inflation signal: resample monthly CPI to annual → trend
reserve store get CPIAUCSL --format jsonl \
  | reserve transform resample --freq annual --method mean \
  | reserve analyze trend

# Year-over-year unemployment change → CSV file
reserve store get UNRATE --format jsonl \
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
reserve store get CPIAUCSL --format jsonl --out cpi.jsonl
```

---

## Global Flags

These flags apply to every command:

```
--format table|json|jsonl|csv|tsv|md    output format
--out <path>                            write output to file instead of stdout
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

`config.json` in the working directory (created by `reserve config init`):

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
3. `api_key` in `config.json`

**Database path resolution order:**

1. `RESERVE_DB_PATH` environment variable
2. `db_path` in `config.json`
3. Default: `~/.reserve/reserve.db`

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for the full version history.

---

## License

MIT — see [LICENSE](LICENSE).

FRED® is a registered trademark of the Federal Reserve Bank of St. Louis.  
This project is not affiliated with or endorsed by the Federal Reserve Bank of St. Louis.
