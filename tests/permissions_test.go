// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package tests

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/compliance"
	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
)

func TestPermissionsCompliance(t *testing.T) {
	cfg := configOrSkip(t)
	client := newPermissionsClient(cfg)

	printBanner(t, "PERMISSIONS & RIGHTS COMPLIANCE")
	r := &result{}

	classCases := []struct {
		seriesID string
		want     string
	}{
		{"BAMLC0A0CM", compliance.StatusCopyrightedPreapprovalRequired},
		{"MORTGAGE30US", compliance.StatusCopyrightedCitationRequired},
		{"FEDFUNDS", compliance.StatusPublicDomainCitationRequested},
		{"00XALCEZ17M086NEST", compliance.StatusAmbiguousConflict},
	}

	for _, tc := range classCases {
		meta, err := client.GetSeries(context.Background(), tc.seriesID)
		if err != nil {
			permFail(r, t, fmt.Sprintf("GetSeries(%s) returned metadata", tc.seriesID), err.Error())
			continue
		}
		tags, err := client.GetSeriesTags(context.Background(), tc.seriesID)
		if err != nil {
			permFail(r, t, fmt.Sprintf("GetSeriesTags(%s) returned tags", tc.seriesID), err.Error())
			continue
		}
		enriched := compliance.EnrichSeriesMeta(*meta, tags)
		permCheck(r, t,
			enriched.CopyrightStatus == tc.want,
			permLabel(tc.seriesID, fmt.Sprintf("classified as %s", tc.want)),
			permLabel(tc.seriesID, "classification mismatch"),
			fmt.Sprintf("got %q want %q", enriched.CopyrightStatus, tc.want),
		)
	}

	renderCases := []struct {
		seriesID     string
		start        string
		end          string
		wantCitation string
	}{
		{
			seriesID:     "MORTGAGE30US",
			start:        "2026-01-01",
			end:          "2026-02-01",
			wantCitation: "Source: Freddie Mac via FRED",
		},
		{
			seriesID:     "FEDFUNDS",
			start:        "2026-01-01",
			end:          "2026-02-01",
			wantCitation: "Source: Board of Governors via FRED",
		},
	}

	for _, tc := range renderCases {
		resultData, err := fetchSeriesDataResult(client, tc.seriesID, tc.start, tc.end)
		if err != nil {
			permFail(r, t, fmt.Sprintf("%s fetched observations for render checks", tc.seriesID), err.Error())
			continue
		}

		var tableBuf bytes.Buffer
		err = render.Render(&tableBuf, resultData, render.FormatTable)
		permCheck(r, t,
			err == nil && strings.Contains(tableBuf.String(), tc.wantCitation),
			permLabel(tc.seriesID, "table output includes citation footer"),
			permLabel(tc.seriesID, "table output missing citation"),
			tableBuf.String(),
		)

		var jsonBuf bytes.Buffer
		err = render.Render(&jsonBuf, resultData, render.FormatJSON)
		permCheck(r, t,
			err == nil && strings.Contains(jsonBuf.String(), tc.wantCitation),
			permLabel(tc.seriesID, "json output includes citation text"),
			permLabel(tc.seriesID, "json output missing citation"),
			jsonBuf.String(),
		)

		var csvBuf bytes.Buffer
		err = render.Render(&csvBuf, resultData, render.FormatCSV)
		permCheck(r, t,
			err == nil && strings.Contains(csvBuf.String(), "citation_text") && strings.Contains(csvBuf.String(), tc.wantCitation),
			permLabel(tc.seriesID, "csv output includes citation column"),
			permLabel(tc.seriesID, "csv output missing citation"),
			csvBuf.String(),
		)
	}

	preMeta, err := client.GetSeries(context.Background(), "BAMLC0A0CM")
	if err != nil {
		permFail(r, t, "BAMLC0A0CM metadata fetched for block check", err.Error())
	} else if preTags, tagsErr := client.GetSeriesTags(context.Background(), "BAMLC0A0CM"); tagsErr != nil {
		permFail(r, t, "BAMLC0A0CM tags fetched for block check", tagsErr.Error())
	} else {
		enriched := compliance.EnrichSeriesMeta(*preMeta, preTags)
		localCfg := *cfg
		localCfg.AllowOverrideWithPermissionRecord = true
		localCfg.GrantedSeriesPermissions = nil
		decision := compliance.Evaluate(&localCfg, enriched, "display")
		permCheck(r, t,
			!decision.Allowed,
			permLabel("BAMLC0A0CM", "is blocked without a local grant"),
			permLabel("BAMLC0A0CM", "should be blocked without a local grant"),
		)
	}

	ambMeta, err := client.GetSeries(context.Background(), "00XALCEZ17M086NEST")
	if err != nil {
		permFail(r, t, "00XALCEZ17M086NEST metadata fetched for ambiguity check", err.Error())
	} else if ambTags, tagsErr := client.GetSeriesTags(context.Background(), "00XALCEZ17M086NEST"); tagsErr != nil {
		permFail(r, t, "00XALCEZ17M086NEST tags fetched for ambiguity check", tagsErr.Error())
	} else {
		enriched := compliance.EnrichSeriesMeta(*ambMeta, ambTags)
		localCfg := *cfg
		localCfg.BlockAmbiguousRights = true
		decision := compliance.Evaluate(&localCfg, enriched, "display")
		permCheck(r, t,
			!decision.Allowed,
			permLabel("00XALCEZ17M086NEST", "is blocked for ambiguous rights"),
			permLabel("00XALCEZ17M086NEST", "should be blocked for ambiguous rights"),
		)
	}

	r.summary(t, "PERMISSIONS & RIGHTS COMPLIANCE")
}

func newPermissionsClient(cfg *config.Config) *fred.Client {
	return fred.NewClient(
		cfg.APIKey,
		cfg.BaseURL,
		15*time.Second,
		cfg.Rate,
		false,
	)
}

func fetchSeriesDataResult(client *fred.Client, seriesID, start, end string) (*model.Result, error) {
	meta, err := client.GetSeries(context.Background(), seriesID)
	if err != nil {
		return nil, fmt.Errorf("GetSeries(%s): %w", seriesID, err)
	}
	tags, err := client.GetSeriesTags(context.Background(), seriesID)
	if err != nil {
		return nil, fmt.Errorf("GetSeriesTags(%s): %w", seriesID, err)
	}
	data, err := client.GetObservations(context.Background(), seriesID, fred.ObsOptions{
		Start: start,
		End:   end,
	})
	if err != nil {
		return nil, fmt.Errorf("GetObservations(%s): %w", seriesID, err)
	}
	enriched := compliance.EnrichSeriesMeta(*meta, tags)
	data.Meta = &enriched
	return &model.Result{
		Kind: model.KindSeriesData,
		Data: data,
	}, nil
}

func permLabel(seriesID, detail string) string {
	return fmt.Sprintf("%-18s %s", seriesID, detail)
}

func permCheck(r *result, t *testing.T, condition bool, passLabel, failLabel string, detail ...string) {
	if condition {
		r.pass(t, passLabel)
		return
	}
	r.fail(t, failLabel, detail...)
}

func permFail(r *result, t *testing.T, label string, detail ...string) {
	r.fail(t, label, detail...)
}
