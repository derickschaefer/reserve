package cmd

import (
	"math"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
)

func TestLatestRowRoundTripPreservesNumericValue(t *testing.T) {
	src := &model.Observation{
		Date:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		Value:    4.25,
		ValueRaw: "4.25",
	}

	row := latestRowFromObservation("UNRATE", src)
	got := latestRowToSeriesData(row)
	if got == nil || len(got.Obs) != 1 {
		t.Fatalf("unexpected series data shape: %+v", got)
	}
	if got.SeriesID != "UNRATE" {
		t.Fatalf("series id mismatch: got %q", got.SeriesID)
	}
	if got.Obs[0].Value != 4.25 {
		t.Fatalf("numeric value mismatch: got %v want 4.25", got.Obs[0].Value)
	}
	if got.Obs[0].ValueRaw != "4.25" {
		t.Fatalf("raw value mismatch: got %q want %q", got.Obs[0].ValueRaw, "4.25")
	}
}

func TestLatestRowRoundTripPreservesNaN(t *testing.T) {
	src := &model.Observation{
		Date:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		Value:    math.NaN(),
		ValueRaw: ".",
	}
	row := latestRowFromObservation("GDP", src)
	got := latestRowToSeriesData(row)
	if got == nil || len(got.Obs) != 1 {
		t.Fatalf("unexpected series data shape: %+v", got)
	}
	if !math.IsNaN(got.Obs[0].Value) {
		t.Fatalf("expected NaN value, got %v", got.Obs[0].Value)
	}
	if got.Obs[0].ValueRaw != "." {
		t.Fatalf("raw value mismatch: got %q want .", got.Obs[0].ValueRaw)
	}
}
