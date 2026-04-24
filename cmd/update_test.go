// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

func TestRunUpdateApplyDryRun(t *testing.T) {
	origClient := updateHTTPClient
	origURL := updateManifestURL
	origVersion := Version
	origOS := updateTargetOS
	origArch := updateTargetArch
	origExec := updateExecutable

	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "reserve")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	newBinary := []byte("new-binary")
	archive := mustBuildTarGz(t, "reserve", newBinary)
	checksum := sha256.Sum256(archive)

	updateHTTPClient = newUpdateTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			_ = json.NewEncoder(w).Encode(updateManifest{
				LatestVersion: "v1.1.3",
				ReleaseURL:    "https://example.com/releases/v1.1.3",
				Summary:       "Dry-run candidate.",
			})
		case "/releases/v1.1.3/reserve_linux_amd64.tar.gz":
			_, _ = w.Write(archive)
		case "/releases/v1.1.3/SHA256SUMS":
			_, _ = w.Write([]byte(hex.EncodeToString(checksum[:]) + "  reserve_linux_amd64.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	updateManifestURL = "https://mock.reserve.local/release.json"
	Version = "v1.1.2"
	updateTargetOS = "linux"
	updateTargetArch = "amd64"
	updateExecutable = func() (string, error) { return execPath, nil }

	t.Cleanup(func() {
		updateHTTPClient = origClient
		updateManifestURL = origURL
		Version = origVersion
		updateTargetOS = origOS
		updateTargetArch = origArch
		updateExecutable = origExec
	})

	result := runUpdateApply(context.Background(), true, false)
	if result.Status != "dry_run" {
		t.Fatalf("status = %q, want dry_run", result.Status)
	}
	if !result.Verified || !result.Downloaded {
		t.Fatalf("expected download and verification to succeed: %#v", result)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	if string(got) != "old-binary" {
		t.Fatalf("dry-run replaced executable: %q", string(got))
	}
}

func TestRunUpdateApplyReplacesExecutable(t *testing.T) {
	origClient := updateHTTPClient
	origURL := updateManifestURL
	origVersion := Version
	origOS := updateTargetOS
	origArch := updateTargetArch
	origExec := updateExecutable

	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "reserve")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	newBinary := []byte("new-binary")
	archive := mustBuildTarGz(t, "reserve", newBinary)
	checksum := sha256.Sum256(archive)

	updateHTTPClient = newUpdateTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			_ = json.NewEncoder(w).Encode(updateManifest{
				LatestVersion: "v1.1.3",
				ReleaseURL:    "https://example.com/releases/v1.1.3",
			})
		case "/releases/v1.1.3/reserve_linux_amd64.tar.gz":
			_, _ = w.Write(archive)
		case "/releases/v1.1.3/SHA256SUMS":
			_, _ = w.Write([]byte(hex.EncodeToString(checksum[:]) + "  reserve_linux_amd64.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	updateManifestURL = "https://mock.reserve.local/release.json"
	Version = "v1.1.2"
	updateTargetOS = "linux"
	updateTargetArch = "amd64"
	updateExecutable = func() (string, error) { return execPath, nil }

	t.Cleanup(func() {
		updateHTTPClient = origClient
		updateManifestURL = origURL
		Version = origVersion
		updateTargetOS = origOS
		updateTargetArch = origArch
		updateExecutable = origExec
	})

	result := runUpdateApply(context.Background(), false, false)
	if result.Status != "applied" {
		t.Fatalf("status = %q, want applied", result.Status)
	}
	if !result.Applied || !result.Verified {
		t.Fatalf("expected applied result: %#v", result)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("got executable %q, want new-binary", string(got))
	}
}

func TestRunUpdateApplyForceSameVersion(t *testing.T) {
	origClient := updateHTTPClient
	origURL := updateManifestURL
	origVersion := Version
	origOS := updateTargetOS
	origArch := updateTargetArch
	origExec := updateExecutable

	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "reserve")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	newBinary := []byte("forced-binary")
	archive := mustBuildTarGz(t, "reserve", newBinary)
	checksum := sha256.Sum256(archive)

	updateHTTPClient = newUpdateTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release.json":
			_ = json.NewEncoder(w).Encode(updateManifest{
				LatestVersion: "v1.1.2",
				ReleaseURL:    "https://example.com/releases/v1.1.2",
			})
		case "/releases/v1.1.2/reserve_linux_amd64.tar.gz":
			_, _ = w.Write(archive)
		case "/releases/v1.1.2/SHA256SUMS":
			_, _ = w.Write([]byte(hex.EncodeToString(checksum[:]) + "  reserve_linux_amd64.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	updateManifestURL = "https://mock.reserve.local/release.json"
	Version = "v1.1.2"
	updateTargetOS = "linux"
	updateTargetArch = "amd64"
	updateExecutable = func() (string, error) { return execPath, nil }

	t.Cleanup(func() {
		updateHTTPClient = origClient
		updateManifestURL = origURL
		Version = origVersion
		updateTargetOS = origOS
		updateTargetArch = origArch
		updateExecutable = origExec
	})

	result := runUpdateApply(context.Background(), false, true)
	if result.Status != "applied" {
		t.Fatalf("status = %q, want applied", result.Status)
	}
	if !result.Forced {
		t.Fatalf("expected forced apply")
	}
}

func TestRunUpdateApplyWindowsManual(t *testing.T) {
	origClient := updateHTTPClient
	origURL := updateManifestURL
	origVersion := Version
	origOS := updateTargetOS
	origArch := updateTargetArch

	updateHTTPClient = newUpdateTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/release.json" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(updateManifest{
			LatestVersion: "v1.1.3",
			ReleaseURL:    "https://example.com/releases/v1.1.3",
		})
	}))
	updateManifestURL = "https://mock.reserve.local/release.json"
	Version = "v1.1.2"
	updateTargetOS = "windows"
	updateTargetArch = "amd64"

	t.Cleanup(func() {
		updateHTTPClient = origClient
		updateManifestURL = origURL
		Version = origVersion
		updateTargetOS = origOS
		updateTargetArch = origArch
	})

	result := runUpdateApply(context.Background(), false, false)
	if result.Status != "manual" {
		t.Fatalf("status = %q, want manual", result.Status)
	}
	if !strings.Contains(result.ArchiveURL, "reserve_windows_amd64.zip") {
		t.Fatalf("archive_url = %q", result.ArchiveURL)
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

func TestRenderUpdateApplyText(t *testing.T) {
	var buf bytes.Buffer
	err := renderUpdateApplyText(&buf, updateApplyResult{
		Status:         "dry_run",
		CurrentVersion: "v1.1.2",
		TargetVersion:  "v1.1.3",
		Forced:         true,
		DryRun:         true,
		Verified:       true,
		ArchiveURL:     "https://example.com/releases/v1.1.3/reserve_linux_amd64.tar.gz",
		InstallPath:    "/usr/local/bin/reserve",
	})
	if err != nil {
		t.Fatalf("renderUpdateApplyText: %v", err)
	}

	out := buf.String()
	for _, needle := range []string{
		"Dry run complete",
		"current  v1.1.2",
		"target   v1.1.3",
		"Mode: force",
		"Mode: dry-run",
		"Verified: SHA256SUMS",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected output to contain %q, got:\n%s", needle, out)
		}
	}
}

func mustBuildTarGz(t *testing.T, name string, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	hdr := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("write archive payload: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return buf.Bytes()
}
