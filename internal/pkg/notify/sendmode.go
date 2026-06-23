package notify

import "context"

// WebhookSendMode selects how outbound webhook POSTs are delivered.
type WebhookSendMode string

const (
	// WebhookSendModeCurl posts via the container curl binary.
	WebhookSendModeCurl WebhookSendMode = "curl"
	// WebhookSendModeHTTP posts via Go net/http (with curl fallback on
	// retryable network errors — the pre-2026-06 behaviour).
	WebhookSendModeHTTP WebhookSendMode = "http"
)

type webhookSendModeContextKey struct{}

// ContextWithWebhookSendMode attaches the send mode for one delivery.
func ContextWithWebhookSendMode(ctx context.Context, mode WebhookSendMode) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, webhookSendModeContextKey{}, NormalizeWebhookSendMode(string(mode)))
}

// WebhookSendModeFromContext returns the mode stored on ctx, or the
// product default (curl) when unset.
func WebhookSendModeFromContext(ctx context.Context) WebhookSendMode {
	if ctx == nil {
		return WebhookSendModeCurl
	}
	v, ok := ctx.Value(webhookSendModeContextKey{}).(WebhookSendMode)
	if !ok || v == "" {
		return WebhookSendModeCurl
	}
	return v
}

// NormalizeWebhookSendMode maps wire / DB values to a supported mode.
// Unknown values fall back to curl (the safer default for Feishu CDN).
func NormalizeWebhookSendMode(raw string) WebhookSendMode {
	switch WebhookSendMode(raw) {
	case WebhookSendModeHTTP:
		return WebhookSendModeHTTP
	default:
		return WebhookSendModeCurl
	}
}
