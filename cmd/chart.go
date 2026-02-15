package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/derickschaefer/reserve/internal/chart"
	"github.com/derickschaefer/reserve/internal/pipeline"
)

var chartCmd = &cobra.Command{
	Use:   "chart",
	Short: "Render a time series as an ASCII chart (reads JSONL from stdin)",
	Long: `Chart commands read JSONL observations from stdin and render to the terminal.

Pipeline examples:
  reserve store get CPIAUCSL --format jsonl | reserve transform resample --freq annual --method mean | reserve chart bar
  reserve store get UNRATE --format jsonl | reserve chart plot
  reserve store get GDP --format jsonl | reserve transform pct-change | reserve chart plot --title "GDP QoQ Growth"`,
}

// ─── chart bar ───────────────────────────────────────────────────────────────

var (
	chartBarWidth   int
	chartBarMaxBars int
)

var chartBarCmd = &cobra.Command{
	Use:   "bar",
	Short: "Horizontal bar chart, one bar per observation",
	Long: `Renders a horizontal bar chart with one labeled bar per observation.

Best suited for low-frequency or resampled data (annual, quarterly). For
monthly or daily series, pipe through transform resample first:

  reserve store get CPIAUCSL --format jsonl \
    | reserve transform resample --freq annual --method mean \
    | reserve chart bar

Negative values are supported — bars extend left from a zero baseline.
NaN observations are silently skipped.`,
	Example: `  # Annual CPI — the natural use case
  reserve store get CPIAUCSL --format jsonl | reserve transform resample --freq annual --method mean | reserve chart bar

  # Quarterly GDP growth
  reserve store get GDP --format jsonl | reserve transform pct-change | reserve transform resample --freq annual --method mean | reserve chart bar

  # Fed funds rate by year
  reserve store get FEDFUNDS --format jsonl | reserve transform resample --freq annual --method mean | reserve chart bar

  # Last 10 years only
  reserve store get UNRATE --format jsonl | reserve transform filter --after 2015-01-01 | reserve transform resample --freq annual --method mean | reserve chart bar --max-bars 10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		if seriesID == "" {
			seriesID = "series"
		}
		return chart.Bar(os.Stdout, seriesID, obs, chart.BarOptions{
			Width:   chartBarWidth,
			MaxBars: chartBarMaxBars,
		})
	},
}

// ─── chart plot ──────────────────────────────────────────────────────────────

var (
	chartPlotWidth  int
	chartPlotHeight int
	chartPlotTitle  string
)

var chartPlotCmd = &cobra.Command{
	Use:   "plot",
	Short: "Multi-line ASCII chart with labeled axes",
	Long: `Renders a multi-line chart with Y-axis tick labels and X-axis date labels.

NaN values appear as gaps in the curve, not zeros. Width auto-detects from
$COLUMNS (falls back to 80). Override with --width and --height.`,
	Example: `  reserve store get UNRATE --format jsonl | reserve chart plot
  reserve store get CPIAUCSL --format jsonl | reserve chart plot --height 8
  reserve store get GDP --format jsonl | reserve transform pct-change | reserve chart plot --title "GDP QoQ %"
  reserve store get UNRATE --format jsonl | reserve window roll --stat mean --window 12 | reserve chart plot
  reserve obs get FEDFUNDS --start 2015-01-01 --format jsonl | reserve chart plot --width 100 --height 16`,
	RunE: func(cmd *cobra.Command, args []string) error {
		seriesID, obs, err := pipeline.ReadObservations(os.Stdin)
		if err != nil {
			return err
		}
		if seriesID == "" {
			seriesID = "series"
		}

		title := chartPlotTitle
		if title == "" {
			title = seriesID
		}

		// If --width not set and we're in a terminal, auto-detect.
		// chart.Plot handles width=0 by calling termWidth() internally.
		return chart.Plot(os.Stdout, seriesID, obs, chart.PlotOptions{
			Width:  chartPlotWidth,
			Height: chartPlotHeight,
			Title:  title,
		})
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(chartCmd)
	chartCmd.AddCommand(chartBarCmd)
	chartCmd.AddCommand(chartPlotCmd)

	// bar flags
	chartBarCmd.Flags().IntVar(&chartBarWidth, "width", 0,
		"total chart width in characters (default: auto-detect from $COLUMNS, fallback 80)")
	chartBarCmd.Flags().IntVar(&chartBarMaxBars, "max-bars", 0,
		"maximum bars to render — takes the last N if series is longer (0 = no limit)")

	// plot flags
	chartPlotCmd.Flags().IntVar(&chartPlotWidth, "width", 0,
		"chart width in characters (default: auto-detect from $COLUMNS, fallback 80)")
	chartPlotCmd.Flags().IntVar(&chartPlotHeight, "height", 12,
		"chart height in rows (default 12)")
	chartPlotCmd.Flags().StringVar(&chartPlotTitle, "title", "",
		"chart title (default: series ID)")

	chartCmd.SilenceUsage = true
	chartBarCmd.SilenceUsage = true
	chartPlotCmd.SilenceUsage = true
}
