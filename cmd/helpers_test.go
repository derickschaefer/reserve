// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
)

func TestOutputWriterDefault(t *testing.T) {
	globalFlags.Out = ""
	w, closeFn, err := outputWriter(os.Stdout)
	if err != nil {
		t.Fatalf("outputWriter default: %v", err)
	}
	if w != os.Stdout {
		t.Fatalf("expected stdout writer passthrough")
	}
	if err := closeFn(); err != nil {
		t.Fatalf("default closer should be nil error, got: %v", err)
	}
}

func TestOutputWriterFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.txt")
	globalFlags.Out = p
	t.Cleanup(func() { globalFlags.Out = "" })

	w, closeFn, err := outputWriter(os.Stdout)
	if err != nil {
		t.Fatalf("outputWriter file: %v", err)
	}
	if w == os.Stdout {
		t.Fatalf("expected file writer, got stdout")
	}
	if err := closeFn(); err != nil {
		t.Fatalf("closing output writer: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
}

func TestParseIntIDAllowsZero(t *testing.T) {
	got, err := parseIntID("0", "release ID")
	if err != nil {
		t.Fatalf("expected zero to be valid, got error: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected parsed zero, got %d", got)
	}
}

func TestResolveSeriesIDsUsesAliases(t *testing.T) {
	deps := &app.Deps{Config: &config.Config{
		SeriesAliases: map[string]config.Alias{
			"pce-services": {SeriesID: "PB0000031Q225SBEA"},
			"cpi":          {SeriesID: "CPIAUCSL"},
		},
	}}

	got := resolveSeriesIDs(deps, []string{"pce-services", "GDP", "cpi", "GDP"})
	want := []string{"PB0000031Q225SBEA", "GDP", "CPIAUCSL"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("id[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

type testObsSource struct {
	started chan string
	release chan struct{}
	resp    map[string]testObsResponse
	active  int32
	peak    int32
}

type testObsResponse struct {
	data  *model.SeriesData
	cache bool
	warn  []string
	err   error
}

func (s *testObsSource) name() string         { return "test" }
func (s *testObsSource) requiresAPIKey() bool { return false }
func (s *testObsSource) get(_ context.Context, _ *app.Deps, id string, _ fred.ObsOptions) (*model.SeriesData, bool, []string, error) {
	cur := atomic.AddInt32(&s.active, 1)
	for {
		peak := atomic.LoadInt32(&s.peak)
		if cur <= peak || atomic.CompareAndSwapInt32(&s.peak, peak, cur) {
			break
		}
	}
	s.started <- id
	<-s.release
	atomic.AddInt32(&s.active, -1)
	r := s.resp[id]
	return r.data, r.cache, r.warn, r.err
}

func TestBatchGetObsSynctestConcurrencyAndOrdering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ids := []string{"A", "B", "C", "D"}
		src := &testObsSource{
			started: make(chan string, len(ids)),
			release: make(chan struct{}, len(ids)),
			resp: map[string]testObsResponse{
				"A": {data: &model.SeriesData{SeriesID: "A"}, cache: false},
				"B": {err: fmt.Errorf("boom")},
				"C": {data: &model.SeriesData{SeriesID: "C"}, cache: true, warn: []string{"C had a soft warning"}},
				"D": {data: &model.SeriesData{SeriesID: "D"}, cache: false},
			},
		}
		deps := &app.Deps{Config: &config.Config{Concurrency: 2}}

		type out struct {
			datas    []*model.SeriesData
			warnings []string
			anyCache bool
		}
		done := make(chan out, 1)
		go func() {
			datas, warnings, anyCache := batchGetObs(context.Background(), deps, ids, fred.ObsOptions{}, src)
			done <- out{datas: datas, warnings: warnings, anyCache: anyCache}
		}()

		// First wave is capped by configured concurrency.
		synctest.Wait()
		if got := len(src.started); got != 2 {
			t.Fatalf("first wave started = %d, want 2", got)
		}

		// Release first wave, then second wave should start.
		src.release <- struct{}{}
		src.release <- struct{}{}
		synctest.Wait()
		if got := len(src.started); got != 4 {
			t.Fatalf("after second wave started = %d, want 4", got)
		}

		// Release remaining workers and collect output.
		src.release <- struct{}{}
		src.release <- struct{}{}
		synctest.Wait()
		got := <-done

		if p := atomic.LoadInt32(&src.peak); p > 2 {
			t.Fatalf("peak concurrency = %d, want <= 2", p)
		}
		if !got.anyCache {
			t.Fatalf("anyCache = false, want true")
		}
		if len(got.datas) != 3 {
			t.Fatalf("datas len = %d, want 3", len(got.datas))
		}
		wantOrder := []string{"A", "C", "D"} // B errored; remaining preserve input order
		for i, want := range wantOrder {
			if got.datas[i].SeriesID != want {
				t.Fatalf("datas[%d] = %q, want %q", i, got.datas[i].SeriesID, want)
			}
		}

		if len(got.warnings) != 2 {
			t.Fatalf("warnings len = %d, want 2 (%v)", len(got.warnings), got.warnings)
		}
		warnings := append([]string(nil), got.warnings...)
		sort.Strings(warnings)
		joined := strings.Join(warnings, " | ")
		if !strings.Contains(joined, "B: boom") {
			t.Fatalf("expected error warning for B, got %v", got.warnings)
		}
		if !strings.Contains(joined, "C had a soft warning") {
			t.Fatalf("expected source warning for C, got %v", got.warnings)
		}
	})
}

func TestBatchGetObsSynctestDefaultConcurrencyFallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ids := []string{"S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8", "S9"}
		resp := make(map[string]testObsResponse, len(ids))
		for _, id := range ids {
			resp[id] = testObsResponse{data: &model.SeriesData{SeriesID: id}}
		}
		src := &testObsSource{
			started: make(chan string, len(ids)),
			release: make(chan struct{}, len(ids)),
			resp:    resp,
		}
		deps := &app.Deps{Config: &config.Config{Concurrency: 0}} // should fallback to 8

		done := make(chan struct{}, 1)
		go func() {
			batchGetObs(context.Background(), deps, ids, fred.ObsOptions{}, src)
			done <- struct{}{}
		}()

		synctest.Wait()
		if got := len(src.started); got != 8 {
			t.Fatalf("started with fallback concurrency = %d, want 8", got)
		}

		for range ids {
			src.release <- struct{}{}
		}
		synctest.Wait()
		<-done
	})
}

func TestBatchGetSeriesSynctestConcurrencyOrderingAndWarnings(t *testing.T) {
	orig := seriesComplianceLookup
	t.Cleanup(func() { seriesComplianceLookup = orig })

	synctest.Test(t, func(t *testing.T) {
		ids := []string{"S1", "S2", "S3", "S4"}

		started := make(chan string, len(ids))
		release := make(chan struct{}, len(ids))
		var active int32
		var peak int32

		seriesComplianceLookup = func(_ context.Context, _ *app.Deps, id, _ string) (model.SeriesMeta, error) {
			cur := atomic.AddInt32(&active, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
					break
				}
			}
			started <- id
			<-release
			atomic.AddInt32(&active, -1)
			if id == "S2" {
				return model.SeriesMeta{}, fmt.Errorf("missing metadata")
			}
			return model.SeriesMeta{ID: id, Title: "Title " + id}, nil
		}

		deps := &app.Deps{Config: &config.Config{Concurrency: 2}}
		type out struct {
			metas    []model.SeriesMeta
			warnings []string
		}
		done := make(chan out, 1)
		go func() {
			metas, warnings := batchGetSeries(context.Background(), deps, ids)
			done <- out{metas: metas, warnings: warnings}
		}()

		synctest.Wait()
		if got := len(started); got != 2 {
			t.Fatalf("first wave started = %d, want 2", got)
		}

		release <- struct{}{}
		release <- struct{}{}
		synctest.Wait()
		if got := len(started); got != 4 {
			t.Fatalf("second wave started = %d, want 4", got)
		}

		release <- struct{}{}
		release <- struct{}{}
		synctest.Wait()
		got := <-done

		if p := atomic.LoadInt32(&peak); p > 2 {
			t.Fatalf("peak concurrency = %d, want <= 2", p)
		}
		if len(got.metas) != 3 {
			t.Fatalf("metas len = %d, want 3", len(got.metas))
		}
		wantMetaOrder := []string{"S1", "S3", "S4"}
		for i, want := range wantMetaOrder {
			if got.metas[i].ID != want {
				t.Fatalf("metas[%d].ID = %q, want %q", i, got.metas[i].ID, want)
			}
		}
		if len(got.warnings) != 1 {
			t.Fatalf("warnings len = %d, want 1 (%v)", len(got.warnings), got.warnings)
		}
		if !strings.Contains(got.warnings[0], "S2:") || !strings.Contains(got.warnings[0], "missing metadata") {
			t.Fatalf("unexpected warning: %v", got.warnings)
		}
	})
}

func TestBatchGetSeriesSynctestDefaultConcurrencyFallback(t *testing.T) {
	orig := seriesComplianceLookup
	t.Cleanup(func() { seriesComplianceLookup = orig })

	synctest.Test(t, func(t *testing.T) {
		ids := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"}
		started := make(chan string, len(ids))
		release := make(chan struct{}, len(ids))

		seriesComplianceLookup = func(_ context.Context, _ *app.Deps, id, _ string) (model.SeriesMeta, error) {
			started <- id
			<-release
			return model.SeriesMeta{ID: id}, nil
		}

		deps := &app.Deps{Config: &config.Config{Concurrency: 0}} // fallback to 8
		done := make(chan struct{}, 1)
		go func() {
			batchGetSeries(context.Background(), deps, ids)
			done <- struct{}{}
		}()

		synctest.Wait()
		if got := len(started); got != 8 {
			t.Fatalf("started with fallback concurrency = %d, want 8", got)
		}

		for range ids {
			release <- struct{}{}
		}
		synctest.Wait()
		<-done
	})
}
