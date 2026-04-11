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
)

type updateRoundTripFunc func(*http.Request) (*http.Response, error)

func (f updateRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newUpdateTestClient(handler http.HandlerFunc) *http.Client {
	return &http.Client{
		Transport: updateRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := newResponseRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    int
		wantErr bool
	}{
		{name: "equal", current: "v1.1.0", latest: "v1.1.0", want: 0},
		{name: "newer available", current: "v1.0.9", latest: "v1.1.0", want: -1},
		{name: "current newer", current: "v1.2.0", latest: "v1.1.0", want: 1},
		{name: "no v prefix required", current: "1.0.9", latest: "1.1.0", want: -1},
		{name: "invalid", current: "main", latest: "v1.1.0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compareVersions(tt.current, tt.latest)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("compareVersions: %v", err)
			}
			if got != tt.want {
				t.Fatalf("compareVersions = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunUpdateCheckUpdateAvailable(t *testing.T) {
	origClient := updateHTTPClient
	updateHTTPClient = newUpdateTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(updateManifest{
			LatestVersion:      "v1.1.0",
			ReleaseURL:         "https://example.com/releases/v1.1.0",
			Summary:            "Adds version checks and user config placement.",
			Highlights:         []string{"Version checks", "User config placement"},
			UpdateInstructions: "Download the latest release from the GitHub releases page or rerun the install script from download.reservecli.dev.",
		})
	}))
	t.Cleanup(func() { updateHTTPClient = origClient })

	origURL := updateManifestURL
	updateManifestURL = "https://mock.reserve.local/release.json"
	t.Cleanup(func() { updateManifestURL = origURL })

	origVersion := Version
	Version = "v1.0.9"
	t.Cleanup(func() { Version = origVersion })

	result := runUpdateCheck(context.Background())
	if !result.UpdateAvailable {
		t.Fatalf("expected update to be available")
	}
	if result.Status != "update_available" {
		t.Fatalf("status = %q, want update_available", result.Status)
	}
	if result.LatestVersion != "v1.1.0" {
		t.Fatalf("latest_version = %q", result.LatestVersion)
	}
	if result.Summary == "" {
		t.Fatalf("expected summary")
	}
}

func TestRunUpdateCheckCurrent(t *testing.T) {
	origClient := updateHTTPClient
	updateHTTPClient = newUpdateTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(updateManifest{
			LatestVersion: "v1.1.0",
		})
	}))
	t.Cleanup(func() { updateHTTPClient = origClient })

	origURL := updateManifestURL
	updateManifestURL = "https://mock.reserve.local/release.json"
	t.Cleanup(func() { updateManifestURL = origURL })

	origVersion := Version
	Version = "v1.1.0"
	t.Cleanup(func() { Version = origVersion })

	result := runUpdateCheck(context.Background())
	if result.UpdateAvailable {
		t.Fatalf("did not expect update to be available")
	}
	if result.Status != "current" {
		t.Fatalf("status = %q, want current", result.Status)
	}
}

func TestRunUpdateCheckManifestError(t *testing.T) {
	origClient := updateHTTPClient
	updateHTTPClient = newUpdateTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(func() { updateHTTPClient = origClient })

	origURL := updateManifestURL
	updateManifestURL = "https://mock.reserve.local/release.json"
	t.Cleanup(func() { updateManifestURL = origURL })

	result := runUpdateCheck(context.Background())
	if result.Status != "error" {
		t.Fatalf("status = %q, want error", result.Status)
	}
	if result.Error == "" {
		t.Fatalf("expected error message")
	}
}

func TestRenderUpdateCheckText(t *testing.T) {
	var buf bytes.Buffer
	err := renderUpdateCheckText(&buf, updateCheckResult{
		Status:             "update_available",
		CurrentVersion:     "v1.0.9",
		LatestVersion:      "v1.1.0",
		UpdateAvailable:    true,
		Summary:            "Adds version checks, user config discovery, and local config overrides.",
		Highlights:         []string{"Version update checks", "Per-user config locations"},
		UpdateInstructions: "Download the latest release from the GitHub releases page or rerun the install script from download.reservecli.dev.",
		ReleaseURL:         "https://example.com/releases/v1.1.0",
	})
	if err != nil {
		t.Fatalf("renderUpdateCheckText: %v", err)
	}

	out := buf.String()
	for _, needle := range []string{
		"Update available",
		"current  v1.0.9",
		"latest   v1.1.0",
		"Highlights:",
		"More info: https://example.com/releases/v1.1.0",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected output to contain %q, got:\n%s", needle, out)
		}
	}
}
