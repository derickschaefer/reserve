package fred

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/derickschaefer/reserve/internal/model"
)

// SearchTagsOptions holds options for SearchTags.
type SearchTagsOptions struct {
	Limit int
}

// SearchTags searches for FRED tags matching a query string.
func (c *Client) SearchTags(ctx context.Context, query string, opts SearchTagsOptions) ([]model.Tag, error) {
	params := url.Values{}
	params.Set("search_text", query)
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	} else {
		params.Set("limit", "20")
	}

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
	if err := c.get(ctx, "tags", params, &raw); err != nil {
		return nil, fmt.Errorf("tag search %q: %w", query, err)
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

// GetTagSeriesOptions holds options for GetTagSeries.
type GetTagSeriesOptions struct {
	MatchAll bool // if true, series must have ALL tags; otherwise ANY
	Limit    int
}

// GetTagSeries fetches series associated with one or more tags.
func (c *Client) GetTagSeries(ctx context.Context, tagNames []string, opts GetTagSeriesOptions) ([]model.SeriesMeta, error) {
	if len(tagNames) == 0 {
		return nil, fmt.Errorf("at least one tag name required")
	}

	params := url.Values{}
	params.Set("tag_names", strings.Join(tagNames, ";"))
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	} else {
		params.Set("limit", "20")
	}

	var raw struct {
		Seriess []rawSeriesMeta `json:"seriess"`
	}
	if err := c.get(ctx, "tags/series", params, &raw); err != nil {
		return nil, fmt.Errorf("tag series %v: %w", tagNames, err)
	}

	result := make([]model.SeriesMeta, len(raw.Seriess))
	for i, s := range raw.Seriess {
		result[i] = normalizeSeriesMeta(s)
	}
	return result, nil
}

// GetRelatedTags fetches tags related to a given tag.
func (c *Client) GetRelatedTags(ctx context.Context, tagName string, limit int) ([]model.Tag, error) {
	params := url.Values{}
	params.Set("tag_names", tagName)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	} else {
		params.Set("limit", "20")
	}

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
	if err := c.get(ctx, "related_tags", params, &raw); err != nil {
		return nil, fmt.Errorf("related tags %q: %w", tagName, err)
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
