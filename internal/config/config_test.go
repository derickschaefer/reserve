package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/derickschaefer/reserve/internal/config"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// writeConfig writes a config.json into dir and changes the working directory
// to dir for the duration of the test. Returns a cleanup function (already
// registered via t.Cleanup, but returned for early use if needed).
func writeConfig(t *testing.T, dir string, f config.File) {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	// Change working directory so config.Load() finds config.json
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// clearEnv unsets FRED_API_KEY and RESERVE_DB_PATH for the duration of the test.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvAPIKey, "")
	t.Setenv(config.EnvDBPath, "")
}

// ─── Defaults ─────────────────────────────────────────────────────────────────

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	clearEnv(t)
	// Change to temp dir so no config.json is found
	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() { _ = os.Chdir(orig) })

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Format != config.DefaultFormat {
		t.Errorf("Format: expected %q, got %q", config.DefaultFormat, cfg.Format)
	}
	if cfg.Timeout != config.DefaultTimeout {
		t.Errorf("Timeout: expected %v, got %v", config.DefaultTimeout, cfg.Timeout)
	}
	if cfg.Concurrency != config.DefaultConcurrency {
		t.Errorf("Concurrency: expected %d, got %d", config.DefaultConcurrency, cfg.Concurrency)
	}
	if cfg.Rate != config.DefaultRate {
		t.Errorf("Rate: expected %g, got %g", config.DefaultRate, cfg.Rate)
	}
	if cfg.BaseURL == "" {
		t.Error("BaseURL should have a default value")
	}
	if cfg.DBPath == "" {
		t.Error("DBPath should have a default (home dir based) value")
	}
}

// ─── Config file loading ──────────────────────────────────────────────────────

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	clearEnv(t)
	writeConfig(t, dir, config.File{
		APIKey:        "filekey123",
		DefaultFormat: "json",
		Timeout:       "60s",
		Concurrency:   4,
		Rate:          2.5,
		BaseURL:       "https://custom.example.com/",
		DBPath:        "/tmp/test.db",
	})

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.APIKey != "filekey123" {
		t.Errorf("APIKey: expected filekey123, got %q", cfg.APIKey)
	}
	if cfg.Format != "json" {
		t.Errorf("Format: expected json, got %q", cfg.Format)
	}
	if cfg.Timeout.String() != "1m0s" {
		t.Errorf("Timeout: expected 1m0s, got %q", cfg.Timeout.String())
	}
	if cfg.Concurrency != 4 {
		t.Errorf("Concurrency: expected 4, got %d", cfg.Concurrency)
	}
	if cfg.Rate != 2.5 {
		t.Errorf("Rate: expected 2.5, got %g", cfg.Rate)
	}
	if cfg.BaseURL != "https://custom.example.com/" {
		t.Errorf("BaseURL: expected custom URL, got %q", cfg.BaseURL)
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("DBPath: expected /tmp/test.db, got %q", cfg.DBPath)
	}
}

func TestLoadConfigPathRecorded(t *testing.T) {
	dir := t.TempDir()
	clearEnv(t)
	writeConfig(t, dir, config.File{APIKey: "k"})

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ConfigPath == "" {
		t.Error("ConfigPath should be set when config.json is found")
	}
	if !strings.Contains(cfg.ConfigPath, "config.json") {
		t.Errorf("ConfigPath should contain config.json, got %q", cfg.ConfigPath)
	}
}

func TestLoadNoConfigFile(t *testing.T) {
	dir := t.TempDir()
	clearEnv(t)
	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() { _ = os.Chdir(orig) })

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load without config.json should not error: %v", err)
	}
	if cfg.ConfigPath != "" {
		t.Errorf("ConfigPath should be empty when no file found, got %q", cfg.ConfigPath)
	}
}

func TestLoadInvalidTimeoutIgnored(t *testing.T) {
	// Invalid timeout string in file should be ignored, not error
	dir := t.TempDir()
	clearEnv(t)
	writeConfig(t, dir, config.File{
		APIKey:  "k",
		Timeout: "not-a-duration",
	})

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Should fall back to default timeout
	if cfg.Timeout != config.DefaultTimeout {
		t.Errorf("invalid timeout should use default %v, got %v", config.DefaultTimeout, cfg.Timeout)
	}
}

// ─── Environment variable priority ───────────────────────────────────────────

func TestLoadEnvAPIKeyOverridesFile(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, config.File{APIKey: "filekey"})
	t.Setenv(config.EnvAPIKey, "envkey")
	t.Setenv(config.EnvDBPath, "")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "envkey" {
		t.Errorf("env FRED_API_KEY should override file: expected envkey, got %q", cfg.APIKey)
	}
}

func TestLoadEnvDBPath(t *testing.T) {
	dir := t.TempDir()
	clearEnv(t)
	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() { _ = os.Chdir(orig) })
	t.Setenv(config.EnvDBPath, "/custom/path/reserve.db")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBPath != "/custom/path/reserve.db" {
		t.Errorf("RESERVE_DB_PATH: expected /custom/path/reserve.db, got %q", cfg.DBPath)
	}
}

// ─── CLI flag priority ────────────────────────────────────────────────────────

func TestLoadFlagAPIKeyOverridesEnvAndFile(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, config.File{APIKey: "filekey"})
	t.Setenv(config.EnvAPIKey, "envkey")
	t.Setenv(config.EnvDBPath, "")

	cfg, err := config.Load("flagkey")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "flagkey" {
		t.Errorf("flag --api-key should override env and file: expected flagkey, got %q", cfg.APIKey)
	}
}

func TestLoadFlagEmptyDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	clearEnv(t)
	writeConfig(t, dir, config.File{APIKey: "filekey"})

	cfg, err := config.Load("") // empty flag = not set
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "filekey" {
		t.Errorf("empty flag should not override file value: expected filekey, got %q", cfg.APIKey)
	}
}

// ─── Validate ─────────────────────────────────────────────────────────────────

func TestValidateWithAPIKey(t *testing.T) {
	cfg := &config.Config{APIKey: "somekey"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate with API key should not error: %v", err)
	}
}

func TestValidateWithoutAPIKey(t *testing.T) {
	cfg := &config.Config{}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate without API key should return error")
	}
}

func TestValidateErrorMentionsAPIKey(t *testing.T) {
	cfg := &config.Config{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("error should mention API key, got: %v", err)
	}
}

// ─── RedactedAPIKey ───────────────────────────────────────────────────────────

func TestRedactedAPIKeyNormal(t *testing.T) {
	cfg := &config.Config{APIKey: "abcdefghij"}
	redacted := cfg.RedactedAPIKey()

	// Should preserve first 2 and last 2 characters
	if !strings.HasPrefix(redacted, "ab") {
		t.Errorf("redacted key should start with 'ab', got %q", redacted)
	}
	if !strings.HasSuffix(redacted, "ij") {
		t.Errorf("redacted key should end with 'ij', got %q", redacted)
	}
	if !strings.Contains(redacted, "****") {
		t.Errorf("redacted key should contain '****', got %q", redacted)
	}
}

func TestRedactedAPIKeyShort(t *testing.T) {
	// Keys <= 4 chars should return "****"
	for _, key := range []string{"", "a", "ab", "abc", "abcd"} {
		cfg := &config.Config{APIKey: key}
		if cfg.RedactedAPIKey() != "****" {
			t.Errorf("short key %q should redact to '****', got %q", key, cfg.RedactedAPIKey())
		}
	}
}

func TestRedactedAPIKeyNotPlaintext(t *testing.T) {
	cfg := &config.Config{APIKey: "supersecretkey123"}
	redacted := cfg.RedactedAPIKey()
	if redacted == cfg.APIKey {
		t.Error("redacted key should not equal the original")
	}
}

// ─── WriteFile / Template ─────────────────────────────────────────────────────

func TestWriteFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	f := config.File{
		APIKey:        "testkey",
		DefaultFormat: "csv",
		Timeout:       "45s",
		Concurrency:   6,
		Rate:          3.0,
		BaseURL:       "https://api.example.com/",
		DBPath:        "/data/reserve.db",
	}

	if err := config.WriteFile(path, f); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got config.File
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.APIKey != f.APIKey {
		t.Errorf("APIKey: expected %q, got %q", f.APIKey, got.APIKey)
	}
	if got.DefaultFormat != f.DefaultFormat {
		t.Errorf("DefaultFormat: expected %q, got %q", f.DefaultFormat, got.DefaultFormat)
	}
	if got.Timeout != f.Timeout {
		t.Errorf("Timeout: expected %q, got %q", f.Timeout, got.Timeout)
	}
	if got.Concurrency != f.Concurrency {
		t.Errorf("Concurrency: expected %d, got %d", f.Concurrency, got.Concurrency)
	}
	if got.Rate != f.Rate {
		t.Errorf("Rate: expected %g, got %g", f.Rate, got.Rate)
	}
	if got.DBPath != f.DBPath {
		t.Errorf("DBPath: expected %q, got %q", f.DBPath, got.DBPath)
	}
}

func TestWriteFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := config.WriteFile(path, config.File{APIKey: "k"}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// Should be 0600 — owner read/write only
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions: expected 0600, got %04o", info.Mode().Perm())
	}
}

func TestWriteFileIsValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := config.WriteFile(path, config.Template()); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, _ := os.ReadFile(path)

	var f config.File
	if err := json.Unmarshal(data, &f); err != nil {
		t.Errorf("WriteFile produced invalid JSON: %v", err)
	}
}

func TestTemplateDefaults(t *testing.T) {
	tmpl := config.Template()

	if tmpl.DefaultFormat != "table" {
		t.Errorf("Template.DefaultFormat: expected table, got %q", tmpl.DefaultFormat)
	}
	if tmpl.Timeout != "30s" {
		t.Errorf("Template.Timeout: expected 30s, got %q", tmpl.Timeout)
	}
	if tmpl.Concurrency != config.DefaultConcurrency {
		t.Errorf("Template.Concurrency: expected %d, got %d", config.DefaultConcurrency, tmpl.Concurrency)
	}
	if tmpl.Rate != config.DefaultRate {
		t.Errorf("Template.Rate: expected %g, got %g", config.DefaultRate, tmpl.Rate)
	}
	if tmpl.APIKey != "" {
		t.Errorf("Template.APIKey should be empty (user fills it in), got %q", tmpl.APIKey)
	}
}

func TestTemplateBaseURL(t *testing.T) {
	tmpl := config.Template()
	if !strings.HasPrefix(tmpl.BaseURL, "https://") {
		t.Errorf("Template.BaseURL should be an https URL, got %q", tmpl.BaseURL)
	}
}
