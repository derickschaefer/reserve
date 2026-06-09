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
	"strings"

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
var analyzeSummaryWindow int

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
			if analyzeSummaryWindow > 0 {
				return fmt.Errorf("--window is not supported with --by-series")
			}
			groups, err := pipeline.ReadObservationGroups(os.Stdin)
			if err != nil {
				return err
			}
			summaries := make([]analyze.Summary, 0, len(groups))
			for _, group := range groups {
				s := analyze.Summarize(group.SeriesID, group.Obs)
				applyProvenanceToSummary(&s, group.Provenance)
				summaries = append(summaries, s)
			}
			return renderSummaryBatch(w, format, summaries)
		}

		seriesID, obs, prov, err := pipeline.ReadObservationsWithProvenance(os.Stdin)
		if err != nil {
			return err
		}

		s := analyze.Summarize(seriesID, obs)
		applyProvenanceToSummary(&s, prov)
		if analyzeSummaryWindow > 0 {
			windows := analyze.SummarizeWindows(seriesID, obs, analyzeSummaryWindow)
			if len(windows) == 0 {
				return fmt.Errorf("window=%d exceeds available observations (%d)", analyzeSummaryWindow, len(obs))
			}
			for i := range windows {
				applyProvenanceToSummary(&windows[i], prov)
			}
			return renderSummaryBatch(w, format, windows)
		}
		return renderSummarySingle(w, format, s)
	},
}

// ─── analyze trend ────────────────────────────────────────────────────────────

var analyzeTrendMethod string
var analyzeTrendConfidence bool
var analyzeCompareAgainst string
var analyzeCompareSeries string
var analyzeRegimeMethod string
var analyzeRegimeThreshold float64

var analyzeTrendCmd = &cobra.Command{
	Use:   "trend",
	Short: "Fit a linear trend: slope, intercept, R², direction",
	Example: `  reserve obs get GDP --from cache --format jsonl | reserve analyze trend
  reserve obs get UNRATE --from cache --format jsonl | reserve analyze trend --method theil-sen`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, prov, err := pipeline.ReadObservationsWithProvenance(os.Stdin)
		if err != nil {
			return err
		}

		tr, err := analyze.Trend(seriesID, obs, analyze.TrendMethod(analyzeTrendMethod))
		if err != nil {
			return err
		}
		applyProvenanceToTrend(&tr, prov)
		if analyzeTrendConfidence {
			tr.Confidence = analyze.AddTrendConfidence(tr, obs)
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
			{"Context", "-"},
			{"Series", tr.SeriesID},
			{"Method", string(tr.Method)},
			{"Trend", "-"},
			{"Direction", tr.Direction},
			{"Slope / Day", fmtFloatTable(tr.Slope, 6)},
			{"Slope / Year", fmtFloatTable(tr.SlopePerYear, 4)},
			{"Fit", "-"},
			{"Intercept", fmtFloatTable(tr.Intercept, 4)},
			{"R2", fmtFloatTable(tr.R2, 4)},
		}
		if analyzeTrendConfidence {
			rows = append(rows,
				[]string{"Confidence", "-"},
				[]string{"Slope StdErr", fmtConfidence(tr.Confidence, func(c *analyze.TrendConfidence) float64 { return c.SlopeStdErr }, 6)},
				[]string{"Slope P-Value", fmtConfidence(tr.Confidence, func(c *analyze.TrendConfidence) float64 { return c.SlopePValue }, 6)},
				[]string{"Slope CI95 Low", fmtConfidence(tr.Confidence, func(c *analyze.TrendConfidence) float64 { return c.SlopeCI95Low }, 6)},
				[]string{"Slope CI95 High", fmtConfidence(tr.Confidence, func(c *analyze.TrendConfidence) float64 { return c.SlopeCI95High }, 6)},
				[]string{"Slope/Year CI95 Low", fmtConfidence(tr.Confidence, func(c *analyze.TrendConfidence) float64 { return c.SlopeYearCI95Low }, 4)},
				[]string{"Slope/Year CI95 High", fmtConfidence(tr.Confidence, func(c *analyze.TrendConfidence) float64 { return c.SlopeYearCI95High }, 4)},
			)
		}
		printSimpleTable(w, []string{"METRIC", "VALUE"}, func(add func(...string)) {
			for _, row := range rows {
				add(row[0], row[1])
			}
		})
		if citation := strings.TrimSpace(tr.CitationText); citation != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, citation)
		}
		return nil
	},
}

var analyzeCompareCmd = &cobra.Command{
	Use:   "compare --against <SERIES_ID>",
	Short: "Compare two aligned series: correlation, beta, delta, tracking error",
	Example: `  reserve obs get UNRATE FEDFUNDS --format jsonl | reserve analyze compare --against FEDFUNDS
  reserve obs get CPIAUCSL GDPDEF --format jsonl | reserve analyze compare --series CPIAUCSL --against GDPDEF --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(analyzeCompareAgainst) == "" {
			return fmt.Errorf("--against is required")
		}
		groups, err := pipeline.ReadObservationGroups(os.Stdin)
		if err != nil {
			return err
		}
		byID := map[string]pipeline.ObservationGroup{}
		for _, g := range groups {
			byID[g.SeriesID] = g
		}
		rhsID := strings.ToUpper(strings.TrimSpace(analyzeCompareAgainst))
		rhs, ok := byID[rhsID]
		if !ok {
			return fmt.Errorf("against series %q not found in input stream", rhsID)
		}
		lhsID := strings.ToUpper(strings.TrimSpace(analyzeCompareSeries))
		if lhsID == "" {
			for _, g := range groups {
				if g.SeriesID != rhsID {
					lhsID = g.SeriesID
					break
				}
			}
		}
		if lhsID == "" {
			return fmt.Errorf("compare requires two series in input stream")
		}
		lhs, ok := byID[lhsID]
		if !ok {
			return fmt.Errorf("series %q not found in input stream", lhsID)
		}
		res, err := analyze.Compare(lhsID, lhs.Obs, rhsID, rhs.Obs)
		if err != nil {
			return err
		}
		applyProvenanceToCompare(&res, lhs.Provenance, rhs.Provenance)
		format := resolveFormat("")
		w, closeFn, err := outputWriter(cmd.OutOrStdout())
		if err != nil {
			return err
		}
		defer closeFn()
		if format == "json" || format == "jsonl" {
			enc := json.NewEncoder(w)
			if format == "json" {
				enc.SetIndent("", "  ")
			}
			return enc.Encode(res)
		}
		printSimpleTable(w, []string{"METRIC", "VALUE"}, func(add func(...string)) {
			add("Series", res.SeriesID)
			add("Against", res.AgainstSeriesID)
			add("Aligned Obs", fmt.Sprintf("%d", res.CountAligned))
			add("Correlation", fmtFloatTable(res.Correlation, 4))
			add("Beta", fmtFloatTable(res.Beta, 4))
			add("Delta Mean", fmtFloatTable(res.DeltaMean, 4))
			add("Delta Last", fmtFloatTable(res.DeltaLast, 4))
			add("Tracking Error", fmtFloatTable(res.TrackingError, 4))
		})
		if footer := compareCitationFooter(lhsID, lhs.Provenance.CitationText, rhsID, rhs.Provenance.CitationText); footer != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, footer)
		}
		return nil
	},
}

var analyzeRegimeCmd = &cobra.Command{
	Use:   "regime",
	Short: "Experimental regime detection (change points + labeled segments)",
	Example: `  reserve obs get UNRATE --start 2010-01-01 --format jsonl | reserve analyze regime --method cusum
  reserve obs get GDP --from cache --format jsonl | reserve analyze regime --method cusum --threshold 5`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, prov, err := pipeline.ReadObservationsWithProvenance(os.Stdin)
		if err != nil {
			return err
		}
		method := strings.ToLower(strings.TrimSpace(analyzeRegimeMethod))
		if method == "" {
			method = "cusum"
		}
		if method != "cusum" {
			return fmt.Errorf("unsupported regime method %q (supported: cusum)", method)
		}
		res, err := analyze.RegimeCUSUM(seriesID, obs, analyzeRegimeThreshold)
		if err != nil {
			return err
		}
		applyProvenanceToRegime(&res, prov)
		format := resolveFormat("")
		w, closeFn, err := outputWriter(cmd.OutOrStdout())
		if err != nil {
			return err
		}
		defer closeFn()
		if format == "json" || format == "jsonl" {
			enc := json.NewEncoder(w)
			if format == "json" {
				enc.SetIndent("", "  ")
			}
			return enc.Encode(res)
		}
		printSimpleTable(w, []string{"FIELD", "VALUE"}, func(add func(...string)) {
			add("Series", res.SeriesID)
			add("Method", res.Method)
			add("Threshold", fmtFloatTable(res.Threshold, 2))
			add("Signal", res.Signal)
			add("Min Gap", fmt.Sprintf("%d", res.MinGap))
			add("Change Points", fmt.Sprintf("%d", len(res.ChangePoints)))
			add("Segments", fmt.Sprintf("%d", len(res.Segments)))
		})
		if len(res.ChangePoints) > 0 {
			fmt.Fprintln(w)
			printSimpleTable(w, []string{"CP_DATE", "SIDE", "VALUE", "SCORE"}, func(add func(...string)) {
				for _, cp := range res.ChangePoints {
					add(cp.Date, cp.Side, fmtFloatTable(cp.Value, 4), fmtFloatTable(cp.Score, 3))
				}
			})
		}
		if len(res.Segments) > 0 {
			fmt.Fprintln(w)
			printSimpleTable(w, []string{"START_DATE", "END_DATE", "COUNT", "DIRECTION", "SLOPE_PER_YEAR"}, func(add func(...string)) {
				for _, s := range res.Segments {
					add(s.StartDate, s.EndDate, fmt.Sprintf("%d", s.Count), s.Direction, fmtFloatTable(s.SlopePerYear, 4))
				}
			})
		}
		if citation := strings.TrimSpace(res.CitationText); citation != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, citation)
		}
		return nil
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.AddCommand(analyzeSummaryCmd)
	analyzeCmd.AddCommand(analyzeTrendCmd)
	analyzeCmd.AddCommand(analyzeCompareCmd)
	analyzeCmd.AddCommand(analyzeRegimeCmd)

	analyzeSummaryCmd.Flags().BoolVar(&analyzeSummaryBySeries, "by-series", false,
		"group multi-series JSONL input by series_id and emit one summary per series")
	analyzeSummaryCmd.Flags().IntVar(&analyzeSummaryWindow, "window", 0,
		"rolling window size (observations) for summary output")
	analyzeTrendCmd.Flags().StringVar(&analyzeTrendMethod, "method", "linear",
		"regression method: linear|theil-sen")
	analyzeTrendCmd.Flags().BoolVar(&analyzeTrendConfidence, "confidence", false,
		"include confidence metadata for linear trend (stderr, p-value, 95% CI)")
	analyzeCompareCmd.Flags().StringVar(&analyzeCompareAgainst, "against", "", "series ID to compare against (must exist in input stream)")
	analyzeCompareCmd.Flags().StringVar(&analyzeCompareSeries, "series", "", "primary series ID (defaults to first non-against series)")
	analyzeRegimeCmd.Flags().StringVar(&analyzeRegimeMethod, "method", "cusum", "experimental method: cusum")
	analyzeRegimeCmd.Flags().Float64Var(&analyzeRegimeThreshold, "threshold", 5.0, "cusum threshold multiplier")
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
		{"Context", "-"},
		{"Version", s.AnalysisVersion},
		{"Series", s.SeriesID},
		{"Start Date", s.StartDate},
		{"End Date", s.EndDate},
		{"Observations", fmt.Sprintf("%d", s.Count)},
		{"Data Quality", "-"},
		{"Missing Count", fmt.Sprintf("%d", s.MissingCount)},
		{"Missing %", fmt.Sprintf("%.1f%%", s.MissingPct)},
		{"Distribution", "-"},
		{"Mean", fmtFloatTable(s.Mean, 4)},
		{"Std Dev", fmtFloatTable(s.Std, 4)},
		{"Min", fmtFloatTable(s.Min, 4)},
		{"P25", fmtFloatTable(s.P25, 4)},
		{"Median", fmtFloatTable(s.Median, 4)},
		{"P75", fmtFloatTable(s.P75, 4)},
		{"Max", fmtFloatTable(s.Max, 4)},
		{"Skew", fmtFloatTable(s.Skew, 4)},
		{"Movement", "-"},
		{"First", fmtFloatTable(s.First, 4)},
		{"Last", fmtFloatTable(s.Last, 4)},
		{"Change", fmtFloatTable(s.Change, 4)},
		{"Change %", fmtPctTable(s.ChangePct)},
	}
	printSimpleTable(w, []string{"METRIC", "VALUE"}, func(add func(...string)) {
		for _, row := range rows {
			add(row[0], row[1])
		}
	})
	if citation := strings.TrimSpace(s.CitationText); citation != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, citation)
	}
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
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].SeriesID != sorted[j].SeriesID {
				return sorted[i].SeriesID < sorted[j].SeriesID
			}
			return sorted[i].StartDate < sorted[j].StartDate
		})
		isWindowBatch := len(sorted) > 1
		for i := 1; i < len(sorted); i++ {
			if sorted[i].SeriesID != sorted[0].SeriesID {
				isWindowBatch = false
				break
			}
		}
		if isWindowBatch {
			printSimpleTable(w, []string{"SERIES", "START_DATE", "END_DATE", "COUNT", "MISS", "MEAN", "STD", "MIN", "MEDIAN", "MAX", "CHANGE_PCT"}, func(add func(...string)) {
				for _, s := range sorted {
					add(
						s.SeriesID,
						s.StartDate,
						s.EndDate,
						fmt.Sprintf("%d", s.Count),
						fmtMissCompact(s.MissingCount, s.MissingPct),
						fmtFloatTable(s.Mean, 4),
						fmtFloatTable(s.Std, 4),
						fmtFloatTable(s.Min, 4),
						fmtFloatTable(s.Median, 4),
						fmtFloatTable(s.Max, 4),
						fmtPctTable(s.ChangePct),
					)
				}
			})
		} else {
			printSimpleTable(w, []string{"SERIES", "COUNT", "MISS", "MEAN", "STD", "MIN", "MEDIAN", "MAX", "CHANGE_PCT"}, func(add func(...string)) {
				for _, s := range sorted {
					add(
						s.SeriesID,
						fmt.Sprintf("%d", s.Count),
						fmtMissCompact(s.MissingCount, s.MissingPct),
						fmtFloatTable(s.Mean, 4),
						fmtFloatTable(s.Std, 4),
						fmtFloatTable(s.Min, 4),
						fmtFloatTable(s.Median, 4),
						fmtFloatTable(s.Max, 4),
						fmtPctTable(s.ChangePct),
					)
				}
			})
		}
		if footer := summaryCitationFooter(sorted); footer != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, footer)
		}
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

func fmtFloatTable(v float64, decimals int) string {
	if math.IsNaN(v) {
		return "."
	}
	base := fmt.Sprintf("%.*f", decimals, v)
	parts := strings.SplitN(base, ".", 2)
	intPart := parts[0]
	sign := ""
	if strings.HasPrefix(intPart, "-") {
		sign = "-"
		intPart = strings.TrimPrefix(intPart, "-")
	}
	withCommas := addThousandsSeparators(intPart)
	if len(parts) == 2 {
		return sign + withCommas + "." + parts[1]
	}
	return sign + withCommas
}

func fmtPctTable(v float64) string {
	if math.IsNaN(v) {
		return "."
	}
	return fmtFloatTable(v, 2) + "%"
}

func addThousandsSeparators(s string) string {
	if len(s) <= 3 {
		return s
	}
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/3)
	b.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func fmtConfidence(c *analyze.TrendConfidence, f func(*analyze.TrendConfidence) float64, decimals int) string {
	if c == nil {
		return "n/a"
	}
	return fmtFloatTable(f(c), decimals)
}

func fmtMissCompact(count int, pct float64) string {
	return fmt.Sprintf("%d|%.1f%%", count, pct)
}

func applyProvenanceToSummary(s *analyze.Summary, p pipeline.Provenance) {
	s.CitationText = p.CitationText
	s.SourceName = p.SourceName
	s.SourceNames = append([]string(nil), p.SourceNames...)
}

func applyProvenanceToTrend(t *analyze.TrendResult, p pipeline.Provenance) {
	t.CitationText = p.CitationText
	t.SourceName = p.SourceName
	t.SourceNames = append([]string(nil), p.SourceNames...)
}

func applyProvenanceToRegime(r *analyze.RegimeResult, p pipeline.Provenance) {
	r.CitationText = p.CitationText
	r.SourceName = p.SourceName
	r.SourceNames = append([]string(nil), p.SourceNames...)
}

func applyProvenanceToCompare(c *analyze.CompareResult, lhs, rhs pipeline.Provenance) {
	c.CitationText = lhs.CitationText
	c.SourceName = lhs.SourceName
	c.SourceNames = append([]string(nil), lhs.SourceNames...)
	c.AgainstCitationText = rhs.CitationText
	c.AgainstSourceName = rhs.SourceName
	c.AgainstSourceNames = append([]string(nil), rhs.SourceNames...)
}

func summaryCitationFooter(summaries []analyze.Summary) string {
	seen := map[string]struct{}{}
	type row struct {
		seriesID string
		citation string
	}
	rows := make([]row, 0, len(summaries))
	for _, s := range summaries {
		c := strings.TrimSpace(s.CitationText)
		if c == "" {
			continue
		}
		key := s.SeriesID + "|" + c
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		rows = append(rows, row{seriesID: s.SeriesID, citation: c})
	}
	if len(rows) == 0 {
		return ""
	}
	first := rows[0].citation
	allSame := true
	for i := 1; i < len(rows); i++ {
		if rows[i].citation != first {
			allSame = false
			break
		}
	}
	if allSame {
		return first
	}
	lines := []string{"Sources by series:"}
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("- %s: %s", r.seriesID, normalizeCitationLabel(r.citation)))
	}
	return strings.Join(lines, "\n")
}

func compareCitationFooter(lhsSeriesID, lhsCitation, rhsSeriesID, rhsCitation string) string {
	lhsCitation = strings.TrimSpace(lhsCitation)
	rhsCitation = strings.TrimSpace(rhsCitation)
	switch {
	case lhsCitation == "" && rhsCitation == "":
		return ""
	case lhsCitation != "" && rhsCitation != "" && lhsCitation == rhsCitation:
		return lhsCitation
	case lhsCitation != "" && rhsCitation == "":
		return lhsCitation
	case lhsCitation == "" && rhsCitation != "":
		return rhsCitation
	default:
		return strings.Join([]string{
			"Sources by series:",
			fmt.Sprintf("- %s: %s", lhsSeriesID, normalizeCitationLabel(lhsCitation)),
			fmt.Sprintf("- %s: %s", rhsSeriesID, normalizeCitationLabel(rhsCitation)),
		}, "\n")
	}
}
