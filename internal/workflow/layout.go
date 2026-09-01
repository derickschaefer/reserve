// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultRootDir        = ".reserve/workflows"
	DefaultRepositoryName = "personal"
	CollectionFileName    = "collection.yaml"
	CollectionReadmeName  = "README.md"
	WorkflowsDirName      = "workflows"
)

var namespaceComponentRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// CollectionDocument is the on-disk YAML representation of a workflow collection.
type CollectionDocument struct {
	Collection Collection `yaml:"collection"`
}

// Collection captures the metadata for a logical bundle of workflows.
type Collection struct {
	Title         string `yaml:"title,omitempty"`
	Summary       string `yaml:"summary,omitempty"`
	Version       string `yaml:"version,omitempty"`
	Author        string `yaml:"author,omitempty"`
	License       string `yaml:"license,omitempty"`
	Documentation string `yaml:"documentation,omitempty"`
}

// Ref names a workflow in the filesystem layout.
type Ref struct {
	Repository string
	Collection string
	Workflow   string
}

// CollectionRef names a collection in the filesystem layout.
type CollectionRef struct {
	Repository string
	Collection string
}

// Entry is a discovered workflow plus its surrounding collection metadata.
type Entry struct {
	Ref        Ref
	Path       string
	Collection CollectionDocument
	Workflow   Document
}

// ResolveRootDir returns the absolute root directory for the workflow store.
func ResolveRootDir(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		return filepath.Join(home, DefaultRootDir), nil
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

// CollectionDir returns the collection directory for a repository/collection pair.
func CollectionDir(root, repository, collection string) string {
	return filepath.Join(root, normalizeRepository(repository), normalizeSlug(collection))
}

// CollectionPath returns the collection manifest path.
func CollectionPath(root, repository, collection string) string {
	return filepath.Join(CollectionDir(root, repository, collection), CollectionFileName)
}

// CollectionReadmePath returns the collection documentation path.
func CollectionReadmePath(root, repository, collection string) string {
	return filepath.Join(CollectionDir(root, repository, collection), CollectionReadmeName)
}

// WorkflowsDir returns the workflow document directory for a collection.
func WorkflowsDir(root, repository, collection string) string {
	return filepath.Join(CollectionDir(root, repository, collection), WorkflowsDirName)
}

// WorkflowPath returns the workflow document path for a workflow name.
func WorkflowPath(root, repository, collection, workflow string) string {
	return filepath.Join(WorkflowsDir(root, repository, collection), normalizeWorkflowName(workflow)+".yaml")
}

// ParseRef parses repository/collection/workflow references.
//
// Supported forms:
//
//	workflow
//	collection/workflow
//	repository/collection/workflow
func ParseRef(input string) (Ref, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Ref{}, fmt.Errorf("workflow reference cannot be empty")
	}
	parts := strings.Split(input, "/")
	for i, part := range parts {
		kind := "workflow"
		if len(parts) == 3 && i == 0 {
			kind = "repository"
		} else if (len(parts) == 2 && i == 0) || (len(parts) == 3 && i == 1) {
			kind = "collection"
		}
		if err := validateNamespaceComponent(kind, part); err != nil {
			return Ref{}, err
		}
	}
	switch len(parts) {
	case 1:
		return Ref{
			Repository: DefaultRepositoryName,
			Workflow:   normalizeWorkflowName(parts[0]),
		}, nil
	case 2:
		return Ref{
			Repository: DefaultRepositoryName,
			Collection: normalizeSlug(parts[0]),
			Workflow:   normalizeWorkflowName(parts[1]),
		}, nil
	case 3:
		return Ref{
			Repository: normalizeRepository(parts[0]),
			Collection: normalizeSlug(parts[1]),
			Workflow:   normalizeWorkflowName(parts[2]),
		}, nil
	default:
		return Ref{}, fmt.Errorf("invalid workflow reference %q", input)
	}
}

// ParseCollectionRef parses repository/collection references.
//
// Supported forms:
//
//	collection
//	repository/collection
func ParseCollectionRef(input string) (CollectionRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return CollectionRef{}, fmt.Errorf("collection reference cannot be empty")
	}
	parts := strings.Split(input, "/")
	for i, part := range parts {
		kind := "collection"
		if len(parts) == 2 && i == 0 {
			kind = "repository"
		}
		if err := validateNamespaceComponent(kind, part); err != nil {
			return CollectionRef{}, err
		}
	}
	switch len(parts) {
	case 1:
		return CollectionRef{
			Repository: DefaultRepositoryName,
			Collection: normalizeSlug(parts[0]),
		}, nil
	case 2:
		return CollectionRef{
			Repository: normalizeRepository(parts[0]),
			Collection: normalizeSlug(parts[1]),
		}, nil
	default:
		return CollectionRef{}, fmt.Errorf("invalid collection reference %q", input)
	}
}

// LoadCollection reads and parses a collection document from disk.
func LoadCollection(path string) (CollectionDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CollectionDocument{}, err
	}
	var doc CollectionDocument
	if err := decodeStrictYAML(data, &doc); err != nil {
		return CollectionDocument{}, fmt.Errorf("parse collection yaml: %w", err)
	}
	doc.Normalize()
	return doc, nil
}

// WriteCollection writes a normalized collection document to disk.
func WriteCollection(path string, doc CollectionDocument) error {
	doc.Normalize()
	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode collection yaml: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create collection dir: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write collection yaml: %w", err)
	}
	return nil
}

// Normalize trims whitespace on collection fields.
func (d *CollectionDocument) Normalize() {
	d.Collection.Normalize()
}

// Normalize trims whitespace on collection fields.
func (c *Collection) Normalize() {
	c.Title = strings.TrimSpace(c.Title)
	c.Summary = strings.TrimSpace(c.Summary)
	c.Version = strings.TrimSpace(c.Version)
	c.Author = strings.TrimSpace(c.Author)
	c.License = strings.TrimSpace(c.License)
	c.Documentation = strings.TrimSpace(c.Documentation)
}

// Validate reports whether the collection has meaningful content.
func (d CollectionDocument) Validate() error {
	if !d.Collection.HasContent() {
		return fmt.Errorf("collection document is empty")
	}
	return nil
}

// HasContent reports whether any collection metadata is present.
func (c Collection) HasContent() bool {
	return strings.TrimSpace(c.Title) != "" ||
		strings.TrimSpace(c.Summary) != "" ||
		strings.TrimSpace(c.Version) != "" ||
		strings.TrimSpace(c.Author) != "" ||
		strings.TrimSpace(c.License) != "" ||
		strings.TrimSpace(c.Documentation) != ""
}

// Discover walks the workflow store and returns all discovered workflows.
func Discover(root string) ([]Entry, error) {
	root, err := ResolveRootDir(root)
	if err != nil {
		return nil, err
	}
	repos, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []Entry
	for _, repoEntry := range repos {
		if !repoEntry.IsDir() {
			continue
		}
		if err := validateNamespaceComponent("repository", repoEntry.Name()); err != nil {
			return nil, err
		}
		repoName := normalizeRepository(repoEntry.Name())
		repoPath := filepath.Join(root, repoEntry.Name())
		collections, err := os.ReadDir(repoPath)
		if err != nil {
			return nil, err
		}
		for _, collectionEntry := range collections {
			if !collectionEntry.IsDir() {
				continue
			}
			if err := validateNamespaceComponent("collection", collectionEntry.Name()); err != nil {
				return nil, err
			}
			collectionName := normalizeSlug(collectionEntry.Name())
			manifestPath := CollectionPath(root, repoName, collectionName)
			if err := EnsureWithinRoot(root, manifestPath); err != nil {
				return nil, err
			}
			collectionDoc, err := LoadCollection(manifestPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}

			workflowDir := WorkflowsDir(root, repoName, collectionName)
			if err := EnsureWithinRoot(root, workflowDir); err != nil {
				return nil, err
			}
			workflows, err := os.ReadDir(workflowDir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			for _, workflowEntry := range workflows {
				if workflowEntry.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(workflowEntry.Name()))
				if ext != ".yaml" && ext != ".yml" {
					continue
				}
				baseName := strings.TrimSuffix(workflowEntry.Name(), ext)
				if err := validateNamespaceComponent("workflow", baseName); err != nil {
					return nil, err
				}
				workflowName := normalizeWorkflowName(baseName)
				path := filepath.Join(workflowDir, workflowEntry.Name())
				if err := EnsureWithinRoot(root, path); err != nil {
					return nil, err
				}
				doc, err := LoadFile(path)
				if err != nil {
					return nil, err
				}
				entries = append(entries, Entry{
					Ref: Ref{
						Repository: repoName,
						Collection: collectionName,
						Workflow:   workflowName,
					},
					Path:       path,
					Collection: collectionDoc,
					Workflow:   doc,
				})
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		li := entries[i].Ref
		lj := entries[j].Ref
		if li.Repository != lj.Repository {
			return li.Repository < lj.Repository
		}
		if li.Collection != lj.Collection {
			return li.Collection < lj.Collection
		}
		return li.Workflow < lj.Workflow
	})
	return entries, nil
}

// ResolveEntry resolves a workflow reference or a file path.
func ResolveEntry(root, input string) (Entry, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Entry{}, fmt.Errorf("workflow reference cannot be empty")
	}
	if info, err := os.Stat(input); err == nil && !info.IsDir() {
		doc, err := LoadFile(input)
		if err != nil {
			return Entry{}, err
		}
		ref, _ := inferRef(root, input)
		return Entry{Ref: ref, Path: input, Workflow: doc}, nil
	}
	root, err := ResolveRootDir(root)
	if err != nil {
		return Entry{}, err
	}
	ref, err := ParseRef(input)
	if err != nil {
		return Entry{}, err
	}
	if ref.Collection == "" {
		return Entry{}, fmt.Errorf("workflow reference %q must include a collection", input)
	}
	path := WorkflowPath(root, ref.Repository, ref.Collection, ref.Workflow)
	if err := EnsureWithinRoot(root, path); err != nil {
		return Entry{}, err
	}
	doc, err := LoadFile(path)
	if err != nil {
		return Entry{}, err
	}
	collectionPath := CollectionPath(root, ref.Repository, ref.Collection)
	if err := EnsureWithinRoot(root, collectionPath); err != nil {
		return Entry{}, err
	}
	collectionDoc, err := LoadCollection(collectionPath)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Ref:        ref,
		Path:       path,
		Collection: collectionDoc,
		Workflow:   doc,
	}, nil
}

// EnsureWithinRoot verifies that path remains under root, including through
// any existing symlinks in either path.
func EnsureWithinRoot(root, path string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve workflow root: %w", err)
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve workflow path: %w", err)
	}
	if !pathWithin(rootAbs, pathAbs) {
		return fmt.Errorf("workflow path %q escapes workflow root %q", path, root)
	}

	resolvedRoot, err := resolveExistingPath(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve workflow root symlinks: %w", err)
	}
	resolvedPath, err := resolveExistingPath(pathAbs)
	if err != nil {
		return fmt.Errorf("resolve workflow path symlinks: %w", err)
	}
	if !pathWithin(resolvedRoot, resolvedPath) {
		return fmt.Errorf("workflow path %q escapes workflow root %q through a symlink", path, root)
	}
	return nil
}

func validateNamespaceComponent(kind, value string) error {
	if value == "" || value != strings.TrimSpace(value) || value == "." || value == ".." || !namespaceComponentRE.MatchString(value) {
		return fmt.Errorf("invalid %s name %q: use letters, numbers, dots, underscores, or hyphens", kind, value)
	}
	return nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveExistingPath(path string) (string, error) {
	path = filepath.Clean(path)
	current := path
	var missing []string
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func inferRef(root, path string) (Ref, bool) {
	root, err := ResolveRootDir(root)
	if err != nil {
		return Ref{}, false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return Ref{}, false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 4 {
		return Ref{}, false
	}
	if parts[len(parts)-2] != WorkflowsDirName {
		return Ref{}, false
	}
	repo := normalizeRepository(parts[0])
	collection := normalizeSlug(parts[1])
	workflowName := normalizeWorkflowName(strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1])))
	return Ref{Repository: repo, Collection: collection, Workflow: workflowName}, true
}

func normalizeRepository(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultRepositoryName
	}
	return strings.ToLower(s)
}

func normalizeSlug(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeWorkflowName(s string) string {
	return strings.TrimSpace(s)
}
