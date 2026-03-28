// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

// Package config handles loading and resolving reserve configuration.
// Resolution order (first non-empty value wins):
//  1. CLI flag --api-key
//  2. Environment variable FRED_API_KEY
//  3. config.json in the current working directory
//  4. config.json in the per-user config directory
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultConfigFile  = "config.json"
	DefaultFormat      = "table"
	DefaultTimeout     = 30 * time.Second
	DefaultConcurrency = 8
	DefaultRate        = 5.0
	EnvAPIKey          = "FRED_API_KEY"
	EnvDBPath          = "RESERVE_DB_PATH"
)

type configCandidate struct {
	path   string
	exists bool
}

// File is the on-disk representation of config.json.
type File struct {
	APIKey        string  `json:"api_key"`
	DefaultFormat string  `json:"default_format"`
	Timeout       string  `json:"timeout"`
	Concurrency   int     `json:"concurrency"`
	Rate          float64 `json:"rate"`
	BaseURL       string  `json:"base_url"`
	DBPath        string  `json:"db_path"`
}

// Config is the fully-resolved runtime configuration.
// All callers use this struct; the File is only read during loading.
type Config struct {
	APIKey      string
	Format      string
	Timeout     time.Duration
	Concurrency int
	Rate        float64
	BaseURL     string
	DBPath      string
	ConfigPath  string // path of the config.json that was loaded (empty if none found)

	// Runtime overrides set from CLI flags after Load()
	NoCache bool
	Refresh bool
	Quiet   bool
	Verbose bool
	Debug   bool
}

// Load resolves configuration from all sources.
// flagAPIKey is the value of --api-key (empty string if not set).
func Load(flagAPIKey string) (*Config, error) {
	cfg := &Config{
		Format:      DefaultFormat,
		Timeout:     DefaultTimeout,
		Concurrency: DefaultConcurrency,
		Rate:        DefaultRate,
		BaseURL:     "https://api.stlouisfed.org/fred/",
	}

	// Layer 1: per-user config.json (lowest file priority)
	if f, path, err := loadUserFile(); err == nil {
		applyFile(cfg, f, path)
	}

	// Layer 2: local config.json overrides per-user config
	if f, path, err := loadLocalFile(); err == nil {
		applyFile(cfg, f, path)
	}

	// Layer 3: environment variable
	if v := os.Getenv(EnvAPIKey); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv(EnvDBPath); v != "" {
		cfg.DBPath = v
	}

	// Layer 4: CLI flag (highest priority)
	if flagAPIKey != "" {
		cfg.APIKey = flagAPIKey
	}

	// Set default DB path if still unset
	if cfg.DBPath == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.DBPath = filepath.Join(home, ".reserve", "reserve.db")
		}
	}

	return cfg, nil
}

// Validate returns an error if required fields are missing.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return errors.New(
			"API key not found.\n\n" +
				"Set it one of these ways:\n" +
				"  1. CLI flag:        reserve --api-key YOUR_KEY ...\n" +
				"  2. Environment:     export FRED_API_KEY=YOUR_KEY\n" +
				"  3. config.json:     {\"api_key\": \"YOUR_KEY\"}\n\n" +
				"Get a free key at https://fred.stlouisfed.org/docs/api/api_key.html",
		)
	}
	return nil
}

// RedactedAPIKey returns the API key with most characters replaced by asterisks.
// Safe for logging and display.
func (c *Config) RedactedAPIKey() string {
	if len(c.APIKey) <= 4 {
		return "****"
	}
	return c.APIKey[:2] + "****" + c.APIKey[len(c.APIKey)-2:]
}

// UserConfigPath returns the per-user config.json path for reserve.
func UserConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("resolve user config dir: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "reserve", DefaultConfigFile), nil
}

// PreferredConfigPath returns the path config init/set should target by default.
// A local config.json wins when present; otherwise the per-user config path is used.
func PreferredConfigPath() (string, error) {
	local, err := localConfigCandidate()
	if err != nil {
		return "", err
	}
	if local.exists {
		return local.path, nil
	}
	return UserConfigPath()
}

func loadLocalFile() (*File, string, error) {
	candidate, err := localConfigCandidate()
	if err != nil {
		return nil, "", err
	}
	return loadFile(candidate)
}

func loadUserFile() (*File, string, error) {
	candidate, err := userConfigCandidate()
	if err != nil {
		return nil, "", err
	}
	return loadFile(candidate)
}

func localConfigCandidate() (configCandidate, error) {
	path, err := filepath.Abs(DefaultConfigFile)
	if err != nil {
		return configCandidate{}, err
	}
	_, statErr := os.Stat(path)
	if statErr == nil {
		return configCandidate{path: path, exists: true}, nil
	}
	if os.IsNotExist(statErr) {
		return configCandidate{path: path, exists: false}, nil
	}
	return configCandidate{}, statErr
}

func userConfigCandidate() (configCandidate, error) {
	path, err := UserConfigPath()
	if err != nil {
		return configCandidate{}, err
	}
	_, statErr := os.Stat(path)
	if statErr == nil {
		return configCandidate{path: path, exists: true}, nil
	}
	if os.IsNotExist(statErr) {
		return configCandidate{path: path, exists: false}, nil
	}
	return configCandidate{}, statErr
}

func loadFile(candidate configCandidate) (*File, string, error) {
	if !candidate.exists {
		return nil, "", fmt.Errorf("config.json not found at %s", candidate.path)
	}
	path := candidate.path
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading config.json: %w", err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, "", fmt.Errorf("parsing config.json: %w", err)
	}
	return &f, path, nil
}

// applyFile copies values from a parsed File into cfg,
// skipping any fields that are zero/empty.
func applyFile(cfg *Config, f *File, path string) {
	cfg.ConfigPath = path
	if f.APIKey != "" {
		cfg.APIKey = f.APIKey
	}
	if f.DefaultFormat != "" {
		cfg.Format = f.DefaultFormat
	}
	if f.Timeout != "" {
		if d, err := time.ParseDuration(f.Timeout); err == nil {
			cfg.Timeout = d
		}
	}
	if f.Concurrency > 0 {
		cfg.Concurrency = f.Concurrency
	}
	if f.Rate > 0 {
		cfg.Rate = f.Rate
	}
	if f.BaseURL != "" {
		cfg.BaseURL = f.BaseURL
	}
	if f.DBPath != "" {
		cfg.DBPath = f.DBPath
	}
}

// Template returns a File populated with sensible defaults, suitable for
// writing an initial config.json via `reserve config init`.
func Template() File {
	return File{
		APIKey:        "",
		DefaultFormat: "table",
		Timeout:       "30s",
		Concurrency:   DefaultConcurrency,
		Rate:          DefaultRate,
		BaseURL:       "https://api.stlouisfed.org/fred/",
	}
}

// WriteFile serialises a File to the given path.
func WriteFile(path string, f File) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
