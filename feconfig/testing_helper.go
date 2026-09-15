package feconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ParseHelper decodes a MachineConfig from its JSON representation. Decoding
// is strict: unknown fields are rejected, so a typo in a config file is
// reported instead of being silently ignored.
//
// TEMPORARY: it stands in for the HC -> MC adapter, which will own parsing of
// a human-readable config (and its dialects). It exists so tests and the demo
// can turn JSON into a MachineConfig today; it will be removed once the
// adapter lands.
func ParseHelper(raw json.RawMessage) (*MachineConfig, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var mc MachineConfig
	if err := dec.Decode(&mc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedConfig, err)
	}
	return &mc, nil
}
