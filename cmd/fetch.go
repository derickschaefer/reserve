package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/derickschaefer/reserve/internal/store"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Batch-fetch and optionally persist series data",
	Long: `Convenience commands for fetching multiple items in one call.

fetch series   — bulk-fetch metadata and/or observations for a list of series
fetch category — fetch all series under a category (optionally recursive)
fetch query    — search and fetch the top N results

Use --store to persist observations to the local database for offline analysis.`,
}

// ─── fetch series ─────────────────────────────────────────────────────────────

var (
	fetchWithMeta bool
	fetchWithObs  bool
	fetchStore    bool
	fetchStart    string
	fetchEnd      string
)

var fetchSeriesCmd = &cobra.Command{
	Use:   "series <SERIES_ID...>",
	Short: "Bulk-fetch metadata and/or observations for multiple series",
	Example: `  reserve fetch series GDP CPIAUCSL UNRATE
  reserve fetch series GDP CPIAUCSL --with-obs --start 2020-01-01
  reserve fetch series GDP --with-obs --format csv --out data.csv
  reserve fetch series GDP CPIAUCSL UNRATE --store`,
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

		// --store implies --with-obs
		if fetchStore {
			fetchWithObs = true
		}

		if !fetchWithObs {
			// Metadata only
			metas, warnings := batchGetSeries(cmd.Context(), deps, ids)
			sort.Slice(metas, func(i, j int) bool { return metas[i].ID < metas[j].ID })
			result := &model.Result{
				Kind:        model.KindSeriesMeta,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("fetch series %s", strings.Join(ids, " ")),
				Data:        metas,
				Warnings:    warnings,
				Stats: model.ResultStats{
					DurationMs: time.Since(start).Milliseconds(),
					Items:      len(metas),
				},
			}
			if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
				return err
			}
			render.PrintFooter(cmd.OutOrStdout(), result, deps.Config.Verbose)
			return nil
		}

		// With observations
		opts := fred.ObsOptions{Start: fetchStart, End: fetchEnd}
		datas, warnings := batchGetObs(cmd.Context(), deps, ids, opts)

		// Persist to local store if --store flag is set.
		//
		// Previously this looped over each series calling PutObs + PutSeriesMeta
		// individually, producing N×2 separate write transactions (and N×2 fsyncs).
		// Now we collect everything first and write in exactly two batch transactions:
		// one for all observations and one for all metadata.
		if fetchStore {
			if err := deps.RequireStore(); err != nil {
				return err
			}
			defer deps.Close()

			// ── Step 1: collect obs entries keyed by canonical obs key ────────
			obsEntries := make(map[string]model.SeriesData, len(datas))
			for _, data := range datas {
				key := store.ObsKey(data.SeriesID, fetchStart, fetchEnd, "", "", "")
				obsEntries[key] = *data
			}

			// ── Step 2: fetch metadata via the existing concurrent helper ─────
			// batchGetSeries fires concurrent API calls — same pattern as the
			// metadata-only path. Replaces the old per-series GetSeries loop.
			metaSlice, metaWarnings := batchGetSeries(cmd.Context(), deps, ids)
			warnings = append(warnings, metaWarnings...)

			// ── Step 3: single write transaction for all observations ─────────
			if err := deps.Store.PutObsBatch(obsEntries); err != nil {
				return fmt.Errorf("storing observations: %w", err)
			}

			// ── Step 4: single write transaction for all metadata ─────────────
			if len(metaSlice) > 0 {
				if err := deps.Store.PutSeriesMetaBatch(metaSlice); err != nil {
					// Non-fatal: obs are safely stored; warn and continue.
					warnings = append(warnings, fmt.Sprintf("storing metadata: %v", err))
				}
			}

			if !deps.Config.Quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Stored %d/%d series to %s\n",
					len(obsEntries), len(ids), deps.Config.DBPath)
				for _, w := range warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "  ⚠  %s\n", w)
				}
			}
			return nil
		}

		if fetchWithMeta {
			metas, metaWarn := batchGetSeries(cmd.Context(), deps, ids)
			warnings = append(warnings, metaWarn...)
			metaMap := make(map[string]*model.SeriesMeta, len(metas))
			for i := range metas {
				metaMap[metas[i].ID] = &metas[i]
			}
			for _, d := range datas {
				d.Meta = metaMap[d.SeriesID]
			}
		}

		for _, data := range datas {
			result := &model.Result{
				Kind:        model.KindSeriesData,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("fetch series %s", data.SeriesID),
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
		render.PrintFooter(cmd.OutOrStdout(), &model.Result{Warnings: warnings}, deps.Config.Verbose)
		return nil
	},
}

// ─── fetch category ───────────────────────────────────────────────────────────

var (
	fetchCategoryRecursive   bool
	fetchCategoryDepth       int
	fetchCategoryLimitSeries int
)

var fetchCategoryCmd = &cobra.Command{
	Use:   "category <CATEGORY_ID|root>",
	Short: "Fetch all series under a category",
	Example: `  reserve fetch category 32991 --limit-series 20
  reserve fetch category root --recursive --depth 1 --limit-series 5`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseCategoryID(args[0])
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
		format := resolveFormat(deps.Config.Format)

		var allMetas []model.SeriesMeta
		var warnings []string

		err = collectCategorySeries(cmd, deps, id, 0, fetchCategoryDepth, fetchCategoryRecursive, fetchCategoryLimitSeries, &allMetas, &warnings)
		if err != nil {
			return err
		}

		sort.Slice(allMetas, func(i, j int) bool { return allMetas[i].ID < allMetas[j].ID })
		result := &model.Result{
			Kind:        model.KindSeriesMeta,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("fetch category %d", id),
			Data:        allMetas,
			Warnings:    warnings,
			Stats: model.ResultStats{
				DurationMs: time.Since(start).Milliseconds(),
				Items:      len(allMetas),
			},
		}
		if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
			return err
		}
		render.PrintFooter(cmd.OutOrStdout(), result, deps.Config.Verbose)
		return nil
	},
}

// collectCategorySeries recursively collects series metadata from a category subtree.
func collectCategorySeries(cmd *cobra.Command, deps *app.Deps, categoryID, depth, maxDepth int, recursive bool, limitSeries int, out *[]model.SeriesMeta, warnings *[]string) error {
	metas, err := deps.Client.GetCategorySeries(cmd.Context(), categoryID, fred.CategorySeriesOptions{
		Limit: limitSeries,
	})
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("category %d: %v", categoryID, err))
	} else {
		*out = append(*out, metas...)
	}

	if recursive && depth < maxDepth {
		children, err := deps.Client.GetCategoryChildren(cmd.Context(), categoryID)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("category children %d: %v", categoryID, err))
			return nil
		}
		for _, child := range children {
			if err := collectCategorySeries(cmd, deps, child.ID, depth+1, maxDepth, recursive, limitSeries, out, warnings); err != nil {
				return err
			}
		}
	}
	return nil
}

// ─── fetch query ──────────────────────────────────────────────────────────────

var (
	fetchQueryTop     int
	fetchQueryWithObs bool
)

var fetchQueryCmd = &cobra.Command{
	Use:   "query <search-query>",
	Short: "Search and fetch the top N matching series",
	Example: `  reserve fetch query "consumer price index" --top 5
  reserve fetch query "gdp" --top 3 --with-obs --start 2020-01-01`,
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
		format := resolveFormat(deps.Config.Format)

		metas, err := deps.Client.SearchSeries(cmd.Context(), args[0], fred.SearchSeriesOptions{
			Limit: fetchQueryTop,
		})
		if err != nil {
			return err
		}

		if !fetchQueryWithObs {
			result := &model.Result{
				Kind:        model.KindSearchResult,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("fetch query %q", args[0]),
				Data:        &model.SearchResult{Query: args[0], Type: "series", Series: metas},
				Stats: model.ResultStats{
					DurationMs: time.Since(start).Milliseconds(),
					Items:      len(metas),
				},
			}
			return render.RenderTo(globalFlags.Out, result, format)
		}

		// Fetch observations for each matched series
		ids := make([]string, len(metas))
		for i, m := range metas {
			ids[i] = m.ID
		}
		opts := fred.ObsOptions{Start: fetchStart, End: fetchEnd}
		datas, warnings := batchGetObs(cmd.Context(), deps, ids, opts)

		for _, data := range datas {
			result := &model.Result{
				Kind:        model.KindSeriesData,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("fetch query %q %s", args[0], data.SeriesID),
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

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(fetchCmd)
	fetchCmd.AddCommand(fetchSeriesCmd)
	fetchCmd.AddCommand(fetchCategoryCmd)
	fetchCmd.AddCommand(fetchQueryCmd)

	fetchSeriesCmd.Flags().BoolVar(&fetchWithMeta, "with-meta", false, "include series metadata")
	fetchSeriesCmd.Flags().BoolVar(&fetchWithObs, "with-obs", false, "include observations")
	fetchSeriesCmd.Flags().BoolVar(&fetchStore, "store", false, "persist observations to local database")
	fetchSeriesCmd.Flags().StringVar(&fetchStart, "start", "", "observation start date YYYY-MM-DD")
	fetchSeriesCmd.Flags().StringVar(&fetchEnd, "end", "", "observation end date YYYY-MM-DD")

	fetchCategoryCmd.Flags().BoolVar(&fetchCategoryRecursive, "recursive", false, "recursively fetch child categories")
	fetchCategoryCmd.Flags().IntVar(&fetchCategoryDepth, "depth", 1, "max recursion depth (used with --recursive)")
	fetchCategoryCmd.Flags().IntVar(&fetchCategoryLimitSeries, "limit-series", 20, "max series per category")

	fetchQueryCmd.Flags().IntVar(&fetchQueryTop, "top", 10, "number of search results to fetch")
	fetchQueryCmd.Flags().BoolVar(&fetchQueryWithObs, "with-obs", false, "also fetch observations for matched series")
	fetchQueryCmd.Flags().StringVar(&fetchStart, "start", "", "observation start date YYYY-MM-DD")
	fetchQueryCmd.Flags().StringVar(&fetchEnd, "end", "", "observation end date YYYY-MM-DD")
}
