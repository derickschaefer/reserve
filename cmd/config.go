package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage reserve configuration",
	Long:  `Read and write reserve configuration stored in config.json.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a template config.json in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.DefaultConfigFile
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
			type configOut struct {
				APIKey      string  `json:"api_key"`
				Format      string  `json:"default_format"`
				Timeout     string  `json:"timeout"`
				Concurrency int     `json:"concurrency"`
				Rate        float64 `json:"rate"`
				BaseURL     string  `json:"base_url"`
				DBPath      string  `json:"db_path"`
				ConfigFile  string  `json:"config_file"`
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(configOut{
				APIKey:      apiKey,
				Format:      cfg.Format,
				Timeout:     cfg.Timeout.String(),
				Concurrency: cfg.Concurrency,
				Rate:        cfg.Rate,
				BaseURL:     cfg.BaseURL,
				DBPath:      cfg.DBPath,
				ConfigFile:  src,
			})
		default:
			_ = result
			rows := [][]string{
				{"api_key", apiKey},
				{"default_format", cfg.Format},
				{"timeout", cfg.Timeout.String()},
				{"concurrency", fmt.Sprintf("%d", cfg.Concurrency)},
				{"rate", fmt.Sprintf("%.1f req/s", cfg.Rate)},
				{"base_url", cfg.BaseURL},
				{"db_path", cfg.DBPath},
				{"config_file", src},
			}
			printKVTable(rows)
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
			path = config.DefaultConfigFile
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
		default:
			return fmt.Errorf("unknown config key: %q\n\nValid keys: api_key, default_format, timeout, concurrency, rate, base_url, db_path", key)
		}

		if err := config.WriteFile(path, f); err != nil {
			return err
		}
		fmt.Printf("✓ Set %s in %s\n", key, path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)

	configGetCmd.Flags().BoolVar(&configGetShowSecrets, "show-secrets", false, "show API key in plain text")
}

// loadConfigFile reads config.json from cwd; used by configSetCmd.
func loadConfigFile() (*config.File, string, error) {
	path := config.DefaultConfigFile
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

// printKVTable renders a two-column key/value table to stdout using aligned columns.
func printKVTable(rows [][]string) {
	maxKey := 0
	for _, r := range rows {
		if len(r[0]) > maxKey {
			maxKey = len(r[0])
		}
	}
	for _, r := range rows {
		padding := strings.Repeat(" ", maxKey-len(r[0]))
		fmt.Printf("  %s%s  %s\n", r[0], padding, r[1])
	}
}
