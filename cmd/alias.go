// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/config"
	"github.com/spf13/cobra"
)

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage local aliases for FRED series IDs",
	Long: `Manage local aliases for FRED series IDs.

Aliases are stored in config.json and resolve before reserve calls FRED. They
let you use memorable local names for long or awkward series IDs.`,
}

var aliasSetCmd = &cobra.Command{
	Use:     "set <ALIAS> <SERIES_ID>",
	Short:   "Set a local alias for a FRED series ID",
	Example: `  reserve alias set pce-services PB0000031Q225SBEA`,
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias, err := validateAliasName(args[0])
		if err != nil {
			return err
		}
		seriesID := strings.ToUpper(strings.TrimSpace(args[1]))
		if seriesID == "" {
			return fmt.Errorf("series ID cannot be empty")
		}

		deps, err := buildDeps()
		if err != nil {
			return err
		}
		if err := deps.Config.Validate(); err != nil {
			return err
		}

		if _, err := deps.Client.GetSeries(cmd.Context(), seriesID); err != nil {
			return fmt.Errorf("target series %q could not be verified: %w", seriesID, err)
		}
		conflicts, err := aliasConflictsWithSeriesID(cmd, deps, alias)
		if err != nil {
			return err
		}
		if conflicts {
			return fmt.Errorf("alias %q conflicts with an existing FRED series ID; choose a different alias", alias)
		}

		f, path, err := loadOrTemplateConfig()
		if err != nil {
			return err
		}
		f.SeriesAliases = config.NormalizeSeriesAliases(f.SeriesAliases)
		if f.SeriesAliases == nil {
			f.SeriesAliases = map[string]config.Alias{}
		}
		entry := config.Alias{SeriesID: seriesID}
		if existing, ok := f.SeriesAliases[alias]; ok {
			entry.Note = existing.Note
		}
		if cmd.Flags().Changed("note") {
			entry.Note = strings.TrimSpace(aliasSetNote)
		}
		f.SeriesAliases[alias] = entry
		if err := config.WriteFile(path, *f); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Set alias %s -> %s in %s\n", alias, seriesID, path)
		return nil
	},
}

var aliasListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List local series aliases",
	Example: `  reserve alias list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(globalFlags.APIKey)
		if err != nil {
			return err
		}
		aliases := config.NormalizeSeriesAliases(cfg.SeriesAliases)
		if len(aliases) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No series aliases configured.")
			return nil
		}
		format := resolveFormat(cfg.Format)
		if format == "json" {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(aliases)
		}
		printAliasTable(cmd.OutOrStdout(), aliases)
		return nil
	},
}

var aliasGetCmd = &cobra.Command{
	Use:     "get <ALIAS>",
	Short:   "Show the series ID for a local alias",
	Example: `  reserve alias get pce-services`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := config.NormalizeAlias(args[0])
		cfg, err := config.Load(globalFlags.APIKey)
		if err != nil {
			return err
		}
		entry, ok := cfg.SeriesAliases[alias]
		if !ok {
			return fmt.Errorf("alias %q not found", alias)
		}
		format := resolveFormat(cfg.Format)
		if format == "json" {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]config.Alias{alias: entry})
		}
		if entry.Note == "" {
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", alias, entry.SeriesID)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s (%s)\n", alias, entry.SeriesID, entry.Note)
		return nil
	},
}

var aliasDeleteCmd = &cobra.Command{
	Use:     "delete <ALIAS>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete a local series alias",
	Example: `  reserve alias delete pce-services`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := config.NormalizeAlias(args[0])
		f, path, err := loadAliasOwnerConfig(alias)
		if err != nil {
			return err
		}
		f.SeriesAliases = config.NormalizeSeriesAliases(f.SeriesAliases)
		delete(f.SeriesAliases, alias)
		if len(f.SeriesAliases) == 0 {
			f.SeriesAliases = nil
		}
		if err := config.WriteFile(path, *f); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted alias %s from %s\n", alias, path)
		return nil
	},
}

var aliasNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var reservedAliases = map[string]struct{}{
	"help": {},
}

func validateAliasName(alias string) (string, error) {
	alias = config.NormalizeAlias(alias)
	if alias == "" {
		return "", fmt.Errorf("alias cannot be empty")
	}
	if !aliasNameRE.MatchString(alias) {
		return "", fmt.Errorf("alias %q is invalid: use letters, numbers, dot, underscore, or hyphen", alias)
	}
	if isReservedAlias(alias) {
		return "", fmt.Errorf("alias %q is reserved", alias)
	}
	return alias, nil
}

func isReservedAlias(alias string) bool {
	if _, ok := reservedAliases[alias]; ok {
		return true
	}
	for _, c := range rootCmd.Commands() {
		if c.Name() == alias {
			return true
		}
		for _, a := range c.Aliases {
			if a == alias {
				return true
			}
		}
	}
	return false
}

var seriesIDLookup = func(ctx context.Context, deps *app.Deps, seriesID string) error {
	_, err := deps.Client.GetSeries(ctx, seriesID)
	return err
}

func aliasConflictsWithSeriesID(cmd *cobra.Command, deps *app.Deps, alias string) (bool, error) {
	err := seriesIDLookup(cmd.Context(), deps, strings.ToUpper(alias))
	if err == nil {
		return true, nil
	}
	if isTransientAPIError(err) {
		return false, fmt.Errorf("unable to verify alias collision for %q due to transient FRED/API error: %w", alias, err)
	}
	return false, nil
}

func isTransientAPIError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "temporary") ||
		strings.Contains(msg, "http 502") ||
		strings.Contains(msg, "http 503") ||
		strings.Contains(msg, "http 504") ||
		strings.Contains(msg, "gateway time-out") ||
		strings.Contains(msg, "no such host")
}

func loadAliasOwnerConfig(alias string) (*config.File, string, error) {
	paths, err := editableConfigPaths()
	if err != nil {
		return nil, "", err
	}
	for _, path := range paths {
		f, err := readConfigAtPath(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, "", err
		}
		aliases := config.NormalizeSeriesAliases(f.SeriesAliases)
		if _, ok := aliases[alias]; ok {
			f.SeriesAliases = aliases
			return f, path, nil
		}
	}
	return nil, "", fmt.Errorf("alias %q not found", alias)
}

func editableConfigPaths() ([]string, error) {
	preferred, err := config.PreferredConfigPath()
	if err != nil {
		return nil, err
	}
	paths := []string{preferred}
	local, err := filepath.Abs(config.DefaultConfigFile)
	if err != nil {
		return nil, err
	}
	user, err := config.UserConfigPath()
	if err != nil {
		return nil, err
	}
	for _, p := range []string{local, user} {
		if p != preferred {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func readConfigAtPath(path string) (*config.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f config.File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func printAliasTable(w io.Writer, aliases map[string]config.Alias) {
	keys := make([]string, 0, len(aliases))
	for alias := range aliases {
		keys = append(keys, alias)
	}
	sort.Strings(keys)
	printSimpleTable(w, []string{"ALIAS", "SERIES", "NOTE"}, func(add func(...string)) {
		for _, alias := range keys {
			add(alias, aliases[alias].SeriesID, aliases[alias].Note)
		}
	})
}

var aliasSetNote string

func init() {
	rootCmd.AddCommand(aliasCmd)
	aliasCmd.AddCommand(aliasSetCmd)
	aliasCmd.AddCommand(aliasListCmd)
	aliasCmd.AddCommand(aliasGetCmd)
	aliasCmd.AddCommand(aliasDeleteCmd)

	aliasSetCmd.Flags().StringVar(&aliasSetNote, "note", "", "optional user note for this alias")
}
