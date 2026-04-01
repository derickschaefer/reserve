// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/store"
)

func TestCollectStoreWarningsOnAdditionalObsSet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reserve.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	existingKey := store.ObsKey("CPIAUCSL", "2020-01-01", "", "", "", "")
	if err := s.PutObs(existingKey, model.SeriesData{
		SeriesID: "CPIAUCSL",
		Obs: []model.Observation{
			{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Value: 1, ValueRaw: "1"},
		},
	}); err != nil {
		t.Fatalf("PutObs: %v", err)
	}

	warnings, err := collectStoreWarnings(s, map[string]model.SeriesData{
		store.ObsKey("CPIAUCSL", "2010-01-01", "", "", "", ""): {SeriesID: "CPIAUCSL"},
	})
	if err != nil {
		t.Fatalf("collectStoreWarnings: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %v", warnings)
	}
}

func TestCollectStoreWarningsNoWarningOnExactOverwrite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reserve.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	existingKey := store.ObsKey("CPIAUCSL", "2020-01-01", "", "", "", "")
	if err := s.PutObs(existingKey, model.SeriesData{
		SeriesID: "CPIAUCSL",
		Obs: []model.Observation{
			{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Value: 1, ValueRaw: "1"},
		},
	}); err != nil {
		t.Fatalf("PutObs: %v", err)
	}

	warnings, err := collectStoreWarnings(s, map[string]model.SeriesData{
		existingKey: {SeriesID: "CPIAUCSL"},
	})
	if err != nil {
		t.Fatalf("collectStoreWarnings: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}
