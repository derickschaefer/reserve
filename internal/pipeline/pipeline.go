// Package pipeline provides helpers for reading and writing Observation
// streams via stdin/stdout in JSONL format — the canonical pipe format.
package pipeline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
)

// ReadObservations reads JSONL records from r (stdin) and returns
// the series_id and slice of Observations.
// Each line must be a JSON object with at least "date" and "value" fields.
func ReadObservations(r io.Reader) (string, []model.Observation, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var obs []model.Observation
	seriesID := ""

	type row struct {
		SeriesID string      `json:"series_id"`
		Date     string      `json:"date"`
		Value    interface{} `json:"value"`
		ValueRaw string      `json:"value_raw"`
	}

	lineNum := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNum++
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		var rec row
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return "", nil, fmt.Errorf("line %d: invalid JSON: %w", lineNum, err)
		}

		if seriesID == "" && rec.SeriesID != "" {
			seriesID = rec.SeriesID
		}

		date, err := time.Parse("2006-01-02", rec.Date)
		if err != nil {
			return "", nil, fmt.Errorf("line %d: invalid date %q", lineNum, rec.Date)
		}

		// Parse value: may be null (NaN), float64, or string
		var val float64
		raw := rec.ValueRaw
		switch v := rec.Value.(type) {
		case nil:
			val = math.NaN()
			if raw == "" {
				raw = "."
			}
		case float64:
			val = v
			if raw == "" {
				raw = fmt.Sprintf("%g", v)
			}
		case string:
			if v == "" || v == "." {
				val = math.NaN()
				raw = "."
			} else {
				return "", nil, fmt.Errorf("line %d: unexpected string value %q", lineNum, v)
			}
		default:
			return "", nil, fmt.Errorf("line %d: unexpected value type %T", lineNum, rec.Value)
		}

		obs = append(obs, model.Observation{
			Date:     date,
			Value:    val,
			ValueRaw: raw,
		})
	}
	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("reading input: %w", err)
	}
	if len(obs) == 0 {
		return "", nil, fmt.Errorf("no observations read from input (is stdin empty?)")
	}
	return seriesID, obs, nil
}

// WriteJSONL writes observations as JSONL to w.
func WriteJSONL(w io.Writer, seriesID string, obs []model.Observation) error {
	enc := json.NewEncoder(w)
	for _, o := range obs {
		var val interface{}
		if math.IsNaN(o.Value) {
			val = nil
		} else {
			val = o.Value
		}
		rec := map[string]interface{}{
			"series_id": seriesID,
			"date":      o.Date.Format("2006-01-02"),
			"value":     val,
			"value_raw": o.ValueRaw,
		}
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}
	return nil
}

// IsTTY returns true if stdout is a terminal (not a pipe).
func IsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
