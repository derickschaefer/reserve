// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/spf13/cobra"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestParseCategoryID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "root keyword", input: "root", want: 0},
		{name: "root keyword mixed case", input: " Root ", want: 0},
		{name: "numeric id", input: "32991", want: 32991},
		{name: "negative id", input: "-1", want: -1},
		{name: "reject non numeric suffix", input: "12abc", wantErr: true},
		{name: "reject empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCategoryID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCategoryID(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("parseCategoryID(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestWalkCategoryTreeHonorsDepthLimit(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/category/children" {
			http.NotFound(w, r)
			return
		}

		switch r.URL.Query().Get("category_id") {
		case "0":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"categories": []map[string]any{
					{"id": 10, "name": "Labor", "parent_id": 0},
					{"id": 20, "name": "Prices", "parent_id": 0},
				},
			})
		case "10":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"categories": []map[string]any{
					{"id": 11, "name": "Employment", "parent_id": 10},
				},
			})
		case "20":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"categories": []map[string]any{
					{"id": 21, "name": "Inflation", "parent_id": 20},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"categories": []map[string]any{}})
		}
	})

	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := newResponseRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
		Timeout: 5 * time.Second,
	}

	client := fred.NewClient("test_key", "https://mock.fred.local/", 5*time.Second, 1000, false)
	client.SetHTTPClient(httpClient)
	deps := &app.Deps{
		Config: &config.Config{},
		Client: client,
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(context.Background())

	if err := walkCategoryTree(cmd, deps, 0, 1, 1, ""); err != nil {
		t.Fatalf("walkCategoryTree depth=1: %v", err)
	}

	got := out.String()
	for _, want := range []string{"[10] Labor", "[20] Prices"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"[11] Employment", "[21] Inflation"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("did not expect output to contain %q, got:\n%s", unwanted, got)
		}
	}
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		header: make(http.Header),
		body:   &bytes.Buffer{},
		code:   http.StatusOK,
	}
}

type responseRecorder struct {
	header http.Header
	body   *bytes.Buffer
	code   int
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}

func (r *responseRecorder) Result() *http.Response {
	return &http.Response{
		StatusCode: r.code,
		Header:     r.header.Clone(),
		Body:       ioNopCloser{Reader: bytes.NewReader(r.body.Bytes())},
	}
}

type ioNopCloser struct {
	*bytes.Reader
}

func (ioNopCloser) Close() error { return nil }
