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
