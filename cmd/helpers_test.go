package cmd

import (
	"os"
	"path/filepath"
	"testing"
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
