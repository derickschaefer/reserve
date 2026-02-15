package chart_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/chart"
	"github.com/derickschaefer/reserve/internal/model"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// obs builds a slice of Observations from alternating (date string, value) pairs.
// date format: "2006-01-02". Panics on bad date strings.
func obs(pairs ...interface{}) []model.Observation {
	var out []model.Observation
	for i := 0; i < len(pairs)-1; i += 2 {
		dateStr := pairs[i].(string)
		val := pairs[i+1].(float64)
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			panic("obs: bad date: " + dateStr)
		}
		out = append(out, model.Observation{
			Date:  t,
			Value: val,
		})
	}
	return out
}

// annualObs builds annual observations (YYYY-01-01) for consecutive years
// starting at startYear, using the provided values.
func annualObs(startYear int, values ...float64) []model.Observation {
	out := make([]model.Observation, len(values))
	for i, v := range values {
		out[i] = model.Observation{
			Date:     time.Date(startYear+i, 1, 1, 0, 0, 0, 0, time.UTC),
			Value:    v,
			ValueRaw: ".",
		}
	}
	return out
}

// monthlyObs builds monthly observations (YYYY-MM-01) starting at the given
// year and month, using the provided values.
func monthlyObs(year, month int, values ...float64) []model.Observation {
	out := make([]model.Observation, len(values))
	for i, v := range values {
		// Use time.Date with month > 12 — Go normalises automatically
		out[i] = model.Observation{
			Date:     time.Date(year, time.Month(month+i), 1, 0, 0, 0, 0, time.UTC),
			Value:    v,
			ValueRaw: ".",
		}
	}
	return out
}

// ─── Bar tests ────────────────────────────────────────────────────────────────

func TestBarBasic(t *testing.T) {
	observations := annualObs(2020, 3.5, 5.4, 3.7, 4.0)
	var buf strings.Builder
	err := chart.Bar(&buf, "UNRATE", observations, chart.BarOptions{Width: 60})
	if err != nil {
		t.Fatalf("Bar returned error: %v", err)
	}
	out := buf.String()

	// Header line present
	if !strings.Contains(out, "UNRATE") {
		t.Error("output missing series ID")
	}
	if !strings.Contains(out, "2020") {
		t.Error("output missing start year")
	}
	if !strings.Contains(out, "2023") {
		t.Error("output missing end year")
	}

	// One bar line per observation
	lines := nonEmptyLines(out)
	// First line is header, remaining are bars
	if len(lines) != 5 { // 1 header + 4 bars
		t.Errorf("expected 5 lines (1 header + 4 bars), got %d:\n%s", len(lines), out)
	}

	// Each bar line contains block characters
	for _, line := range lines[1:] {
		if !strings.Contains(line, "█") {
			t.Errorf("bar line missing block character: %q", line)
		}
	}
}

func TestBarAllNaN(t *testing.T) {
	observations := annualObs(2020, math.NaN(), math.NaN(), math.NaN())
	var buf strings.Builder
	err := chart.Bar(&buf, "TEST", observations, chart.BarOptions{Width: 60})
	if err == nil {
		t.Fatal("expected error for all-NaN input, got nil")
	}
	if !strings.Contains(err.Error(), "no non-NaN") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBarSingleObservation(t *testing.T) {
	observations := annualObs(2020, 5.0)
	var buf strings.Builder
	err := chart.Bar(&buf, "TEST", observations, chart.BarOptions{Width: 60})
	if err != nil {
		t.Fatalf("Bar with single observation returned error: %v", err)
	}
	// Should render without panic — flat series handled via valRange=1 guard
}

func TestBarNaNFiltered(t *testing.T) {
	// NaN observations should be silently skipped
	observations := []model.Observation{
		{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Value: 3.5},
		{Date: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), Value: math.NaN()},
		{Date: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), Value: 4.2},
	}
	var buf strings.Builder
	err := chart.Bar(&buf, "TEST", observations, chart.BarOptions{Width: 60})
	if err != nil {
		t.Fatalf("Bar returned error: %v", err)
	}
	lines := nonEmptyLines(buf.String())
	// 1 header + 2 valid bars (NaN skipped)
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (1 header + 2 bars), got %d:\n%s", len(lines), buf.String())
	}
}

func TestBarMaxBars(t *testing.T) {
	// 10 annual observations, cap at 5 — should take last 5
	observations := annualObs(2010, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	var buf strings.Builder
	err := chart.Bar(&buf, "TEST", observations, chart.BarOptions{
		Width:   60,
		MaxBars: 5,
	})
	if err != nil {
		t.Fatalf("Bar returned error: %v", err)
	}
	lines := nonEmptyLines(buf.String())
	// 1 header + 5 bars
	if len(lines) != 6 {
		t.Errorf("expected 6 lines (1 header + 5 bars), got %d:\n%s", len(lines), buf.String())
	}
	// Last bar should be 2019 (year 10), not 2010
	if !strings.Contains(buf.String(), "2019") {
		t.Error("expected last bar to be 2019 (last 5 of 2010–2019)")
	}
	if strings.Contains(buf.String(), "2010") {
		t.Error("expected 2010 to be excluded by MaxBars=5")
	}
}

func TestBarNegativeValues(t *testing.T) {
	// GDP growth can go negative — bidirectional bar
	observations := annualObs(2018, 2.9, 2.3, -3.4, 5.7, 2.1)
	var buf strings.Builder
	err := chart.Bar(&buf, "GDP", observations, chart.BarOptions{Width: 80})
	if err != nil {
		t.Fatalf("Bar returned error: %v", err)
	}
	out := buf.String()

	// Should contain zero-line marker
	if !strings.Contains(out, "│") {
		t.Error("bidirectional bar missing zero-line │ character")
	}

	// The negative year (2020) should still have block characters
	if !strings.Contains(out, "█") {
		t.Error("negative bar missing block characters")
	}
}

func TestBarFlatSeries(t *testing.T) {
	// All same value — valRange=0 guard must not panic or divide by zero
	observations := annualObs(2020, 5.0, 5.0, 5.0, 5.0)
	var buf strings.Builder
	err := chart.Bar(&buf, "TEST", observations, chart.BarOptions{Width: 60})
	if err != nil {
		t.Fatalf("Bar with flat series returned error: %v", err)
	}
}

func TestBarDensityWarning(t *testing.T) {
	// More than 60 observations should trigger a density warning
	values := make([]float64, 65)
	for i := range values {
		values[i] = float64(i) + 1.0
	}
	observations := monthlyObs(2019, 1, values...)
	var buf strings.Builder
	err := chart.Bar(&buf, "TEST", observations, chart.BarOptions{Width: 80})
	if err != nil {
		t.Fatalf("Bar returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "⚠") {
		t.Error("expected density warning for 65-observation series")
	}
}

func TestBarDateFormatAnnual(t *testing.T) {
	// Annual series should use YYYY date format
	observations := annualObs(2020, 3.5, 4.0, 4.5)
	var buf strings.Builder
	_ = chart.Bar(&buf, "TEST", observations, chart.BarOptions{Width: 60})
	out := buf.String()

	// Annual format: just year, no month
	if strings.Contains(out, "2020-01") {
		t.Error("annual series should use YYYY format, not YYYY-MM")
	}
	if !strings.Contains(out, "2020") {
		t.Error("annual series missing year label")
	}
}

func TestBarDateFormatMonthly(t *testing.T) {
	// Monthly series should use YYYY-MM date format
	observations := monthlyObs(2024, 1, 3.5, 3.7, 3.8, 3.9)
	var buf strings.Builder
	_ = chart.Bar(&buf, "TEST", observations, chart.BarOptions{Width: 60})
	out := buf.String()

	if !strings.Contains(out, "2024-01") {
		t.Error("monthly series should use YYYY-MM format")
	}
}

// ─── Plot tests ───────────────────────────────────────────────────────────────

func TestPlotBasic(t *testing.T) {
	observations := monthlyObs(2020, 1,
		3.5, 4.4, 14.7, 13.3, 11.1, 8.4, 6.9, 6.0, 6.9, 6.7, 6.4, 6.7,
	)
	var buf strings.Builder
	err := chart.Plot(&buf, "UNRATE", observations, chart.PlotOptions{
		Width:  80,
		Height: 8,
	})
	if err != nil {
		t.Fatalf("Plot returned error: %v", err)
	}
	out := buf.String()

	// Header present
	if !strings.Contains(out, "UNRATE") {
		t.Error("output missing series ID")
	}
	if !strings.Contains(out, "2020-01") {
		t.Error("output missing start date")
	}

	// Bottom axis present
	if !strings.Contains(out, "└") {
		t.Error("output missing bottom-left corner └")
	}
	if !strings.Contains(out, "─") {
		t.Error("output missing horizontal axis ─")
	}
}

func TestPlotLineCount(t *testing.T) {
	observations := monthlyObs(2020, 1, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0)
	height := 8
	var buf strings.Builder
	err := chart.Plot(&buf, "TEST", observations, chart.PlotOptions{
		Width:  80,
		Height: height,
	})
	if err != nil {
		t.Fatalf("Plot returned error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	// header + height data rows + bottom axis + x labels = height + 3
	expected := height + 3
	if len(lines) != expected {
		t.Errorf("expected %d lines, got %d:\n%s", expected, len(lines), buf.String())
	}
}

func TestPlotTitleOverride(t *testing.T) {
	observations := monthlyObs(2020, 1, 1.0, 2.0, 3.0)
	var buf strings.Builder
	_ = chart.Plot(&buf, "UNRATE", observations, chart.PlotOptions{
		Width:  60,
		Height: 6,
		Title:  "Custom Title",
	})
	out := buf.String()
	if !strings.Contains(out, "Custom Title") {
		t.Error("custom title not present in output")
	}
	if strings.Contains(strings.Split(out, "\n")[0], "UNRATE") {
		t.Error("series ID should be replaced by custom title on header line")
	}
}

func TestPlotAllNaN(t *testing.T) {
	observations := monthlyObs(2020, 1, math.NaN(), math.NaN(), math.NaN())
	var buf strings.Builder
	err := chart.Plot(&buf, "TEST", observations, chart.PlotOptions{Width: 80, Height: 8})
	if err == nil {
		t.Fatal("expected error for all-NaN input, got nil")
	}
	if !strings.Contains(err.Error(), "non-NaN") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestPlotSingleObservation(t *testing.T) {
	observations := monthlyObs(2020, 1, 5.0)
	var buf strings.Builder
	err := chart.Plot(&buf, "TEST", observations, chart.PlotOptions{Width: 80, Height: 8})
	if err == nil {
		t.Fatal("expected error for single observation, got nil")
	}
}

func TestPlotNaNGaps(t *testing.T) {
	// NaN in the middle should not crash and should render as a gap (space)
	observations := []model.Observation{
		{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Value: 3.5},
		{Date: time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC), Value: math.NaN()},
		{Date: time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC), Value: math.NaN()},
		{Date: time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC), Value: 4.1},
		{Date: time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC), Value: 4.5},
	}
	var buf strings.Builder
	err := chart.Plot(&buf, "TEST", observations, chart.PlotOptions{Width: 60, Height: 6})
	if err != nil {
		t.Fatalf("Plot with NaN gaps returned error: %v", err)
	}
}

func TestPlotFlatSeries(t *testing.T) {
	// All same value — rowForValue degenerate case
	observations := monthlyObs(2020, 1, 5.0, 5.0, 5.0, 5.0, 5.0)
	var buf strings.Builder
	err := chart.Plot(&buf, "TEST", observations, chart.PlotOptions{Width: 60, Height: 6})
	if err != nil {
		t.Fatalf("Plot with flat series returned error: %v", err)
	}
}

func TestPlotWidthRespected(t *testing.T) {
	observations := monthlyObs(2020, 1, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0)
	width := 60
	var buf strings.Builder
	_ = chart.Plot(&buf, "TEST", observations, chart.PlotOptions{
		Width:  width,
		Height: 6,
	})
	for i, line := range strings.Split(buf.String(), "\n") {
		// Use rune count — box-drawing chars are multi-byte in UTF-8
		runeLen := len([]rune(line))
		if runeLen > width+2 { // small tolerance for axis label overhang
			t.Errorf("line %d exceeds width %d: runes=%d %q", i, width, runeLen, line)
		}
	}
}

func TestPlotXAxisLabels(t *testing.T) {
	// Start and end dates should appear somewhere in the output
	observations := monthlyObs(2015, 1,
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
	)
	var buf strings.Builder
	_ = chart.Plot(&buf, "TEST", observations, chart.PlotOptions{Width: 80, Height: 8})
	out := buf.String()

	if !strings.Contains(out, "2015") {
		t.Error("x-axis missing start year 2015")
	}
	if !strings.Contains(out, "2016") {
		t.Error("x-axis missing end year 2016")
	}
}

// ─── Utilities ────────────────────────────────────────────────────────────────

// nonEmptyLines returns lines with at least one non-space character.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
