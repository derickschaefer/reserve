package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/derickschaefer/reserve/internal/analyze"
	"github.com/derickschaefer/reserve/internal/pipeline"
	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze a time series (reads JSONL from stdin)",
	Long: `Analyze operators read JSONL observations from stdin and print results.

Examples:
  reserve store get GDP --format jsonl | reserve analyze summary
  reserve store get UNRATE --format jsonl | reserve transform pct-change | reserve analyze trend`,
}

// ─── analyze summary ─────────────────────────────────────────────────────────

var analyzeSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Descriptive statistics: count, mean, std, min, max, median, skew",
	Example: `  reserve store get GDP --format jsonl | reserve analyze summary
  reserve store get UNRATE --format jsonl | reserve transform pct-change | reserve analyze summary`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}

		s := analyze.Summarize(seriesID, obs)

		format := resolveFormat("")
		w, closeFn, err := outputWriter(cmd.OutOrStdout())
		if err != nil {
			return err
		}
		defer closeFn()
		if format == "json" {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(s)
		}

		// Table output
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
	},
}

// ─── analyze trend ────────────────────────────────────────────────────────────

var analyzeTrendMethod string

var analyzeTrendCmd = &cobra.Command{
	Use:   "trend",
	Short: "Fit a linear trend: slope, intercept, R², direction",
	Example: `  reserve store get GDP --format jsonl | reserve analyze trend
  reserve store get UNRATE --format jsonl | reserve analyze trend --method theil-sen`,
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

	analyzeTrendCmd.Flags().StringVar(&analyzeTrendMethod, "method", "linear",
		"regression method: linear|theil-sen")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

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
