package cmd

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/pipeline"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/derickschaefer/reserve/internal/transform"
	"github.com/spf13/cobra"
)

var transformCmd = &cobra.Command{
	Use:   "transform",
	Short: "Transform a time series (reads JSONL from stdin)",
	Long: `Transform operators read JSONL observations from stdin and write to stdout.

Pipeline example:
  reserve store get GDP --format jsonl | reserve transform pct-change
  reserve store get CPIAUCSL --format jsonl | reserve transform diff --order 1 | reserve analyze summary`,
}

// ─── pct-change ───────────────────────────────────────────────────────────────

var transformPctPeriod int

var transformPctCmd = &cobra.Command{
	Use:   "pct-change",
	Short: "Percent change from N periods ago: (v[t]-v[t-N])/|v[t-N]| * 100",
	Example: `  reserve store get GDP --format jsonl | reserve transform pct-change
  reserve store get CPIAUCSL --format jsonl | reserve transform pct-change --period 12`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		out, err := transform.PctChange(obs, transformPctPeriod)
		if err != nil {
			return err
		}
		return writeTransformOutput(cmd, seriesID, out)
	},
}

// ─── diff ─────────────────────────────────────────────────────────────────────

var transformDiffOrder int

var transformDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "First or second difference: v[t] - v[t-1]",
	Example: `  reserve store get UNRATE --format jsonl | reserve transform diff
  reserve store get GDP --format jsonl | reserve transform diff --order 2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		out, err := transform.Diff(obs, transformDiffOrder)
		if err != nil {
			return err
		}
		return writeTransformOutput(cmd, seriesID, out)
	},
}

// ─── log ──────────────────────────────────────────────────────────────────────

var transformLogCmd = &cobra.Command{
	Use:     "log",
	Short:   "Natural log of each observation value",
	Example: `  reserve store get GDP --format jsonl | reserve transform log`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		out, warnings := transform.Log(obs)
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "⚠  %s\n", w)
		}
		return writeTransformOutput(cmd, seriesID, out)
	},
}

// ─── normalize ────────────────────────────────────────────────────────────────

var transformNormMethod string

var transformNormCmd = &cobra.Command{
	Use:   "normalize",
	Short: "Normalize observations: zscore (default) or minmax",
	Example: `  reserve store get UNRATE --format jsonl | reserve transform normalize
  reserve store get CPIAUCSL --format jsonl | reserve transform normalize --method minmax`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		out, err := transform.Normalize(obs, transform.NormalizeMethod(transformNormMethod))
		if err != nil {
			return err
		}
		return writeTransformOutput(cmd, seriesID, out)
	},
}

// ─── index ────────────────────────────────────────────────────────────────────

var (
	transformIndexBase float64
	transformIndexAt   string
)

var transformIndexCmd = &cobra.Command{
	Use:     "index",
	Short:   "Re-index series so value at --at date equals --base",
	Example: `  reserve store get CPIAUCSL --format jsonl | reserve transform index --base 100 --at 2010-01-01`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if transformIndexAt == "" {
			return fmt.Errorf("--at YYYY-MM-DD is required")
		}
		anchor, err := time.Parse("2006-01-02", transformIndexAt)
		if err != nil {
			return fmt.Errorf("--at: invalid date %q, expected YYYY-MM-DD", transformIndexAt)
		}
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		out, err := transform.Index(obs, transformIndexBase, anchor)
		if err != nil {
			return err
		}
		return writeTransformOutput(cmd, seriesID, out)
	},
}

// ─── resample ─────────────────────────────────────────────────────────────────

var (
	transformResampleFreq   string
	transformResampleMethod string
)

var transformResampleCmd = &cobra.Command{
	Use:   "resample",
	Short: "Downsample to lower frequency: monthly, quarterly, or annual",
	Example: `  reserve store get UNRATE --format jsonl | reserve transform resample --freq quarterly --method mean
  reserve store get CPIAUCSL --format jsonl | reserve transform resample --freq annual --method last`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		out, err := transform.Resample(obs,
			transform.ResampleFreq(transformResampleFreq),
			transform.ResampleMethod(transformResampleMethod),
		)
		if err != nil {
			return err
		}
		return writeTransformOutput(cmd, seriesID, out)
	},
}

// ─── filter ───────────────────────────────────────────────────────────────────

var (
	transformFilterAfter  string
	transformFilterBefore string
	transformFilterMin    float64
	transformFilterMax    float64
	transformFilterDrop   bool
)

var transformFilterCmd = &cobra.Command{
	Use:   "filter",
	Short: "Filter observations by date range or value bounds",
	Example: `  reserve store get UNRATE --format jsonl | reserve transform filter --after 2020-01-01
  reserve store get GDP --format jsonl | reserve transform filter --min 20000 --max 25000`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		opts := transform.FilterOptions{
			DropMissing: transformFilterDrop,
			MinValue:    math.NaN(),
			MaxValue:    math.NaN(),
		}
		if transformFilterAfter != "" {
			if opts.After, err = time.Parse("2006-01-02", transformFilterAfter); err != nil {
				return fmt.Errorf("--after: invalid date %q", transformFilterAfter)
			}
		}
		if transformFilterBefore != "" {
			if opts.Before, err = time.Parse("2006-01-02", transformFilterBefore); err != nil {
				return fmt.Errorf("--before: invalid date %q", transformFilterBefore)
			}
		}
		if cmd.Flags().Changed("min") {
			opts.MinValue = transformFilterMin
		}
		if cmd.Flags().Changed("max") {
			opts.MaxValue = transformFilterMax
		}
		out := transform.Filter(obs, opts)
		return writeTransformOutput(cmd, seriesID, out)
	},
}

// ─── window roll ──────────────────────────────────────────────────────────────

var windowCmd = &cobra.Command{
	Use:   "window",
	Short: "Rolling window operations on a time series",
}

var (
	windowRollWindow     int
	windowRollMinPeriods int
	windowRollStat       string
)

var windowRollCmd = &cobra.Command{
	Use:   "roll",
	Short: "Rolling window statistic: mean, std, min, max, or sum",
	Example: `  reserve store get UNRATE --format jsonl | reserve window roll --stat mean --window 12
  reserve store get GDP --format jsonl | reserve window roll --stat std --window 4 --min-periods 2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		out, err := transform.Roll(obs, windowRollWindow, windowRollMinPeriods, transform.RollStat(windowRollStat))
		if err != nil {
			return err
		}
		return writeTransformOutput(cmd, seriesID, out)
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(transformCmd)
	transformCmd.AddCommand(transformPctCmd)
	transformCmd.AddCommand(transformDiffCmd)
	transformCmd.AddCommand(transformLogCmd)
	transformCmd.AddCommand(transformNormCmd)
	transformCmd.AddCommand(transformIndexCmd)
	transformCmd.AddCommand(transformResampleCmd)
	transformCmd.AddCommand(transformFilterCmd)

	rootCmd.AddCommand(windowCmd)
	windowCmd.AddCommand(windowRollCmd)

	// pct-change flags
	transformPctCmd.Flags().IntVar(&transformPctPeriod, "period", 1, "lag period (1 = MoM, 12 = YoY)")

	// diff flags
	transformDiffCmd.Flags().IntVar(&transformDiffOrder, "order", 1, "difference order: 1 or 2")

	// normalize flags
	transformNormCmd.Flags().StringVar(&transformNormMethod, "method", "zscore", "normalization method: zscore|minmax")

	// index flags
	transformIndexCmd.Flags().Float64Var(&transformIndexBase, "base", 100, "base value at anchor date")
	transformIndexCmd.Flags().StringVar(&transformIndexAt, "at", "", "anchor date YYYY-MM-DD (required)")

	// resample flags
	transformResampleCmd.Flags().StringVar(&transformResampleFreq, "freq", "quarterly", "target frequency: monthly|quarterly|annual")
	transformResampleCmd.Flags().StringVar(&transformResampleMethod, "method", "mean", "aggregation method: mean|last|sum")

	// filter flags
	transformFilterCmd.Flags().StringVar(&transformFilterAfter, "after", "", "keep obs with date > YYYY-MM-DD")
	transformFilterCmd.Flags().StringVar(&transformFilterBefore, "before", "", "keep obs with date < YYYY-MM-DD")
	transformFilterCmd.Flags().Float64Var(&transformFilterMin, "min", 0, "keep obs with value >= min")
	transformFilterCmd.Flags().Float64Var(&transformFilterMax, "max", 0, "keep obs with value <= max")
	transformFilterCmd.Flags().BoolVar(&transformFilterDrop, "drop-missing", false, "drop NaN observations")

	// window roll flags
	windowRollCmd.Flags().IntVar(&windowRollWindow, "window", 12, "window size (number of observations)")
	windowRollCmd.Flags().IntVar(&windowRollMinPeriods, "min-periods", 1, "minimum non-NaN values required in window")
	windowRollCmd.Flags().StringVar(&windowRollStat, "stat", "mean", "statistic: mean|std|min|max|sum")
}

// ─── Output helper ────────────────────────────────────────────────────────────

// writeTransformOutput writes obs to stdout in JSONL (pipeline) or table (terminal).
func writeTransformOutput(cmd *cobra.Command, seriesID string, obs []model.Observation) error {
	format := resolveFormat("")
	// If no explicit format and stdout is a terminal, use table
	if globalFlags.Format == "" {
		if pipeline.IsTTY() {
			format = render.FormatTable
		} else {
			format = render.FormatJSONL
		}
	}

	if format == render.FormatJSONL {
		return pipeline.WriteJSONL(os.Stdout, seriesID, obs)
	}

	result := buildSeriesDataResult("transform", &model.SeriesData{
		SeriesID: seriesID,
		Obs:      obs,
	})
	return render.RenderTo(globalFlags.Out, result, format)
}
