package directivefile

import (
	"os"
	"testing"
)

func readTestdata(t *testing.T, path string) []byte {
	t.Helper()

	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return input
}
