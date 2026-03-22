// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"reflect"
	"sort"
	"testing"
)

func TestParseLLMTopicsDefaultIsTOC(t *testing.T) {
	got := parseLLMTopics("")
	want := []string{"toc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default topics mismatch: got %v want %v", got, want)
	}
}

func TestParseLLMTopicsAll(t *testing.T) {
	got := parseLLMTopics("all")
	if len(got) != len(topicRegistry) {
		t.Fatalf("all topics size mismatch: got %d want %d", len(got), len(topicRegistry))
	}
	for i, tpc := range topicRegistry {
		if got[i] != tpc.Name {
			t.Fatalf("topic index %d mismatch: got %q want %q", i, got[i], tpc.Name)
		}
	}
}

func TestBuildProgramLLMDocIncludesAllCommandGuides(t *testing.T) {
	doc := buildProgramLLMDoc()

	if got := doc["scope"]; got != "program" {
		t.Fatalf("scope = %v, want program", got)
	}

	program, ok := doc["program"].(map[string]any)
	if !ok {
		t.Fatalf("program section missing or wrong type")
	}

	if got := program["command_count"]; got != len(llmCommandRegistry) {
		t.Fatalf("command_count = %v, want %d", got, len(llmCommandRegistry))
	}

	guides, ok := program["command_guides"].(map[string]any)
	if !ok {
		t.Fatalf("command_guides missing or wrong type")
	}
	if len(guides) != len(llmCommandRegistry) {
		t.Fatalf("command_guides size = %d, want %d", len(guides), len(llmCommandRegistry))
	}
}

func TestBuildCommandLLMDoc(t *testing.T) {
	doc, ok := buildCommandLLMDoc("series")
	if !ok {
		t.Fatalf("expected series guide")
	}
	if got := doc["scope"]; got != "command" {
		t.Fatalf("scope = %v, want command", got)
	}
	if got := doc["command_name"]; got != "series" {
		t.Fatalf("command_name = %v, want series", got)
	}

	cmdDoc, ok := doc["command"].(map[string]any)
	if !ok {
		t.Fatalf("command section missing or wrong type")
	}
	verbs, ok := cmdDoc["verbs"].(map[string]any)
	if !ok {
		t.Fatalf("verbs missing or wrong type")
	}
	if _, exists := verbs["get"]; !exists {
		t.Fatalf("series guide missing get verb")
	}
	if _, exists := verbs["search"]; !exists {
		t.Fatalf("series guide missing search verb")
	}
	if _, exists := verbs["describe"]; exists {
		t.Fatalf("series guide should not advertise describe")
	}
}

func TestBuildCommandLLMDocUnknown(t *testing.T) {
	if _, ok := buildCommandLLMDoc("does-not-exist"); ok {
		t.Fatalf("expected unknown command to fail lookup")
	}
}

func TestLLMCommandRegistryMatchesTopLevelCommands(t *testing.T) {
	got := llmCommandNames()
	sort.Strings(got)

	var want []string
	for _, c := range rootCmd.Commands() {
		if c.Hidden {
			continue
		}
		if c.Name() == "help" {
			continue
		}
		want = append(want, c.Name())
	}
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("llm command registry mismatch:\n got: %v\nwant: %v", got, want)
	}
}
