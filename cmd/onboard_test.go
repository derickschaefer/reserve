// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestParseOnboardTopicsDefaultIsTOC(t *testing.T) {
	got := parseOnboardTopics("")
	want := []string{"toc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default topics mismatch: got %v want %v", got, want)
	}
}

func TestParseOnboardTopicsAll(t *testing.T) {
	got := parseOnboardTopics("all")
	if len(got) != len(topicRegistry) {
		t.Fatalf("all topics size mismatch: got %d want %d", len(got), len(topicRegistry))
	}
	for i, tpc := range topicRegistry {
		if got[i] != tpc.Name {
			t.Fatalf("topic index %d mismatch: got %q want %q", i, got[i], tpc.Name)
		}
	}
}

func TestBuildProgramOnboardDocIncludesRoutingIndexes(t *testing.T) {
	doc := buildProgramOnboardDoc()

	if got := doc["scope"]; got != "program" {
		t.Fatalf("scope = %v, want program", got)
	}

	program, ok := doc["program"].(map[string]any)
	if !ok {
		t.Fatalf("program section missing or wrong type")
	}

	if got := program["command_count"]; got != len(onboardCommandRegistry) {
		t.Fatalf("command_count = %v, want %d", got, len(onboardCommandRegistry))
	}

	commandIndex, ok := program["command_index"].([]map[string]any)
	if !ok {
		t.Fatalf("command_index missing or wrong type")
	}
	if len(commandIndex) != len(onboardCommandRegistry) {
		t.Fatalf("command_index size = %d, want %d", len(commandIndex), len(onboardCommandRegistry))
	}

	topicIndex, ok := program["topic_index"].(map[string]any)
	if !ok {
		t.Fatalf("topic_index missing or wrong type")
	}
	if _, ok := topicIndex["topics"]; !ok {
		t.Fatalf("topic_index missing topics field")
	}
}

func TestBuildCommandOnboardDoc(t *testing.T) {
	doc, ok := buildCommandOnboardDoc("series")
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

func TestBuildCommandOnboardDocUnknown(t *testing.T) {
	if _, ok := buildCommandOnboardDoc("does-not-exist"); ok {
		t.Fatalf("expected unknown command to fail lookup")
	}
}

func TestOnboardCommandRegistryMatchesTopLevelCommands(t *testing.T) {
	got := onboardCommandNames()
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
		t.Fatalf("onboard command registry mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestAllCommandGuidesHaveRequiredFields(t *testing.T) {
	required := []string{
		"purpose",
		"summary",
		"description",
		"mental_model",
		"when_to_use",
		"when_not_to_use",
		"common_user_intents",
		"pipeline_role",
		"input_output_contract",
		"verbs",
		"flags",
		"output_kinds",
		"examples",
		"gotchas",
		"related_commands",
	}

	for _, guide := range onboardCommandRegistry {
		doc := guide.Build()
		for _, field := range required {
			v, ok := doc[field]
			if !ok {
				t.Fatalf("%s guide missing required field %q", guide.Name, field)
			}
			switch vv := v.(type) {
			case string:
				if vv == "" {
					t.Fatalf("%s guide has empty string field %q", guide.Name, field)
				}
			case []string:
				if len(vv) == 0 {
					t.Fatalf("%s guide has empty list field %q", guide.Name, field)
				}
			case map[string]any:
				if len(vv) == 0 {
					t.Fatalf("%s guide has empty map field %q", guide.Name, field)
				}
			default:
				t.Fatalf("%s guide field %q has unsupported type %T", guide.Name, field, v)
			}
		}
	}
}

func TestStoreCommandRemoved(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "store" {
			t.Fatalf("store command should be removed from the root command tree")
		}
	}
}

func TestExportOnboardBundleWritesProgramAndCommandFiles(t *testing.T) {
	dir := t.TempDir()
	if err := exportOnboardBundle(dir); err != nil {
		t.Fatalf("exportOnboardBundle: %v", err)
	}

	programPath := filepath.Join(dir, "program.json")
	if _, err := os.Stat(programPath); err != nil {
		t.Fatalf("program.json missing: %v", err)
	}

	for _, guide := range onboardCommandRegistry {
		path := filepath.Join(dir, guide.Name+".json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s missing: %v", path, err)
		}
	}
}

func TestExportOnboardBundleWritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := exportOnboardBundle(dir); err != nil {
		t.Fatalf("exportOnboardBundle: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "obs.json"))
	if err != nil {
		t.Fatalf("read obs.json: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal obs.json: %v", err)
	}
	if got := doc["scope"]; got != "command" {
		t.Fatalf("scope = %v, want command", got)
	}
	if got := doc["command_name"]; got != "obs" {
		t.Fatalf("command_name = %v, want obs", got)
	}
}

func TestAnalyzeGuideHighlightsBySeries(t *testing.T) {
	doc, ok := buildCommandOnboardDoc("analyze")
	if !ok {
		t.Fatalf("expected analyze guide")
	}
	cmdDoc := doc["command"].(map[string]any)
	gotchas := cmdDoc["gotchas"].([]string)

	found := false
	for _, item := range gotchas {
		if strings.Contains(item, "--by-series") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("analyze guide should mention --by-series in gotchas")
	}
}

func TestObsGuideHighlightsBatchedObsGet(t *testing.T) {
	doc, ok := buildCommandOnboardDoc("obs")
	if !ok {
		t.Fatalf("expected obs guide")
	}
	cmdDoc := doc["command"].(map[string]any)
	examples := cmdDoc["examples"].([]string)

	found := false
	for _, item := range examples {
		if strings.Contains(item, "analyze summary --by-series") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("obs guide should include batched by-series example")
	}
}
