package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/spf13/cobra"
)

var seriesCmd = &cobra.Command{
	Use:   "series",
	Short: "Discover and inspect FRED data series",
	Long: `Commands for finding and inspecting FRED data series.

A series is a named sequence of economic data observations, such as:
  GDP       Gross Domestic Product
  CPIAUCSL  Consumer Price Index for All Urban Consumers
  UNRATE    Unemployment Rate`,
}

// ─── series get ───────────────────────────────────────────────────────────────

var seriesGetCmd = &cobra.Command{
	Use:   "get <SERIES_ID...>",
	Short: "Fetch metadata for one or more series",
	Example: `  reserve series get GDP
  reserve series get GDP CPIAUCSL UNRATE --format json`,
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

		if len(ids) == 1 {
			meta, err := deps.Client.GetSeries(cmd.Context(), ids[0])
			if err != nil {
				return err
			}
			result := &model.Result{
				Kind:        model.KindSeriesMeta,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("series get %s", ids[0]),
				Data:        meta,
				Stats: model.ResultStats{
					DurationMs: time.Since(start).Milliseconds(),
					Items:      1,
				},
			}
			format := resolveFormat(deps.Config.Format)
			if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
				return err
			}
			render.PrintFooter(cmd.OutOrStdout(), result, deps.Config.Verbose)
			return nil
		}

		// Batch: fetch concurrently
		metas, warnings := batchGetSeries(cmd.Context(), deps, ids)
		sort.Slice(metas, func(i, j int) bool { return metas[i].ID < metas[j].ID })

		result := &model.Result{
			Kind:        model.KindSeriesMeta,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("series get %s", strings.Join(ids, " ")),
			Data:        metas,
			Warnings:    warnings,
			Stats: model.ResultStats{
				DurationMs: time.Since(start).Milliseconds(),
				Items:      len(metas),
			},
		}
		format := resolveFormat(deps.Config.Format)
		if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
			return err
		}
		render.PrintFooter(cmd.OutOrStdout(), result, deps.Config.Verbose)
		return nil
	},
}

// ─── series search ────────────────────────────────────────────────────────────

var (
	seriesSearchTags  []string
	seriesSearchLimit int
)

var seriesSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for series by keyword",
	Example: `  reserve series search "consumer price index"
  reserve series search "unemployment" --limit 5
  reserve series search "inflation" --tag monthly --format csv`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}

		start := time.Now()
		opts := fred.SearchSeriesOptions{
			Tags:  seriesSearchTags,
			Limit: seriesSearchLimit,
		}
		metas, err := deps.Client.SearchSeries(cmd.Context(), args[0], opts)
		if err != nil {
			return err
		}

		result := &model.Result{
			Kind:        model.KindSearchResult,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("series search %q", args[0]),
			Data: &model.SearchResult{
				Query:  args[0],
				Type:   "series",
				Series: metas,
			},
			Stats: model.ResultStats{
				DurationMs: time.Since(start).Milliseconds(),
				Items:      len(metas),
			},
		}

		format := resolveFormat(deps.Config.Format)
		if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
			return err
		}
		render.PrintFooter(cmd.OutOrStdout(), result, deps.Config.Verbose)
		return nil
	},
}

// ─── series tags ──────────────────────────────────────────────────────────────

var seriesTagsCmd = &cobra.Command{
	Use:   "tags <SERIES_ID>",
	Short: "List tags associated with a series",
	Example: `  reserve series tags CPIAUCSL
  reserve series tags GDP --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}

		start := time.Now()
		tags, err := deps.Client.GetSeriesTags(cmd.Context(), args[0])
		if err != nil {
			return err
		}

		format := resolveFormat(deps.Config.Format)
		if format == render.FormatTable {
			renderTagsTable(cmd.OutOrStdout(), tags)
			return nil
		}

		result := &model.Result{
			Kind:        model.KindTag,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("series tags %s", args[0]),
			Data:        tags,
			Stats: model.ResultStats{
				DurationMs: time.Since(start).Milliseconds(),
				Items:      len(tags),
			},
		}
		return render.RenderTo(globalFlags.Out, result, format)
	},
}

// ─── series categories ────────────────────────────────────────────────────────

var seriesCategoriesCmd = &cobra.Command{
	Use:   "categories <SERIES_ID>",
	Short: "List categories a series belongs to",
	Example: `  reserve series categories GDP
  reserve series categories UNRATE --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}

		start := time.Now()
		cats, err := deps.Client.GetSeriesCategories(cmd.Context(), args[0])
		if err != nil {
			return err
		}

		format := resolveFormat(deps.Config.Format)
		if format == render.FormatTable {
			renderCategoriesTable(cmd.OutOrStdout(), cats)
			return nil
		}

		result := &model.Result{
			Kind:        model.KindCategory,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("series categories %s", args[0]),
			Data:        cats,
			Stats: model.ResultStats{
				DurationMs: time.Since(start).Milliseconds(),
				Items:      len(cats),
			},
		}
		return render.RenderTo(globalFlags.Out, result, format)
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(seriesCmd)
	seriesCmd.AddCommand(seriesGetCmd)
	seriesCmd.AddCommand(seriesSearchCmd)
	seriesCmd.AddCommand(seriesTagsCmd)
	seriesCmd.AddCommand(seriesCategoriesCmd)

	seriesSearchCmd.Flags().StringSliceVar(&seriesSearchTags, "tag", nil, "filter by tag (repeatable)")
	seriesSearchCmd.Flags().IntVar(&seriesSearchLimit, "limit", 20, "max results")
}

// ─── Inline table renderers for tags and categories ───────────────────────────

func renderTagsTable(w interface{ Write(b []byte) (int, error) }, tags []model.Tag) {
	printSimpleTable(w, []string{"NAME", "GROUP", "POPULARITY", "SERIES COUNT"}, func(add func(...string)) {
		for _, t := range tags {
			add(t.Name, t.GroupID, fmt.Sprintf("%d", t.Popularity), fmt.Sprintf("%d", t.SeriesCount))
		}
	})
}

func renderCategoriesTable(w interface{ Write(b []byte) (int, error) }, cats []model.Category) {
	printSimpleTable(w, []string{"ID", "NAME", "PARENT ID"}, func(add func(...string)) {
		for _, c := range cats {
			add(fmt.Sprintf("%d", c.ID), c.Name, fmt.Sprintf("%d", c.ParentID))
		}
	})
}
