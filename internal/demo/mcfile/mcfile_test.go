package mcfile

import (
	"errors"
	"testing"

	"github.com/n24-x/fe/feconfig"
)

const idA = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

// TestParse covers the JSON -> value step, including the strict decoding
// (unknown fields rejected) that is this helper's responsibility alone.
func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr error // errors.Is target; nil means it must parse
	}{
		{
			name: "valid config",
			raw:  `{"options":{"bus":{"router_capacity":8}},"instances":[{"id":"` + idA + `","mod_id":"dns","config":{},"deps":[]}]}`,
		},
		{
			name: "options omitted",
			raw:  `{"instances":[]}`,
		},
		{
			name: "empty instances",
			raw:  `{}`,
		},
		{
			name:    "malformed json",
			raw:     `{"instances":`,
			wantErr: feconfig.ErrMalformedConfig,
		},
		{
			name:    "unknown top-level field",
			raw:     `{"extra":1,"instances":[]}`,
			wantErr: feconfig.ErrMalformedConfig,
		},
		{
			name:    "unknown instance field",
			raw:     `{"instances":[{"id":"` + idA + `","mod_id":"dns","config":{},"deps":[],"unexpected":true}]}`,
			wantErr: feconfig.ErrMalformedConfig,
		},
		{
			name:    "unknown options field",
			raw:     `{"options":{"bogus":1},"instances":[]}`,
			wantErr: feconfig.ErrMalformedConfig,
		},
		{
			name:    "unknown bus option field",
			raw:     `{"options":{"bus":{"bogus":1}},"instances":[]}`,
			wantErr: feconfig.ErrMalformedConfig,
		},
		{
			// The framework handle is not config: json:"-" makes strict
			// decoding reject it rather than accept a nil-handler logger.
			name:    "logger is not decodable",
			raw:     `{"options":{"logger":{}},"instances":[]}`,
			wantErr: feconfig.ErrMalformedConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.raw))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want errors.Is(err, %v), got err = %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("Parse returned nil config without error")
			}
		})
	}
}

// TestParseOptions verifies the framework facility options survive the JSON
// round trip.
func TestParseOptions(t *testing.T) {
	mc, err := Parse([]byte(`{"options":{"bus":{"router_capacity":8}},"instances":[]}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := mc.Options.Bus.RouterCapacity; got != 8 {
		t.Fatalf("Options.Bus.RouterCapacity = %d, want 8", got)
	}
}
