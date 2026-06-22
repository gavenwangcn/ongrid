package report

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	bizsetting "github.com/ongridio/ongrid/internal/manager/biz/setting"
	model "github.com/ongridio/ongrid/internal/manager/model/setting"
	"github.com/ongridio/ongrid/internal/pkg/errs"
	"github.com/ongridio/ongrid/internal/pkg/llm"
)

type fakeModelCatalog struct {
	providers []llm.ProviderInfo
	defProv   string
	defModel  string
}

func (f fakeModelCatalog) Providers() []llm.ProviderInfo { return f.providers }
func (f fakeModelCatalog) Default() (string, string)   { return f.defProv, f.defModel }

type memSettingRepo struct {
	mu   sync.Mutex
	rows map[string]*model.Setting
}

func newMemSettingRepo() *memSettingRepo {
	return &memSettingRepo{rows: make(map[string]*model.Setting)}
}

func (r *memSettingRepo) key(cat, key string) string { return cat + "|" + key }

func (r *memSettingRepo) Get(_ context.Context, category, key string) (*model.Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[r.key(category, key)]
	if !ok {
		return nil, errs.ErrNotFound
	}
	cp := *row
	return &cp, nil
}

func (r *memSettingRepo) Set(_ context.Context, category, key, value string, sensitive bool) (*model.Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := &model.Setting{
		Category:  category,
		Key:       key,
		Value:     value,
		Sensitive: sensitive,
		UpdatedAt: time.Now(),
	}
	r.rows[r.key(category, key)] = row
	return row, nil
}

func (r *memSettingRepo) List(_ context.Context, category string) ([]*model.Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*model.Setting, 0)
	for _, row := range r.rows {
		if category != "" && row.Category != category {
			continue
		}
		cp := *row
		out = append(out, &cp)
	}
	return out, nil
}

func (r *memSettingRepo) Delete(_ context.Context, category, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(category, key)
	if _, ok := r.rows[k]; !ok {
		return errs.ErrNotFound
	}
	delete(r.rows, k)
	return nil
}

func testModelConfigService(t *testing.T) *ModelConfigService {
	t.Helper()
	catalog := fakeModelCatalog{
		providers: []llm.ProviderInfo{
			{ID: "openai", Label: "OpenAI", Models: []string{"gpt-4o", "gpt-5"}, Model: "gpt-4o"},
			{ID: "anthropic", Label: "Anthropic", Models: []string{"claude-sonnet-4-6"}, Model: "claude-sonnet-4-6"},
		},
		defProv:  "openai",
		defModel: "gpt-4o",
	}
	return NewModelConfigService(bizsetting.New(newMemSettingRepo(), nil), catalog)
}

func TestModelConfig_GetDefault(t *testing.T) {
	svc := testModelConfigService(t)
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.UsePlatformDefault {
		t.Fatalf("UsePlatformDefault = false, want true")
	}
	if got.PlatformDefault.Provider != "openai" || got.PlatformDefault.Model != "gpt-4o" {
		t.Fatalf("platform default = %+v", got.PlatformDefault)
	}
	if len(got.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(got.Providers))
	}
}

func TestModelConfig_SetAndResolve(t *testing.T) {
	svc := testModelConfigService(t)
	ctx := context.Background()

	got, err := svc.Set(ctx, "anthropic", "claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	if got.UsePlatformDefault || got.Provider != "anthropic" || got.Model != "claude-sonnet-4-6" {
		t.Fatalf("set result = %+v", got)
	}
	prov, mdl := svc.ResolveSpawnModel(ctx)
	if prov != "anthropic" || mdl != "claude-sonnet-4-6" {
		t.Fatalf("resolve = (%q,%q)", prov, mdl)
	}
	if !svc.ProviderConfigured("anthropic") {
		t.Fatal("anthropic should be configured")
	}
}

func TestModelConfig_ClearPin(t *testing.T) {
	svc := testModelConfigService(t)
	ctx := context.Background()
	if _, err := svc.Set(ctx, "openai", "gpt-5"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Set(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !got.UsePlatformDefault {
		t.Fatalf("after clear: %+v", got)
	}
	prov, mdl := svc.ResolveSpawnModel(ctx)
	if prov != "" || mdl != "" {
		t.Fatalf("resolve after clear = (%q,%q)", prov, mdl)
	}
}

func TestModelConfig_InvalidProvider(t *testing.T) {
	svc := testModelConfigService(t)
	_, err := svc.Set(context.Background(), "unknown", "x")
	if err == nil || !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("Set unknown provider err = %v", err)
	}
}

func TestModelConfig_InvalidModel(t *testing.T) {
	svc := testModelConfigService(t)
	_, err := svc.Set(context.Background(), "openai", "not-a-model")
	if err == nil || !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("Set invalid model err = %v", err)
	}
}

func TestModelConfig_DefaultModelWhenEmpty(t *testing.T) {
	svc := testModelConfigService(t)
	ctx := context.Background()
	got, err := svc.Set(ctx, "openai", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "gpt-4o" {
		t.Fatalf("model = %q, want gpt-4o", got.Model)
	}
}
