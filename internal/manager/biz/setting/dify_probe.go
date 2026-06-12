package setting

import (
	"context"
	"fmt"
	"time"

	"github.com/ongridio/ongrid/internal/pkg/llm"
)

// DifyURLProbe exercises the configured CheryGPT / Dify Service API with
// a minimal blocking chat-messages call. Used by the Integrations /
// LLM settings "测试连接" button and the system-health check.
type DifyURLProbe struct {
	resolver *LLMSettingsResolver
	env      llm.DifyConfig
	extras   DifyEnvDefaults
	timeout  time.Duration
}

// NewDifyURLProbe wires a probe against the resolver. resolver must be
// non-nil; Probe returns an error when CheryGPT is not configured.
func NewDifyURLProbe(r *LLMSettingsResolver, env llm.DifyConfig, extras DifyEnvDefaults) *DifyURLProbe {
	return &DifyURLProbe{resolver: r, env: env, extras: extras, timeout: 15 * time.Second}
}

// Probe runs a lightweight chat-messages round-trip.
func (p *DifyURLProbe) Probe(ctx context.Context) error {
	if p == nil || p.resolver == nil {
		return fmt.Errorf("dify probe not wired")
	}
	cfg := p.resolver.ResolveDifyConfig(ctx, p.env, p.extras)
	if !cfg.Configured() {
		return fmt.Errorf("cherygpt is not configured (api key and base url required)")
	}
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	client := llm.NewDifyClient(cfg, nil, nil)
	_, err := client.Chat(ctx, llm.ChatReq{
		Messages: []llm.Message{{Role: "user", Content: "ping"}},
	})
	return err
}
