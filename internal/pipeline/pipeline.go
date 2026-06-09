// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

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

// ObservationGroup holds a single grouped observation stream keyed by series ID.
type ObservationGroup struct {
	SeriesID   string
	Obs        []model.Observation
	Provenance Provenance
}

type Provenance struct {
	CitationText string   `json:"citation_text,omitempty"`
	SourceName   string   `json:"source_name,omitempty"`
	SourceNames  []string `json:"source_names,omitempty"`
}

type observationRow struct {
	SeriesID    string      `json:"series_id"`
	Date        string      `json:"date"`
	Value       interface{} `json:"value"`
	ValueRaw    string      `json:"value_raw"`
	Citation    string      `json:"citation_text"`
	SourceName  string      `json:"source_name"`
	SourceNames []string    `json:"source_names"`
}

// ReadObservations reads JSONL records from r (stdin) and returns
// the series_id and slice of Observations.
// Each line must be a JSON object with at least "date" and "value" fields.
func ReadObservations(r io.Reader) (string, []model.Observation, error) {
	seriesID, obs, _, err := readObservationsInternal(r)
	return seriesID, obs, err
}

// ReadObservationsWithCitation reads JSONL records and returns series_id,
// observations, and optional citation text when present on input rows.
func ReadObservationsWithCitation(r io.Reader) (string, []model.Observation, string, error) {
	seriesID, obs, prov, err := ReadObservationsWithProvenance(r)
	if err != nil {
		return "", nil, "", err
	}
	return seriesID, obs, prov.CitationText, nil
}

// ReadObservationsWithProvenance reads JSONL rows and returns the parsed observations
// with any carried provenance fields from the stream.
func ReadObservationsWithProvenance(r io.Reader) (string, []model.Observation, Provenance, error) {
	return readObservationsWithProvenance(r)
}

func readObservationsInternal(r io.Reader) (string, []model.Observation, string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var obs []model.Observation
	seriesID := ""
	citation := ""

	lineNum := 0
	for scanner.Scan() {
		rec, observation, skip, err := parseObservationLine(scanner.Text(), &lineNum)
		if err != nil {
			return "", nil, "", err
		}
		if skip {
			continue
		}
		if seriesID == "" && rec.SeriesID != "" {
			seriesID = rec.SeriesID
		}
		if citation == "" && strings.TrimSpace(rec.Citation) != "" {
			citation = strings.TrimSpace(rec.Citation)
		}
		obs = append(obs, observation)
	}
	if err := scanner.Err(); err != nil {
		return "", nil, "", fmt.Errorf("reading input: %w", err)
	}
	if len(obs) == 0 {
		return "", nil, "", fmt.Errorf("no observations read from input (is stdin empty?)")
	}
	return seriesID, obs, citation, nil
}

// ReadObservationGroups reads JSONL records from r and groups them by series_id.
// The returned slice preserves the order in which each distinct series first appeared.
func ReadObservationGroups(r io.Reader) ([]ObservationGroup, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	groups := make(map[string][]model.Observation)
	provenance := make(map[string]Provenance)
	order := make([]string, 0)
	lineNum := 0

	for scanner.Scan() {
		rec, observation, skip, err := parseObservationLine(scanner.Text(), &lineNum)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}

		seriesID := rec.SeriesID
		if _, ok := groups[seriesID]; !ok {
			order = append(order, seriesID)
		}
		groups[seriesID] = append(groups[seriesID], observation)
		p := provenance[seriesID]
		if p.CitationText == "" && strings.TrimSpace(rec.Citation) != "" {
			p.CitationText = strings.TrimSpace(rec.Citation)
		}
		if p.SourceName == "" && strings.TrimSpace(rec.SourceName) != "" {
			p.SourceName = strings.TrimSpace(rec.SourceName)
		}
		if len(rec.SourceNames) > 0 && len(p.SourceNames) == 0 {
			p.SourceNames = normalizeSourceNames(rec.SourceNames)
		}
		provenance[seriesID] = p
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("no observations read from input (is stdin empty?)")
	}

	out := make([]ObservationGroup, 0, len(order))
	for _, seriesID := range order {
		out = append(out, ObservationGroup{
			SeriesID:   seriesID,
			Obs:        groups[seriesID],
			Provenance: provenance[seriesID],
		})
	}
	return out, nil
}

func readObservationsWithProvenance(r io.Reader) (string, []model.Observation, Provenance, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var obs []model.Observation
	seriesID := ""
	lineNum := 0
	var prov Provenance

	for scanner.Scan() {
		rec, observation, skip, err := parseObservationLine(scanner.Text(), &lineNum)
		if err != nil {
			return "", nil, Provenance{}, err
		}
		if skip {
			continue
		}
		if seriesID == "" && rec.SeriesID != "" {
			seriesID = rec.SeriesID
		}
		if prov.CitationText == "" && strings.TrimSpace(rec.Citation) != "" {
			prov.CitationText = strings.TrimSpace(rec.Citation)
		}
		if prov.SourceName == "" && strings.TrimSpace(rec.SourceName) != "" {
			prov.SourceName = strings.TrimSpace(rec.SourceName)
		}
		if len(prov.SourceNames) == 0 && len(rec.SourceNames) > 0 {
			prov.SourceNames = normalizeSourceNames(rec.SourceNames)
		}
		obs = append(obs, observation)
	}
	if err := scanner.Err(); err != nil {
		return "", nil, Provenance{}, fmt.Errorf("reading input: %w", err)
	}
	if len(obs) == 0 {
		return "", nil, Provenance{}, fmt.Errorf("no observations read from input (is stdin empty?)")
	}
	return seriesID, obs, prov, nil
}

func normalizeSourceNames(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func parseObservationLine(line string, lineNum *int) (observationRow, model.Observation, bool, error) {
	var zeroObs model.Observation
	var rec observationRow

	line = strings.TrimSpace(line)
	*lineNum = *lineNum + 1
	if line == "" || strings.HasPrefix(line, "//") {
		return rec, zeroObs, true, nil
	}
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		return rec, zeroObs, false, fmt.Errorf("line %d: invalid JSON: %w", *lineNum, err)
	}

	date, err := time.Parse("2006-01-02", rec.Date)
	if err != nil {
		return rec, zeroObs, false, fmt.Errorf("line %d: invalid date %q", *lineNum, rec.Date)
	}

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
			return rec, zeroObs, false, fmt.Errorf("line %d: unexpected string value %q", *lineNum, v)
		}
	default:
		return rec, zeroObs, false, fmt.Errorf("line %d: unexpected value type %T", *lineNum, rec.Value)
	}

	return rec, model.Observation{
		Date:     date,
		Value:    val,
		ValueRaw: raw,
	}, false, nil
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
