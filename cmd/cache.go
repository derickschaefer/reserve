package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Inspect and manage the local data store",
	Long: `Commands for inspecting and clearing the local bbolt database.

The local store accumulates series data fetched with 'reserve fetch --store'.
It is an intentional data store, not a transparent cache — data persists until
you explicitly clear it.`,
}

// ─── cache stats ──────────────────────────────────────────────────────────────

var cacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show row counts and sizes for each bucket",
	Example: `  reserve cache stats`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		defer deps.Close()

		stats, err := deps.Store.Stats()
		if err != nil {
			return fmt.Errorf("reading store stats: %w", err)
		}

		// Sort by bucket name for deterministic output
		sort.Slice(stats, func(i, j int) bool { return stats[i].Name < stats[j].Name })

		fmt.Fprintf(cmd.OutOrStdout(), "Database: %s\n\n", deps.Store.Path())
		printSimpleTable(cmd.OutOrStdout(), []string{"BUCKET", "ROWS", "SIZE"}, func(add func(...string)) {
			for _, s := range stats {
				add(s.Name, fmt.Sprintf("%d", s.Count), humanBytes(s.Bytes))
			}
		})
		return nil
	},
}

// ─── cache clear ──────────────────────────────────────────────────────────────

var (
	cacheClearAll    bool
	cacheClearBucket string
)

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete entries from the local store",
	Example: `  reserve cache clear --all
  reserve cache clear --bucket obs
  reserve cache clear --bucket series_meta`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cacheClearAll && cacheClearBucket == "" {
			return fmt.Errorf("specify --all or --bucket <name>\n\nBuckets: obs, series_meta, snapshots")
		}

		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		defer deps.Close()

		if cacheClearAll {
			if err := deps.Store.ClearAll(); err != nil {
				return fmt.Errorf("clearing all buckets: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Cleared all buckets")
			return nil
		}

		if err := deps.Store.ClearBucket(cacheClearBucket); err != nil {
			return fmt.Errorf("clearing bucket %q: %w", cacheClearBucket, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Cleared bucket %q\n", cacheClearBucket)
		return nil
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheStatsCmd)
	cacheCmd.AddCommand(cacheClearCmd)

	cacheClearCmd.Flags().BoolVar(&cacheClearAll, "all", false, "clear all buckets")
	cacheClearCmd.Flags().StringVar(&cacheClearBucket, "bucket", "", "clear a specific bucket: obs|series_meta|snapshots")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func humanBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
