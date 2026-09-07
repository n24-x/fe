package timex

import (
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		// standard Go durations pass through unchanged
		{"300ms", 300 * time.Millisecond},
		{"1.5h", 90 * time.Minute},
		{"2h45m", 2*time.Hour + 45*time.Minute},
		{"-1h30m", -(time.Hour + 30*time.Minute)},

		// "d" extension: 1d = 24h, composable with other units
		{"1d", 24 * time.Hour},
		{"0.5d", 12 * time.Hour},
		{"1.5d", 36 * time.Hour},
		{"-1d", -24 * time.Hour},
		{"1d12h", 36 * time.Hour},
		{"2d6h30m", 54*time.Hour + 30*time.Minute},
	}
	for _, tt := range tests {
		got, err := ParseDuration(tt.in)
		if err != nil {
			t.Errorf("ParseDuration(%q) returned error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseDurationErrors(t *testing.T) {
	tests := []string{
		"",                        // empty string, missing unit
		"d",                       // "d" without a leading number
		"ad",                      // non-numeric value before "d"
		"5",                       // missing unit
		"xxxx",                    // unknown unit
		strings.Repeat("1", 1025), // too long (len > 1024)
	}
	for _, in := range tests {
		if got, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) = %v, want error", in, got)
		}
	}
}
