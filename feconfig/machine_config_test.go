package feconfig

import (
	"encoding/json"
	"errors"
	"testing"
)

// validConfigRaw is a valid config; each negative case corrupts it locally.
const validConfigRaw = `{
    "options": {
        "version": 1
    },
    "instances": [
        {
            "id": "3f2504e0-4f89-41d3-9a0c-0305e82c3301",
            "mod_id": "dns.resolver",
            "config": {
                "enable_cache": true
            },
            "deps": []
        },
        {
            "id": "7b1d2a5e-9f4c-4a8b-8c3d-2e5f1a6b7c8d",
            "mod_id": "dns.forwarder",
            "config": {
                "dns_inst": "3f2504e0-4f89-41d3-9a0c-0305e82c3301",
                "upstream": "1.1.1.1"
            },
            "deps": [
                "3f2504e0-4f89-41d3-9a0c-0305e82c3301"
            ]
        },
        {
            "id": "c0d1e2f3-4a5b-4c6d-9e0f-1a2b3c4d5e6f",
            "mod_id": "http.static",
            "config": {
                "data": {}
            },
            "deps": []
        }
    ]
}`

func TestMachineConfigValidate(t *testing.T) {
	u1 := "3f2504e0-4f89-41d3-9a0c-0305e82c3301"
	u2 := "7b1d2a5e-9f4c-4a8b-8c3d-2e5f1a6b7c8d"
	u3 := "c0d1e2f3-4a5b-4c6d-9e0f-1a2b3c4d5e6f"
	validID := "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e" // valid v4

	tests := []struct {
		name string
		raw  string
		want error // errors.Is(err, want) must hold; nil means it should pass
	}{
		{
			name: "empty deps",
			raw:  validConfigRaw,
		},
		{
			name: "unknown top-level field",
			raw:  `{"extra": 1, "options": {"version": 1}, "instances": []}`,
			want: ErrMalformedConfig,
		},
		{
			name: "unknown options field",
			raw:  `{"options": {"version": 1, "log_level": "debug"}, "instances": []}`,
			want: ErrMalformedConfig,
		},
		{
			name: "unknown instance field",
			raw: `{"options": {"version": 1}, "instances": [{
				"id": "` + validID + `", "mod_id": "dns.resolver",
				"config": {}, "deps": [], "unexpected": true}]}`,
			want: ErrMalformedConfig,
		},
		{
			name: "version type mismatch",
			raw:  `{"options": {"version": "1"}, "instances": []}`,
			want: ErrMalformedConfig,
		},
		{
			name: "empty mod_id",
			raw: `{"options": {"version": 1}, "instances": [{
				"id": "` + validID + `", "mod_id": "", "config": {}, "deps": []}]}`,
			want: ErrMissingModuleID,
		},
		{
			name: "invalid uuid id",
			raw: `{"options": {"version": 1}, "instances": [{
				"id": "not-a-uuid", "mod_id": "dns.resolver", "config": {}, "deps": []}]}`,
			want: ErrInvalidID,
		},
		{
			name: "id non-v4",
			// valid UUID but version bits = 1 (high nibble of byte 6)
			raw: `{"options": {"version": 1}, "instances": [{
				"id": "3f2504e0-4f89-11d3-9a0c-0305e82c3301", "mod_id": "dns.resolver", "config": {}, "deps": []}]}`,
			want: ErrInvalidID,
		},
		{
			name: "dep references undefined instance",
			raw: `{"options": {"version": 1}, "instances": [{
				"id": "` + u1 + `", "mod_id": "dns.resolver", "config": {}, "deps": ["` + u2 + `"]}]}`,
			want: ErrUndefinedDep,
		},
		{
			name: "duplicate id",
			// passes strict decoding and UUID checks, but declaring the same id twice is invalid
			raw: `{"options": {"version": 1}, "instances": [
				{"id": "` + u1 + `", "mod_id": "dns.resolver", "config": {}, "deps": []},
				{"id": "` + u1 + `", "mod_id": "dns.forwarder", "config": {}, "deps": []}]}`,
			want: ErrDuplicateID,
		},
		{
			name: "valid dep chain",
			raw: `{"options": {"version": 1}, "instances": [
				{"id": "` + u1 + `", "mod_id": "dns.resolver", "config": {}, "deps": []},
				{"id": "` + u2 + `", "mod_id": "dns.forwarder", "config": {}, "deps": ["` + u1 + `"]},
				{"id": "` + u3 + `", "mod_id": "http.static", "config": {}, "deps": ["` + u2 + `"]}]}`,
		},
		{
			name: "cyclic deps",
			// a depends on b, b depends on a → cycle must be rejected
			raw: `{"options": {"version": 1}, "instances": [
				{"id": "` + u1 + `", "mod_id": "dns.resolver", "config": {}, "deps": ["` + u2 + `"]},
				{"id": "` + u2 + `", "mod_id": "dns.forwarder", "config": {}, "deps": ["` + u1 + `"]}]}`,
			want: ErrCyclicDeps,
		},
		{
			name: "self dep cycle",
			// an instance depending on itself is a cycle
			raw: `{"options": {"version": 1}, "instances": [
				{"id": "` + u1 + `", "mod_id": "dns.resolver", "config": {}, "deps": ["` + u1 + `"]}]}`,
			want: ErrCyclicDeps,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MachineConfigValidate(json.RawMessage(tt.raw))
			if tt.want == nil {
				if err != nil {
					t.Fatalf("expected pass, got error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("want errors.Is(err, %v), got err = %v", tt.want, err)
			}
		})
	}
}
