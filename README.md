# reserve

A command-line tool for exploring and retrieving economic data from the
Federal Reserve Bank of St. Louis FRED® API.

> **Note:** FRED® is a registered trademark of the Federal Reserve Bank of St. Louis.
> Data sourced from FRED®, Federal Reserve Bank of St. Louis; https://fred.stlouisfed.org/

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

## Quick Start

**1. Get a free API key**

Register at https://fred.stlouisfed.org/docs/api/api_key.html

**2. Configure**

```bash
reserve config init        # creates config.json in current directory
reserve config set api_key YOUR_KEY_HERE
```

Or set via environment variable:

```bash
export FRED_API_KEY=YOUR_KEY_HERE
```

**3. Search and explore**

```bash
reserve series search "consumer price index" --limit 5
reserve series get CPIAUCSL
reserve obs get CPIAUCSL --start 2020-01-01
reserve obs latest GDP UNRATE CPIAUCSL
```

## Commands

### series

```
reserve series get <SERIES_ID...>          Fetch series metadata
reserve series search "<query>"            Search series by keyword
reserve series tags <SERIES_ID>            List tags for a series
reserve series categories <SERIES_ID>      List categories for a series
```

### obs

```
reserve obs get <SERIES_ID...>             Fetch observations
  --start YYYY-MM-DD                       Start date filter
  --end   YYYY-MM-DD                       End date filter
  --freq  daily|weekly|monthly|quarterly|annual
  --units lin|chg|ch1|pch|pc1|pca|log
  --agg   avg|sum|eop
  --limit N

reserve obs latest <SERIES_ID...>          Most recent observation
```

### config

```
reserve config init                        Create template config.json
reserve config get [--show-secrets]        Print resolved configuration
reserve config set <key> <value>           Update a config value
```

## Global Flags

```
--format table|json|jsonl|csv|tsv|md    Output format (default: table)
--out <path>                            Write output to file
--api-key <key>                         Override API key
--timeout <duration>                    HTTP timeout (default: 30s)
--concurrency <n>                       Parallel requests (default: 8)
--rate <n>                              Requests/sec limit (default: 5.0)
--no-cache                              Bypass cache reads
--refresh                               Force re-fetch, overwrite cache
--verbose                               Show timing and cache stats
--debug                                 Log HTTP requests (key redacted)
--quiet                                 Suppress non-error output
```

## Output Formats

```bash
reserve obs get GDP --format json        Full result envelope as JSON
reserve obs get GDP --format jsonl       One observation per line (pipeline-friendly)
reserve obs get GDP --format csv         CSV with header row
reserve obs get GDP --format tsv         Tab-separated
reserve obs get GDP --format md          Markdown table
reserve obs get GDP --out data.csv --format csv   Write to file
```

## Configuration

`config.json` in the working directory (created by `reserve config init`):

```json
{
  "api_key": "YOUR_KEY_HERE",
  "default_format": "table",
  "timeout": "30s",
  "concurrency": 8,
  "rate": 5.0
}
```

**Resolution order** (first non-empty wins):

1. `--api-key` CLI flag
2. `FRED_API_KEY` environment variable
3. `config.json` in current directory

## Roadmap

See [DEVPLAN.md](DEVPLAN.md) for the full phased development plan.

- **Phase 1** ✓ — Scaffold, config, API client, series + obs commands
- **Phase 2** — Full subcommand tree (category, release, source, tag, fetch, meta)
- **Phase 3** — bbolt persistence and cache with TTL
- **Phase 4** — Transform and analysis pipeline
- **Phase 5** — Model, explain, export, report, email

## License

MIT — see [LICENSE](LICENSE).

FRED® is a registered trademark of the Federal Reserve Bank of St. Louis.
This project is not affiliated with or endorsed by the Federal Reserve Bank of St. Louis.
