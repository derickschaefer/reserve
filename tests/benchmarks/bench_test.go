// Package benchmarks measures encoding/json performance on real FRED API
// payloads. Fixtures are pre-fetched FRED JSON responses committed to
// tests/benchmarks/fixtures/ — no network access required at benchmark time.
//
// # One-time setup
//
//	cd tests/benchmarks && FRED_API_KEY=your_key ./fetch_fixtures.sh
//
// # v1 baseline (always works)
//
//	go test ./tests/benchmarks/... -bench=. -benchmem -count=10 | tee v1.txt
//
// # v2 internals via GOEXPERIMENT (same code, v2 engine under the hood)
//
//	GOEXPERIMENT=jsonv2 go test ./tests/benchmarks/... -bench=. -benchmem -count=10 | tee v2exp.txt
//
// # explicit v2 API benchmarks + parity test (requires GOEXPERIMENT=jsonv2)
//
//	GOEXPERIMENT=jsonv2 go test ./tests/benchmarks/... -bench=. -run TestV1V2Parity -v -benchmem -count=10 | tee v2.txt
//
// # compare all three
//
//	benchstat v1.txt v2exp.txt v2.txt
package benchmarks_test

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/pipeline"
)

// ─── Fixture loading ──────────────────────────────────────────────────────────

func fixtureDir(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "fixtures")
}

// loadFixture reads a fixture file. Skips if not present — run fetch_fixtures.sh.
func loadFixture(t testing.TB, name string) []byte {
	t.Helper()
	path := filepath.Join(fixtureDir(t), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture %q not found — run fetch_fixtures.sh first: %v", name, err)
	}
	return data
}

// ─── Raw FRED API types ───────────────────────────────────────────────────────

// fredObsResponse mirrors the FRED /series/observations JSON envelope.
type fredObsResponse struct {
	ObservationStart string `json:"observation_start"`
	ObservationEnd   string `json:"observation_end"`
	Units            string `json:"units"`
	Count            int    `json:"count"`
	Observations     []struct {
		RealtimeStart string `json:"realtime_start"`
		RealtimeEnd   string `json:"realtime_end"`
		Date          string `json:"date"`
		Value         string `json:"value"`
	} `json:"observations"`
}

// fredSeriesResponse mirrors the FRED /series JSON envelope.
type fredSeriesResponse struct {
	Seriess []struct {
		ID                      string `json:"id"`
		Title                   string `json:"title"`
		ObservationStart        string `json:"observation_start"`
		ObservationEnd          string `json:"observation_end"`
		Frequency               string `json:"frequency"`
		FrequencyShort          string `json:"frequency_short"`
		Units                   string `json:"units"`
		UnitsShort              string `json:"units_short"`
		SeasonalAdjustment      string `json:"seasonal_adjustment"`
		SeasonalAdjustmentShort string `json:"seasonal_adjustment_short"`
		LastUpdated             string `json:"last_updated"`
		Popularity              int    `json:"popularity"`
		Notes                   string `json:"notes"`
	} `json:"seriess"`
}

// ─── Shared helpers (used by both bench_test.go and bench_v2_test.go) ─────────

// toSeriesData converts a raw FRED obs response to model.SeriesData.
func toSeriesData(t testing.TB, seriesID string, raw fredObsResponse) model.SeriesData {
	t.Helper()
	obs := make([]model.Observation, 0, len(raw.Observations))
	for _, o := range raw.Observations {
		date, err := time.Parse("2006-01-02", o.Date)
		if err != nil {
			continue
		}
		val := math.NaN()
		trimmed := strings.TrimSpace(o.Value)
		if trimmed != "." && trimmed != "" {
			if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
				val = f
			}
		}
		obs = append(obs, model.Observation{
			Date:          date,
			Value:         val,
			ValueRaw:      o.Value,
			RealtimeStart: o.RealtimeStart,
			RealtimeEnd:   o.RealtimeEnd,
		})
	}
	return model.SeriesData{SeriesID: seriesID, Obs: obs}
}

// loadObsFixture loads and parses an observations fixture into model.SeriesData.
func loadObsFixture(t testing.TB, fixtureName, seriesID string) model.SeriesData {
	t.Helper()
	raw := loadFixture(t, fixtureName)
	var resp fredObsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("setup: unmarshal %s fixture: %v", fixtureName, err)
	}
	sd := toSeriesData(t, seriesID, resp)
	// Only log at test level (not benchmark) to avoid per-iteration noise.
	if _, ok := t.(*testing.T); ok {
		t.Logf("%s: %d observations loaded from fixture", seriesID, len(sd.Obs))
	}
	return sd
}

// safeObsRow is a JSON-safe representation of a single observation.
// NaN values are stored as null — identical to the store layer's approach.
type safeObsRow struct {
	Date          string   `json:"date"`
	Value         *float64 `json:"value"` // null = NaN/missing
	ValueRaw      string   `json:"value_raw"`
	RealtimeStart string   `json:"realtime_start,omitempty"`
	RealtimeEnd   string   `json:"realtime_end,omitempty"`
}

// safeSeriesData is a JSON-safe envelope for SeriesData benchmark marshaling.
type safeSeriesData struct {
	SeriesID string       `json:"series_id"`
	Obs      []safeObsRow `json:"observations"`
}

// toSafeSeriesData converts model.SeriesData → safeSeriesData (NaN → null).
// This mirrors what store.PutObs does internally so the benchmark reflects
// real store write-path behavior.
func toSafeSeriesData(sd model.SeriesData) safeSeriesData {
	rows := make([]safeObsRow, len(sd.Obs))
	for i, o := range sd.Obs {
		row := safeObsRow{
			Date:          o.Date.Format("2006-01-02"),
			ValueRaw:      o.ValueRaw,
			RealtimeStart: o.RealtimeStart,
			RealtimeEnd:   o.RealtimeEnd,
		}
		if !math.IsNaN(o.Value) {
			v := o.Value
			row.Value = &v
		}
		rows[i] = row
	}
	return safeSeriesData{SeriesID: sd.SeriesID, Obs: rows}
}

// loadMetaFixtures loads all series metadata fixtures into a slice.
func loadMetaFixtures(t testing.TB) []model.SeriesMeta {
	t.Helper()
	ids := []string{"GDP", "CPIAUCSL", "UNRATE", "FEDFUNDS", "DGS10",
		"M2SL", "PAYEMS", "INDPRO", "HOUST", "RSXFS"}
	var metas []model.SeriesMeta
	for _, id := range ids {
		data := loadFixture(t, "meta_"+id)
		var resp fredSeriesResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			t.Logf("skipping meta_%s: %v", id, err)
			continue
		}
		for _, s := range resp.Seriess {
			metas = append(metas, model.SeriesMeta{
				ID:                      s.ID,
				Title:                   s.Title,
				ObservationStart:        s.ObservationStart,
				ObservationEnd:          s.ObservationEnd,
				Frequency:               s.Frequency,
				FrequencyShort:          s.FrequencyShort,
				Units:                   s.Units,
				UnitsShort:              s.UnitsShort,
				SeasonalAdjustment:      s.SeasonalAdjustment,
				SeasonalAdjustmentShort: s.SeasonalAdjustmentShort,
				LastUpdated:             s.LastUpdated,
				Popularity:              s.Popularity,
				Notes:                   s.Notes,
				FetchedAt:               time.Now(),
			})
		}
	}
	if len(metas) == 0 {
		t.Skip("no meta fixtures found — run fetch_fixtures.sh first")
	}
	return metas
}

// ─── Group 1: Unmarshal raw FRED API JSON ─────────────────────────────────────
// Decoding /series/observations HTTP response — the API client hot path.
// Run with GOEXPERIMENT=jsonv2 to see v2 engine gains on unchanged code.

func BenchmarkUnmarshalRawObs_GDP(b *testing.B) {
	data := loadFixture(b, "gdp_obs")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var resp fredObsResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalRawObs_CPIAUCSL(b *testing.B) {
	data := loadFixture(b, "cpiaucsl_obs")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var resp fredObsResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalRawObs_UNRATE(b *testing.B) {
	data := loadFixture(b, "unrate_obs")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var resp fredObsResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			b.Fatal(err)
		}
	}
}

// ─── Group 2: Marshal store envelope (store write path) ──────────────────────
// Uses safeSeriesData (NaN→null) to mirror what store.PutObs actually marshals.

func BenchmarkMarshalSeriesData_GDP(b *testing.B) {
	safe := toSafeSeriesData(loadObsFixture(b, "gdp_obs", "GDP"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(&safe)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkMarshalSeriesData_CPIAUCSL(b *testing.B) {
	safe := toSafeSeriesData(loadObsFixture(b, "cpiaucsl_obs", "CPIAUCSL"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(&safe)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

// ─── Group 3: Unmarshal store envelope (store read path) ─────────────────────

func BenchmarkUnmarshalSeriesData_CPIAUCSL(b *testing.B) {
	safe := toSafeSeriesData(loadObsFixture(b, "cpiaucsl_obs", "CPIAUCSL"))
	data, err := json.Marshal(&safe)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out safeSeriesData
		if err := json.Unmarshal(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalSeriesData_UNRATE(b *testing.B) {
	safe := toSafeSeriesData(loadObsFixture(b, "unrate_obs", "UNRATE"))
	data, err := json.Marshal(&safe)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out safeSeriesData
		if err := json.Unmarshal(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}

// ─── Group 4: JSONL pipeline round-trip ──────────────────────────────────────
// WriteJSONL → ReadObservations: hot path for every pipeline command.
// GOEXPERIMENT=jsonv2 upgrades this transparently — zero code change needed.

func BenchmarkJSONLRoundTrip_GDP(b *testing.B) {
	sd := loadObsFixture(b, "gdp_obs", "GDP")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if err := pipeline.WriteJSONL(&buf, sd.SeriesID, sd.Obs); err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(buf.Len()))
		if _, _, err := pipeline.ReadObservations(&buf); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONLRoundTrip_CPIAUCSL(b *testing.B) {
	sd := loadObsFixture(b, "cpiaucsl_obs", "CPIAUCSL")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if err := pipeline.WriteJSONL(&buf, sd.SeriesID, sd.Obs); err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(buf.Len()))
		if _, _, err := pipeline.ReadObservations(&buf); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONLRoundTrip_UNRATE(b *testing.B) {
	sd := loadObsFixture(b, "unrate_obs", "UNRATE")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if err := pipeline.WriteJSONL(&buf, sd.SeriesID, sd.Obs); err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(buf.Len()))
		if _, _, err := pipeline.ReadObservations(&buf); err != nil {
			b.Fatal(err)
		}
	}
}

// ─── Group 5: SeriesMeta batch ────────────────────────────────────────────────

func BenchmarkMarshalMetaBatch(b *testing.B) {
	metas := loadMetaFixtures(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(metas)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkUnmarshalMetaBatch(b *testing.B) {
	metas := loadMetaFixtures(b)
	data, _ := json.Marshal(metas)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out []model.SeriesMeta
		if err := json.Unmarshal(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}
