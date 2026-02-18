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

var metaCmd = &cobra.Command{
	Use:   "meta",
	Short: "Fetch raw metadata for FRED entities",
	Long: `Fetch and display metadata for any FRED entity type.

  reserve meta series CPIAUCSL GDP
  reserve meta category 32991
  reserve meta release 10
  reserve meta source 1
  reserve meta tag inflation`,
}

// ─── meta series ──────────────────────────────────────────────────────────────

var metaSeriesCmd = &cobra.Command{
	Use:   "series <SERIES_ID...>",
	Short: "Fetch metadata for one or more series",
	Example: `  reserve meta series GDP CPIAUCSL
  reserve meta series UNRATE --format json`,
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
		metas, warnings := batchGetSeries(cmd.Context(), deps, ids)
		sort.Slice(metas, func(i, j int) bool { return metas[i].ID < metas[j].ID })

		result := &model.Result{
			Kind:        model.KindSeriesMeta,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("meta series %s", strings.Join(ids, " ")),
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

// ─── meta category ────────────────────────────────────────────────────────────

var metaCategoryCmd = &cobra.Command{
	Use:   "category <CATEGORY_ID...>",
	Short: "Fetch metadata for one or more categories",
	Example: `  reserve meta category 32991 0
  reserve meta category 32991 --format json`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		var cats []model.Category
		var warnings []string
		for _, arg := range args {
			id, err := parseCategoryID(arg)
			if err != nil {
				warnings = append(warnings, err.Error())
				continue
			}
			cat, err := deps.Client.GetCategory(cmd.Context(), id)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", arg, err))
				continue
			}
			cats = append(cats, *cat)
		}
		format := resolveFormat(deps.Config.Format)
		if err := render.RenderCategories(cmd.OutOrStdout(), cats, format); err != nil {
			return err
		}
		for _, w := range warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "⚠  %s\n", w)
		}
		return nil
	},
}

// ─── meta release ─────────────────────────────────────────────────────────────

var metaReleaseCmd = &cobra.Command{
	Use:   "release <RELEASE_ID...>",
	Short: "Fetch metadata for one or more releases",
	Example: `  reserve meta release 10 11
  reserve meta release 10 --format json`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		var releases []model.Release
		var warnings []string
		for _, arg := range args {
			id, err := parseIntID(arg, "release ID")
			if err != nil {
				warnings = append(warnings, err.Error())
				continue
			}
			rel, err := deps.Client.GetRelease(cmd.Context(), id)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", arg, err))
				continue
			}
			releases = append(releases, *rel)
		}
		format := resolveFormat(deps.Config.Format)
		if err := render.RenderReleases(cmd.OutOrStdout(), releases, format); err != nil {
			return err
		}
		for _, w := range warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "⚠  %s\n", w)
		}
		return nil
	},
}

// ─── meta source ──────────────────────────────────────────────────────────────

var metaSourceCmd = &cobra.Command{
	Use:   "source <SOURCE_ID...>",
	Short: "Fetch metadata for one or more sources",
	Example: `  reserve meta source 1 2
  reserve meta source 1 --format json`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		var sources []model.Source
		var warnings []string
		for _, arg := range args {
			id, err := parseIntID(arg, "source ID")
			if err != nil {
				warnings = append(warnings, err.Error())
				continue
			}
			src, err := deps.Client.GetSource(cmd.Context(), id)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", arg, err))
				continue
			}
			sources = append(sources, *src)
		}
		format := resolveFormat(deps.Config.Format)
		if err := render.RenderSources(cmd.OutOrStdout(), sources, format); err != nil {
			return err
		}
		for _, w := range warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "⚠  %s\n", w)
		}
		return nil
	},
}

// ─── meta tag ─────────────────────────────────────────────────────────────────

var metaTagCmd = &cobra.Command{
	Use:   "tag <TAG...>",
	Short: "Look up metadata for one or more tags",
	Example: `  reserve meta tag inflation
  reserve meta tag inflation cpi --format json`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		// Use tag search to look up each tag by exact name
		var tags []model.Tag
		var warnings []string
		for _, name := range args {
			results, err := deps.Client.SearchTags(cmd.Context(), name, fred.SearchTagsOptions{Limit: 5})
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			// Find exact match
			found := false
			for _, t := range results {
				if strings.EqualFold(t.Name, name) {
					tags = append(tags, t)
					found = true
					break
				}
			}
			if !found && len(results) > 0 {
				tags = append(tags, results[0]) // best match
			} else if !found {
				warnings = append(warnings, fmt.Sprintf("%s: tag not found", name))
			}
		}
		format := resolveFormat(deps.Config.Format)
		if err := render.RenderTags(cmd.OutOrStdout(), tags, format); err != nil {
			return err
		}
		for _, w := range warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "⚠  %s\n", w)
		}
		return nil
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(metaCmd)
	metaCmd.AddCommand(metaSeriesCmd)
	metaCmd.AddCommand(metaCategoryCmd)
	metaCmd.AddCommand(metaReleaseCmd)
	metaCmd.AddCommand(metaSourceCmd)
	metaCmd.AddCommand(metaTagCmd)
}
