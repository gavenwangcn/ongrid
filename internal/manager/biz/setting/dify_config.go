package setting

import (
	"context"

	"github.com/ongridio/ongrid/internal/pkg/llm"
)

// DifyConfigAdapter implements llm.DifyConfigResolver by merging env
// defaults with system_settings.llm.dify_* rows.
type DifyConfigAdapter struct {
	resolver *LLMSettingsResolver
	env      llm.DifyConfig
	extras   DifyEnvDefaults
}

// NewDifyConfigAdapter wires the adapter used by llm.MultiClient when
// building dify sub-clients.
func NewDifyConfigAdapter(r *LLMSettingsResolver, env llm.DifyConfig, extras DifyEnvDefaults) *DifyConfigAdapter {
	return &DifyConfigAdapter{resolver: r, env: env, extras: extras}
}

// ResolveDifyConfig implements llm.DifyConfigResolver.
func (a *DifyConfigAdapter) ResolveDifyConfig(ctx context.Context) (llm.DifyConfig, bool) {
	if a == nil || a.resolver == nil {
		return llm.DifyConfig{}, false
	}
	cfg := a.resolver.ResolveDifyConfig(ctx, a.env, a.extras)
	return cfg, cfg.Configured()
}
