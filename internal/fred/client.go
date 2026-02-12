// Package fred implements the HTTP client for the Federal Reserve Bank of
// St. Louis (FRED) API. All methods are context-aware, respect the shared
// rate limiter, and retry on transient errors (429, 5xx).
package fred

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/util"
)

const (
	defaultBaseURL = "https://api.stlouisfed.org/fred/"
	maxRetries     = 4
)

// Client is the FRED API HTTP client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	limiter    *rate.Limiter
	debug      bool
}

// NewClient creates a Client with the given API key and timeout.
func NewClient(apiKey, baseURL string, timeout time.Duration, ratePerSec float64, debug bool) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	burst := int(ratePerSec)
	if burst < 1 {
		burst = 1
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		limiter: rate.NewLimiter(rate.Limit(ratePerSec), burst),
		debug:   debug,
	}
}

// ─── Observations ─────────────────────────────────────────────────────────────

// ObsOptions holds optional parameters for GetObservations.
type ObsOptions struct {
	Start string // YYYY-MM-DD
	End   string // YYYY-MM-DD
	Freq  string // daily|weekly|monthly|quarterly|annual
	Units string // lin|chg|ch1|pch|pc1|pca|cch|cca|log
	Agg   string // avg|sum|eop
	Limit int
}

// freqMap maps CLI-friendly frequency names to FRED API values.
var freqMap = map[string]string{
	"daily":     "d",
	"weekly":    "w",
	"monthly":   "m",
	"quarterly": "q",
	"annual":    "a",
	// pass-through short codes
	"d": "d", "w": "w", "m": "m", "q": "q", "a": "a",
}

// aggMap maps CLI-friendly aggregation names to FRED API values.
var aggMap = map[string]string{
	"avg": "avg", "average": "avg",
	"sum": "sum",
	"eop": "eop", "end": "eop",
}

// GetObservations fetches time series observations for a single series.
func (c *Client) GetObservations(ctx context.Context, seriesID string, opts ObsOptions) (*model.SeriesData, error) {
	params := url.Values{}
	params.Set("series_id", strings.ToUpper(seriesID))
	if opts.Start != "" {
		params.Set("observation_start", opts.Start)
	}
	if opts.End != "" {
		params.Set("observation_end", opts.End)
	}
	if opts.Freq != "" {
		if v, ok := freqMap[strings.ToLower(opts.Freq)]; ok {
			params.Set("frequency", v)
		}
	}
	if opts.Units != "" {
		params.Set("units", strings.ToLower(opts.Units))
	}
	if opts.Agg != "" {
		if v, ok := aggMap[strings.ToLower(opts.Agg)]; ok {
			params.Set("aggregation_method", v)
		}
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}

	var raw struct {
		Observations []struct {
			Date          string `json:"date"`
			Value         string `json:"value"`
			RealtimeStart string `json:"realtime_start"`
			RealtimeEnd   string `json:"realtime_end"`
		} `json:"observations"`
	}

	if err := c.get(ctx, "series/observations", params, &raw); err != nil {
		return nil, fmt.Errorf("observations %s: %w", seriesID, err)
	}

	obs := make([]model.Observation, 0, len(raw.Observations))
	for _, o := range raw.Observations {
		date, err := util.ParseDate(o.Date)
		if err != nil {
			continue // skip malformed dates
		}
		obs = append(obs, model.Observation{
			Date:          date,
			Value:         util.ParseObsValue(o.Value),
			ValueRaw:      o.Value,
			RealtimeStart: o.RealtimeStart,
			RealtimeEnd:   o.RealtimeEnd,
		})
	}

	return &model.SeriesData{
		SeriesID: strings.ToUpper(seriesID),
		Obs:      obs,
	}, nil
}

// GetLatestObservation returns the most recent observation for a series.
func (c *Client) GetLatestObservation(ctx context.Context, seriesID string) (*model.Observation, error) {
	params := url.Values{}
	params.Set("series_id", strings.ToUpper(seriesID))
	params.Set("sort_order", "desc")
	params.Set("limit", "1")

	var raw struct {
		Observations []struct {
			Date  string `json:"date"`
			Value string `json:"value"`
		} `json:"observations"`
	}
	if err := c.get(ctx, "series/observations", params, &raw); err != nil {
		return nil, fmt.Errorf("latest observation %s: %w", seriesID, err)
	}
	if len(raw.Observations) == 0 {
		return nil, fmt.Errorf("no observations found for %s", seriesID)
	}
	o := raw.Observations[0]
	date, err := util.ParseDate(o.Date)
	if err != nil {
		return nil, err
	}
	obs := &model.Observation{
		Date:     date,
		Value:    util.ParseObsValue(o.Value),
		ValueRaw: o.Value,
	}
	return obs, nil
}

// ─── Series ───────────────────────────────────────────────────────────────────

// GetSeries fetches metadata for one or more series IDs.
func (c *Client) GetSeries(ctx context.Context, seriesID string) (*model.SeriesMeta, error) {
	params := url.Values{}
	params.Set("series_id", strings.ToUpper(seriesID))

	var raw struct {
		Seriess []rawSeriesMeta `json:"seriess"`
	}
	if err := c.get(ctx, "series", params, &raw); err != nil {
		return nil, fmt.Errorf("series %s: %w", seriesID, err)
	}
	if len(raw.Seriess) == 0 {
		return nil, fmt.Errorf("series not found: %s", seriesID)
	}
	m := normalizeSeriesMeta(raw.Seriess[0])
	return &m, nil
}

// SearchSeriesOptions holds options for SearchSeries.
type SearchSeriesOptions struct {
	Tags   []string
	Source int
	Limit  int
	Offset int
}

// SearchSeries searches for series matching query.
func (c *Client) SearchSeries(ctx context.Context, query string, opts SearchSeriesOptions) ([]model.SeriesMeta, error) {
	params := url.Values{}
	params.Set("search_text", query)
	params.Set("search_type", "full_text")
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	} else {
		params.Set("limit", "20")
	}
	if opts.Offset > 0 {
		params.Set("offset", strconv.Itoa(opts.Offset))
	}
	if len(opts.Tags) > 0 {
		params.Set("tag_names", strings.Join(opts.Tags, ";"))
	}

	var raw struct {
		Seriess []rawSeriesMeta `json:"seriess"`
	}
	if err := c.get(ctx, "series/search", params, &raw); err != nil {
		return nil, fmt.Errorf("series search %q: %w", query, err)
	}

	result := make([]model.SeriesMeta, len(raw.Seriess))
	for i, s := range raw.Seriess {
		result[i] = normalizeSeriesMeta(s)
	}
	return result, nil
}

// GetSeriesTags returns the tags associated with a series.
func (c *Client) GetSeriesTags(ctx context.Context, seriesID string) ([]model.Tag, error) {
	params := url.Values{}
	params.Set("series_id", strings.ToUpper(seriesID))

	var raw struct {
		Tags []struct {
			Name        string `json:"name"`
			GroupID     string `json:"group_id"`
			Notes       string `json:"notes"`
			Created     string `json:"created"`
			Popularity  int    `json:"popularity"`
			SeriesCount int    `json:"series_count"`
		} `json:"tags"`
	}
	if err := c.get(ctx, "series/tags", params, &raw); err != nil {
		return nil, fmt.Errorf("series tags %s: %w", seriesID, err)
	}

	tags := make([]model.Tag, len(raw.Tags))
	for i, t := range raw.Tags {
		tags[i] = model.Tag{
			Name:        t.Name,
			GroupID:     t.GroupID,
			Notes:       t.Notes,
			Created:     t.Created,
			Popularity:  t.Popularity,
			SeriesCount: t.SeriesCount,
		}
	}
	return tags, nil
}

// GetSeriesCategories returns the categories a series belongs to.
func (c *Client) GetSeriesCategories(ctx context.Context, seriesID string) ([]model.Category, error) {
	params := url.Values{}
	params.Set("series_id", strings.ToUpper(seriesID))

	var raw struct {
		Categories []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			ParentID int    `json:"parent_id"`
		} `json:"categories"`
	}
	if err := c.get(ctx, "series/categories", params, &raw); err != nil {
		return nil, fmt.Errorf("series categories %s: %w", seriesID, err)
	}

	cats := make([]model.Category, len(raw.Categories))
	for i, cat := range raw.Categories {
		cats[i] = model.Category{ID: cat.ID, Name: cat.Name, ParentID: cat.ParentID}
	}
	return cats, nil
}

// ─── Low-level HTTP ───────────────────────────────────────────────────────────

// get performs a GET request to the FRED API, handling rate limiting and retries.
func (c *Client) get(ctx context.Context, endpoint string, params url.Values, out interface{}) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}

	params.Set("api_key", c.apiKey)
	params.Set("file_type", "json")

	reqURL := c.baseURL + endpoint + "?" + params.Encode()

	if c.debug {
		// Log URL with API key redacted
		safe := strings.Replace(reqURL, c.apiKey, "REDACTED", 1)
		slog.Debug("fred request", "url", safe)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))*500) * time.Millisecond
			slog.Debug("retrying after backoff", "attempt", attempt, "backoff", backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return fmt.Errorf("building request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "reserve-cli/1.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("reading body: %w", err)
			continue
		}

		if c.debug {
			slog.Debug("fred response", "status", resp.StatusCode, "bytes", len(body))
		}

		// Retry on server errors and rate limiting
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			// Try to extract FRED error message
			var apiErr struct {
				Error string `json:"error_message"`
			}
			_ = json.Unmarshal(body, &apiErr)
			if apiErr.Error != "" {
				return fmt.Errorf("API error: %s", apiErr.Error)
			}
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
		return nil
	}
	return fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

type rawSeriesMeta struct {
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
}

func normalizeSeriesMeta(r rawSeriesMeta) model.SeriesMeta {
	return model.SeriesMeta{
		ID:                      r.ID,
		Title:                   r.Title,
		ObservationStart:        r.ObservationStart,
		ObservationEnd:          r.ObservationEnd,
		Frequency:               r.Frequency,
		FrequencyShort:          r.FrequencyShort,
		Units:                   r.Units,
		UnitsShort:              r.UnitsShort,
		SeasonalAdjustment:      r.SeasonalAdjustment,
		SeasonalAdjustmentShort: r.SeasonalAdjustmentShort,
		LastUpdated:             r.LastUpdated,
		Popularity:              r.Popularity,
		Notes:                   r.Notes,
		FetchedAt:               time.Now(),
	}
}
