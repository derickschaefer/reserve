package cmd

// cmd/llm.go — machine-readable context document for LLM onboarding.
//
// Usage:
//   reserve llm                          # start bundle — paste into your LLM session
//   reserve llm --topic start            # same as bare reserve llm
//   reserve llm --topic toc              # table of contents / two-step handshake
//   reserve llm --topic pipeline         # stdin/stdout semantics
//   reserve llm --topic commands         # full command reference
//   reserve llm --topic data-model       # types, NaN, Result envelope
//   reserve llm --topic examples         # verified end-to-end examples
//   reserve llm --topic gotchas          # sharp edges and known gaps
//   reserve llm --topic version          # build metadata
//   reserve llm --topic toc,pipeline     # comma-separated multi-topic
//   reserve llm --topic all              # everything (large context)
//
// LLM onboarding workflow:
//   1. reserve llm                       (paste output → LLM is ready immediately)
//   3. Ask your macroeconomics question.
//
// Two-step handshake (token-conservative):
//   1. reserve llm --topic toc           (paste → LLM requests topics it needs)
//   2. reserve llm --topic <requested>   (paste → LLM says ready)
//   3. Ask your macroeconomics question.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// ─── Topic registry ───────────────────────────────────────────────────────────

type llmTopic struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var topicRegistry = []llmTopic{
	{"start", "Curated onboarding bundle: commands + pipeline + gotchas + examples. One command, ready to work."},
	{"toc", "Topic index and LLM interaction guide. Use for the two-step handshake pattern."},
	{"commands", "Full command reference: all nouns, verbs, flags, output formats."},
	{"pipeline", "stdin/stdout semantics, JSONL format, operator chaining, format requirements."},
	{"data-model", "Core types: Observation, SeriesData, Result envelope, NaN conventions."},
	{"examples", "Verified end-to-end examples with real FRED series and confirmed output."},
	{"gotchas", "Sharp edges, missing data handling, multi-series limitations, known gaps."},
	{"version", "Build metadata, Go version, platform. For provenance and reproducibility."},
}

// ─── Command ──────────────────────────────────────────────────────────────────

var llmTopicFlag string

var llmCmd = &cobra.Command{
	Use:   "llm",
	Short: "Emit a machine-readable context document for LLM onboarding",
	Long: `Emit a structured JSON document describing reserve's commands, pipeline
semantics, verified examples, and known gotchas — formatted for efficient
LLM context window ingestion.

Bare 'reserve llm' emits the curated start bundle — the minimum context an
LLM needs to work confidently with reserve in a single command. Paste the
output into your LLM session and start asking questions.

Two-step handshake pattern (token-conservative):
  1. reserve llm --topic toc
     Paste into your LLM session. The LLM identifies which topics it needs.
  2. reserve llm --topic <requested topics>
     Paste the targeted output. The LLM confirms it is ready.
  3. Ask your question.

For large context windows:
  reserve llm --topic all

Topics:
  start       Curated onboarding bundle (default) — commands, pipeline, gotchas, examples
  toc         Topic index and interaction guide — use for the two-step handshake
  commands    Full command reference
  pipeline    stdin/stdout and JSONL semantics
  data-model  Types, NaN handling, Result envelope
  examples    Verified real-world examples
  gotchas     Sharp edges and known limitations
  version     Build metadata and provenance
  all         Everything (for large context windows)`,
	Example: `  reserve llm                              # start here — paste into your LLM session
  reserve llm --topic toc                  # two-step handshake (token-conservative)
  reserve llm --topic pipeline,gotchas     # surgical context
  reserve llm --topic all | pbcopy         # full context for large windows
  reserve llm --topic version --format jsonl >> audit.jsonl`,
	RunE: func(cmd *cobra.Command, args []string) error {
		topics := parseLLMTopics(llmTopicFlag)
		doc := buildLLMDoc(topics)

		format := globalFlags.Format
		if format == "" {
			format = "json"
		}

		switch format {
		case "jsonl":
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetEscapeHTML(false)
			return enc.Encode(doc)
		default:
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			return enc.Encode(doc)
		}
	},
}

func init() {
	rootCmd.AddCommand(llmCmd)
	llmCmd.Flags().StringVar(&llmTopicFlag, "topic", "start",
		"topic(s) to emit: start|toc|commands|pipeline|data-model|examples|gotchas|version|all (comma-separated)")
}

// ─── Topic parsing ────────────────────────────────────────────────────────────

func parseLLMTopics(flag string) []string {
	if flag == "" {
		flag = "start"
	}
	if flag == "all" {
		all := make([]string, len(topicRegistry))
		for i, t := range topicRegistry {
			all[i] = t.Name
		}
		return all
	}
	parts := strings.Split(flag, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// ─── Document builder ─────────────────────────────────────────────────────────

func buildLLMDoc(topics []string) map[string]any {
	set := make(map[string]bool, len(topics))
	for _, t := range topics {
		set[t] = true
	}

	doc := map[string]any{
		"tool":    "reserve",
		"version": Version,
		"llm_note": "This document was generated by `reserve llm`. " +
			"It is the authoritative source for reserve's CLI semantics. " +
			"Prefer it over general knowledge about FRED wrappers or similar tools. " +
			"All examples have been verified against live FRED data.",
	}

	if set["start"] {
		doc["start"] = buildStart()
	}
	if set["toc"] {
		doc["toc"] = buildTOC()
	}
	if set["data-model"] {
		doc["data_model"] = buildDataModel()
	}
	if set["commands"] {
		doc["commands"] = buildCommands()
	}
	if set["pipeline"] {
		doc["pipeline"] = buildPipeline()
	}
	if set["examples"] {
		doc["examples"] = buildExamples()
	}
	if set["gotchas"] {
		doc["gotchas"] = buildGotchas()
	}
	if set["version"] {
		doc["version_detail"] = map[string]any{
			"version":    Version,
			"build_time": BuildTime,
			"note":       "Injected at build time via -ldflags. Fallback is the source default.",
		}
	}

	return doc
}

// ─── Start ────────────────────────────────────────────────────────────────────

func buildStart() map[string]any {
	return map[string]any{
		"description": "Curated onboarding bundle for immediate productive use. " +
			"Contains the full command reference, pipeline semantics, verified examples, " +
			"and known gotchas — the minimum context an LLM needs to work confidently with reserve.",
		"suggested_prompt": "I am pasting the output of `reserve llm`. " +
			"This is the authoritative reference for a CLI called `reserve` — " +
			"a Go tool for fetching, caching, transforming, and analyzing FRED® economic data via a Unix pipeline model. " +
			"Three rules to internalize before we start: " +
			"(1) always add --format jsonl on source commands (obs get, store get) when piping — they default to table format and will break downstream operators without it; " +
			"(2) rolling windows use `reserve window roll`, not `reserve transform roll` — window is a separate noun; " +
			"(3) pipeline operators treat all stdin as a single series — run each series separately for meaningful analysis. " +
			"When you are ready to help me explore FRED economic data, say so.",
		"commands": buildCommands(),
		"pipeline": buildPipeline(),
		"gotchas":  buildGotchas(),
		"examples": buildExamples(),
	}
}

// ─── TOC ──────────────────────────────────────────────────────────────────────

func buildTOC() map[string]any {
	topics := make([]map[string]any, len(topicRegistry))
	for i, t := range topicRegistry {
		topics[i] = map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"fetch":       fmt.Sprintf("reserve llm --topic %s", t.Name),
		}
	}
	return map[string]any{
		"description": "reserve is a Go CLI for the FRED® economic data API. " +
			"It fetches, caches, transforms, and analyzes time series via a Unix pipeline model. " +
			"Every command reads/writes a uniform Result envelope. " +
			"Pipeline operators communicate via JSONL on stdin/stdout.",
		"topics":       topics,
		"quick_start":  "reserve llm  — emits the curated start bundle; paste into your LLM session and begin",
		"multi_topic":  "reserve llm --topic pipeline,gotchas",
		"full_context": "reserve llm --topic all",
		"prompt_template": "I am pasting the output of `reserve llm --topic <topics>`. " +
			"This is the authoritative reference for a CLI called reserve. " +
			"Use it to answer my questions about fetching and analyzing FRED economic data. " +
			"Tell me when you are ready.",
	}
}

// ─── Data model ───────────────────────────────────────────────────────────────

func buildDataModel() map[string]any {
	return map[string]any{
		"observation": map[string]any{
			"type": "Observation",
			"fields": map[string]any{
				"date":           "time.Time — always first of month for monthly series",
				"value":          "float64 — math.NaN() when the data point is missing",
				"value_raw":      "string — original API string, e.g. '.' for missing",
				"realtime_start": "string — optional, FRED vintage date",
				"realtime_end":   "string — optional, FRED vintage date",
			},
			"nan_convention": "FRED encodes missing values as '.' or empty string. reserve converts these to math.NaN() on ingest and represents them as JSON null in JSONL output. Never interpolated. Never zero-filled.",
		},
		"series_data": map[string]any{
			"type": "SeriesData",
			"fields": map[string]any{
				"series_id":    "string — FRED series identifier e.g. 'CPIAUCSL'",
				"meta":         "*SeriesMeta — optional, attached when fetched with metadata",
				"observations": "[]Observation",
			},
		},
		"series_meta": map[string]any{
			"type": "SeriesMeta",
			"fields": map[string]any{
				"id":           "FRED series ID",
				"title":        "human-readable name",
				"units":        "full units string e.g. 'Index 1982-84=100'",
				"frequency":    "e.g. 'Monthly', 'Quarterly', 'Annual'",
				"seasonal_adj": "e.g. 'Seasonally Adjusted'",
				"last_updated": "date of most recent data revision",
				"popularity":   "integer 0-100, FRED's own popularity score",
				"notes":        "long-form description, may contain HTML",
			},
		},
		"result_envelope": map[string]any{
			"type": "Result",
			"fields": map[string]any{
				"kind":         "string constant e.g. 'series_data', 'series_meta'",
				"generated_at": "time.Time — when the command ran",
				"command":      "string — the command that produced this result",
				"data":         "any — typed payload; kind identifies what is inside",
				"warnings":     "[]string — non-fatal issues e.g. one series in a batch failed",
				"stats": map[string]any{
					"cache_hit":   "bool — true if data came from local bbolt cache",
					"duration_ms": "int64 — wall time in milliseconds",
					"items":       "int — number of observations or series returned",
				},
			},
			"note": "All commands return this envelope with --format json. Pipeline operators (transform, window, analyze) emit plain JSONL rows, not the full envelope.",
		},
		"jsonl_row": map[string]any{
			"description": "One line of JSONL emitted by pipeline operators",
			"schema": map[string]any{
				"series_id": "string",
				"date":      "YYYY-MM-DD",
				"value":     "float64 or null (null = missing/NaN)",
				"value_raw": "string — original value string",
			},
			"example": `{"series_id":"CPIAUCSL","date":"2024-01-01","value":308.417,"value_raw":"308.417"}`,
		},
	}
}

// ─── Commands ─────────────────────────────────────────────────────────────────

func buildCommands() map[string]any {
	return map[string]any{
		"global_flags": map[string]any{
			"--format":      "table|json|jsonl|csv|tsv|md  (default: table for terminal, jsonl when piped)",
			"--out":         "write output to file instead of stdout",
			"--api-key":     "FRED API key override (also: FRED_API_KEY env, config.json)",
			"--timeout":     "HTTP request timeout e.g. 30s, 2m  (default: 30s)",
			"--concurrency": "max parallel requests for batch operations  (default: 8)",
			"--rate":        "API requests/sec client-side limit  (default: 5.0)",
			"--verbose":     "show timing and cache stats after output",
			"--debug":       "log HTTP requests with API key redacted",
			"--quiet":       "suppress all non-error output",
			"--no-cache":    "bypass local database reads",
			"--refresh":     "force re-fetch and overwrite cached entries",
		},
		"nouns": map[string]any{
			"obs": map[string]any{
				"description": "Fetch live observations from the FRED API",
				"verbs": map[string]any{
					"get": map[string]any{
						"usage":   "reserve obs get <SERIES_ID...>",
						"flags":   "--start YYYY-MM-DD  --end YYYY-MM-DD  --freq M|Q|A  --units lin|chg|ch1|pch|pc1|pca|cch|cca|log  --agg avg|sum|eop  --limit N",
						"note":    "CRITICAL: defaults to table format. Always add --format jsonl when piping to transform/window/analyze.",
						"example": "reserve obs get CPIAUCSL --start 2020-01-01 --format jsonl",
					},
					"latest": map[string]any{
						"usage":   "reserve obs latest <SERIES_ID...>",
						"example": "reserve obs latest FEDFUNDS UNRATE",
					},
				},
			},
			"series": map[string]any{
				"description": "Discover and inspect FRED data series",
				"verbs": map[string]any{
					"get":        "reserve series get <SERIES_ID...>  — fetch metadata",
					"search":     "reserve series search \"<query>\" [--limit N]",
					"tags":       "reserve series tags <SERIES_ID>",
					"categories": "reserve series categories <SERIES_ID>",
					"related":    "reserve series related <SERIES_ID>",
					"describe":   "reserve series describe <SERIES_ID>  — metadata + recent obs",
				},
			},
			"store": map[string]any{
				"description": "Read from local bbolt cache (populated by fetch)",
				"verbs": map[string]any{
					"get":  "reserve store get <SERIES_ID> [--format jsonl]  — primary pipeline source",
					"list": "reserve store list  — show all cached series",
				},
				"note": "store get is preferred over obs get in pipelines — no API call, no rate limit.",
			},
			"fetch": map[string]any{
				"description": "Pull data from FRED API and persist to local cache",
				"verbs": map[string]any{
					"series":   "reserve fetch series <SERIES_ID...> [--store]",
					"category": "reserve fetch category <CATEGORY_ID>",
					"query":    "reserve fetch query \"<search>\" [--limit N]",
				},
			},
			"transform": map[string]any{
				"description": "Stateless pipeline operators — read JSONL from stdin, emit JSONL to stdout",
				"verbs": map[string]any{
					"pct-change": "reserve transform pct-change [--period N]  default period=1 (MoM). Use --period 12 for YoY on monthly data.",
					"diff":       "reserve transform diff [--order 1|2]  first or second difference",
					"log":        "reserve transform log  natural log; non-positive values → NaN with warning",
					"index":      "reserve transform index --base 100 --at YYYY-MM-DD  rescale so anchor date = base",
					"normalize":  "reserve transform normalize [--method zscore|minmax]",
					"resample":   "reserve transform resample --freq monthly|quarterly|annual --method mean|last|sum",
					"filter":     "reserve transform filter [--after YYYY-MM-DD] [--before YYYY-MM-DD] [--min N] [--max N] [--drop-missing]",
				},
			},
			"window": map[string]any{
				"description": "Rolling window statistics — separate noun from transform",
				"critical":    "This is `reserve window roll`, NOT `reserve transform roll`. A common mistake.",
				"verbs": map[string]any{
					"roll": "reserve window roll --stat mean|std|min|max|sum --window N [--min-periods M]",
				},
				"nan_behavior": "NaN values are skipped in window computation. If fewer than --min-periods valid values exist in a window, output is NaN.",
			},
			"analyze": map[string]any{
				"description": "Statistical analysis — reads JSONL from stdin, prints results to terminal",
				"verbs": map[string]any{
					"summary": map[string]any{
						"usage":  "reserve analyze summary",
						"output": "series_id, count, missing (count + pct), mean, std, min, p25, median, p75, max, skew, first, last, change, change_pct",
					},
					"trend": map[string]any{
						"usage":  "reserve analyze trend [--method linear|theil-sen]",
						"output": "series_id, method, direction (up|down|flat), slope_per_day, slope_per_year, intercept, r2",
						"note":   "theil-sen is robust to outliers; use for series with COVID spikes or structural breaks.",
					},
				},
			},
			"cache": map[string]any{
				"description": "Manage local bbolt database",
				"verbs": map[string]any{
					"stats":   "reserve cache stats  — bucket row counts and DB file size",
					"clear":   "reserve cache clear --all  |  --bucket obs|series_meta",
					"compact": "reserve cache compact  — rewrite DB to reclaim freed space",
				},
			},
			"snapshot": map[string]any{
				"description": "Save and replay exact command lines for reproducible workflows",
				"verbs": map[string]any{
					"save":   "reserve snapshot save --name <n> --cmd \"<command>\"",
					"list":   "reserve snapshot list",
					"show":   "reserve snapshot show <ULID>",
					"run":    "reserve snapshot run <ULID>",
					"delete": "reserve snapshot delete <ULID>",
				},
				"note": "Snapshot IDs are ULIDs — lexicographically sortable and collision-resistant.",
			},
			"config": map[string]any{
				"verbs": map[string]any{
					"init": "reserve config init  — create config.json template",
					"get":  "reserve config get [--show-secrets]",
					"set":  "reserve config set <key> <value>",
				},
				"resolution_order": []string{
					"1. --api-key CLI flag (highest priority)",
					"2. FRED_API_KEY environment variable",
					"3. api_key in config.json",
				},
				"db_path_resolution": []string{
					"1. RESERVE_DB_PATH environment variable",
					"2. db_path in config.json",
					"3. ~/.reserve/reserve.db (default)",
				},
			},
			"meta": map[string]any{
				"description": "Batch metadata retrieval for series, categories, releases, sources, tags",
				"verbs":       []string{"meta series", "meta category", "meta release", "meta source", "meta tag"},
			},
			"category": map[string]any{
				"verbs": []string{"category get", "category list", "category tree", "category series"},
			},
			"release": map[string]any{
				"verbs": []string{"release list", "release get", "release dates", "release series"},
			},
			"source": map[string]any{
				"verbs": []string{"source list", "source get", "source releases"},
			},
			"tag": map[string]any{
				"verbs": []string{"tag search", "tag series", "tag related"},
			},
			"search": map[string]any{
				"usage": "reserve search \"<query>\" [--limit N]  — global full-text search across series",
			},
			"version": map[string]any{
				"usage":   "reserve version [--format json|jsonl]",
				"example": "reserve version --format jsonl >> audit.jsonl",
			},
		},
	}
}

// ─── Pipeline ─────────────────────────────────────────────────────────────────

func buildPipeline() map[string]any {
	return map[string]any{
		"model":         "Unix stdin/stdout. Every pipeline operator reads JSONL from stdin and writes JSONL to stdout. analyze verbs are terminal — they consume JSONL and print a summary table or JSON, not JSONL.",
		"critical_rule": "obs get defaults to TABLE format even when piped. You MUST add --format jsonl on the source command or the downstream operator will fail with 'invalid character +' (table border characters are not JSON).",
		"jsonl_format": map[string]any{
			"one_object_per_line": true,
			"schema":              `{"series_id":"CPIAUCSL","date":"2024-01-01","value":308.417,"value_raw":"308.417"}`,
			"missing_value":       `{"series_id":"UNEMPLOY","date":"2025-10-01","value":null,"value_raw":"."}`,
		},
		"correct_pipeline_pattern": "reserve obs get CPIAUCSL --start 2020-01-01 --format jsonl | reserve transform pct-change --period 12 | reserve window roll --stat mean --window 3 | reserve analyze trend",
		"wrong_pipeline_pattern":   "reserve obs get CPIAUCSL --start 2020-01-01 | reserve transform pct-change  ← missing --format jsonl, will fail",
		"format_autodetection":     "transform and window commands auto-detect: if stdout is a terminal they emit table, if piped they emit jsonl. The SOURCE command (obs get, store get) does NOT auto-detect — it must be told explicitly.",
		"preferred_source":         "reserve store get is preferred over reserve obs get in pipelines. store get reads from the local bbolt cache — no API call, no rate limiting, no network dependency.",
		"operator_chain_anatomy": map[string]any{
			"source":    "obs get / store get  — emits JSONL",
			"transform": "transform pct-change / diff / log / index / normalize / resample / filter  — JSONL → JSONL",
			"window":    "window roll  — JSONL → JSONL  (note: separate noun, not under transform)",
			"terminal":  "analyze summary / analyze trend  — JSONL → table or JSON summary",
		},
		"multi_series_limitation": "Pipeline operators treat all JSONL on stdin as a single series. If you pipe obs get with two series IDs, the interleaved rows are treated as one stream. Run each series separately and compare results manually.",
		"store_get_pattern":       "reserve store get CPIAUCSL --format jsonl | reserve transform pct-change --period 12 | reserve analyze summary",
	}
}

// ─── Examples ─────────────────────────────────────────────────────────────────

func buildExamples() map[string]any {
	return map[string]any{
		"note": "All examples verified against live FRED data as of February 2026.",
		"verified_spot_checks": []map[string]any{
			{"series": "FEDFUNDS", "date": "2026-01", "value": 3.64, "source": "FRED web, Feb 2 2026"},
			{"series": "UNRATE", "date": "2020-04", "value": 14.8, "note": "Revised upward from initial 14.7 release — API returns current vintage"},
			{"series": "JTSJOL", "date": "2025-12", "value": 6542.0, "source": "FRED web, Feb 5 2026"},
			{"series": "UNEMPLOY", "date": "2026-01", "value": 7362.0, "source": "FRED web, Feb 11 2026"},
		},
		"examples": []map[string]any{
			{
				"name":        "CPI inflation trend",
				"description": "Year-over-year CPI inflation, 3-month smoothed, linear trend fit",
				"command":     "reserve obs get CPIAUCSL --start 2020-01-01 --format jsonl | reserve transform pct-change --period 12 | reserve window roll --stat mean --window 3 | reserve analyze trend",
				"output": map[string]any{
					"series_id":      "CPIAUCSL",
					"method":         "linear",
					"direction":      "down",
					"slope_per_day":  -0.002075,
					"slope_per_year": -0.7580,
					"intercept":      6.3370,
					"r2":             0.2699,
				},
				"interpretation": "Inflation trend is downward at -0.758 pp/year since 2020.",
			},
			{
				"name":        "Unemployment summary statistics",
				"description": "Post-COVID unemployment distribution with missing value handling",
				"command":     "reserve obs get UNRATE --start 2020-01-01 --format jsonl | reserve transform filter --drop-missing | reserve analyze summary",
				"output": map[string]any{
					"series_id":  "UNRATE",
					"count":      60,
					"missing":    0,
					"mean":       4.89,
					"std":        2.31,
					"min":        3.4,
					"max":        14.8,
					"median":     4.0,
					"change_pct": -63.5,
				},
			},
			{
				"name":        "Fed funds rate — latest reading",
				"description": "Single observation lookup for cross-checking",
				"command":     "reserve obs latest FEDFUNDS",
				"output":      `{"series_id":"FEDFUNDS","date":"2026-01-01","value":3.64,"value_raw":"3.64"}`,
			},
			{
				"name":        "Single observation lookup",
				"description": "Retrieve a specific data point for cross-checking",
				"command":     "reserve obs get UNRATE --start 2020-04-01 --end 2020-04-30 --format jsonl",
				"output":      `{"series_id":"UNRATE","date":"2020-04-01","value":14.8,"value_raw":"14.8"}`,
				"note":        "value_raw is the literal API string. If FRED web shows 14.7, the API has a revised vintage at 14.8.",
			},
		},
	}
}

// ─── Gotchas ──────────────────────────────────────────────────────────────────

func buildGotchas() map[string]any {
	return map[string]any{
		"critical": []map[string]any{
			{
				"id":      "format-jsonl-required",
				"title":   "Always --format jsonl on the source command when piping",
				"detail":  "obs get and store get default to table format regardless of whether stdout is a terminal. Table output starts with '+---' border characters which are not valid JSON. Downstream operators fail with 'invalid character +'. This is the single most common mistake.",
				"wrong":   "reserve obs get CPIAUCSL | reserve transform pct-change",
				"correct": "reserve obs get CPIAUCSL --format jsonl | reserve transform pct-change",
			},
			{
				"id":      "window-not-transform",
				"title":   "reserve window roll, not reserve transform roll",
				"detail":  "Rolling window statistics live under the 'window' noun, not 'transform'. There is no 'reserve transform roll' command. Using it produces 'unknown flag: --window'.",
				"wrong":   "reserve transform roll --window 3 --stat mean",
				"correct": "reserve window roll --window 3 --stat mean",
			},
			{
				"id":      "multi-series-pipeline",
				"title":   "Pipeline operators treat all stdin as one series",
				"detail":  "If you run 'reserve obs get UNRATE FEDFUNDS --format jsonl', the interleaved JSONL rows from both series are consumed as a single stream by downstream operators. For meaningful analysis, pipe each series separately.",
				"wrong":   "reserve obs get UNRATE FEDFUNDS --format jsonl | reserve analyze summary",
				"correct": "reserve obs get UNRATE --format jsonl | reserve analyze summary\nreserve obs get FEDFUNDS --format jsonl | reserve analyze summary",
			},
		},
		"data_quality": []map[string]any{
			{
				"id":     "nan-not-zero",
				"title":  "Missing values are NaN, not zero",
				"detail": "FRED encodes missing observations as '.' — reserve stores these as math.NaN() and emits them as JSON null. Transforms skip NaN inputs. Dividing a series that contains NaN will not produce zero — it propagates NaN. Use --drop-missing to exclude them explicitly.",
			},
			{
				"id":     "vintage-revisions",
				"title":  "API returns current vintage, not initial release",
				"detail": "FRED revises historical data. The API always returns the most recent revision. UNRATE April 2020 was initially published as 14.7% and later revised to 14.8%. reserve returns 14.8 because that is the current API value.",
			},
			{
				"id":     "government-shutdown-gap",
				"title":  "Government shutdown data gaps",
				"detail": "During US government shutdowns, BLS and Census data releases are delayed or cancelled. FRED reflects these gaps as missing observations. reserve will emit NaN/null for these periods — this is correct behavior, not a bug.",
			},
		},
		"pipeline_tips": []map[string]any{
			{
				"id":     "store-over-obs",
				"title":  "Prefer store get over obs get in pipelines",
				"detail": "reserve store get reads from local bbolt — no network, no rate limiting, instant. Use 'reserve fetch series <ID> --store' once to accumulate data, then build pipelines against the local store.",
			},
			{
				"id":     "analyze-is-terminal",
				"title":  "analyze verbs do not emit JSONL",
				"detail": "analyze summary and analyze trend consume the JSONL stream and print a formatted table or JSON summary. They are terminal operators — you cannot pipe their output into another reserve command.",
			},
		},
	}
}
