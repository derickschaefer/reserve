package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/spf13/cobra"
)

var (
	searchType  string
	searchLimit int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across supported global FRED entities",
	Long: `Perform a full-text search across FRED.

Use --type to restrict to a specific entity type:
  series (default), tag, all`,
	Example: `  reserve search "consumer price index"
  reserve search "unemployment" --type series --limit 10
  reserve search "inflation" --type tag --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}

		query := args[0]
		start := time.Now()
		format := resolveFormat(deps.Config.Format)

		switch strings.ToLower(searchType) {
		case "series", "":
			metas, err := deps.Client.SearchSeries(cmd.Context(), query, fred.SearchSeriesOptions{
				Limit: searchLimit,
			})
			if err != nil {
				return err
			}
			result := &model.Result{
				Kind:        model.KindSearchResult,
				GeneratedAt: time.Now(),
				Command:     fmt.Sprintf("search %q", query),
				Data: &model.SearchResult{
					Query:  query,
					Type:   "series",
					Series: metas,
				},
				Stats: model.ResultStats{
					DurationMs: time.Since(start).Milliseconds(),
					Items:      len(metas),
				},
			}
			return render.RenderTo(globalFlags.Out, result, format)

		case "tag":
			tags, err := deps.Client.SearchTags(cmd.Context(), query, fred.SearchTagsOptions{
				Limit: searchLimit,
			})
			if err != nil {
				return err
			}
			return render.RenderTags(cmd.OutOrStdout(), tags, format)

		case "all":
			// Best-effort: run series search and tag search, combine
			metas, seriesErr := deps.Client.SearchSeries(cmd.Context(), query, fred.SearchSeriesOptions{
				Limit: searchLimit,
			})
			tags, tagErr := deps.Client.SearchTags(cmd.Context(), query, fred.SearchTagsOptions{
				Limit: searchLimit,
			})

			var warnings []string
			if seriesErr != nil {
				warnings = append(warnings, fmt.Sprintf("series search: %v", seriesErr))
			}
			if tagErr != nil {
				warnings = append(warnings, fmt.Sprintf("tag search: %v", tagErr))
			}

			if len(metas) > 0 {
				if !deps.Config.Quiet {
					fmt.Fprintf(cmd.OutOrStdout(), "── Series ──\n")
				}
				result := &model.Result{
					Kind:     model.KindSearchResult,
					Command:  fmt.Sprintf("search %q --type all", query),
					Warnings: warnings,
					Data:     &model.SearchResult{Query: query, Type: "series", Series: metas},
				}
				if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
					return err
				}
			}
			if len(tags) > 0 {
				if !deps.Config.Quiet {
					fmt.Fprintf(cmd.OutOrStdout(), "\n── Tags ──\n")
				}
				if err := render.RenderTags(cmd.OutOrStdout(), tags, format); err != nil {
					return err
				}
			}
			for _, w := range warnings {
				fmt.Fprintf(cmd.OutOrStdout(), "⚠  %s\n", w)
			}
			return nil

		default:
			return fmt.Errorf("unknown --type %q: choose series|tag|all", searchType)
		}
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().StringVar(&searchType, "type", "series", "entity type: series|tag|all")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "max results")
}
