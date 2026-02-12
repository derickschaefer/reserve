// Package cmd implements the reserve CLI command tree.
// This file defines the root command and registers all global persistent flags.
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/derickschaefer/reserve/internal/app"
	"github.com/derickschaefer/reserve/internal/config"
)

// globalFlags holds the parsed values of all persistent (global) flags.
// Commands read from this struct via the deps they receive.
var globalFlags struct {
	APIKey      string
	Format      string
	Out         string
	NoCache     bool
	Refresh     bool
	Timeout     string
	Concurrency int
	Rate        float64
	Quiet       bool
	Verbose     bool
	Debug       bool
	Pager       string
}

// rootCmd is the base command. Running `reserve` with no subcommand
// prints help.
var rootCmd = &cobra.Command{
	Use:   "reserve",
	Short: "reserve — Federal Reserve Economic Data (FRED) CLI",
	Long: `reserve is a command-line tool for exploring and retrieving economic data
from the Federal Reserve Bank of St. Louis FRED® API.

Data sourced from FRED®, Federal Reserve Bank of St. Louis;
https://fred.stlouisfed.org/

Get a free API key at: https://fred.stlouisfed.org/docs/api/api_key.html

Quick start:
  reserve config init          # create a config.json with your API key
  reserve series get GDP       # fetch GDP series metadata
  reserve obs get GDP          # fetch GDP observations`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute is the entry point called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// buildDeps resolves config and constructs the dependency container.
// Called at the start of each command's RunE.
func buildDeps() (*app.Deps, error) {
	cfg, err := config.Load(globalFlags.APIKey)
	if err != nil {
		return nil, err
	}

	// Apply CLI flag overrides
	cfg.NoCache  = globalFlags.NoCache
	cfg.Refresh  = globalFlags.Refresh
	cfg.Quiet    = globalFlags.Quiet
	cfg.Verbose  = globalFlags.Verbose
	cfg.Debug    = globalFlags.Debug

	if globalFlags.Format != "" {
		cfg.Format = globalFlags.Format
	}
	if globalFlags.Timeout != "" {
		if d, err2 := time.ParseDuration(globalFlags.Timeout); err2 == nil {
			cfg.Timeout = d
		}
	}
	if globalFlags.Concurrency > 0 {
		cfg.Concurrency = globalFlags.Concurrency
	}
	if globalFlags.Rate > 0 {
		cfg.Rate = globalFlags.Rate
	}

	return app.New(cfg), nil
}

func init() {
	pf := rootCmd.PersistentFlags()

	pf.StringVar(&globalFlags.APIKey, "api-key", "",
		"FRED API key (overrides env FRED_API_KEY and config.json)")
	pf.StringVar(&globalFlags.Format, "format", "",
		"output format: table|json|jsonl|csv|tsv|md (default: table)")
	pf.StringVar(&globalFlags.Out, "out", "",
		"write output to file instead of stdout")
	pf.BoolVar(&globalFlags.NoCache, "no-cache", false,
		"bypass cache reads (still writes results to cache)")
	pf.BoolVar(&globalFlags.Refresh, "refresh", false,
		"force re-fetch and overwrite cached entries")
	pf.StringVar(&globalFlags.Timeout, "timeout", "",
		"HTTP request timeout (e.g. 30s, 2m)")
	pf.IntVar(&globalFlags.Concurrency, "concurrency", 0,
		"max parallel requests for batch operations (default: 8)")
	pf.Float64Var(&globalFlags.Rate, "rate", 0,
		"max API requests per second (default: 5.0)")
	pf.BoolVar(&globalFlags.Quiet, "quiet", false,
		"suppress all non-error output")
	pf.BoolVar(&globalFlags.Verbose, "verbose", false,
		"show cache/timing stats after output")
	pf.BoolVar(&globalFlags.Debug, "debug", false,
		"log HTTP requests and responses (API key redacted)")
	pf.StringVar(&globalFlags.Pager, "pager", "auto",
		"pager mode: auto|never")
}
