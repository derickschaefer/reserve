package cmd

import (
	"fmt"
	"time"

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
)

// ─── obs get ──────────────────────────────────────────────────────────────────

var obsGetCmd = &cobra.Command{
	Use:   "get <SERIES_ID...>",
	Short: "Fetch observations for one or more series",
	Example: `  reserve obs get GDP
  reserve obs get CPIAUCSL --start 2020-01-01 --end 2024-12-31
  reserve obs get UNRATE --freq monthly --units pc1
  reserve obs get GDP CPIAUCSL --format csv --out data.csv`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
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

		if len(ids) == 1 {
			data, err := deps.Client.GetObservations(cmd.Context(), ids[0], opts)
			if err != nil {
				return err
			}
			result := &model.Result{
				Kind:        model.KindSeriesData,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("obs get %s", ids[0]),
				Data:        data,
				Stats: model.ResultStats{
					DurationMs: time.Since(start).Milliseconds(),
					Items:      len(data.Obs),
				},
			}
			if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
				return err
			}
			render.PrintFooter(cmd.OutOrStdout(), result, deps.Config.Verbose)
			return nil
		}

		// Multiple series: fetch concurrently, output sequentially
		results, warnings := batchGetObs(cmd.Context(), deps, ids, opts)

		for _, data := range results {
			result := &model.Result{
				Kind:        model.KindSeriesData,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("obs get %s", data.SeriesID),
				Data:        data,
				Warnings:    warnings,
				Stats: model.ResultStats{
					DurationMs: time.Since(start).Milliseconds(),
					Items:      len(data.Obs),
				},
			}
			if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
				return err
			}
		}
		if len(warnings) > 0 {
			render.PrintFooter(cmd.OutOrStdout(), &model.Result{Warnings: warnings}, deps.Config.Verbose)
		}
		return nil
	},
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

		type latestRow struct {
			SeriesID string
			Date     string
			Value    string
		}
		var rows []latestRow
		var warnings []string

		for _, id := range ids {
			obs, err := deps.Client.GetLatestObservation(cmd.Context(), id)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", id, err))
				continue
			}
			rows = append(rows, latestRow{
				SeriesID: id,
				Date:     obs.Date.Format("2006-01-02"),
				Value:    obs.ValueRaw,
			})
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
			return nil
		}

		// For other formats, build a SeriesData result per series
		for _, r := range rows {
			t, _ := time.Parse("2006-01-02", r.Date)
			data := &model.SeriesData{
				SeriesID: r.SeriesID,
				Obs: []model.Observation{{
					Date:     t,
					ValueRaw: r.Value,
				}},
			}
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
	}
}
