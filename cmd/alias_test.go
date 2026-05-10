// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/config"
	"github.com/spf13/cobra"
)

func TestValidateAliasNameNormalizes(t *testing.T) {
	got, err := validateAliasName("PCE-Services")
	if err != nil {
		t.Fatalf("validateAliasName: %v", err)
	}
	if got != "pce-services" {
		t.Fatalf("alias = %q, want pce-services", got)
	}
}

func TestValidateAliasNameRejectsInvalid(t *testing.T) {
	for _, alias := range []string{"", " pce services ", "$bad", "obs"} {
		if _, err := validateAliasName(alias); err == nil {
			t.Fatalf("expected %q to be invalid", alias)
		}
	}
}

func TestPrintAliasTableSortsAliases(t *testing.T) {
	var buf bytes.Buffer
	printAliasTable(&buf, map[string]config.Alias{
		"z": {SeriesID: "ZSERIES"},
		"a": {SeriesID: "ASERIES", Note: "alpha"},
	})
	out := buf.String()
	if !strings.Contains(out, "ASERIES") || !strings.Contains(out, "ZSERIES") {
		t.Fatalf("alias table missing entries: %s", out)
	}
	if strings.Index(out, "ASERIES") > strings.Index(out, "ZSERIES") {
		t.Fatalf("alias table not sorted: %s", out)
	}
}

func TestAliasGetUsesResolvedConfig(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	dir := t.TempDir()
	clearConfigEnvForAliasTest(t, dir)
	writeAliasConfigForTest(t, dir, map[string]config.Alias{"pce": {SeriesID: "PB0000031Q225SBEA", Note: "services"}})

	if err := aliasGetCmd.RunE(cmd, []string{"PCE"}); err != nil {
		t.Fatalf("alias get: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "pce -> PB0000031Q225SBEA (services)") {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestAliasDeleteNormalizesExistingConfigKeys(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	dir := t.TempDir()
	clearConfigEnvForAliasTest(t, dir)
	writeAliasConfigForTest(t, dir, map[string]config.Alias{"PCE": {SeriesID: "PB0000031Q225SBEA"}})

	if err := aliasDeleteCmd.RunE(cmd, []string{"pce"}); err != nil {
		t.Fatalf("alias delete: %v", err)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if _, ok := cfg.SeriesAliases["pce"]; ok {
		t.Fatalf("expected alias to be deleted")
	}
}

func TestAliasDeleteFindsAliasInUserConfigWhenLocalExists(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	dir := t.TempDir()
	clearConfigEnvForAliasTest(t, dir)
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	// local config exists, but alias only exists in user config.
	writeAliasConfigForTest(t, dir, map[string]config.Alias{"local-only": {SeriesID: "GDP"}})
	userPath, err := config.UserConfigPath()
	if err != nil {
		t.Fatalf("UserConfigPath: %v", err)
	}
	writeAliasConfigAtPath(t, userPath, map[string]config.Alias{"pce": {SeriesID: "PB0000031Q225SBEA"}})

	if err := aliasDeleteCmd.RunE(cmd, []string{"pce"}); err != nil {
		t.Fatalf("alias delete: %v", err)
	}

	userCfg, err := readConfigAtPath(userPath)
	if err != nil {
		t.Fatalf("read user config: %v", err)
	}
	if _, ok := userCfg.SeriesAliases["pce"]; ok {
		t.Fatalf("expected user alias to be deleted")
	}
}

func TestAliasConflictTransientErrorFailsClosed(t *testing.T) {
	origLookup := seriesIDLookup
	t.Cleanup(func() { seriesIDLookup = origLookup })
	seriesIDLookup = func(context.Context, *app.Deps, string) error {
		return errors.New("HTTP 504: gateway time-out")
	}

	conflicts, err := aliasConflictsWithSeriesID(&cobra.Command{}, &app.Deps{}, "fedfunds")
	if err == nil {
		t.Fatalf("expected transient-error failure")
	}
	if conflicts {
		t.Fatalf("expected no definitive conflict on transient error")
	}
}

func clearConfigEnvForAliasTest(t *testing.T, dir string) {
	t.Helper()
	t.Setenv(config.EnvAPIKey, "")
	t.Setenv(config.EnvDBPath, "")
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("LOCALAPPDATA", dir)
}

func writeAliasConfigForTest(t *testing.T, dir string, aliases map[string]config.Alias) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := config.WriteFile(filepath.Join(dir, "config.json"), config.File{
		APIKey:        "test-key",
		SeriesAliases: aliases,
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func writeAliasConfigAtPath(t *testing.T, path string, aliases map[string]config.Alias) {
	t.Helper()
	if err := config.WriteFile(path, config.File{
		APIKey:        "test-key",
		SeriesAliases: aliases,
	}); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
