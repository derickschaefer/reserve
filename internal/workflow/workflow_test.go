// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package workflow

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNormalizeAndValidate(t *testing.T) {
	data := []byte(`
workflow:
  title:   Inflation Dashboard
  summary:  A repeatable CPI workflow
  difficulty: Beginner
  estimated_runtime: 5 seconds
  categories:
    - inflation
    - Inflation
    -   inflation
  concepts:
    - CPI
    - FRED
    - Time Series
    - cpi
  outputs:
    - chart
    - json
    - JSON
  requires_network: true
  reserve_version: ">=1.2"
  author:  Derick Schaefer
  documentation: README.md
  contract:
    - label: start date
      format: YYYY-MM-DD
      sample: 2020-01-01
      description: First date in the requested range
    - label: end date
      format: YYYY-MM-DD
      sample: 2024-12-31
      description: Final date in the requested range
  pipeline:
    -   printf 'start=%s end=%s\n' @1 @2
    - printf 'ignored empty check'
`)

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	got := doc.Workflow
	if got.Title != "Inflation Dashboard" {
		t.Fatalf("Title = %q", got.Title)
	}
	if got.Summary != "A repeatable CPI workflow" {
		t.Fatalf("Summary = %q", got.Summary)
	}
	if got.EstimatedRuntime != "5 seconds" {
		t.Fatalf("EstimatedRuntime = %q", got.EstimatedRuntime)
	}
	if len(got.Contract) != 2 || got.Contract[0].Label != "start date" || got.Contract[1].Format != "YYYY-MM-DD" {
		t.Fatalf("Contract = %#v", got.Contract)
	}
	if strings.Join(got.Categories, ",") != "inflation" {
		t.Fatalf("Categories = %#v", got.Categories)
	}
	if strings.Join(got.Concepts, ",") != "CPI,FRED,Time Series" {
		t.Fatalf("Concepts = %#v", got.Concepts)
	}
	if strings.Join(got.Outputs, ",") != "chart,json" {
		t.Fatalf("Outputs = %#v", got.Outputs)
	}
	if !got.RequiresNetwork {
		t.Fatalf("RequiresNetwork = false")
	}
	if got.ReserveVersion != ">=1.2" {
		t.Fatalf("ReserveVersion = %q", got.ReserveVersion)
	}
	if len(got.Pipeline) != 2 || got.Pipeline[0] != "printf 'start=%s end=%s\\n' @1 @2" {
		t.Fatalf("Pipeline = %#v", got.Pipeline)
	}

	rendered, err := got.ApplyContract([]string{"2020-01-01", "2024-12-31"})
	if err != nil {
		t.Fatalf("ApplyContract: %v", err)
	}
	if !strings.Contains(rendered[0], "'2020-01-01'") || !strings.Contains(rendered[0], "'2024-12-31'") {
		t.Fatalf("ApplyContract rendered step = %#v", rendered)
	}
}

func TestWriteAndLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	doc := Document{
		Workflow: Workflow{
			Title:         "GDP Trend",
			Summary:       "A simple trend workflow",
			Categories:    []string{"gdp", "GDP"},
			Concepts:      []string{"Time Series"},
			Outputs:       []string{"json"},
			Documentation: "README.md",
		},
	}

	if err := WriteFile(path, doc); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got.Workflow.Title != "GDP Trend" {
		t.Fatalf("unexpected title %q", got.Workflow.Title)
	}
	if strings.Join(got.Workflow.Categories, ",") != "gdp" {
		t.Fatalf("unexpected categories %#v", got.Workflow.Categories)
	}
}

func TestEmptyDocumentIsInvalid(t *testing.T) {
	if err := (Document{}).Validate(); err == nil {
		t.Fatalf("expected empty document to be invalid")
	}
}

func TestParseRejectsUnknownFieldsAndMultipleDocuments(t *testing.T) {
	tests := map[string]string{
		"unknown workflow field":  "workflow:\n  title: Example\n  pipline: []\n",
		"unknown top-level field": "workflow:\n  title: Example\nextra: true\n",
		"multiple documents":      "workflow:\n  title: Example\n---\nworkflow:\n  title: Other\n",
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(data)); err == nil {
				t.Fatalf("expected strict YAML parsing to fail")
			}
		})
	}
}

func TestReserveVersionValidation(t *testing.T) {
	tests := []struct {
		name        string
		requirement string
		current     string
		wantErr     bool
	}{
		{name: "minimum compatible", requirement: ">=1.2", current: "v1.2.0"},
		{name: "minimum newer compatible", requirement: ">=1.2", current: "v1.3.0"},
		{name: "maximum compatible", requirement: "<2.0", current: "v1.2.0"},
		{name: "exact compatible", requirement: "1.2.0", current: "v1.2.0"},
		{name: "minimum incompatible", requirement: ">=1.3", current: "v1.2.0", wantErr: true},
		{name: "exact incompatible", requirement: "=1.2.1", current: "v1.2.0", wantErr: true},
		{name: "unsupported expression", requirement: "^1.2", current: "v1.2.0", wantErr: true},
		{name: "invalid version", requirement: ">=banana", current: "v1.2.0", wantErr: true},
		{name: "empty requirement", current: "v1.2.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Document{Workflow: Workflow{Title: "Example", ReserveVersion: tt.requirement}}
			err := doc.ValidateForVersion(tt.current)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateForVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRejectsMalformedReserveVersionWithoutCurrentVersion(t *testing.T) {
	doc := Document{Workflow: Workflow{Title: "Example", ReserveVersion: ">=1.2 <2.0"}}
	if err := doc.Validate(); err == nil {
		t.Fatalf("expected malformed reserve_version to fail validation")
	}
}

func TestContractValidation(t *testing.T) {
	valid := Workflow{
		Contract: []ContractSlot{
			{Label: "start date", Format: "YYYY-MM-DD", Sample: "2020-01-01", Description: "First date in the requested range"},
			{Label: "end date", Format: "YYYY-MM-DD", Sample: "2024-12-31", Description: "Final date in the requested range"},
		},
		Pipeline: []string{"printf @1 @2"},
	}
	if err := valid.ValidateContract(); err != nil {
		t.Fatalf("ValidateContract(valid): %v", err)
	}

	noSlots := Workflow{Pipeline: []string{"printf @1"}}
	if err := noSlots.ValidateContract(); err == nil {
		t.Fatalf("expected placeholder without contract to fail")
	}
}
