// Package chart provides ASCII terminal chart rendering for time series data.
// Two renderers are available:
//
//   - Bar: horizontal bar chart, one bar per observation — best for low-frequency
//     or resampled series (annual, quarterly)
//   - Plot: multi-line ASCII chart with labeled axes — best for continuous series
//
// Both renderers handle NaN values gracefully (as gaps, not zeros) and require
// no external dependencies beyond the Go standard library.
package chart

import (
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/derickschaefer/reserve/internal/model"
)

// ─── Bar ─────────────────────────────────────────────────────────────────────

// BarOptions controls horizontal bar chart rendering.
type BarOptions struct {
	// Width is the total character width available for the chart.
	// If 0, auto-detects from $COLUMNS, falls back to 80.
	Width int
	// MaxBars is the maximum number of bars to render.
	// If the series has more observations than MaxBars, it is resampled
	// by taking the last value of each bucket. If 0, no limit is applied.
	MaxBars int
}

// Bar renders a horizontal bar chart of obs to w, one bar per observation.
//
// Best used with low-frequency or resampled data (annual, quarterly). For
// monthly or daily series, pipe through `transform resample` first.
//
// Output example:
//
//	UNRATE  2000–2024
//	2020-01  3.5  ████████████
//	2021-01  5.4  ████████████████████
//	2022-01  3.7  █████████████
func Bar(w io.Writer, seriesID string, obs []model.Observation, opts BarOptions) error {
	totalWidth := opts.Width
	if totalWidth <= 0 {
		totalWidth = termWidth()
	}

	// Filter to non-NaN observations
	var valid []model.Observation
	for _, o := range obs {
		if !math.IsNaN(o.Value) {
			valid = append(valid, o)
		}
	}
	if len(valid) < 1 {
		return fmt.Errorf("chart bar: no non-NaN observations to render")
	}

	// Optionally cap number of bars — take last N if over limit
	maxBars := opts.MaxBars
	if maxBars > 0 && len(valid) > maxBars {
		valid = valid[len(valid)-maxBars:]
	}

	// Warn if series looks too dense for bar chart
	if len(valid) > 60 {
		fmt.Fprintf(w, "⚠  %d observations — consider piping through: reserve transform resample --freq annual --method mean\n\n", len(valid))
	}

	// Min / max (handle negative values — bar from zero baseline)
	minVal, maxVal := valid[0].Value, valid[0].Value
	for _, o := range valid[1:] {
		if o.Value < minVal {
			minVal = o.Value
		}
		if o.Value > maxVal {
			maxVal = o.Value
		}
	}

	// Date label width — use the longest date string in the series
	dateFmt := "2006-01-02"
	if isMonthly(valid) {
		dateFmt = "2006-01"
	}
	if isAnnual(valid) {
		dateFmt = "2006"
	}
	dateWidth := len(valid[0].Date.Format(dateFmt))

	// Value label width
	valWidth := 0
	for _, o := range valid {
		if l := len(formatFloat(o.Value)); l > valWidth {
			valWidth = l
		}
	}

	// Bar area width = totalWidth - dateWidth - valWidth - separators (4 chars)
	barAreaWidth := totalWidth - dateWidth - valWidth - 4
	if barAreaWidth < 4 {
		barAreaWidth = 4
	}

	// Scale: handle negative values by finding the range and zero position
	valRange := maxVal - minVal
	if valRange == 0 {
		valRange = 1 // avoid divide-by-zero for flat series
	}

	// Determine if we have negative values — affects baseline
	hasNeg := minVal < 0
	var zeroPos int // column index of the zero line within bar area
	if hasNeg {
		zeroPos = int(math.Round((-minVal / valRange) * float64(barAreaWidth-1)))
	}

	// Header
	first := valid[0].Date.Format(dateFmt)
	last := valid[len(valid)-1].Date.Format(dateFmt)
	fmt.Fprintf(w, "%s  %s – %s\n", seriesID, first, last)

	// Render each bar
	for _, o := range valid {
		dateLabel := o.Date.Format(dateFmt)
		valLabel := formatFloat(o.Value)

		var bar string
		if hasNeg {
			bar = buildBiBar(o.Value, minVal, maxVal, barAreaWidth, zeroPos)
		} else {
			barLen := int(math.Round((o.Value - minVal) / valRange * float64(barAreaWidth)))
			if barLen < 1 {
				barLen = 1 // minimum 1 block so every bar is visible
			}
			if barLen > barAreaWidth {
				barLen = barAreaWidth
			}
			bar = strings.Repeat("█", barLen)
		}

		fmt.Fprintf(w, "%-*s  %*s  %s\n",
			dateWidth, dateLabel,
			valWidth, valLabel,
			bar,
		)
	}

	return nil
}

// buildBiBar renders a bar that may extend left (negative) or right (positive)
// from a zero baseline at zeroPos within a field of width barAreaWidth.
func buildBiBar(val, minVal, maxVal float64, barAreaWidth, zeroPos int) string {
	valRange := maxVal - minVal
	buf := []rune(strings.Repeat(" ", barAreaWidth))

	// Mark zero line
	if zeroPos >= 0 && zeroPos < barAreaWidth {
		buf[zeroPos] = '│'
	}

	if val >= 0 {
		// Fill right from zeroPos
		end := zeroPos + int(math.Round(val/valRange*float64(barAreaWidth-1)))
		if end > barAreaWidth {
			end = barAreaWidth
		}
		for i := zeroPos + 1; i <= end && i < barAreaWidth; i++ {
			buf[i] = '█'
		}
	} else {
		// Fill left from zeroPos
		start := zeroPos - int(math.Round((-val)/valRange*float64(barAreaWidth-1)))
		if start < 0 {
			start = 0
		}
		for i := start; i < zeroPos && i < barAreaWidth; i++ {
			buf[i] = '█'
		}
	}

	return string(buf)
}

// isMonthly returns true if observations appear to be monthly frequency.
func isMonthly(obs []model.Observation) bool {
	if len(obs) < 2 {
		return false
	}
	// Check if day component is always 1 and months differ
	for _, o := range obs {
		if o.Date.Day() != 1 {
			return false
		}
	}
	return true
}

// isAnnual returns true if observations appear to be annual frequency.
func isAnnual(obs []model.Observation) bool {
	if len(obs) < 2 {
		return false
	}
	for i := 1; i < len(obs) && i < 5; i++ {
		months := int(obs[i].Date.Sub(obs[i-1].Date).Hours() / 24 / 28)
		if months < 10 {
			return false
		}
	}
	return true
}

// ─── Plot ─────────────────────────────────────────────────────────────────────

// PlotOptions controls multi-line ASCII plot rendering.
type PlotOptions struct {
	// Width is the total character width of the chart (including Y-axis label).
	// If 0, auto-detects from $COLUMNS, falls back to 80.
	Width int
	// Height is the number of data rows in the chart body (not counting axis labels).
	// If 0, defaults to 12.
	Height int
	// Title overrides the default title (seriesID). Empty = use seriesID.
	Title string
}

// Plot renders a multi-line ASCII chart of obs to w.
func Plot(w io.Writer, seriesID string, obs []model.Observation, opts PlotOptions) error {
	width := opts.Width
	if width <= 0 {
		width = termWidth()
	}
	height := opts.Height
	if height <= 0 {
		height = 12
	}
	title := opts.Title
	if title == "" {
		title = seriesID
	}

	// Collect valid values for scaling
	var validVals []float64
	for _, o := range obs {
		if !math.IsNaN(o.Value) {
			validVals = append(validVals, o.Value)
		}
	}
	if len(validVals) < 2 {
		return fmt.Errorf("chart plot: need at least 2 non-NaN observations (got %d)", len(validVals))
	}

	minVal, maxVal := validVals[0], validVals[0]
	for _, v := range validVals[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Y-axis label width: measure the widest tick label
	ticks := yTicks(minVal, maxVal, height)
	yLabelWidth := 0
	for _, t := range ticks {
		if l := len(formatFloat(t)); l > yLabelWidth {
			yLabelWidth = l
		}
	}
	yAxisWidth := yLabelWidth + 2 // label + " ┤" or " ┼"

	// Plot body width (number of data columns)
	plotWidth := width - yAxisWidth
	if plotWidth < 10 {
		plotWidth = 10
	}

	// Sample obs into plotWidth columns, each column = one cell
	cols := sampleCols(obs, plotWidth)

	// Build the grid: grid[row][col] = true means draw a character here
	// row 0 = top (maxVal), row height-1 = bottom (minVal)
	grid := buildGrid(cols, minVal, maxVal, height)

	// Print title + date range header
	dateFirst := obs[0].Date.Format("2006-01")
	dateLast := obs[len(obs)-1].Date.Format("2006-01")
	fmt.Fprintf(w, "%s  (%s to %s)\n", title, dateFirst, dateLast)

	// Print rows top to bottom
	for row := 0; row < height; row++ {
		// Y-axis label: print on rows that have a tick
		label := ""
		for _, t := range ticks {
			if math.Abs(rowForValue(t, minVal, maxVal, height)-float64(row)) < 0.5 {
				label = formatFloat(t)
				break
			}
		}
		labelPadded := fmt.Sprintf("%*s", yLabelWidth, label)

		// Axis character
		axisCh := "┤"
		if label != "" && math.Abs(minVal) < 1e-9 && row == height-1 {
			axisCh = "┼"
		} else if label == "" {
			axisCh = " "
		}

		// Build the data row
		var rowSB strings.Builder
		for col := 0; col < plotWidth; col++ {
			rowSB.WriteRune(grid[row][col])
		}

		fmt.Fprintf(w, "%s%s%s\n", labelPadded, axisCh, rowSB.String())
	}

	// Bottom axis line
	bottomLine := strings.Repeat("─", plotWidth)
	fmt.Fprintf(w, "%s└%s\n", strings.Repeat(" ", yLabelWidth), bottomLine)

	// X-axis date labels: start, middle, end
	xLabels := xAxisLabels(obs, plotWidth)
	fmt.Fprintf(w, "%s %s\n", strings.Repeat(" ", yLabelWidth), xLabels)

	return nil
}

// ─── Grid building ────────────────────────────────────────────────────────────

// sampleCols reduces obs to exactly n columns by sampling.
// Each column holds the average of its bucket, or NaN if all are NaN.
func sampleCols(obs []model.Observation, n int) []float64 {
	total := len(obs)
	cols := make([]float64, n)
	for col := 0; col < n; col++ {
		lo := col * total / n
		hi := (col+1)*total/n - 1
		if hi >= total {
			hi = total - 1
		}
		sum, count := 0.0, 0
		for i := lo; i <= hi; i++ {
			if !math.IsNaN(obs[i].Value) {
				sum += obs[i].Value
				count++
			}
		}
		if count == 0 {
			cols[col] = math.NaN()
		} else {
			cols[col] = sum / float64(count)
		}
	}
	return cols
}

// rowForValue returns the float row index (0=top=max) for a given value.
func rowForValue(v, minVal, maxVal float64, height int) float64 {
	if maxVal == minVal {
		return float64(height) / 2
	}
	return (maxVal - v) / (maxVal - minVal) * float64(height-1)
}

// buildGrid renders columns into a height×width rune grid using
// box-drawing characters to connect adjacent data points.
func buildGrid(cols []float64, minVal, maxVal float64, height int) [][]rune {
	grid := make([][]rune, height)
	for r := range grid {
		grid[r] = make([]rune, len(cols))
		for c := range grid[r] {
			grid[r][c] = ' '
		}
	}

	// For each column, find the row index of its value
	rowOf := make([]int, len(cols))
	for col, v := range cols {
		if math.IsNaN(v) {
			rowOf[col] = -1 // sentinel: gap
		} else {
			r := int(math.Round(rowForValue(v, minVal, maxVal, height)))
			if r < 0 {
				r = 0
			}
			if r >= height {
				r = height - 1
			}
			rowOf[col] = r
		}
	}

	// Draw each column
	for col := 0; col < len(cols); col++ {
		r := rowOf[col]
		if r < 0 {
			continue // NaN gap
		}

		// Determine connecting characters based on neighbours
		prevRow := -2
		if col > 0 {
			prevRow = rowOf[col-1]
		}
		nextRow := -2
		if col < len(cols)-1 {
			nextRow = rowOf[col+1]
		}

		if prevRow == -2 && nextRow == -2 {
			// Isolated point
			grid[r][col] = '·'
			continue
		}

		// Horizontal run
		if (prevRow < 0 || prevRow == r) && (nextRow < 0 || nextRow == r) {
			grid[r][col] = '─'
			continue
		}

		// Transitions
		goingUp := (nextRow >= 0 && nextRow < r) || (prevRow >= 0 && prevRow < r)
		goingDown := (nextRow >= 0 && nextRow > r) || (prevRow >= 0 && prevRow > r)

		switch {
		case prevRow >= 0 && prevRow < r && nextRow >= 0 && nextRow < r:
			// Both neighbours above: flat top of a valley — shouldn't happen with smooth data
			grid[r][col] = '─'
		case prevRow >= 0 && prevRow > r && nextRow >= 0 && nextRow > r:
			// Both neighbours below: peak
			grid[r][col] = '─'
		case (prevRow < 0 || prevRow < r) && nextRow >= 0 && nextRow > r:
			grid[r][col] = '╭'
		case (prevRow < 0 || prevRow > r) && nextRow >= 0 && nextRow < r:
			grid[r][col] = '╰'
		case prevRow >= 0 && prevRow < r && (nextRow < 0 || nextRow > r):
			grid[r][col] = '╮'
		case prevRow >= 0 && prevRow > r && (nextRow < 0 || nextRow < r):
			grid[r][col] = '╯'
		default:
			if goingUp || goingDown {
				grid[r][col] = '│'
			} else {
				grid[r][col] = '─'
			}
		}

		// Fill vertical connectors between this row and previous column's row
		if prevRow >= 0 && prevRow != r {
			lo, hi := r, prevRow
			if lo > hi {
				lo, hi = hi, lo
			}
			for fill := lo + 1; fill < hi; fill++ {
				if grid[fill][col] == ' ' {
					grid[fill][col] = '│'
				}
			}
		}
	}

	return grid
}

// ─── Axis helpers ─────────────────────────────────────────────────────────────

// yTicks returns 3–5 evenly-spaced tick values for the Y axis.
func yTicks(minVal, maxVal float64, height int) []float64 {
	if maxVal == minVal {
		return []float64{minVal}
	}
	nTicks := 4
	if height <= 6 {
		nTicks = 3
	}
	ticks := make([]float64, nTicks)
	for i := 0; i < nTicks; i++ {
		ticks[i] = minVal + float64(i)*(maxVal-minVal)/float64(nTicks-1)
	}
	return ticks
}

// xAxisLabels builds a padded string with start, middle, and end date labels.
func xAxisLabels(obs []model.Observation, plotWidth int) string {
	if len(obs) == 0 {
		return ""
	}
	startLabel := obs[0].Date.Format("2006-01")
	endLabel := obs[len(obs)-1].Date.Format("2006-01")
	midLabel := obs[len(obs)/2].Date.Format("2006-01")

	// Position: start at left, mid centred, end at right
	midPos := plotWidth/2 - len(midLabel)/2
	endPos := plotWidth - len(endLabel)

	buf := []rune(strings.Repeat(" ", plotWidth))

	writeAt := func(pos int, s string) {
		for i, ch := range s {
			if pos+i >= 0 && pos+i < len(buf) {
				buf[pos+i] = ch
			}
		}
	}

	writeAt(0, startLabel)
	writeAt(midPos, midLabel)
	writeAt(endPos, endLabel)

	return string(buf)
}

// ─── Utilities ────────────────────────────────────────────────────────────────

// formatFloat formats a float for axis labels: no unnecessary trailing zeros,
// at least one decimal place, compact notation for large/small numbers.
func formatFloat(v float64) string {
	if math.IsNaN(v) {
		return "."
	}
	abs := math.Abs(v)
	var s string
	switch {
	case abs == 0:
		return "0"
	case abs >= 1e6:
		s = strconv.FormatFloat(v/1e6, 'f', 1, 64) + "M"
	case abs >= 1e3:
		s = strconv.FormatFloat(v/1e3, 'f', 1, 64) + "K"
	case abs >= 100:
		s = strconv.FormatFloat(v, 'f', 1, 64)
	case abs >= 10:
		s = strconv.FormatFloat(v, 'f', 2, 64)
	case abs >= 1:
		s = strconv.FormatFloat(v, 'f', 2, 64)
	default:
		s = strconv.FormatFloat(v, 'f', 4, 64)
	}
	// Trim trailing zeros after decimal point, keep at least one decimal
	if strings.Contains(s, ".") && !strings.Contains(s, "M") && !strings.Contains(s, "K") {
		s = strings.TrimRight(s, "0")
		if strings.HasSuffix(s, ".") {
			s += "0"
		}
	}
	return s
}

// termWidth returns the terminal width from $COLUMNS, defaulting to 80.
func termWidth() int {
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if n, err := strconv.Atoi(cols); err == nil && n > 20 {
			return n
		}
	}
	return 80
}
