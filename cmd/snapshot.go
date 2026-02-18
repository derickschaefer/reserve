package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/derickschaefer/reserve/internal/store"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Save and replay exact command lines",
	Long: `Snapshots let you save a reserve command and replay it later,
producing reproducible output from the same parameters.

  reserve snapshot save --name "monthly-cpi" --cmd "obs get CPIAUCSL --start 2020-01-01"
  reserve snapshot list
  reserve snapshot run <ID>`,
}

// ─── snapshot save ────────────────────────────────────────────────────────────

var (
	snapshotSaveName string
	snapshotSaveCmd  string
)

var snapshotSaveCommand = &cobra.Command{
	Use:   "save",
	Short: "Save a command line as a named snapshot",
	Example: `  reserve snapshot save --name "gdp-trend" --cmd "obs get GDP --start 2000-01-01"
  reserve snapshot save --name "cpi-monthly" --cmd "obs get CPIAUCSL --format csv"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if snapshotSaveName == "" {
			return fmt.Errorf("--name is required")
		}
		if snapshotSaveCmd == "" {
			return fmt.Errorf("--cmd is required")
		}

		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		defer deps.Close()

		id := newSnapshotID()
		snap := store.Snapshot{
			ID:          id,
			Name:        snapshotSaveName,
			CommandLine: snapshotSaveCmd,
			CreatedAt:   time.Now().UTC(),
		}
		if err := deps.Store.PutSnapshot(snap); err != nil {
			return fmt.Errorf("saving snapshot: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Saved snapshot %s  (%s)\n", id, snapshotSaveName)
		return nil
	},
}

// ─── snapshot list ────────────────────────────────────────────────────────────

var snapshotListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all saved snapshots",
	Example: `  reserve snapshot list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		defer deps.Close()

		snaps, err := deps.Store.ListSnapshots()
		if err != nil {
			return fmt.Errorf("listing snapshots: %w", err)
		}
		if len(snaps) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No snapshots saved.")
			fmt.Fprintln(cmd.OutOrStdout(), "  Use: reserve snapshot save --name <name> --cmd \"<command>\"")
			return nil
		}

		printSimpleTable(cmd.OutOrStdout(), []string{"ID", "NAME", "COMMAND", "CREATED"}, func(add func(...string)) {
			for _, s := range snaps {
				cmdPreview := s.CommandLine
				if len(cmdPreview) > 50 {
					cmdPreview = cmdPreview[:47] + "..."
				}
				add(s.ID, s.Name, cmdPreview, s.CreatedAt.Format("2006-01-02 15:04"))
			}
		})
		return nil
	},
}

// ─── snapshot show ────────────────────────────────────────────────────────────

var snapshotShowCmd = &cobra.Command{
	Use:     "show <ID>",
	Short:   "Show full details of a snapshot",
	Example: `  reserve snapshot show 01HX...`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		defer deps.Close()

		snap, ok, err := deps.Store.GetSnapshot(args[0])
		if err != nil {
			return fmt.Errorf("reading snapshot: %w", err)
		}
		if !ok {
			return fmt.Errorf("snapshot %q not found", args[0])
		}

		printSimpleTable(cmd.OutOrStdout(), []string{"FIELD", "VALUE"}, func(add func(...string)) {
			add("ID", snap.ID)
			add("Name", snap.Name)
			add("Command", snap.CommandLine)
			add("Created", snap.CreatedAt.Format(time.RFC3339))
		})
		return nil
	},
}

// ─── snapshot run ─────────────────────────────────────────────────────────────

var snapshotRunCmd = &cobra.Command{
	Use:     "run <ID>",
	Short:   "Re-execute a saved snapshot",
	Example: `  reserve snapshot run 01HX...`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}

		// Read snapshot BEFORE closing the store
		snap, ok, err := deps.Store.GetSnapshot(args[0])
		deps.Close() // Close now — child process will open its own handle
		if err != nil {
			return fmt.Errorf("reading snapshot: %w", err)
		}
		if !ok {
			return fmt.Errorf("snapshot %q not found", args[0])
		}

		// Re-execute using the current binary with the stored command line.
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding executable: %w", err)
		}

		parts := strings.Fields(snap.CommandLine)
		c := exec.CommandContext(cmd.Context(), self, parts...)
		c.Stdout = cmd.OutOrStdout()
		c.Stderr = cmd.ErrOrStderr()

		if !deps.Config.Quiet {
			fmt.Fprintf(cmd.OutOrStdout(), "▶ %s %s\n\n", self, snap.CommandLine)
		}
		return c.Run()
	},
}

// ─── snapshot delete ──────────────────────────────────────────────────────────

var snapshotDeleteCmd = &cobra.Command{
	Use:     "delete <ID>",
	Short:   "Delete a saved snapshot",
	Example: `  reserve snapshot delete 01HX...`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.RequireStore(); err != nil {
			return err
		}
		defer deps.Close()

		snap, ok, err := deps.Store.GetSnapshot(args[0])
		if err != nil {
			return fmt.Errorf("reading snapshot: %w", err)
		}
		if !ok {
			return fmt.Errorf("snapshot %q not found", args[0])
		}

		if err := deps.Store.DeleteSnapshot(args[0]); err != nil {
			return fmt.Errorf("deleting snapshot: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted snapshot %s  (%s)\n", snap.ID, snap.Name)
		return nil
	},
}

// ─── Registration ─────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotSaveCommand)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotShowCmd)
	snapshotCmd.AddCommand(snapshotRunCmd)
	snapshotCmd.AddCommand(snapshotDeleteCmd)

	snapshotSaveCommand.Flags().StringVar(&snapshotSaveName, "name", "", "human-readable name for the snapshot (required)")
	snapshotSaveCommand.Flags().StringVar(&snapshotSaveCmd, "cmd", "", "command line to save, without the binary name (required)")
	snapshotSaveCommand.MarkFlagRequired("name")
	snapshotSaveCommand.MarkFlagRequired("cmd")
}

// ─── ID generation ────────────────────────────────────────────────────────────

// newSnapshotID generates a time-sortable snapshot ID.
// Format: YYYYMMDDHHmmss + 4 random hex chars — no external dependency needed.
func newSnapshotID() string {
	now := time.Now().UTC()
	base := now.Format("20060102150405")
	// Add pseudo-random suffix from nanoseconds
	nano := now.UnixNano() & 0xFFFF
	return fmt.Sprintf("%s%04x", base, nano)
}
