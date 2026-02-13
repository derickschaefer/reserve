// Package transform implements stateless pipeline operators that take a slice
// of Observations and return a new slice. Each operator is a pure function;
// no side effects, no I/O.
package transform

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
)

// ─── Percent Change ───────────────────────────────────────────────────────────

// PctChange computes (v[t] - v[t-period]) / v[t-period] * 100.
// Leading observations that have no prior period are dropped.
// NaN inputs propagate as NaN outputs.
func PctChange(obs []model.Observation, period int) ([]model.Observation, error) {
	if period < 1 {
		return nil, fmt.Errorf("pct-change: period must be >= 1, got %d", period)
	}
	if len(obs) <= period {
		return nil, fmt.Errorf("pct-change: need more than %d observations, got %d", period, len(obs))
	}
	out := make([]model.Observation, 0, len(obs)-period)
	for i := period; i < len(obs); i++ {
		curr := obs[i].Value
		prev := obs[i-period].Value
		var val float64
		if math.IsNaN(curr) || math.IsNaN(prev) || prev == 0 {
			val = math.NaN()
		} else {
			val = (curr - prev) / math.Abs(prev) * 100
		}
		out = append(out, model.Observation{
			Date:     obs[i].Date,
			Value:    val,
			ValueRaw: formatRaw(val),
		})
	}
	return out, nil
}

// ─── Difference ───────────────────────────────────────────────────────────────

// Diff computes the n-th order difference. order=1: v[t]-v[t-1], order=2: diff of diff.
func Diff(obs []model.Observation, order int) ([]model.Observation, error) {
	if order < 1 || order > 2 {
		return nil, fmt.Errorf("diff: order must be 1 or 2, got %d", order)
	}
	result := obs
	var err error
	for i := 0; i < order; i++ {
		result, err = diffOnce(result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func diffOnce(obs []model.Observation) ([]model.Observation, error) {
	if len(obs) < 2 {
		return nil, fmt.Errorf("diff: need at least 2 observations, got %d", len(obs))
	}
	out := make([]model.Observation, 0, len(obs)-1)
	for i := 1; i < len(obs); i++ {
		var val float64
		if math.IsNaN(obs[i].Value) || math.IsNaN(obs[i-1].Value) {
			val = math.NaN()
		} else {
			val = obs[i].Value - obs[i-1].Value
		}
		out = append(out, model.Observation{
			Date:     obs[i].Date,
			Value:    val,
			ValueRaw: formatRaw(val),
		})
	}
	return out, nil
}

// ─── Log ──────────────────────────────────────────────────────────────────────

// Log computes the natural log of each observation value.
// Non-positive values produce NaN with a warning; NaN inputs stay NaN.
func Log(obs []model.Observation) ([]model.Observation, []string) {
	out := make([]model.Observation, len(obs))
	var warnings []string
	for i, o := range obs {
		var val float64
		if math.IsNaN(o.Value) {
			val = math.NaN()
		} else if o.Value <= 0 {
			val = math.NaN()
			warnings = append(warnings, fmt.Sprintf("%s: log(%g) is undefined, set to NaN",
				o.Date.Format("2006-01-02"), o.Value))
		} else {
			val = math.Log(o.Value)
		}
		out[i] = model.Observation{
			Date:     o.Date,
			Value:    val,
			ValueRaw: formatRaw(val),
		}
	}
	return out, warnings
}

// ─── Index ────────────────────────────────────────────────────────────────────

// Index re-scales the series so the value at anchorDate equals base.
// All other values are scaled proportionally.
func Index(obs []model.Observation, base float64, anchorDate time.Time) ([]model.Observation, error) {
	// Find anchor value
	var anchor float64
	found := false
	for _, o := range obs {
		if o.Date.Equal(anchorDate) {
			if math.IsNaN(o.Value) {
				return nil, fmt.Errorf("index: anchor date %s has missing value",
					anchorDate.Format("2006-01-02"))
			}
			if o.Value == 0 {
				return nil, fmt.Errorf("index: anchor date %s has zero value, cannot index",
					anchorDate.Format("2006-01-02"))
			}
			anchor = o.Value
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("index: anchor date %s not found in series",
			anchorDate.Format("2006-01-02"))
	}

	scale := base / anchor
	out := make([]model.Observation, len(obs))
	for i, o := range obs {
		var val float64
		if math.IsNaN(o.Value) {
			val = math.NaN()
		} else {
			val = o.Value * scale
		}
		out[i] = model.Observation{
			Date:     o.Date,
			Value:    val,
			ValueRaw: formatRaw(val),
		}
	}
	return out, nil
}

// ─── Normalize ────────────────────────────────────────────────────────────────

// NormalizeMethod selects the normalization algorithm.
type NormalizeMethod string

const (
	NormalizeZScore NormalizeMethod = "zscore"
	NormalizeMinMax NormalizeMethod = "minmax"
)

// Normalize scales observations using z-score or min-max normalization.
// NaN values are skipped when computing statistics but preserved in output.
func Normalize(obs []model.Observation, method NormalizeMethod) ([]model.Observation, error) {
	// Collect non-NaN values
	var vals []float64
	for _, o := range obs {
		if !math.IsNaN(o.Value) {
			vals = append(vals, o.Value)
		}
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("normalize: no non-NaN values in series")
	}

	var a, b float64 // output = (v - a) / b
	switch method {
	case NormalizeZScore:
		mean := mean(vals)
		std := stddev(vals, mean)
		if std == 0 {
			return nil, fmt.Errorf("normalize: standard deviation is zero, cannot z-score")
		}
		a, b = mean, std
	case NormalizeMinMax:
		mn, mx := minmax(vals)
		rng := mx - mn
		if rng == 0 {
			return nil, fmt.Errorf("normalize: min == max (%g), cannot min-max normalize", mn)
		}
		a, b = mn, rng
	default:
		return nil, fmt.Errorf("normalize: unknown method %q (use zscore or minmax)", method)
	}

	out := make([]model.Observation, len(obs))
	for i, o := range obs {
		var val float64
		if math.IsNaN(o.Value) {
			val = math.NaN()
		} else {
			val = (o.Value - a) / b
		}
		out[i] = model.Observation{
			Date:     o.Date,
			Value:    val,
			ValueRaw: formatRaw(val),
		}
	}
	return out, nil
}

// ─── Resample ─────────────────────────────────────────────────────────────────

// ResampleFreq is the target frequency for resampling.
type ResampleFreq string

const (
	ResampleMonthly   ResampleFreq = "monthly"
	ResampleQuarterly ResampleFreq = "quarterly"
	ResampleAnnual    ResampleFreq = "annual"
)

// ResampleMethod is the aggregation method for resampling.
type ResampleMethod string

const (
	ResampleMean ResampleMethod = "mean"
	ResampleLast ResampleMethod = "last"
	ResampleSum  ResampleMethod = "sum"
)

// Resample aggregates observations to a lower frequency.
// Observations are grouped by period; NaN values are skipped in aggregation.
func Resample(obs []model.Observation, freq ResampleFreq, method ResampleMethod) ([]model.Observation, error) {
	if len(obs) == 0 {
		return nil, fmt.Errorf("resample: empty input")
	}

	// Group by period key
	groups := make(map[string][]float64)
	keys := make(map[string]time.Time) // period key → period start date

	for _, o := range obs {
		key, start := periodKey(o.Date, freq)
		if !math.IsNaN(o.Value) {
			groups[key] = append(groups[key], o.Value)
		}
		if _, exists := keys[key]; !exists {
			keys[key] = start
		}
	}

	// Sort period keys
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	out := make([]model.Observation, 0, len(sorted))
	for _, k := range sorted {
		vals := groups[k]
		var val float64
		if len(vals) == 0 {
			val = math.NaN()
		} else {
			switch method {
			case ResampleMean:
				val = mean(vals)
			case ResampleLast:
				val = vals[len(vals)-1]
			case ResampleSum:
				val = sum(vals)
			default:
				return nil, fmt.Errorf("resample: unknown method %q (use mean, last, sum)", method)
			}
		}
		out = append(out, model.Observation{
			Date:     keys[k],
			Value:    val,
			ValueRaw: formatRaw(val),
		})
	}
	return out, nil
}

// periodKey returns a sortable string key and canonical start date for a period.
func periodKey(t time.Time, freq ResampleFreq) (string, time.Time) {
	switch freq {
	case ResampleQuarterly:
		q := (t.Month()-1)/3 + 1
		start := time.Date(t.Year(), time.Month((q-1)*3+1), 1, 0, 0, 0, 0, time.UTC)
		return fmt.Sprintf("%04d-Q%d", t.Year(), q), start
	case ResampleAnnual:
		start := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return fmt.Sprintf("%04d", t.Year()), start
	default: // monthly
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		return fmt.Sprintf("%04d-%02d", t.Year(), t.Month()), start
	}
}

// ─── Filter ───────────────────────────────────────────────────────────────────

// FilterOptions describes a date/value filter predicate.
type FilterOptions struct {
	After      time.Time // keep obs with date > After (zero = no lower bound)
	Before     time.Time // keep obs with date < Before (zero = no upper bound)
	MinValue   float64   // keep obs with value >= MinValue (NaN = no lower bound)
	MaxValue   float64   // keep obs with value <= MaxValue (NaN = no upper bound)
	DropMissing bool     // drop NaN observations
}

// Filter returns observations matching all non-zero criteria in opts.
func Filter(obs []model.Observation, opts FilterOptions) []model.Observation {
	out := make([]model.Observation, 0, len(obs))
	for _, o := range obs {
		if !opts.After.IsZero() && !o.Date.After(opts.After) {
			continue
		}
		if !opts.Before.IsZero() && !o.Date.Before(opts.Before) {
			continue
		}
		if math.IsNaN(o.Value) {
			if opts.DropMissing {
				continue
			}
		} else {
			if !math.IsNaN(opts.MinValue) && o.Value < opts.MinValue {
				continue
			}
			if !math.IsNaN(opts.MaxValue) && o.Value > opts.MaxValue {
				continue
			}
		}
		out = append(out, o)
	}
	return out
}

// ─── Rolling Window ───────────────────────────────────────────────────────────

// RollStat selects the statistic for rolling window computation.
type RollStat string

const (
	RollMean RollStat = "mean"
	RollStd  RollStat = "std"
	RollMin  RollStat = "min"
	RollMax  RollStat = "max"
	RollSum  RollStat = "sum"
)

// Roll computes a rolling window statistic. Window observations include the
// current point and the (window-1) preceding points. NaN values are skipped.
// If fewer than minPeriods non-NaN values exist in a window, the output is NaN.
func Roll(obs []model.Observation, window int, minPeriods int, stat RollStat) ([]model.Observation, error) {
	if window < 1 {
		return nil, fmt.Errorf("roll: window must be >= 1, got %d", window)
	}
	if minPeriods < 1 {
		minPeriods = 1
	}
	if minPeriods > window {
		return nil, fmt.Errorf("roll: min-periods (%d) cannot exceed window (%d)", minPeriods, window)
	}

	out := make([]model.Observation, len(obs))
	for i, o := range obs {
		start := i - window + 1
		if start < 0 {
			start = 0
		}
		window_obs := obs[start : i+1]

		// Collect non-NaN values in window
		var vals []float64
		for _, w := range window_obs {
			if !math.IsNaN(w.Value) {
				vals = append(vals, w.Value)
			}
		}

		var val float64
		if len(vals) < minPeriods {
			val = math.NaN()
		} else {
			switch stat {
			case RollMean:
				val = mean(vals)
			case RollStd:
				val = stddev(vals, mean(vals))
			case RollMin:
				val, _ = minmax(vals)
			case RollMax:
				_, val = minmax(vals)
			case RollSum:
				val = sum(vals)
			default:
				return nil, fmt.Errorf("roll: unknown stat %q (use mean, std, min, max, sum)", stat)
			}
		}
		out[i] = model.Observation{
			Date:     o.Date,
			Value:    val,
			ValueRaw: formatRaw(val),
		}
	}
	return out, nil
}

// ─── Math helpers ─────────────────────────────────────────────────────────────

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return math.NaN()
	}
	return sum(vals) / float64(len(vals))
}

func sum(vals []float64) float64 {
	var s float64
	for _, v := range vals {
		s += v
	}
	return s
}

func stddev(vals []float64, m float64) float64 {
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

func minmax(vals []float64) (float64, float64) {
	mn, mx := vals[0], vals[0]
	for _, v := range vals[1:] {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	return mn, mx
}

func formatRaw(v float64) string {
	if math.IsNaN(v) {
		return "."
	}
	return fmt.Sprintf("%g", v)
}
