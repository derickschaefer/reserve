// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/config"
)

func TestOutputWriterDefault(t *testing.T) {
	globalFlags.Out = ""
	w, closeFn, err := outputWriter(os.Stdout)
	if err != nil {
		t.Fatalf("outputWriter default: %v", err)
	}
	if w != os.Stdout {
		t.Fatalf("expected stdout writer passthrough")
	}
	if err := closeFn(); err != nil {
		t.Fatalf("default closer should be nil error, got: %v", err)
	}
}

func TestOutputWriterFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.txt")
	globalFlags.Out = p
	t.Cleanup(func() { globalFlags.Out = "" })

	w, closeFn, err := outputWriter(os.Stdout)
	if err != nil {
		t.Fatalf("outputWriter file: %v", err)
	}
	if w == os.Stdout {
		t.Fatalf("expected file writer, got stdout")
	}
	if err := closeFn(); err != nil {
		t.Fatalf("closing output writer: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
}

func TestParseIntIDAllowsZero(t *testing.T) {
	got, err := parseIntID("0", "release ID")
	if err != nil {
		t.Fatalf("expected zero to be valid, got error: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected parsed zero, got %d", got)
	}
}

func TestResolveSeriesIDsUsesAliases(t *testing.T) {
	deps := &app.Deps{Config: &config.Config{
		SeriesAliases: map[string]config.Alias{
			"pce-services": {SeriesID: "PB0000031Q225SBEA"},
			"cpi":          {SeriesID: "CPIAUCSL"},
		},
	}}

	got := resolveSeriesIDs(deps, []string{"pce-services", "GDP", "cpi", "GDP"})
	want := []string{"PB0000031Q225SBEA", "GDP", "CPIAUCSL"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("id[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
