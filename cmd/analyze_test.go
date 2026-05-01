// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestAnalyzeSummaryBySeriesJSON(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"FEDFUNDS","date":"2025-01-01","value":4.25,"value_raw":"4.25"}`,
		`{"series_id":"UNRATE","date":"2025-01-01","value":4.0,"value_raw":"4.0"}`,
		`{"series_id":"FEDFUNDS","date":"2025-02-01","value":4.5,"value_raw":"4.5"}`,
		`{"series_id":"UNRATE","date":"2025-02-01","value":4.1,"value_raw":"4.1"}`,
	}, "\n") + "\n"

	out, err := runAnalyzeSummaryForTest(t, input, true, "json")
	if err != nil {
		t.Fatalf("runAnalyzeSummaryForTest: %v", err)
	}

	var summaries []map[string]any
	if err := json.Unmarshal([]byte(out), &summaries); err != nil {
		t.Fatalf("unmarshal summaries: %v\n%s", err, out)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0]["series_id"] != "FEDFUNDS" {
		t.Fatalf("first summary series_id = %v", summaries[0]["series_id"])
	}
	if summaries[1]["series_id"] != "UNRATE" {
		t.Fatalf("second summary series_id = %v", summaries[1]["series_id"])
	}
}

func TestAnalyzeSummaryBySeriesJSONL(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"FEDFUNDS","date":"2025-01-01","value":4.25,"value_raw":"4.25"}`,
		`{"series_id":"UNRATE","date":"2025-01-01","value":4.0,"value_raw":"4.0"}`,
		`{"series_id":"FEDFUNDS","date":"2025-02-01","value":4.5,"value_raw":"4.5"}`,
		`{"series_id":"UNRATE","date":"2025-02-01","value":4.1,"value_raw":"4.1"}`,
	}, "\n") + "\n"

	out, err := runAnalyzeSummaryForTest(t, input, true, "jsonl")
	if err != nil {
		t.Fatalf("runAnalyzeSummaryForTest: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 jsonl lines, got %d\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], `"series_id":"FEDFUNDS"`) {
		t.Fatalf("first line missing FEDFUNDS: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"series_id":"UNRATE"`) {
		t.Fatalf("second line missing UNRATE: %s", lines[1])
	}
}

func TestAnalyzeSummarySingleSeriesJSONL(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"FEDFUNDS","date":"2025-01-01","value":4.25,"value_raw":"4.25"}`,
		`{"series_id":"FEDFUNDS","date":"2025-02-01","value":4.5,"value_raw":"4.5"}`,
	}, "\n") + "\n"

	out, err := runAnalyzeSummaryForTest(t, input, false, "jsonl")
	if err != nil {
		t.Fatalf("runAnalyzeSummaryForTest: %v", err)
	}
	if !strings.Contains(out, `"series_id":"FEDFUNDS"`) {
		t.Fatalf("missing series_id in output: %s", out)
	}
	if strings.Count(strings.TrimSpace(out), "\n")+1 != 1 {
		t.Fatalf("expected single jsonl object, got:\n%s", out)
	}
}

func runAnalyzeSummaryForTest(t *testing.T, input string, bySeries bool, format string) (string, error) {
	t.Helper()

	tmp, err := os.CreateTemp(t.TempDir(), "analyze-stdin-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := tmp.WriteString(input); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}

	origStdin := os.Stdin
	origFormat := globalFlags.Format
	origBySeries := analyzeSummaryBySeries
	os.Stdin = tmp
	globalFlags.Format = format
	analyzeSummaryBySeries = bySeries
	t.Cleanup(func() {
		os.Stdin = origStdin
		globalFlags.Format = origFormat
		analyzeSummaryBySeries = origBySeries
		_ = tmp.Close()
	})

	var buf bytes.Buffer
	analyzeSummaryCmd.SetOut(&buf)
	analyzeSummaryCmd.SetErr(&buf)
	err = analyzeSummaryCmd.RunE(analyzeSummaryCmd, nil)
	return buf.String(), err
}
