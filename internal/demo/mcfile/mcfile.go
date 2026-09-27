// Package mcfile is a development-only helper: it reads a machine config file
// into a feconfig.MachineConfig.
//
// It lives under internal/demo because the framework does not own the step
// before a machine config exists. What a human writes, in what format, and the
// adapter that expands it into an MC are the application's business — fe
// exports no MC parser, and takes a MachineConfig value however it was built.
// This is the parser the demo and the config fixtures use.
//
// Decoding is strict: unknown fields are rejected, so a typo in a config file
// is reported instead of being silently ignored.
package mcfile

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/n24-x/fe/feconfig"
)

// Parse decodes a machine config from its JSON representation. Decoding is
// strict, as described in the package comment.
func Parse(raw []byte) (*feconfig.MachineConfig, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var mc feconfig.MachineConfig
	if err := dec.Decode(&mc); err != nil {
		return nil, fmt.Errorf("%w: %w", feconfig.ErrMalformedConfig, err)
	}
	return &mc, nil
}
