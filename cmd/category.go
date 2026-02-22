package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/spf13/cobra"
)

var categoryCmd = &cobra.Command{
	Use:   "category",
	Short: "Browse the FRED category hierarchy",
	Long: `Commands for exploring the FRED data category tree.

Categories are organised in a hierarchy rooted at category 0 (root).
Use "list" to list direct children, "tree" to recursively expand, and
"series" to list the series belonging to a category.`,
}

// ─── category get ─────────────────────────────────────────────────────────────

var categoryGetCmd = &cobra.Command{
	Use:   "get <CATEGORY_ID|root>",
	Short: "Fetch metadata for a category",
	Example: `  reserve category get 0          # root category
  reserve category get root
  reserve category get 32991`,
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
		cat, err := deps.Client.GetCategory(cmd.Context(), id)
		if err != nil {
			return err
		}
		format := resolveFormat(deps.Config.Format)
		return render.RenderCategories(cmd.OutOrStdout(), []model.Category{*cat}, format)
	},
}

// ─── category list ────────────────────────────────────────────────────────────

var categoryListCmd = &cobra.Command{
	Use:   "list <CATEGORY_ID|root>",
	Short: "List direct children of a category",
	Example: `  reserve category list root
  reserve category list 0
  reserve category list 32991`,
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
		cats, err := deps.Client.GetCategoryChildren(cmd.Context(), id)
		if err != nil {
			return err
		}
		format := resolveFormat(deps.Config.Format)
		return render.RenderCategories(cmd.OutOrStdout(), cats, format)
	},
}

// ─── category tree ────────────────────────────────────────────────────────────

var categoryTreeDepth int

var categoryTreeCmd = &cobra.Command{
	Use:   "tree <CATEGORY_ID|root>",
	Short: "Recursively display the category subtree",
	Example: `  reserve category tree root --depth 2
  reserve category tree 32991 --depth 3`,
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
		root, err := deps.Client.GetCategory(cmd.Context(), id)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[%d] %s\n", root.ID, root.Name)
		return walkCategoryTree(cmd, deps, id, 1, categoryTreeDepth, "")
	},
}

// walkCategoryTree recursively prints the category tree up to maxDepth.
func walkCategoryTree(cmd *cobra.Command, deps *app.Deps, parentID, depth, maxDepth int, prefix string) error {
	if depth > maxDepth {
		return nil
	}
	cats, err := deps.Client.GetCategoryChildren(cmd.Context(), parentID)
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "%s  ⚠  %v\n", prefix, err)
		return nil
	}
	for i, cat := range cats {
		isLast := i == len(cats)-1
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s%s[%d] %s\n", prefix, connector, cat.ID, cat.Name)
		if err := walkCategoryTree(cmd, deps, cat.ID, depth+1, maxDepth, childPrefix); err != nil {
			return err
		}
	}
	return nil
}

// ─── category series ──────────────────────────────────────────────────────────

var (
	categorySeriesLimit  int
	categorySeriesFilter string
)

var categorySeriesCmd = &cobra.Command{
	Use:   "series <CATEGORY_ID>",
	Short: "List series belonging to a category",
	Example: `  reserve category series 32991 --limit 10
  reserve category series 32991 --format csv`,
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
		metas, err := deps.Client.GetCategorySeries(cmd.Context(), id, fred.CategorySeriesOptions{
			Limit:  categorySeriesLimit,
			Filter: categorySeriesFilter,
		})
		if err != nil {
			return err
		}
		result := &model.Result{
			Kind:        model.KindSeriesMeta,
			GeneratedAt: time.Now(),
			Command:     fmt.Sprintf("category series %d", id),
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
	rootCmd.AddCommand(categoryCmd)
	categoryCmd.AddCommand(categoryGetCmd)
	categoryCmd.AddCommand(categoryListCmd)
	categoryCmd.AddCommand(categoryTreeCmd)
	categoryCmd.AddCommand(categorySeriesCmd)

	categoryTreeCmd.Flags().IntVar(&categoryTreeDepth, "depth", 2, "maximum recursion depth")
	categorySeriesCmd.Flags().IntVar(&categorySeriesLimit, "limit", 20, "max series to return")
	categorySeriesCmd.Flags().StringVar(&categorySeriesFilter, "filter", "", "filter expression: field=value")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// parseCategoryID converts "root" or a numeric string to an int category ID.
func parseCategoryID(s string) (int, error) {
	if strings.EqualFold(strings.TrimSpace(s), "root") {
		return 0, nil
	}
	var id int
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid category ID %q: expected a number or \"root\"", s)
	}
	return id, nil
}
