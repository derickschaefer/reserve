// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"testing"

	snlib "github.com/derickschaefer/reserve/internal/snippet"
)

func TestValidateSnippetName(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{in: "pcu_annual_bar"},
		{in: "my.snippet-1"},
		{in: "", wantErr: true},
		{in: "@bad", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			err := validateSnippetName(tc.in)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSnippetPreview(t *testing.T) {
	short := "echo hi"
	if got := snippetPreview(short); got != short {
		t.Fatalf("short preview changed: %q", got)
	}
	long := "012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789xyz"
	got := snippetPreview(long)
	if len(got) != 90 {
		t.Fatalf("preview length = %d, want 90", len(got))
	}
	if got[len(got)-3:] != "..." {
		t.Fatalf("preview should end with ..., got %q", got)
	}
}

func TestSnippetDescription(t *testing.T) {
	withDesc := snippetDescription(snlib.Snippet{Command: "echo hi", Description: "My snippet"})
	if withDesc != "My snippet" {
		t.Fatalf("expected description, got %q", withDesc)
	}
	withoutDesc := snippetDescription(snlib.Snippet{Command: "echo hi"})
	if withoutDesc != "echo hi" {
		t.Fatalf("expected command preview fallback, got %q", withoutDesc)
	}
}
