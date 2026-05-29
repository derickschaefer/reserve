// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/derickschaefer/reserve/internal/config"
	snlib "github.com/derickschaefer/reserve/internal/snippet"
	"github.com/spf13/cobra"
)

var snippetNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

var snippetCmd = &cobra.Command{
	Use:   "snippet",
	Short: "Manage reusable workflow snippets",
	Long: `Manage reusable workflow snippets stored in filesystem-backed snippet libraries.

Snippets are saved under your snippet home (default ~/.reserve/snippets) in
library files such as personal/snippets.yaml.`,
}

var snippetSetCmd = &cobra.Command{
	Use:     "set <NAME> --cmd \"<COMMAND>\"",
	Short:   "Create or update a named snippet",
	Example: `  reserve snippet set pcu_annual_bar --desc "Bar chart of Semiconductor & Electronic PPI" --cmd "./reserve obs get PCU3344133441 --start 2018-01-01 --end 2026-05-01 --format jsonl | ./reserve transform resample --freq annual --method mean | ./reserve chart bar"`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		command := strings.TrimSpace(snippetSetCommand)
		if command == "" {
			return fmt.Errorf("--cmd cannot be empty")
		}
		home, enabled, err := snippetSettings()
		if err != nil {
			return err
		}
		_ = enabled

		ref, err := snlib.ParseRef(args[0])
		if err != nil {
			return err
		}
		if err := validateSnippetName(ref.Name); err != nil {
			return err
		}
		libName := ref.Library
		if libName == "" {
			libName = snlib.DefaultLibrary
		}

		lib, err := snlib.LoadOrInitLibrary(home, libName)
		if err != nil {
			return err
		}
		lib.Snippets[ref.Name] = snlib.Snippet{
			Description: strings.TrimSpace(snippetSetDescription),
			Command:     command,
		}
		if err := snlib.SaveLibrary(home, lib); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Set snippet %s/%s in %s\n", libName, ref.Name, snlib.LibraryPath(home, libName))
		return nil
	},
}

var snippetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved snippets",
	Example: `  reserve snippet list
  reserve snippet list --library personal`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, enabled, err := snippetSettings()
		if err != nil {
			return err
		}
		refs, values, err := snlib.List(home, enabled, snippetListLibrary)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No snippets configured.")
			return nil
		}

		cfg, err := config.Load(globalFlags.APIKey)
		if err != nil {
			return err
		}
		format := resolveFormat(cfg.Format)
		if format == "json" {
			type row struct {
				Library     string `json:"library"`
				Name        string `json:"name"`
				Description string `json:"description,omitempty"`
				Command     string `json:"command"`
			}
			out := make([]row, 0, len(refs))
			for _, r := range refs {
				s := values[r]
				out = append(out, row{Library: r.Library, Name: r.Name, Description: s.Description, Command: s.Command})
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		printSimpleTable(cmd.OutOrStdout(), []string{"LIBRARY", "NAME", "DESCRIPTION"}, func(add func(...string)) {
			for _, r := range refs {
				s := values[r]
				add(r.Library, r.Name, snippetDescription(s))
			}
		})
		return nil
	},
}

var snippetGetCmd = &cobra.Command{
	Use:   "get <NAME>",
	Short: "Show a snippet command",
	Example: `  reserve snippet get pcu_annual_bar
  reserve snippet get personal/pcu_annual_bar`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, enabled, err := snippetSettings()
		if err != nil {
			return err
		}
		ref, s, err := snlib.Resolve(home, args[0], enabled)
		if err != nil {
			return err
		}
		_ = ref
		fmt.Fprintln(cmd.OutOrStdout(), s.Command)
		return nil
	},
}

var snippetDeleteCmd = &cobra.Command{
	Use:     "delete <NAME>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete a saved snippet",
	Example: `  reserve snippet delete pcu_annual_bar`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, enabled, err := snippetSettings()
		if err != nil {
			return err
		}
		ref, _, err := snlib.Resolve(home, args[0], enabled)
		if err != nil {
			return err
		}
		lib, err := snlib.LoadOrInitLibrary(home, ref.Library)
		if err != nil {
			return err
		}
		delete(lib.Snippets, ref.Name)
		if err := snlib.SaveLibrary(home, lib); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted snippet %s/%s\n", ref.Library, ref.Name)
		return nil
	},
}

var snippetRunCmd = &cobra.Command{
	Use:     "run <NAME>",
	Short:   "Run a saved snippet command in your shell",
	Example: `  reserve snippet run pcu_annual_bar`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, enabled, err := snippetSettings()
		if err != nil {
			return err
		}
		ref, s, err := snlib.Resolve(home, args[0], enabled)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "▶ %s/%s: %s\n", ref.Library, ref.Name, s.Command)
		proc := exec.CommandContext(cmd.Context(), "bash", "-lc", s.Command)
		proc.Stdout = cmd.OutOrStdout()
		proc.Stderr = cmd.ErrOrStderr()
		proc.Stdin = cmd.InOrStdin()
		return proc.Run()
	},
}

var snippetSetCommand string
var snippetSetDescription string
var snippetListLibrary string

func init() {
	rootCmd.AddCommand(snippetCmd)
	snippetCmd.AddCommand(snippetSetCmd)
	snippetCmd.AddCommand(snippetListCmd)
	snippetCmd.AddCommand(snippetGetCmd)
	snippetCmd.AddCommand(snippetDeleteCmd)
	snippetCmd.AddCommand(snippetRunCmd)

	snippetSetCmd.Flags().StringVar(&snippetSetCommand, "cmd", "", "snippet shell command to store")
	snippetSetCmd.Flags().StringVar(&snippetSetDescription, "desc", "", "short human description")
	_ = snippetSetCmd.MarkFlagRequired("cmd")

	snippetListCmd.Flags().StringVar(&snippetListLibrary, "library", "", "only list snippets from one library")
}

func validateSnippetName(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return fmt.Errorf("snippet name cannot be empty")
	}
	if !snippetNameRE.MatchString(name) {
		return fmt.Errorf("snippet name %q is invalid: use letters, numbers, dot, underscore, or hyphen", name)
	}
	return nil
}

func snippetDescription(s snlib.Snippet) string {
	desc := strings.TrimSpace(s.Description)
	if desc != "" {
		return desc
	}
	return snippetPreview(s.Command)
}

func snippetPreview(command string) string {
	const max = 90
	command = strings.TrimSpace(command)
	if len(command) <= max {
		return command
	}
	return command[:max-3] + "..."
}

func snippetSettings() (home string, enabled []string, err error) {
	cfg, err := config.Load(globalFlags.APIKey)
	if err != nil {
		return "", nil, err
	}
	home, err = snlib.ResolveHome(cfg.Snippet.Home)
	if err != nil {
		return "", nil, err
	}
	enabled = cfg.Snippet.Enabled
	return home, enabled, nil
}
