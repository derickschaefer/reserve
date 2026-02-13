package fred

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/derickschaefer/reserve/internal/model"
)

// ListReleases fetches all FRED data releases.
func (c *Client) ListReleases(ctx context.Context, limit int) ([]model.Release, error) {
	params := url.Values{}
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
	if err := c.get(ctx, "releases", params, &raw); err != nil {
		return nil, fmt.Errorf("releases list: %w", err)
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

// GetRelease fetches metadata for a single release.
func (c *Client) GetRelease(ctx context.Context, releaseID int) (*model.Release, error) {
	params := url.Values{}
	params.Set("release_id", strconv.Itoa(releaseID))

	var raw struct {
		Releases []struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			PressRelease bool   `json:"press_release"`
			Link         string `json:"link"`
			Notes        string `json:"notes"`
		} `json:"releases"`
	}
	if err := c.get(ctx, "release", params, &raw); err != nil {
		return nil, fmt.Errorf("release %d: %w", releaseID, err)
	}
	if len(raw.Releases) == 0 {
		return nil, fmt.Errorf("release not found: %d", releaseID)
	}
	r := raw.Releases[0]
	return &model.Release{
		ID:           r.ID,
		Name:         r.Name,
		PressRelease: r.PressRelease,
		Link:         r.Link,
		Notes:        r.Notes,
	}, nil
}

// ReleaseDate represents a single release date record.
type ReleaseDate struct {
	ReleaseID   int    `json:"release_id"`
	ReleaseName string `json:"release_name"`
	Date        string `json:"date"`
}

// GetReleaseDates fetches the scheduled/actual release dates for a release.
func (c *Client) GetReleaseDates(ctx context.Context, releaseID int, limit int) ([]ReleaseDate, error) {
	params := url.Values{}
	params.Set("release_id", strconv.Itoa(releaseID))
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	} else {
		params.Set("limit", "20")
	}

	var raw struct {
		ReleaseDates []struct {
			ReleaseID   int    `json:"release_id"`
			ReleaseName string `json:"release_name"`
			Date        string `json:"date"`
		} `json:"release_dates"`
	}
	if err := c.get(ctx, "release/dates", params, &raw); err != nil {
		return nil, fmt.Errorf("release dates %d: %w", releaseID, err)
	}

	dates := make([]ReleaseDate, len(raw.ReleaseDates))
	for i, d := range raw.ReleaseDates {
		dates[i] = ReleaseDate{
			ReleaseID:   d.ReleaseID,
			ReleaseName: d.ReleaseName,
			Date:        d.Date,
		}
	}
	return dates, nil
}

// GetReleaseSeries fetches the series belonging to a release.
func (c *Client) GetReleaseSeries(ctx context.Context, releaseID int, limit int) ([]model.SeriesMeta, error) {
	params := url.Values{}
	params.Set("release_id", strconv.Itoa(releaseID))
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	} else {
		params.Set("limit", "20")
	}

	var raw struct {
		Seriess []rawSeriesMeta `json:"seriess"`
	}
	if err := c.get(ctx, "release/series", params, &raw); err != nil {
		return nil, fmt.Errorf("release series %d: %w", releaseID, err)
	}

	result := make([]model.SeriesMeta, len(raw.Seriess))
	for i, s := range raw.Seriess {
		result[i] = normalizeSeriesMeta(s)
	}
	return result, nil
}
