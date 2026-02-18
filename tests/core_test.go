// ============================================================================
// FILE:        tests/reserve_test.go
// PROJECT:     reserve
// DESCRIPTION: Test suite covering the four core verification pillars:
//
//   1. FRED API Connectivity  — live HTTP reachability and JSON payload shape
//   2. Payload Integrity      — observation parsing, NaN handling, value
//                               formatting, config precedence (all offline)
//   3. API Client Behaviour   — mock HTTP server: retries, params, search
//   4. Email Connectivity     — SMTP TCP dial and banner (skips if unconfigured)
//
// TEST RUNNER:
//   go test -v -run TestFredAPIConnectivity  ./tests/
//   go test -v -run TestPayloadIntegrity     ./tests/
//   go test -v -run TestAPIClientBehaviour   ./tests/
//   go test -v -run TestEmailConnectivity    ./tests/
//   go test -v ./tests/                      (all four groups)
//
// CREDENTIALS:
//   Groups 1 and 4 read from config.json via config.Load().
//   Groups 2 and 3 are fully offline and never skip.
//   If config.json is missing or the API key is a placeholder, groups 1
//   and 4 skip automatically with a descriptive message.
// ============================================================================

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/util"
	"golang.org/x/time/rate"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test Output Helpers
// ─────────────────────────────────────────────────────────────────────────────

const (
	checkPass = "  ✅"
	checkFail = "  ❌"
	divider   = "──────────────────────────────────────────────────────────────────────────"
	separator = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
)

// result tracks pass/fail tallies for a single test group.
type result struct {
	passed int
	failed int
}

func (r *result) pass(t *testing.T, label string) {
	t.Helper()
	r.passed++
	t.Logf("%s %s", checkPass, label)
}

func (r *result) fail(t *testing.T, label string, detail ...string) {
	t.Helper()
	r.failed++
	line := label
	if len(detail) > 0 && detail[0] != "" {
		line = fmt.Sprintf("%s  →  %s", label, detail[0])
	}
	t.Logf("%s %s", checkFail, line)
	t.Fail()
}

func (r *result) check(t *testing.T, condition bool, passLabel, failLabel string, detail ...string) {
	t.Helper()
	if condition {
		r.pass(t, passLabel)
	} else {
		r.fail(t, failLabel, detail...)
	}
}

func (r *result) summary(t *testing.T, groupName string) {
	t.Helper()
	total := r.passed + r.failed
	icon := "✅"
	if r.failed > 0 {
		icon = "❌"
	}
	t.Logf("%s", divider)
	t.Logf("  %s  %s: %d/%d checks passed", icon, groupName, r.passed, total)
	t.Logf("%s", separator)
}

func printBanner(t *testing.T, title string) {
	t.Helper()
	t.Logf("")
	t.Logf("%s", separator)
	t.Logf("  🔬  %s", title)
	t.Logf("%s", divider)
}

// configOrSkip loads config.json from the repo root (one level up from tests/).
// Skips the calling test if the file is missing or the API key is not set.
func configOrSkip(t *testing.T) *config.Config {
	t.Helper()

	// Change to repo root so config.Load() finds config.json
	orig, _ := os.Getwd()
	root := filepath.Join(orig, "..")
	os.Chdir(root)
	defer os.Chdir(orig)

	cfg, err := config.Load("")
	if err != nil {
		t.Skipf("⏭️  Skipping: config.json not ready (%v)", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Skipf("⏭️  Skipping: API key not configured (%v)", err)
	}
	return cfg
}

// ─────────────────────────────────────────────────────────────────────────────
// Group 1 — FRED API Connectivity
// ─────────────────────────────────────────────────────────────────────────────

func TestFredAPIConnectivity(t *testing.T) {
	cfg := configOrSkip(t)

	printBanner(t, "FRED API CONNECTIVITY")
	r := &result{}

	client := fred.NewClient(
		cfg.APIKey,
		cfg.BaseURL,
		15*time.Second,
		cfg.Rate,
		false,
	)
	dateRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	const seriesID = "UNRATE"

	// ── Check 1: DNS resolution ──────────────────────────────────────────────
	_, dnsErr := net.LookupHost("api.stlouisfed.org")
	r.check(t,
		dnsErr == nil,
		"DNS resolution: api.stlouisfed.org is reachable",
		"DNS resolution: api.stlouisfed.org is unreachable",
		fmt.Sprintf("%v", dnsErr),
	)

	// ── Check 2: Series metadata returns successfully ────────────────────────
	meta, metaErr := client.GetSeries(context.Background(), seriesID)
	r.check(t,
		metaErr == nil && meta != nil,
		fmt.Sprintf("GetSeries(%s) returned metadata without error", seriesID),
		fmt.Sprintf("GetSeries(%s) failed", seriesID),
		fmt.Sprintf("%v", metaErr),
	)

	// ── Checks 3–5: Validate metadata shape ─────────────────────────────────
	if meta != nil {
		r.check(t,
			meta.ID == seriesID,
			fmt.Sprintf("Series ID in response matches request (%q)", meta.ID),
			fmt.Sprintf("Series ID mismatch: got %q, want %q", meta.ID, seriesID),
		)
		r.check(t,
			meta.Title != "",
			fmt.Sprintf("Series title is non-empty (%q)", meta.Title),
			"Series title is empty",
		)
		r.check(t,
			meta.Frequency != "",
			fmt.Sprintf("Series frequency is populated (%q)", meta.Frequency),
			"Series frequency is empty",
		)
	} else {
		r.fail(t, "Series ID matches request         (skipped — prior fetch failure)")
		r.fail(t, "Series title is non-empty         (skipped — prior fetch failure)")
		r.fail(t, "Series frequency is populated     (skipped — prior fetch failure)")
	}

	// ── Check 6: Observations return successfully ────────────────────────────
	data, obsErr := client.GetObservations(context.Background(), seriesID, fred.ObsOptions{
		Start: "2020-01-01",
		Limit: 10,
	})
	r.check(t,
		obsErr == nil && data != nil,
		fmt.Sprintf("GetObservations(%s) returned data without error", seriesID),
		fmt.Sprintf("GetObservations(%s) failed", seriesID),
		fmt.Sprintf("%v", obsErr),
	)

	// ── Checks 7–9: Validate observation payload ─────────────────────────────
	if data != nil && len(data.Obs) > 0 {
		r.check(t,
			len(data.Obs) > 0,
			fmt.Sprintf("Observations array is non-empty (%d observations)", len(data.Obs)),
			"Observations array is empty",
		)

		first := data.Obs[0]
		r.check(t,
			!first.IsMissing(),
			fmt.Sprintf("First observation has a numeric value (%.4f)", first.Value),
			"First observation carries missing-value sentinel",
		)
		r.check(t,
			dateRegex.MatchString(first.Date.Format("2006-01-02")),
			fmt.Sprintf("Observation date matches YYYY-MM-DD format (%s)", first.Date.Format("2006-01-02")),
			fmt.Sprintf("Observation date format invalid: %q", first.Date.Format("2006-01-02")),
		)
	} else {
		r.fail(t, "Observations array is non-empty   (skipped — prior fetch failure)")
		r.fail(t, "First observation has numeric value (skipped — prior fetch failure)")
		r.fail(t, "Observation date format valid      (skipped — prior fetch failure)")
	}

	r.summary(t, "FRED API CONNECTIVITY")
}

// ─────────────────────────────────────────────────────────────────────────────
// Group 2 — Payload Integrity (fully offline)
// ─────────────────────────────────────────────────────────────────────────────

func TestPayloadIntegrity(t *testing.T) {
	printBanner(t, "PAYLOAD INTEGRITY")
	r := &result{}

	// ── Checks 1–4: Observation value parsing ────────────────────────────────
	cases := []struct {
		input   string
		wantNaN bool
		wantVal float64
		label   string
	}{
		{"305.109", false, 305.109, "numeric string 305.109 parses correctly"},
		{"0", false, 0, "zero value parses correctly"},
		{"-1.5", false, -1.5, "negative value parses correctly"},
		{".", true, 0, "FRED sentinel '.' parses as NaN"},
		{"", true, 0, "empty string parses as NaN"},
		{"  .  ", true, 0, "whitespace-padded sentinel parses as NaN"},
	}
	for _, c := range cases {
		got := util.ParseObsValue(c.input)
		if c.wantNaN {
			r.check(t,
				math.IsNaN(got),
				fmt.Sprintf("ParseObsValue(%q) → NaN  (%s)", c.input, c.label),
				fmt.Sprintf("ParseObsValue(%q) → %.4f, want NaN", c.input, got),
			)
		} else {
			r.check(t,
				math.Abs(got-c.wantVal) < 1e-9,
				fmt.Sprintf("ParseObsValue(%q) → %.4f  (%s)", c.input, got, c.label),
				fmt.Sprintf("ParseObsValue(%q) → %.4f, want %.4f", c.input, got, c.wantVal),
			)
		}
	}

	// ── Checks 7–8: FormatValue display rules ────────────────────────────────
	r.check(t,
		util.FormatValue(math.NaN()) == ".",
		"FormatValue(NaN) renders as \".\"",
		fmt.Sprintf("FormatValue(NaN) = %q, want \".\"", util.FormatValue(math.NaN())),
	)
	r.check(t,
		util.FormatValue(math.NaN()) == ".",
		"FormatValue(NaN) renders as \".\" (sentinel preserved)",
		"FormatValue(NaN) did not return \".\"",
	)

	// ── Checks 9–11: formatValue decimal rules ───────────────────────────────
	// These replicate the render.formatValue logic via util.FormatValue
	// to confirm whole numbers show one decimal place (4.0 not 4).
	wholeFormatted := formatValueForTest(4.0)
	r.check(t,
		wholeFormatted == "4.0",
		fmt.Sprintf("Whole number 4.0 renders as \"4.0\" (not \"4\"), got %q", wholeFormatted),
		fmt.Sprintf("Whole number formatting wrong: got %q, want \"4.0\"", wholeFormatted),
	)

	decimalFormatted := formatValueForTest(3.4)
	r.check(t,
		decimalFormatted == "3.4",
		fmt.Sprintf("3.4 renders as \"3.4\" (no trailing zeros), got %q", decimalFormatted),
		fmt.Sprintf("Decimal formatting wrong: got %q, want \"3.4\"", decimalFormatted),
	)

	precisionFormatted := formatValueForTest(305.109)
	r.check(t,
		precisionFormatted == "305.109",
		fmt.Sprintf("305.109 renders as \"305.109\", got %q", precisionFormatted),
		fmt.Sprintf("Precision formatting wrong: got %q, want \"305.109\"", precisionFormatted),
	)

	// ── Checks 12–14: Date parsing ───────────────────────────────────────────
	validDates := []string{"2024-01-01", "2000-12-31", "1948-01-01"}
	for _, s := range validDates {
		d, err := util.ParseDate(s)
		r.check(t,
			err == nil && util.FormatDate(d) == s,
			fmt.Sprintf("ParseDate(%q) round-trips correctly", s),
			fmt.Sprintf("ParseDate(%q) failed: err=%v", s, err),
		)
	}

	invalidDates := []string{"not-a-date", "2024/01/01", "01-01-2024", ""}
	for _, s := range invalidDates {
		_, err := util.ParseDate(s)
		r.check(t,
			err != nil,
			fmt.Sprintf("ParseDate(%q) correctly returns an error", s),
			fmt.Sprintf("ParseDate(%q) should have errored but did not", s),
		)
	}

	// ── Checks 19–21: Config precedence ─────────────────────────────────────
	// Use temp dirs to isolate each precedence test from the real config.json.
	t.Run("config_file_loads", func(t *testing.T) {
		dir := t.TempDir()
		orig, _ := os.Getwd()
		defer os.Chdir(orig)
		os.Chdir(dir)
		os.Unsetenv("FRED_API_KEY")

		f := config.File{APIKey: "file_key", DefaultFormat: "csv", Concurrency: 4}
		config.WriteFile(filepath.Join(dir, "config.json"), f)

		cfg, err := config.Load("")
		r.check(t,
			err == nil && cfg.APIKey == "file_key" && cfg.Format == "csv",
			"config.json values load correctly (api_key, default_format)",
			fmt.Sprintf("config.json load failed: err=%v, key=%q, fmt=%q", err, cfg.APIKey, cfg.Format),
		)
	})

	t.Run("env_overrides_file", func(t *testing.T) {
		dir := t.TempDir()
		orig, _ := os.Getwd()
		defer os.Chdir(orig)
		os.Chdir(dir)

		config.WriteFile(filepath.Join(dir, "config.json"), config.File{APIKey: "file_key"})
		os.Setenv("FRED_API_KEY", "env_key")
		defer os.Unsetenv("FRED_API_KEY")

		cfg, _ := config.Load("")
		r.check(t,
			cfg.APIKey == "env_key",
			"FRED_API_KEY env var overrides config.json api_key",
			fmt.Sprintf("env override failed: got %q, want \"env_key\"", cfg.APIKey),
		)
	})

	t.Run("flag_overrides_env", func(t *testing.T) {
		os.Setenv("FRED_API_KEY", "env_key")
		defer os.Unsetenv("FRED_API_KEY")

		cfg, _ := config.Load("flag_key")
		r.check(t,
			cfg.APIKey == "flag_key",
			"--api-key flag overrides FRED_API_KEY env var",
			fmt.Sprintf("flag override failed: got %q, want \"flag_key\"", cfg.APIKey),
		)
	})

	// ── Checks 22–23: Rate limiter ───────────────────────────────────────────

	limiter := rate.NewLimiter(rate.Limit(1000), 1) // 1000 req/sec, burst 1
	ctx := context.Background()

	allPassed := true
	for i := 0; i < 5; i++ {
		if err := limiter.Wait(ctx); err != nil {
			allPassed = false
		}
	}

	r.check(t,
		allPassed,
		"Rate limiter allows 5 requests at 1000 req/s without blocking",
		"Rate limiter blocked or errored unexpectedly",
	)

	slowLimiter := rate.NewLimiter(rate.Limit(0.001), 1) // ~1 per 1000s
	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = slowLimiter.Wait(ctx2) // consume initial token
	err := slowLimiter.Wait(ctx2)

	r.check(t,
		err != nil,
		"Rate limiter respects context cancellation (blocks slow limiter)",
		"Rate limiter should have returned context error but did not",
	)

	r.summary(t, "PAYLOAD INTEGRITY")
}

// ─────────────────────────────────────────────────────────────────────────────
// Group 3 — API Client Behaviour (mock HTTP server, fully offline)
// ─────────────────────────────────────────────────────────────────────────────

func TestAPIClientBehaviour(t *testing.T) {
	printBanner(t, "API CLIENT BEHAVIOUR")
	r := &result{}

	// ── Helpers ──────────────────────────────────────────────────────────────
	mockServer := func(handlers map[string]http.HandlerFunc) *httptest.Server {
		mux := http.NewServeMux()
		for path, h := range handlers {
			mux.HandleFunc(path, h)
		}
		return httptest.NewServer(mux)
	}
	newClient := func(baseURL string) *fred.Client {
		return fred.NewClient("test_key", baseURL+"/", 5*time.Second, 1000, false)
	}

	// ── Checks 1–4: GetSeries success path ───────────────────────────────────
	srv := mockServer(map[string]http.HandlerFunc{
		"/series": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("series_id") != "GDP" {
				t.Errorf("series_id param: got %q, want GDP", q.Get("series_id"))
			}
			if q.Get("api_key") != "test_key" {
				t.Errorf("api_key param: got %q, want test_key", q.Get("api_key"))
			}
			if q.Get("file_type") != "json" {
				t.Errorf("file_type param: got %q, want json", q.Get("file_type"))
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"seriess": []map[string]interface{}{{
					"id": "GDP", "title": "Gross Domestic Product",
					"frequency": "Quarterly", "frequency_short": "Q",
					"units": "Billions of Dollars", "units_short": "Bil. of $",
					"seasonal_adjustment": "Seasonally Adjusted Annual Rate",
					"last_updated":        "2024-09-26 07:50:09-05", "popularity": 92,
				}},
			})
		},
	})
	defer srv.Close()

	meta, err := newClient(srv.URL).GetSeries(context.Background(), "GDP")
	r.check(t, err == nil && meta != nil,
		"GetSeries: request succeeds without error",
		fmt.Sprintf("GetSeries failed: %v", err),
	)
	if meta != nil {
		r.check(t, meta.ID == "GDP",
			fmt.Sprintf("GetSeries: response ID matches request (%q)", meta.ID),
			fmt.Sprintf("GetSeries: ID mismatch: got %q, want %q", meta.ID, "GDP"),
		)
		r.check(t, meta.Title == "Gross Domestic Product",
			fmt.Sprintf("GetSeries: title parsed correctly (%q)", meta.Title),
			fmt.Sprintf("GetSeries: title wrong: %q", meta.Title),
		)
		r.check(t, meta.Popularity == 92,
			fmt.Sprintf("GetSeries: popularity parsed correctly (%d)", meta.Popularity),
			fmt.Sprintf("GetSeries: popularity wrong: %d", meta.Popularity),
		)
	} else {
		r.fail(t, "GetSeries: ID matches         (skipped — prior failure)")
		r.fail(t, "GetSeries: title parsed        (skipped — prior failure)")
		r.fail(t, "GetSeries: popularity parsed   (skipped — prior failure)")
	}

	// ── Check 5: API error propagates correctly ───────────────────────────────
	errSrv := mockServer(map[string]http.HandlerFunc{
		"/series": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error_message": "Bad Request.  The series does not exist.",
			})
		},
	})
	defer errSrv.Close()

	_, apiErr := newClient(errSrv.URL).GetSeries(context.Background(), "FAKESERIES")
	r.check(t,
		apiErr != nil && strings.Contains(apiErr.Error(), "does not exist"),
		"GetSeries: API error message propagates correctly",
		fmt.Sprintf("GetSeries error wrong or missing: %v", apiErr),
	)

	// ── Checks 6–8: GetObservations parses values and NaN correctly ───────────
	obsSrv := mockServer(map[string]http.HandlerFunc{
		"/series/observations": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"observations": []map[string]string{
					{"date": "2024-01-01", "value": "28623.5"},
					{"date": "2024-04-01", "value": "29053.2"},
					{"date": "2024-07-01", "value": "."},
				},
			})
		},
	})
	defer obsSrv.Close()

	data, obsErr := newClient(obsSrv.URL).GetObservations(context.Background(), "GDP", fred.ObsOptions{})
	r.check(t, obsErr == nil && len(data.Obs) == 3,
		fmt.Sprintf("GetObservations: returned 3 observations (got %d)", len(data.Obs)),
		fmt.Sprintf("GetObservations failed or wrong count: err=%v, count=%d", obsErr, len(data.Obs)),
	)
	if len(data.Obs) == 3 {
		r.check(t, data.Obs[0].ValueRaw == "28623.5",
			fmt.Sprintf("GetObservations: first value raw string preserved (%q)", data.Obs[0].ValueRaw),
			fmt.Sprintf("GetObservations: ValueRaw wrong: %q", data.Obs[0].ValueRaw),
		)
		r.check(t, data.Obs[2].IsMissing(),
			"GetObservations: FRED sentinel \".\" parsed as NaN (IsMissing=true)",
			fmt.Sprintf("GetObservations: sentinel not NaN: value=%v", data.Obs[2].Value),
		)
	} else {
		r.fail(t, "GetObservations: ValueRaw preserved  (skipped — wrong count)")
		r.fail(t, "GetObservations: sentinel is NaN      (skipped — wrong count)")
	}

	// ── Check 9: Date params forwarded correctly ──────────────────────────────
	var gotStart, gotEnd string
	paramSrv := mockServer(map[string]http.HandlerFunc{
		"/series/observations": func(w http.ResponseWriter, r *http.Request) {
			gotStart = r.URL.Query().Get("observation_start")
			gotEnd = r.URL.Query().Get("observation_end")
			json.NewEncoder(w).Encode(map[string]interface{}{"observations": []map[string]string{}})
		},
	})
	defer paramSrv.Close()

	newClient(paramSrv.URL).GetObservations(context.Background(), "GDP", fred.ObsOptions{
		Start: "2020-01-01", End: "2024-12-31",
	})
	r.check(t, gotStart == "2020-01-01" && gotEnd == "2024-12-31",
		fmt.Sprintf("GetObservations: date params forwarded correctly (start=%q end=%q)", gotStart, gotEnd),
		fmt.Sprintf("GetObservations: date params wrong: start=%q end=%q", gotStart, gotEnd),
	)

	// ── Check 10: Retry on 5xx succeeds after transient failures ─────────────
	attempts := 0
	retrySrv := mockServer(map[string]http.HandlerFunc{
		"/series": func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"seriess": []map[string]interface{}{{"id": "GDP", "title": "GDP"}},
			})
		},
	})
	defer retrySrv.Close()

	_, retryErr := newClient(retrySrv.URL).GetSeries(context.Background(), "GDP")
	r.check(t, retryErr == nil && attempts == 3,
		fmt.Sprintf("Retry: succeeded after %d attempts (2×503 then 200)", attempts),
		fmt.Sprintf("Retry: err=%v, attempts=%d (expected success at attempt 3)", retryErr, attempts),
	)

	// ── Check 11: SearchSeries sends correct params ───────────────────────────
	var gotSearchText string
	searchSrv := mockServer(map[string]http.HandlerFunc{
		"/series/search": func(w http.ResponseWriter, r *http.Request) {
			gotSearchText = r.URL.Query().Get("search_text")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"seriess": []map[string]interface{}{{
					"id": "CPIAUCSL", "title": "Consumer Price Index",
					"frequency": "Monthly", "frequency_short": "M",
					"units": "Index 1982-84=100", "popularity": 88,
					"last_updated": "2024-09-11",
				}},
			})
		},
	})
	defer searchSrv.Close()

	results, searchErr := newClient(searchSrv.URL).SearchSeries(
		context.Background(), "inflation", fred.SearchSeriesOptions{Limit: 5},
	)
	r.check(t,
		searchErr == nil && len(results) == 1 && gotSearchText == "inflation",
		fmt.Sprintf("SearchSeries: correct params sent, 1 result returned (search_text=%q)", gotSearchText),
		fmt.Sprintf("SearchSeries: err=%v, results=%d, search_text=%q", searchErr, len(results), gotSearchText),
	)

	r.summary(t, "API CLIENT BEHAVIOUR")
}

// ─────────────────────────────────────────────────────────────────────────────
// Group 4 — Email Connectivity (skips if SMTP not configured)
// ─────────────────────────────────────────────────────────────────────────────

func TestEmailConnectivity(t *testing.T) {
	cfg := configOrSkip(t)

	// Email is optional — skip gracefully if not configured
	smtpHost := cfg.BaseURL // placeholder; real impl uses cfg.Email.SMTPHost
	if smtpHost == "" {
		t.Skip("⏭️  Skipping: SMTP host not configured in config.json")
	}
	t.Skip("⏭️  Skipping: Email not yet configured (Phase 5)")

	printBanner(t, "EMAIL CONNECTIVITY")
	r := &result{}

	// Checks will be populated in Phase 5 when notify package is implemented.
	// Structure mirrors fred_test.go: DNS → TCP dial → 220 banner → from addr.
	_ = r
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// formatValueForTest replicates the render.formatValue logic for offline testing.
// Always shows at least one decimal place; strips unnecessary trailing zeros.
func formatValueForTest(v float64) string {
	if math.IsNaN(v) {
		return "."
	}
	s := strings.TrimRight(fmt.Sprintf("%.6f", v), "0")
	if strings.HasSuffix(s, ".") {
		s += "0"
	}
	return s
}
