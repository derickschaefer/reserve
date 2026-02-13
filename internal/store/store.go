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
const schemaVersion = 1

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

// migrate ensures all buckets exist and schema is current.
func (s *Store) migrate() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// Create all buckets if they don't exist.
		for _, name := range [][]byte{bucketObs, bucketSeriesMeta, bucketSnapshots, bucketInternal} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("creating bucket %s: %w", name, err)
			}
		}

		// Write schema version if not set.
		meta := tx.Bucket(bucketInternal)
		if meta.Get([]byte("schema_version")) == nil {
			if err := meta.Put([]byte("schema_version"), []byte(fmt.Sprintf("%d", schemaVersion))); err != nil {
				return err
			}
			if err := meta.Put([]byte("created_at"), []byte(time.Now().UTC().Format(time.RFC3339))); err != nil {
				return err
			}
		}
		return nil
	})
}

// ─── Series Metadata ──────────────────────────────────────────────────────────

// PutSeriesMeta stores metadata for a series, stamping FetchedAt.
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
// Value is a *float64 so that missing values (NaN) are stored as JSON null
// rather than NaN, which encoding/json cannot handle.
type storedObsRow struct {
	Date          string   `json:"date"`
	Value         *float64 `json:"value"` // null = missing
	ValueRaw      string   `json:"value_raw"`
	RealtimeStart string   `json:"realtime_start,omitempty"`
	RealtimeEnd   string   `json:"realtime_end,omitempty"`
}

// storedObs is the on-disk envelope for a series observations entry.
type storedObs struct {
	SeriesID  string         `json:"series_id"`
	FetchedAt time.Time      `json:"fetched_at"`
	Obs       []storedObsRow `json:"observations"`
}

// obsToStored converts model.Observation → storedObsRow (NaN → null).
func obsToStored(o model.Observation) storedObsRow {
	row := storedObsRow{
		Date:          o.Date.Format("2006-01-02"),
		ValueRaw:      o.ValueRaw,
		RealtimeStart: o.RealtimeStart,
		RealtimeEnd:   o.RealtimeEnd,
	}
	if !o.IsMissing() {
		v := o.Value
		row.Value = &v
	}
	return row
}

// storedToObs converts storedObsRow → model.Observation (null → NaN).
func storedToObs(r storedObsRow) model.Observation {
	t, _ := time.Parse("2006-01-02", r.Date)
	obs := model.Observation{
		Date:          t,
		ValueRaw:      r.ValueRaw,
		RealtimeStart: r.RealtimeStart,
		RealtimeEnd:   r.RealtimeEnd,
	}
	if r.Value != nil {
		obs.Value = *r.Value
	} else {
		obs.Value = math.NaN()
	}
	return obs
}

// PutObs stores observations under the given key.
func (s *Store) PutObs(key string, data model.SeriesData) error {
	rows := make([]storedObsRow, len(data.Obs))
	for i, o := range data.Obs {
		rows[i] = obsToStored(o)
	}
	envelope := storedObs{
		SeriesID:  data.SeriesID,
		FetchedAt: time.Now().UTC(),
		Obs:       rows,
	}
	b, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encoding obs: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketObs).Put([]byte(key), b)
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
		obs[i] = storedToObs(r)
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

// ClearBucket deletes all entries in the named bucket.
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
