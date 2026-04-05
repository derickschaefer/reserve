// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/derickschaefer/reserve/internal/config"
)

func TestConfigGrantAndRevoke(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.WriteFile(path, config.Template()); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() { _ = os.Chdir(orig) })

	configGrantCmd.SetArgs([]string{"bamlc0a0cm"})
	if err := configGrantCmd.RunE(configGrantCmd, []string{"bamlc0a0cm"}); err != nil {
		t.Fatalf("config grant: %v", err)
	}

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !cfg.HasGrantedSeriesPermission("BAMLC0A0CM") {
		t.Fatalf("expected BAMLC0A0CM to be granted")
	}

	configRevokeCmd.SetArgs([]string{"BAMLC0A0CM"})
	if err := configRevokeCmd.RunE(configRevokeCmd, []string{"BAMLC0A0CM"}); err != nil {
		t.Fatalf("config revoke: %v", err)
	}

	cfg, err = config.Load("")
	if err != nil {
		t.Fatalf("config.Load after revoke: %v", err)
	}
	if cfg.HasGrantedSeriesPermission("BAMLC0A0CM") {
		t.Fatalf("expected BAMLC0A0CM grant to be removed")
	}
}
