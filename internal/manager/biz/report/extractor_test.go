package report

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ongridio/ongrid/internal/pkg/llm"
)

type fakeContentLLM struct {
	replies []string
	errs    []error
	calls   int
}

func (f *fakeContentLLM) Chat(_ context.Context, req llm.ChatReq) (*llm.ChatResp, error) {
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	reply := `{"version":"1","narrative":{"headline":"ok","paragraphs":[{"text":"p"}]},"advice":[]}`
	if len(f.replies) > 0 {
		if i < len(f.replies) {
			reply = f.replies[i]
		} else {
			reply = f.replies[len(f.replies)-1]
		}
	}
	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_schema" {
		// simulate one provider that rejects json_schema
		if f.errs == nil && f.calls == 1 && strings.Contains(reply, "REJECT_SCHEMA") {
			return nil, errors.New("response_format json_schema unsupported")
		}
	}
	return &llm.ChatResp{Assistant: llm.Message{Role: "assistant", Content: reply}}, nil
}

func TestContentExtractor_CoercesNestedDraft(t *testing.T) {
	draft := `{
		"report_meta": {"title": "周报"},
		"resource_overview": {"headline": "资源平稳", "narrative": "CPU 均值 2%。"},
		"recommendations": ["关注磁盘"]
	}`
	llmFake := &fakeContentLLM{replies: []string{`{
		"version":"1",
		"narrative":{"headline":"资源平稳","paragraphs":[{"text":"CPU 均值 2%。"}]},
		"advice":[{"text":"关注磁盘"}]
	}`}}
	ext := NewContentExtractor(llmFake, nil)
	got, err := ext.Extract(context.Background(), ExtractContentReq{
		RawOutput:  draft,
		Locale:     "zh",
		ParseError: "missing narrative.headline",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Narrative.Headline != "资源平稳" {
		t.Errorf("headline: %q", got.Narrative.Headline)
	}
	if len(got.Advice) != 1 || got.Advice[0].Text != "关注磁盘" {
		t.Errorf("advice: %+v", got.Advice)
	}
}

func TestContentExtractor_JSONSchemaFallback(t *testing.T) {
	llmFake := &fakeContentLLM{
		errs: []error{errors.New("response_format json_schema unsupported")},
		replies: []string{`{"version":"1","narrative":{"headline":"h","paragraphs":[{"text":"p"}]},"advice":[]}`},
	}
	ext := NewContentExtractor(llmFake, nil)
	got, err := ext.Extract(context.Background(), ExtractContentReq{RawOutput: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Narrative.Headline != "h" {
		t.Errorf("headline: %q", got.Narrative.Headline)
	}
	if llmFake.calls != 2 {
		t.Errorf("calls = %d, want 2 (schema then json_object)", llmFake.calls)
	}
}

func TestBuildSchemaCorrectionPrompt_IncludesSchema(t *testing.T) {
	p := buildSchemaCorrectionPrompt("facts block", `{"bad":true}`, "missing headline", "zh")
	if !strings.Contains(p, RequiredLLMOutputSchema()) {
		t.Error("correction prompt missing schema")
	}
	if !strings.Contains(p, "missing headline") {
		t.Error("correction prompt missing parse error")
	}
	if !strings.Contains(p, "facts block") {
		t.Error("correction prompt missing facts")
	}
}

func TestIsResponseFormatUnsupported(t *testing.T) {
	if !isResponseFormatUnsupported(errors.New("response_format json_schema unsupported")) {
		t.Error("expected unsupported")
	}
	if isResponseFormatUnsupported(errors.New("timeout")) {
		t.Error("timeout is not format unsupported")
	}
}
