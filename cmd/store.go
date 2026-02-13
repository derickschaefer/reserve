package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/derickschaefer/reserve/internal/render"
)

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Inspect locally accumulated data",
	Long: `Commands for inspecting what data has been accumulated in the local database.

Use 'reserve fetch series --store' to accumulate data.
Use 'reserve cache stats' for bucket-level storage stats.`,
}

// ─── store list ───────────────────────────────────────────────────────────────

var storeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List series accumulated in the local database",
	Example: `  reserve store list
  reserve store list --format csv`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		defer deps.Close()

		metas, err := deps.Store.ListSeriesMeta()
		if err != nil {
			return fmt.Errorf("reading store: %w", err)
		}

		if len(metas) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No series in local database.")
			fmt.Fprintln(cmd.OutOrStdout(), "  Use: reserve fetch series <ID...> --store")
			return nil
		}

		// Sort by ID for deterministic output
		sort.Slice(metas, func(i, j int) bool { return metas[i].ID < metas[j].ID })

		// List obs keys so we can show date ranges stored
		obsKeys, _ := deps.Store.ListObsKeys("")

		// Build a map of series → stored key count
		keyCounts := make(map[string]int)
		for _, k := range obsKeys {
			// Keys are: series:<ID>|...
			id := extractSeriesIDFromKey(k)
			if id != "" {
				keyCounts[id]++
			}
		}

		format := resolveFormat(deps.Config.Format)
		if format == render.FormatTable || format == "" {
			printSimpleTable(cmd.OutOrStdout(), []string{"ID", "TITLE", "FREQ", "UNITS", "FETCHED AT", "OBS SETS"}, func(add func(...string)) {
				for _, m := range metas {
					title := m.Title
					if len(title) > 40 {
						title = title[:37] + "..."
					}
					units := m.UnitsShort
					if units == "" {
						units = m.Units
					}
					fetchedAt := ""
					if !m.FetchedAt.IsZero() {
						fetchedAt = m.FetchedAt.Format("2006-01-02 15:04")
					}
					sets := keyCounts[m.ID]
					add(m.ID, title, m.FrequencyShort, units, fetchedAt, fmt.Sprintf("%d", sets))
				}
			})
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d series  •  %d observation sets  •  %s\n",
				len(metas), len(obsKeys), deps.Store.Path())
			return nil
		}

		// Non-table formats: use the standard result envelope
		return render.RenderTo(globalFlags.Out, buildSeriesMetaResult("store list", metas), format)
	},
}

// ─── store get ────────────────────────────────────────────────────────────────

var storeGetCmd = &cobra.Command{
	Use:   "get <SERIES_ID>",
	Short: "Read stored observations for a series",
	Example: `  reserve store get GDP
  reserve store get CPIAUCSL --format csv`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := normaliseIDs(args)[0]

		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		defer deps.Close()

		// Find matching obs keys for this series
		keys, err := deps.Store.ListObsKeys(id)
		if err != nil {
			return fmt.Errorf("reading store: %w", err)
		}
		if len(keys) == 0 {
			return fmt.Errorf("no stored observations for %s\n\n  Use: reserve fetch series %s --store", id, id)
		}

		// Use the first (and usually only) key
		data, ok, err := deps.Store.GetObs(keys[0])
		if err != nil {
			return fmt.Errorf("reading obs: %w", err)
		}
		if !ok {
			return fmt.Errorf("observation data missing for key %s", keys[0])
		}

		// Attach metadata if available
		if meta, ok, _ := deps.Store.GetSeriesMeta(id); ok {
			data.Meta = &meta
		}

		format := resolveFormat(deps.Config.Format)
		result := buildSeriesDataResult("store get "+id, &data)
		if err := render.RenderTo(globalFlags.Out, result, format); err != nil {
			return err
		}
		render.PrintFooter(cmd.OutOrStdout(), result, deps.Config.Verbose)
		return nil
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(storeCmd)
	storeCmd.AddCommand(storeListCmd)
	storeCmd.AddCommand(storeGetCmd)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// extractSeriesIDFromKey parses the series ID from an obs bucket key.
// Key format: series:<ID>|start:...|...
func extractSeriesIDFromKey(key string) string {
	// Strip "series:" prefix
	if len(key) < 8 || key[:7] != "series:" {
		return ""
	}
	rest := key[7:]
	// ID ends at first "|" or end of string
	for i, c := range rest {
		if c == '|' {
			return rest[:i]
		}
	}
	return rest
}
