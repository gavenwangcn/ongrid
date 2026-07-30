package store

import (
	"testing"
)

func TestBuildResourceExprsIncludesNetwork(t *testing.T) {
	exprs := buildResourceExprs("168h", []uint64{7, 9})
	for _, key := range []string{"cpu", "mem", "disk", "net_rx", "net_tx"} {
		if _, ok := exprs[key]; !ok {
			t.Fatalf("buildResourceExprs missing %q", key)
		}
	}
	if !contains(exprs["net_rx"].inner, networkIfaceFilter) {
		t.Errorf("net_rx inner missing iface filter: %s", exprs["net_rx"].inner)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})())
}
