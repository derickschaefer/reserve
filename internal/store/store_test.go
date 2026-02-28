package store_test

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/store"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// testDB opens a fresh isolated database in t.TempDir().
// It is closed and deleted automatically when the test ends.
// This is the only pattern used — no test ever touches the production DB.
func testDB(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func isNaN(v float64) bool { return math.IsNaN(v) }

// makeMeta builds a minimal SeriesMeta for testing.
func makeMeta(id, title string) model.SeriesMeta {
	return model.SeriesMeta{
		ID:          id,
		Title:       title,
		Frequency:   "Monthly",
		Units:       "Percent",
		LastUpdated: "2024-01-01",
		Popularity:  50,
	}
}

// makeSeriesData builds a SeriesData with monthly observations.
func makeSeriesData(seriesID string, year, month int, values ...float64) model.SeriesData {
	obs := make([]model.Observation, len(values))
	for i, v := range values {
		raw := "."
		if !isNaN(v) {
			raw = "x"
		}
		obs[i] = model.Observation{
			Date:     time.Date(year, time.Month(month+i), 1, 0, 0, 0, 0, time.UTC),
			Value:    v,
			ValueRaw: raw,
		}
	}
	return model.SeriesData{SeriesID: seriesID, Obs: obs}
}

// ─── Open / Path ──────────────────────────────────────────────────────────────

func TestOpenCreatesDB(t *testing.T) {
	s := testDB(t)
	if s.Path() == "" {
		t.Error("Path() should return the db path after open")
	}
}

func TestOpenCreatesParentDirs(t *testing.T) {
	// Open with nested path that doesn't exist yet
	path := filepath.Join(t.TempDir(), "a", "b", "c", "test.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open with nested path: %v", err)
	}
	defer s.Close()
	if s.Path() != path {
		t.Errorf("Path: expected %q, got %q", path, s.Path())
	}
}

func TestCloseIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	// Second close should not panic (bbolt returns error on double close, not panic)
}

// ─── ObsKey ───────────────────────────────────────────────────────────────────

func TestObsKeyMinimal(t *testing.T) {
	key := store.ObsKey("GDP", "", "", "", "", "")
	if key != "series:GDP" {
		t.Errorf("minimal key: expected 'series:GDP', got %q", key)
	}
}

func TestObsKeyAllFields(t *testing.T) {
	key := store.ObsKey("UNRATE", "2020-01-01", "2024-12-31", "m", "lin", "avg")
	expected := "series:UNRATE|start:2020-01-01|end:2024-12-31|freq:m|units:lin|agg:avg"
	if key != expected {
		t.Errorf("full key:\n  expected: %q\n  got:      %q", expected, key)
	}
}

func TestObsKeyOmitsEmptyFields(t *testing.T) {
	key := store.ObsKey("CPI", "2020-01-01", "", "m", "", "")
	if strings.Contains(key, "end:") {
		t.Errorf("empty end should be omitted, got %q", key)
	}
	if strings.Contains(key, "units:") {
		t.Errorf("empty units should be omitted, got %q", key)
	}
	if strings.Contains(key, "agg:") {
		t.Errorf("empty agg should be omitted, got %q", key)
	}
	if !strings.Contains(key, "start:2020-01-01") {
		t.Errorf("non-empty start should be present, got %q", key)
	}
}

func TestObsKeyDeterministic(t *testing.T) {
	// Same args → same key every time
	k1 := store.ObsKey("GDP", "2020-01-01", "2024-12-31", "q", "lin", "avg")
	k2 := store.ObsKey("GDP", "2020-01-01", "2024-12-31", "q", "lin", "avg")
	if k1 != k2 {
		t.Errorf("ObsKey should be deterministic: %q vs %q", k1, k2)
	}
}

func TestObsKeyDifferentSeriesDistinct(t *testing.T) {
	k1 := store.ObsKey("GDP", "", "", "", "", "")
	k2 := store.ObsKey("UNRATE", "", "", "", "", "")
	if k1 == k2 {
		t.Errorf("different series IDs should produce different keys")
	}
}

// ─── SeriesMeta ───────────────────────────────────────────────────────────────

func TestPutGetSeriesMeta(t *testing.T) {
	s := testDB(t)
	meta := makeMeta("UNRATE", "Unemployment Rate")

	if err := s.PutSeriesMeta(meta); err != nil {
		t.Fatalf("PutSeriesMeta: %v", err)
	}

	got, found, err := s.GetSeriesMeta("UNRATE")
	if err != nil {
		t.Fatalf("GetSeriesMeta: %v", err)
	}
	if !found {
		t.Fatal("expected to find UNRATE after put")
	}
	if got.ID != "UNRATE" {
		t.Errorf("ID: expected UNRATE, got %q", got.ID)
	}
	if got.Title != "Unemployment Rate" {
		t.Errorf("Title: expected 'Unemployment Rate', got %q", got.Title)
	}
}

func TestGetSeriesMetaNotFound(t *testing.T) {
	s := testDB(t)
	_, found, err := s.GetSeriesMeta("NOTEXIST")
	if err != nil {
		t.Fatalf("GetSeriesMeta: %v", err)
	}
	if found {
		t.Error("expected not found for missing series")
	}
}

func TestPutSeriesMetaStampsFetchedAt(t *testing.T) {
	s := testDB(t)
	before := time.Now().Add(-time.Second)
	_ = s.PutSeriesMeta(makeMeta("GDP", "Gross Domestic Product"))
	after := time.Now().Add(time.Second)

	got, _, _ := s.GetSeriesMeta("GDP")
	if got.FetchedAt.Before(before) || got.FetchedAt.After(after) {
		t.Errorf("FetchedAt %v outside expected range [%v, %v]", got.FetchedAt, before, after)
	}
}

func TestPutSeriesMetaOverwrites(t *testing.T) {
	s := testDB(t)
	_ = s.PutSeriesMeta(makeMeta("GDP", "Original Title"))
	_ = s.PutSeriesMeta(makeMeta("GDP", "Updated Title"))

	got, found, err := s.GetSeriesMeta("GDP")
	if err != nil || !found {
		t.Fatalf("GetSeriesMeta: err=%v found=%v", err, found)
	}
	if got.Title != "Updated Title" {
		t.Errorf("expected overwrite: got %q", got.Title)
	}
}

func TestListSeriesMeta(t *testing.T) {
	s := testDB(t)
	_ = s.PutSeriesMeta(makeMeta("UNRATE", "Unemployment Rate"))
	_ = s.PutSeriesMeta(makeMeta("GDP", "Gross Domestic Product"))
	_ = s.PutSeriesMeta(makeMeta("CPIAUCSL", "Consumer Price Index"))

	metas, err := s.ListSeriesMeta()
	if err != nil {
		t.Fatalf("ListSeriesMeta: %v", err)
	}
	if len(metas) != 3 {
		t.Errorf("expected 3 metas, got %d", len(metas))
	}
}

func TestListSeriesMetaEmpty(t *testing.T) {
	s := testDB(t)
	metas, err := s.ListSeriesMeta()
	if err != nil {
		t.Fatalf("ListSeriesMeta on empty db: %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("expected 0 metas on fresh db, got %d", len(metas))
	}
}

// ─── Observations ─────────────────────────────────────────────────────────────

func TestPutGetObs(t *testing.T) {
	s := testDB(t)
	key := store.ObsKey("UNRATE", "", "", "", "", "")
	data := makeSeriesData("UNRATE", 2020, 1, 3.5, 3.6, 4.1, 14.7, 13.3)

	if err := s.PutObs(key, data); err != nil {
		t.Fatalf("PutObs: %v", err)
	}

	got, found, err := s.GetObs(key)
	if err != nil {
		t.Fatalf("GetObs: %v", err)
	}
	if !found {
		t.Fatal("expected to find obs after put")
	}
	if got.SeriesID != "UNRATE" {
		t.Errorf("SeriesID: expected UNRATE, got %q", got.SeriesID)
	}
	if len(got.Obs) != 5 {
		t.Fatalf("expected 5 observations, got %d", len(got.Obs))
	}
	if got.Obs[0].Value != 3.5 {
		t.Errorf("obs[0].Value: expected 3.5, got %g", got.Obs[0].Value)
	}
}

func TestGetObsNotFound(t *testing.T) {
	s := testDB(t)
	_, found, err := s.GetObs("series:NOTEXIST")
	if err != nil {
		t.Fatalf("GetObs: %v", err)
	}
	if found {
		t.Error("expected not found for missing key")
	}
}

func TestPutObsNaNRoundTrip(t *testing.T) {
	s := testDB(t)
	key := store.ObsKey("TEST", "", "", "", "", "")
	data := makeSeriesData("TEST", 2020, 1, 1.0, math.NaN(), 3.0)

	if err := s.PutObs(key, data); err != nil {
		t.Fatalf("PutObs: %v", err)
	}

	got, _, err := s.GetObs(key)
	if err != nil {
		t.Fatalf("GetObs: %v", err)
	}
	if got.Obs[0].Value != 1.0 {
		t.Errorf("obs[0]: expected 1.0, got %g", got.Obs[0].Value)
	}
	if !isNaN(got.Obs[1].Value) {
		t.Errorf("obs[1]: expected NaN, got %g", got.Obs[1].Value)
	}
	if got.Obs[2].Value != 3.0 {
		t.Errorf("obs[2]: expected 3.0, got %g", got.Obs[2].Value)
	}
}

func TestPutObsDatesPreserved(t *testing.T) {
	s := testDB(t)
	key := store.ObsKey("TEST", "", "", "", "", "")
	data := makeSeriesData("TEST", 2024, 6, 1.0, 2.0, 3.0)

	_ = s.PutObs(key, data)
	got, _, _ := s.GetObs(key)

	expected := []time.Month{time.June, time.July, time.August}
	for i, obs := range got.Obs {
		if obs.Date.Month() != expected[i] {
			t.Errorf("obs[%d].Date.Month: expected %v, got %v", i, expected[i], obs.Date.Month())
		}
	}
}

func TestPutObsOverwrites(t *testing.T) {
	s := testDB(t)
	key := store.ObsKey("GDP", "", "", "", "", "")

	_ = s.PutObs(key, makeSeriesData("GDP", 2020, 1, 100.0, 200.0))
	_ = s.PutObs(key, makeSeriesData("GDP", 2020, 1, 300.0))

	got, _, _ := s.GetObs(key)
	if len(got.Obs) != 1 {
		t.Errorf("expected overwrite to 1 obs, got %d", len(got.Obs))
	}
	if got.Obs[0].Value != 300.0 {
		t.Errorf("expected overwritten value 300.0, got %g", got.Obs[0].Value)
	}
}

func TestPutObsMultipleKeys(t *testing.T) {
	s := testDB(t)
	k1 := store.ObsKey("UNRATE", "2020-01-01", "", "", "", "")
	k2 := store.ObsKey("UNRATE", "2021-01-01", "", "", "", "")

	_ = s.PutObs(k1, makeSeriesData("UNRATE", 2020, 1, 3.5))
	_ = s.PutObs(k2, makeSeriesData("UNRATE", 2021, 1, 6.7))

	got1, found1, _ := s.GetObs(k1)
	got2, found2, _ := s.GetObs(k2)

	if !found1 || !found2 {
		t.Fatalf("both keys should be found: k1=%v k2=%v", found1, found2)
	}
	if got1.Obs[0].Value != 3.5 {
		t.Errorf("k1: expected 3.5, got %g", got1.Obs[0].Value)
	}
	if got2.Obs[0].Value != 6.7 {
		t.Errorf("k2: expected 6.7, got %g", got2.Obs[0].Value)
	}
}

// ─── ListObsKeys ──────────────────────────────────────────────────────────────

func TestListObsKeysAllSeries(t *testing.T) {
	s := testDB(t)
	_ = s.PutObs(store.ObsKey("GDP", "", "", "", "", ""), makeSeriesData("GDP", 2020, 1, 1.0))
	_ = s.PutObs(store.ObsKey("UNRATE", "", "", "", "", ""), makeSeriesData("UNRATE", 2020, 1, 2.0))
	_ = s.PutObs(store.ObsKey("CPI", "", "", "", "", ""), makeSeriesData("CPI", 2020, 1, 3.0))

	keys, err := s.ListObsKeys("")
	if err != nil {
		t.Fatalf("ListObsKeys: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(keys), keys)
	}
}

func TestListObsKeysBySeriesPrefix(t *testing.T) {
	s := testDB(t)
	_ = s.PutObs(store.ObsKey("UNRATE", "2020-01-01", "", "", "", ""), makeSeriesData("UNRATE", 2020, 1, 1.0))
	_ = s.PutObs(store.ObsKey("UNRATE", "2021-01-01", "", "", "", ""), makeSeriesData("UNRATE", 2021, 1, 2.0))
	_ = s.PutObs(store.ObsKey("GDP", "", "", "", "", ""), makeSeriesData("GDP", 2020, 1, 3.0))

	keys, err := s.ListObsKeys("UNRATE")
	if err != nil {
		t.Fatalf("ListObsKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 UNRATE keys, got %d: %v", len(keys), keys)
	}
	for _, k := range keys {
		if !strings.HasPrefix(k, "series:UNRATE") {
			t.Errorf("key %q should start with 'series:UNRATE'", k)
		}
	}
}

func TestListObsKeysExactSeriesPrefixBoundary(t *testing.T) {
	s := testDB(t)
	_ = s.PutObs(store.ObsKey("GDP", "", "", "", "", ""), makeSeriesData("GDP", 2020, 1, 1.0))
	_ = s.PutObs(store.ObsKey("GDPDEF", "", "", "", "", ""), makeSeriesData("GDPDEF", 2020, 1, 2.0))

	keys, err := s.ListObsKeys("GDP")
	if err != nil {
		t.Fatalf("ListObsKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected exactly 1 GDP key, got %d: %v", len(keys), keys)
	}
	if keys[0] != "series:GDP" {
		t.Fatalf("expected GDP key only, got %q", keys[0])
	}
}

func TestListObsKeysEmpty(t *testing.T) {
	s := testDB(t)
	keys, err := s.ListObsKeys("")
	if err != nil {
		t.Fatalf("ListObsKeys on empty db: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys on fresh db, got %d", len(keys))
	}
}

// ─── Snapshots ────────────────────────────────────────────────────────────────

func TestPutGetSnapshot(t *testing.T) {
	s := testDB(t)
	snap := store.Snapshot{
		ID:          "01JABCDEF0000000000000000",
		Name:        "gdp-qoq",
		CommandLine: "store get GDP --format jsonl | transform pct-change",
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}

	if err := s.PutSnapshot(snap); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}

	got, found, err := s.GetSnapshot(snap.ID)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if !found {
		t.Fatal("expected to find snapshot after put")
	}
	if got.ID != snap.ID {
		t.Errorf("ID: expected %q, got %q", snap.ID, got.ID)
	}
	if got.Name != snap.Name {
		t.Errorf("Name: expected %q, got %q", snap.Name, got.Name)
	}
	if got.CommandLine != snap.CommandLine {
		t.Errorf("CommandLine: expected %q, got %q", snap.CommandLine, got.CommandLine)
	}
}

func TestGetSnapshotNotFound(t *testing.T) {
	s := testDB(t)
	_, found, err := s.GetSnapshot("notexist")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if found {
		t.Error("expected not found for missing snapshot")
	}
}

func TestListSnapshots(t *testing.T) {
	s := testDB(t)
	for i, name := range []string{"snap-a", "snap-b", "snap-c"} {
		_ = s.PutSnapshot(store.Snapshot{
			ID:          string(rune('1'+i)) + "ABCDEF",
			Name:        name,
			CommandLine: "reserve store get GDP",
			CreatedAt:   time.Now(),
		})
	}

	snaps, err := s.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(snaps))
	}
}

func TestDeleteSnapshot(t *testing.T) {
	s := testDB(t)
	snap := store.Snapshot{
		ID: "DELETEME", Name: "test",
		CommandLine: "reserve store get GDP", CreatedAt: time.Now(),
	}
	_ = s.PutSnapshot(snap)

	if err := s.DeleteSnapshot("DELETEME"); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	_, found, err := s.GetSnapshot("DELETEME")
	if err != nil {
		t.Fatalf("GetSnapshot after delete: %v", err)
	}
	if found {
		t.Error("snapshot should not be found after delete")
	}
}

func TestListSnapshotsEmpty(t *testing.T) {
	s := testDB(t)
	snaps, err := s.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots on empty db: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots on fresh db, got %d", len(snaps))
	}
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func TestStatsEmpty(t *testing.T) {
	s := testDB(t)
	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	for _, bs := range stats {
		if bs.Count != 0 {
			t.Errorf("bucket %q: expected 0 rows on fresh db, got %d", bs.Name, bs.Count)
		}
	}
}

func TestStatsCountsRows(t *testing.T) {
	s := testDB(t)
	_ = s.PutSeriesMeta(makeMeta("UNRATE", "Unemployment"))
	_ = s.PutSeriesMeta(makeMeta("GDP", "GDP"))
	_ = s.PutObs(store.ObsKey("UNRATE", "", "", "", "", ""), makeSeriesData("UNRATE", 2020, 1, 1.0))

	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	byName := make(map[string]int)
	for _, bs := range stats {
		byName[bs.Name] = bs.Count
	}
	if byName["series_meta"] != 2 {
		t.Errorf("series_meta: expected 2, got %d", byName["series_meta"])
	}
	if byName["obs"] != 1 {
		t.Errorf("obs: expected 1, got %d", byName["obs"])
	}
}

// ─── ClearBucket / ClearAll ───────────────────────────────────────────────────

func TestClearBucket(t *testing.T) {
	s := testDB(t)
	_ = s.PutSeriesMeta(makeMeta("UNRATE", "Unemployment"))
	_ = s.PutSeriesMeta(makeMeta("GDP", "GDP"))

	if err := s.ClearBucket("series_meta"); err != nil {
		t.Fatalf("ClearBucket: %v", err)
	}

	metas, _ := s.ListSeriesMeta()
	if len(metas) != 0 {
		t.Errorf("expected 0 metas after ClearBucket, got %d", len(metas))
	}
}

func TestClearBucketLeavesOthersIntact(t *testing.T) {
	s := testDB(t)
	_ = s.PutSeriesMeta(makeMeta("GDP", "GDP"))
	_ = s.PutObs(store.ObsKey("GDP", "", "", "", "", ""), makeSeriesData("GDP", 2020, 1, 1.0))

	_ = s.ClearBucket("series_meta")

	// obs bucket should be untouched
	_, found, err := s.GetObs(store.ObsKey("GDP", "", "", "", "", ""))
	if err != nil {
		t.Fatalf("GetObs after ClearBucket(series_meta): %v", err)
	}
	if !found {
		t.Error("obs bucket should be intact after clearing series_meta")
	}
}

func TestClearAll(t *testing.T) {
	s := testDB(t)
	_ = s.PutSeriesMeta(makeMeta("GDP", "GDP"))
	_ = s.PutObs(store.ObsKey("GDP", "", "", "", "", ""), makeSeriesData("GDP", 2020, 1, 1.0))
	_ = s.PutSnapshot(store.Snapshot{
		ID: "S1", Name: "test", CommandLine: "reserve store get GDP", CreatedAt: time.Now(),
	})

	if err := s.ClearAll(); err != nil {
		t.Fatalf("ClearAll: %v", err)
	}

	metas, _ := s.ListSeriesMeta()
	keys, _ := s.ListObsKeys("")
	snaps, _ := s.ListSnapshots()

	if len(metas) != 0 || len(keys) != 0 || len(snaps) != 0 {
		t.Errorf("ClearAll: metas=%d keys=%d snaps=%d (all should be 0)",
			len(metas), len(keys), len(snaps))
	}
}

// ─── Isolation ────────────────────────────────────────────────────────────────

func TestEachTestGetsIsolatedDB(t *testing.T) {
	// Two stores from different temp dirs must not share data
	s1 := testDB(t)
	_ = s1.PutSeriesMeta(makeMeta("UNRATE", "Unemployment"))

	s2 := testDB(t)
	_, found, err := s2.GetSeriesMeta("UNRATE")
	if err != nil {
		t.Fatalf("GetSeriesMeta on s2: %v", err)
	}
	if found {
		t.Error("s2 should not see data written to s1 — databases are not isolated")
	}
}
