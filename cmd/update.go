// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultUpdateManifestURL = "https://download.reservecli.dev/release.json"

var updateManifestURL = defaultUpdateManifestURL
var updateHTTPClient = http.DefaultClient

type updateManifest struct {
	LatestVersion      string   `json:"latest_version"`
	PublishedAt        string   `json:"published_at,omitempty"`
	DownloadURL        string   `json:"download_url,omitempty"`
	ReleaseURL         string   `json:"release_url,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	Highlights         []string `json:"highlights,omitempty"`
	Severity           string   `json:"severity,omitempty"`
	UpdateInstructions string   `json:"update_instructions,omitempty"`
}

type updateCheckResult struct {
	Status             string   `json:"status"`
	CurrentVersion     string   `json:"current_version"`
	LatestVersion      string   `json:"latest_version,omitempty"`
	UpdateAvailable    bool     `json:"update_available"`
	PublishedAt        string   `json:"published_at,omitempty"`
	DownloadURL        string   `json:"download_url,omitempty"`
	ReleaseURL         string   `json:"release_url,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	Highlights         []string `json:"highlights,omitempty"`
	Severity           string   `json:"severity,omitempty"`
	UpdateInstructions string   `json:"update_instructions,omitempty"`
	Error              string   `json:"error,omitempty"`
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for newer reserve releases",
	Long:  "Check a lightweight release manifest for newer reserve versions and display release highlights.",
}

var updateCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check whether a newer reserve version is available",
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runUpdateCheck(cmd.Context())
		return renderUpdateCheck(cmd, result)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.AddCommand(updateCheckCmd)
}

func runUpdateCheck(parent context.Context) updateCheckResult {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := 5 * time.Second
	if globalFlags.Timeout != "" {
		if d, err := time.ParseDuration(globalFlags.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	manifest, err := fetchUpdateManifest(ctx, updateManifestURL)
	if err != nil {
		return updateCheckResult{
			Status:         "error",
			CurrentVersion: Version,
			Error:          err.Error(),
		}
	}

	available, cmpErr := isUpdateAvailable(Version, manifest.LatestVersion)
	if cmpErr != nil {
		return updateCheckResult{
			Status:         "error",
			CurrentVersion: Version,
			LatestVersion:  manifest.LatestVersion,
			Error:          cmpErr.Error(),
		}
	}

	status := "current"
	if available {
		status = "update_available"
	}

	return updateCheckResult{
		Status:             status,
		CurrentVersion:     Version,
		LatestVersion:      manifest.LatestVersion,
		UpdateAvailable:    available,
		PublishedAt:        manifest.PublishedAt,
		DownloadURL:        manifest.DownloadURL,
		ReleaseURL:         manifest.ReleaseURL,
		Summary:            manifest.Summary,
		Highlights:         manifest.Highlights,
		Severity:           manifest.Severity,
		UpdateInstructions: manifest.UpdateInstructions,
	}
}

func fetchUpdateManifest(ctx context.Context, url string) (*updateManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating update check request: %w", err)
	}

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("checking update manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checking update manifest: unexpected HTTP %d", resp.StatusCode)
	}

	var manifest updateManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decoding update manifest: %w", err)
	}
	if strings.TrimSpace(manifest.LatestVersion) == "" {
		return nil, fmt.Errorf("update manifest missing latest_version")
	}
	return &manifest, nil
}

func isUpdateAvailable(currentVersion, latestVersion string) (bool, error) {
	cmp, err := compareVersions(currentVersion, latestVersion)
	if err != nil {
		return false, err
	}
	return cmp < 0, nil
}

func compareVersions(currentVersion, latestVersion string) (int, error) {
	currentParts, err := parseVersion(currentVersion)
	if err != nil {
		return 0, fmt.Errorf("parse current version %q: %w", currentVersion, err)
	}
	latestParts, err := parseVersion(latestVersion)
	if err != nil {
		return 0, fmt.Errorf("parse latest version %q: %w", latestVersion, err)
	}

	for i := 0; i < len(currentParts) && i < len(latestParts); i++ {
		switch {
		case currentParts[i] < latestParts[i]:
			return -1, nil
		case currentParts[i] > latestParts[i]:
			return 1, nil
		}
	}
	return 0, nil
}

func parseVersion(v string) ([]int, error) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("expected semantic version X.Y.Z")
	}

	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid numeric segment %q", part)
		}
		out = append(out, n)
	}
	return out, nil
}

func renderUpdateCheck(cmd *cobra.Command, result updateCheckResult) error {
	format := globalFlags.Format
	if format == "" {
		format = "text"
	}

	w, closeFn, err := outputWriter(cmd.OutOrStdout())
	if err != nil {
		return err
	}
	defer closeFn()

	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	case "jsonl":
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "%s\n", b)
		return err
	default:
		return renderUpdateCheckText(w, result)
	}
}

func renderUpdateCheckText(w io.Writer, result updateCheckResult) error {
	switch result.Status {
	case "update_available":
		if _, err := fmt.Fprintln(w, "Update available"); err != nil {
			return err
		}
	case "current":
		if _, err := fmt.Fprintln(w, "reserve is up to date"); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintln(w, "Unable to check for updates"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "current  %s\n", result.CurrentVersion); err != nil {
		return err
	}
	if result.LatestVersion != "" {
		if _, err := fmt.Fprintf(w, "latest   %s\n", result.LatestVersion); err != nil {
			return err
		}
	}

	if result.Status == "error" {
		if result.Error != "" {
			_, err := fmt.Fprintf(w, "\n%s\n", result.Error)
			return err
		}
		return nil
	}

	if result.UpdateAvailable {
		if result.Summary != "" {
			if _, err := fmt.Fprintf(w, "\n%s\n", result.Summary); err != nil {
				return err
			}
		}
		if len(result.Highlights) > 0 {
			if _, err := fmt.Fprintln(w, "\nHighlights:"); err != nil {
				return err
			}
			for _, item := range result.Highlights {
				if _, err := fmt.Fprintf(w, "- %s\n", item); err != nil {
					return err
				}
			}
		}
		if result.UpdateInstructions != "" {
			if _, err := fmt.Fprintf(w, "\nUpdate: %s\n", result.UpdateInstructions); err != nil {
				return err
			}
		}
		if result.ReleaseURL != "" {
			if _, err := fmt.Fprintf(w, "More info: %s\n", result.ReleaseURL); err != nil {
				return err
			}
		}
	}

	return nil
}
