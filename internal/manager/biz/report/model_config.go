package report

import (
	"context"
	"fmt"
	"strings"

	bizsetting "github.com/ongridio/ongrid/internal/manager/biz/setting"
	settingmodel "github.com/ongridio/ongrid/internal/manager/model/setting"
	"github.com/ongridio/ongrid/internal/pkg/errs"
	"github.com/ongridio/ongrid/internal/pkg/llm"
)

// ModelCatalog is the read-only provider list the report model picker uses.
type ModelCatalog interface {
	Providers() []llm.ProviderInfo
	Default() (provider, model string)
}

// ModelConfigService manages the fixed LLM used by the report worker.
type ModelConfigService struct {
	settings *bizsetting.Service
	catalog  ModelCatalog
}

// ModelConfigView is returned to the SPA model-settings page.
type ModelConfigView struct {
	Provider           string              `json:"provider"`
	Model              string              `json:"model"`
	UsePlatformDefault bool                `json:"use_platform_default"`
	PlatformDefault    modelChoiceDTO      `json:"platform_default"`
	Providers          []providerModelsDTO `json:"providers"`
}

type modelChoiceDTO struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type providerModelsDTO struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Models []string `json:"models"`
	Model  string   `json:"model,omitempty"`
}

// NewModelConfigService wires report model settings. catalog may be nil
// (empty provider list in GET; validation on PUT is provider-only).
func NewModelConfigService(settings *bizsetting.Service, catalog ModelCatalog) *ModelConfigService {
	return &ModelConfigService{settings: settings, catalog: catalog}
}

// Get returns the current pin plus the platform catalog for the picker.
func (s *ModelConfigService) Get(ctx context.Context) (ModelConfigView, error) {
	out := ModelConfigView{Providers: []providerModelsDTO{}}
	if s.catalog != nil {
		for _, p := range s.catalog.Providers() {
			out.Providers = append(out.Providers, providerModelsDTO{
				ID:     p.ID,
				Label:  p.Label,
				Models: append([]string(nil), p.Models...),
				Model:  p.Model,
			})
		}
		defProv, defModel := s.catalog.Default()
		out.PlatformDefault = modelChoiceDTO{Provider: defProv, Model: defModel}
	}
	if s.settings == nil {
		out.UsePlatformDefault = true
		return out, nil
	}
	prov, _, _ := s.settings.Get(ctx, settingmodel.CategoryReport, settingmodel.KeyReportLLMProvider)
	mdl, _, _ := s.settings.Get(ctx, settingmodel.CategoryReport, settingmodel.KeyReportLLMModel)
	out.Provider = strings.TrimSpace(prov)
	out.Model = strings.TrimSpace(mdl)
	out.UsePlatformDefault = out.Provider == ""
	return out, nil
}

// Set pins or clears the report worker model. Empty provider clears the
// pin and reverts to the platform default.
func (s *ModelConfigService) Set(ctx context.Context, provider, model string) (ModelConfigView, error) {
	if s.settings == nil {
		return ModelConfigView{}, fmt.Errorf("%w: report model settings unavailable", errs.ErrNotWiredYet)
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" {
		_ = s.settings.Delete(ctx, settingmodel.CategoryReport, settingmodel.KeyReportLLMProvider)
		_ = s.settings.Delete(ctx, settingmodel.CategoryReport, settingmodel.KeyReportLLMModel)
		return s.Get(ctx)
	}
	if err := s.validateChoice(provider, model); err != nil {
		return ModelConfigView{}, err
	}
	if model == "" {
		model = s.defaultModelFor(provider)
	}
	if model == "" {
		return ModelConfigView{}, fmt.Errorf("%w: model required for provider %q", errs.ErrInvalid, provider)
	}
	if err := s.settings.Set(ctx, settingmodel.CategoryReport, settingmodel.KeyReportLLMProvider, provider, false); err != nil {
		return ModelConfigView{}, err
	}
	if err := s.settings.Set(ctx, settingmodel.CategoryReport, settingmodel.KeyReportLLMModel, model, false); err != nil {
		return ModelConfigView{}, err
	}
	return s.Get(ctx)
}

// ResolveSpawnModel returns the provider/model to pin on the reporter
// worker. Empty strings mean "use RoutingChatModel default".
func (s *ModelConfigService) ResolveSpawnModel(ctx context.Context) (provider, model string) {
	if s == nil || s.settings == nil {
		return "", ""
	}
	prov, ok, err := s.settings.Get(ctx, settingmodel.CategoryReport, settingmodel.KeyReportLLMProvider)
	if err != nil || !ok {
		return "", ""
	}
	prov = strings.TrimSpace(prov)
	if prov == "" {
		return "", ""
	}
	mdl, _, _ := s.settings.Get(ctx, settingmodel.CategoryReport, settingmodel.KeyReportLLMModel)
	mdl = strings.TrimSpace(mdl)
	if mdl == "" {
		mdl = s.defaultModelFor(prov)
	}
	return prov, mdl
}

// ProviderConfigured reports whether provider has a usable API key in
// the live LLM catalog (used by ready checks).
func (s *ModelConfigService) ProviderConfigured(provider string) bool {
	provider = strings.TrimSpace(provider)
	if provider == "" || s.catalog == nil {
		return false
	}
	for _, p := range s.catalog.Providers() {
		if p.ID == provider {
			return true
		}
	}
	return false
}

func (s *ModelConfigService) validateChoice(provider, model string) error {
	if s.catalog == nil {
		return nil
	}
	var info *llm.ProviderInfo
	for _, p := range s.catalog.Providers() {
		if p.ID == provider {
			cp := p
			info = &cp
			break
		}
	}
	if info == nil {
		return fmt.Errorf("%w: LLM provider %q is not configured", errs.ErrInvalid, provider)
	}
	if model == "" {
		return nil
	}
	for _, m := range info.Models {
		if m == model {
			return nil
		}
	}
	if info.Model == model {
		return nil
	}
	return fmt.Errorf("%w: model %q is not available for provider %q", errs.ErrInvalid, model, provider)
}

func (s *ModelConfigService) defaultModelFor(provider string) string {
	if s.catalog == nil {
		return ""
	}
	for _, p := range s.catalog.Providers() {
		if p.ID == provider {
			if p.Model != "" {
				return p.Model
			}
			if len(p.Models) > 0 {
				return p.Models[0]
			}
			return ""
		}
	}
	return ""
}
