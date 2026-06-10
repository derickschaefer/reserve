// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/derickschaefer/reserve/internal/config"
)

func TestBuildDepsRejectsInvalidGlobalOverrides(t *testing.T) {
	cases := []struct {
		name    string
		flag    string
		value   string
		wantErr string
	}{
		{name: "format", flag: "format", value: "jsno", wantErr: "--format"},
		{name: "timeout syntax", flag: "timeout", value: "30", wantErr: "--timeout"},
		{name: "timeout zero", flag: "timeout", value: "0s", wantErr: "--timeout must be > 0"},
		{name: "concurrency zero", flag: "concurrency", value: "0", wantErr: "--concurrency must be > 0"},
		{name: "concurrency negative", flag: "concurrency", value: "-1", wantErr: "--concurrency must be > 0"},
		{name: "rate zero", flag: "rate", value: "0", wantErr: "--rate must be > 0"},
		{name: "rate negative", flag: "rate", value: "-1", wantErr: "--rate must be > 0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateBuildDepsConfig(t)
			resetGlobalFlag(t, tc.flag)

			if err := rootCmd.PersistentFlags().Set(tc.flag, tc.value); err != nil {
				t.Fatalf("set %s: %v", tc.flag, err)
			}
			t.Cleanup(func() { resetGlobalFlag(t, tc.flag) })

			deps, err := buildDeps()
			if deps != nil {
				deps.Close()
			}
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func isolateBuildDepsConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(config.EnvAPIKey, "")
	t.Setenv(config.EnvDBPath, filepath.Join(dir, "reserve.db"))
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("APPDATA", filepath.Join(dir, "appdata"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "localappdata"))
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func resetGlobalFlag(t *testing.T, name string) {
	t.Helper()
	flag := rootCmd.PersistentFlags().Lookup(name)
	if flag == nil {
		t.Fatalf("unknown flag %q", name)
	}
	if err := rootCmd.PersistentFlags().Set(name, flag.DefValue); err != nil {
		t.Fatalf("reset %s: %v", name, err)
	}
	flag.Changed = false
}

func TestRewriteArgsForAIOnboard(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "top level command",
			args: []string{"obs", "--ai-onboard"},
			want: []string{"onboard", "obs"},
		},
		{
			name: "nested command collapses to top level guide",
			args: []string{"obs", "get", "GDP", "--ai-onboard"},
			want: []string{"onboard", "obs"},
		},
		{
			name: "global flag value before command",
			args: []string{"--api-key", "secret", "obs", "--ai-onboard"},
			want: []string{"onboard", "obs"},
		},
		{
			name: "preserves format and out",
			args: []string{"obs", "--ai-onboard", "--format", "jsonl", "--out", "guide.jsonl"},
			want: []string{"onboard", "obs", "--format", "jsonl", "--out", "guide.jsonl"},
		},
		{
			name: "topic mode without command",
			args: []string{"--ai-onboard", "--topic", "pipeline"},
			want: []string{"onboard", "--topic", "pipeline"},
		},
		{
			name: "onboard command remains onboard",
			args: []string{"onboard", "obs", "--ai-onboard"},
			want: []string{"onboard", "obs"},
		},
		{
			name: "explicit false does not rewrite",
			args: []string{"obs", "--ai-onboard=false"},
			want: []string{"obs", "--ai-onboard=false"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteArgsForAIOnboard(tc.args)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rewriteArgsForAIOnboard(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
