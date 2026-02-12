// Package model defines the canonical data types used throughout reserve.
// These types are the single source of truth for all FRED API entities and
// the result envelope that every command returns.
package model

import (
	"math"
	"time"
)

// ─── FRED Entity Types ────────────────────────────────────────────────────────

// Category represents a FRED data category node in the hierarchy.
type Category struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ParentID int    `json:"parent_id"`
}

// SeriesMeta holds metadata for a single FRED data series.
type SeriesMeta struct {
	ID                     string    `json:"id"`
	Title                  string    `json:"title"`
	ObservationStart       string    `json:"observation_start"`
	ObservationEnd         string    `json:"observation_end"`
	Frequency              string    `json:"frequency"`
	FrequencyShort         string    `json:"frequency_short"`
	Units                  string    `json:"units"`
	UnitsShort             string    `json:"units_short"`
	SeasonalAdjustment     string    `json:"seasonal_adjustment"`
	SeasonalAdjustmentShort string   `json:"seasonal_adjustment_short"`
	LastUpdated            string    `json:"last_updated"`
	Popularity             int       `json:"popularity"`
	Notes                  string    `json:"notes"`
	FetchedAt              time.Time `json:"fetched_at,omitempty"`
}

// Release represents a FRED data release.
type Release struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	PressRelease  bool   `json:"press_release"`
	Link          string `json:"link"`
	Notes         string `json:"notes"`
}

// Source represents a FRED data source (institution).
type Source struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Link  string `json:"link"`
	Notes string `json:"notes"`
}

// Tag represents a FRED tag that can be applied to series.
type Tag struct {
	Name       string    `json:"name"`
	GroupID    string    `json:"group_id"`
	Notes      string    `json:"notes"`
	Created    string    `json:"created"`
	Popularity int       `json:"popularity"`
	SeriesCount int      `json:"series_count"`
}

// ─── Time Series Types ────────────────────────────────────────────────────────

// Observation is a single data point in a time series.
// Value is NaN when the raw value is "." or empty (missing data).
// ValueRaw preserves the original string from the API response.
type Observation struct {
	Date          time.Time `json:"date"`
	Value         float64   `json:"value"`
	ValueRaw      string    `json:"value_raw"`
	RealtimeStart string    `json:"realtime_start,omitempty"`
	RealtimeEnd   string    `json:"realtime_end,omitempty"`
}

// IsMissing returns true if the observation value is NaN (missing data).
func (o Observation) IsMissing() bool {
	return math.IsNaN(o.Value)
}

// SeriesData bundles observations with optional metadata for a single series.
type SeriesData struct {
	SeriesID string      `json:"series_id"`
	Meta     *SeriesMeta `json:"meta,omitempty"`
	Obs      []Observation `json:"observations"`
}

// ─── Result Envelope ─────────────────────────────────────────────────────────

// ResultStats carries performance and cache metadata for a command result.
type ResultStats struct {
	CacheHit   bool  `json:"cache_hit"`
	DurationMs int64 `json:"duration_ms"`
	Items      int   `json:"items"`
}

// Result is the uniform envelope returned by every command.
// The Data field holds the typed payload; Kind identifies what is in it.
// Renderers switch on Kind to format output appropriately.
type Result struct {
	Kind        string      `json:"kind"`
	GeneratedAt time.Time   `json:"generated_at"`
	Command     string      `json:"command"`
	Data        interface{} `json:"data"`
	Warnings    []string    `json:"warnings,omitempty"`
	Stats       ResultStats `json:"stats"`
}

// Kind constants for Result.Kind.
const (
	KindSeriesMeta  = "series_meta"
	KindSeriesData  = "series_data"
	KindCategory    = "category"
	KindRelease     = "release"
	KindSource      = "source"
	KindTag         = "tag"
	KindTable       = "table"
	KindReport      = "report"
	KindSearchResult = "search_result"
)

// SearchResult holds mixed-type results from a global search query.
type SearchResult struct {
	Query   string        `json:"query"`
	Type    string        `json:"type"`
	Series  []SeriesMeta  `json:"series,omitempty"`
}
