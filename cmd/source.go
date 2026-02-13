package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
)

var sourceCmd = &cobra.Command{
	Use:   "source",
	Short: "Explore FRED data sources",
	Long:  `Commands for browsing the institutions (sources) that publish data on FRED.`,
}

// ─── source list ──────────────────────────────────────────────────────────────

var sourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all FRED data sources",
	Example: `  reserve source list
  reserve source list --format csv`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}
		start := time.Now()
		sources, err := deps.Client.ListSources(cmd.Context(), 0)
		if err != nil {
			return err
		}
		format := resolveFormat(deps.Config.Format)
		if err := render.RenderSources(cmd.OutOrStdout(), sources, format); err != nil {
			return err
		}
		if deps.Config.Verbose {
			fmt.Fprintf(cmd.OutOrStdout(), "\n[%d items • %dms]\n", len(sources), time.Since(start).Milliseconds())
		}
		return nil
	},
}

// ─── source get ───────────────────────────────────────────────────────────────

var sourceGetCmd = &cobra.Command{
	Use:   "get <SOURCE_ID>",
	Short: "Fetch metadata for a data source",
	Example: `  reserve source get 1
  reserve source get 1 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseIntID(args[0], "source ID")
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
		src, err := deps.Client.GetSource(cmd.Context(), id)
		if err != nil {
			return err
		}
		format := resolveFormat(deps.Config.Format)
		return render.RenderSources(cmd.OutOrStdout(), []model.Source{*src}, format)
	},
}

// ─── source releases ──────────────────────────────────────────────────────────

var sourceReleasesLimit int

var sourceReleasesCmd = &cobra.Command{
	Use:   "releases <SOURCE_ID>",
	Short: "List releases published by a source",
	Example: `  reserve source releases 1
  reserve source releases 1 --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseIntID(args[0], "source ID")
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
		releases, err := deps.Client.GetSourceReleases(cmd.Context(), id, sourceReleasesLimit)
		if err != nil {
			return err
		}
		format := resolveFormat(deps.Config.Format)
		return render.RenderReleases(cmd.OutOrStdout(), releases, format)
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(sourceCmd)
	sourceCmd.AddCommand(sourceListCmd)
	sourceCmd.AddCommand(sourceGetCmd)
	sourceCmd.AddCommand(sourceReleasesCmd)

	sourceReleasesCmd.Flags().IntVar(&sourceReleasesLimit, "limit", 0, "max releases (0 = all)")
}
