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
		`{"series_id":"FEDFUNDS","date":"2025-01-01","value":4.25,"value_raw":"4.25","source_name":"Board of Governors","source_names":["Board of Governors"],"citation_text":"Source: Board of Governors via FRED"}`,
		`{"series_id":"FEDFUNDS","date":"2025-02-01","value":4.5,"value_raw":"4.5","source_name":"Board of Governors","source_names":["Board of Governors"],"citation_text":"Source: Board of Governors via FRED"}`,
	}, "\n") + "\n"

	out, err := runAnalyzeSummaryForTest(t, input, false, "jsonl")
	if err != nil {
		t.Fatalf("runAnalyzeSummaryForTest: %v", err)
	}
	if !strings.Contains(out, `"series_id":"FEDFUNDS"`) {
		t.Fatalf("missing series_id in output: %s", out)
	}
	if !strings.Contains(out, `"source_name":"Board of Governors"`) {
		t.Fatalf("missing source_name in output: %s", out)
	}
	if !strings.Contains(out, `"citation_text":"Source: Board of Governors via FRED"`) {
		t.Fatalf("missing citation_text in output: %s", out)
	}
	if strings.Count(strings.TrimSpace(out), "\n")+1 != 1 {
		t.Fatalf("expected single jsonl object, got:\n%s", out)
	}
}

func TestAnalyzeSummarySingleSeriesTable(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"GDP","date":"2020-01-01","value":21751.238,"value_raw":"21751.238"}`,
		`{"series_id":"GDP","date":"2020-04-01","value":19958.291,"value_raw":"19958.291"}`,
	}, "\n") + "\n"

	out, err := runAnalyzeSummaryForTest(t, input, false, "table")
	if err != nil {
		t.Fatalf("runAnalyzeSummaryForTest: %v", err)
	}
	for _, token := range []string{"+", "METRIC", "VALUE", "Version", "Data Quality", "Distribution", "Movement", "Missing Count", "Missing %", "21,751.2380", "19,958.2910"} {
		if !strings.Contains(out, token) {
			t.Fatalf("table output missing %q:\n%s", token, out)
		}
	}
}

func TestAnalyzeTrendSingleSeriesTable(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"GDP","date":"2020-01-01","value":1.0,"value_raw":"1.0","citation_text":"Source: Bureau of Economic Analysis via FRED"}`,
		`{"series_id":"GDP","date":"2020-04-01","value":2.0,"value_raw":"2.0","citation_text":"Source: Bureau of Economic Analysis via FRED"}`,
		`{"series_id":"GDP","date":"2020-07-01","value":3.0,"value_raw":"3.0","citation_text":"Source: Bureau of Economic Analysis via FRED"}`,
	}, "\n") + "\n"

	tmp, err := os.CreateTemp(t.TempDir(), "analyze-trend-stdin-*.jsonl")
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
	origMethod := analyzeTrendMethod
	os.Stdin = tmp
	globalFlags.Format = "table"
	analyzeTrendMethod = "linear"
	t.Cleanup(func() {
		os.Stdin = origStdin
		globalFlags.Format = origFormat
		analyzeTrendMethod = origMethod
		_ = tmp.Close()
	})

	var buf bytes.Buffer
	analyzeTrendCmd.SetOut(&buf)
	analyzeTrendCmd.SetErr(&buf)
	if err := analyzeTrendCmd.RunE(analyzeTrendCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	out := buf.String()
	for _, token := range []string{"METRIC", "VALUE", "Context", "Trend", "Fit", "Slope / Year", "R2", "1.0000"} {
		if !strings.Contains(out, token) {
			t.Fatalf("trend table output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "Sources") {
		t.Fatalf("unexpected Sources row in trend table:\n%s", out)
	}
	if !strings.Contains(out, "Source: Bureau of Economic Analysis via FRED") {
		t.Fatalf("expected citation footer in trend table:\n%s", out)
	}
}

func TestAnalyzeTrendConfidenceJSON(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"GDP","date":"2020-01-01","value":1.0,"value_raw":"1.0"}`,
		`{"series_id":"GDP","date":"2020-04-01","value":2.0,"value_raw":"2.0"}`,
		`{"series_id":"GDP","date":"2020-07-01","value":3.0,"value_raw":"3.0"}`,
		`{"series_id":"GDP","date":"2020-10-01","value":4.0,"value_raw":"4.0"}`,
	}, "\n") + "\n"

	tmp, err := os.CreateTemp(t.TempDir(), "analyze-trend-confidence-stdin-*.jsonl")
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
	origMethod := analyzeTrendMethod
	origConfidence := analyzeTrendConfidence
	os.Stdin = tmp
	globalFlags.Format = "json"
	analyzeTrendMethod = "linear"
	analyzeTrendConfidence = true
	t.Cleanup(func() {
		os.Stdin = origStdin
		globalFlags.Format = origFormat
		analyzeTrendMethod = origMethod
		analyzeTrendConfidence = origConfidence
		_ = tmp.Close()
	})

	var buf bytes.Buffer
	analyzeTrendCmd.SetOut(&buf)
	analyzeTrendCmd.SetErr(&buf)
	if err := analyzeTrendCmd.RunE(analyzeTrendCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, buf.String())
	}
	conf, ok := payload["confidence"].(map[string]any)
	if !ok {
		t.Fatalf("expected confidence object in JSON output: %v", payload)
	}
	if _, ok := conf["slope_stderr"]; !ok {
		t.Fatalf("confidence missing slope_stderr: %v", conf)
	}
}

func TestAnalyzeSummaryWindowJSONL(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"GDP","date":"2020-01-01","value":1.0,"value_raw":"1.0"}`,
		`{"series_id":"GDP","date":"2020-04-01","value":2.0,"value_raw":"2.0"}`,
		`{"series_id":"GDP","date":"2020-07-01","value":3.0,"value_raw":"3.0"}`,
		`{"series_id":"GDP","date":"2020-10-01","value":4.0,"value_raw":"4.0"}`,
	}, "\n") + "\n"

	tmp, err := os.CreateTemp(t.TempDir(), "analyze-summary-window-stdin-*.jsonl")
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
	origWindow := analyzeSummaryWindow
	os.Stdin = tmp
	globalFlags.Format = "jsonl"
	analyzeSummaryWindow = 2
	t.Cleanup(func() {
		os.Stdin = origStdin
		globalFlags.Format = origFormat
		analyzeSummaryWindow = origWindow
		_ = tmp.Close()
	})

	var buf bytes.Buffer
	analyzeSummaryCmd.SetOut(&buf)
	analyzeSummaryCmd.SetErr(&buf)
	if err := analyzeSummaryCmd.RunE(analyzeSummaryCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 rolling summaries, got %d\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], `"start_date":"2020-01-01"`) || !strings.Contains(lines[0], `"end_date":"2020-04-01"`) {
		t.Fatalf("unexpected first window boundaries: %s", lines[0])
	}
}

func TestAnalyzeSummaryWindowTableUsesCompactMissColumn(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"GDP","date":"2020-01-01","value":1.0,"value_raw":"1.0"}`,
		`{"series_id":"GDP","date":"2020-04-01","value":2.0,"value_raw":"2.0"}`,
		`{"series_id":"GDP","date":"2020-07-01","value":3.0,"value_raw":"3.0"}`,
		`{"series_id":"GDP","date":"2020-10-01","value":4.0,"value_raw":"4.0"}`,
	}, "\n") + "\n"

	tmp, err := os.CreateTemp(t.TempDir(), "analyze-summary-window-table-stdin-*.jsonl")
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
	origWindow := analyzeSummaryWindow
	os.Stdin = tmp
	globalFlags.Format = "table"
	analyzeSummaryWindow = 2
	t.Cleanup(func() {
		os.Stdin = origStdin
		globalFlags.Format = origFormat
		analyzeSummaryWindow = origWindow
		_ = tmp.Close()
	})

	var buf bytes.Buffer
	analyzeSummaryCmd.SetOut(&buf)
	analyzeSummaryCmd.SetErr(&buf)
	if err := analyzeSummaryCmd.RunE(analyzeSummaryCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "MISS") {
		t.Fatalf("expected compact MISS column header:\n%s", out)
	}
	if strings.Contains(out, "SRC") {
		t.Fatalf("unexpected source column in table:\n%s", out)
	}
	if strings.Contains(out, "MISSING_COUNT") || strings.Contains(out, "MISSING_PCT") {
		t.Fatalf("unexpected wide missing headers in window table:\n%s", out)
	}
	if !strings.Contains(out, "0|0.0%") {
		t.Fatalf("expected compact MISS value in window table:\n%s", out)
	}
}

func TestAnalyzeCompareJSON(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"A","date":"2020-01-01","value":1.0,"value_raw":"1.0","citation_text":"Source: Alpha Bureau via FRED","source_name":"Alpha Bureau","source_names":["Alpha Bureau"]}`,
		`{"series_id":"B","date":"2020-01-01","value":2.0,"value_raw":"2.0","citation_text":"Source: Beta Board via FRED","source_name":"Beta Board","source_names":["Beta Board"]}`,
		`{"series_id":"A","date":"2020-04-01","value":2.0,"value_raw":"2.0","citation_text":"Source: Alpha Bureau via FRED","source_name":"Alpha Bureau","source_names":["Alpha Bureau"]}`,
		`{"series_id":"B","date":"2020-04-01","value":3.0,"value_raw":"3.0","citation_text":"Source: Beta Board via FRED","source_name":"Beta Board","source_names":["Beta Board"]}`,
		`{"series_id":"A","date":"2020-07-01","value":3.0,"value_raw":"3.0","citation_text":"Source: Alpha Bureau via FRED","source_name":"Alpha Bureau","source_names":["Alpha Bureau"]}`,
		`{"series_id":"B","date":"2020-07-01","value":4.0,"value_raw":"4.0","citation_text":"Source: Beta Board via FRED","source_name":"Beta Board","source_names":["Beta Board"]}`,
	}, "\n") + "\n"

	tmp, err := os.CreateTemp(t.TempDir(), "analyze-compare-stdin-*.jsonl")
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
	origAgainst := analyzeCompareAgainst
	origSeries := analyzeCompareSeries
	os.Stdin = tmp
	globalFlags.Format = "json"
	analyzeCompareAgainst = "B"
	analyzeCompareSeries = "A"
	t.Cleanup(func() {
		os.Stdin = origStdin
		globalFlags.Format = origFormat
		analyzeCompareAgainst = origAgainst
		analyzeCompareSeries = origSeries
		_ = tmp.Close()
	})

	var buf bytes.Buffer
	analyzeCompareCmd.SetOut(&buf)
	analyzeCompareCmd.SetErr(&buf)
	if err := analyzeCompareCmd.RunE(analyzeCompareCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if !strings.Contains(buf.String(), `"series_id": "A"`) || !strings.Contains(buf.String(), `"against_series_id": "B"`) {
		t.Fatalf("unexpected compare output: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"citation_text": "Source: Alpha Bureau via FRED"`) {
		t.Fatalf("missing lhs citation in compare output: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"against_citation_text": "Source: Beta Board via FRED"`) {
		t.Fatalf("missing rhs citation in compare output: %s", buf.String())
	}
}

func TestAnalyzeCompareTableShowsSourcesBySeriesFooter(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"A","date":"2020-01-01","value":1.0,"value_raw":"1.0","citation_text":"Source: Alpha Bureau via FRED"}`,
		`{"series_id":"B","date":"2020-01-01","value":2.0,"value_raw":"2.0","citation_text":"Source: Beta Board via FRED"}`,
		`{"series_id":"A","date":"2020-04-01","value":2.0,"value_raw":"2.0","citation_text":"Source: Alpha Bureau via FRED"}`,
		`{"series_id":"B","date":"2020-04-01","value":3.0,"value_raw":"3.0","citation_text":"Source: Beta Board via FRED"}`,
		`{"series_id":"A","date":"2020-07-01","value":3.0,"value_raw":"3.0","citation_text":"Source: Alpha Bureau via FRED"}`,
		`{"series_id":"B","date":"2020-07-01","value":4.0,"value_raw":"4.0","citation_text":"Source: Beta Board via FRED"}`,
	}, "\n") + "\n"

	tmp, err := os.CreateTemp(t.TempDir(), "analyze-compare-table-stdin-*.jsonl")
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
	origAgainst := analyzeCompareAgainst
	origSeries := analyzeCompareSeries
	os.Stdin = tmp
	globalFlags.Format = "table"
	analyzeCompareAgainst = "B"
	analyzeCompareSeries = "A"
	t.Cleanup(func() {
		os.Stdin = origStdin
		globalFlags.Format = origFormat
		analyzeCompareAgainst = origAgainst
		analyzeCompareSeries = origSeries
		_ = tmp.Close()
	})

	var buf bytes.Buffer
	analyzeCompareCmd.SetOut(&buf)
	analyzeCompareCmd.SetErr(&buf)
	if err := analyzeCompareCmd.RunE(analyzeCompareCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Sources by series:") {
		t.Fatalf("expected sources footer header:\n%s", out)
	}
	if !strings.Contains(out, "- A: Alpha Bureau via FRED") {
		t.Fatalf("expected A source line:\n%s", out)
	}
	if !strings.Contains(out, "- B: Beta Board via FRED") {
		t.Fatalf("expected B source line:\n%s", out)
	}
}

func TestAnalyzeRegimeJSON(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"X","date":"2020-01-01","value":1.0,"value_raw":"1.0","citation_text":"Source: X Agency via FRED"}`,
		`{"series_id":"X","date":"2020-04-01","value":1.0,"value_raw":"1.0","citation_text":"Source: X Agency via FRED"}`,
		`{"series_id":"X","date":"2020-07-01","value":1.0,"value_raw":"1.0","citation_text":"Source: X Agency via FRED"}`,
		`{"series_id":"X","date":"2020-10-01","value":10.0,"value_raw":"10.0","citation_text":"Source: X Agency via FRED"}`,
		`{"series_id":"X","date":"2021-01-01","value":10.0,"value_raw":"10.0","citation_text":"Source: X Agency via FRED"}`,
		`{"series_id":"X","date":"2021-04-01","value":10.0,"value_raw":"10.0","citation_text":"Source: X Agency via FRED"}`,
	}, "\n") + "\n"

	tmp, err := os.CreateTemp(t.TempDir(), "analyze-regime-stdin-*.jsonl")
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
	origMethod := analyzeRegimeMethod
	origThreshold := analyzeRegimeThreshold
	os.Stdin = tmp
	globalFlags.Format = "json"
	analyzeRegimeMethod = "cusum"
	analyzeRegimeThreshold = 2.0
	t.Cleanup(func() {
		os.Stdin = origStdin
		globalFlags.Format = origFormat
		analyzeRegimeMethod = origMethod
		analyzeRegimeThreshold = origThreshold
		_ = tmp.Close()
	})

	var buf bytes.Buffer
	analyzeRegimeCmd.SetOut(&buf)
	analyzeRegimeCmd.SetErr(&buf)
	if err := analyzeRegimeCmd.RunE(analyzeRegimeCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if !strings.Contains(buf.String(), `"method": "cusum"`) {
		t.Fatalf("unexpected regime output: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"signal": "diff"`) {
		t.Fatalf("expected hardened regime signal metadata: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"citation_text": "Source: X Agency via FRED"`) {
		t.Fatalf("expected citation in regime JSON: %s", buf.String())
	}
}

func TestAnalyzeRegimeTableShowsCitationFooter(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"X","date":"2020-01-01","value":1.0,"value_raw":"1.0","citation_text":"Source: X Agency via FRED"}`,
		`{"series_id":"X","date":"2020-04-01","value":2.0,"value_raw":"2.0","citation_text":"Source: X Agency via FRED"}`,
		`{"series_id":"X","date":"2020-07-01","value":3.0,"value_raw":"3.0","citation_text":"Source: X Agency via FRED"}`,
		`{"series_id":"X","date":"2020-10-01","value":4.0,"value_raw":"4.0","citation_text":"Source: X Agency via FRED"}`,
		`{"series_id":"X","date":"2021-01-01","value":5.0,"value_raw":"5.0","citation_text":"Source: X Agency via FRED"}`,
	}, "\n") + "\n"

	tmp, err := os.CreateTemp(t.TempDir(), "analyze-regime-table-stdin-*.jsonl")
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
	origMethod := analyzeRegimeMethod
	origThreshold := analyzeRegimeThreshold
	os.Stdin = tmp
	globalFlags.Format = "table"
	analyzeRegimeMethod = "cusum"
	analyzeRegimeThreshold = 2.0
	t.Cleanup(func() {
		os.Stdin = origStdin
		globalFlags.Format = origFormat
		analyzeRegimeMethod = origMethod
		analyzeRegimeThreshold = origThreshold
		_ = tmp.Close()
	})

	var buf bytes.Buffer
	analyzeRegimeCmd.SetOut(&buf)
	analyzeRegimeCmd.SetErr(&buf)
	if err := analyzeRegimeCmd.RunE(analyzeRegimeCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Signal") || !strings.Contains(out, "Min Gap") {
		t.Fatalf("expected hardened regime fields in table: %s", out)
	}
	if !strings.Contains(out, "Source: X Agency via FRED") {
		t.Fatalf("expected citation footer in regime table: %s", out)
	}
}

func TestAnalyzeSummaryTablePrintsCitationFooter(t *testing.T) {
	input := strings.Join([]string{
		`{"series_id":"GDP","date":"2020-01-01","value":1.0,"value_raw":"1.0","citation_text":"Source: Bureau of Economic Analysis via FRED"}`,
		`{"series_id":"GDP","date":"2020-04-01","value":2.0,"value_raw":"2.0","citation_text":"Source: Bureau of Economic Analysis via FRED"}`,
	}, "\n") + "\n"
	out, err := runAnalyzeSummaryForTest(t, input, false, "table")
	if err != nil {
		t.Fatalf("runAnalyzeSummaryForTest: %v", err)
	}
	if !strings.Contains(out, "Source: Bureau of Economic Analysis via FRED") {
		t.Fatalf("expected citation footer in summary table:\n%s", out)
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
