// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/store"
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

func TestResolveObsSourceDefaultIsLive(t *testing.T) {
	src, err := resolveObsSource("")
	if err != nil {
		t.Fatalf("resolveObsSource default: %v", err)
	}
	if src.name() != "live" {
		t.Fatalf("default source = %q, want live", src.name())
	}
}

func TestResolveObsSourceRejectsUnknown(t *testing.T) {
	if _, err := resolveObsSource("bogus"); err == nil {
		t.Fatalf("expected unknown source error")
	}
}

func TestResolveObsSourceRecognisesPlannedButUnconfiguredSources(t *testing.T) {
	if _, err := resolveObsSource("snowflake"); err == nil || err.Error() != "source 'snowflake' not configured" {
		t.Fatalf("unexpected snowflake resolution error: %v", err)
	}
	if _, err := resolveObsSource("bogus"); err == nil || err.Error() != "unknown source: bogus" {
		t.Fatalf("unexpected bogus resolution error: %v", err)
	}
}

func TestValidateObsSourceConfigSkipsAPIKeyForCache(t *testing.T) {
	deps := &app.Deps{Config: &config.Config{DBPath: "/tmp/reserve.db"}}
	src, err := resolveObsSource("cache")
	if err != nil {
		t.Fatalf("resolveObsSource(cache): %v", err)
	}
	if err := validateObsSourceConfig(deps, src); err != nil {
		t.Fatalf("cache source should not require API key: %v", err)
	}
}

func TestValidateObsSourceConfigRequiresAPIKeyForLive(t *testing.T) {
	deps := &app.Deps{Config: &config.Config{}}
	src, err := resolveObsSource("live")
	if err != nil {
		t.Fatalf("resolveObsSource(live): %v", err)
	}
	if err := validateObsSourceConfig(deps, src); err == nil {
		t.Fatalf("live source should require API key")
	}
}

func TestCacheObsSourceGetsStoredData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reserve.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	data := model.SeriesData{
		SeriesID: "GDP",
		Obs: []model.Observation{{
			Date:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Value:    100,
			ValueRaw: "100",
		}},
	}
	key := store.ObsKey("GDP", "2024-01-01", "2024-12-31", "", "", "")
	if err := s.PutObs(key, data); err != nil {
		t.Fatalf("PutObs: %v", err)
	}
	if err := s.PutSeriesMeta(model.SeriesMeta{ID: "GDP", Title: "Gross Domestic Product"}); err != nil {
		t.Fatalf("PutSeriesMeta: %v", err)
	}

	deps := &app.Deps{
		Config: &config.Config{DBPath: dbPath},
		Store:  s,
	}

	src, err := resolveObsSource("cache")
	if err != nil {
		t.Fatalf("resolveObsSource(cache): %v", err)
	}
	got, cacheHit, err := src.get(t.Context(), deps, "GDP", fred.ObsOptions{
		Start: "2024-01-01",
		End:   "2024-12-31",
	})
	if err != nil {
		t.Fatalf("cache source get: %v", err)
	}
	if !cacheHit {
		t.Fatalf("expected cacheHit=true")
	}
	if got == nil || got.SeriesID != "GDP" {
		t.Fatalf("unexpected series data: %+v", got)
	}
	if got.Meta == nil || got.Meta.Title != "Gross Domestic Product" {
		t.Fatalf("expected attached metadata, got %+v", got.Meta)
	}
	if len(got.Obs) != 1 || got.Obs[0].Value != 100 {
		t.Fatalf("unexpected observations: %+v", got.Obs)
	}
}
