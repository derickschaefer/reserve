package pipeline_test

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/pipeline"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func isNaN(v float64) bool { return math.IsNaN(v) }

// jsonl joins lines with newlines and appends a trailing newline.
func jsonl(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// mkobs builds a single model.Observation for write tests.
func mkobs(year, month, day int, value float64, raw string) model.Observation {
	return model.Observation{
		Date:     time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC),
		Value:    value,
		ValueRaw: raw,
	}
}

// ─── ReadObservations ─────────────────────────────────────────────────────────

func TestReadBasicFloat(t *testing.T) {
	input := jsonl(
		`{"series_id":"UNRATE","date":"2020-01-01","value":3.5,"value_raw":"3.5"}`,
		`{"series_id":"UNRATE","date":"2020-02-01","value":3.6,"value_raw":"3.6"}`,
	)
	sid, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sid != "UNRATE" {
		t.Errorf("series_id: expected UNRATE, got %q", sid)
	}
	if len(observations) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(observations))
	}
	if observations[0].Value != 3.5 {
		t.Errorf("obs[0].Value: expected 3.5, got %g", observations[0].Value)
	}
	if observations[1].Value != 3.6 {
		t.Errorf("obs[1].Value: expected 3.6, got %g", observations[1].Value)
	}
}

func TestReadNullValueBecomesNaN(t *testing.T) {
	input := jsonl(
		`{"series_id":"TEST","date":"2020-01-01","value":1.0,"value_raw":"1.0"}`,
		`{"series_id":"TEST","date":"2020-02-01","value":null,"value_raw":"."}`,
		`{"series_id":"TEST","date":"2020-03-01","value":3.0,"value_raw":"3.0"}`,
	)
	_, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNaN(observations[1].Value) {
		t.Errorf("null value should become NaN, got %g", observations[1].Value)
	}
	if observations[0].Value != 1.0 {
		t.Errorf("obs[0]: expected 1.0, got %g", observations[0].Value)
	}
	if observations[2].Value != 3.0 {
		t.Errorf("obs[2]: expected 3.0, got %g", observations[2].Value)
	}
}

func TestReadDotStringValueBecomesNaN(t *testing.T) {
	// FRED emits "." as the missing-value sentinel in some formats
	input := jsonl(
		`{"series_id":"TEST","date":"2020-01-01","value":".","value_raw":"."}`,
	)
	_, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNaN(observations[0].Value) {
		t.Errorf(`"." string value should become NaN, got %g`, observations[0].Value)
	}
}

func TestReadEmptyStringValueBecomesNaN(t *testing.T) {
	input := jsonl(
		`{"series_id":"TEST","date":"2020-01-01","value":"","value_raw":"."}`,
	)
	_, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNaN(observations[0].Value) {
		t.Errorf(`empty string value should become NaN, got %g`, observations[0].Value)
	}
}

func TestReadSeriesIDFromFirstRecord(t *testing.T) {
	input := jsonl(
		`{"series_id":"GDP","date":"2020-01-01","value":21000.0}`,
		`{"series_id":"GDP","date":"2020-04-01","value":19500.0}`,
	)
	sid, _, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sid != "GDP" {
		t.Errorf("expected series_id GDP, got %q", sid)
	}
}

func TestReadSeriesIDEmptyWhenAbsent(t *testing.T) {
	// Records without series_id field → seriesID returned as empty string
	input := jsonl(
		`{"date":"2020-01-01","value":1.0}`,
		`{"date":"2020-02-01","value":2.0}`,
	)
	sid, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sid != "" {
		t.Errorf("expected empty series_id, got %q", sid)
	}
	if len(observations) != 2 {
		t.Errorf("expected 2 observations, got %d", len(observations))
	}
}

func TestReadDateParsed(t *testing.T) {
	input := jsonl(
		`{"series_id":"TEST","date":"2024-06-15","value":5.0}`,
	)
	_, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	if !observations[0].Date.Equal(expected) {
		t.Errorf("date: expected %v, got %v", expected, observations[0].Date)
	}
}

func TestReadValueRawPreserved(t *testing.T) {
	input := jsonl(
		`{"series_id":"TEST","date":"2020-01-01","value":3.14159,"value_raw":"3.14159"}`,
	)
	_, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if observations[0].ValueRaw != "3.14159" {
		t.Errorf("value_raw: expected 3.14159, got %q", observations[0].ValueRaw)
	}
}

func TestReadValueRawDefaultsForNull(t *testing.T) {
	input := jsonl(
		`{"series_id":"TEST","date":"2020-01-01","value":null}`,
	)
	_, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if observations[0].ValueRaw != "." {
		t.Errorf(`value_raw for null: expected ".", got %q`, observations[0].ValueRaw)
	}
}

func TestReadSkipsBlankLines(t *testing.T) {
	input := `{"series_id":"TEST","date":"2020-01-01","value":1.0}` + "\n" +
		"\n" +
		"   \n" +
		`{"series_id":"TEST","date":"2020-02-01","value":2.0}` + "\n"
	_, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(observations) != 2 {
		t.Errorf("blank lines should be skipped: expected 2 obs, got %d", len(observations))
	}
}

func TestReadSkipsCommentLines(t *testing.T) {
	input := `// this is a comment` + "\n" +
		`{"series_id":"TEST","date":"2020-01-01","value":1.0}` + "\n"
	_, observations, err := pipeline.ReadObservations(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(observations) != 1 {
		t.Errorf("comment lines should be skipped: expected 1 obs, got %d", len(observations))
	}
}

func TestReadEmptyInputError(t *testing.T) {
	_, _, err := pipeline.ReadObservations(strings.NewReader(""))
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestReadBlankOnlyInputError(t *testing.T) {
	_, _, err := pipeline.ReadObservations(strings.NewReader("\n\n\n"))
	if err == nil {
		t.Error("expected error for blank-only input")
	}
}

func TestReadInvalidJSONError(t *testing.T) {
	_, _, err := pipeline.ReadObservations(strings.NewReader("not json at all\n"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error should mention invalid JSON, got: %v", err)
	}
}

func TestReadInvalidDateError(t *testing.T) {
	input := jsonl(`{"series_id":"TEST","date":"not-a-date","value":1.0}`)
	_, _, err := pipeline.ReadObservations(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for invalid date")
	}
	if !strings.Contains(err.Error(), "invalid date") {
		t.Errorf("error should mention invalid date, got: %v", err)
	}
}

func TestReadUnexpectedStringValueError(t *testing.T) {
	// A non-empty, non-"." string is not a valid value
	input := jsonl(`{"series_id":"TEST","date":"2020-01-01","value":"notanumber"}`)
	_, _, err := pipeline.ReadObservations(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for unexpected string value")
	}
}

func TestReadLargeInput(t *testing.T) {
	// 1000 records — verifies scanner buffer handles volume without truncation
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString(`{"series_id":"TEST","date":"2020-01-01","value":1.0}` + "\n")
	}
	_, observations, err := pipeline.ReadObservations(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(observations) != 1000 {
		t.Errorf("expected 1000 observations, got %d", len(observations))
	}
}

// ─── WriteJSONL ───────────────────────────────────────────────────────────────

func TestWriteBasicFloat(t *testing.T) {
	observations := []model.Observation{
		mkobs(2020, 1, 1, 3.5, "3.5"),
		mkobs(2020, 2, 1, 4.2, "4.2"),
	}
	var buf bytes.Buffer
	if err := pipeline.WriteJSONL(&buf, "UNRATE", observations); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"series_id":"UNRATE"`) {
		t.Error("output missing series_id")
	}
	if !strings.Contains(out, `"date":"2020-01-01"`) {
		t.Error("output missing date")
	}
	if !strings.Contains(out, `"value":3.5`) {
		t.Error("output missing float value")
	}
}

func TestWriteNaNAsNull(t *testing.T) {
	observations := []model.Observation{
		mkobs(2020, 1, 1, math.NaN(), "."),
	}
	var buf bytes.Buffer
	if err := pipeline.WriteJSONL(&buf, "TEST", observations); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `"value":null`) {
		t.Errorf("NaN should be written as null, got: %s", buf.String())
	}
}

func TestWriteDateFormat(t *testing.T) {
	observations := []model.Observation{
		mkobs(2024, 6, 15, 1.0, "1"),
	}
	var buf bytes.Buffer
	_ = pipeline.WriteJSONL(&buf, "TEST", observations)
	if !strings.Contains(buf.String(), `"date":"2024-06-15"`) {
		t.Errorf("date should be YYYY-MM-DD, got: %s", buf.String())
	}
}

func TestWriteValueRawPreserved(t *testing.T) {
	observations := []model.Observation{
		mkobs(2020, 1, 1, 3.14159, "3.14159"),
	}
	var buf bytes.Buffer
	_ = pipeline.WriteJSONL(&buf, "TEST", observations)
	if !strings.Contains(buf.String(), `"value_raw":"3.14159"`) {
		t.Errorf("value_raw should be preserved, got: %s", buf.String())
	}
}

func TestWriteOneLinePerObservation(t *testing.T) {
	observations := []model.Observation{
		mkobs(2020, 1, 1, 1.0, "1"),
		mkobs(2020, 2, 1, 2.0, "2"),
		mkobs(2020, 3, 1, 3.0, "3"),
	}
	var buf bytes.Buffer
	_ = pipeline.WriteJSONL(&buf, "TEST", observations)
	lines := nonEmptyLines(buf.String())
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (one per obs), got %d:\n%s", len(lines), buf.String())
	}
}

func TestWriteEmptySlice(t *testing.T) {
	var buf bytes.Buffer
	if err := pipeline.WriteJSONL(&buf, "TEST", nil); err != nil {
		t.Fatalf("WriteJSONL with nil slice should not error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("nil slice should produce no output, got: %q", buf.String())
	}
}

// ─── Round-trip ───────────────────────────────────────────────────────────────

func TestRoundTrip(t *testing.T) {
	// Write observations then read them back — everything should survive intact
	original := []model.Observation{
		mkobs(2020, 1, 1, 3.5, "3.5"),
		mkobs(2020, 2, 1, math.NaN(), "."),
		mkobs(2020, 3, 1, 4.2, "4.2"),
	}

	var buf bytes.Buffer
	if err := pipeline.WriteJSONL(&buf, "ROUNDTRIP", original); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}

	sid, result, err := pipeline.ReadObservations(&buf)
	if err != nil {
		t.Fatalf("ReadObservations: %v", err)
	}

	if sid != "ROUNDTRIP" {
		t.Errorf("series_id: expected ROUNDTRIP, got %q", sid)
	}
	if len(result) != len(original) {
		t.Fatalf("length mismatch: expected %d, got %d", len(original), len(result))
	}
	for i, orig := range original {
		if !orig.Date.Equal(result[i].Date) {
			t.Errorf("obs[%d].Date: expected %v, got %v", i, orig.Date, result[i].Date)
		}
		if isNaN(orig.Value) {
			if !isNaN(result[i].Value) {
				t.Errorf("obs[%d].Value: expected NaN, got %g", i, result[i].Value)
			}
		} else if result[i].Value != orig.Value {
			t.Errorf("obs[%d].Value: expected %g, got %g", i, orig.Value, result[i].Value)
		}
	}
}

func TestRoundTripManyObservations(t *testing.T) {
	// 500 observations with every 7th as NaN
	original := make([]model.Observation, 500)
	for i := range original {
		d := time.Date(2000, time.Month(1+i%12), 1, 0, 0, 0, 0, time.UTC)
		if i%7 == 0 {
			original[i] = model.Observation{Date: d, Value: math.NaN(), ValueRaw: "."}
		} else {
			original[i] = model.Observation{Date: d, Value: float64(i), ValueRaw: "x"}
		}
	}

	var buf bytes.Buffer
	if err := pipeline.WriteJSONL(&buf, "BIG", original); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	_, result, err := pipeline.ReadObservations(&buf)
	if err != nil {
		t.Fatalf("ReadObservations: %v", err)
	}
	if len(result) != 500 {
		t.Errorf("expected 500 obs, got %d", len(result))
	}
	for i, orig := range original {
		if isNaN(orig.Value) != isNaN(result[i].Value) {
			t.Errorf("obs[%d]: NaN mismatch (expected NaN=%v)", i, isNaN(orig.Value))
		}
	}
}

func TestRoundTripSeriesIDPreserved(t *testing.T) {
	observations := []model.Observation{mkobs(2020, 1, 1, 1.0, "1")}
	var buf bytes.Buffer
	_ = pipeline.WriteJSONL(&buf, "FEDFUNDS", observations)
	sid, _, err := pipeline.ReadObservations(&buf)
	if err != nil {
		t.Fatalf("ReadObservations: %v", err)
	}
	if sid != "FEDFUNDS" {
		t.Errorf("series_id not preserved: expected FEDFUNDS, got %q", sid)
	}
}
