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
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	DefaultPersonOrg   = "student"
)

var defaultRightsRefreshDays = map[string]int{
	"default": 30,
	"export":  7,
	"publish": 7,
}

type configCandidate struct {
	path   string
	exists bool
}

// File is the on-disk representation of config.json.
type File struct {
	APIKey                               string         `json:"api_key"`
	DefaultFormat                        string         `json:"default_format"`
	Timeout                              string         `json:"timeout"`
	Concurrency                          int            `json:"concurrency"`
	Rate                                 float64        `json:"rate"`
	BaseURL                              string         `json:"base_url"`
	DBPath                               string         `json:"db_path"`
	PersonOrgType                        string         `json:"person_org_type"`
	BlockUnknownRights                   bool           `json:"block_unknown_rights"`
	BlockAmbiguousRights                 bool           `json:"block_ambiguous_rights"`
	BlockPreapprovalRequiredInCommercial bool           `json:"block_preapproval_required_in_commercial"`
	RequireCitationOnDisplay             bool           `json:"require_citation_on_display"`
	RequireCitationOnExport              bool           `json:"require_citation_on_export"`
	AllowOverrideWithPermissionRecord    bool           `json:"allow_override_with_permission_record"`
	GrantedSeriesPermissions             []string       `json:"granted_series_permissions,omitempty"`
	RightsRefreshDays                    map[string]int `json:"rights_refresh_days"`
	LogComplianceDecisions               bool           `json:"log_compliance_decisions"`
}

// Config is the fully-resolved runtime configuration.
// All callers use this struct; the File is only read during loading.
type Config struct {
	APIKey                               string
	Format                               string
	Timeout                              time.Duration
	Concurrency                          int
	Rate                                 float64
	BaseURL                              string
	DBPath                               string
	PersonOrgType                        string
	BlockUnknownRights                   bool
	BlockAmbiguousRights                 bool
	BlockPreapprovalRequiredInCommercial bool
	RequireCitationOnDisplay             bool
	RequireCitationOnExport              bool
	AllowOverrideWithPermissionRecord    bool
	GrantedSeriesPermissions             []string
	RightsRefreshDays                    map[string]int
	LogComplianceDecisions               bool
	ConfigPath                           string // path of the config.json that was loaded (empty if none found)

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
		Format:                               DefaultFormat,
		Timeout:                              DefaultTimeout,
		Concurrency:                          DefaultConcurrency,
		Rate:                                 DefaultRate,
		BaseURL:                              "https://api.stlouisfed.org/fred/",
		PersonOrgType:                        DefaultPersonOrg,
		BlockUnknownRights:                   true,
		BlockAmbiguousRights:                 true,
		BlockPreapprovalRequiredInCommercial: true,
		RequireCitationOnDisplay:             true,
		RequireCitationOnExport:              true,
		AllowOverrideWithPermissionRecord:    true,
		RightsRefreshDays:                    cloneRightsRefreshDays(defaultRightsRefreshDays),
		LogComplianceDecisions:               true,
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

	if err := validateRuntime(cfg); err != nil {
		return nil, err
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
	f, err := parseFile(data)
	if err != nil {
		return nil, "", fmt.Errorf("parsing config.json: %w", err)
	}
	raw, err := parseRawConfig(data)
	if err != nil {
		return nil, "", fmt.Errorf("parsing config.json: %w", err)
	}
	if needsUpgrade(raw) {
		if err := upgradeFileInPlace(path, data, f); err != nil {
			return nil, "", err
		}
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
	if f.PersonOrgType != "" {
		cfg.PersonOrgType = f.PersonOrgType
	}
	cfg.BlockUnknownRights = f.BlockUnknownRights
	cfg.BlockAmbiguousRights = f.BlockAmbiguousRights
	cfg.BlockPreapprovalRequiredInCommercial = f.BlockPreapprovalRequiredInCommercial
	cfg.RequireCitationOnDisplay = f.RequireCitationOnDisplay
	cfg.RequireCitationOnExport = f.RequireCitationOnExport
	cfg.AllowOverrideWithPermissionRecord = f.AllowOverrideWithPermissionRecord
	cfg.GrantedSeriesPermissions = normalizeSeriesIDs(f.GrantedSeriesPermissions)
	if len(f.RightsRefreshDays) > 0 {
		cfg.RightsRefreshDays = cloneRightsRefreshDays(f.RightsRefreshDays)
	}
	cfg.LogComplianceDecisions = f.LogComplianceDecisions
}

// Template returns a File populated with sensible defaults, suitable for
// writing an initial config.json via `reserve config init`.
func Template() File {
	return File{
		APIKey:                               "",
		DefaultFormat:                        "table",
		Timeout:                              "30s",
		Concurrency:                          DefaultConcurrency,
		Rate:                                 DefaultRate,
		BaseURL:                              "https://api.stlouisfed.org/fred/",
		PersonOrgType:                        DefaultPersonOrg,
		BlockUnknownRights:                   true,
		BlockAmbiguousRights:                 true,
		BlockPreapprovalRequiredInCommercial: true,
		RequireCitationOnDisplay:             true,
		RequireCitationOnExport:              true,
		AllowOverrideWithPermissionRecord:    true,
		RightsRefreshDays:                    cloneRightsRefreshDays(defaultRightsRefreshDays),
		LogComplianceDecisions:               true,
	}
}

// WriteFile serialises a File to the given path.
func WriteFile(path string, f File) error {
	f = canonicalizeFile(f)
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func (c *Config) RightsRefreshDaysFor(action string) int {
	if days, ok := c.RightsRefreshDays[action]; ok && days > 0 {
		return days
	}
	if days, ok := c.RightsRefreshDays["default"]; ok && days > 0 {
		return days
	}
	return defaultRightsRefreshDays["default"]
}

func (c *Config) HasGrantedSeriesPermission(seriesID string) bool {
	seriesID = strings.ToUpper(strings.TrimSpace(seriesID))
	for _, id := range c.GrantedSeriesPermissions {
		if id == seriesID {
			return true
		}
	}
	return false
}

func cloneRightsRefreshDays(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeSeriesIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.ToUpper(strings.TrimSpace(id))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	slices.Sort(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseRawConfig(data []byte) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func parseFile(data []byte) (File, error) {
	raw, err := parseRawConfig(data)
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return File{}, err
	}
	f = withMissingDefaults(raw, f)
	if err := validateFile(f); err != nil {
		return File{}, err
	}
	return canonicalizeFile(f), nil
}

func canonicalizeFile(f File) File {
	if f.PersonOrgType == "" {
		f.PersonOrgType = DefaultPersonOrg
	}
	f.GrantedSeriesPermissions = normalizeSeriesIDs(f.GrantedSeriesPermissions)
	f.RightsRefreshDays = mergeRightsRefreshDays(f.RightsRefreshDays)
	return f
}

func withMissingDefaults(raw map[string]json.RawMessage, f File) File {
	legacyMissingComplianceBlock := strings.TrimSpace(f.PersonOrgType) == "" &&
		len(f.RightsRefreshDays) == 0 &&
		!f.BlockUnknownRights &&
		!f.BlockAmbiguousRights &&
		!f.BlockPreapprovalRequiredInCommercial &&
		!f.RequireCitationOnDisplay &&
		!f.RequireCitationOnExport &&
		!f.AllowOverrideWithPermissionRecord &&
		!f.LogComplianceDecisions

	if _, ok := raw["person_org_type"]; !ok {
		f.PersonOrgType = DefaultPersonOrg
	} else if strings.TrimSpace(f.PersonOrgType) == "" {
		f.PersonOrgType = DefaultPersonOrg
	}
	if _, ok := raw["block_unknown_rights"]; !ok || legacyMissingComplianceBlock {
		f.BlockUnknownRights = true
	}
	if _, ok := raw["block_ambiguous_rights"]; !ok || legacyMissingComplianceBlock {
		f.BlockAmbiguousRights = true
	}
	if _, ok := raw["block_preapproval_required_in_commercial"]; !ok || legacyMissingComplianceBlock {
		f.BlockPreapprovalRequiredInCommercial = true
	}
	if _, ok := raw["require_citation_on_display"]; !ok || legacyMissingComplianceBlock {
		f.RequireCitationOnDisplay = true
	}
	if _, ok := raw["require_citation_on_export"]; !ok || legacyMissingComplianceBlock {
		f.RequireCitationOnExport = true
	}
	if _, ok := raw["allow_override_with_permission_record"]; !ok || legacyMissingComplianceBlock {
		f.AllowOverrideWithPermissionRecord = true
	}
	if _, ok := raw["log_compliance_decisions"]; !ok || legacyMissingComplianceBlock {
		f.LogComplianceDecisions = true
	}
	if _, ok := raw["rights_refresh_days"]; !ok || legacyMissingComplianceBlock {
		f.RightsRefreshDays = cloneRightsRefreshDays(defaultRightsRefreshDays)
	} else {
		f.RightsRefreshDays = mergeRightsRefreshDays(f.RightsRefreshDays)
	}
	return f
}

func mergeRightsRefreshDays(in map[string]int) map[string]int {
	out := cloneRightsRefreshDays(defaultRightsRefreshDays)
	for k, v := range in {
		if v > 0 {
			out[k] = v
		}
	}
	return out
}

func validateRuntime(cfg *Config) error {
	if !slices.Contains([]string{"commercial", "personal", "student"}, cfg.PersonOrgType) {
		return fmt.Errorf("config.json: person_org_type must be one of commercial, personal, student")
	}
	for action, days := range cfg.RightsRefreshDays {
		if days <= 0 {
			return fmt.Errorf("config.json: rights_refresh_days.%s must be > 0", action)
		}
	}
	return nil
}

func validateFile(f File) error {
	f = canonicalizeFile(f)
	cfg := &Config{
		PersonOrgType:     f.PersonOrgType,
		RightsRefreshDays: mergeRightsRefreshDays(f.RightsRefreshDays),
	}
	return validateRuntime(cfg)
}

func needsUpgrade(raw map[string]json.RawMessage) bool {
	required := []string{
		"person_org_type",
		"block_unknown_rights",
		"block_ambiguous_rights",
		"block_preapproval_required_in_commercial",
		"require_citation_on_display",
		"require_citation_on_export",
		"allow_override_with_permission_record",
		"log_compliance_decisions",
		"rights_refresh_days",
	}
	for _, key := range required {
		if _, ok := raw[key]; !ok {
			return true
		}
	}
	if rawDays, ok := raw["rights_refresh_days"]; ok {
		var days map[string]int
		if err := json.Unmarshal(rawDays, &days); err != nil {
			return true
		}
		for key := range defaultRightsRefreshDays {
			if _, ok := days[key]; !ok {
				return true
			}
		}
	}
	return false
}

func upgradeFileInPlace(path string, original []byte, upgraded File) error {
	bakPath := path + ".bak"
	if err := os.WriteFile(bakPath, original, 0600); err != nil {
		return fmt.Errorf("writing config backup: %w", err)
	}
	restore := func(cause error) error {
		_ = os.WriteFile(path, original, 0600)
		return cause
	}

	if err := validateFile(upgraded); err != nil {
		return restore(fmt.Errorf("validating config upgrade: %w", err))
	}
	if err := WriteFile(path, upgraded); err != nil {
		return restore(fmt.Errorf("writing upgraded config: %w", err))
	}
	reloaded, err := os.ReadFile(path)
	if err != nil {
		return restore(fmt.Errorf("re-reading upgraded config: %w", err))
	}
	if _, err := parseFile(reloaded); err != nil {
		return restore(fmt.Errorf("validating upgraded config: %w", err))
	}
	if err := os.Remove(bakPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing config backup: %w", err)
	}
	return nil
}
