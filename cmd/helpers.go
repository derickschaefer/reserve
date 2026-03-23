// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
	"github.com/olekukonko/tablewriter"
)

// normaliseIDs upper-cases all series IDs and removes duplicates while
// preserving order.
func normaliseIDs(ids []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.ToUpper(strings.TrimSpace(id))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// resolveFormat returns the effective format string, falling back to "table".
func resolveFormat(cfgFormat string) string {
	if globalFlags.Format != "" {
		return globalFlags.Format
	}
	if cfgFormat != "" {
		return cfgFormat
	}
	return render.FormatTable
}

// batchGetSeries fetches metadata for multiple series IDs concurrently.
// It respects deps.Config.Concurrency and collects errors as warnings.
func batchGetSeries(ctx context.Context, deps *app.Deps, ids []string) ([]model.SeriesMeta, []string) {
	type result struct {
		meta model.SeriesMeta
		err  error
		idx  int
	}

	concurrency := deps.Config.Concurrency
	if concurrency <= 0 {
		concurrency = 8
	}

	sem := make(chan struct{}, concurrency)
	results := make([]result, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			meta, err := deps.Client.GetSeries(ctx, id)
			if err != nil {
				results[i] = result{idx: i, err: err}
				return
			}
			results[i] = result{idx: i, meta: *meta}
		}()
	}
	wg.Wait()

	var metas []model.SeriesMeta
	var warnings []string
	for _, r := range results {
		if r.err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", ids[r.idx], r.err))
		} else {
			metas = append(metas, r.meta)
		}
	}
	return metas, warnings
}

type obsResult struct {
	data  *model.SeriesData
	err   error
	idx   int
	cache bool
}

type obsSource interface {
	name() string
	requiresAPIKey() bool
	get(context.Context, *app.Deps, string, fred.ObsOptions) (*model.SeriesData, bool, error)
}

type liveObsSource struct{}

func (liveObsSource) name() string         { return "live" }
func (liveObsSource) requiresAPIKey() bool { return true }

func (liveObsSource) get(ctx context.Context, deps *app.Deps, id string, opts fred.ObsOptions) (*model.SeriesData, bool, error) {
	data, err := deps.Client.GetObservations(ctx, id, opts)
	return data, false, err
}

type cacheObsSource struct{}

func (cacheObsSource) name() string         { return "cache" }
func (cacheObsSource) requiresAPIKey() bool { return false }

func (cacheObsSource) get(_ context.Context, deps *app.Deps, id string, opts fred.ObsOptions) (*model.SeriesData, bool, error) {
	if err := deps.RequireStore(); err != nil {
		return nil, false, fmt.Errorf("source 'cache' unavailable: %w", err)
	}

	key := obsCacheKey(id, opts)
	if key != "" {
		data, ok, err := deps.Store.GetObs(key)
		if err != nil {
			return nil, false, fmt.Errorf("reading cache: %w", err)
		}
		if ok {
			attachCachedMeta(deps, id, &data)
			return &data, true, nil
		}
		return nil, false, fmt.Errorf("no cached observations for %s matching the requested parameters", id)
	}

	keys, err := deps.Store.ListObsKeys(id)
	if err != nil {
		return nil, false, fmt.Errorf("reading cache: %w", err)
	}
	if len(keys) == 0 {
		return nil, false, fmt.Errorf("no cached observations for %s", id)
	}

	data, ok, err := deps.Store.GetObs(keys[0])
	if err != nil {
		return nil, false, fmt.Errorf("reading cache: %w", err)
	}
	if !ok {
		return nil, false, fmt.Errorf("cached observation data missing for %s", id)
	}
	attachCachedMeta(deps, id, &data)
	return &data, true, nil
}

func resolveObsSource(name string) (obsSource, error) {
	switch name {
	case "", "live":
		return liveObsSource{}, nil
	case "cache":
		return cacheObsSource{}, nil
	case "snowflake", "postgres", "s3", "http":
		return nil, fmt.Errorf("source '%s' not configured", name)
	default:
		return nil, fmt.Errorf("unknown source: %s", name)
	}
}

func obsCacheKey(seriesID string, opts fred.ObsOptions) string {
	if opts.Start == "" && opts.End == "" && opts.Freq == "" && opts.Units == "" && opts.Agg == "" {
		return ""
	}
	return storeObsKey(seriesID, opts)
}

func storeObsKey(seriesID string, opts fred.ObsOptions) string {
	return fmt.Sprintf("series:%s%s%s%s%s%s",
		seriesID,
		optionalObsKeyPart("start", opts.Start),
		optionalObsKeyPart("end", opts.End),
		optionalObsKeyPart("freq", opts.Freq),
		optionalObsKeyPart("units", opts.Units),
		optionalObsKeyPart("agg", opts.Agg),
	)
}

func optionalObsKeyPart(label, value string) string {
	if value == "" {
		return ""
	}
	return "|" + label + ":" + value
}

func attachCachedMeta(deps *app.Deps, id string, data *model.SeriesData) {
	if deps.Store == nil {
		return
	}
	if meta, ok, err := deps.Store.GetSeriesMeta(id); err == nil && ok {
		data.Meta = &meta
	}
}

// batchGetObs fetches observations for multiple series IDs concurrently.
func batchGetObs(ctx context.Context, deps *app.Deps, ids []string, opts fred.ObsOptions, src obsSource) ([]*model.SeriesData, []string, bool) {
	type result struct {
		data  *model.SeriesData
		err   error
		idx   int
		cache bool
	}

	concurrency := deps.Config.Concurrency
	if concurrency <= 0 {
		concurrency = 8
	}

	sem := make(chan struct{}, concurrency)
	results := make([]result, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			data, cache, err := src.get(ctx, deps, id, opts)
			results[i] = result{idx: i, data: data, cache: cache, err: err}
		}()
	}
	wg.Wait()

	// Return in original ID order
	var datas []*model.SeriesData
	var warnings []string
	anyCache := false
	for i, r := range results {
		if r.err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", ids[i], r.err))
		} else if r.data != nil {
			datas = append(datas, r.data)
			anyCache = anyCache || r.cache
		}
	}
	return datas, warnings, anyCache
}

// printSimpleTable renders a simple table with headers using tablewriter.
// The add callback is called with row values as variadic strings.
func printSimpleTable(w io.Writer, headers []string, fill func(add func(...string))) {
	tw := tablewriter.NewWriter(w)
	tw.SetHeader(headers)
	tw.SetBorder(true)
	tw.SetRowLine(false)
	tw.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	tw.SetAlignment(tablewriter.ALIGN_LEFT)
	tw.SetAutoWrapText(false)

	fill(func(cols ...string) {
		tw.Append(cols)
	})
	tw.Render()
}

// outputWriter returns the destination writer for command output.
// If --out is set, it opens/creates that file and returns a closer.
func outputWriter(defaultWriter io.Writer) (io.Writer, func() error, error) {
	if globalFlags.Out == "" {
		return defaultWriter, func() error { return nil }, nil
	}
	f, err := os.Create(globalFlags.Out)
	if err != nil {
		return nil, nil, fmt.Errorf("creating output file: %w", err)
	}
	return f, f.Close, nil
}

// parseIntID parses a string as a non-negative integer ID, with a descriptive label for errors.
func parseIntID(s, label string) (int, error) {
	var id int
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil || id < 0 {
		return 0, fmt.Errorf("invalid %s %q: expected a non-negative integer", label, s)
	}
	return id, nil
}

// buildSeriesMetaResult wraps a []SeriesMeta slice in a Result envelope.
func buildSeriesMetaResult(command string, metas []model.SeriesMeta) *model.Result {
	return &model.Result{
		Kind:        model.KindSeriesMeta,
		GeneratedAt: time.Now(),
		Command:     command,
		Data:        metas,
		Stats:       model.ResultStats{Items: len(metas)},
	}
}

// buildSeriesDataResult wraps a *SeriesData in a Result envelope.
func buildSeriesDataResult(command string, data *model.SeriesData) *model.Result {
	return &model.Result{
		Kind:        model.KindSeriesData,
		GeneratedAt: time.Now(),
		Command:     command,
		Data:        data,
		Stats:       model.ResultStats{Items: len(data.Obs)},
	}
}
