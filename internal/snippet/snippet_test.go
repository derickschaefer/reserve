package snippet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveHomeDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := ResolveHome("")
	if err != nil {
		t.Fatalf("ResolveHome: %v", err)
	}
	want := filepath.Join(home, ".reserve", "snippets")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLoadSaveResolve(t *testing.T) {
	home := t.TempDir()
	lib := Library{
		Name: DefaultLibrary,
		Snippets: map[string]Snippet{
			"foo": {Command: "echo foo", Description: "desc"},
		},
	}
	if err := SaveLibrary(home, lib); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}
	ref, s, err := Resolve(home, "foo", nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ref.Library != DefaultLibrary || ref.Name != "foo" {
		t.Fatalf("unexpected ref %+v", ref)
	}
	if s.Command != "echo foo" {
		t.Fatalf("unexpected command %q", s.Command)
	}
}

func TestResolveAmbiguous(t *testing.T) {
	home := t.TempDir()
	for _, lib := range []string{"personal", "official"} {
		if err := SaveLibrary(home, Library{
			Name:     lib,
			Snippets: map[string]Snippet{"foo": {Command: "echo " + lib}},
		}); err != nil {
			t.Fatalf("SaveLibrary(%s): %v", lib, err)
		}
	}
	_, _, err := Resolve(home, "foo", []string{"official"})
	if err == nil {
		t.Fatalf("expected ambiguity error")
	}
}

func TestList(t *testing.T) {
	home := t.TempDir()
	if err := SaveLibrary(home, Library{
		Name:     "personal",
		Snippets: map[string]Snippet{"b": {Command: "echo b"}, "a": {Command: "echo a"}},
	}); err != nil {
		t.Fatalf("SaveLibrary: %v", err)
	}
	refs, _, err := List(home, nil, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(refs) != 2 || refs[0].Name != "a" || refs[1].Name != "b" {
		t.Fatalf("unexpected refs: %+v", refs)
	}
	if _, err := os.Stat(LibraryPath(home, "personal")); err != nil {
		t.Fatalf("missing library file: %v", err)
	}
}
