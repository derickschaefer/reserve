//go:build goexperiment.jsonv2

// This file contains benchmarks that call encoding/json/v2 directly and a
// correctness parity test comparing v1 and v2 output on real FRED data.
// It only compiles when GOEXPERIMENT=jsonv2 is set.
//
// Run:
//
//	GOEXPERIMENT=jsonv2 go test ./tests/benchmarks/... -bench=. -run TestV1V2Parity -v -benchmem -count=10
package benchmarks_test

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/derickschaefer/reserve/internal/model"
)

// ─── Explicit v2 API benchmarks ───────────────────────────────────────────────
// These call jsonv2.Marshal/Unmarshal directly rather than relying on
// GOEXPERIMENT to swap the v1 internals. Run alongside the v1 benchmarks
// in bench_test.go for a direct API comparison.

func BenchmarkV2UnmarshalRawObs_GDP(b *testing.B) {
	data := loadFixture(b, "gdp_obs")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var resp fredObsResponse
		if err := jsonv2.Unmarshal(data, &resp); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkV2UnmarshalRawObs_CPIAUCSL(b *testing.B) {
	data := loadFixture(b, "cpiaucsl_obs")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var resp fredObsResponse
		if err := jsonv2.Unmarshal(data, &resp); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkV2UnmarshalRawObs_UNRATE(b *testing.B) {
	data := loadFixture(b, "unrate_obs")
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var resp fredObsResponse
		if err := jsonv2.Unmarshal(data, &resp); err != nil {
			b.Fatal(err)
		}
	}
}

// Marshal/unmarshal use safeSeriesData (NaN→null) to mirror store.PutObs behavior.

func BenchmarkV2MarshalSeriesData_GDP(b *testing.B) {
	safe := toSafeSeriesData(loadObsFixture(b, "gdp_obs", "GDP"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := jsonv2.Marshal(&safe)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkV2MarshalSeriesData_CPIAUCSL(b *testing.B) {
	safe := toSafeSeriesData(loadObsFixture(b, "cpiaucsl_obs", "CPIAUCSL"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := jsonv2.Marshal(&safe)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkV2UnmarshalSeriesData_CPIAUCSL(b *testing.B) {
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
		if err := jsonv2.Unmarshal(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkV2MarshalMetaBatch(b *testing.B) {
	metas := loadMetaFixtures(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := jsonv2.Marshal(metas)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkV2UnmarshalMetaBatch(b *testing.B) {
	metas := loadMetaFixtures(b)
	data, _ := json.Marshal(metas)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out []model.SeriesMeta
		if err := jsonv2.Unmarshal(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}

// ─── Byte identity test ───────────────────────────────────────────────────────
// Checks whether v1 and v2 produce identical bytes for our marshal payloads.
// This is the key question for the Go team: is the marshal regression a pure
// performance regression, or is v2 emitting different (possibly more correct)
// output?
//
// Run: GOEXPERIMENT=jsonv2 go test ./tests/benchmarks/... -run TestMarshalByteIdentity -v

func TestMarshalByteIdentity(t *testing.T) {
	cases := []struct{ fixture, id string }{
		{"gdp_obs", "GDP"},
		{"cpiaucsl_obs", "CPIAUCSL"},
		{"unrate_obs", "UNRATE"},
	}

	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			safe := toSafeSeriesData(loadObsFixture(t, c.fixture, c.id))

			v1bytes, err := json.Marshal(&safe)
			if err != nil {
				t.Fatalf("v1 Marshal: %v", err)
			}
			v2bytes, err := jsonv2.Marshal(&safe)
			if err != nil {
				t.Fatalf("v2 Marshal: %v", err)
			}

			if bytes.Equal(v1bytes, v2bytes) {
				t.Logf("✓ %s: byte-for-byte identical (%d bytes)", c.id, len(v1bytes))
				return
			}

			// Outputs differ — find and report the first divergence point.
			t.Errorf("✗ %s: v1 and v2 output differ (v1=%d bytes, v2=%d bytes)",
				c.id, len(v1bytes), len(v2bytes))

			// Find first differing byte position.
			minLen := len(v1bytes)
			if len(v2bytes) < minLen {
				minLen = len(v2bytes)
			}
			firstDiff := -1
			for i := 0; i < minLen; i++ {
				if v1bytes[i] != v2bytes[i] {
					firstDiff = i
					break
				}
			}

			if firstDiff >= 0 {
				// Show context window around the first difference.
				start := firstDiff - 40
				if start < 0 {
					start = 0
				}
				end := firstDiff + 80
				if end > minLen {
					end = minLen
				}
				t.Logf("  first diff at byte %d:", firstDiff)
				t.Logf("  v1: ...%s...", string(v1bytes[start:end]))
				t.Logf("  v2: ...%s...", string(v2bytes[start:end]))
			} else {
				// One is a prefix of the other.
				t.Logf("  v1 tail: ...%s", string(v1bytes[minLen-80:]))
				t.Logf("  v2 tail: ...%s", string(v2bytes[minLen-80:]))
			}

			// Also check metadata batch since it has more varied field types.
		})
	}

	t.Run("MetaBatch", func(t *testing.T) {
		metas := loadMetaFixtures(t)

		v1bytes, err := json.Marshal(metas)
		if err != nil {
			t.Fatalf("v1 Marshal: %v", err)
		}
		v2bytes, err := jsonv2.Marshal(metas)
		if err != nil {
			t.Fatalf("v2 Marshal: %v", err)
		}

		if bytes.Equal(v1bytes, v2bytes) {
			t.Logf("✓ MetaBatch: byte-for-byte identical (%d bytes)", len(v1bytes))
			return
		}

		t.Errorf("✗ MetaBatch: v1 and v2 output differ (v1=%d bytes, v2=%d bytes)",
			len(v1bytes), len(v2bytes))

		minLen := len(v1bytes)
		if len(v2bytes) < minLen {
			minLen = len(v2bytes)
		}
		for i := 0; i < minLen; i++ {
			if v1bytes[i] != v2bytes[i] {
				start := i - 40
				if start < 0 {
					start = 0
				}
				end := i + 80
				if end > minLen {
					end = minLen
				}
				t.Logf("  first diff at byte %d:", i)
				t.Logf("  v1: ...%s...", string(v1bytes[start:end]))
				t.Logf("  v2: ...%s...", string(v2bytes[start:end]))
				break
			}
		}
	})
}

// ─── Parity test ──────────────────────────────────────────────────────────────
// Verifies v1 and v2 produce identical output for our store envelope types.
// Uses safeSeriesData (*float64 for values) so NaN is represented as null —
// the same representation the store layer uses in production.
//
// Run: GOEXPERIMENT=jsonv2 go test ./tests/benchmarks/... -run TestV1V2Parity -v

func TestV1V2Parity(t *testing.T) {
	cases := []struct{ fixture, id string }{
		{"gdp_obs", "GDP"},
		{"cpiaucsl_obs", "CPIAUCSL"},
		{"unrate_obs", "UNRATE"},
	}

	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			safe := toSafeSeriesData(loadObsFixture(t, c.fixture, c.id))

			// Marshal with both encoders
			v1bytes, err := json.Marshal(&safe)
			if err != nil {
				t.Fatalf("v1 Marshal: %v", err)
			}
			v2bytes, err := jsonv2.Marshal(&safe)
			if err != nil {
				t.Fatalf("v2 Marshal: %v", err)
			}

			// Unmarshal each output with its own decoder
			var fromV1, fromV2 safeSeriesData
			if err := json.Unmarshal(v1bytes, &fromV1); err != nil {
				t.Fatalf("v1 Unmarshal of v1 output: %v", err)
			}
			if err := jsonv2.Unmarshal(v2bytes, &fromV2); err != nil {
				t.Fatalf("v2 Unmarshal of v2 output: %v", err)
			}

			// Observation count must match
			if len(fromV1.Obs) != len(fromV2.Obs) {
				t.Errorf("count mismatch: v1=%d v2=%d", len(fromV1.Obs), len(fromV2.Obs))
				return
			}

			// Value-by-value comparison (*float64: nil=missing, non-nil=numeric)
			mismatches := 0
			for i := range fromV1.Obs {
				o1, o2 := fromV1.Obs[i], fromV2.Obs[i]
				null1, null2 := o1.Value == nil, o2.Value == nil
				switch {
				case null1 != null2:
					t.Errorf("obs[%d] null mismatch: v1_null=%v v2_null=%v", i, null1, null2)
					mismatches++
				case !null1 && *o1.Value != *o2.Value:
					t.Errorf("obs[%d] value mismatch: v1=%g v2=%g", i, *o1.Value, *o2.Value)
					mismatches++
				}
				if mismatches > 5 {
					t.Log("stopping after 5 mismatches")
					break
				}
			}

			// Cross-decode: v1 output readable by v2 decoder and vice versa
			var crossV1, crossV2 safeSeriesData
			if err := jsonv2.Unmarshal(v1bytes, &crossV1); err != nil {
				t.Errorf("v2 cannot decode v1 output: %v", err)
			}
			if err := json.Unmarshal(v2bytes, &crossV2); err != nil {
				t.Errorf("v1 cannot decode v2 output: %v", err)
			}

			t.Logf("%-10s obs=%d  v1=%d bytes  v2=%d bytes  mismatches=%d",
				c.id, len(fromV1.Obs), len(v1bytes), len(v2bytes), mismatches)
		})
	}
}
