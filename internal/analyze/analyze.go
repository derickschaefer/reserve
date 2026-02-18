// Package analyze computes statistical summaries and trend analysis over
// slices of Observations. All functions are pure; no I/O.
package analyze

import (
	"fmt"
	"math"
	"sort"

	"github.com/derickschaefer/reserve/internal/model"
)

// ─── Summary ──────────────────────────────────────────────────────────────────

// Summary holds descriptive statistics for a series.
type Summary struct {
	SeriesID   string  `json:"series_id"`
	Count      int     `json:"count"`       // total observations
	Missing    int     `json:"missing"`     // NaN count
	MissingPct float64 `json:"missing_pct"` // percent missing
	Mean       float64 `json:"mean"`
	Std        float64 `json:"std"`
	Min        float64 `json:"min"`
	P25        float64 `json:"p25"`
	Median     float64 `json:"median"`
	P75        float64 `json:"p75"`
	Max        float64 `json:"max"`
	Skew       float64 `json:"skew"`
	First      float64 `json:"first"`      // first non-NaN value
	Last       float64 `json:"last"`       // last non-NaN value
	Change     float64 `json:"change"`     // Last - First
	ChangePct  float64 `json:"change_pct"` // (Last-First)/First * 100
}

// Summarize computes descriptive statistics over obs.
// NaN values are excluded from all numeric computations but counted.
func Summarize(seriesID string, obs []model.Observation) Summary {
	s := Summary{SeriesID: seriesID, Count: len(obs)}
	if len(obs) == 0 {
		return s
	}

	var vals []float64
	for _, o := range obs {
		if math.IsNaN(o.Value) {
			s.Missing++
		} else {
			vals = append(vals, o.Value)
		}
	}
	if s.Count > 0 {
		s.MissingPct = float64(s.Missing) / float64(s.Count) * 100
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
	SeriesID     string      `json:"series_id"`
	Method       TrendMethod `json:"method"`
	Slope        float64     `json:"slope"` // units per day
	Intercept    float64     `json:"intercept"`
	R2           float64     `json:"r2"`
	Direction    string      `json:"direction"`      // "up", "down", "flat"
	SlopePerYear float64     `json:"slope_per_year"` // slope * 365.25
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
