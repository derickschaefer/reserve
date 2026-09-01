// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package fred

import (
	"strings"
	"testing"
)

func TestNoObservationsErrorDescribesRequestedRange(t *testing.T) {
	err := noObservationsError("gdp", ObsOptions{Start: "2026-07-01", End: "2026-08-27"})
	want := "no observations found for GDP between 2026-07-01 and 2026-08-27"
	if err.Error() != want {
		t.Fatalf("noObservationsError() = %q, want %q", err, want)
	}
}

func TestNoObservationsErrorHandlesPartialRanges(t *testing.T) {
	tests := []struct {
		name string
		opts ObsOptions
		want string
	}{
		{name: "start", opts: ObsOptions{Start: "2026-07-01"}, want: "on or after 2026-07-01"},
		{name: "end", opts: ObsOptions{End: "2026-08-27"}, want: "on or before 2026-08-27"},
		{name: "unbounded", want: "no observations found for GDP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noObservationsError("GDP", tt.opts).Error(); !strings.Contains(got, tt.want) {
				t.Fatalf("noObservationsError() = %q, want substring %q", got, tt.want)
			}
		})
	}
}
