// Package store provides a thin bbolt wrapper for reserve's local data store.
//
// Design philosophy: the store is an intentional data accumulator, not a
// transparent HTTP cache. Data is written explicitly via fetch commands and
// read by analysis commands. No TTL, no auto-invalidation — you own your data.
//
// Buckets:
//
//	obs         — accumulated observations keyed by series+params
//	series_meta — metadata for fetched series
//	snapshots   — saved command lines for reproducible workflows
//	config      — reserved for future use (api_key etc. stay in config.json)
//	_meta       — internal: schema version, created_at
//
// Schema v2 changes (from v1):
//   - storedObs envelope now carries realtime_start / realtime_end at the
//     envelope level rather than per-row, cutting per-observation storage by ~35%.
//   - New batch write methods: PutObsBatch, PutSeriesMetaBatch.
//   - New maintenance method: Compact.
package store

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/derickschaefer/reserve/internal/model"
)

// Current schema version. Bump when bucket layout or key format changes.
const schemaVersion = 2

// Bucket name constants.
var (
	bucketObs        = []byte("obs")
	bucketSeriesMeta = []byte("series_meta")
	bucketSnapshots  = []byte("snapshots")
	bucketInternal   = []byte("_meta")
)

// AllBuckets lists every top-level bucket for stats and clear operations.
var AllBuckets = []string{"obs", "series_meta", "snapshots"}

// Store wraps a bbolt database.
type Store struct {
	db *bolt.DB
}

// Open opens (or creates) the bbolt database at path.
// Parent directories are created automatically.
// Runs schema migrations on every open.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening db %s: %w", path, err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Path returns the filesystem path of the open database.
func (s *Store) Path() string {
	return s.db.Path()
}

// ─── Migrations ───────────────────────────────────────────────────────────────

// migrate ensures all buckets exist and the schema version is current.
// v1 → v2: realtime fields moved to envelope level; old obs entries are
// dropped (pre-release, no installed user data to preserve).
func (s *Store) migrate() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// Create all buckets if they don't exist.
		for _, name := range [][]byte{bucketObs, bucketSeriesMeta, bucketSnapshots, bucketInternal} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("creating bucket %s: %w", name, err)
			}
		}

		meta := tx.Bucket(bucketInternal)
		raw := meta.Get([]byte("schema_version"))

		if raw == nil {
			// Fresh database — write current version and timestamp.
			if err := meta.Put([]byte("schema_version"), []byte(fmt.Sprintf("%d", schemaVersion))); err != nil {
				return err
			}
			return meta.Put([]byte("created_at"), []byte(time.Now().UTC().Format(time.RFC3339)))
		}

		// Check whether an upgrade is needed.
		var existing int
		fmt.Sscanf(string(raw), "%d", &existing)

		if existing < 2 {
			// v1 → v2: drop all obs entries; the slimmed envelope format is
			// incompatible with v1 rows. Users re-fetch after upgrade.
			if err := tx.DeleteBucket(bucketObs); err != nil {
				return fmt.Errorf("dropping obs bucket for v2 migration: %w", err)
			}
			if _, err := tx.CreateBucket(bucketObs); err != nil {
				return fmt.Errorf("recreating obs bucket for v2 migration: %w", err)
			}
			if err := meta.Put([]byte("schema_version"), []byte(fmt.Sprintf("%d", schemaVersion))); err != nil {
				return err
			}
		}

		return nil
	})
}

// ─── Series Metadata ──────────────────────────────────────────────────────────

// PutSeriesMeta stores metadata for a single series, stamping FetchedAt.
// Prefer PutSeriesMetaBatch when writing multiple series at once.
func (s *Store) PutSeriesMeta(meta model.SeriesMeta) error {
	meta.FetchedAt = time.Now().UTC()
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding series meta: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSeriesMeta).Put([]byte(meta.ID), data)
	})
}

// PutSeriesMetaBatch writes multiple series metadata entries in a single write
// transaction, replacing N fsyncs with one. This is the preferred method for
// batch fetch operations.
func (s *Store) PutSeriesMetaBatch(metas []model.SeriesMeta) error {
	if len(metas) == 0 {
		return nil
	}
	now := time.Now().UTC()
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketSeriesMeta)
		for _, meta := range metas {
			meta.FetchedAt = now
			data, err := json.Marshal(meta)
			if err != nil {
				return fmt.Errorf("encoding series meta %s: %w", meta.ID, err)
			}
			if err := bucket.Put([]byte(meta.ID), data); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetSeriesMeta retrieves metadata for a series by ID.
// Returns (meta, true, nil) if found, (zero, false, nil) if not found.
func (s *Store) GetSeriesMeta(id string) (model.SeriesMeta, bool, error) {
	var meta model.SeriesMeta
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketSeriesMeta).Get([]byte(id))
		if v == nil {
			return nil
		}
		return json.Unmarshal(v, &meta)
	})
	if err != nil {
		return meta, false, err
	}
	return meta, meta.ID != "", nil
}

// ListSeriesMeta returns all stored series metadata, sorted by ID.
func (s *Store) ListSeriesMeta() ([]model.SeriesMeta, error) {
	var metas []model.SeriesMeta
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSeriesMeta).ForEach(func(k, v []byte) error {
			var m model.SeriesMeta
			if err := json.Unmarshal(v, &m); err != nil {
				return err
			}
			metas = append(metas, m)
			return nil
		})
	})
	return metas, err
}

// ─── Observations ─────────────────────────────────────────────────────────────

// ObsKey builds the canonical key for an observations entry.
// Format: series:<ID>|start:<date>|end:<date>|freq:<f>|units:<u>|agg:<a>
// Empty optional fields are omitted.
func ObsKey(seriesID, start, end, freq, units, agg string) string {
	key := "series:" + seriesID
	if start != "" {
		key += "|start:" + start
	}
	if end != "" {
		key += "|end:" + end
	}
	if freq != "" {
		key += "|freq:" + freq
	}
	if units != "" {
		key += "|units:" + units
	}
	if agg != "" {
		key += "|agg:" + agg
	}
	return key
}

// storedObsRow is the JSON-safe on-disk representation of a single observation.
//
// Schema v2: realtime_start and realtime_end have been moved to the storedObs
// envelope. They are almost always identical across every row in a fetch, so
// storing them per-row was pure redundancy (~35% size reduction for typical
// FRED series). Value is *float64 so that missing values (NaN) are stored as
// JSON null rather than NaN, which encoding/json cannot handle.
type storedObsRow struct {
	Date     string   `json:"date"`
	Value    *float64 `json:"value"` // null = missing
	ValueRaw string   `json:"value_raw"`
}

// storedObs is the on-disk envelope for a series observations entry.
//
// Schema v2: realtime_start / realtime_end are stored once at the envelope
// level and applied to every observation on read.
type storedObs struct {
	SeriesID      string         `json:"series_id"`
	FetchedAt     time.Time      `json:"fetched_at"`
	RealtimeStart string         `json:"realtime_start,omitempty"`
	RealtimeEnd   string         `json:"realtime_end,omitempty"`
	Obs           []storedObsRow `json:"observations"`
}

// obsToStored converts model.Observation → storedObsRow (NaN → null).
// The realtime fields are not stored on the row; they come from the envelope.
func obsToStored(o model.Observation) storedObsRow {
	row := storedObsRow{
		Date:     o.Date.Format("2006-01-02"),
		ValueRaw: o.ValueRaw,
	}
	if !o.IsMissing() {
		v := o.Value
		row.Value = &v
	}
	return row
}

// storedToObs converts storedObsRow → model.Observation (null → NaN).
// realtimeStart / realtimeEnd come from the parent envelope.
func storedToObs(r storedObsRow, realtimeStart, realtimeEnd string) model.Observation {
	t, _ := time.Parse("2006-01-02", r.Date)
	obs := model.Observation{
		Date:          t,
		ValueRaw:      r.ValueRaw,
		RealtimeStart: realtimeStart,
		RealtimeEnd:   realtimeEnd,
	}
	if r.Value != nil {
		obs.Value = *r.Value
	} else {
		obs.Value = math.NaN()
	}
	return obs
}

// realtimeFieldsFromData extracts the realtime window from the first observation
// in a SeriesData slice. FRED returns the same realtime_start / realtime_end
// for every observation in a single request, so storing it once is correct.
func realtimeFieldsFromData(data model.SeriesData) (start, end string) {
	if len(data.Obs) > 0 {
		return data.Obs[0].RealtimeStart, data.Obs[0].RealtimeEnd
	}
	return "", ""
}

// PutObs stores observations under the given key in a single write transaction.
// Prefer PutObsBatch when writing multiple series at once.
func (s *Store) PutObs(key string, data model.SeriesData) error {
	rtStart, rtEnd := realtimeFieldsFromData(data)
	rows := make([]storedObsRow, len(data.Obs))
	for i, o := range data.Obs {
		rows[i] = obsToStored(o)
	}
	envelope := storedObs{
		SeriesID:      data.SeriesID,
		FetchedAt:     time.Now().UTC(),
		RealtimeStart: rtStart,
		RealtimeEnd:   rtEnd,
		Obs:           rows,
	}
	b, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encoding obs: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketObs).Put([]byte(key), b)
	})
}

// PutObsBatch writes multiple series observations entries in a single write
// transaction, replacing N fsyncs with one. The map key is the canonical obs
// key (built with ObsKey). This is the preferred method for batch fetch
// operations and eliminates the N×fsync bottleneck entirely.
func (s *Store) PutObsBatch(entries map[string]model.SeriesData) error {
	if len(entries) == 0 {
		return nil
	}
	now := time.Now().UTC()
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketObs)
		for key, data := range entries {
			rtStart, rtEnd := realtimeFieldsFromData(data)
			rows := make([]storedObsRow, len(data.Obs))
			for i, o := range data.Obs {
				rows[i] = obsToStored(o)
			}
			envelope := storedObs{
				SeriesID:      data.SeriesID,
				FetchedAt:     now,
				RealtimeStart: rtStart,
				RealtimeEnd:   rtEnd,
				Obs:           rows,
			}
			b, err := json.Marshal(envelope)
			if err != nil {
				return fmt.Errorf("encoding obs %s: %w", data.SeriesID, err)
			}
			if err := bucket.Put([]byte(key), b); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetObs retrieves observations by key.
// Returns (data, true, nil) if found, (zero, false, nil) if not found.
func (s *Store) GetObs(key string) (model.SeriesData, bool, error) {
	var envelope storedObs
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketObs).Get([]byte(key))
		if v == nil {
			return nil
		}
		return json.Unmarshal(v, &envelope)
	})
	if err != nil {
		return model.SeriesData{}, false, err
	}
	if envelope.SeriesID == "" {
		return model.SeriesData{}, false, nil
	}
	obs := make([]model.Observation, len(envelope.Obs))
	for i, r := range envelope.Obs {
		obs[i] = storedToObs(r, envelope.RealtimeStart, envelope.RealtimeEnd)
	}
	return model.SeriesData{SeriesID: envelope.SeriesID, Obs: obs}, true, nil
}

// ListObsKeys returns all keys in the obs bucket for a given series prefix.
// Pass seriesID="" to list all keys.
func (s *Store) ListObsKeys(seriesID string) ([]string, error) {
	prefix := []byte("series:")
	if seriesID != "" {
		prefix = []byte("series:" + seriesID)
	}
	var keys []string
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketObs).Cursor()
		for k, _ := c.Seek(prefix); k != nil; k, _ = c.Next() {
			if len(k) < len(prefix) {
				break
			}
			keys = append(keys, string(k))
		}
		return nil
	})
	return keys, err
}

// ─── Snapshots ────────────────────────────────────────────────────────────────

// Snapshot represents a saved command for reproducible workflows.
type Snapshot struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	CommandLine string    `json:"command_line"`
	CreatedAt   time.Time `json:"created_at"`
}

// PutSnapshot saves a snapshot. The key is snap:<ID>.
func (s *Store) PutSnapshot(snap Snapshot) error {
	b, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("encoding snapshot: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSnapshots).Put([]byte("snap:"+snap.ID), b)
	})
}

// GetSnapshot retrieves a snapshot by ID.
func (s *Store) GetSnapshot(id string) (Snapshot, bool, error) {
	var snap Snapshot
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketSnapshots).Get([]byte("snap:" + id))
		if v == nil {
			return nil
		}
		return json.Unmarshal(v, &snap)
	})
	if err != nil {
		return snap, false, err
	}
	return snap, snap.ID != "", nil
}

// ListSnapshots returns all snapshots in creation order.
func (s *Store) ListSnapshots() ([]Snapshot, error) {
	var snaps []Snapshot
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSnapshots).ForEach(func(k, v []byte) error {
			var snap Snapshot
			if err := json.Unmarshal(v, &snap); err != nil {
				return err
			}
			snaps = append(snaps, snap)
			return nil
		})
	})
	return snaps, err
}

// DeleteSnapshot removes a snapshot by ID.
func (s *Store) DeleteSnapshot(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSnapshots).Delete([]byte("snap:" + id))
	})
}

// ─── Stats & Maintenance ──────────────────────────────────────────────────────

// BucketStats holds row count and byte size for a single bucket.
type BucketStats struct {
	Name  string
	Count int
	Bytes int64
}

// Stats returns row counts and approximate sizes for all buckets.
func (s *Store) Stats() ([]BucketStats, error) {
	buckets := map[string][]byte{
		"obs":         bucketObs,
		"series_meta": bucketSeriesMeta,
		"snapshots":   bucketSnapshots,
	}

	var stats []BucketStats
	err := s.db.View(func(tx *bolt.Tx) error {
		for name, bname := range buckets {
			b := tx.Bucket(bname)
			if b == nil {
				continue
			}
			var count int
			var bytes int64
			b.ForEach(func(k, v []byte) error {
				count++
				bytes += int64(len(k) + len(v))
				return nil
			})
			stats = append(stats, BucketStats{Name: name, Count: count, Bytes: bytes})
		}
		return nil
	})
	return stats, err
}

// ClearBucket deletes all entries in the named bucket by drop-and-recreate,
// which is more efficient than iterating keys and returns pages to bbolt's
// internal freelist. Note: the database file does not shrink automatically;
// use Compact to reclaim disk space.
func (s *Store) ClearBucket(name string) error {
	bname := []byte(name)
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(bname); err != nil {
			return fmt.Errorf("clearing bucket %s: %w", name, err)
		}
		_, err := tx.CreateBucket(bname)
		return err
	})
}

// ClearAll deletes all entries from every user-facing bucket.
func (s *Store) ClearAll() error {
	for _, name := range AllBuckets {
		if err := s.ClearBucket(name); err != nil {
			return err
		}
	}
	return nil
}

// Compact rewrites the entire database to a new file, reclaiming disk space
// freed by prior deletions. bbolt does not shrink the file automatically after
// ClearBucket / ClearAll — free pages are reused internally but the file
// footprint does not decrease until compaction.
//
// The operation is safe: all live data is copied to a temporary file first,
// then the original is atomically replaced. The Store remains usable after
// Compact returns.
func (s *Store) Compact() (beforeBytes, afterBytes int64, err error) {
	path := s.db.Path()
	tmpPath := path + ".compact.tmp"

	// Measure before size.
	if fi, err2 := os.Stat(path); err2 == nil {
		beforeBytes = fi.Size()
	}

	// Open destination database.
	dst, err := bolt.Open(tmpPath, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return beforeBytes, 0, fmt.Errorf("opening temp db for compaction: %w", err)
	}

	// Copy all live pages from src to dst.
	if err = bolt.Compact(dst, s.db, 0); err != nil {
		dst.Close()
		os.Remove(tmpPath)
		return beforeBytes, 0, fmt.Errorf("compacting db: %w", err)
	}
	dst.Close()

	// Close the original before replacing it.
	if err = s.db.Close(); err != nil {
		os.Remove(tmpPath)
		return beforeBytes, 0, fmt.Errorf("closing db before compaction swap: %w", err)
	}

	// Atomically replace original with compacted copy.
	if err = os.Rename(tmpPath, path); err != nil {
		// Attempt to reopen the original so the Store isn't left broken.
		s.db, _ = bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
		return beforeBytes, 0, fmt.Errorf("replacing db with compacted copy: %w", err)
	}

	// Reopen the now-compacted database.
	s.db, err = bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return beforeBytes, 0, fmt.Errorf("reopening compacted db: %w", err)
	}

	// Measure after size.
	if fi, err2 := os.Stat(path); err2 == nil {
		afterBytes = fi.Size()
	}

	return beforeBytes, afterBytes, nil
}
