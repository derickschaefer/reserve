// Package render converts Result values into human-readable or machine-parseable
// output. Each format is a separate function; the top-level Render dispatcher
// selects based on the format string.
package render

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/derickschaefer/reserve/internal/model"
	"github.com/olekukonko/tablewriter"
)

// Format constants matching --format flag values.
const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatJSONL = "jsonl"
	FormatCSV   = "csv"
	FormatTSV   = "tsv"
	FormatMD    = "md"
)

// Render writes result to w in the specified format.
func Render(w io.Writer, result *model.Result, format string) error {
	switch format {
	case FormatJSON:
		return renderJSON(w, result)
	case FormatJSONL:
		return renderJSONL(w, result)
	case FormatCSV:
		return renderDelimited(w, result, ',')
	case FormatTSV:
		return renderDelimited(w, result, '\t')
	case FormatMD:
		return renderMarkdown(w, result)
	default:
		return renderTable(w, result)
	}
}

// RenderTo writes to stdout by default; if path is non-empty, writes to file.
func RenderTo(path string, result *model.Result, format string) error {
	if path == "" {
		return Render(os.Stdout, result, format)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()
	return Render(f, result, format)
}

// ─── JSON ─────────────────────────────────────────────────────────────────────

func renderJSON(w io.Writer, result *model.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// ─── JSONL ────────────────────────────────────────────────────────────────────

// jsonlRow is a canonical JSONL record for time series observations.
type jsonlRow struct {
	SeriesID string      `json:"series_id"`
	Date     string      `json:"date"`
	Value    interface{} `json:"value"` // float64 or null
	ValueRaw string      `json:"value_raw"`
}

func renderJSONL(w io.Writer, result *model.Result) error {
	enc := json.NewEncoder(w)
	switch result.Kind {
	case model.KindSeriesData:
		sd, ok := result.Data.(*model.SeriesData)
		if !ok {
			return renderJSON(w, result)
		}
		for _, obs := range sd.Obs {
			row := jsonlRow{
				SeriesID: sd.SeriesID,
				Date:     obs.Date.Format("2006-01-02"),
				ValueRaw: obs.ValueRaw,
			}
			if math.IsNaN(obs.Value) {
				row.Value = nil
			} else {
				row.Value = obs.Value
			}
			if err := enc.Encode(row); err != nil {
				return err
			}
		}
		return nil
	case model.KindSeriesMeta:
		return enc.Encode(result.Data)
	default:
		return enc.Encode(result.Data)
	}
}

// ─── Table ────────────────────────────────────────────────────────────────────

func renderTable(w io.Writer, result *model.Result) error {
	switch result.Kind {
	case model.KindSeriesData:
		sd, ok := result.Data.(*model.SeriesData)
		if !ok {
			return fmt.Errorf("unexpected data type for series_data")
		}
		return renderObsTable(w, sd)
	case model.KindSeriesMeta:
		meta, ok := result.Data.(*model.SeriesMeta)
		if !ok {
			// could be a slice
			if metas, ok2 := result.Data.([]model.SeriesMeta); ok2 {
				return renderSeriesMetaSliceTable(w, metas)
			}
			return fmt.Errorf("unexpected data type for series_meta")
		}
		return renderSeriesMetaTable(w, meta)
	case model.KindSearchResult:
		sr, ok := result.Data.(*model.SearchResult)
		if !ok {
			return fmt.Errorf("unexpected data type for search_result")
		}
		return renderSearchTable(w, sr)
	default:
		// Fallback: JSON
		return renderJSON(w, result)
	}
}

func renderObsTable(w io.Writer, sd *model.SeriesData) error {
	tw := tablewriter.NewWriter(w)
	tw.SetHeader([]string{"SERIES", "DATE", "VALUE"})
	tw.SetBorder(true)
	tw.SetRowLine(false)
	tw.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	tw.SetAlignment(tablewriter.ALIGN_LEFT)
	tw.SetColumnAlignment([]int{
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_RIGHT,
	})
	tw.SetAutoWrapText(false)

	for _, obs := range sd.Obs {
		val := formatValue(obs.Value)
		tw.Append([]string{
			sd.SeriesID,
			obs.Date.Format("2006-01-02"),
			val,
		})
	}
	tw.Render()
	return nil
}

func renderSeriesMetaTable(w io.Writer, m *model.SeriesMeta) error {
	tw := tablewriter.NewWriter(w)
	tw.SetHeader([]string{"FIELD", "VALUE"})
	tw.SetBorder(true)
	tw.SetRowLine(false)
	tw.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	tw.SetAlignment(tablewriter.ALIGN_LEFT)
	tw.SetColWidth(80)
	tw.SetAutoWrapText(true)

	rows := [][]string{
		{"ID", m.ID},
		{"Title", m.Title},
		{"Frequency", m.Frequency},
		{"Units", m.Units},
		{"Seasonal Adj.", m.SeasonalAdjustment},
		{"Observation Start", m.ObservationStart},
		{"Observation End", m.ObservationEnd},
		{"Last Updated", m.LastUpdated},
		{"Popularity", fmt.Sprintf("%d", m.Popularity)},
	}
	if m.Notes != "" {
		notes := m.Notes
		if len(notes) > 200 {
			notes = notes[:200] + "…"
		}
		rows = append(rows, []string{"Notes", notes})
	}
	for _, r := range rows {
		tw.Append(r)
	}
	tw.Render()
	return nil
}

func renderSeriesMetaSliceTable(w io.Writer, metas []model.SeriesMeta) error {
	tw := tablewriter.NewWriter(w)
	tw.SetHeader([]string{"ID", "TITLE", "FREQ", "UNITS", "LAST UPDATED"})
	tw.SetBorder(true)
	tw.SetRowLine(false)
	tw.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	tw.SetAlignment(tablewriter.ALIGN_LEFT)
	tw.SetAutoWrapText(false)
	tw.SetColWidth(40)

	for _, m := range metas {
		title := m.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		units := m.UnitsShort
		if units == "" {
			units = m.Units
		}
		if len(units) > 20 {
			units = units[:17] + "..."
		}
		tw.Append([]string{
			m.ID,
			title,
			m.FrequencyShort,
			units,
			m.LastUpdated,
		})
	}
	tw.Render()
	return nil
}

func renderSearchTable(w io.Writer, sr *model.SearchResult) error {
	fmt.Fprintf(w, "Search results for: %q\n\n", sr.Query)
	return renderSeriesMetaSliceTable(w, sr.Series)
}

// ─── CSV / TSV ────────────────────────────────────────────────────────────────

func renderDelimited(w io.Writer, result *model.Result, sep rune) error {
	cw := csv.NewWriter(w)
	cw.Comma = sep

	switch result.Kind {
	case model.KindSeriesData:
		sd, ok := result.Data.(*model.SeriesData)
		if !ok {
			return fmt.Errorf("unexpected data type for series_data")
		}
		_ = cw.Write([]string{"series_id", "date", "value", "value_raw"})
		for _, obs := range sd.Obs {
			_ = cw.Write([]string{
				sd.SeriesID,
				obs.Date.Format("2006-01-02"),
				formatValue(obs.Value),
				obs.ValueRaw,
			})
		}
	case model.KindSeriesMeta:
		if metas, ok := result.Data.([]model.SeriesMeta); ok {
			_ = cw.Write([]string{"id", "title", "frequency", "units", "seasonal_adjustment", "observation_start", "observation_end", "last_updated", "popularity"})
			for _, m := range metas {
				_ = cw.Write([]string{
					m.ID, m.Title, m.Frequency, m.Units,
					m.SeasonalAdjustment, m.ObservationStart, m.ObservationEnd,
					m.LastUpdated, fmt.Sprintf("%d", m.Popularity),
				})
			}
		} else if meta, ok := result.Data.(*model.SeriesMeta); ok {
			_ = cw.Write([]string{"field", "value"})
			_ = cw.Write([]string{"id", meta.ID})
			_ = cw.Write([]string{"title", meta.Title})
			_ = cw.Write([]string{"frequency", meta.Frequency})
			_ = cw.Write([]string{"units", meta.Units})
		}
	default:
		// Fallback: serialize as JSON on a single line
		b, _ := json.Marshal(result.Data)
		_ = cw.Write([]string{string(b)})
	}

	cw.Flush()
	return cw.Error()
}

// ─── Markdown ─────────────────────────────────────────────────────────────────

func renderMarkdown(w io.Writer, result *model.Result) error {
	switch result.Kind {
	case model.KindSeriesData:
		sd, ok := result.Data.(*model.SeriesData)
		if !ok {
			return renderJSON(w, result)
		}
		fmt.Fprintf(w, "| SERIES | DATE | VALUE |\n|--------|------|-------|\n")
		for _, obs := range sd.Obs {
			fmt.Fprintf(w, "| %s | %s | %s |\n",
				sd.SeriesID,
				obs.Date.Format("2006-01-02"),
				formatValue(obs.Value),
			)
		}
		return nil
	case model.KindSeriesMeta:
		if metas, ok := result.Data.([]model.SeriesMeta); ok {
			fmt.Fprintf(w, "| ID | TITLE | FREQ | UNITS | LAST UPDATED |\n|----|----|----|----|----|\n")
			for _, m := range metas {
				title := m.Title
				if len(title) > 50 {
					title = title[:47] + "..."
				}
				fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n",
					m.ID, mdEscape(title), m.FrequencyShort, m.UnitsShort, m.LastUpdated)
			}
		}
		return nil
	default:
		return renderJSON(w, result)
	}
}

// ─── Warnings / Stats Footer ─────────────────────────────────────────────────

// PrintFooter writes warnings and stats to w when verbose mode is on.
func PrintFooter(w io.Writer, result *model.Result, verbose bool) {
	for _, warn := range result.Warnings {
		fmt.Fprintf(w, "⚠  %s\n", warn)
	}
	if verbose {
		src := "live"
		if result.Stats.CacheHit {
			src = "cache"
		}
		fmt.Fprintf(w, "\n[%s • %d items • %dms • %s]\n",
			result.GeneratedAt.Format(time.RFC3339),
			result.Stats.Items,
			result.Stats.DurationMs,
			src,
		)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// formatValue formats an observation value for display.
// Always shows at least one decimal place (e.g. 4.0, not 4).
// Trims unnecessary trailing zeros beyond the first (e.g. 3.400000 → 3.4).
// Missing values (NaN) render as ".".
func formatValue(v float64) string {
	if math.IsNaN(v) {
		return "."
	}
	// Trim trailing zeros but keep at least one digit after the decimal point.
	s := strings.TrimRight(fmt.Sprintf("%.6f", v), "0")
	if strings.HasSuffix(s, ".") {
		s += "0" // "4." → "4.0"
	}
	return s
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
