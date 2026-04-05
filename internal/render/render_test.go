// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
)

func TestRenderTable_CitationRequired_AppendsCitationFooter(t *testing.T) {
	result := &model.Result{
		Kind: model.KindSeriesData,
		Data: &model.SeriesData{
			SeriesID: "MORTGAGE30US",
			Meta: &model.SeriesMeta{
				ID:              "MORTGAGE30US",
				CopyrightStatus: "copyrighted_citation_required",
				CitationText:    "Source: Freddie Mac via FRED",
			},
			Obs: []model.Observation{
				{
					Date:     time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC),
					Value:    6.16,
					ValueRaw: "6.16",
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := Render(&buf, result, FormatTable); err != nil {
		t.Fatalf("Render(table): %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "MORTGAGE30US") {
		t.Fatalf("table output missing series row: %s", out)
	}
	if !strings.Contains(out, "Source: Freddie Mac via FRED") {
		t.Fatalf("table output missing citation footer: %s", out)
	}
}

func TestRenderJSON_SeriesData_IncludesRightsFields(t *testing.T) {
	result := &model.Result{
		Kind: model.KindSeriesData,
		Data: &model.SeriesData{
			SeriesID: "FEDFUNDS",
			Meta: &model.SeriesMeta{
				ID:              "FEDFUNDS",
				CopyrightStatus: "public_domain_citation_requested",
				CitationText:    "Source: Board of Governors via FRED",
			},
			Obs: []model.Observation{
				{
					Date:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Value:    3.64,
					ValueRaw: "3.64",
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := Render(&buf, result, FormatJSON); err != nil {
		t.Fatalf("Render(json): %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"copyright_status": "public_domain_citation_requested"`) {
		t.Fatalf("json output missing copyright status: %s", out)
	}
	if !strings.Contains(out, `"citation_text": "Source: Board of Governors via FRED"`) {
		t.Fatalf("json output missing citation text: %s", out)
	}
}

func TestRenderCSV_SeriesData_IncludesCitationColumn(t *testing.T) {
	result := &model.Result{
		Kind: model.KindSeriesData,
		Data: &model.SeriesData{
			SeriesID: "FEDFUNDS",
			Meta: &model.SeriesMeta{
				ID:              "FEDFUNDS",
				CopyrightStatus: "public_domain_citation_requested",
				CitationText:    "Source: Board of Governors via FRED",
			},
			Obs: []model.Observation{
				{
					Date:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Value:    3.64,
					ValueRaw: "3.64",
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := Render(&buf, result, FormatCSV); err != nil {
		t.Fatalf("Render(csv): %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "citation_text") {
		t.Fatalf("csv output missing citation column: %s", out)
	}
	if !strings.Contains(out, "Source: Board of Governors via FRED") {
		t.Fatalf("csv output missing citation text: %s", out)
	}
}
