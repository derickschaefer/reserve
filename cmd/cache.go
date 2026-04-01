// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
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
	Use:     "stats",
	Short:   "Show row counts and sizes for each bucket",
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

// ─── cache inventory ─────────────────────────────────────────────────────────

type cacheInventoryRow struct {
	SeriesID          string `json:"series_id"`
	Title             string `json:"title,omitempty"`
	ObsSets           int    `json:"obs_sets"`
	Start             string `json:"start,omitempty"`
	End               string `json:"end,omitempty"`
	Points            int    `json:"points"`
	Gaps              int    `json:"gaps,omitempty"`
	GapsNotApplicable bool   `json:"gaps_not_applicable,omitempty"`
	Frequency         string `json:"frequency,omitempty"`
	Meta              bool   `json:"meta"`
	Coverage          string `json:"coverage"`
}

type cacheInventoryOut struct {
	Database string              `json:"database"`
	Series   []cacheInventoryRow `json:"series"`
	Summary  struct {
		SeriesInventoried int `json:"series_inventoried"`
		WithGaps          int `json:"with_gaps"`
		MissingMetadata   int `json:"missing_metadata"`
		MultipleObsSets   int `json:"multiple_obs_sets"`
	} `json:"summary"`
	Actions []string `json:"actions,omitempty"`
}

var cacheInventoryCmd = &cobra.Command{
	Use:     "inventory",
	Short:   "Show per-series cache coverage and completeness",
	Example: `  reserve cache inventory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		defer deps.Close()

		rows, err := buildCacheInventory(deps.Store)
		if err != nil {
			return fmt.Errorf("building cache inventory: %w", err)
		}

		sort.Slice(rows, func(i, j int) bool { return rows[i].SeriesID < rows[j].SeriesID })

		out := cacheInventoryOut{
			Database: deps.Store.Path(),
			Series:   rows,
		}
		for _, row := range rows {
			out.Summary.SeriesInventoried++
			if !row.GapsNotApplicable && row.Gaps > 0 {
				out.Summary.WithGaps++
			}
			if !row.Meta {
				out.Summary.MissingMetadata++
			}
			if row.ObsSets > 1 {
				out.Summary.MultipleObsSets++
			}
		}
		out.Actions = buildInventoryActions(out)

		format := globalFlags.Format
		if format == "" {
			format = "table"
		}

		switch format {
		case "json":
			return writeCacheInventoryJSON(cmd.OutOrStdout(), out, false)
		case "jsonl":
			return writeCacheInventoryJSON(cmd.OutOrStdout(), out, true)
		default:
			return writeCacheInventoryTable(cmd.OutOrStdout(), out)
		}
	},
}

// ─── cache clear ──────────────────────────────────────────────────────────────

var (
	cacheClearAll    bool
	cacheClearBucket string
	cacheClearSeries string
)

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete entries from the local store",
	Long: `Delete entries from one or all buckets.

Note: bbolt does not shrink the database file automatically after clearing.
Free pages are reused internally on the next write. To reclaim disk space,
run 'reserve cache compact' after clearing.`,
	Example: `  reserve cache clear --all
  reserve cache clear --bucket obs
  reserve cache clear --bucket series_meta
  reserve cache clear --series GDP`,
	RunE: func(cmd *cobra.Command, args []string) error {
		selected := 0
		if cacheClearAll {
			selected++
		}
		if cacheClearBucket != "" {
			selected++
		}
		if cacheClearSeries != "" {
			selected++
		}
		if selected == 0 {
			return fmt.Errorf("specify exactly one of --all, --bucket <n>, or --series <id>\n\nBuckets: obs, series_meta")
		}
		if selected > 1 {
			return fmt.Errorf("use only one of --all, --bucket, or --series")
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
			fmt.Fprintln(cmd.OutOrStdout(), "  Run 'reserve cache compact' to reclaim disk space.")
			return nil
		}

		if cacheClearSeries != "" {
			seriesID := strings.ToUpper(strings.TrimSpace(cacheClearSeries))
			removed, err := deps.Store.ClearObsSeries(seriesID)
			if err != nil {
				return fmt.Errorf("clearing cached observations for %q: %w", cacheClearSeries, err)
			}
			if removed == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No cached observation sets found for %q.\n", seriesID)
				fmt.Fprintln(cmd.OutOrStdout(), "Series metadata was left intact.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Cleared %d cached observation set(s) for %q\n", removed, seriesID)
			fmt.Fprintln(cmd.OutOrStdout(), "  Series metadata was left intact.")
			fmt.Fprintln(cmd.OutOrStdout(), "  Run 'reserve cache compact' to reclaim disk space.")
			return nil
		}

		if err := deps.Store.ClearBucket(cacheClearBucket); err != nil {
			return fmt.Errorf("clearing bucket %q: %w", cacheClearBucket, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Cleared bucket %q\n", cacheClearBucket)
		fmt.Fprintln(cmd.OutOrStdout(), "  Run 'reserve cache compact' to reclaim disk space.")
		return nil
	},
}

// ─── cache compact ────────────────────────────────────────────────────────────

var cacheCompactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Rewrite the database file to reclaim freed disk space",
	Long: `Compact rewrites the entire bbolt database to a new file, recovering space
freed by prior 'cache clear' operations.

bbolt uses copy-on-write and never shrinks the database file automatically —
deleted pages are added to an internal freelist and reused on future writes.
Compaction is the only way to reduce the file's on-disk footprint.

The operation is safe: all live data is copied to a temporary file first, then
the original is atomically replaced. The database remains fully usable after
compaction completes.`,
	Example: `  reserve cache compact`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		// Note: we do NOT defer deps.Close() here because Compact() closes and
		// reopens the underlying bolt.DB itself. The Store handle remains valid
		// after Compact returns, so we close it normally at the end.
		defer deps.Close()

		fmt.Fprintf(cmd.OutOrStdout(), "Compacting %s ...\n", deps.Store.Path())

		before, after, err := deps.Store.Compact()
		if err != nil {
			return fmt.Errorf("compaction failed: %w", err)
		}

		saved := before - after
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Compaction complete\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  Before: %s\n", humanBytes(before))
		fmt.Fprintf(cmd.OutOrStdout(), "  After:  %s\n", humanBytes(after))
		if saved > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  Saved:  %s\n", humanBytes(saved))
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "  No space reclaimed (database was already compact).")
		}
		return nil
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheStatsCmd)
	cacheCmd.AddCommand(cacheInventoryCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheCompactCmd)

	cacheClearCmd.Flags().BoolVar(&cacheClearAll, "all", false, "clear all buckets")
	cacheClearCmd.Flags().StringVar(&cacheClearBucket, "bucket", "", "clear a specific bucket: obs|series_meta")
	cacheClearCmd.Flags().StringVar(&cacheClearSeries, "series", "", "clear cached observation sets for a specific series ID (metadata is preserved)")
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

type inventoryAccumulator struct {
	seriesID  string
	title     string
	frequency string
	meta      bool
	obsSets   int
	dates     map[string]time.Time
}

func buildCacheInventory(s interface {
	Path() string
	ListObsKeys(string) ([]string, error)
	GetObs(string) (model.SeriesData, bool, error)
	ListSeriesMeta() ([]model.SeriesMeta, error)
}) ([]cacheInventoryRow, error) {
	metas, err := s.ListSeriesMeta()
	if err != nil {
		return nil, err
	}
	metaByID := make(map[string]model.SeriesMeta, len(metas))
	for _, meta := range metas {
		metaByID[meta.ID] = meta
	}

	keys, err := s.ListObsKeys("")
	if err != nil {
		return nil, err
	}

	bySeries := make(map[string]*inventoryAccumulator)
	for _, key := range keys {
		data, found, err := s.GetObs(key)
		if err != nil {
			return nil, err
		}
		if !found || data.SeriesID == "" {
			continue
		}

		acc := bySeries[data.SeriesID]
		if acc == nil {
			acc = &inventoryAccumulator{
				seriesID: data.SeriesID,
				dates:    make(map[string]time.Time),
			}
			if meta, ok := metaByID[data.SeriesID]; ok {
				acc.title = meta.Title
				acc.frequency = normalizeFrequency(meta.Frequency)
				acc.meta = true
			}
			bySeries[data.SeriesID] = acc
		}
		acc.obsSets++
		for _, obs := range data.Obs {
			key := obs.Date.Format("2006-01-02")
			acc.dates[key] = obs.Date
		}
	}

	rows := make([]cacheInventoryRow, 0, len(bySeries))
	for _, acc := range bySeries {
		row := cacheInventoryRow{
			SeriesID:  acc.seriesID,
			Title:     acc.title,
			ObsSets:   acc.obsSets,
			Points:    len(acc.dates),
			Frequency: acc.frequency,
			Meta:      acc.meta,
			Coverage:  "unknown",
		}
		if len(acc.dates) == 0 {
			rows = append(rows, row)
			continue
		}

		dates := make([]time.Time, 0, len(acc.dates))
		for _, dt := range acc.dates {
			dates = append(dates, dt)
		}
		sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

		row.Start = dates[0].Format("2006-01-02")
		row.End = dates[len(dates)-1].Format("2006-01-02")

		gaps, ok := countExpectedGaps(dates, acc.frequency)
		if ok {
			row.Gaps = gaps
			if gaps == 0 {
				row.Coverage = "complete"
			} else {
				row.Coverage = "gapped"
			}
		} else if acc.frequency == "daily" {
			row.GapsNotApplicable = true
			row.Coverage = "not_applicable"
		}

		rows = append(rows, row)
	}
	return rows, nil
}

func normalizeFrequency(freq string) string {
	switch strings.ToLower(strings.TrimSpace(freq)) {
	case "daily":
		return "daily"
	case "weekly", "weekly, ending friday", "weekly, ending thursday", "weekly, ending wednesday", "weekly, ending tuesday", "weekly, ending monday", "weekly, ending saturday", "weekly, ending sunday":
		return "weekly"
	case "biweekly":
		return "biweekly"
	case "monthly":
		return "monthly"
	case "quarterly":
		return "quarterly"
	case "semiannual":
		return "semiannual"
	case "annual", "annually", "yearly":
		return "annual"
	default:
		return strings.ToLower(strings.TrimSpace(freq))
	}
}

func countExpectedGaps(dates []time.Time, frequency string) (int, bool) {
	if frequency == "daily" {
		return 0, false
	}
	if len(dates) < 2 {
		return 0, true
	}
	gaps := 0
	for i := 0; i < len(dates)-1; i++ {
		expected, ok := nextObservationDate(dates[i], frequency)
		if !ok {
			return 0, false
		}
		for expected.Before(dates[i+1]) {
			gaps++
			expected, ok = nextObservationDate(expected, frequency)
			if !ok {
				return 0, false
			}
		}
	}
	return gaps, true
}

func nextObservationDate(t time.Time, frequency string) (time.Time, bool) {
	switch frequency {
	case "daily":
		return t.AddDate(0, 0, 1), true
	case "weekly":
		return t.AddDate(0, 0, 7), true
	case "biweekly":
		return t.AddDate(0, 0, 14), true
	case "monthly":
		return t.AddDate(0, 1, 0), true
	case "quarterly":
		return t.AddDate(0, 3, 0), true
	case "semiannual":
		return t.AddDate(0, 6, 0), true
	case "annual":
		return t.AddDate(1, 0, 0), true
	default:
		return time.Time{}, false
	}
}

func writeCacheInventoryTable(w io.Writer, out cacheInventoryOut) error {
	fmt.Fprintf(w, "Database: %s\n\n", out.Database)
	printSimpleTable(w, []string{"SERIES", "TITLE", "OBS SETS", "START", "END", "POINTS", "GAPS", "FREQUENCY", "META"}, func(add func(...string)) {
		for _, row := range out.Series {
			add(
				row.SeriesID,
				blankIfEmpty(row.Title, "(metadata missing)"),
				fmt.Sprintf("%d", row.ObsSets),
				blankIfEmpty(row.Start, "-"),
				blankIfEmpty(row.End, "-"),
				fmt.Sprintf("%d", row.Points),
				displayGapCount(row),
				blankIfEmpty(row.Frequency, "unknown"),
				yesNo(row.Meta),
			)
		}
	})
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Summary:\n")
	fmt.Fprintf(w, "  series inventoried: %d\n", out.Summary.SeriesInventoried)
	fmt.Fprintf(w, "  with gaps: %d\n", out.Summary.WithGaps)
	fmt.Fprintf(w, "  missing metadata: %d\n", out.Summary.MissingMetadata)
	fmt.Fprintf(w, "  multiple obs sets: %d\n", out.Summary.MultipleObsSets)
	if len(out.Actions) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Inventory Actions:")
		for _, action := range out.Actions {
			fmt.Fprintf(w, "  - %s\n", action)
		}
	}
	return nil
}

func writeCacheInventoryJSON(w io.Writer, out cacheInventoryOut, jsonl bool) error {
	enc := json.NewEncoder(w)
	if jsonl {
		return enc.Encode(out)
	}
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func blankIfEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func displayGapCount(row cacheInventoryRow) string {
	if row.GapsNotApplicable {
		return "n/a"
	}
	return fmt.Sprintf("%d", row.Gaps)
}

func buildInventoryActions(out cacheInventoryOut) []string {
	actions := make([]string, 0, 3)
	if out.Summary.MissingMetadata > 0 {
		actions = append(actions,
			fmt.Sprintf("%d series are missing metadata. Re-fetch or refresh those series to restore titles and frequency details.", out.Summary.MissingMetadata))
	}
	if out.Summary.WithGaps > 0 {
		actions = append(actions,
			fmt.Sprintf("%d series have date gaps inside their cached range. Refill those series over the reported start/end range before analysis.", out.Summary.WithGaps))
	}
	if out.Summary.MultipleObsSets > 0 {
		actions = append(actions,
			fmt.Sprintf("%d series have multiple cached observation sets. Coverage may still be complete, but a full refresh can standardize them into a single canonical local range.", out.Summary.MultipleObsSets))
	}
	if len(actions) == 0 {
		actions = append(actions, "No coverage or metadata issues detected. Local cached series appear analysis-ready.")
	}
	return actions
}
