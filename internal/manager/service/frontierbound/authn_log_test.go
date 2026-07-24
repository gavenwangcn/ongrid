package frontierbound

import (
	"net"
	"testing"
)

func TestAuthnFailTracker_RecordByAddr(t *testing.T) {
	tr := newAuthnFailTracker()
	addr := &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 12345}

	if got := tr.record(addr); got != 1 {
		t.Fatalf("first record = %d, want 1", got)
	}
	if got := tr.record(addr); got != 2 {
		t.Fatalf("second record = %d, want 2", got)
	}
	if got := tr.record(nil); got != 1 {
		t.Fatalf("unknown addr first = %d, want 1", got)
	}
}

func TestAccessKeySuffix(t *testing.T) {
	if got := accessKeySuffix("ak_live_abcdefgh"); got != "…efgh" {
		t.Fatalf("suffix = %q", got)
	}
	if got := accessKeySuffix("ab"); got != "****" {
		t.Fatalf("short suffix = %q", got)
	}
	if got := accessKeySuffix(""); got != "" {
		t.Fatalf("empty suffix = %q", got)
	}
}

func TestAuthnAddrKey(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9999}
	if got := authnAddrKey(addr); got == "" {
		t.Fatal("expected non-empty addr key")
	}
	if got := authnAddrKey(nil); got != "" {
		t.Fatalf("nil addr key = %q, want empty", got)
	}
}
