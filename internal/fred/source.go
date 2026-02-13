package fred

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/derickschaefer/reserve/internal/model"
)

// ListSources fetches all FRED data sources.
func (c *Client) ListSources(ctx context.Context, limit int) ([]model.Source, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	} else {
		params.Set("limit", "1000")
	}

	var raw struct {
		Sources []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Link  string `json:"link"`
			Notes string `json:"notes"`
		} `json:"sources"`
	}
	if err := c.get(ctx, "sources", params, &raw); err != nil {
		return nil, fmt.Errorf("sources list: %w", err)
	}

	sources := make([]model.Source, len(raw.Sources))
	for i, s := range raw.Sources {
		sources[i] = model.Source{ID: s.ID, Name: s.Name, Link: s.Link, Notes: s.Notes}
	}
	return sources, nil
}

// GetSource fetches metadata for a single data source.
func (c *Client) GetSource(ctx context.Context, sourceID int) (*model.Source, error) {
	params := url.Values{}
	params.Set("source_id", strconv.Itoa(sourceID))

	var raw struct {
		Sources []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Link  string `json:"link"`
			Notes string `json:"notes"`
		} `json:"sources"`
	}
	if err := c.get(ctx, "source", params, &raw); err != nil {
		return nil, fmt.Errorf("source %d: %w", sourceID, err)
	}
	if len(raw.Sources) == 0 {
		return nil, fmt.Errorf("source not found: %d", sourceID)
	}
	s := raw.Sources[0]
	return &model.Source{ID: s.ID, Name: s.Name, Link: s.Link, Notes: s.Notes}, nil
}

// GetSourceReleases fetches the releases published by a source.
func (c *Client) GetSourceReleases(ctx context.Context, sourceID int, limit int) ([]model.Release, error) {
	params := url.Values{}
	params.Set("source_id", strconv.Itoa(sourceID))
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	} else {
		params.Set("limit", "1000")
	}

	var raw struct {
		Releases []struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			PressRelease bool   `json:"press_release"`
			Link         string `json:"link"`
			Notes        string `json:"notes"`
		} `json:"releases"`
	}
	if err := c.get(ctx, "source/releases", params, &raw); err != nil {
		return nil, fmt.Errorf("source releases %d: %w", sourceID, err)
	}

	releases := make([]model.Release, len(raw.Releases))
	for i, r := range raw.Releases {
		releases[i] = model.Release{
			ID:           r.ID,
			Name:         r.Name,
			PressRelease: r.PressRelease,
			Link:         r.Link,
			Notes:        r.Notes,
		}
	}
	return releases, nil
}
