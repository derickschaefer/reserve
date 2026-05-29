// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package snippet

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	SchemaVersion  = "reserve.snippets/v1"
	DefaultLibrary = "personal"
)

type Library struct {
	Schema      string             `yaml:"schema"`
	Name        string             `yaml:"name"`
	Title       string             `yaml:"title,omitempty"`
	Description string             `yaml:"description,omitempty"`
	Version     string             `yaml:"version,omitempty"`
	Author      string             `yaml:"author,omitempty"`
	License     string             `yaml:"license,omitempty"`
	Tags        []string           `yaml:"tags,omitempty"`
	Snippets    map[string]Snippet `yaml:"snippets"`
}

type Snippet struct {
	Title       string   `yaml:"title,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Command     string   `yaml:"command"`
	Tags        []string `yaml:"tags,omitempty"`
	Series      []string `yaml:"series,omitempty"`
}

type Ref struct {
	Library string
	Name    string
}

func ParseRef(input string) (Ref, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Ref{}, fmt.Errorf("snippet name cannot be empty")
	}
	if strings.Count(input, "/") > 1 {
		return Ref{}, fmt.Errorf("invalid snippet name %q", input)
	}
	if strings.Contains(input, "/") {
		parts := strings.SplitN(input, "/", 2)
		lib := normalize(parts[0])
		name := normalize(parts[1])
		if lib == "" || name == "" {
			return Ref{}, fmt.Errorf("invalid snippet name %q", input)
		}
		return Ref{Library: lib, Name: name}, nil
	}
	return Ref{Name: normalize(input)}, nil
}

func ResolveHome(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		return filepath.Join(home, ".reserve", "snippets"), nil
	}
	if strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		input = filepath.Join(home, input[2:])
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func LibraryPath(home, library string) string {
	return filepath.Join(home, normalize(library), "snippets.yaml")
}

func LoadLibrary(home, library string) (Library, error) {
	path := LibraryPath(home, library)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Library{}, err
	}
	var lib Library
	if err := yaml.Unmarshal(raw, &lib); err != nil {
		return Library{}, fmt.Errorf("parse %s: %w", path, err)
	}
	lib = canonicalLibrary(lib, library)
	return lib, nil
}

func LoadOrInitLibrary(home, library string) (Library, error) {
	lib, err := LoadLibrary(home, library)
	if err == nil {
		return lib, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Library{}, err
	}
	lib = canonicalLibrary(Library{}, library)
	return lib, nil
}

func SaveLibrary(home string, lib Library) error {
	lib = canonicalLibrary(lib, lib.Name)
	if lib.Name == "" {
		return fmt.Errorf("library name cannot be empty")
	}
	path := LibraryPath(home, lib.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create snippet library dir: %w", err)
	}
	data, err := yaml.Marshal(lib)
	if err != nil {
		return fmt.Errorf("encode snippet library: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func Resolve(home string, name string, enabled []string) (Ref, Snippet, error) {
	ref, err := ParseRef(name)
	if err != nil {
		return Ref{}, Snippet{}, err
	}
	if ref.Library != "" {
		lib, err := LoadLibrary(home, ref.Library)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return Ref{}, Snippet{}, fmt.Errorf("library %q not found", ref.Library)
			}
			return Ref{}, Snippet{}, err
		}
		s, ok := lib.Snippets[ref.Name]
		if !ok {
			return Ref{}, Snippet{}, fmt.Errorf("snippet %q not found", ref.Library+"/"+ref.Name)
		}
		return ref, s, nil
	}

	libs := SearchOrder(enabled)
	matches := make([]Ref, 0, 2)
	var found Snippet
	for _, libName := range libs {
		lib, err := LoadLibrary(home, libName)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return Ref{}, Snippet{}, err
		}
		s, ok := lib.Snippets[ref.Name]
		if ok {
			matches = append(matches, Ref{Library: libName, Name: ref.Name})
			found = s
		}
	}
	if len(matches) == 0 {
		return Ref{}, Snippet{}, fmt.Errorf("snippet %q not found", ref.Name)
	}
	if len(matches) > 1 {
		lines := make([]string, 0, len(matches)+2)
		lines = append(lines, fmt.Sprintf("snippet name %q exists in:", ref.Name))
		for _, m := range matches {
			lines = append(lines, fmt.Sprintf("  - %s/%s", m.Library, m.Name))
		}
		lines = append(lines, "please specify a library-qualified name")
		return Ref{}, Snippet{}, errors.New(strings.Join(lines, "\n"))
	}
	return matches[0], found, nil
}

func SearchOrder(enabled []string) []string {
	out := []string{DefaultLibrary}
	seen := map[string]struct{}{DefaultLibrary: {}}
	for _, lib := range enabled {
		lib = normalize(lib)
		if lib == "" {
			continue
		}
		if _, ok := seen[lib]; ok {
			continue
		}
		seen[lib] = struct{}{}
		out = append(out, lib)
	}
	return out
}

func List(home string, enabled []string, libraryFilter string) ([]Ref, map[Ref]Snippet, error) {
	libs := SearchOrder(enabled)
	if libraryFilter != "" {
		libs = []string{normalize(libraryFilter)}
	}
	refs := make([]Ref, 0)
	values := make(map[Ref]Snippet)
	for _, libName := range libs {
		lib, err := LoadLibrary(home, libName)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, nil, err
		}
		names := make([]string, 0, len(lib.Snippets))
		for name := range lib.Snippets {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, n := range names {
			r := Ref{Library: libName, Name: n}
			refs = append(refs, r)
			values[r] = lib.Snippets[n]
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Library == refs[j].Library {
			return refs[i].Name < refs[j].Name
		}
		return refs[i].Library < refs[j].Library
	})
	return refs, values, nil
}

func canonicalLibrary(lib Library, defaultName string) Library {
	lib.Schema = SchemaVersion
	lib.Name = normalize(firstNonEmpty(lib.Name, defaultName))
	if lib.Snippets == nil {
		lib.Snippets = map[string]Snippet{}
	}
	norm := make(map[string]Snippet, len(lib.Snippets))
	for name, sn := range lib.Snippets {
		name = normalize(name)
		sn.Command = strings.TrimSpace(sn.Command)
		sn.Description = strings.TrimSpace(sn.Description)
		sn.Title = strings.TrimSpace(sn.Title)
		if name == "" || sn.Command == "" {
			continue
		}
		norm[name] = sn
	}
	lib.Snippets = norm
	return lib
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
