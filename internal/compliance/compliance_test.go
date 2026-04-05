// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package compliance_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/compliance"
	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/store"
)

func TestEnrichSeriesMetaClassifiesPreapprovalSeries(t *testing.T) {
	meta := compliance.EnrichSeriesMeta(model.SeriesMeta{ID: "ICEIDX"}, []model.Tag{
		{Name: "copyrighted: pre-approval required", GroupID: "cc"},
		{Name: "ice data indices, llc", GroupID: "src"},
	})
	if meta.CopyrightStatus != compliance.StatusCopyrightedPreapprovalRequired {
		t.Fatalf("copyright status = %q", meta.CopyrightStatus)
	}
	if meta.CitationText != "" {
		t.Fatalf("preapproval series should not auto-generate citation text, got %q", meta.CitationText)
	}
	if !meta.PermissionRequired {
		t.Fatalf("preapproval series should require permission")
	}
	if meta.UsageAllowedCommercial {
		t.Fatalf("commercial use should be blocked without permission")
	}
}

func TestEnrichSeriesMetaMarksMixedRightsTagsAmbiguous(t *testing.T) {
	meta := compliance.EnrichSeriesMeta(model.SeriesMeta{ID: "MIXED"}, []model.Tag{
		{Name: "copyrighted: citation required", GroupID: "cc"},
		{Name: "public domain: citation requested", GroupID: "cc"},
		{Name: "Bureau of Economic Analysis", GroupID: "src", Notes: "Bureau of Economic Analysis"},
	})
	if meta.CopyrightStatus != compliance.StatusAmbiguousConflict {
		t.Fatalf("copyright status = %q", meta.CopyrightStatus)
	}
	if !meta.RightsAmbiguous {
		t.Fatalf("mixed rights tags should be marked ambiguous")
	}
}

func TestEvaluateBlocksStaleUnknownRightsInCommercialMode(t *testing.T) {
	cfg := &config.Config{
		PersonOrgType:      "commercial",
		BlockUnknownRights: true,
		RightsRefreshDays:  map[string]int{"default": 30, "export": 7, "publish": 7},
	}
	decision := compliance.Evaluate(cfg, model.SeriesMeta{
		ID:              "UNK",
		CopyrightStatus: compliance.StatusUnknown,
	}, "display")
	if decision.Allowed {
		t.Fatalf("unknown rights should be blocked")
	}
}

func TestNeedsRefreshUsesActionThreshold(t *testing.T) {
	cfg := &config.Config{
		RightsRefreshDays: map[string]int{"default": 30, "export": 7, "publish": 7},
	}
	meta := model.SeriesMeta{
		ID:                "GDP",
		LastRightsCheckAt: time.Now().Add(-10 * 24 * time.Hour),
	}
	if compliance.NeedsRefresh(cfg, meta, "display", time.Now()) {
		t.Fatalf("display should still be fresh under 30-day threshold")
	}
	if !compliance.NeedsRefresh(cfg, meta, "export", time.Now()) {
		t.Fatalf("export should require refresh under 7-day threshold")
	}
}

func TestResetBackfillStateClearsMarker(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "reserve.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	if err := s.PutInternalBool("rights_backfill_completed", true); err != nil {
		t.Fatalf("PutInternalBool: %v", err)
	}
	if err := compliance.ResetBackfillState(s); err != nil {
		t.Fatalf("ResetBackfillState: %v", err)
	}
	_, found, err := s.GetInternalBool("rights_backfill_completed")
	if err != nil {
		t.Fatalf("GetInternalBool: %v", err)
	}
	if found {
		t.Fatalf("expected marker to be deleted")
	}
}

func TestEvaluateAllowsGrantedPreapprovalSeries(t *testing.T) {
	cfg := &config.Config{
		PersonOrgType:                     "student",
		AllowOverrideWithPermissionRecord: true,
	}
	decision := compliance.Evaluate(cfg, model.SeriesMeta{
		ID:               "BAMLC0A0CM",
		CopyrightStatus:  compliance.StatusCopyrightedPreapprovalRequired,
		PermissionOnFile: true,
	}, "display")
	if !decision.Allowed {
		t.Fatalf("expected granted preapproval series to be allowed")
	}
}

func TestEnrichSeriesMetaClassifiesCitationRequired(t *testing.T) {
	meta := compliance.EnrichSeriesMeta(model.SeriesMeta{ID: "MORTGAGE30US"}, []model.Tag{
		{Name: "copyrighted: citation required", GroupID: "cc"},
		{Name: "Freddie Mac", GroupID: "src", Notes: "Freddie Mac"},
	})
	if meta.CopyrightStatus != compliance.StatusCopyrightedCitationRequired {
		t.Fatalf("copyright status = %q", meta.CopyrightStatus)
	}
	if meta.CitationText != "Source: Freddie Mac via FRED" {
		t.Fatalf("citation text = %q", meta.CitationText)
	}
}

func TestEnrichSeriesMetaClassifiesPublicDomainCitationRequested(t *testing.T) {
	meta := compliance.EnrichSeriesMeta(model.SeriesMeta{ID: "FEDFUNDS"}, []model.Tag{
		{Name: "public domain: citation requested", GroupID: "cc"},
		{Name: "Board of Governors", GroupID: "src", Notes: "Board of Governors"},
	})
	if meta.CopyrightStatus != compliance.StatusPublicDomainCitationRequested {
		t.Fatalf("copyright status = %q", meta.CopyrightStatus)
	}
	if meta.CitationText != "Source: Board of Governors via FRED" {
		t.Fatalf("citation text = %q", meta.CitationText)
	}
}

func TestEvaluateBlocksPreapprovalSeriesWithoutGrant(t *testing.T) {
	cfg := &config.Config{
		PersonOrgType:                     "student",
		AllowOverrideWithPermissionRecord: true,
	}
	decision := compliance.Evaluate(cfg, model.SeriesMeta{
		ID:               "BAMLC0A0CM",
		CopyrightStatus:  compliance.StatusCopyrightedPreapprovalRequired,
		PermissionOnFile: false,
	}, "display")
	if decision.Allowed {
		t.Fatalf("expected preapproval series without grant to be blocked")
	}
}

func TestEvaluateBlocksAmbiguousRightsWhenConfigured(t *testing.T) {
	cfg := &config.Config{
		PersonOrgType:          "student",
		BlockAmbiguousRights:   true,
		RightsRefreshDays:      map[string]int{"default": 30, "export": 7, "publish": 7},
		LogComplianceDecisions: true,
	}
	decision := compliance.Evaluate(cfg, model.SeriesMeta{
		ID:              "00XALCEZ17M086NEST",
		CopyrightStatus: compliance.StatusAmbiguousConflict,
		RightsAmbiguous: true,
	}, "display")
	if decision.Allowed {
		t.Fatalf("expected ambiguous rights to be blocked")
	}
}
