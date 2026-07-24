package frontierbound

import (
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
)

// authnFailTracker counts edge authn failures per remote addr so operators
// can correlate WARN lines with a specific host without logging credentials
// or revealing whether an access key exists.
type authnFailTracker struct {
	mu      sync.Mutex
	byAddr  map[string]uint64
	unknown uint64
}

func newAuthnFailTracker() *authnFailTracker {
	return &authnFailTracker{byAddr: make(map[string]uint64)}
}

func (t *authnFailTracker) record(addr net.Addr) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := authnAddrKey(addr)
	if key == "" {
		t.unknown++
		return t.unknown
	}
	t.byAddr[key]++
	return t.byAddr[key]
}

func authnAddrKey(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	s := strings.TrimSpace(addr.String())
	return s
}

func frontierAuthnDebug() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ONGRID_FRONTIER_AUTHN_DEBUG")))
	return v == "1" || v == "true" || v == "yes"
}

// accessKeySuffix returns the last four runes of an access key for debug
// correlation. Never log the full key or the secret.
func accessKeySuffix(accessKey string) string {
	accessKey = strings.TrimSpace(accessKey)
	if accessKey == "" {
		return ""
	}
	r := []rune(accessKey)
	if len(r) <= 4 {
		return "****"
	}
	return "…" + string(r[len(r)-4:])
}

func logEdgeAuthnFailed(log *slog.Logger, tracker *authnFailTracker, addr net.Addr, accessKey string, err error) {
	if log == nil {
		log = slog.Default()
	}
	failCount := tracker.record(addr)
	addrStr := authnAddrKey(addr)
	attrs := []any{
		slog.Any("err", err),
		slog.Uint64("fail_count", failCount),
	}
	if addrStr != "" {
		attrs = append(attrs, slog.String("addr", addrStr))
	} else {
		attrs = append(attrs, slog.String("addr", "unknown"))
	}
	if frontierAuthnDebug() {
		if suffix := accessKeySuffix(accessKey); suffix != "" {
			attrs = append(attrs, slog.String("access_key_suffix", suffix))
		}
	}
	log.Warn("frontierbound: edge authn failed", attrs...)
}
