// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

// ============================================================================
// FILE:        tests/phase2_test.go
// PROJECT:     reserve
// DESCRIPTION: Durable CLI contract suite covering:
//
//   1. Command Surface      — durable root/help contracts for shipped commands
//   2. New API Endpoints    — mock HTTP server for category/release/source/tag
//   3. Batch Concurrency    — worker pool respects --concurrency ceiling
//   4. Partial Failures     — per-item errors collected as warnings
//   5. Category Helpers     — parseCategoryID, walkCategoryTree depth limit
// ============================================================================

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/derickschaefer/reserve/internal/store"
)

// ─────────────────────────────────────────────────────────────────────────────
// Group 5 — Command Surface
// ─────────────────────────────────────────────────────────────────────────────

func TestCommandSurface(t *testing.T) {
	printBanner(t, "COMMAND SURFACE")
	r := &result{}

	rootHelp := runReserveHelp(t, "--help")
	for _, cmdName := range []string{
		"analyze", "cache", "category", "chart", "completion", "config",
		"fetch", "meta", "obs", "onboard", "release", "search",
		"series", "source", "tag", "transform", "version", "window",
	} {
		r.check(t, strings.Contains(rootHelp, "\n  "+cmdName),
			fmt.Sprintf("root help includes [%s]", cmdName),
			fmt.Sprintf("root help is missing [%s]", cmdName),
		)
	}
	r.check(t, !strings.Contains(rootHelp, "\n  store"),
		"root help does not advertise deprecated command [store]",
		"root help still advertises deprecated command [store]",
	)

	obsGetHelp := runReserveHelp(t, "obs", "get", "--help")
	for _, token := range []string{"--from string", "live|cache", "--start string", "--end string"} {
		r.check(t, strings.Contains(obsGetHelp, token),
			fmt.Sprintf("obs get help includes [%s]", token),
			fmt.Sprintf("obs get help is missing [%s]", token),
		)
	}

	onboardHelp := runReserveHelp(t, "onboard", "--help")
	for _, token := range []string{"reserve onboard [command]", "--topic string", "reserve onboard series", "export"} {
		r.check(t, strings.Contains(onboardHelp, token),
			fmt.Sprintf("onboard help includes [%s]", token),
			fmt.Sprintf("onboard help is missing [%s]", token),
		)
	}

	seriesHelp := runReserveHelp(t, "series", "--help")
	for _, sub := range []string{"get", "search", "tags", "categories"} {
		r.check(t, strings.Contains(seriesHelp, sub),
			fmt.Sprintf("series help includes [%s]", sub),
			fmt.Sprintf("series help is missing [%s]", sub),
		)
	}
	r.summary(t, "COMMAND SURFACE")
}

// ─────────────────────────────────────────────────────────────────────────────
// Group 6 — New API Endpoints (mock HTTP server)
// ─────────────────────────────────────────────────────────────────────────────

func TestNewAPIEndpoints(t *testing.T) {
	printBanner(t, "NEW API ENDPOINTS")
	r := &result{}

	// ── Category endpoints ────────────────────────────────────────────────────
	client := newMockFREDClient(t, map[string]http.HandlerFunc{
		"/category": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"categories": []map[string]interface{}{
					{"id": 0, "name": "Categories", "parent_id": 0},
				},
			})
		},
		"/category/children": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"categories": []map[string]interface{}{
					{"id": 32991, "name": "Employment Cost Index", "parent_id": 0},
					{"id": 10, "name": "Population, Employment, & Labor Markets", "parent_id": 0},
				},
			})
		},
		"/category/series": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"seriess": []map[string]interface{}{
					{"id": "UNRATE", "title": "Unemployment Rate", "frequency": "Monthly", "frequency_short": "M",
						"units": "Percent", "units_short": "%", "popularity": 89, "last_updated": "2024-09-06"},
				},
			})
		},
	})

	cat, err := client.GetCategory(context.Background(), 0)
	r.check(t, err == nil && cat != nil && cat.ID == 0,
		"GetCategory(0): returns root category",
		fmt.Sprintf("GetCategory(0) failed: %v", err),
	)

	children, err := client.GetCategoryChildren(context.Background(), 0)
	r.check(t, err == nil && len(children) == 2,
		fmt.Sprintf("GetCategoryChildren(0): returned %d children", len(children)),
		fmt.Sprintf("GetCategoryChildren(0) failed: err=%v count=%d", err, len(children)),
	)

	catSeries, err := client.GetCategorySeries(context.Background(), 32991, fred.CategorySeriesOptions{Limit: 10})
	catSeriesIDs := make([]string, len(catSeries))
	for i, m := range catSeries {
		catSeriesIDs[i] = m.ID
	}
	r.check(t, err == nil && len(catSeries) == 1 && catSeries[0].ID == "UNRATE",
		fmt.Sprintf("GetCategorySeries(32991): returned series %v", catSeriesIDs),
		fmt.Sprintf("GetCategorySeries failed: err=%v count=%d", err, len(catSeries)),
	)

	// ── Release endpoints ─────────────────────────────────────────────────────
	relClient := newMockFREDClient(t, map[string]http.HandlerFunc{
		"/releases": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"releases": []map[string]interface{}{
					{"id": 10, "name": "Employment Situation", "press_release": true, "link": "http://www.bls.gov/news.release/empsit.nr0.htm"},
					{"id": 11, "name": "Advance Monthly Sales for Retail", "press_release": true, "link": ""},
				},
			})
		},
		"/release": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"releases": []map[string]interface{}{
					{"id": 10, "name": "Employment Situation", "press_release": true, "link": "http://www.bls.gov/news.release/empsit.nr0.htm"},
				},
			})
		},
		"/release/dates": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"release_dates": []map[string]interface{}{
					{"release_id": 10, "release_name": "Employment Situation", "date": "2024-10-04"},
					{"release_id": 10, "release_name": "Employment Situation", "date": "2024-11-01"},
				},
			})
		},
		"/release/series": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"seriess": []map[string]interface{}{
					{"id": "UNRATE", "title": "Unemployment Rate", "frequency": "Monthly", "frequency_short": "M",
						"units": "Percent", "units_short": "%", "popularity": 89, "last_updated": "2024-09-06"},
				},
			})
		},
	})

	releases, err := relClient.ListReleases(context.Background(), 0)
	r.check(t, err == nil && len(releases) == 2,
		fmt.Sprintf("ListReleases: returned %d releases", len(releases)),
		fmt.Sprintf("ListReleases failed: err=%v count=%d", err, len(releases)),
	)

	rel, err := relClient.GetRelease(context.Background(), 10)
	r.check(t, err == nil && rel != nil && rel.ID == 10 && rel.Name == "Employment Situation",
		fmt.Sprintf("GetRelease(10): name=%q", rel.Name),
		fmt.Sprintf("GetRelease(10) failed: %v", err),
	)

	dates, err := relClient.GetReleaseDates(context.Background(), 10, 5)
	r.check(t, err == nil && len(dates) == 2,
		fmt.Sprintf("GetReleaseDates(10): returned %d dates", len(dates)),
		fmt.Sprintf("GetReleaseDates(10) failed: err=%v count=%d", err, len(dates)),
	)

	relSeries, err := relClient.GetReleaseSeries(context.Background(), 10, 10)
	r.check(t, err == nil && len(relSeries) == 1,
		fmt.Sprintf("GetReleaseSeries(10): returned %d series", len(relSeries)),
		fmt.Sprintf("GetReleaseSeries(10) failed: err=%v", err),
	)

	// ── Source endpoints ──────────────────────────────────────────────────────
	srcClient := newMockFREDClient(t, map[string]http.HandlerFunc{
		"/sources": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"sources": []map[string]interface{}{
					{"id": 1, "name": "Board of Governors of the Federal Reserve System (US)", "link": "http://www.federalreserve.gov/"},
					{"id": 3, "name": "Federal Reserve Bank of Philadelphia", "link": "https://www.philadelphiafed.org/"},
				},
			})
		},
		"/source": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"sources": []map[string]interface{}{
					{"id": 1, "name": "Board of Governors of the Federal Reserve System (US)", "link": "http://www.federalreserve.gov/"},
				},
			})
		},
		"/source/releases": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"releases": []map[string]interface{}{
					{"id": 15, "name": "G.17 Industrial Production and Capacity Utilization", "press_release": false, "link": ""},
				},
			})
		},
	})

	sources, err := srcClient.ListSources(context.Background(), 0)
	r.check(t, err == nil && len(sources) == 2,
		fmt.Sprintf("ListSources: returned %d sources", len(sources)),
		fmt.Sprintf("ListSources failed: err=%v count=%d", err, len(sources)),
	)

	src, err := srcClient.GetSource(context.Background(), 1)
	r.check(t, err == nil && src != nil && src.ID == 1,
		fmt.Sprintf("GetSource(1): name=%q", src.Name),
		fmt.Sprintf("GetSource(1) failed: %v", err),
	)

	srcRels, err := srcClient.GetSourceReleases(context.Background(), 1, 0)
	r.check(t, err == nil && len(srcRels) == 1,
		fmt.Sprintf("GetSourceReleases(1): returned %d releases", len(srcRels)),
		fmt.Sprintf("GetSourceReleases(1) failed: %v", err),
	)

	// ── Tag endpoints ─────────────────────────────────────────────────────────
	tagClient := newMockFREDClient(t, map[string]http.HandlerFunc{
		"/tags": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"tags": []map[string]interface{}{
					{"name": "inflation", "group_id": "gen", "popularity": 82, "series_count": 312},
					{"name": "cpi", "group_id": "gen", "popularity": 78, "series_count": 145},
				},
			})
		},
		"/tags/series": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"seriess": []map[string]interface{}{
					{"id": "CPIAUCSL", "title": "Consumer Price Index", "frequency": "Monthly", "frequency_short": "M",
						"units": "Index", "units_short": "Idx", "popularity": 88, "last_updated": "2024-09-11"},
				},
			})
		},
		"/related_tags": func(w http.ResponseWriter, req *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"tags": []map[string]interface{}{
					{"name": "price index", "group_id": "gen", "popularity": 75, "series_count": 200},
				},
			})
		},
	})

	tags, err := tagClient.SearchTags(context.Background(), "inflation", fred.SearchTagsOptions{Limit: 10})
	r.check(t, err == nil && len(tags) == 2 && tags[0].Name == "inflation",
		fmt.Sprintf("SearchTags: returned %d tags, first=%q", len(tags), tags[0].Name),
		fmt.Sprintf("SearchTags failed: err=%v count=%d", err, len(tags)),
	)

	tagSeries, err := tagClient.GetTagSeries(context.Background(), []string{"inflation"}, fred.GetTagSeriesOptions{Limit: 10})
	r.check(t, err == nil && len(tagSeries) == 1 && tagSeries[0].ID == "CPIAUCSL",
		fmt.Sprintf("GetTagSeries: returned %d series", len(tagSeries)),
		fmt.Sprintf("GetTagSeries failed: err=%v count=%d", err, len(tagSeries)),
	)

	relatedTags, err := tagClient.GetRelatedTags(context.Background(), "inflation", 10)
	r.check(t, err == nil && len(relatedTags) == 1,
		fmt.Sprintf("GetRelatedTags: returned %d tags", len(relatedTags)),
		fmt.Sprintf("GetRelatedTags failed: err=%v", err),
	)

	r.summary(t, "NEW API ENDPOINTS")
}

// ─────────────────────────────────────────────────────────────────────────────
// Group 7 — Batch Concurrency
// ─────────────────────────────────────────────────────────────────────────────

func TestBatchConcurrency(t *testing.T) {
	printBanner(t, "BATCH CONCURRENCY")
	r := &result{}

	const concurrencyLimit = 3
	const numRequests = 9

	var activeCount int64
	var peakActive int64
	var mu sync.Mutex

	client := newMockFREDClient(t, map[string]http.HandlerFunc{"/series": func(w http.ResponseWriter, req *http.Request) {
		current := atomic.AddInt64(&activeCount, 1)
		mu.Lock()
		if current > peakActive {
			peakActive = current
		}
		mu.Unlock()

		time.Sleep(20 * time.Millisecond) // simulate latency
		atomic.AddInt64(&activeCount, -1)

		seriesID := req.URL.Query().Get("series_id")
		if seriesID == "" {
			seriesID = "UNKNOWN"
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"seriess": []map[string]interface{}{
				{"id": seriesID, "title": "Test Series", "frequency": "Monthly", "frequency_short": "M",
					"units": "Units", "units_short": "U", "popularity": 1, "last_updated": "2024-01-01"},
			},
		})
	}})

	// Build IDs
	ids := make([]string, numRequests)
	for i := 0; i < numRequests; i++ {
		ids[i] = fmt.Sprintf("SERIES%02d", i+1)
	}

	// Worker pool (mirrors batchGetSeries logic)
	type res struct {
		meta model.SeriesMeta
		err  error
	}
	results := make([]res, numRequests)
	sem := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup

	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			meta, err := client.GetSeries(context.Background(), id)
			if err != nil {
				results[i] = res{err: err}
				return
			}
			results[i] = res{meta: *meta}
		}()
	}
	wg.Wait()

	// Count successes
	successes := 0
	for _, res := range results {
		if res.err == nil {
			successes++
		}
	}

	r.check(t, successes == numRequests,
		fmt.Sprintf("All %d requests completed successfully", numRequests),
		fmt.Sprintf("Only %d/%d requests succeeded", successes, numRequests),
	)

	r.check(t, peakActive <= int64(concurrencyLimit),
		fmt.Sprintf("Peak concurrent requests (%d) did not exceed limit (%d)", peakActive, concurrencyLimit),
		fmt.Sprintf("Concurrency limit VIOLATED: peak=%d limit=%d", peakActive, concurrencyLimit),
	)

	r.check(t, peakActive > 1,
		fmt.Sprintf("Worker pool actually parallelised (peak=%d > 1)", peakActive),
		"Worker pool ran sequentially (no concurrency benefit)",
	)

	r.summary(t, "BATCH CONCURRENCY")
}

// ─────────────────────────────────────────────────────────────────────────────
// Group 8 — Partial Failure / Warnings
// ─────────────────────────────────────────────────────────────────────────────

func TestPartialFailureWarnings(t *testing.T) {
	printBanner(t, "PARTIAL FAILURE / WARNINGS")
	r := &result{}

	// Server that returns 200 for SERIES01 and 400 for all others
	client := newMockFREDClient(t, map[string]http.HandlerFunc{"/series": func(w http.ResponseWriter, req *http.Request) {
		id := req.URL.Query().Get("series_id")
		if id == "SERIES01" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"seriess": []map[string]interface{}{
					{"id": "SERIES01", "title": "Good Series", "frequency": "Monthly", "frequency_short": "M",
						"units": "Units", "units_short": "U", "popularity": 50, "last_updated": "2024-01-01"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error_message": "Series does not exist."})
	}})
	ids := []string{"SERIES01", "BADFOO", "BADBAR"}

	// Simulate batchGetSeries pattern
	type result2 struct {
		meta model.SeriesMeta
		err  error
		idx  int
	}
	res2 := make([]result2, len(ids))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			meta, err := client.GetSeries(context.Background(), id)
			if err != nil {
				res2[i] = result2{idx: i, err: err}
				return
			}
			res2[i] = result2{idx: i, meta: *meta}
		}()
	}
	wg.Wait()

	var metas []model.SeriesMeta
	var warnings []string
	for i, r2 := range res2 {
		if r2.err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", ids[i], r2.err))
		} else {
			metas = append(metas, r2.meta)
		}
	}

	r.check(t, len(metas) == 1 && metas[0].ID == "SERIES01",
		fmt.Sprintf("Partial batch: 1 successful result returned (ID=%s)", metas[0].ID),
		fmt.Sprintf("Partial batch wrong: got %d results", len(metas)),
	)

	r.check(t, len(warnings) == 2,
		fmt.Sprintf("Partial batch: 2 warnings collected for failed requests (got %d)", len(warnings)),
		fmt.Sprintf("Warning count wrong: got %d, want 2", len(warnings)),
	)

	// Verify warnings contain series IDs
	warnText := fmt.Sprintf("%v", warnings)
	r.check(t, len(warnings) > 0 && (contains(warnText, "BADFOO") || contains(warnText, "BADBAR")),
		"Warnings include the failed series IDs",
		fmt.Sprintf("Warnings don't reference failed IDs: %v", warnings),
	)

	r.summary(t, "PARTIAL FAILURE / WARNINGS")
}

// ─────────────────────────────────────────────────────────────────────────────
// Group 9 — Value Semantics (offline, no network listeners)
// ─────────────────────────────────────────────────────────────────────────────

func TestValueSemanticsOffline(t *testing.T) {
	printBanner(t, "VALUE SEMANTICS")
	r := &result{}

	// 1) Rendering keeps numeric value fidelity across machine formats.
	sd := &model.SeriesData{
		SeriesID: "UNRATE",
		Obs: []model.Observation{{
			Date:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Value:    4.25,
			ValueRaw: "4.25",
		}},
	}
	res := &model.Result{Kind: model.KindSeriesData, Data: sd}

	var jsonBuf bytes.Buffer
	_ = render.Render(&jsonBuf, res, render.FormatJSON)
	jsonOut := jsonBuf.String()
	r.check(t, containsStr(jsonOut, `"value": 4.25`),
		"JSON output preserves numeric value 4.25",
		fmt.Sprintf("JSON output missing expected numeric value: %s", jsonOut),
	)
	r.check(t, !containsStr(jsonOut, `"value": 0`),
		"JSON output does not regress to zero value",
		fmt.Sprintf("JSON output regressed to zero value: %s", jsonOut),
	)

	var jsonlBuf bytes.Buffer
	_ = render.Render(&jsonlBuf, res, render.FormatJSONL)
	jsonlOut := jsonlBuf.String()
	r.check(t, containsStr(jsonlOut, `"value":4.25`) || containsStr(jsonlOut, `"value": 4.25`),
		"JSONL output preserves numeric value 4.25",
		fmt.Sprintf("JSONL output missing expected numeric value: %s", jsonlOut),
	)

	var csvBuf bytes.Buffer
	_ = render.Render(&csvBuf, res, render.FormatCSV)
	csvOut := csvBuf.String()
	r.check(t, containsStr(csvOut, ",4.25,"),
		"CSV output preserves numeric value 4.25",
		fmt.Sprintf("CSV output missing expected numeric value: %s", csvOut),
	)

	// 2) Missing values stay null (jsonl) and '.' in delimited output.
	sdMissing := &model.SeriesData{
		SeriesID: "UNRATE",
		Obs: []model.Observation{{
			Date:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Value:    math.NaN(),
			ValueRaw: ".",
		}},
	}
	missingRes := &model.Result{Kind: model.KindSeriesData, Data: sdMissing}

	var missJSONL bytes.Buffer
	_ = render.Render(&missJSONL, missingRes, render.FormatJSONL)
	r.check(t, containsStr(missJSONL.String(), `"value":null`) || containsStr(missJSONL.String(), `"value": null`),
		"JSONL output encodes missing values as null",
		fmt.Sprintf("JSONL missing-value encoding wrong: %s", missJSONL.String()),
	)

	var missCSV bytes.Buffer
	_ = render.Render(&missCSV, missingRes, render.FormatCSV)
	r.check(t, containsStr(missCSV.String(), ",.,"),
		"CSV output encodes missing values as '.'",
		fmt.Sprintf("CSV missing-value encoding wrong: %s", missCSV.String()),
	)

	// 3) Store key lookup uses exact series boundary (GDP != GDPDEF).
	dbPath := filepath.Join(t.TempDir(), "value-semantics.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	_ = s.PutObs(store.ObsKey("GDP", "", "", "", "", ""), model.SeriesData{
		SeriesID: "GDP",
		Obs: []model.Observation{{
			Date:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Value:    1.0,
			ValueRaw: "1",
		}},
	})
	_ = s.PutObs(store.ObsKey("GDPDEF", "", "", "", "", ""), model.SeriesData{
		SeriesID: "GDPDEF",
		Obs: []model.Observation{{
			Date:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Value:    2.0,
			ValueRaw: "2",
		}},
	})

	keys, err := s.ListObsKeys("GDP")
	if err != nil {
		t.Fatalf("ListObsKeys(GDP): %v", err)
	}
	r.check(t, len(keys) == 1 && keys[0] == "series:GDP",
		"Store key lookup for GDP excludes GDPDEF",
		fmt.Sprintf("exact-series boundary failed: keys=%v", keys),
	)

	r.summary(t, "VALUE SEMANTICS")
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func runReserveHelp(t *testing.T, args ...string) string {
	t.Helper()

	cmdArgs := append([]string{"run", ".."}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = filepath.Join("..", "tests")
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "gocache"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(cmdArgs, " "), err, string(out))
	}
	return string(out)
}
