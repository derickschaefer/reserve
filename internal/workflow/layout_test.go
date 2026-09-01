// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectionLayoutHelpers(t *testing.T) {
	root, err := ResolveRootDir("")
	if err != nil {
		t.Fatalf("ResolveRootDir: %v", err)
	}
	if !strings.Contains(root, filepath.Join(".reserve", "workflows")) {
		t.Fatalf("unexpected default root %q", root)
	}

	if got := CollectionPath("/tmp/root", "official", "inflation"); got != filepath.Join("/tmp/root", "official", "inflation", "collection.yaml") {
		t.Fatalf("CollectionPath = %q", got)
	}
	if got := CollectionReadmePath("/tmp/root", "official", "inflation"); got != filepath.Join("/tmp/root", "official", "inflation", "README.md") {
		t.Fatalf("CollectionReadmePath = %q", got)
	}
	if got := WorkflowPath("/tmp/root", "official", "inflation", "cpi-dashboard"); got != filepath.Join("/tmp/root", "official", "inflation", "workflows", "cpi-dashboard.yaml") {
		t.Fatalf("WorkflowPath = %q", got)
	}
}

func TestParseRefsRejectUnsafeNames(t *testing.T) {
	invalidWorkflowRefs := []string{
		"../inflation/workflow",
		"official/../workflow",
		"official/inflation/..",
		"official//workflow",
		`official/inflation/workflow\escape`,
		"official/inflation/work flow",
		"/official/inflation/workflow",
	}
	for _, ref := range invalidWorkflowRefs {
		if _, err := ParseRef(ref); err == nil {
			t.Errorf("ParseRef(%q) succeeded, want error", ref)
		}
	}

	got, err := ParseRef("Official/Inflation/GDP-Summary")
	if err != nil {
		t.Fatalf("ParseRef(valid): %v", err)
	}
	if got.Repository != "official" || got.Collection != "inflation" || got.Workflow != "GDP-Summary" {
		t.Fatalf("ParseRef(valid) = %+v", got)
	}

	for _, ref := range []string{"../inflation", "official/..", "official/", `official/inflation\escape`} {
		if _, err := ParseCollectionRef(ref); err == nil {
			t.Errorf("ParseCollectionRef(%q) succeeded, want error", ref)
		}
	}
}

func TestEnsureWithinRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workflows")
	inside := filepath.Join(root, "official", "inflation", "workflows", "gdp.yaml")
	if err := EnsureWithinRoot(root, inside); err != nil {
		t.Fatalf("EnsureWithinRoot(inside): %v", err)
	}
	if err := EnsureWithinRoot(root, filepath.Join(root, "..", "outside.yaml")); err == nil {
		t.Fatalf("expected lexical escape to fail")
	}
}

func TestEnsureWithinRootRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workflows")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("MkdirAll(root): %v", err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("MkdirAll(outside): %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "official")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	target := filepath.Join(root, "official", "inflation", "workflows", "gdp.yaml")
	if err := EnsureWithinRoot(root, target); err == nil {
		t.Fatalf("expected symlink escape to fail")
	}
}

func TestDiscoverAndResolveEntry(t *testing.T) {
	root := t.TempDir()
	manifest := CollectionPath(root, "official", "inflation")
	if err := WriteCollection(manifest, CollectionDocument{
		Collection: Collection{Title: "Inflation Toolkit", Documentation: "README.md"},
	}); err != nil {
		t.Fatalf("WriteCollection: %v", err)
	}
	workflowPath := WorkflowPath(root, "official", "inflation", "cpi-dashboard")
	if err := WriteFile(workflowPath, Document{
		Workflow: Workflow{
			Title:            "CPI Dashboard",
			Summary:          "Repeatable CPI analysis",
			EstimatedRuntime: "5 seconds",
			Pipeline:         []string{"printf 'ok\\n'"},
		},
	}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entries, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Ref.Repository != "official" || entries[0].Ref.Collection != "inflation" || entries[0].Ref.Workflow != "cpi-dashboard" {
		t.Fatalf("unexpected ref %+v", entries[0].Ref)
	}

	entry, err := ResolveEntry(root, "official/inflation/cpi-dashboard")
	if err != nil {
		t.Fatalf("ResolveEntry: %v", err)
	}
	if entry.Path != workflowPath {
		t.Fatalf("entry path = %q want %q", entry.Path, workflowPath)
	}
}

func TestCollectionParseNormalizeValidate(t *testing.T) {
	doc := CollectionDocument{
		Collection: Collection{
			Title:         "  Inflation Toolkit  ",
			Summary:       "  Curated inflation workflows  ",
			Version:       " 1.0 ",
			Author:        " Derick Schaefer ",
			License:       " MIT ",
			Documentation: " README.md ",
		},
	}
	doc.Normalize()
	if doc.Collection.Title != "Inflation Toolkit" {
		t.Fatalf("Title = %q", doc.Collection.Title)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestLoadCollectionRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collection.yaml")
	data := []byte("collection:\n  title: Inflation\n  documntation: README.md\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadCollection(path); err == nil {
		t.Fatalf("expected unknown collection field to fail")
	}
}
