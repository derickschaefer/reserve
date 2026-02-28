package cmd

import (
	"fmt"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Explore FRED data releases",
	Long:  `Commands for browsing FRED data releases and their publication schedules.`,
}

// ─── release list ─────────────────────────────────────────────────────────────

var releaseListLimit int

var releaseListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all FRED releases",
	Example: `  reserve release list
  reserve release list --limit 20 --format csv`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		start := time.Now()
		releases, err := deps.Client.ListReleases(cmd.Context(), releaseListLimit)
		if err != nil {
			return err
		}
		w, closeFn, err := outputWriter(cmd.OutOrStdout())
		if err != nil {
			return err
		}
		defer closeFn()

		format := resolveFormat(deps.Config.Format)
		if !deps.Config.Quiet {
			fmt.Fprintf(w, "Total releases: %d\n\n", len(releases))
		}
		if err := render.RenderReleases(w, releases, format); err != nil {
			return err
		}
		if deps.Config.Verbose {
			fmt.Fprintf(w, "\n[%d items • %dms]\n", len(releases), time.Since(start).Milliseconds())
		}
		return nil
	},
}

// ─── release get ──────────────────────────────────────────────────────────────

var releaseGetCmd = &cobra.Command{
	Use:   "get <RELEASE_ID>",
	Short: "Fetch metadata for a release",
	Example: `  reserve release get 10
  reserve release get 10 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseIntID(args[0], "release ID")
		if err != nil {
			return err
		}
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		rel, err := deps.Client.GetRelease(cmd.Context(), id)
		if err != nil {
			return err
		}
		w, closeFn, err := outputWriter(cmd.OutOrStdout())
		if err != nil {
			return err
		}
		defer closeFn()

		format := resolveFormat(deps.Config.Format)
		return render.RenderReleases(w, []model.Release{*rel}, format)
	},
}

// ─── release dates ────────────────────────────────────────────────────────────

var releaseDatesLimit int

var releaseDatesCmd = &cobra.Command{
	Use:   "dates <RELEASE_ID>",
	Short: "Show release dates for a release",
	Example: `  reserve release dates 10
  reserve release dates 10 --limit 5`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseIntID(args[0], "release ID")
		if err != nil {
			return err
		}
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		dates, err := deps.Client.GetReleaseDates(cmd.Context(), id, releaseDatesLimit)
		if err != nil {
			return err
		}
		w, closeFn, err := outputWriter(cmd.OutOrStdout())
		if err != nil {
			return err
		}
		defer closeFn()

		format := resolveFormat(deps.Config.Format)
		if format == render.FormatTable || format == "" {
			printSimpleTable(w, []string{"RELEASE ID", "RELEASE NAME", "DATE"}, func(add func(...string)) {
				for _, d := range dates {
					add(fmt.Sprintf("%d", d.ReleaseID), d.ReleaseName, d.Date)
				}
			})
			return nil
		}
		result := &model.Result{
			Kind:        model.KindRelease,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("release dates %d", id),
			Data:        dates,
		}
		return render.Render(w, result, format)
	},
}

// ─── release series ───────────────────────────────────────────────────────────

var releaseSeriesLimit int

var releaseSeriesCmd = &cobra.Command{
	Use:   "series <RELEASE_ID>",
	Short: "List series published in a release",
	Example: `  reserve release series 10 --limit 20
  reserve release series 10 --format csv`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseIntID(args[0], "release ID")
		if err != nil {
			return err
		}
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		start := time.Now()
		metas, err := deps.Client.GetReleaseSeries(cmd.Context(), id, releaseSeriesLimit)
		if err != nil {
			return err
		}
		result := &model.Result{
			Kind:        model.KindSeriesMeta,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("release series %d", id),
			Data:        metas,
			Stats: model.ResultStats{
				DurationMs: time.Since(start).Milliseconds(),
				Items:      len(metas),
			},
		}
		format := resolveFormat(deps.Config.Format)
		return render.RenderTo(globalFlags.Out, result, format)
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(releaseCmd)
	releaseCmd.AddCommand(releaseListCmd)
	releaseCmd.AddCommand(releaseGetCmd)
	releaseCmd.AddCommand(releaseDatesCmd)
	releaseCmd.AddCommand(releaseSeriesCmd)

	releaseListCmd.Flags().IntVar(&releaseListLimit, "limit", 0, "max releases (0 = all)")
	releaseDatesCmd.Flags().IntVar(&releaseDatesLimit, "limit", 20, "max dates to show")
	releaseSeriesCmd.Flags().IntVar(&releaseSeriesLimit, "limit", 20, "max series to return")
}
