package transform_test

import (
	"math"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/transform"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// makeObs builds a monthly observation slice from a start year/month and values.
// Go's time.Date normalises month overflow, so this handles year boundaries.
func makeObs(year, month int, values ...float64) []model.Observation {
	out := make([]model.Observation, len(values))
	for i, v := range values {
		out[i] = model.Observation{
			Date:  time.Date(year, time.Month(month+i), 1, 0, 0, 0, 0, time.UTC),
			Value: v,
		}
	}
	return out
}

// makeAnnual builds annual observations (Jan 1) from a start year.
func makeAnnual(startYear int, values ...float64) []model.Observation {
	out := make([]model.Observation, len(values))
	for i, v := range values {
		out[i] = model.Observation{
			Date:  time.Date(startYear+i, 1, 1, 0, 0, 0, 0, time.UTC),
			Value: v,
		}
	}
	return out
}

// date parses "YYYY-MM-DD" and panics on error — test use only.
func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic("date: " + err.Error())
	}
	return t
}

// isNaN is a test helper that returns true if v is NaN.
func isNaN(v float64) bool { return math.IsNaN(v) }

// approxEqual returns true if a and b are within tolerance.
func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// values extracts just the float values from a slice of observations.
func values(obs []model.Observation) []float64 {
	out := make([]float64, len(obs))
	for i, o := range obs {
		out[i] = o.Value
	}
	return out
}

// ─── PctChange ────────────────────────────────────────────────────────────────

func TestPctChangePeriod1(t *testing.T) {
	// 100 → 110 → 121: each is +10%
	obs := makeObs(2020, 1, 100.0, 110.0, 121.0)
	out, err := transform.PctChange(obs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(out))
	}
	if !approxEqual(out[0].Value, 10.0, 1e-9) {
		t.Errorf("out[0]: expected 10.0, got %g", out[0].Value)
	}
	if !approxEqual(out[1].Value, 10.0, 1e-9) {
		t.Errorf("out[1]: expected 10.0, got %g", out[1].Value)
	}
}

func TestPctChangePeriod12(t *testing.T) {
	// 12 monthly observations: index 12 vs index 0 should be (110-100)/100*100 = 10%
	vals := make([]float64, 13)
	for i := range vals {
		vals[i] = 100.0
	}
	vals[12] = 110.0
	obs := makeObs(2020, 1, vals...)
	out, err := transform.PctChange(obs, 12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 output, got %d", len(out))
	}
	if !approxEqual(out[0].Value, 10.0, 1e-9) {
		t.Errorf("expected 10.0, got %g", out[0].Value)
	}
}

func TestPctChangeNaNPropagates(t *testing.T) {
	obs := makeObs(2020, 1, 100.0, math.NaN(), 110.0)
	out, err := transform.PctChange(obs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// out[0]: NaN/100 → NaN; out[1]: 110/NaN → NaN
	if !isNaN(out[0].Value) {
		t.Errorf("out[0]: expected NaN, got %g", out[0].Value)
	}
	if !isNaN(out[1].Value) {
		t.Errorf("out[1]: expected NaN, got %g", out[1].Value)
	}
}

func TestPctChangeZeroDenominator(t *testing.T) {
	obs := makeObs(2020, 1, 0.0, 100.0)
	out, err := transform.PctChange(obs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNaN(out[0].Value) {
		t.Errorf("zero denominator should produce NaN, got %g", out[0].Value)
	}
}

func TestPctChangeInvalidPeriod(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0)
	_, err := transform.PctChange(obs, 0)
	if err == nil {
		t.Error("expected error for period=0")
	}
}

func TestPctChangeTooFewObs(t *testing.T) {
	obs := makeObs(2020, 1, 1.0)
	_, err := transform.PctChange(obs, 1)
	if err == nil {
		t.Error("expected error when len(obs) <= period")
	}
}

func TestPctChangeOutputLength(t *testing.T) {
	obs := makeObs(2020, 1, 1, 2, 3, 4, 5)
	out, err := transform.PctChange(obs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should drop the first `period` observations
	if len(out) != len(obs)-1 {
		t.Errorf("expected %d outputs, got %d", len(obs)-1, len(out))
	}
}

func TestPctChangeDatesPreserved(t *testing.T) {
	obs := makeObs(2020, 1, 100.0, 110.0, 121.0)
	out, _ := transform.PctChange(obs, 1)
	// Dates should align with the current (not prior) observation
	if !out[0].Date.Equal(obs[1].Date) {
		t.Errorf("date mismatch: expected %v, got %v", obs[1].Date, out[0].Date)
	}
}

// ─── Diff ─────────────────────────────────────────────────────────────────────

func TestDiffOrder1(t *testing.T) {
	obs := makeObs(2020, 1, 10.0, 12.0, 15.0, 13.0)
	out, err := transform.Diff(obs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []float64{2.0, 3.0, -2.0}
	if len(out) != len(expected) {
		t.Fatalf("expected %d outputs, got %d", len(expected), len(out))
	}
	for i, exp := range expected {
		if !approxEqual(out[i].Value, exp, 1e-9) {
			t.Errorf("out[%d]: expected %g, got %g", i, exp, out[i].Value)
		}
	}
}

func TestDiffOrder2(t *testing.T) {
	// Second difference of [1, 2, 4, 7]: first diff = [1,2,3], second diff = [1,1]
	obs := makeObs(2020, 1, 1.0, 2.0, 4.0, 7.0)
	out, err := transform.Diff(obs, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(out))
	}
	for i, v := range out {
		if !approxEqual(v.Value, 1.0, 1e-9) {
			t.Errorf("out[%d]: expected 1.0, got %g", i, v.Value)
		}
	}
}

func TestDiffNaNPropagates(t *testing.T) {
	obs := makeObs(2020, 1, 10.0, math.NaN(), 15.0)
	out, err := transform.Diff(obs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNaN(out[0].Value) {
		t.Errorf("out[0]: expected NaN, got %g", out[0].Value)
	}
	if !isNaN(out[1].Value) {
		t.Errorf("out[1]: expected NaN, got %g", out[1].Value)
	}
}

func TestDiffInvalidOrder(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0)
	_, err := transform.Diff(obs, 3)
	if err == nil {
		t.Error("expected error for order=3")
	}
	_, err = transform.Diff(obs, 0)
	if err == nil {
		t.Error("expected error for order=0")
	}
}

func TestDiffTooFewObs(t *testing.T) {
	obs := makeObs(2020, 1, 5.0)
	_, err := transform.Diff(obs, 1)
	if err == nil {
		t.Error("expected error for single observation")
	}
}

// ─── Log ──────────────────────────────────────────────────────────────────────

func TestLogPositiveValues(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, math.E, math.E*math.E)
	out, warnings := transform.Log(obs)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
	expected := []float64{0.0, 1.0, 2.0}
	for i, exp := range expected {
		if !approxEqual(out[i].Value, exp, 1e-9) {
			t.Errorf("out[%d]: expected %g, got %g", i, exp, out[i].Value)
		}
	}
}

func TestLogNonPositiveProducesNaNAndWarning(t *testing.T) {
	obs := makeObs(2020, 1, 10.0, 0.0, -5.0, 20.0)
	out, warnings := transform.Log(obs)
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings (zero and negative), got %d: %v", len(warnings), warnings)
	}
	if !approxEqual(out[0].Value, math.Log(10.0), 1e-9) {
		t.Errorf("out[0]: expected log(10), got %g", out[0].Value)
	}
	if !isNaN(out[1].Value) {
		t.Errorf("out[1]: expected NaN for log(0), got %g", out[1].Value)
	}
	if !isNaN(out[2].Value) {
		t.Errorf("out[2]: expected NaN for log(-5), got %g", out[2].Value)
	}
	if !approxEqual(out[3].Value, math.Log(20.0), 1e-9) {
		t.Errorf("out[3]: expected log(20), got %g", out[3].Value)
	}
}

func TestLogNaNPassthrough(t *testing.T) {
	obs := makeObs(2020, 1, 10.0, math.NaN(), 20.0)
	out, warnings := transform.Log(obs)
	if len(warnings) != 0 {
		t.Errorf("NaN input should not generate a warning, got: %v", warnings)
	}
	if !isNaN(out[1].Value) {
		t.Errorf("NaN input should produce NaN output, got %g", out[1].Value)
	}
}

func TestLogOutputLength(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0)
	out, _ := transform.Log(obs)
	if len(out) != len(obs) {
		t.Errorf("Log should preserve length: expected %d, got %d", len(obs), len(out))
	}
}

// ─── Index ────────────────────────────────────────────────────────────────────

func TestIndexBasic(t *testing.T) {
	// Value at 2020-01-01 is 200; index to base 100 → scale factor 0.5
	obs := makeObs(2020, 1, 200.0, 400.0, 100.0)
	anchor := date("2020-01-01")
	out, err := transform.Index(obs, 100.0, anchor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []float64{100.0, 200.0, 50.0}
	for i, exp := range expected {
		if !approxEqual(out[i].Value, exp, 1e-9) {
			t.Errorf("out[%d]: expected %g, got %g", i, exp, out[i].Value)
		}
	}
}

func TestIndexAnchorAtBase(t *testing.T) {
	// Anchor value equals base → no scaling
	obs := makeObs(2020, 1, 100.0, 150.0, 75.0)
	out, err := transform.Index(obs, 100.0, date("2020-01-01"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, o := range out {
		if !approxEqual(o.Value, obs[i].Value, 1e-9) {
			t.Errorf("out[%d]: expected %g, got %g", i, obs[i].Value, o.Value)
		}
	}
}

func TestIndexMissingAnchorDate(t *testing.T) {
	obs := makeObs(2020, 1, 100.0, 200.0)
	_, err := transform.Index(obs, 100.0, date("2019-01-01"))
	if err == nil {
		t.Error("expected error when anchor date not in series")
	}
}

func TestIndexZeroAnchorValue(t *testing.T) {
	obs := makeObs(2020, 1, 0.0, 100.0)
	_, err := transform.Index(obs, 100.0, date("2020-01-01"))
	if err == nil {
		t.Error("expected error when anchor value is zero")
	}
}

func TestIndexNaNAnchorValue(t *testing.T) {
	obs := makeObs(2020, 1, math.NaN(), 100.0)
	_, err := transform.Index(obs, 100.0, date("2020-01-01"))
	if err == nil {
		t.Error("expected error when anchor value is NaN")
	}
}

func TestIndexNaNPreserved(t *testing.T) {
	obs := makeObs(2020, 1, 100.0, math.NaN(), 200.0)
	out, err := transform.Index(obs, 100.0, date("2020-01-01"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNaN(out[1].Value) {
		t.Errorf("NaN should be preserved through Index, got %g", out[1].Value)
	}
}

// ─── Normalize ────────────────────────────────────────────────────────────────

func TestNormalizeZScore(t *testing.T) {
	// Values 1,2,3,4,5: mean=3, std=√2.5≈1.5811
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0, 5.0)
	out, err := transform.Normalize(obs, transform.NormalizeZScore)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Middle value (3) should be ~0
	if !approxEqual(out[2].Value, 0.0, 1e-9) {
		t.Errorf("z-score of mean should be 0, got %g", out[2].Value)
	}
	// First and last should be symmetric
	if !approxEqual(out[0].Value, -out[4].Value, 1e-9) {
		t.Errorf("z-scores should be symmetric: %g vs %g", out[0].Value, out[4].Value)
	}
}

func TestNormalizeMinMax(t *testing.T) {
	obs := makeObs(2020, 1, 0.0, 50.0, 100.0)
	out, err := transform.Normalize(obs, transform.NormalizeMinMax)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approxEqual(out[0].Value, 0.0, 1e-9) {
		t.Errorf("min should map to 0, got %g", out[0].Value)
	}
	if !approxEqual(out[2].Value, 1.0, 1e-9) {
		t.Errorf("max should map to 1, got %g", out[2].Value)
	}
	if !approxEqual(out[1].Value, 0.5, 1e-9) {
		t.Errorf("midpoint should map to 0.5, got %g", out[1].Value)
	}
}

func TestNormalizeZScoreFlatSeries(t *testing.T) {
	obs := makeObs(2020, 1, 5.0, 5.0, 5.0)
	_, err := transform.Normalize(obs, transform.NormalizeZScore)
	if err == nil {
		t.Error("expected error for flat series z-score (std=0)")
	}
}

func TestNormalizeMinMaxFlatSeries(t *testing.T) {
	obs := makeObs(2020, 1, 5.0, 5.0, 5.0)
	_, err := transform.Normalize(obs, transform.NormalizeMinMax)
	if err == nil {
		t.Error("expected error for flat series minmax (range=0)")
	}
}

func TestNormalizeAllNaN(t *testing.T) {
	obs := makeObs(2020, 1, math.NaN(), math.NaN())
	_, err := transform.Normalize(obs, transform.NormalizeZScore)
	if err == nil {
		t.Error("expected error for all-NaN input")
	}
}

func TestNormalizeNaNPreserved(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, math.NaN(), 3.0)
	out, err := transform.Normalize(obs, transform.NormalizeMinMax)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNaN(out[1].Value) {
		t.Errorf("NaN should be preserved, got %g", out[1].Value)
	}
}

func TestNormalizeUnknownMethod(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0)
	_, err := transform.Normalize(obs, "bogus")
	if err == nil {
		t.Error("expected error for unknown normalize method")
	}
}

// ─── Resample ─────────────────────────────────────────────────────────────────

func TestResampleMonthlyToAnnualMean(t *testing.T) {
	// 12 months of 2020, all value=1.0 except January=12.0 and December=0.0
	// Mean = (12 + 10*1 + 0) / 12 = 22/12 ≈ 1.833
	vals := make([]float64, 12)
	for i := range vals {
		vals[i] = 1.0
	}
	vals[0] = 12.0
	vals[11] = 0.0
	obs := makeObs(2020, 1, vals...)
	out, err := transform.Resample(obs, transform.ResampleAnnual, transform.ResampleMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 annual observation, got %d", len(out))
	}
	expected := (12.0 + 10.0 + 0.0) / 12.0
	if !approxEqual(out[0].Value, expected, 1e-9) {
		t.Errorf("annual mean: expected %g, got %g", expected, out[0].Value)
	}
}

func TestResampleMonthlyToAnnualLast(t *testing.T) {
	obs := makeObs(2020, 1, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12)
	out, err := transform.Resample(obs, transform.ResampleAnnual, transform.ResampleLast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 output, got %d", len(out))
	}
	if !approxEqual(out[0].Value, 12.0, 1e-9) {
		t.Errorf("annual last: expected 12.0, got %g", out[0].Value)
	}
}

func TestResampleMonthlyToAnnualSum(t *testing.T) {
	obs := makeObs(2020, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1) // 12 months of 1.0
	out, err := transform.Resample(obs, transform.ResampleAnnual, transform.ResampleSum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approxEqual(out[0].Value, 12.0, 1e-9) {
		t.Errorf("annual sum: expected 12.0, got %g", out[0].Value)
	}
}

func TestResampleMonthlyToQuarterly(t *testing.T) {
	// 12 months → 4 quarters; each quarter mean = mean of its 3 months
	obs := makeObs(2020, 1,
		1, 2, 3, // Q1 mean = 2
		4, 5, 6, // Q2 mean = 5
		7, 8, 9, // Q3 mean = 8
		10, 11, 12, // Q4 mean = 11
	)
	out, err := transform.Resample(obs, transform.ResampleQuarterly, transform.ResampleMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("expected 4 quarterly observations, got %d", len(out))
	}
	expected := []float64{2.0, 5.0, 8.0, 11.0}
	for i, exp := range expected {
		if !approxEqual(out[i].Value, exp, 1e-9) {
			t.Errorf("Q%d: expected %g, got %g", i+1, exp, out[i].Value)
		}
	}
}

func TestResampleMultiYear(t *testing.T) {
	// 24 months across 2 years → 2 annual observations
	vals := make([]float64, 24)
	for i := range vals {
		vals[i] = 1.0
	}
	obs := makeObs(2020, 1, vals...)
	out, err := transform.Resample(obs, transform.ResampleAnnual, transform.ResampleMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 annual observations, got %d", len(out))
	}
}

func TestResampleNaNSkipped(t *testing.T) {
	// 3 months in Q1: values 1, NaN, 3 → mean of [1,3] = 2
	obs := makeObs(2020, 1, 1.0, math.NaN(), 3.0)
	out, err := transform.Resample(obs, transform.ResampleQuarterly, transform.ResampleMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approxEqual(out[0].Value, 2.0, 1e-9) {
		t.Errorf("expected NaN-skipped mean=2.0, got %g", out[0].Value)
	}
}

func TestResampleEmptyInput(t *testing.T) {
	_, err := transform.Resample(nil, transform.ResampleAnnual, transform.ResampleMean)
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestResampleUnknownMethod(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0)
	_, err := transform.Resample(obs, transform.ResampleAnnual, "bogus")
	if err == nil {
		t.Error("expected error for unknown resample method")
	}
}

func TestResampleOutputDatesAreperiodStart(t *testing.T) {
	// Annual resample of any month → output date should be Jan 1
	obs := makeObs(2020, 6, 1.0, 2.0, 3.0) // June, July, August
	out, err := transform.Resample(obs, transform.ResampleAnnual, transform.ResampleMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out[0].Date.Month() != time.January {
		t.Errorf("annual resample output date should be January, got %v", out[0].Date.Month())
	}
	if out[0].Date.Day() != 1 {
		t.Errorf("annual resample output date should be day 1, got %d", out[0].Date.Day())
	}
}

// ─── Filter ───────────────────────────────────────────────────────────────────

func TestFilterAfter(t *testing.T) {
	obs := makeObs(2020, 1, 1, 2, 3, 4, 5) // Jan–May 2020
	out := transform.Filter(obs, transform.FilterOptions{
		After:    date("2020-03-01"), // exclusive: keep April, May
		MinValue: math.NaN(),
		MaxValue: math.NaN(),
	})
	if len(out) != 2 {
		t.Fatalf("expected 2 observations after 2020-03-01, got %d", len(out))
	}
	if out[0].Date.Month() != time.April {
		t.Errorf("first result should be April, got %v", out[0].Date.Month())
	}
}

func TestFilterBefore(t *testing.T) {
	obs := makeObs(2020, 1, 1, 2, 3, 4, 5) // Jan–May 2020
	out := transform.Filter(obs, transform.FilterOptions{
		Before:   date("2020-03-01"), // exclusive: keep Jan, Feb
		MinValue: math.NaN(),
		MaxValue: math.NaN(),
	})
	if len(out) != 2 {
		t.Fatalf("expected 2 observations before 2020-03-01, got %d", len(out))
	}
}

func TestFilterAfterAndBefore(t *testing.T) {
	obs := makeObs(2020, 1, 1, 2, 3, 4, 5)
	out := transform.Filter(obs, transform.FilterOptions{
		After:    date("2020-01-31"),
		Before:   date("2020-04-01"),
		MinValue: math.NaN(),
		MaxValue: math.NaN(),
	})
	// Feb and Mar only
	if len(out) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(out))
	}
}

func TestFilterMinValue(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 5.0, 3.0, 7.0, 2.0)
	out := transform.Filter(obs, transform.FilterOptions{
		MinValue: 3.0,
		MaxValue: math.NaN(),
	})
	for _, o := range out {
		if o.Value < 3.0 {
			t.Errorf("value %g should have been filtered (min=3.0)", o.Value)
		}
	}
	if len(out) != 3 { // 5, 3, 7
		t.Errorf("expected 3 observations >= 3.0, got %d", len(out))
	}
}

func TestFilterMaxValue(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 5.0, 3.0, 7.0, 2.0)
	out := transform.Filter(obs, transform.FilterOptions{
		MinValue: math.NaN(),
		MaxValue: 4.0,
	})
	for _, o := range out {
		if o.Value > 4.0 {
			t.Errorf("value %g should have been filtered (max=4.0)", o.Value)
		}
	}
}

func TestFilterDropMissing(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, math.NaN(), 3.0, math.NaN(), 5.0)
	out := transform.Filter(obs, transform.FilterOptions{
		DropMissing: true,
		MinValue:    math.NaN(),
		MaxValue:    math.NaN(),
	})
	if len(out) != 3 {
		t.Fatalf("expected 3 non-NaN observations, got %d", len(out))
	}
	for _, o := range out {
		if isNaN(o.Value) {
			t.Error("DropMissing should remove all NaN observations")
		}
	}
}

func TestFilterNaNKeptWhenDropMissingFalse(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, math.NaN(), 3.0)
	out := transform.Filter(obs, transform.FilterOptions{
		DropMissing: false,
		MinValue:    math.NaN(),
		MaxValue:    math.NaN(),
	})
	if len(out) != 3 {
		t.Fatalf("expected all 3 observations kept, got %d", len(out))
	}
}

func TestFilterNoOptions(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0)
	out := transform.Filter(obs, transform.FilterOptions{
		MinValue: math.NaN(),
		MaxValue: math.NaN(),
	})
	if len(out) != len(obs) {
		t.Errorf("empty filter should return all observations: expected %d, got %d", len(obs), len(out))
	}
}

func TestFilterAllExcluded(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0)
	out := transform.Filter(obs, transform.FilterOptions{
		MinValue: 10.0,
		MaxValue: math.NaN(),
	})
	if len(out) != 0 {
		t.Errorf("all values < 10 should be filtered, got %d observations", len(out))
	}
}

// ─── Roll ─────────────────────────────────────────────────────────────────────

func TestRollMean(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0, 5.0)
	out, err := transform.Roll(obs, 3, 1, transform.RollMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != len(obs) {
		t.Fatalf("Roll should preserve length: expected %d, got %d", len(obs), len(out))
	}
	// out[2] = mean(1,2,3) = 2
	if !approxEqual(out[2].Value, 2.0, 1e-9) {
		t.Errorf("out[2]: expected 2.0, got %g", out[2].Value)
	}
	// out[4] = mean(3,4,5) = 4
	if !approxEqual(out[4].Value, 4.0, 1e-9) {
		t.Errorf("out[4]: expected 4.0, got %g", out[4].Value)
	}
}

func TestRollMeanPartialWindow(t *testing.T) {
	// With minPeriods=1, early observations use whatever they have
	obs := makeObs(2020, 1, 2.0, 4.0, 6.0)
	out, err := transform.Roll(obs, 3, 1, transform.RollMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// out[0] = mean(2) = 2.0
	if !approxEqual(out[0].Value, 2.0, 1e-9) {
		t.Errorf("out[0]: expected 2.0, got %g", out[0].Value)
	}
	// out[1] = mean(2,4) = 3.0
	if !approxEqual(out[1].Value, 3.0, 1e-9) {
		t.Errorf("out[1]: expected 3.0, got %g", out[1].Value)
	}
}

func TestRollMinPeriods(t *testing.T) {
	// minPeriods=3, window=3: first two outputs should be NaN
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0)
	out, err := transform.Roll(obs, 3, 3, transform.RollMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNaN(out[0].Value) {
		t.Errorf("out[0]: expected NaN (window only has 1 value), got %g", out[0].Value)
	}
	if !isNaN(out[1].Value) {
		t.Errorf("out[1]: expected NaN (window only has 2 values), got %g", out[1].Value)
	}
	if isNaN(out[2].Value) {
		t.Errorf("out[2]: expected non-NaN (window has 3 values), got NaN")
	}
}

func TestRollStd(t *testing.T) {
	// std of [1,2,3] with ddof=1 = 1.0
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0, 5.0)
	out, err := transform.Roll(obs, 3, 3, transform.RollStd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First full window at index 2: std([1,2,3]) = 1.0
	if !approxEqual(out[2].Value, 1.0, 1e-9) {
		t.Errorf("out[2]: expected std=1.0, got %g", out[2].Value)
	}
}

func TestRollMin(t *testing.T) {
	obs := makeObs(2020, 1, 5.0, 3.0, 8.0, 1.0, 4.0)
	out, err := transform.Roll(obs, 3, 1, transform.RollMin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// out[2] = min(5,3,8) = 3
	if !approxEqual(out[2].Value, 3.0, 1e-9) {
		t.Errorf("out[2]: expected 3.0, got %g", out[2].Value)
	}
	// out[3] = min(3,8,1) = 1
	if !approxEqual(out[3].Value, 1.0, 1e-9) {
		t.Errorf("out[3]: expected 1.0, got %g", out[3].Value)
	}
}

func TestRollMax(t *testing.T) {
	obs := makeObs(2020, 1, 5.0, 3.0, 8.0, 1.0, 4.0)
	out, err := transform.Roll(obs, 3, 1, transform.RollMax)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// out[2] = max(5,3,8) = 8
	if !approxEqual(out[2].Value, 8.0, 1e-9) {
		t.Errorf("out[2]: expected 8.0, got %g", out[2].Value)
	}
}

func TestRollSum(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0)
	out, err := transform.Roll(obs, 3, 1, transform.RollSum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// out[2] = 1+2+3 = 6
	if !approxEqual(out[2].Value, 6.0, 1e-9) {
		t.Errorf("out[2]: expected 6.0, got %g", out[2].Value)
	}
	// out[3] = 2+3+4 = 9
	if !approxEqual(out[3].Value, 9.0, 1e-9) {
		t.Errorf("out[3]: expected 9.0, got %g", out[3].Value)
	}
}

func TestRollNaNSkippedInWindow(t *testing.T) {
	// NaN in window should be skipped; if enough non-NaN remain, compute normally
	obs := makeObs(2020, 1, 1.0, math.NaN(), 3.0, 4.0)
	out, err := transform.Roll(obs, 3, 2, transform.RollMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// out[2]: window=[1, NaN, 3], non-NaN=[1,3], mean=2.0 (minPeriods=2 satisfied)
	if !approxEqual(out[2].Value, 2.0, 1e-9) {
		t.Errorf("out[2]: expected 2.0 (NaN skipped), got %g", out[2].Value)
	}
}

func TestRollNaNWindowBelowMinPeriods(t *testing.T) {
	// All NaN in window → output NaN
	obs := makeObs(2020, 1, math.NaN(), math.NaN(), 3.0)
	out, err := transform.Roll(obs, 3, 2, transform.RollMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// out[1]: window=[NaN, NaN], 0 non-NaN < minPeriods=2 → NaN
	if !isNaN(out[1].Value) {
		t.Errorf("out[1]: expected NaN (too few non-NaN in window), got %g", out[1].Value)
	}
}

func TestRollInvalidWindow(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0)
	_, err := transform.Roll(obs, 0, 1, transform.RollMean)
	if err == nil {
		t.Error("expected error for window=0")
	}
}

func TestRollMinPeriodsExceedsWindow(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0)
	_, err := transform.Roll(obs, 2, 5, transform.RollMean)
	if err == nil {
		t.Error("expected error when minPeriods > window")
	}
}

func TestRollUnknownStat(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0)
	_, err := transform.Roll(obs, 2, 1, "bogus")
	if err == nil {
		t.Error("expected error for unknown roll stat")
	}
}

func TestRollPreservesLength(t *testing.T) {
	obs := makeObs(2020, 1, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	out, err := transform.Roll(obs, 4, 1, transform.RollMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != len(obs) {
		t.Errorf("Roll should preserve input length: expected %d, got %d", len(obs), len(out))
	}
}

func TestRollPreservesDates(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0)
	out, _ := transform.Roll(obs, 2, 1, transform.RollMean)
	for i := range obs {
		if !out[i].Date.Equal(obs[i].Date) {
			t.Errorf("out[%d]: date mismatch: expected %v, got %v", i, obs[i].Date, out[i].Date)
		}
	}
}

// ─── Composition ──────────────────────────────────────────────────────────────

func TestPctChangeThenRoll(t *testing.T) {
	// Realistic pipeline: monthly data → pct-change → 3-month rolling mean
	obs := makeObs(2020, 1, 100, 102, 101, 104, 103, 106, 105)
	pct, err := transform.PctChange(obs, 1)
	if err != nil {
		t.Fatalf("PctChange: %v", err)
	}
	rolled, err := transform.Roll(pct, 3, 1, transform.RollMean)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	// Should have same length as pct (no length change from Roll)
	if len(rolled) != len(pct) {
		t.Errorf("composition length mismatch: %d vs %d", len(rolled), len(pct))
	}
	// All rolled values should be finite (no NaN bleed from small pct changes)
	for i, o := range rolled {
		if isNaN(o.Value) {
			t.Errorf("rolled[%d]: unexpected NaN", i)
		}
	}
}

func TestResampleThenDiff(t *testing.T) {
	// Annual resample then first difference should yield n-1 observations
	obs := makeObs(2020, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 2020: mean=1
		2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, // 2021: mean=2
		3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, // 2022: mean=3
	)
	annual, err := transform.Resample(obs, transform.ResampleAnnual, transform.ResampleMean)
	if err != nil {
		t.Fatalf("Resample: %v", err)
	}
	diff, err := transform.Diff(annual, 1)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diff) != 2 {
		t.Fatalf("expected 2 diffs (3 annual - 1), got %d", len(diff))
	}
	// Each year increments by 1 → diff should all be 1.0
	for i, o := range diff {
		if !approxEqual(o.Value, 1.0, 1e-9) {
			t.Errorf("diff[%d]: expected 1.0, got %g", i, o.Value)
		}
	}
}
