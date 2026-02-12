// Package util provides shared utilities: time parsing, hashing, errors,
// and a simple token-bucket rate limiter backed by stdlib only.
package util

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── Rate Limiter ─────────────────────────────────────────────────────────────

// Limiter is a token-bucket rate limiter.
// It allows up to Rate tokens per second, with a burst of one token.
type Limiter struct {
	mu       sync.Mutex
	rate     float64   // tokens per second
	tokens   float64   // current token count
	last     time.Time // last refill time
	maxBurst float64
}

// NewLimiter creates a Limiter that allows r events per second.
func NewLimiter(r float64) *Limiter {
	return &Limiter{
		rate:     r,
		tokens:   r, // start full
		last:     time.Now(),
		maxBurst: r, // burst = 1 second worth
	}
}

// Wait blocks until a token is available or ctx is cancelled.
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(l.last).Seconds()
		l.tokens = math.Min(l.tokens+elapsed*l.rate, l.maxBurst)
		l.last = now

		if l.tokens >= 1.0 {
			l.tokens--
			l.mu.Unlock()
			return nil
		}

		// How long until we have a token?
		wait := time.Duration((1.0-l.tokens)/l.rate*1000) * time.Millisecond
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			// retry
		}
	}
}

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
// Uses strconv.ParseFloat to avoid locale issues.
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

func (m *MultiError) Add(err error) {
	if err != nil {
		m.Errors = append(m.Errors, err)
	}
}

func (m *MultiError) Err() error {
	if len(m.Errors) == 0 {
		return nil
	}
	return m
}

func (m *MultiError) Error() string {
	msgs := make([]string, len(m.Errors))
	for i, e := range m.Errors {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}
