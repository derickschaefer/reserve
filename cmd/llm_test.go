package cmd

import (
	"reflect"
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
