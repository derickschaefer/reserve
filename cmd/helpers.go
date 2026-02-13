package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/render"
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

// batchGetObs fetches observations for multiple series IDs concurrently.
func batchGetObs(ctx context.Context, deps *app.Deps, ids []string, opts fred.ObsOptions) ([]*model.SeriesData, []string) {
	type result struct {
		data *model.SeriesData
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

			data, err := deps.Client.GetObservations(ctx, id, opts)
			results[i] = result{idx: i, data: data, err: err}
		}()
	}
	wg.Wait()

	// Return in original ID order
	var datas []*model.SeriesData
	var warnings []string
	for i, r := range results {
		if r.err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", ids[i], r.err))
		} else if r.data != nil {
			datas = append(datas, r.data)
		}
	}
	return datas, warnings
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

// parseIntID parses a string as a positive integer ID, with a descriptive label for errors.
func parseIntID(s, label string) (int, error) {
	var id int
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil || id < 0 {
		return 0, fmt.Errorf("invalid %s %q: expected a positive integer", label, s)
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
