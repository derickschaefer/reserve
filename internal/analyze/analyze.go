// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

// Package analyze computes statistical summaries and trend analysis over
// slices of Observations. All functions are pure; no I/O.
package analyze

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
)

// ─── Summary ──────────────────────────────────────────────────────────────────

// Summary holds descriptive statistics for a series.
type Summary struct {
	AnalysisVersion string   `json:"analysis_version"`
	SeriesID        string   `json:"series_id"`
	CitationText    string   `json:"citation_text,omitempty"`
	SourceName      string   `json:"source_name,omitempty"`
	SourceNames     []string `json:"source_names,omitempty"`
	StartDate       string   `json:"start_date,omitempty"`
	EndDate         string   `json:"end_date,omitempty"`
	Count           int      `json:"count"` // total observations
	NObs            int      `json:"n_obs"` // alias for count in machine-consumption pipelines
	MissingCount    int      `json:"missing_count"`
	MissingPct      float64  `json:"missing_pct"` // percent missing
	Mean            float64  `json:"mean"`
	Std             float64  `json:"std"`
	Min             float64  `json:"min"`
	P25             float64  `json:"p25"`
	Median          float64  `json:"median"`
	P75             float64  `json:"p75"`
	Max             float64  `json:"max"`
	Skew            float64  `json:"skew"`
	First           float64  `json:"first"`      // first non-NaN value
	Last            float64  `json:"last"`       // last non-NaN value
	Change          float64  `json:"change"`     // Last - First
	ChangePct       float64  `json:"change_pct"` // (Last-First)/First * 100
}

// Summarize computes descriptive statistics over obs.
// NaN values are excluded from all numeric computations but counted.
func Summarize(seriesID string, obs []model.Observation) Summary {
	s := Summary{
		AnalysisVersion: "1.0",
		SeriesID:        seriesID,
		Count:           len(obs),
		NObs:            len(obs),
	}
	if len(obs) == 0 {
		return s
	}
	s.StartDate = obs[0].Date.Format("2006-01-02")
	s.EndDate = obs[0].Date.Format("2006-01-02")

	var vals []float64
	for _, o := range obs {
		if o.Date.Format("2006-01-02") < s.StartDate {
			s.StartDate = o.Date.Format("2006-01-02")
		}
		if o.Date.Format("2006-01-02") > s.EndDate {
			s.EndDate = o.Date.Format("2006-01-02")
		}
		if math.IsNaN(o.Value) {
			s.MissingCount++
		} else {
			vals = append(vals, o.Value)
		}
	}
	if s.Count > 0 {
		s.MissingPct = float64(s.MissingCount) / float64(s.Count) * 100
	}
	if len(vals) == 0 {
		s.Mean = math.NaN()
		s.Std = math.NaN()
		s.Min = math.NaN()
		s.Max = math.NaN()
		s.Median = math.NaN()
		s.P25 = math.NaN()
		s.P75 = math.NaN()
		s.Skew = math.NaN()
		s.First = math.NaN()
		s.Last = math.NaN()
		s.Change = math.NaN()
		s.ChangePct = math.NaN()
		return s
	}

	// Sort for percentile computation
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)

	s.Min = sorted[0]
	s.Max = sorted[len(sorted)-1]
	s.Mean = sumF(vals) / float64(len(vals))
	s.Std = stddevF(vals, s.Mean)
	s.Median = percentile(sorted, 50)
	s.P25 = percentile(sorted, 25)
	s.P75 = percentile(sorted, 75)
	s.Skew = skewness(vals, s.Mean, s.Std)

	// First and last non-NaN values in original order
	for _, o := range obs {
		if !math.IsNaN(o.Value) {
			s.First = o.Value
			break
		}
	}
	for i := len(obs) - 1; i >= 0; i-- {
		if !math.IsNaN(obs[i].Value) {
			s.Last = obs[i].Value
			break
		}
	}
	s.Change = s.Last - s.First
	if s.First != 0 {
		s.ChangePct = s.Change / math.Abs(s.First) * 100
	} else {
		s.ChangePct = math.NaN()
	}

	return s
}

// ─── Trend ────────────────────────────────────────────────────────────────────

// TrendMethod selects the regression algorithm.
type TrendMethod string

const (
	TrendLinear   TrendMethod = "linear"
	TrendTheilSen TrendMethod = "theil-sen"
)

// TrendResult holds the output of a trend analysis.
type TrendResult struct {
	SeriesID     string           `json:"series_id"`
	CitationText string           `json:"citation_text,omitempty"`
	SourceName   string           `json:"source_name,omitempty"`
	SourceNames  []string         `json:"source_names,omitempty"`
	Method       TrendMethod      `json:"method"`
	Slope        float64          `json:"slope"` // units per day
	Intercept    float64          `json:"intercept"`
	R2           float64          `json:"r2"`
	Direction    string           `json:"direction"`      // "up", "down", "flat"
	SlopePerYear float64          `json:"slope_per_year"` // slope * 365.25
	Confidence   *TrendConfidence `json:"confidence,omitempty"`
}

type TrendConfidence struct {
	SlopeStdErr       float64 `json:"slope_stderr"`
	SlopePValue       float64 `json:"slope_p_value"`
	SlopeCI95Low      float64 `json:"slope_ci95_low"`
	SlopeCI95High     float64 `json:"slope_ci95_high"`
	SlopeYearCI95Low  float64 `json:"slope_per_year_ci95_low"`
	SlopeYearCI95High float64 `json:"slope_per_year_ci95_high"`
}

type CompareResult struct {
	SeriesID            string   `json:"series_id"`
	CitationText        string   `json:"citation_text,omitempty"`
	SourceName          string   `json:"source_name,omitempty"`
	SourceNames         []string `json:"source_names,omitempty"`
	AgainstSeriesID     string   `json:"against_series_id"`
	AgainstCitationText string   `json:"against_citation_text,omitempty"`
	AgainstSourceName   string   `json:"against_source_name,omitempty"`
	AgainstSourceNames  []string `json:"against_source_names,omitempty"`
	CountAligned        int      `json:"count_aligned"`
	Correlation         float64  `json:"correlation"`
	Beta                float64  `json:"beta"`
	DeltaMean           float64  `json:"delta_mean"`
	DeltaLast           float64  `json:"delta_last"`
	TrackingError       float64  `json:"tracking_error"`
}

type RegimeChangePoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
	Score float64 `json:"score"`
	Side  string  `json:"side"`
}

type RegimeSegment struct {
	StartDate    string  `json:"start_date"`
	EndDate      string  `json:"end_date"`
	Count        int     `json:"count"`
	SlopePerYear float64 `json:"slope_per_year"`
	Direction    string  `json:"direction"`
}

type RegimeResult struct {
	SeriesID     string              `json:"series_id"`
	CitationText string              `json:"citation_text,omitempty"`
	SourceName   string              `json:"source_name,omitempty"`
	SourceNames  []string            `json:"source_names,omitempty"`
	Method       string              `json:"method"`
	Threshold    float64             `json:"threshold"`
	MinGap       int                 `json:"min_gap"`
	Signal       string              `json:"signal"`
	ChangePoints []RegimeChangePoint `json:"change_points"`
	Segments     []RegimeSegment     `json:"segments"`
}

// Trend fits a linear trend to the observations.
// X values are days since the first observation date.
// NaN observations are excluded.
func Trend(seriesID string, obs []model.Observation, method TrendMethod) (TrendResult, error) {
	tr := TrendResult{SeriesID: seriesID, Method: method}

	// Build (x, y) pairs
	var pts []point
	var t0 int64
	first := true
	for _, o := range obs {
		if math.IsNaN(o.Value) {
			continue
		}
		unix := o.Date.Unix()
		if first {
			t0 = unix
			first = false
		}
		x := float64(unix-t0) / 86400 // days from first obs
		pts = append(pts, point{x, o.Value})
	}
	if len(pts) < 2 {
		return tr, fmt.Errorf("trend: need at least 2 non-NaN observations, got %d", len(pts))
	}

	switch method {
	case TrendTheilSen:
		tr.Slope = theilSenSlope(pts)
		// Use OLS intercept with Theil-Sen slope
		xMean := meanPts(pts, func(p point) float64 { return p.x })
		yMean := meanPts(pts, func(p point) float64 { return p.y })
		tr.Intercept = yMean - tr.Slope*xMean
	default: // linear OLS
		tr.Slope, tr.Intercept = olsRegress(pts)
	}

	tr.R2 = r2(pts, tr.Slope, tr.Intercept)
	tr.SlopePerYear = tr.Slope * 365.25

	switch {
	case tr.SlopePerYear > 0.01:
		tr.Direction = "up"
	case tr.SlopePerYear < -0.01:
		tr.Direction = "down"
	default:
		tr.Direction = "flat"
	}
	return tr, nil
}

// AddTrendConfidence computes confidence metadata for linear OLS trend results.
// For unsupported methods, it returns nil.
func AddTrendConfidence(tr TrendResult, obs []model.Observation) *TrendConfidence {
	if tr.Method != TrendLinear {
		return nil
	}
	var pts []point
	var t0 int64
	first := true
	for _, o := range obs {
		if math.IsNaN(o.Value) {
			continue
		}
		unix := o.Date.Unix()
		if first {
			t0 = unix
			first = false
		}
		x := float64(unix-t0) / 86400
		pts = append(pts, point{x, o.Value})
	}
	if len(pts) < 3 {
		return nil
	}
	xMean := meanPts(pts, func(p point) float64 { return p.x })
	var sxx float64
	var rss float64
	for _, p := range pts {
		dx := p.x - xMean
		sxx += dx * dx
		res := p.y - (tr.Slope*p.x + tr.Intercept)
		rss += res * res
	}
	if sxx == 0 {
		return nil
	}
	df := float64(len(pts) - 2)
	if df <= 0 {
		return nil
	}
	s2 := rss / df
	stderr := math.Sqrt(s2 / sxx)
	if stderr == 0 {
		return &TrendConfidence{
			SlopeStdErr:       0,
			SlopePValue:       0,
			SlopeCI95Low:      tr.Slope,
			SlopeCI95High:     tr.Slope,
			SlopeYearCI95Low:  tr.SlopePerYear,
			SlopeYearCI95High: tr.SlopePerYear,
		}
	}
	tAbs := math.Abs(tr.Slope / stderr)
	// Normal approximation for two-sided p-value.
	p := 2 * (1 - normalCDF(tAbs))
	z95 := 1.96
	ciLow := tr.Slope - z95*stderr
	ciHigh := tr.Slope + z95*stderr
	return &TrendConfidence{
		SlopeStdErr:       stderr,
		SlopePValue:       p,
		SlopeCI95Low:      ciLow,
		SlopeCI95High:     ciHigh,
		SlopeYearCI95Low:  ciLow * 365.25,
		SlopeYearCI95High: ciHigh * 365.25,
	}
}

func Compare(lhsSeriesID string, lhs []model.Observation, rhsSeriesID string, rhs []model.Observation) (CompareResult, error) {
	res := CompareResult{SeriesID: lhsSeriesID, AgainstSeriesID: rhsSeriesID}
	rhsByDate := make(map[time.Time]float64, len(rhs))
	for _, o := range rhs {
		if math.IsNaN(o.Value) {
			continue
		}
		rhsByDate[o.Date] = o.Value
	}
	var x, y []float64
	for _, o := range lhs {
		if math.IsNaN(o.Value) {
			continue
		}
		v, ok := rhsByDate[o.Date]
		if !ok || math.IsNaN(v) {
			continue
		}
		x = append(x, o.Value)
		y = append(y, v)
	}
	if len(x) < 2 {
		return res, fmt.Errorf("compare: need at least 2 aligned non-NaN observations, got %d", len(x))
	}
	res.CountAligned = len(x)
	mx := sumF(x) / float64(len(x))
	my := sumF(y) / float64(len(y))
	var cov, vx, vy float64
	for i := range x {
		dx := x[i] - mx
		dy := y[i] - my
		cov += dx * dy
		vx += dx * dx
		vy += dy * dy
	}
	if vx > 0 && vy > 0 {
		res.Correlation = cov / math.Sqrt(vx*vy)
	}
	if vy > 0 {
		res.Beta = cov / vy
	}
	var diffs []float64
	for i := range x {
		d := x[i] - y[i]
		diffs = append(diffs, d)
	}
	res.DeltaMean = sumF(diffs) / float64(len(diffs))
	res.DeltaLast = diffs[len(diffs)-1]
	res.TrackingError = stddevF(diffs, res.DeltaMean)
	return res, nil
}

func RegimeCUSUM(seriesID string, obs []model.Observation, threshold float64) (RegimeResult, error) {
	result := RegimeResult{
		SeriesID:  seriesID,
		Method:    "cusum",
		Threshold: threshold,
		Signal:    "diff",
	}
	clean := make([]model.Observation, 0, len(obs))
	for _, o := range obs {
		if !math.IsNaN(o.Value) {
			clean = append(clean, o)
		}
	}
	if len(clean) < 5 {
		return result, fmt.Errorf("regime: need at least 5 non-NaN observations, got %d", len(clean))
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i].Date.Before(clean[j].Date) })
	// Hardened signal: run CUSUM on first differences, not raw levels.
	// This reduces false positives in long monotonic trends.
	diffs := make([]float64, 0, len(clean)-1)
	for i := 1; i < len(clean); i++ {
		diffs = append(diffs, clean[i].Value-clean[i-1].Value)
	}
	mean := sumF(diffs) / float64(len(diffs))
	std := stddevF(diffs, mean)
	if std == 0 {
		result.Segments = []RegimeSegment{segmentForRange(clean, 0, len(clean)-1)}
		return result, nil
	}
	k := 0.5 * std
	h := threshold * std
	minGap := regimeMinGap(len(clean))
	result.MinGap = minGap
	var spos, sneg float64
	lastSplit := 0
	lastCPIdx := -minGap - 1
	for i, o := range clean {
		if i == 0 {
			continue
		}
		d := (clean[i].Value - clean[i-1].Value) - mean
		spos = math.Max(0, spos+d-k)
		sneg = math.Min(0, sneg+d+k)
		if i-lastCPIdx < minGap {
			continue
		}
		if spos > h {
			result.ChangePoints = append(result.ChangePoints, RegimeChangePoint{
				Date:  o.Date.Format("2006-01-02"),
				Value: o.Value,
				Score: spos / std,
				Side:  "up",
			})
			if i-lastSplit >= 1 {
				result.Segments = append(result.Segments, segmentForRange(clean, lastSplit, i))
			}
			lastSplit = i + 1
			lastCPIdx = i
			spos, sneg = 0, 0
			continue
		}
		if -sneg > h {
			result.ChangePoints = append(result.ChangePoints, RegimeChangePoint{
				Date:  o.Date.Format("2006-01-02"),
				Value: o.Value,
				Score: -sneg / std,
				Side:  "down",
			})
			if i-lastSplit >= 1 {
				result.Segments = append(result.Segments, segmentForRange(clean, lastSplit, i))
			}
			lastSplit = i + 1
			lastCPIdx = i
			spos, sneg = 0, 0
		}
	}
	if lastSplit <= len(clean)-1 {
		result.Segments = append(result.Segments, segmentForRange(clean, lastSplit, len(clean)-1))
	}
	return result, nil
}

func regimeMinGap(n int) int {
	// 6 observations works well as a default debounce for monthly macro series,
	// while still permitting meaningful regime transitions.
	if n < 24 {
		return 3
	}
	return 6
}

// SummarizeWindows returns rolling-window summaries across a single series.
func SummarizeWindows(seriesID string, obs []model.Observation, window int) []Summary {
	if window <= 0 || len(obs) < window {
		return nil
	}
	sorted := append([]model.Observation(nil), obs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })
	out := make([]Summary, 0, len(sorted)-window+1)
	for i := window; i <= len(sorted); i++ {
		w := sorted[i-window : i]
		out = append(out, Summarize(seriesID, w))
	}
	return out
}

// ─── Math helpers ─────────────────────────────────────────────────────────────

func sumF(vals []float64) float64 {
	var s float64
	for _, v := range vals {
		s += v
	}
	return s
}

func stddevF(vals []float64, m float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	var sq float64
	for _, v := range vals {
		d := v - m
		sq += d * d
	}
	return math.Sqrt(sq / float64(len(vals)-1))
}

func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	idx := p / 100 * float64(n-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func skewness(vals []float64, mean, std float64) float64 {
	n := float64(len(vals))
	if n < 3 || std == 0 {
		return 0
	}
	var s float64
	for _, v := range vals {
		d := (v - mean) / std
		s += d * d * d
	}
	return s * n / ((n - 1) * (n - 2))
}

type point struct{ x, y float64 }

func olsRegress(pts []point) (slope, intercept float64) {
	n := float64(len(pts))
	var xSum, ySum, xySum, x2Sum float64
	for _, p := range pts {
		xSum += p.x
		ySum += p.y
		xySum += p.x * p.y
		x2Sum += p.x * p.x
	}
	denom := n*x2Sum - xSum*xSum
	if denom == 0 {
		return 0, ySum / n
	}
	slope = (n*xySum - xSum*ySum) / denom
	intercept = (ySum - slope*xSum) / n
	return
}

func theilSenSlope(pts []point) float64 {
	var slopes []float64
	for i := 0; i < len(pts); i++ {
		for j := i + 1; j < len(pts); j++ {
			dx := pts[j].x - pts[i].x
			if dx == 0 {
				continue
			}
			slopes = append(slopes, (pts[j].y-pts[i].y)/dx)
		}
	}
	if len(slopes) == 0 {
		return 0
	}
	sort.Float64s(slopes)
	return percentile(slopes, 50)
}

func r2(pts []point, slope, intercept float64) float64 {
	var yMean float64
	for _, p := range pts {
		yMean += p.y
	}
	yMean /= float64(len(pts))

	var ssTot, ssRes float64
	for _, p := range pts {
		pred := slope*p.x + intercept
		ssTot += (p.y - yMean) * (p.y - yMean)
		ssRes += (p.y - pred) * (p.y - pred)
	}
	if ssTot == 0 {
		return 1
	}
	return 1 - ssRes/ssTot
}

func meanPts(pts []point, f func(point) float64) float64 {
	var s float64
	for _, p := range pts {
		s += f(p)
	}
	return s / float64(len(pts))
}

func normalCDF(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt2))
}

func segmentForRange(obs []model.Observation, start, end int) RegimeSegment {
	seg := RegimeSegment{
		StartDate: obs[start].Date.Format("2006-01-02"),
		EndDate:   obs[end].Date.Format("2006-01-02"),
		Count:     end - start + 1,
	}
	if seg.Count < 2 {
		seg.Direction = "flat"
		return seg
	}
	slope, _ := olsRegress(segmentPoints(obs[start : end+1]))
	seg.SlopePerYear = slope * 365.25
	switch {
	case seg.SlopePerYear > 0.01:
		seg.Direction = "up"
	case seg.SlopePerYear < -0.01:
		seg.Direction = "down"
	default:
		seg.Direction = "flat"
	}
	return seg
}

func segmentPoints(obs []model.Observation) []point {
	pts := make([]point, 0, len(obs))
	t0 := obs[0].Date.Unix()
	for _, o := range obs {
		x := float64(o.Date.Unix()-t0) / 86400
		pts = append(pts, point{x, o.Value})
	}
	return pts
}
