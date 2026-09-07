package hashx

import "testing"

func TestHashString(t *testing.T) {
	// deterministic
	if HashString("hello") != HashString("hello") {
		t.Fatal("HashString must be deterministic")
	}

	// distinct inputs differ (collisions are theoretically possible for any
	// hash, but for these inputs a collision is a real bug)
	pairs := []struct{ a, b string }{
		{"", "x"},
		{"hello", "hello!"},
		{"abc", "abd"},
		{"a", "aa"},
	}
	for _, p := range pairs {
		if HashString(p.a) == HashString(p.b) {
			t.Errorf("HashString(%q) == HashString(%q) == %d: want distinct", p.a, p.b, HashString(p.a))
		}
	}
}
