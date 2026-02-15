package analyze_test

import (
	"math"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/analyze"
	"github.com/derickschaefer/reserve/internal/model"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// makeObs builds monthly observations starting at year/month.
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

// makeAnnual builds annual observations (Jan 1) starting at startYear.
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

func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func isNaN(v float64) bool { return math.IsNaN(v) }

// ─── Summarize ────────────────────────────────────────────────────────────────

func TestSummarizeBasicCounts(t *testing.T) {
	obs := makeObs(2020, 1, 1.0, 2.0, math.NaN(), 4.0, 5.0)
	s := analyze.Summarize("TEST", obs)

	if s.SeriesID != "TEST" {
		t.Errorf("SeriesID: expected TEST, got %q", s.SeriesID)
	}
	if s.Count != 5 {
		t.Errorf("Count: expected 5, got %d", s.Count)
	}
	if s.Missing != 1 {
		t.Errorf("Missing: expected 1, got %d", s.Missing)
	}
	if !approxEqual(s.MissingPct, 20.0, 1e-9) {
		t.Errorf("MissingPct: expected 20.0, got %g", s.MissingPct)
	}
}

func TestSummarizeMeanAndStd(t *testing.T) {
	// Values 1,2,3,4,5: mean=3, population-style std via sample formula
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0, 5.0)
	s := analyze.Summarize("TEST", obs)

	if !approxEqual(s.Mean, 3.0, 1e-9) {
		t.Errorf("Mean: expected 3.0, got %g", s.Mean)
	}
	// Sample std of [1,2,3,4,5] = sqrt(2.5) ≈ 1.5811
	expectedStd := math.Sqrt(2.5)
	if !approxEqual(s.Std, expectedStd, 1e-6) {
		t.Errorf("Std: expected %g, got %g", expectedStd, s.Std)
	}
}

func TestSummarizeMinMax(t *testing.T) {
	obs := makeObs(2020, 1, 5.0, 2.0, 8.0, 1.0, 9.0, 3.0)
	s := analyze.Summarize("TEST", obs)

	if !approxEqual(s.Min, 1.0, 1e-9) {
		t.Errorf("Min: expected 1.0, got %g", s.Min)
	}
	if !approxEqual(s.Max, 9.0, 1e-9) {
		t.Errorf("Max: expected 9.0, got %g", s.Max)
	}
}

func TestSummarizeMedian(t *testing.T) {
	// Odd count: median of [1,2,3,4,5] = 3
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0, 5.0)
	s := analyze.Summarize("TEST", obs)
	if !approxEqual(s.Median, 3.0, 1e-9) {
		t.Errorf("Median: expected 3.0, got %g", s.Median)
	}
}

func TestSummarizeMedianEvenCount(t *testing.T) {
	// Even count: median of [1,2,3,4] = 2.5
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0)
	s := analyze.Summarize("TEST", obs)
	if !approxEqual(s.Median, 2.5, 1e-6) {
		t.Errorf("Median: expected 2.5, got %g", s.Median)
	}
}

func TestSummarizePercentiles(t *testing.T) {
	// [1,2,3,4,5]: P25=1.5 (approx), P75=4.5 (approx) via linear interpolation
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0, 5.0)
	s := analyze.Summarize("TEST", obs)

	if s.P25 >= s.Median {
		t.Errorf("P25 (%g) should be less than Median (%g)", s.P25, s.Median)
	}
	if s.P75 <= s.Median {
		t.Errorf("P75 (%g) should be greater than Median (%g)", s.P75, s.Median)
	}
	if s.P25 >= s.P75 {
		t.Errorf("P25 (%g) should be less than P75 (%g)", s.P25, s.P75)
	}
}

func TestSummarizeFirstLast(t *testing.T) {
	// First and last should be the first/last non-NaN in original order
	obs := []model.Observation{
		{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Value: math.NaN()},
		{Date: time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC), Value: 10.0},
		{Date: time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC), Value: 20.0},
		{Date: time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC), Value: math.NaN()},
	}
	s := analyze.Summarize("TEST", obs)

	if !approxEqual(s.First, 10.0, 1e-9) {
		t.Errorf("First: expected 10.0 (first non-NaN), got %g", s.First)
	}
	if !approxEqual(s.Last, 20.0, 1e-9) {
		t.Errorf("Last: expected 20.0 (last non-NaN), got %g", s.Last)
	}
}

func TestSummarizeChange(t *testing.T) {
	obs := makeObs(2020, 1, 100.0, 110.0, 120.0, 130.0)
	s := analyze.Summarize("TEST", obs)

	if !approxEqual(s.Change, 30.0, 1e-9) {
		t.Errorf("Change: expected 30.0, got %g", s.Change)
	}
	if !approxEqual(s.ChangePct, 30.0, 1e-9) {
		t.Errorf("ChangePct: expected 30.0%%, got %g", s.ChangePct)
	}
}

func TestSummarizeChangeZeroFirst(t *testing.T) {
	// First=0 → ChangePct should be NaN (division by zero)
	obs := makeObs(2020, 1, 0.0, 10.0, 20.0)
	s := analyze.Summarize("TEST", obs)
	if !isNaN(s.ChangePct) {
		t.Errorf("ChangePct: expected NaN when First=0, got %g", s.ChangePct)
	}
}

func TestSummarizeSkew(t *testing.T) {
	// Symmetric series should have skew near 0
	obs := makeObs(2020, 1, 1.0, 2.0, 3.0, 4.0, 5.0)
	s := analyze.Summarize("TEST", obs)
	if !approxEqual(s.Skew, 0.0, 1e-9) {
		t.Errorf("Skew of symmetric series: expected 0.0, got %g", s.Skew)
	}

	// Right-skewed series: large outlier on right → positive skew
	obsSkewed := makeObs(2020, 1, 1.0, 1.0, 1.0, 1.0, 100.0)
	sSkewed := analyze.Summarize("TEST", obsSkewed)
	if sSkewed.Skew <= 0 {
		t.Errorf("Right-skewed series should have positive skew, got %g", sSkewed.Skew)
	}
}

func TestSummarizeNaNExcludedFromStats(t *testing.T) {
	// NaN values should not affect mean, min, max etc.
	obsClean := makeObs(2020, 1, 1.0, 2.0, 3.0)
	obsWithNaN := []model.Observation{
		{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Value: 1.0},
		{Date: time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC), Value: math.NaN()},
		{Date: time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC), Value: 2.0},
		{Date: time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC), Value: math.NaN()},
		{Date: time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC), Value: 3.0},
	}
	sClean := analyze.Summarize("A", obsClean)
	sNaN := analyze.Summarize("B", obsWithNaN)

	if !approxEqual(sClean.Mean, sNaN.Mean, 1e-9) {
		t.Errorf("NaN should not affect mean: %g vs %g", sClean.Mean, sNaN.Mean)
	}
	if !approxEqual(sClean.Min, sNaN.Min, 1e-9) {
		t.Errorf("NaN should not affect min: %g vs %g", sClean.Min, sNaN.Min)
	}
	if !approxEqual(sClean.Max, sNaN.Max, 1e-9) {
		t.Errorf("NaN should not affect max: %g vs %g", sClean.Max, sNaN.Max)
	}
}

func TestSummarizeEmptyInput(t *testing.T) {
	s := analyze.Summarize("TEST", nil)
	if s.Count != 0 {
		t.Errorf("Count: expected 0 for empty input, got %d", s.Count)
	}
}

func TestSummarizeAllNaN(t *testing.T) {
	obs := makeObs(2020, 1, math.NaN(), math.NaN(), math.NaN())
	s := analyze.Summarize("TEST", obs)

	if s.Count != 3 {
		t.Errorf("Count: expected 3, got %d", s.Count)
	}
	if s.Missing != 3 {
		t.Errorf("Missing: expected 3, got %d", s.Missing)
	}
	if !approxEqual(s.MissingPct, 100.0, 1e-9) {
		t.Errorf("MissingPct: expected 100.0, got %g", s.MissingPct)
	}
	if !isNaN(s.Mean) {
		t.Errorf("Mean: expected NaN for all-NaN input, got %g", s.Mean)
	}
	if !isNaN(s.Min) {
		t.Errorf("Min: expected NaN for all-NaN input, got %g", s.Min)
	}
}

func TestSummarizeSingleValue(t *testing.T) {
	obs := makeObs(2020, 1, 42.0)
	s := analyze.Summarize("TEST", obs)

	if s.Count != 1 {
		t.Errorf("Count: expected 1, got %d", s.Count)
	}
	if !approxEqual(s.Mean, 42.0, 1e-9) {
		t.Errorf("Mean: expected 42.0, got %g", s.Mean)
	}
	if !approxEqual(s.Min, 42.0, 1e-9) {
		t.Errorf("Min: expected 42.0, got %g", s.Min)
	}
	if !approxEqual(s.Max, 42.0, 1e-9) {
		t.Errorf("Max: expected 42.0, got %g", s.Max)
	}
	// Std of single value = 0
	if !approxEqual(s.Std, 0.0, 1e-9) {
		t.Errorf("Std: expected 0.0 for single value, got %g", s.Std)
	}
}

// ─── Trend ────────────────────────────────────────────────────────────────────

func TestTrendLinearUpward(t *testing.T) {
	// Perfectly linear increasing series: 1,2,3,...,10 over 10 years
	obs := makeAnnual(2010, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	tr, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Direction != "up" {
		t.Errorf("Direction: expected up, got %q", tr.Direction)
	}
	if tr.SlopePerYear <= 0 {
		t.Errorf("SlopePerYear: expected positive for increasing series, got %g", tr.SlopePerYear)
	}
	// R² should be very close to 1 for a perfectly linear series
	if !approxEqual(tr.R2, 1.0, 1e-6) {
		t.Errorf("R2: expected ~1.0 for perfect linear series, got %g", tr.R2)
	}
}

func TestTrendLinearDownward(t *testing.T) {
	obs := makeAnnual(2010, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1)
	tr, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Direction != "down" {
		t.Errorf("Direction: expected down, got %q", tr.Direction)
	}
	if tr.SlopePerYear >= 0 {
		t.Errorf("SlopePerYear: expected negative for decreasing series, got %g", tr.SlopePerYear)
	}
}

func TestTrendFlat(t *testing.T) {
	// Constant series → slope ≈ 0 → flat direction
	obs := makeAnnual(2010, 5.0, 5.0, 5.0, 5.0, 5.0, 5.0, 5.0, 5.0, 5.0, 5.0)
	tr, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Direction != "flat" {
		t.Errorf("Direction: expected flat for constant series, got %q", tr.Direction)
	}
}

func TestTrendR2Range(t *testing.T) {
	// R² must always be in [0, 1]
	obs := makeObs(2020, 1, 3.5, 4.4, 14.7, 13.3, 11.1, 8.4, 6.9, 6.0, 6.9, 6.7, 6.4, 6.7)
	tr, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.R2 < 0 || tr.R2 > 1 {
		t.Errorf("R2 must be in [0,1], got %g", tr.R2)
	}
}

func TestTrendSlopePerYearConsistent(t *testing.T) {
	// SlopePerYear should be Slope * 365.25
	obs := makeAnnual(2010, 1, 2, 3, 4, 5)
	tr, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := tr.Slope * 365.25
	if !approxEqual(tr.SlopePerYear, expected, 1e-6) {
		t.Errorf("SlopePerYear: expected %g, got %g", expected, tr.SlopePerYear)
	}
}

func TestTrendNaNExcluded(t *testing.T) {
	// NaN observations should be silently dropped; result should still succeed
	obs := []model.Observation{
		{Date: time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC), Value: 1.0},
		{Date: time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC), Value: math.NaN()},
		{Date: time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC), Value: 3.0},
		{Date: time.Date(2013, 1, 1, 0, 0, 0, 0, time.UTC), Value: math.NaN()},
		{Date: time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC), Value: 5.0},
	}
	tr, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err != nil {
		t.Fatalf("unexpected error with NaN gaps: %v", err)
	}
	if tr.Direction != "up" {
		t.Errorf("Direction: expected up for 1,3,5 series, got %q", tr.Direction)
	}
}

func TestTrendTooFewObs(t *testing.T) {
	obs := makeObs(2020, 1, 5.0) // only 1 observation
	_, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err == nil {
		t.Error("expected error for single observation")
	}
}

func TestTrendTooFewObsAfterNaN(t *testing.T) {
	obs := makeObs(2020, 1, math.NaN(), 5.0) // only 1 non-NaN
	_, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err == nil {
		t.Error("expected error when fewer than 2 non-NaN observations")
	}
}

func TestTrendAllNaN(t *testing.T) {
	obs := makeObs(2020, 1, math.NaN(), math.NaN(), math.NaN())
	_, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err == nil {
		t.Error("expected error for all-NaN input")
	}
}

func TestTrendSeriesIDPreserved(t *testing.T) {
	obs := makeAnnual(2010, 1.0, 2.0, 3.0)
	tr, err := analyze.Trend("MYID", obs, analyze.TrendLinear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.SeriesID != "MYID" {
		t.Errorf("SeriesID: expected MYID, got %q", tr.SeriesID)
	}
}

func TestTrendMethodPreserved(t *testing.T) {
	obs := makeAnnual(2010, 1.0, 2.0, 3.0, 4.0, 5.0)
	tr, err := analyze.Trend("TEST", obs, analyze.TrendTheilSen)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Method != analyze.TrendTheilSen {
		t.Errorf("Method: expected theil-sen, got %q", tr.Method)
	}
}

// ─── Theil-Sen ────────────────────────────────────────────────────────────────

func TestTrendTheilSenUpward(t *testing.T) {
	obs := makeAnnual(2010, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	tr, err := analyze.Trend("TEST", obs, analyze.TrendTheilSen)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Direction != "up" {
		t.Errorf("Direction: expected up, got %q", tr.Direction)
	}
	if tr.SlopePerYear <= 0 {
		t.Errorf("SlopePerYear: expected positive, got %g", tr.SlopePerYear)
	}
}

func TestTrendTheilSenRobustToOutlier(t *testing.T) {
	// Theil-Sen is robust: adding one massive outlier shouldn't flip direction
	// Base series clearly trending up: 1,2,3,4,5,6,7,8,9,10
	// With outlier: inject -1000 in the middle
	obs := []model.Observation{
		{Date: time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC), Value: 1.0},
		{Date: time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC), Value: 2.0},
		{Date: time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC), Value: 3.0},
		{Date: time.Date(2013, 1, 1, 0, 0, 0, 0, time.UTC), Value: -1000.0}, // outlier
		{Date: time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC), Value: 5.0},
		{Date: time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC), Value: 6.0},
		{Date: time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC), Value: 7.0},
		{Date: time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC), Value: 8.0},
		{Date: time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), Value: 9.0},
		{Date: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC), Value: 10.0},
	}
	trTS, errTS := analyze.Trend("TEST", obs, analyze.TrendTheilSen)
	trOLS, errOLS := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if errTS != nil || errOLS != nil {
		t.Fatalf("unexpected errors: TS=%v OLS=%v", errTS, errOLS)
	}

	// Theil-Sen should still show upward direction
	if trTS.Direction != "up" {
		t.Errorf("Theil-Sen should be robust to outlier; direction=%q", trTS.Direction)
	}
	// OLS may or may not flip — we don't assert its direction here
	_ = trOLS
}

func TestTrendTheilSenR2Range(t *testing.T) {
	obs := makeObs(2020, 1, 3.5, 4.4, 14.7, 13.3, 11.1, 8.4, 6.9, 6.0, 6.9, 6.7, 6.4, 6.7)
	tr, err := analyze.Trend("TEST", obs, analyze.TrendTheilSen)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.R2 < 0 || tr.R2 > 1 {
		t.Errorf("R2 must be in [0,1], got %g", tr.R2)
	}
}

// ─── Composition ──────────────────────────────────────────────────────────────

func TestSummarizeThenTrendDirection(t *testing.T) {
	// Upward series: summary change should be positive AND trend direction = up
	obs := makeAnnual(2010, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100)
	s := analyze.Summarize("TEST", obs)
	tr, err := analyze.Trend("TEST", obs, analyze.TrendLinear)
	if err != nil {
		t.Fatalf("Trend: %v", err)
	}

	if s.Change <= 0 {
		t.Errorf("Summary.Change should be positive for upward series, got %g", s.Change)
	}
	if tr.Direction != "up" {
		t.Errorf("Trend.Direction should be up, got %q", tr.Direction)
	}
}

func TestSummarizeCountMatchesNonNaN(t *testing.T) {
	// Count - Missing should equal the number of valid values used in stats
	obs := makeObs(2020, 1, 1.0, math.NaN(), 3.0, math.NaN(), 5.0)
	s := analyze.Summarize("TEST", obs)

	validCount := s.Count - s.Missing
	if validCount != 3 {
		t.Errorf("valid count (Count-Missing): expected 3, got %d", validCount)
	}
	// Mean of [1,3,5] = 3
	if !approxEqual(s.Mean, 3.0, 1e-9) {
		t.Errorf("Mean of [1,3,5]: expected 3.0, got %g", s.Mean)
	}
}
