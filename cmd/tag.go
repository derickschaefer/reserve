package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
)

var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Search and explore FRED tags",
	Long:  `Commands for browsing FRED tags and finding series by tag.`,
}

// ─── tag search ───────────────────────────────────────────────────────────────

var tagSearchLimit int

var tagSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for FRED tags by keyword",
	Example: `  reserve tag search "inflation"
  reserve tag search "employment" --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		tags, err := deps.Client.SearchTags(cmd.Context(), args[0], fred.SearchTagsOptions{
			Limit: tagSearchLimit,
		})
		if err != nil {
			return err
		}
		format := resolveFormat(deps.Config.Format)
		return render.RenderTags(cmd.OutOrStdout(), tags, format)
	},
}

// ─── tag series ───────────────────────────────────────────────────────────────

var (
	tagSeriesLimit  int
	tagSeriesAll    bool
)

var tagSeriesCmd = &cobra.Command{
	Use:   "series <TAG...>",
	Short: "List series associated with one or more tags",
	Example: `  reserve tag series inflation
  reserve tag series inflation monthly --all
  reserve tag series cpi --limit 10 --format csv`,
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
		metas, err := deps.Client.GetTagSeries(cmd.Context(), args, fred.GetTagSeriesOptions{
			MatchAll: tagSeriesAll,
			Limit:    tagSeriesLimit,
		})
		if err != nil {
			return err
		}
		result := &model.Result{
			Kind:        model.KindSeriesMeta,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("tag series %v", args),
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

// ─── tag related ──────────────────────────────────────────────────────────────

var tagRelatedLimit int

var tagRelatedCmd = &cobra.Command{
	Use:   "related <TAG>",
	Short: "Find tags related to a given tag",
	Example: `  reserve tag related inflation
  reserve tag related cpi --limit 20`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		tags, err := deps.Client.GetRelatedTags(cmd.Context(), args[0], tagRelatedLimit)
		if err != nil {
			return err
		}
		format := resolveFormat(deps.Config.Format)
		return render.RenderTags(cmd.OutOrStdout(), tags, format)
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(tagCmd)
	tagCmd.AddCommand(tagSearchCmd)
	tagCmd.AddCommand(tagSeriesCmd)
	tagCmd.AddCommand(tagRelatedCmd)

	tagSearchCmd.Flags().IntVar(&tagSearchLimit, "limit", 20, "max tags to return")
	tagSeriesCmd.Flags().IntVar(&tagSeriesLimit, "limit", 20, "max series to return")
	tagSeriesCmd.Flags().BoolVar(&tagSeriesAll, "all", false, "series must match ALL tags (default: any)")
	tagRelatedCmd.Flags().IntVar(&tagRelatedLimit, "limit", 20, "max tags to return")
}
