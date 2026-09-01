// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package workflow

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document is the on-disk YAML representation of a workflow.
//
// The document is intentionally wrapped so future top-level keys can be added
// without changing the workflow metadata shape.
type Document struct {
	Workflow Workflow `yaml:"workflow"`
}

// Workflow captures the core workflow metadata for RESERVE 1.2.
//
// The fields here are the proposed YAML vocabulary for the ecosystem. The
// values are kept as plain strings/slices so authors can edit them by hand
// while strict decoding catches unsupported fields.
type Workflow struct {
	Title            string         `yaml:"title,omitempty"`
	Summary          string         `yaml:"summary,omitempty"`
	Difficulty       string         `yaml:"difficulty,omitempty"`
	EstimatedRuntime string         `yaml:"estimated_runtime,omitempty"`
	Categories       []string       `yaml:"categories,omitempty"`
	Concepts         []string       `yaml:"concepts,omitempty"`
	Outputs          []string       `yaml:"outputs,omitempty"`
	RequiresNetwork  bool           `yaml:"requires_network,omitempty"`
	ReserveVersion   string         `yaml:"reserve_version,omitempty"`
	Author           string         `yaml:"author,omitempty"`
	Documentation    string         `yaml:"documentation,omitempty"`
	Contract         []ContractSlot `yaml:"contract,omitempty"`
	Pipeline         []string       `yaml:"pipeline,omitempty"`
}

// ContractSlot describes one positional runtime input for a workflow.
type ContractSlot struct {
	Label       string `yaml:"label,omitempty"`
	Format      string `yaml:"format,omitempty"`
	Sample      string `yaml:"sample,omitempty"`
	Description string `yaml:"description,omitempty"`
}

var (
	contractPlaceholderRE = regexp.MustCompile(`@([1-9][0-9]*)`)
	versionConstraintRE   = regexp.MustCompile(`^(>=|<=|==|=|>|<)?\s*(v?[0-9]+\.[0-9]+(?:\.[0-9]+)?)$`)
)

// Parse decodes a workflow document from YAML bytes and normalizes its fields.
func Parse(data []byte) (Document, error) {
	var doc Document
	if err := decodeStrictYAML(data, &doc); err != nil {
		return Document{}, fmt.Errorf("parse workflow yaml: %w", err)
	}
	doc.Normalize()
	return doc, nil
}

// LoadFile reads and parses a workflow document from disk.
func LoadFile(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	return Parse(data)
}

// WriteFile serializes a workflow document to disk as pretty-printed YAML.
func WriteFile(path string, doc Document) error {
	doc.Normalize()
	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode workflow yaml: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create workflow dir: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write workflow yaml: %w", err)
	}
	return nil
}

// Normalize trims whitespace and deduplicates list fields in-place.
func (d *Document) Normalize() {
	d.Workflow.Normalize()
}

// Normalize trims whitespace and deduplicates list fields in-place.
func (w *Workflow) Normalize() {
	w.Title = strings.TrimSpace(w.Title)
	w.Summary = strings.TrimSpace(w.Summary)
	w.Difficulty = strings.TrimSpace(w.Difficulty)
	w.EstimatedRuntime = strings.TrimSpace(w.EstimatedRuntime)
	w.ReserveVersion = strings.TrimSpace(w.ReserveVersion)
	w.Author = strings.TrimSpace(w.Author)
	w.Documentation = strings.TrimSpace(w.Documentation)
	w.Contract = normalizeContract(w.Contract)
	w.Categories = normalizeSlugs(w.Categories)
	w.Outputs = normalizeSlugs(w.Outputs)
	w.Concepts = normalizeLabels(w.Concepts)
	w.Pipeline = normalizePipeline(w.Pipeline)
}

// Validate reports structural problems with a workflow document.
//
// The v1.2.0 foundation intentionally keeps validation light: we want to catch
// empty or malformed documents without freezing the format before the rest of
// the ecosystem lands.
func (d Document) Validate() error {
	if !d.Workflow.HasContent() {
		return fmt.Errorf("workflow document is empty")
	}
	if err := d.Workflow.ValidateContract(); err != nil {
		return err
	}
	if err := d.Workflow.ValidateReserveVersion(""); err != nil {
		return err
	}
	return nil
}

// ValidateForVersion validates the document and checks compatibility with the
// running reserve version.
func (d Document) ValidateForVersion(currentVersion string) error {
	if err := d.Validate(); err != nil {
		return err
	}
	return d.Workflow.ValidateReserveVersion(currentVersion)
}

// HasContent reports whether any workflow metadata is present.
func (w Workflow) HasContent() bool {
	return strings.TrimSpace(w.Title) != "" ||
		strings.TrimSpace(w.Summary) != "" ||
		strings.TrimSpace(w.Difficulty) != "" ||
		strings.TrimSpace(w.EstimatedRuntime) != "" ||
		len(w.Categories) > 0 ||
		len(w.Concepts) > 0 ||
		len(w.Outputs) > 0 ||
		w.RequiresNetwork ||
		strings.TrimSpace(w.ReserveVersion) != "" ||
		strings.TrimSpace(w.Author) != "" ||
		strings.TrimSpace(w.Documentation) != "" ||
		len(w.Contract) > 0 ||
		len(w.Pipeline) > 0
}

// ValidateContract reports problems with workflow runtime inputs and pipeline references.
func (w Workflow) ValidateContract() error {
	max := maxContractPlaceholder(w.Pipeline)
	if len(w.Contract) == 0 {
		if max > 0 {
			return fmt.Errorf("workflow pipeline references @%d but contract declares no runtime slots", max)
		}
		return nil
	}

	for i, slot := range w.Contract {
		if strings.TrimSpace(slot.Label) == "" && strings.TrimSpace(slot.Format) == "" && strings.TrimSpace(slot.Description) == "" {
			return fmt.Errorf("workflow contract slot %d is empty", i+1)
		}
	}

	if max > len(w.Contract) {
		return fmt.Errorf("workflow pipeline references @%d but contract declares only %d slot(s)", max, len(w.Contract))
	}

	for i := range w.Contract {
		if !contractSlotReferenced(w.Pipeline, i+1) {
			return fmt.Errorf("workflow contract slot @%d is not used by the pipeline", i+1)
		}
	}
	return nil
}

// ValidateReserveVersion validates reserve_version and, when currentVersion is
// provided, reports whether the running binary satisfies it.
func (w Workflow) ValidateReserveVersion(currentVersion string) error {
	requirement := strings.TrimSpace(w.ReserveVersion)
	if requirement == "" {
		return nil
	}

	match := versionConstraintRE.FindStringSubmatch(requirement)
	if len(match) != 3 {
		return fmt.Errorf("invalid reserve_version %q: expected a single constraint such as >=1.2", requirement)
	}
	operator := match[1]
	if operator == "" {
		operator = "="
	}
	required, err := parseSemanticVersion(match[2])
	if err != nil {
		return fmt.Errorf("invalid reserve_version %q: %w", requirement, err)
	}
	if strings.TrimSpace(currentVersion) == "" {
		return nil
	}
	current, err := parseSemanticVersion(currentVersion)
	if err != nil {
		return fmt.Errorf("cannot enforce reserve_version %q against running version %q: %w", requirement, currentVersion, err)
	}

	comparison := compareSemanticVersions(current, required)
	compatible := false
	switch operator {
	case ">=":
		compatible = comparison >= 0
	case ">":
		compatible = comparison > 0
	case "<=":
		compatible = comparison <= 0
	case "<":
		compatible = comparison < 0
	case "=", "==":
		compatible = comparison == 0
	}
	if !compatible {
		return fmt.Errorf("workflow requires reserve %s; running version is %s", requirement, currentVersion)
	}
	return nil
}

// ApplyContract expands positional runtime values into a workflow pipeline.
func (w Workflow) ApplyContract(inputs []string) ([]string, error) {
	if len(inputs) != len(w.Contract) {
		return nil, fmt.Errorf("workflow requires %d runtime value(s), got %d", len(w.Contract), len(inputs))
	}
	out := make([]string, 0, len(w.Pipeline))
	for _, step := range w.Pipeline {
		expanded, err := expandContractStep(step, inputs)
		if err != nil {
			return nil, err
		}
		out = append(out, expanded)
	}
	return out, nil
}

func normalizeSlugs(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeLabels(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizePipeline(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeContract(in []ContractSlot) []ContractSlot {
	if len(in) == 0 {
		return nil
	}
	out := make([]ContractSlot, 0, len(in))
	for _, slot := range in {
		slot.Label = strings.TrimSpace(slot.Label)
		slot.Format = strings.TrimSpace(slot.Format)
		slot.Sample = strings.TrimSpace(slot.Sample)
		slot.Description = strings.TrimSpace(slot.Description)
		if slot.Label == "" && slot.Format == "" && slot.Description == "" {
			continue
		}
		out = append(out, slot)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func expandContractStep(step string, inputs []string) (string, error) {
	var expandErr error
	expanded := contractPlaceholderRE.ReplaceAllStringFunc(step, func(match string) string {
		if expandErr != nil {
			return match
		}
		sub := contractPlaceholderRE.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		idx, err := strconv.Atoi(sub[1])
		if err != nil || idx < 1 || idx > len(inputs) {
			expandErr = fmt.Errorf("workflow pipeline references %s but only %d runtime value(s) were provided", match, len(inputs))
			return match
		}
		return shellQuote(inputs[idx-1])
	})
	if expandErr != nil {
		return "", expandErr
	}
	return expanded, nil
}

func maxContractPlaceholder(pipeline []string) int {
	max := 0
	for _, step := range pipeline {
		matches := contractPlaceholderRE.FindAllStringSubmatch(step, -1)
		for _, match := range matches {
			if len(match) != 2 {
				continue
			}
			idx, err := strconv.Atoi(match[1])
			if err != nil {
				continue
			}
			if idx > max {
				max = idx
			}
		}
	}
	return max
}

func contractSlotReferenced(pipeline []string, slot int) bool {
	token := fmt.Sprintf("@%d", slot)
	for _, step := range pipeline {
		if strings.Contains(step, token) {
			return true
		}
	}
	return false
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func decodeStrictYAML(data []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple YAML documents are not supported")
		}
		return err
	}
	return nil
}

func parseSemanticVersion(value string) ([3]int, error) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "v"))
	parts := strings.Split(value, ".")
	if len(parts) != 2 && len(parts) != 3 {
		return [3]int{}, fmt.Errorf("expected version X.Y or X.Y.Z")
	}
	var version [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return [3]int{}, fmt.Errorf("invalid numeric version segment %q", part)
		}
		version[i] = n
	}
	return version, nil
}

func compareSemanticVersions(left, right [3]int) int {
	for i := range left {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	return 0
}
