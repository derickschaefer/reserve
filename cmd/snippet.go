// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/derickschaefer/reserve/internal/config"
	"github.com/spf13/cobra"
)

const maxSnippets = 10

var snippetNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

var snippetCmd = &cobra.Command{
	Use:   "snippet",
	Short: "Manage reusable pipeline command snippets",
	Long: `Manage reusable pipeline command snippets stored in config.json.

Snippets are local named shell command strings, useful for repeatable reserve
pipelines you do not want to keep retyping.`,
}

var snippetSetCmd = &cobra.Command{
	Use:     "set <NAME> --cmd \"<COMMAND>\"",
	Short:   "Create or update a named snippet",
	Example: `  reserve snippet set pcu_annual_bar --desc "Bar chart of Semiconductor & Electronic PPI" --cmd "./reserve obs get PCU3344133441 --start 2018-01-01 --end 2026-05-01 --format jsonl | ./reserve transform resample --freq annual --method mean | ./reserve chart bar"`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := validateSnippetName(args[0])
		if err != nil {
			return err
		}
		command := strings.TrimSpace(snippetSetCommand)
		if command == "" {
			return fmt.Errorf("--cmd cannot be empty")
		}

		f, path, err := loadOrTemplateConfig()
		if err != nil {
			return err
		}
		f.Snippets = config.NormalizeSnippets(f.Snippets)
		if f.Snippets == nil {
			f.Snippets = map[string]config.Snippet{}
		}
		if _, exists := f.Snippets[name]; !exists && len(f.Snippets) >= maxSnippets {
			return fmt.Errorf("snippet limit reached (%d). Delete one first.", maxSnippets)
		}
		f.Snippets[name] = config.Snippet{
			Command:     command,
			Description: strings.TrimSpace(snippetSetDescription),
		}
		if err := config.WriteFile(path, *f); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Set snippet %s in %s\n", name, path)
		return nil
	},
}

var snippetListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List saved snippets",
	Example: `  reserve snippet list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(globalFlags.APIKey)
		if err != nil {
			return err
		}
		snippets := config.NormalizeSnippets(cfg.Snippets)
		if len(snippets) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No snippets configured.")
			return nil
		}
		format := resolveFormat(cfg.Format)
		if format == "json" {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(snippets)
		}
		keys := sortedSnippetKeys(snippets)
		printSimpleTable(cmd.OutOrStdout(), []string{"NAME", "DESCRIPTION"}, func(add func(...string)) {
			for _, k := range keys {
				add(k, snippetDescription(snippets[k]))
			}
		})
		return nil
	},
}

var snippetGetCmd = &cobra.Command{
	Use:     "get <NAME>",
	Short:   "Show a snippet command",
	Example: `  reserve snippet get pcu_annual_bar`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := normalizeSnippetName(args[0])
		snippets, err := loadResolvedSnippets()
		if err != nil {
			return err
		}
		snippet, ok := snippets[name]
		if !ok {
			return fmt.Errorf("snippet %q not found", name)
		}
		fmt.Fprintln(cmd.OutOrStdout(), snippet.Command)
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
		name := normalizeSnippetName(args[0])
		f, path, err := loadSnippetOwnerConfig(name)
		if err != nil {
			return err
		}
		f.Snippets = config.NormalizeSnippets(f.Snippets)
		delete(f.Snippets, name)
		if len(f.Snippets) == 0 {
			f.Snippets = nil
		}
		if err := config.WriteFile(path, *f); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Deleted snippet %s from %s\n", name, path)
		return nil
	},
}

var snippetRunCmd = &cobra.Command{
	Use:     "run <NAME>",
	Short:   "Run a saved snippet command in your shell",
	Example: `  reserve snippet run pcu_annual_bar`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := normalizeSnippetName(args[0])
		snippets, err := loadResolvedSnippets()
		if err != nil {
			return err
		}
		snippet, ok := snippets[name]
		if !ok {
			return fmt.Errorf("snippet %q not found", name)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "▶ %s\n", snippet.Command)
		proc := exec.CommandContext(cmd.Context(), "bash", "-lc", snippet.Command)
		proc.Stdout = cmd.OutOrStdout()
		proc.Stderr = cmd.ErrOrStderr()
		proc.Stdin = cmd.InOrStdin()
		return proc.Run()
	},
}

var snippetSetCommand string
var snippetSetDescription string

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
}

func validateSnippetName(name string) (string, error) {
	name = normalizeSnippetName(name)
	if name == "" {
		return "", fmt.Errorf("snippet name cannot be empty")
	}
	if !snippetNameRE.MatchString(name) {
		return "", fmt.Errorf("snippet name %q is invalid: use letters, numbers, dot, underscore, or hyphen", name)
	}
	return name, nil
}

func normalizeSnippetName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func sortedSnippetKeys(m map[string]config.Snippet) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func snippetDescription(s config.Snippet) string {
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

func loadResolvedSnippets() (map[string]config.Snippet, error) {
	cfg, err := config.Load(globalFlags.APIKey)
	if err != nil {
		return nil, err
	}
	return config.NormalizeSnippets(cfg.Snippets), nil
}

func loadSnippetOwnerConfig(name string) (*config.File, string, error) {
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
		snippets := config.NormalizeSnippets(f.Snippets)
		if _, ok := snippets[name]; ok {
			f.Snippets = snippets
			return f, path, nil
		}
	}
	return nil, "", fmt.Errorf("snippet %q not found", name)
}
