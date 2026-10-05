// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/derickschaefer/reserve/internal/render"
	"github.com/derickschaefer/reserve/internal/workflow"
	"github.com/spf13/cobra"
)

var workflowRoot string

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage local workflow documents",
	Long: `Manage local workflow documents for the RESERVE 1.2 workflow ecosystem.

Workflows are content-first YAML documents that describe repeatable economic
analyses. v1.2.0 starts with local workflow creation, validation, inspection,
collection-aware discovery, and filesystem persistence.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var workflowCreateCmd = &cobra.Command{
	Use:   "create <FILE|REF>",
	Short: "Create a starter workflow YAML document or collection manifest",
	Example: `  reserve workflow create official/inflation
  reserve workflow create official/inflation/cpi-dashboard`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := workflow.ResolveRootDir(workflowRoot)
		if err != nil {
			return err
		}

		path, kind, ref, collectionRef, err := resolveWorkflowCreatePath(args[0], root)
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists", path)
		} else if !os.IsNotExist(err) {
			return err
		}

		switch kind {
		case workflowCreateKindCollection:
			collectionTitle := titleize(collectionRef.Collection)
			if collectionTitle == "New Workflow" || collectionTitle == "" {
				collectionTitle = titleize(filepath.Base(filepath.Dir(path)))
			}
			doc := workflow.CollectionDocument{
				Collection: workflow.Collection{
					Title:         collectionTitle,
					Summary:       "Describe this collection of workflows.",
					Documentation: "README.md",
				},
			}
			if err := workflow.WriteCollection(path, doc); err != nil {
				return err
			}
			readmePath := filepath.Join(filepath.Dir(path), workflow.CollectionReadmeName)
			if err := scaffoldCollectionReadme(readmePath, collectionTitle); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created collection template at %s\n", path)
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created collection documentation at %s\n", readmePath)
		case workflowCreateKindWorkflow:
			doc := workflow.Document{
				Workflow: workflow.Workflow{
					Title:            titleize(workflowNameFromPath(path)),
					Summary:          "Describe the analysis this workflow performs.",
					Difficulty:       "Beginner",
					EstimatedRuntime: "5 seconds",
					RequiresNetwork:  true,
					ReserveVersion:   ">=1.2",
					Documentation:    "README.md",
				},
			}
			if err := workflow.WriteFile(path, doc); err != nil {
				return err
			}

			if ref.Repository != "" && ref.Collection != "" {
				collectionPath := workflow.CollectionPath(root, ref.Repository, ref.Collection)
				if _, err := os.Stat(collectionPath); os.IsNotExist(err) {
					collectionDoc := workflow.CollectionDocument{
						Collection: workflow.Collection{
							Title:         titleize(ref.Collection),
							Summary:       "Describe this collection of workflows.",
							Documentation: "README.md",
						},
					}
					if err := workflow.WriteCollection(collectionPath, collectionDoc); err != nil {
						return err
					}
					if err := scaffoldCollectionReadme(workflow.CollectionReadmePath(root, ref.Repository, ref.Collection), titleize(ref.Collection)); err != nil {
						return err
					}
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created workflow template at %s\n", path)
		default:
			return fmt.Errorf("unsupported workflow creation target %q", args[0])
		}
		return nil
	},
}

func scaffoldCollectionReadme(path, title string) error {
	content := fmt.Sprintf("# %s\n\nDescribe this workflow collection.\n\n## Workflows\n\nDocument the workflows included in this collection.\n", title)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create collection documentation: %w", err)
	}
	if _, err := io.WriteString(file, content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write collection documentation: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close collection documentation: %w", err)
	}
	return nil
}

var workflowListCmd = &cobra.Command{
	Use:   "list [ROOT]",
	Short: "List workflow documents by repository and collection",
	Example: `  reserve workflow list
  reserve workflow list ~/.reserve/workflows`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := workflowRoot
		if len(args) == 1 {
			root = args[0]
		}
		entries, err := workflow.Discover(root)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No workflow documents found.")
			return nil
		}
		printSimpleTable(cmd.OutOrStdout(), []string{"REPO", "COLLECTION", "WORKFLOW", "TITLE", "SUMMARY"}, func(add func(...string)) {
			for _, entry := range entries {
				add(
					entry.Ref.Repository,
					entry.Ref.Collection,
					entry.Ref.Workflow,
					truncateText(entry.Workflow.Workflow.Title, 36),
					truncateText(entry.Workflow.Workflow.Summary, 72),
				)
			}
		})
		return nil
	},
}

var workflowBrowseCmd = &cobra.Command{
	Use:   "browse [COLLECTION]",
	Short: "Browse official workflows from the RESERVE registry",
	Long: `Browse published official workflows without changing the local workflow store.

This is the v1.2.2 read-only registry preview. It does not require the FRED API
key and does not install or execute remote workflow content.`,
	Example: `  reserve workflow browse
  reserve workflow browse inflation
  reserve workflow browse --format json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		defer deps.Close()

		format := resolveFormat(deps.Config.Format)
		if format != render.FormatTable && format != render.FormatJSON {
			return fmt.Errorf("workflow browse supports only table and json output")
		}

		if len(args) == 1 {
			collection, err := deps.Registry.Collection(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if format == render.FormatJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(collection)
			}
			printSimpleTable(cmd.OutOrStdout(), []string{"ID", "TITLE", "VERSION"}, func(add func(...string)) {
				for _, entry := range collection.Workflows {
					add(entry.ID, entry.Title, entry.LatestVersion)
				}
			})
			return nil
		}

		catalog, err := deps.Registry.Browse(cmd.Context())
		if err != nil {
			return err
		}
		if format == render.FormatJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(catalog)
		}
		printSimpleTable(cmd.OutOrStdout(), []string{"COLLECTION", "ID", "TITLE", "VERSION", "RESERVE"}, func(add func(...string)) {
			for _, entry := range catalog.Workflows {
				add(entry.Collection, entry.ID, entry.Title, entry.LatestVersion, entry.ReserveVersion)
			}
		})
		return nil
	},
}

var workflowGetRemoteCmd = &cobra.Command{
	Use:   "get <COLLECTION> <WORKFLOW> [VERSION]",
	Short: "Retrieve an official workflow artifact",
	Example: `  reserve workflow get inflation cpi-history
  reserve workflow get inflation cpi-history 1.0.0
  reserve workflow get inflation cpi-history --format json`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		deps, err := buildDeps()
		if err != nil {
			return err
		}
		defer deps.Close()

		version := ""
		if len(args) == 3 {
			version = args[2]
		}
		artifact, err := deps.Registry.Workflow(cmd.Context(), args[0], args[1], version)
		if err != nil {
			return err
		}

		format := resolveFormat(deps.Config.Format)
		if format == render.FormatJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"collection": artifact.Collection,
				"workflow":   artifact.WorkflowID,
				"version":    artifact.Version,
				"content":    string(artifact.Content),
			})
		}
		if format != render.FormatTable {
			return fmt.Errorf("workflow get supports table (YAML) and json output")
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), string(artifact.Content))
		return err
	},
}

var workflowShowCmd = &cobra.Command{
	Use:   "show <FILE|REF>",
	Short: "Show a normalized workflow document",
	Example: `  reserve workflow show inflation-dashboard.yaml
  reserve workflow show official/inflation/cpi-dashboard`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entry, err := resolveWorkflowEntry(args[0])
		if err != nil {
			return err
		}
		data, err := workflow.Marshal(entry.Workflow)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), string(data))
		return err
	},
}

var workflowEditCmd = &cobra.Command{
	Use:     "edit <FILE|REF>",
	Short:   "Open a workflow document for editing",
	Example: `  reserve workflow edit inflation-dashboard.yaml`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entry, err := resolveWorkflowEntry(args[0])
		if err != nil {
			return err
		}
		editor := strings.TrimSpace(os.Getenv("EDITOR"))
		if editor == "" {
			return &NoticeError{Message: "workflow edit requires $EDITOR to be set; edit the file directly or set EDITOR and try again"}
		}
		proc := exec.CommandContext(cmd.Context(), editor, entry.Path)
		proc.Stdout = cmd.OutOrStdout()
		proc.Stderr = cmd.ErrOrStderr()
		proc.Stdin = cmd.InOrStdin()
		return proc.Run()
	},
}

var workflowRemoveCmd = &cobra.Command{
	Use:     "remove <FILE|REF>",
	Short:   "Delete a workflow document",
	Example: `  reserve workflow remove inflation-dashboard.yaml`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entry, err := resolveWorkflowEntry(args[0])
		if err != nil {
			return err
		}
		if err := os.Remove(entry.Path); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Removed workflow %s\n", entry.Path)
		return nil
	},
}

var workflowContractCmd = &cobra.Command{
	Use:     "contract <FILE|REF>",
	Short:   "Show the runtime inputs required by a workflow",
	Example: "  reserve workflow contract inflation-dashboard.yaml\n  reserve workflow contract GDP-Summary",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entry, err := resolveWorkflowEntry(args[0])
		if err != nil {
			return err
		}
		if err := entry.Workflow.ValidateForVersion(Version); err != nil {
			return fmt.Errorf("%s: %w", entry.Path, err)
		}
		printWorkflowContract(cmd.OutOrStdout(), workflowRefString(entry.Ref, entry.Path), entry.Workflow.Workflow.Contract)
		return nil
	},
}

var workflowRenderCmd = &cobra.Command{
	Use:     "render <FILE|REF> [ARGS...]",
	Short:   "Render one copy-ready pipeline command without executing it",
	Example: "  reserve workflow render inflation-dashboard.yaml\n  reserve workflow render GDP-Summary 2020-01-01 2024-12-31",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entry, err := resolveWorkflowEntry(args[0])
		if err != nil {
			return err
		}
		if err := entry.Workflow.ValidateForVersion(Version); err != nil {
			return fmt.Errorf("%s: %w", entry.Path, err)
		}

		pipeline, err := entry.Workflow.Workflow.ApplyContract(args[1:])
		if err != nil {
			return &NoticeError{Message: err.Error()}
		}
		if len(pipeline) == 0 {
			return &NoticeError{Message: fmt.Sprintf("workflow %s has no pipeline yet; add workflow.pipeline steps before rendering it", workflowRefString(entry.Ref, entry.Path))}
		}

		fmt.Fprintln(cmd.OutOrStdout(), strings.Join(pipeline, " | "))
		return nil
	},
}

var workflowValidateCmd = &cobra.Command{
	Use:     "validate <FILE|REF...>",
	Short:   "Validate workflow YAML documents",
	Example: `  reserve workflow validate inflation-dashboard.yaml`,
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, arg := range args {
			if doc, path, ok, err := resolveCollectionDocument(arg); err != nil {
				return err
			} else if ok {
				if err := doc.Validate(); err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				continue
			}
			entry, err := resolveWorkflowEntry(arg)
			if err != nil {
				return err
			}
			if err := entry.Workflow.ValidateForVersion(Version); err != nil {
				return fmt.Errorf("%s: %w", entry.Path, err)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Validated %d workflow file(s)\n", len(args))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(workflowCmd)
	workflowCmd.PersistentFlags().StringVar(&workflowRoot, "root", "", "workflow store root (default ~/.reserve/workflows)")
	workflowCmd.AddCommand(workflowCreateCmd)
	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowBrowseCmd)
	workflowCmd.AddCommand(workflowGetRemoteCmd)
	workflowCmd.AddCommand(workflowShowCmd)
	workflowCmd.AddCommand(workflowEditCmd)
	workflowCmd.AddCommand(workflowRemoveCmd)
	workflowCmd.AddCommand(workflowContractCmd)
	workflowCmd.AddCommand(workflowRenderCmd)
	workflowCmd.AddCommand(workflowValidateCmd)
}

func resolveWorkflowEntry(input string) (workflow.Entry, error) {
	entry, err := workflow.ResolveEntry(workflowRoot, input)
	if err == nil {
		return entry, nil
	}
	if strings.Count(strings.TrimSpace(input), "/") == 0 && !hasYAMLExtension(input) {
		if fallback, ok, fallbackErr := resolveWorkflowEntryByName(input); fallbackErr != nil {
			return workflow.Entry{}, fallbackErr
		} else if ok {
			return fallback, nil
		}
	}
	return workflow.Entry{}, err
}

func resolveWorkflowEntryByName(input string) (workflow.Entry, bool, error) {
	root, err := workflow.ResolveRootDir(workflowRoot)
	if err != nil {
		return workflow.Entry{}, false, err
	}
	entries, err := workflow.Discover(root)
	if err != nil {
		return workflow.Entry{}, false, err
	}
	target := strings.TrimSpace(input)
	if target == "" {
		return workflow.Entry{}, false, nil
	}
	var matches []workflow.Entry
	for _, entry := range entries {
		if entry.Ref.Workflow == target || strings.EqualFold(entry.Ref.Workflow, target) {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 0:
		return workflow.Entry{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return workflow.Entry{}, false, fmt.Errorf("workflow name %q matches multiple workflows; use a repository/collection reference", input)
	}
}

type workflowCreateKind string

const (
	workflowCreateKindCollection workflowCreateKind = "collection"
	workflowCreateKindWorkflow   workflowCreateKind = "workflow"
)

func resolveCollectionPath(input, root string) (string, workflow.CollectionRef, error) {
	target := strings.TrimSpace(input)
	if target == "" {
		return "", workflow.CollectionRef{}, fmt.Errorf("collection target cannot be empty")
	}
	if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() {
		return target, workflow.CollectionRef{}, nil
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", workflow.CollectionRef{}, statErr
	}
	if hasYAMLExtension(target) {
		return target, workflow.CollectionRef{}, nil
	}

	ref, err := workflow.ParseCollectionRef(target)
	if err != nil {
		return "", workflow.CollectionRef{}, err
	}
	path := workflow.CollectionPath(root, ref.Repository, ref.Collection)
	if err := workflow.EnsureWithinRoot(root, path); err != nil {
		return "", workflow.CollectionRef{}, err
	}
	return path, ref, nil
}

func resolveWorkflowCreatePath(input, root string) (string, workflowCreateKind, workflow.Ref, workflow.CollectionRef, error) {
	target := strings.TrimSpace(input)
	if target == "" {
		return "", "", workflow.Ref{}, workflow.CollectionRef{}, fmt.Errorf("workflow target cannot be empty")
	}

	if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() {
		if strings.EqualFold(filepath.Base(target), workflow.CollectionFileName) {
			return target, workflowCreateKindCollection, workflow.Ref{}, workflow.CollectionRef{}, nil
		}
		return target, workflowCreateKindWorkflow, workflow.Ref{}, workflow.CollectionRef{}, nil
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", "", workflow.Ref{}, workflow.CollectionRef{}, statErr
	}

	if hasYAMLExtension(target) {
		if strings.EqualFold(filepath.Base(target), workflow.CollectionFileName) {
			return target, workflowCreateKindCollection, workflow.Ref{}, workflow.CollectionRef{}, nil
		}
		return target, workflowCreateKindWorkflow, workflow.Ref{}, workflow.CollectionRef{}, nil
	}

	switch strings.Count(target, "/") {
	case 1:
		ref, err := workflow.ParseCollectionRef(target)
		if err != nil {
			return "", "", workflow.Ref{}, workflow.CollectionRef{}, err
		}
		path := workflow.CollectionPath(root, ref.Repository, ref.Collection)
		if err := workflow.EnsureWithinRoot(root, path); err != nil {
			return "", "", workflow.Ref{}, workflow.CollectionRef{}, err
		}
		return path, workflowCreateKindCollection, workflow.Ref{}, ref, nil
	case 2:
		ref, err := workflow.ParseRef(target)
		if err != nil {
			return "", "", workflow.Ref{}, workflow.CollectionRef{}, err
		}
		path := workflow.WorkflowPath(root, ref.Repository, ref.Collection, ref.Workflow)
		if err := workflow.EnsureWithinRoot(root, path); err != nil {
			return "", "", workflow.Ref{}, workflow.CollectionRef{}, err
		}
		return path, workflowCreateKindWorkflow, ref, workflow.CollectionRef{Repository: ref.Repository, Collection: ref.Collection}, nil
	case 0:
		return target, workflowCreateKindWorkflow, workflow.Ref{}, workflow.CollectionRef{}, nil
	default:
		return "", "", workflow.Ref{}, workflow.CollectionRef{}, fmt.Errorf("invalid workflow target %q", target)
	}
}

func resolveCollectionDocument(input string) (workflow.CollectionDocument, string, bool, error) {
	target := strings.TrimSpace(input)
	if target == "" {
		return workflow.CollectionDocument{}, "", false, fmt.Errorf("collection target cannot be empty")
	}

	root, err := workflow.ResolveRootDir(workflowRoot)
	if err != nil {
		return workflow.CollectionDocument{}, "", false, err
	}

	if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() {
		if strings.EqualFold(filepath.Base(target), workflow.CollectionFileName) {
			doc, err := workflow.LoadCollection(target)
			return doc, target, true, err
		}
		return workflow.CollectionDocument{}, "", false, nil
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return workflow.CollectionDocument{}, "", false, statErr
	}

	if !looksLikeCollectionTarget(target) {
		return workflow.CollectionDocument{}, "", false, nil
	}

	path, _, err := resolveCollectionPath(target, root)
	if err != nil {
		return workflow.CollectionDocument{}, "", false, err
	}
	doc, err := workflow.LoadCollection(path)
	if err != nil {
		return workflow.CollectionDocument{}, "", false, err
	}
	return doc, path, true, nil
}

func looksLikeCollectionTarget(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}
	if hasYAMLExtension(input) {
		return true
	}
	return strings.Count(input, "/") == 1
}

func hasYAMLExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func workflowNameFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func titleize(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return "New Workflow"
	}
	parts := strings.FieldsFunc(in, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	if len(parts) == 0 {
		return strings.ToUpper(in[:1]) + in[1:]
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func workflowRefString(ref workflow.Ref, path string) string {
	if ref.Repository == "" || ref.Collection == "" || ref.Workflow == "" {
		return path
	}
	return fmt.Sprintf("%s/%s/%s", ref.Repository, ref.Collection, ref.Workflow)
}

func printWorkflowContract(out io.Writer, name string, contract []workflow.ContractSlot) {
	fmt.Fprintf(out, "%s contract:\n", name)
	if len(contract) == 0 {
		fmt.Fprintln(out, "  (no runtime inputs)")
		return
	}
	for i, slot := range contract {
		label := strings.TrimSpace(slot.Label)
		if label == "" {
			label = "runtime value"
		}
		format := strings.TrimSpace(slot.Format)
		sample := strings.TrimSpace(slot.Sample)
		description := strings.TrimSpace(slot.Description)
		fmt.Fprintf(out, "  @%d = %s\n", i+1, label)
		if format != "" {
			fmt.Fprintf(out, "    format: %s\n", format)
		}
		if sample != "" {
			fmt.Fprintf(out, "    sample: %s\n", sample)
		}
		if description != "" {
			fmt.Fprintf(out, "    description: %s\n", description)
		}
	}
}
