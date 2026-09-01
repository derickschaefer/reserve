// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"strings"
	"testing"

	"github.com/derickschaefer/reserve/internal/workflow"
)

func TestWorkflowTemplateParsesAndNormalizes(t *testing.T) {
	doc := workflow.Document{
		Workflow: workflow.Workflow{
			Title:            "  Inflation Dashboard  ",
			Summary:          "  Repeatable CPI workflow  ",
			Difficulty:       " Beginner ",
			EstimatedRuntime: " 5 seconds ",
			Categories:       []string{" Inflation ", "inflation"},
			Concepts:         []string{" CPI ", "cpi"},
			Outputs:          []string{" JSON ", "json"},
			RequiresNetwork:  true,
			ReserveVersion:   " >=1.2 ",
			Author:           " Derick Schaefer ",
			Documentation:    " README.md ",
			Contract: []workflow.ContractSlot{
				{Label: "start date", Format: "YYYY-MM-DD", Sample: "2020-01-01", Description: "First date in the requested range"},
				{Label: "end date", Format: "YYYY-MM-DD", Sample: "2024-12-31", Description: "Final date in the requested range"},
			},
		},
	}

	data, err := workflowDocumentBytes(doc)
	if err != nil {
		t.Fatalf("workflowDocumentBytes: %v", err)
	}
	if !strings.Contains(string(data), "workflow:") {
		t.Fatalf("expected workflow document wrapper, got %s", string(data))
	}
	if !strings.Contains(string(data), "contract:") {
		t.Fatalf("expected contract field in workflow document, got %s", string(data))
	}
}

func TestTruncateText(t *testing.T) {
	if got := truncateText("short", 10); got != "short" {
		t.Fatalf("truncateText(short) = %q", got)
	}
	if got := truncateText("this is a very long summary", 12); got != "this is a v…" {
		t.Fatalf("truncateText(long) = %q", got)
	}
}
