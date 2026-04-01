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

func TestBuildCacheInventoryContiguousAndGapped(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reserve.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	metas := []model.SeriesMeta{
		{ID: "CPIAUCSL", Title: "Consumer Price Index", Frequency: "Monthly"},
		{ID: "UNRATE", Title: "Unemployment Rate", Frequency: "Monthly"},
		{ID: "PAYEMS", Title: "All Employees, Total Nonfarm", Frequency: "Monthly"},
	}
	if err := s.PutSeriesMetaBatch(metas); err != nil {
		t.Fatalf("PutSeriesMetaBatch: %v", err)
	}

	if err := s.PutObs(store.ObsKey("CPIAUCSL", "", "", "", "", ""), monthlySeries("CPIAUCSL", "2024-01-01", 3)); err != nil {
		t.Fatalf("PutObs CPIAUCSL: %v", err)
	}
	if err := s.PutObs(store.ObsKey("UNRATE", "2024-01-01", "", "", "", ""), monthlySeries("UNRATE", "2024-01-01", 2)); err != nil {
		t.Fatalf("PutObs UNRATE early: %v", err)
	}
	if err := s.PutObs(store.ObsKey("UNRATE", "2024-03-01", "", "", "", ""), monthlySeries("UNRATE", "2024-03-01", 2)); err != nil {
		t.Fatalf("PutObs UNRATE late: %v", err)
	}
	if err := s.PutObs(store.ObsKey("PAYEMS", "", "", "", "", ""), seriesWithDates("PAYEMS", []string{"2024-01-01", "2024-03-01", "2024-04-01"})); err != nil {
		t.Fatalf("PutObs PAYEMS: %v", err)
	}

	rows, err := buildCacheInventory(s)
	if err != nil {
		t.Fatalf("buildCacheInventory: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	byID := make(map[string]cacheInventoryRow, len(rows))
	for _, row := range rows {
		byID[row.SeriesID] = row
	}

	cpi := byID["CPIAUCSL"]
	if cpi.ObsSets != 1 || cpi.Points != 3 || cpi.Gaps != 0 || cpi.Coverage != "complete" {
		t.Fatalf("unexpected CPIAUCSL row: %+v", cpi)
	}

	unrate := byID["UNRATE"]
	if unrate.ObsSets != 2 || unrate.Points != 4 || unrate.Gaps != 0 || unrate.Coverage != "complete" {
		t.Fatalf("unexpected UNRATE row: %+v", unrate)
	}

	payems := byID["PAYEMS"]
	if payems.ObsSets != 1 || payems.Points != 3 || payems.Gaps != 1 || payems.Coverage != "gapped" {
		t.Fatalf("unexpected PAYEMS row: %+v", payems)
	}
}

func TestBuildCacheInventoryDailyGapsNotApplicable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reserve.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	metas := []model.SeriesMeta{
		{ID: "T10Y2Y", Title: "10-Year Treasury Constant Maturity Minus 2-Year Treasury Constant Maturity", Frequency: "Daily"},
	}
	if err := s.PutSeriesMetaBatch(metas); err != nil {
		t.Fatalf("PutSeriesMetaBatch: %v", err)
	}

	if err := s.PutObs(store.ObsKey("T10Y2Y", "", "", "", "", ""), seriesWithDates("T10Y2Y", []string{
		"2026-03-27",
		"2026-03-30",
		"2026-03-31",
	})); err != nil {
		t.Fatalf("PutObs T10Y2Y: %v", err)
	}

	rows, err := buildCacheInventory(s)
	if err != nil {
		t.Fatalf("buildCacheInventory: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row.SeriesID != "T10Y2Y" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if !row.GapsNotApplicable {
		t.Fatalf("expected daily gaps to be marked not applicable: %+v", row)
	}
	if row.Coverage != "not_applicable" {
		t.Fatalf("expected not_applicable coverage for daily row: %+v", row)
	}
}

func TestBuildCacheInventoryMissingMetadata(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reserve.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.PutObs(store.ObsKey("GDP", "", "", "", "", ""), monthlySeries("GDP", "2024-01-01", 2)); err != nil {
		t.Fatalf("PutObs: %v", err)
	}

	rows, err := buildCacheInventory(s)
	if err != nil {
		t.Fatalf("buildCacheInventory: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Meta {
		t.Fatalf("expected metadata to be missing: %+v", rows[0])
	}
	if rows[0].Frequency != "" {
		t.Fatalf("expected empty frequency without metadata: %+v", rows[0])
	}
}

func TestBuildInventoryActions(t *testing.T) {
	out := cacheInventoryOut{}
	out.Summary.MissingMetadata = 6
	out.Summary.WithGaps = 1
	out.Summary.MultipleObsSets = 2

	actions := buildInventoryActions(out)
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d: %#v", len(actions), actions)
	}
}

func TestBuildInventoryActionsHealthyStore(t *testing.T) {
	out := cacheInventoryOut{}
	actions := buildInventoryActions(out)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0] == "" {
		t.Fatalf("expected non-empty healthy action")
	}
}

func TestDisplayGapCount(t *testing.T) {
	if got := displayGapCount(cacheInventoryRow{Gaps: 3}); got != "3" {
		t.Fatalf("expected numeric gap display, got %q", got)
	}
	if got := displayGapCount(cacheInventoryRow{GapsNotApplicable: true}); got != "n/a" {
		t.Fatalf("expected n/a gap display, got %q", got)
	}
}

func monthlySeries(id, start string, count int) model.SeriesData {
	startDate, err := time.Parse("2006-01-02", start)
	if err != nil {
		panic(err)
	}
	obs := make([]model.Observation, 0, count)
	for i := 0; i < count; i++ {
		obs = append(obs, model.Observation{
			Date:     startDate.AddDate(0, i, 0),
			Value:    float64(i + 1),
			ValueRaw: "1.0",
		})
	}
	return model.SeriesData{SeriesID: id, Obs: obs}
}

func seriesWithDates(id string, dates []string) model.SeriesData {
	obs := make([]model.Observation, 0, len(dates))
	for i, raw := range dates {
		dt, err := time.Parse("2006-01-02", raw)
		if err != nil {
			panic(err)
		}
		obs = append(obs, model.Observation{
			Date:     dt,
			Value:    float64(i + 1),
			ValueRaw: "1.0",
		})
	}
	return model.SeriesData{SeriesID: id, Obs: obs}
}
