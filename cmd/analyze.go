// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"github.com/derickschaefer/reserve/internal/analyze"
	"github.com/derickschaefer/reserve/internal/pipeline"
	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze a time series (reads JSONL from stdin)",
	Long: `Analyze operators read JSONL observations from stdin and print results.

Examples:
  reserve obs get GDP --from cache --format jsonl | reserve analyze summary
  reserve obs get UNRATE --from cache --format jsonl | reserve transform pct-change | reserve analyze trend`,
}

// ─── analyze summary ─────────────────────────────────────────────────────────

var analyzeSummaryBySeries bool

var analyzeSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Descriptive statistics: count, mean, std, min, max, median, skew",
	Example: `  reserve obs get GDP --from cache --format jsonl | reserve analyze summary
  reserve obs get UNRATE --from cache --format jsonl | reserve transform pct-change | reserve analyze summary
  reserve obs get FEDFUNDS T10Y2Y UNRATE --format jsonl | reserve analyze summary --by-series`,
	RunE: func(cmd *cobra.Command, args []string) error {
		format := resolveFormat("")
		w, closeFn, err := outputWriter(cmd.OutOrStdout())
		if err != nil {
			return err
		}
		defer closeFn()

		if analyzeSummaryBySeries {
			groups, err := pipeline.ReadObservationGroups(os.Stdin)
			if err != nil {
				return err
			}
			summaries := make([]analyze.Summary, 0, len(groups))
			for _, group := range groups {
				summaries = append(summaries, analyze.Summarize(group.SeriesID, group.Obs))
			}
			return renderSummaryBatch(w, format, summaries)
		}

		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}

		s := analyze.Summarize(seriesID, obs)
		return renderSummarySingle(w, format, s)
	},
}

// ─── analyze trend ────────────────────────────────────────────────────────────

var analyzeTrendMethod string

var analyzeTrendCmd = &cobra.Command{
	Use:   "trend",
	Short: "Fit a linear trend: slope, intercept, R², direction",
	Example: `  reserve obs get GDP --from cache --format jsonl | reserve analyze trend
  reserve obs get UNRATE --from cache --format jsonl | reserve analyze trend --method theil-sen`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}

		tr, err := analyze.Trend(seriesID, obs, analyze.TrendMethod(analyzeTrendMethod))
		if err != nil {
			return err
		}

		format := resolveFormat("")
		w, closeFn, err := outputWriter(cmd.OutOrStdout())
		if err != nil {
			return err
		}
		defer closeFn()
		if format == "json" {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(tr)
		}

		rows := [][]string{
			{"series_id", tr.SeriesID},
			{"method", string(tr.Method)},
			{"direction", tr.Direction},
			{"slope_per_day", fmt.Sprintf("%.6f", tr.Slope)},
			{"slope_per_year", fmt.Sprintf("%.4f", tr.SlopePerYear)},
			{"intercept", fmt.Sprintf("%.4f", tr.Intercept)},
			{"r2", fmt.Sprintf("%.4f", tr.R2)},
		}
		printKVTableTo(w, rows)
		return nil
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.AddCommand(analyzeSummaryCmd)
	analyzeCmd.AddCommand(analyzeTrendCmd)

	analyzeSummaryCmd.Flags().BoolVar(&analyzeSummaryBySeries, "by-series", false,
		"group multi-series JSONL input by series_id and emit one summary per series")
	analyzeTrendCmd.Flags().StringVar(&analyzeTrendMethod, "method", "linear",
		"regression method: linear|theil-sen")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func renderSummarySingle(w io.Writer, format string, s analyze.Summary) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}
	if format == "jsonl" {
		return json.NewEncoder(w).Encode(s)
	}

	rows := [][]string{
		{"series_id", s.SeriesID},
		{"count", fmt.Sprintf("%d", s.Count)},
		{"missing", fmt.Sprintf("%d (%.1f%%)", s.Missing, s.MissingPct)},
		{"mean", fmtStat(s.Mean)},
		{"std", fmtStat(s.Std)},
		{"min", fmtStat(s.Min)},
		{"p25", fmtStat(s.P25)},
		{"median", fmtStat(s.Median)},
		{"p75", fmtStat(s.P75)},
		{"max", fmtStat(s.Max)},
		{"skew", fmtStat(s.Skew)},
		{"first", fmtStat(s.First)},
		{"last", fmtStat(s.Last)},
		{"change", fmtStat(s.Change)},
		{"change_pct", fmtStatPct(s.ChangePct)},
	}
	printKVTableTo(w, rows)
	return nil
}

func renderSummaryBatch(w io.Writer, format string, summaries []analyze.Summary) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(summaries)
	case "jsonl":
		enc := json.NewEncoder(w)
		for _, s := range summaries {
			if err := enc.Encode(s); err != nil {
				return err
			}
		}
		return nil
	default:
		sorted := append([]analyze.Summary(nil), summaries...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].SeriesID < sorted[j].SeriesID })
		printSimpleTable(w, []string{"SERIES", "COUNT", "MISSING", "MEAN", "STD", "MIN", "MEDIAN", "MAX", "CHANGE_PCT"}, func(add func(...string)) {
			for _, s := range sorted {
				add(
					s.SeriesID,
					fmt.Sprintf("%d", s.Count),
					fmt.Sprintf("%d (%.1f%%)", s.Missing, s.MissingPct),
					fmtStat(s.Mean),
					fmtStat(s.Std),
					fmtStat(s.Min),
					fmtStat(s.Median),
					fmtStat(s.Max),
					fmtStatPct(s.ChangePct),
				)
			}
		})
		return nil
	}
}

func fmtStat(v float64) string {
	if math.IsNaN(v) {
		return "."
	}
	return fmt.Sprintf("%.4f", v)
}

func fmtStatPct(v float64) string {
	if math.IsNaN(v) {
		return "."
	}
	return fmt.Sprintf("%.2f%%", v)
}
