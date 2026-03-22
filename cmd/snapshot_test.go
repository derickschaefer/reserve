// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"regexp"
	"testing"
	"time"
)

func TestNewSnapshotIDIsULIDLike(t *testing.T) {
	id := newSnapshotID()
	if len(id) != 26 {
		t.Fatalf("expected 26-char ULID, got %d (%q)", len(id), id)
	}
	re := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
	if !re.MatchString(id) {
		t.Fatalf("snapshot id not Crockford base32 ULID format: %q", id)
	}
}

func TestNewSnapshotIDUniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := newSnapshotID()
		if seen[id] {
			t.Fatalf("duplicate snapshot id generated: %q", id)
		}
		seen[id] = true
	}
}

func TestNewSnapshotIDSortability(t *testing.T) {
	a := newSnapshotID()
	time.Sleep(2 * time.Millisecond)
	b := newSnapshotID()
	if a >= b {
		t.Fatalf("expected increasing lexical order across time: a=%q b=%q", a, b)
	}
}
