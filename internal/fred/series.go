package fred

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/util"
)

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
	"daily": "d", "weekly": "w", "monthly": "m", "quarterly": "q", "annual": "a",
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
			continue
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
	return &model.Observation{
		Date:     date,
		Value:    util.ParseObsValue(o.Value),
		ValueRaw: o.Value,
	}, nil
}

// ─── Series Metadata ──────────────────────────────────────────────────────────

// GetSeries fetches metadata for a single series.
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

// GetSeriesRelated returns series related to a given series by shared tags.
func (c *Client) GetSeriesRelated(ctx context.Context, seriesID string, limit int) ([]model.SeriesMeta, error) {
	params := url.Values{}
	params.Set("series_id", strings.ToUpper(seriesID))
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	} else {
		params.Set("limit", "20")
	}

	var raw struct {
		Seriess []rawSeriesMeta `json:"seriess"`
	}
	if err := c.get(ctx, "series/related", params, &raw); err != nil {
		return nil, fmt.Errorf("series related %s: %w", seriesID, err)
	}

	result := make([]model.SeriesMeta, len(raw.Seriess))
	for i, s := range raw.Seriess {
		result[i] = normalizeSeriesMeta(s)
	}
	return result, nil
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
