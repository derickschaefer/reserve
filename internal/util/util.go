// Package util provides shared utilities: time parsing, observation value
// formatting, and error helpers.
//
// Rate limiting has moved to golang.org/x/time/rate (used directly in
// internal/fred). This package no longer exports a Limiter type.
package util

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ─── Date Parsing ─────────────────────────────────────────────────────────────

const dateLayout = "2006-01-02"

// ParseDate parses a YYYY-MM-DD string into a time.Time (UTC midnight).
func ParseDate(s string) (time.Time, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: expected YYYY-MM-DD", s)
	}
	return t, nil
}

// FormatDate formats a time.Time as YYYY-MM-DD.
func FormatDate(t time.Time) string {
	return t.Format(dateLayout)
}

// ─── Observation Value Parsing ────────────────────────────────────────────────

// ParseObsValue parses a FRED observation value string.
// Returns NaN for missing values ("." or empty string).
func ParseObsValue(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "." {
		return math.NaN()
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

// FormatValue formats a float64 for display, showing "." for NaN.
func FormatValue(v float64) string {
	if math.IsNaN(v) {
		return "."
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ─── Error Helpers ────────────────────────────────────────────────────────────

// MultiError collects multiple errors and presents them as one.
type MultiError struct {
	Errors []error
}

func (m *MultiError) Error() string {
	if len(m.Errors) == 0 {
		return "no errors"
	}
	msgs := make([]string, len(m.Errors))
	for i, e := range m.Errors {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// Add appends an error to the collection (nil errors are ignored).
func (m *MultiError) Add(err error) {
	if err != nil {
		m.Errors = append(m.Errors, err)
	}
}

// Err returns nil if there are no errors, otherwise returns the MultiError itself.
func (m *MultiError) Err() error {
	if len(m.Errors) == 0 {
		return nil
	}
	return m
}
