package fred

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/derickschaefer/reserve/internal/model"
)

// GetCategory fetches metadata for a single category by ID.
// Use ID 0 for the root category.
func (c *Client) GetCategory(ctx context.Context, categoryID int) (*model.Category, error) {
	params := url.Values{}
	params.Set("category_id", strconv.Itoa(categoryID))

	var raw struct {
		Categories []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			ParentID int    `json:"parent_id"`
		} `json:"categories"`
	}
	if err := c.get(ctx, "category", params, &raw); err != nil {
		return nil, fmt.Errorf("category %d: %w", categoryID, err)
	}
	if len(raw.Categories) == 0 {
		return nil, fmt.Errorf("category not found: %d", categoryID)
	}
	cat := raw.Categories[0]
	return &model.Category{ID: cat.ID, Name: cat.Name, ParentID: cat.ParentID}, nil
}

// GetCategoryChildren fetches the direct children of a category.
func (c *Client) GetCategoryChildren(ctx context.Context, categoryID int) ([]model.Category, error) {
	params := url.Values{}
	params.Set("category_id", strconv.Itoa(categoryID))

	var raw struct {
		Categories []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			ParentID int    `json:"parent_id"`
		} `json:"categories"`
	}
	if err := c.get(ctx, "category/children", params, &raw); err != nil {
		return nil, fmt.Errorf("category children %d: %w", categoryID, err)
	}

	cats := make([]model.Category, len(raw.Categories))
	for i, cat := range raw.Categories {
		cats[i] = model.Category{ID: cat.ID, Name: cat.Name, ParentID: cat.ParentID}
	}
	return cats, nil
}

// CategorySeriesOptions holds optional filters for GetCategorySeries.
type CategorySeriesOptions struct {
	Limit  int
	Offset int
	Filter string // optional filter expression (passed as filter_variable/filter_value)
}

// GetCategorySeries fetches the series belonging to a category.
func (c *Client) GetCategorySeries(ctx context.Context, categoryID int, opts CategorySeriesOptions) ([]model.SeriesMeta, error) {
	params := url.Values{}
	params.Set("category_id", strconv.Itoa(categoryID))
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	} else {
		params.Set("limit", "20")
	}
	if opts.Offset > 0 {
		params.Set("offset", strconv.Itoa(opts.Offset))
	}
	// Simple filter: if the filter string contains "=" we split into variable/value
	if opts.Filter != "" {
		parts := strings.SplitN(opts.Filter, "=", 2)
		if len(parts) == 2 {
			params.Set("filter_variable", strings.TrimSpace(parts[0]))
			params.Set("filter_value", strings.TrimSpace(parts[1]))
		}
	}

	var raw struct {
		Seriess []rawSeriesMeta `json:"seriess"`
	}
	if err := c.get(ctx, "category/series", params, &raw); err != nil {
		return nil, fmt.Errorf("category series %d: %w", categoryID, err)
	}

	result := make([]model.SeriesMeta, len(raw.Seriess))
	for i, s := range raw.Seriess {
		result[i] = normalizeSeriesMeta(s)
	}
	return result, nil
}
