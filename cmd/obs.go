// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/spf13/cobra"
)

var obsCmd = &cobra.Command{
	Use:   "obs",
	Short: "Retrieve time series observations",
	Long: `Fetch historical observations (data points) for one or more FRED series.

Units options:  lin (levels), chg (change), ch1 (change yr ago), pch (% change),
                pc1 (% change yr ago), pca (% chg annual rate), log (natural log)
Freq options:   daily, weekly, monthly, quarterly, annual
Agg options:    avg, sum, eop (end of period)`,
}

var (
	obsStart string
	obsEnd   string
	obsFreq  string
	obsUnits string
	obsAgg   string
	obsLimit int
	obsFrom  string
)

type latestRow struct {
	SeriesID string
	Date     string
	ValueNum float64
	Value    string
	Meta     *model.SeriesMeta
}

func latestRowFromObservation(seriesID string, obs *model.Observation) latestRow {
	return latestRow{
		SeriesID: seriesID,
		Date:     obs.Date.Format("2006-01-02"),
		ValueNum: obs.Value,
		Value:    obs.ValueRaw,
	}
}

func latestRowToSeriesData(r latestRow) *model.SeriesData {
	t, _ := time.Parse("2006-01-02", r.Date)
	return &model.SeriesData{
		SeriesID: r.SeriesID,
		Meta:     r.Meta,
		Obs: []model.Observation{{
			Date:     t,
			Value:    r.ValueNum,
			ValueRaw: r.Value,
		}},
	}
}

// ─── obs get ──────────────────────────────────────────────────────────────────

var obsGetCmd = &cobra.Command{
	Use:   "get <SERIES_ID...>",
	Short: "Fetch observations for one or more series",
	Example: `  reserve obs get GDP
  reserve obs get CPIAUCSL --start 2020-01-01 --end 2024-12-31
  reserve obs get CPIAUCSL --from cache --format jsonl
  reserve obs get UNRATE --freq monthly --units pc1
  reserve obs get GDP CPIAUCSL --format csv --out data.csv`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}

		src, err := resolveObsSource(obsFrom)
		if err != nil {
			return err
		}
		if err := validateObsSourceConfig(deps, src); err != nil {
			return err
		}

		// Validate date flags if provided
		if obsStart != "" {
			if _, err := time.Parse("2006-01-02", obsStart); err != nil {
				return fmt.Errorf("--start: invalid date %q, expected YYYY-MM-DD", obsStart)
			}
		}
		if obsEnd != "" {
			if _, err := time.Parse("2006-01-02", obsEnd); err != nil {
				return fmt.Errorf("--end: invalid date %q, expected YYYY-MM-DD", obsEnd)
			}
		}

		opts := fred.ObsOptions{
			Start: obsStart,
			End:   obsEnd,
			Freq:  obsFreq,
			Units: obsUnits,
			Agg:   obsAgg,
			Limit: obsLimit,
		}

		start := time.Now()
		ids := normaliseIDs(args)
		format := resolveFormat(deps.Config.Format)
		commandFrom := ""
		if obsFrom != "" {
			commandFrom = " --from " + obsFrom
		}
		if deps.Config.Debug {
			fmt.Fprintf(cmd.ErrOrStderr(), "DEBUG obs.get source=%s ids=%d\n", src.name(), len(ids))
		}

		if len(ids) == 1 {
			data, cacheHit, warnings, err := src.get(cmd.Context(), deps, ids[0], opts)
			if err != nil {
				return err
			}
			result := &model.Result{
				Kind:        model.KindSeriesData,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("obs get %s%s", ids[0], commandFrom),
				Data:        data,
				Warnings:    warnings,
				Stats: model.ResultStats{
					CacheHit:   cacheHit,
					DurationMs: time.Since(start).Milliseconds(),
					Items:      len(data.Obs),
				},
			}
			if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
				return err
			}
			render.PrintFooter(obsFooterWriter(cmd, format), result, deps.Config.Verbose)
			return nil
		}

		// Multiple series: fetch concurrently, output sequentially
		results, warnings, anyCache := batchGetObs(cmd.Context(), deps, ids, opts, src)

		for _, data := range results {
			result := &model.Result{
				Kind:        model.KindSeriesData,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("obs get %s%s", data.SeriesID, commandFrom),
				Data:        data,
				Warnings:    warnings,
				Stats: model.ResultStats{
					CacheHit:   anyCache,
					DurationMs: time.Since(start).Milliseconds(),
					Items:      len(data.Obs),
				},
			}
			if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
				return err
			}
		}
		if len(warnings) > 0 {
			render.PrintFooter(obsFooterWriter(cmd, format), &model.Result{Warnings: warnings}, deps.Config.Verbose)
		}
		return nil
	},
}

func validateObsSourceConfig(deps *app.Deps, src obsSource) error {
	if src.requiresAPIKey() {
		return deps.Config.Validate()
	}
	return nil
}

func obsFooterWriter(cmd *cobra.Command, format string) io.Writer {
	switch format {
	case render.FormatJSON, render.FormatJSONL, render.FormatCSV, render.FormatTSV, render.FormatMD:
		return cmd.ErrOrStderr()
	default:
		return cmd.OutOrStdout()
	}
}

// ─── obs latest ───────────────────────────────────────────────────────────────

var obsLatestCmd = &cobra.Command{
	Use:   "latest <SERIES_ID...>",
	Short: "Show the most recent observation for one or more series",
	Example: `  reserve obs latest GDP
  reserve obs latest UNRATE CPIAUCSL --format table`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}

		start := time.Now()
		ids := normaliseIDs(args)
		format := resolveFormat(deps.Config.Format)

		var rows []latestRow
		var warnings []string

		for _, id := range ids {
			meta, err := ensureSeriesCompliance(cmd.Context(), deps, id, "display")
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", id, err))
				continue
			}
			obs, err := deps.Client.GetLatestObservation(cmd.Context(), id)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", id, err))
				continue
			}
			row := latestRowFromObservation(id, obs)
			metaCopy := meta
			row.Meta = &metaCopy
			rows = append(rows, row)
		}

		if format == render.FormatTable || format == "" {
			printSimpleTable(cmd.OutOrStdout(), []string{"SERIES", "DATE", "LATEST VALUE"}, func(add func(...string)) {
				for _, r := range rows {
					add(r.SeriesID, r.Date, r.Value)
				}
			})
			for _, w := range warnings {
				fmt.Fprintf(cmd.OutOrStdout(), "⚠  %s\n", w)
			}
			seenCitation := make(map[string]struct{})
			for _, r := range rows {
				if r.Meta == nil || r.Meta.CitationText == "" {
					continue
				}
				if _, ok := seenCitation[r.Meta.CitationText]; ok {
					continue
				}
				seenCitation[r.Meta.CitationText] = struct{}{}
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), r.Meta.CitationText)
			}
			return nil
		}

		// For other formats, build a SeriesData result per series
		for _, r := range rows {
			data := latestRowToSeriesData(r)
			result := &model.Result{
				Kind:        model.KindSeriesData,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("obs latest %s", r.SeriesID),
				Data:        data,
				Warnings:    warnings,
				Stats: model.ResultStats{
					DurationMs: time.Since(start).Milliseconds(),
					Items:      1,
				},
			}
			if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(obsCmd)
	obsCmd.AddCommand(obsGetCmd)
	obsCmd.AddCommand(obsLatestCmd)

	for _, c := range []*cobra.Command{obsGetCmd} {
		c.Flags().StringVar(&obsStart, "start", "", "start date YYYY-MM-DD")
		c.Flags().StringVar(&obsEnd, "end", "", "end date YYYY-MM-DD")
		c.Flags().StringVar(&obsFreq, "freq", "", "frequency: daily|weekly|monthly|quarterly|annual")
		c.Flags().StringVar(&obsUnits, "units", "", "units: lin|chg|ch1|pch|pc1|pca|cch|cca|log")
		c.Flags().StringVar(&obsAgg, "agg", "", "aggregation: avg|sum|eop")
		c.Flags().IntVar(&obsLimit, "limit", 0, "max observations (0 = all)")
		c.Flags().StringVar(&obsFrom, "from", "", "data source: live|cache (default: live)")
	}
}
