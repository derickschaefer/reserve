// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage reserve configuration",
	Long: `Read and write reserve configuration stored in the user config directory or a local config.json override.

This command family also manages explicit per-series permission overrides for
FRED series where you have independently obtained permission to use the data.
Only grant overrides for series you are actually authorized to use.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a template config.json in the user config directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.UserConfigPath()
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config.json already exists at %s (delete it first to re-initialise)", path)
		}
		tmpl := config.Template()
		if err := config.WriteFile(path, tmpl); err != nil {
			return err
		}
		fmt.Printf("✓ Created %s\n", path)
		fmt.Println("  Edit it and set your api_key to get started.")
		fmt.Println("  Get a free key at: https://fred.stlouisfed.org/docs/api/api_key.html")
		return nil
	},
}

var configGetShowSecrets bool

var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Print the current resolved configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(globalFlags.APIKey)
		if err != nil {
			return err
		}

		apiKey := cfg.RedactedAPIKey()
		if configGetShowSecrets {
			apiKey = cfg.APIKey
		}
		if apiKey == "" {
			apiKey = "(not set)"
		}

		src := "(not found)"
		if cfg.ConfigPath != "" {
			src = cfg.ConfigPath
		}

		format := cfg.Format
		if globalFlags.Format != "" {
			format = globalFlags.Format
		}

		data := []model.SeriesMeta{} // use generic table for config
		_ = data

		result := &model.Result{
			Kind:        model.KindTable,
			GeneratedAt: time.Now(),
			Command:     "config get",
		}

		switch format {
		case render.FormatJSON:
			w, closeFn, err := outputWriter(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			defer closeFn()

			type configOut struct {
				APIKey                             string         `json:"api_key"`
				Format                             string         `json:"default_format"`
				Timeout                            string         `json:"timeout"`
				Concurrency                        int            `json:"concurrency"`
				Rate                               float64        `json:"rate"`
				BaseURL                            string         `json:"base_url"`
				DBPath                             string         `json:"db_path"`
				PersonOrgType                      string         `json:"person_org_type"`
				BlockUnknownRights                 bool           `json:"block_unknown_rights"`
				BlockAmbiguousRights               bool           `json:"block_ambiguous_rights"`
				BlockPreapprovalRequiredCommercial bool           `json:"block_preapproval_required_in_commercial"`
				RequireCitationOnDisplay           bool           `json:"require_citation_on_display"`
				RequireCitationOnExport            bool           `json:"require_citation_on_export"`
				AllowOverrideWithPermissionRecord  bool           `json:"allow_override_with_permission_record"`
				GrantedSeriesPermissions           []string       `json:"granted_series_permissions,omitempty"`
				RightsRefreshDays                  map[string]int `json:"rights_refresh_days"`
				LogComplianceDecisions             bool           `json:"log_compliance_decisions"`
				ConfigFile                         string         `json:"config_file"`
			}
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(configOut{
				APIKey:                             apiKey,
				Format:                             cfg.Format,
				Timeout:                            cfg.Timeout.String(),
				Concurrency:                        cfg.Concurrency,
				Rate:                               cfg.Rate,
				BaseURL:                            cfg.BaseURL,
				DBPath:                             cfg.DBPath,
				PersonOrgType:                      cfg.PersonOrgType,
				BlockUnknownRights:                 cfg.BlockUnknownRights,
				BlockAmbiguousRights:               cfg.BlockAmbiguousRights,
				BlockPreapprovalRequiredCommercial: cfg.BlockPreapprovalRequiredInCommercial,
				RequireCitationOnDisplay:           cfg.RequireCitationOnDisplay,
				RequireCitationOnExport:            cfg.RequireCitationOnExport,
				AllowOverrideWithPermissionRecord:  cfg.AllowOverrideWithPermissionRecord,
				GrantedSeriesPermissions:           cfg.GrantedSeriesPermissions,
				RightsRefreshDays:                  cfg.RightsRefreshDays,
				LogComplianceDecisions:             cfg.LogComplianceDecisions,
				ConfigFile:                         src,
			})
		default:
			_ = result
			w, closeFn, err := outputWriter(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			defer closeFn()

			rows := [][]string{
				{"api_key", apiKey},
				{"default_format", cfg.Format},
				{"timeout", cfg.Timeout.String()},
				{"concurrency", fmt.Sprintf("%d", cfg.Concurrency)},
				{"rate", fmt.Sprintf("%.1f req/s", cfg.Rate)},
				{"base_url", cfg.BaseURL},
				{"db_path", cfg.DBPath},
				{"person_org_type", cfg.PersonOrgType},
				{"block_unknown_rights", fmt.Sprintf("%t", cfg.BlockUnknownRights)},
				{"block_ambiguous_rights", fmt.Sprintf("%t", cfg.BlockAmbiguousRights)},
				{"block_preapproval_required_in_commercial", fmt.Sprintf("%t", cfg.BlockPreapprovalRequiredInCommercial)},
				{"require_citation_on_display", fmt.Sprintf("%t", cfg.RequireCitationOnDisplay)},
				{"require_citation_on_export", fmt.Sprintf("%t", cfg.RequireCitationOnExport)},
				{"allow_override_with_permission_record", fmt.Sprintf("%t", cfg.AllowOverrideWithPermissionRecord)},
				{"granted_series_permissions", strings.Join(cfg.GrantedSeriesPermissions, ", ")},
				{"rights_refresh_days.default", fmt.Sprintf("%d", cfg.RightsRefreshDaysFor("default"))},
				{"rights_refresh_days.export", fmt.Sprintf("%d", cfg.RightsRefreshDaysFor("export"))},
				{"rights_refresh_days.publish", fmt.Sprintf("%d", cfg.RightsRefreshDaysFor("publish"))},
				{"log_compliance_decisions", fmt.Sprintf("%t", cfg.LogComplianceDecisions)},
				{"config_file", src},
			}
			printKVTableTo(w, rows)
			return nil
		}
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value in config.json",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := strings.ToLower(args[0])
		val := args[1]

		// Load existing file or start from template
		var f config.File
		existing, path, err := loadConfigFile()
		if err != nil {
			path, err = config.PreferredConfigPath()
			if err != nil {
				return err
			}
			f = config.Template()
		} else {
			f = *existing
		}

		switch key {
		case "api_key":
			f.APIKey = val
		case "default_format", "format":
			f.DefaultFormat = val
		case "timeout":
			f.Timeout = val
		case "concurrency":
			var n int
			if _, err := fmt.Sscanf(val, "%d", &n); err != nil {
				return fmt.Errorf("concurrency must be an integer")
			}
			f.Concurrency = n
		case "rate":
			var r float64
			if _, err := fmt.Sscanf(val, "%f", &r); err != nil {
				return fmt.Errorf("rate must be a number")
			}
			f.Rate = r
		case "base_url":
			f.BaseURL = val
		case "db_path":
			f.DBPath = val
		case "person_org_type":
			f.PersonOrgType = val
		case "block_unknown_rights":
			if err := setConfigBool(&f.BlockUnknownRights, key, val); err != nil {
				return err
			}
		case "block_ambiguous_rights":
			if err := setConfigBool(&f.BlockAmbiguousRights, key, val); err != nil {
				return err
			}
		case "block_preapproval_required_in_commercial":
			if err := setConfigBool(&f.BlockPreapprovalRequiredInCommercial, key, val); err != nil {
				return err
			}
		case "require_citation_on_display":
			if err := setConfigBool(&f.RequireCitationOnDisplay, key, val); err != nil {
				return err
			}
		case "require_citation_on_export":
			if err := setConfigBool(&f.RequireCitationOnExport, key, val); err != nil {
				return err
			}
		case "allow_override_with_permission_record":
			if err := setConfigBool(&f.AllowOverrideWithPermissionRecord, key, val); err != nil {
				return err
			}
		case "log_compliance_decisions":
			if err := setConfigBool(&f.LogComplianceDecisions, key, val); err != nil {
				return err
			}
		case "rights_refresh_days.default":
			if err := setConfigIntMap(&f.RightsRefreshDays, "default", key, val); err != nil {
				return err
			}
		case "rights_refresh_days.export":
			if err := setConfigIntMap(&f.RightsRefreshDays, "export", key, val); err != nil {
				return err
			}
		case "rights_refresh_days.publish":
			if err := setConfigIntMap(&f.RightsRefreshDays, "publish", key, val); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown config key: %q", key)
		}

		if err := config.WriteFile(path, f); err != nil {
			return err
		}
		fmt.Printf("✓ Set %s in %s\n", key, path)
		return nil
	},
}

var configGrantCmd = &cobra.Command{
	Use:   "grant <SERIES_ID>",
	Short: "Grant local permission override for a specific series",
	Long: `Grant a local permission override for one specific series by adding its
series ID to granted_series_permissions in config.json.

Use this only when you already have proper permission to use the series.`,
	Example: `  reserve config grant BAMLC0A0CM`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, path, err := loadOrTemplateConfig()
		if err != nil {
			return err
		}
		seriesID := strings.ToUpper(strings.TrimSpace(args[0]))
		f.GrantedSeriesPermissions = append(f.GrantedSeriesPermissions, seriesID)
		if err := config.WriteFile(path, *f); err != nil {
			return err
		}
		fmt.Printf("✓ Granted local permission override for %s in %s\n", seriesID, path)
		return nil
	},
}

var configRevokeCmd = &cobra.Command{
	Use:   "revoke <SERIES_ID>",
	Short: "Remove local permission override for a specific series",
	Long: `Remove a local permission override for one specific series from
granted_series_permissions in config.json.`,
	Example: `  reserve config revoke BAMLC0A0CM`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, path, err := loadOrTemplateConfig()
		if err != nil {
			return err
		}
		seriesID := strings.ToUpper(strings.TrimSpace(args[0]))
		var kept []string
		for _, id := range f.GrantedSeriesPermissions {
			if strings.ToUpper(strings.TrimSpace(id)) != seriesID {
				kept = append(kept, id)
			}
		}
		f.GrantedSeriesPermissions = kept
		if err := config.WriteFile(path, *f); err != nil {
			return err
		}
		fmt.Printf("✓ Revoked local permission override for %s in %s\n", seriesID, path)
		return nil
	},
}

var configListGrantsCmd = &cobra.Command{
	Use:     "list-grants",
	Short:   "List locally granted series permission overrides",
	Long:    `List the series IDs currently present in granted_series_permissions in config.json.`,
	Example: `  reserve config list-grants`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(globalFlags.APIKey)
		if err != nil {
			return err
		}
		if len(cfg.GrantedSeriesPermissions) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No granted series permission overrides.")
			return nil
		}
		for _, id := range cfg.GrantedSeriesPermissions {
			fmt.Fprintln(cmd.OutOrStdout(), id)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGrantCmd)
	configCmd.AddCommand(configRevokeCmd)
	configCmd.AddCommand(configListGrantsCmd)

	configGetCmd.Flags().BoolVar(&configGetShowSecrets, "show-secrets", false, "show API key in plain text")
}

// loadConfigFile reads the preferred config.json for editing:
// local override in cwd when present, otherwise the per-user config file.
func loadConfigFile() (*config.File, string, error) {
	path, err := config.PreferredConfigPath()
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var f config.File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, "", err
	}
	return &f, path, nil
}

func loadOrTemplateConfig() (*config.File, string, error) {
	existing, path, err := loadConfigFile()
	if err == nil {
		return existing, path, nil
	}
	path, err = config.PreferredConfigPath()
	if err != nil {
		return nil, "", err
	}
	f := config.Template()
	return &f, path, nil
}

// printKVTable renders a two-column key/value table to stdout using aligned columns.
func printKVTable(rows [][]string) {
	printKVTableTo(os.Stdout, rows)
}

func printKVTableTo(w io.Writer, rows [][]string) {
	maxKey := 0
	for _, r := range rows {
		if len(r[0]) > maxKey {
			maxKey = len(r[0])
		}
	}
	for _, r := range rows {
		padding := strings.Repeat(" ", maxKey-len(r[0]))
		fmt.Fprintf(w, "  %s%s  %s\n", r[0], padding, r[1])
	}
}

func setConfigBool(dst *bool, key, val string) error {
	switch strings.ToLower(val) {
	case "true":
		*dst = true
	case "false":
		*dst = false
	default:
		return fmt.Errorf("%s must be true or false", key)
	}
	return nil
}

func setConfigIntMap(dst *map[string]int, nestedKey, key, val string) error {
	var n int
	if _, err := fmt.Sscanf(val, "%d", &n); err != nil || n <= 0 {
		return fmt.Errorf("%s must be a positive integer", key)
	}
	if *dst == nil {
		*dst = map[string]int{}
	}
	(*dst)[nestedKey] = n
	return nil
}
