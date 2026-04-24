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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultUpdateManifestURL = "https://download.reservecli.dev/release.json"

var (
	updateManifestURL = defaultUpdateManifestURL
	updateHTTPClient  = http.DefaultClient
	updateTargetOS    = runtime.GOOS
	updateTargetArch  = runtime.GOARCH
	updateExecutable  = os.Executable
	updateApplyDryRun bool
	updateApplyForce  bool
)

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

type updatePlan struct {
	Manifest   *updateManifest
	Check      updateCheckResult
	Force      bool
	ArchiveURL string
	Archive    string
	SHA256URL  string
}

type updateApplyResult struct {
	Status          string `json:"status"`
	CurrentVersion  string `json:"current_version"`
	TargetVersion   string `json:"target_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Forced          bool   `json:"forced"`
	DryRun          bool   `json:"dry_run"`
	InstallPath     string `json:"install_path,omitempty"`
	ArchiveURL      string `json:"archive_url,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	Summary         string `json:"summary,omitempty"`
	Applied         bool   `json:"applied"`
	Verified        bool   `json:"verified"`
	Downloaded      bool   `json:"downloaded"`
	Error           string `json:"error,omitempty"`
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

var updateApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Download and install a newer reserve release",
	Long:  "Download the latest reserve release for this platform, verify its checksum, and replace the current binary on macOS and Linux.",
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runUpdateApply(cmd.Context(), updateApplyDryRun, updateApplyForce)
		return renderUpdateApply(cmd, result)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.AddCommand(updateCheckCmd)
	updateCmd.AddCommand(updateApplyCmd)

	updateApplyCmd.Flags().BoolVar(&updateApplyDryRun, "dry-run", false, "download and verify the update without replacing the installed binary")
	updateApplyCmd.Flags().BoolVar(&updateApplyForce, "force", false, "proceed even when the latest release matches the current version")
}

func runUpdateCheck(parent context.Context) updateCheckResult {
	ctx, cancel := context.WithTimeout(baseUpdateContext(parent), updateTimeout())
	defer cancel()

	plan, err := planUpdate(ctx, false)
	if err != nil {
		return updateCheckResult{
			Status:         "error",
			CurrentVersion: Version,
			Error:          err.Error(),
		}
	}
	return plan.Check
}

func runUpdateApply(parent context.Context, dryRun, force bool) updateApplyResult {
	ctx, cancel := context.WithTimeout(baseUpdateContext(parent), updateTimeout())
	defer cancel()

	plan, err := planUpdate(ctx, force)
	if err != nil {
		return updateApplyResult{
			Status:         "error",
			CurrentVersion: Version,
			Forced:         force,
			DryRun:         dryRun,
			Error:          err.Error(),
		}
	}

	result := updateApplyResult{
		Status:          "current",
		CurrentVersion:  Version,
		TargetVersion:   plan.Check.LatestVersion,
		UpdateAvailable: plan.Check.UpdateAvailable,
		Forced:          force,
		DryRun:          dryRun,
		ReleaseURL:      plan.Check.ReleaseURL,
		Summary:         plan.Check.Summary,
	}

	if !plan.Check.UpdateAvailable && !force {
		return result
	}

	if updateTargetOS == "windows" {
		result.Status = "manual"
		result.ArchiveURL = plan.ArchiveURL
		return result
	}

	execPath, err := updateExecutable()
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("locating current executable: %v", err)
		return result
	}

	result.InstallPath = execPath
	result.ArchiveURL = plan.ArchiveURL

	archiveData, err := downloadBytes(ctx, plan.ArchiveURL)
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("downloading release archive: %v", err)
		return result
	}
	result.Downloaded = true

	expectedDigest, err := fetchArchiveChecksum(ctx, plan.SHA256URL, plan.Archive)
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("fetching release checksum: %v", err)
		return result
	}
	if err := verifyArchiveChecksum(archiveData, expectedDigest); err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	result.Verified = true

	binaryName := "reserve"
	binaryData, err := extractTarGzBinary(archiveData, binaryName)
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("extracting release archive: %v", err)
		return result
	}

	if dryRun {
		result.Status = "dry_run"
		return result
	}

	if err := replaceExecutable(execPath, binaryData); err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("installing update: %v", err)
		return result
	}

	result.Status = "applied"
	result.Applied = true
	return result
}

func planUpdate(parent context.Context, force bool) (*updatePlan, error) {
	ctx := baseUpdateContext(parent)

	manifest, err := fetchUpdateManifest(ctx, updateManifestURL)
	if err != nil {
		return nil, err
	}

	check, err := buildUpdateCheckResult(manifest)
	if err != nil {
		return nil, err
	}

	archiveName, err := platformArchiveName(updateTargetOS, updateTargetArch)
	if err != nil {
		return nil, err
	}

	archiveURL, err := releaseAssetURL(updateManifestURL, manifest.LatestVersion, archiveName)
	if err != nil {
		return nil, err
	}

	shaURL, err := releaseAssetURL(updateManifestURL, manifest.LatestVersion, "SHA256SUMS")
	if err != nil {
		return nil, err
	}

	return &updatePlan{
		Manifest:   manifest,
		Check:      check,
		Force:      force,
		ArchiveURL: archiveURL,
		Archive:    archiveName,
		SHA256URL:  shaURL,
	}, nil
}

func baseUpdateContext(parent context.Context) context.Context {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx
}

func updateTimeout() time.Duration {
	timeout := 5 * time.Second
	if globalFlags.Timeout != "" {
		if d, err := time.ParseDuration(globalFlags.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	return timeout
}

func buildUpdateCheckResult(manifest *updateManifest) (updateCheckResult, error) {
	available, err := isUpdateAvailable(Version, manifest.LatestVersion)
	if err != nil {
		return updateCheckResult{}, err
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
	}, nil
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

func downloadBytes(ctx context.Context, sourceURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating download request: %w", err)
	}

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	return data, nil
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

func platformArchiveName(goos, goarch string) (string, error) {
	switch goos {
	case "darwin", "linux":
		switch goarch {
		case "amd64", "arm64":
			return fmt.Sprintf("reserve_%s_%s.tar.gz", goos, goarch), nil
		}
	case "windows":
		switch goarch {
		case "amd64", "arm64":
			return fmt.Sprintf("reserve_windows_%s.zip", goarch), nil
		}
	}
	return "", fmt.Errorf("unsupported platform for self-update: %s/%s", goos, goarch)
}

func releaseAssetURL(manifestURL, version, asset string) (string, error) {
	u, err := url.Parse(manifestURL)
	if err != nil {
		return "", fmt.Errorf("parse update manifest url: %w", err)
	}
	u.Path = strings.TrimSuffix(u.Path, "/release.json")
	u.Path = strings.TrimRight(u.Path, "/") + "/releases/" + strings.TrimSpace(version) + "/" + asset
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func fetchArchiveChecksum(ctx context.Context, sourceURL, archiveName string) (string, error) {
	data, err := downloadBytes(ctx, sourceURL)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == archiveName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum entry not found for %s", archiveName)
}

func verifyArchiveChecksum(data []byte, expected string) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, strings.TrimSpace(expected)) {
		return fmt.Errorf("release checksum mismatch for downloaded archive")
	}
	return nil
}

func extractTarGzBinary(data []byte, binaryName string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != binaryName {
			continue
		}
		return io.ReadAll(tr)
	}
	return nil, fmt.Errorf("archive did not contain expected binary: %s", binaryName)
}

func replaceExecutable(target string, data []byte) error {
	dir := filepath.Dir(target)

	mode := os.FileMode(0o755)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".reserve-update-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, target); err != nil {
		return err
	}
	return nil
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

func renderUpdateApply(cmd *cobra.Command, result updateApplyResult) error {
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
		return renderUpdateApplyText(w, result)
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

func renderUpdateApplyText(w io.Writer, result updateApplyResult) error {
	switch result.Status {
	case "applied":
		if _, err := fmt.Fprintln(w, "Update applied"); err != nil {
			return err
		}
	case "dry_run":
		if _, err := fmt.Fprintln(w, "Dry run complete"); err != nil {
			return err
		}
	case "manual":
		if _, err := fmt.Fprintln(w, "Manual update required on Windows"); err != nil {
			return err
		}
	case "current":
		if _, err := fmt.Fprintln(w, "reserve is already up to date"); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintln(w, "Unable to apply update"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "current  %s\n", result.CurrentVersion); err != nil {
		return err
	}
	if result.TargetVersion != "" {
		if _, err := fmt.Fprintf(w, "target   %s\n", result.TargetVersion); err != nil {
			return err
		}
	}
	if result.InstallPath != "" {
		if _, err := fmt.Fprintf(w, "install  %s\n", result.InstallPath); err != nil {
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

	if result.Forced {
		if _, err := fmt.Fprintln(w, "\nMode: force"); err != nil {
			return err
		}
	}
	if result.DryRun {
		if _, err := fmt.Fprintln(w, "Mode: dry-run"); err != nil {
			return err
		}
	}
	if result.Verified {
		if _, err := fmt.Fprintln(w, "Verified: SHA256SUMS"); err != nil {
			return err
		}
	}
	if result.ArchiveURL != "" {
		if _, err := fmt.Fprintf(w, "Archive: %s\n", result.ArchiveURL); err != nil {
			return err
		}
	}
	if result.Status == "manual" && result.ReleaseURL != "" {
		if _, err := fmt.Fprintf(w, "Release: %s\n", result.ReleaseURL); err != nil {
			return err
		}
	}

	return nil
}
