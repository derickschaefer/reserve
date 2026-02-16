package cmd

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

// Version is the canonical release string. The default here is the fallback
// for `go run` and untagged builds. Production builds overwrite this via:
//
//	go build -ldflags "-X github.com/derickschaefer/reserve/cmd.Version=v1.0.6"
//
// Set once in the Makefile VERSION variable; never edit this string directly
// for a release.
var Version = "v1.0.5"

// versionInfo is the structured payload for --format json output.
// All fields are exported so encoding/json picks them up.
type versionInfo struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	BuildTime string `json:"build_time,omitempty"`
}

// BuildTime is optionally injected at build time alongside Version:
//
//	-ldflags "-X github.com/derickschaefer/reserve/cmd.Version=v1.0.6
//	           -X github.com/derickschaefer/reserve/cmd.BuildTime=2026-02-16T12:00:00Z"
var BuildTime = ""

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the reserve version and build information",
	Long: `Print the reserve version string and build metadata.

Default output is plain text, suitable for shell scripts and pipelines.
Use --format json for structured output.

Examples:
  reserve version
  reserve version --format json
  reserve version --format json | jq .version`,
	RunE: func(cmd *cobra.Command, args []string) error {
		format := globalFlags.Format
		if format == "" {
			format = "text"
		}

		info := versionInfo{
			Version:   Version,
			GoVersion: runtime.Version(),
			GOOS:      runtime.GOOS,
			GOARCH:    runtime.GOARCH,
			BuildTime: BuildTime,
		}

		switch format {
		case "json":
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(info)

		case "jsonl":
			// Single object, one line — useful for mixing into a JSONL pipeline.
			b, err := json.Marshal(info)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", b)
			return nil

		default:
			// Plain text — one value per line, grep/awk friendly.
			fmt.Fprintf(cmd.OutOrStdout(), "reserve %s\n", info.Version)
			fmt.Fprintf(cmd.OutOrStdout(), "go      %s\n", info.GoVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "os      %s/%s\n", info.GOOS, info.GOARCH)
			if info.BuildTime != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "built   %s\n", info.BuildTime)
			}
			return nil
		}
	},
}

// buildTimestamp returns the current UTC time formatted for ldflags injection.
// Use in CI: -ldflags "-X ...BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
func buildTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
