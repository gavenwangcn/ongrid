// Package logger wraps log/slog with ongrid conventions.
//
// gospec red line: structured logs only, JSON handler, never log raw user
// content (chat messages, request bodies, secrets). Injecting trace_id /
// org_id is handled at call sites via slog attributes.
package logger

import (
	"log/slog"
	"os"
	"strings"
	"sync"

	armlog "github.com/jumboframes/armorigo/log"
)

// New returns a *slog.Logger that writes JSON lines to stderr at the given
// minimum level.
func New(level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(h)
}

// NewFromEnv returns a logger whose minimum level comes from ONGRID_LOG_LEVEL
// (debug | info | warn | error). Empty or unknown values default to info.
func NewFromEnv() *slog.Logger {
	quietThirdPartyLogs()
	return New(ParseLevel(os.Getenv("ONGRID_LOG_LEVEL")))
}

var quietThirdPartyOnce sync.Once

// quietThirdPartyLogs turns down chatty INFO lines from tunnel deps
// (geminio uses armorigo/log and logs per-packet "writePkt() is running").
func quietThirdPartyLogs() {
	quietThirdPartyOnce.Do(func() {
		armlog.SetLevel(armlog.LevelWarn)
	})
}

// ParseLevel maps a string to slog.Level. Empty → info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithService returns a logger decorated with a "service" attribute.
// Used by cmd/ongrid and cmd/ongrid-edge at startup.
func WithService(l *slog.Logger, name string) *slog.Logger {
	return l.With(slog.String("service", name))
}
