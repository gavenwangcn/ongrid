package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestStripCodeFences_ChineseProsePrefix(t *testing.T) {
	raw := "根据你的需求，工作流如下：\n{\"name\":\"巡检\",\"description\":\"d\",\"graph\":{\"nodes\":[],\"edges\":[]}}"
	got := stripCodeFences(raw)
	if !strings.HasPrefix(got, "{") || !strings.HasSuffix(got, "}") {
		t.Fatalf("stripCodeFences = %q, want JSON object only", got)
	}
}

func TestStripCodeFences_MarkdownFence(t *testing.T) {
	raw := "说明文字\n```json\n{\"name\":\"x\",\"description\":\"\",\"graph\":{\"nodes\":[],\"edges\":[]}}\n```\n"
	got := stripCodeFences(raw)
	if !strings.Contains(got, `"name":"x"`) {
		t.Fatalf("stripCodeFences = %q", got)
	}
}

func TestParseGeneratedGraph_Valid(t *testing.T) {
	raw := `{"name":"设备巡检","description":"巡检负载","graph":{"nodes":[{"id":"t","type":"trigger.manual","name":"手动","config":{}}],"edges":[]}}`
	in, err := parseGeneratedGraph(raw)
	if err != nil {
		t.Fatal(err)
	}
	if in.Name != "设备巡检" {
		t.Fatalf("name = %q", in.Name)
	}
	if !strings.Contains(in.GraphJSON, `"trigger.manual"`) {
		t.Fatalf("graph = %s", in.GraphJSON)
	}
}

func TestParseGeneratedGraph_DefaultName(t *testing.T) {
	raw := `{"description":"","graph":{"nodes":[{"id":"t","type":"trigger.manual","config":{}}],"edges":[]}}`
	in, err := parseGeneratedGraph(raw)
	if err != nil {
		t.Fatal(err)
	}
	if in.Name != "AI 生成的工作流" {
		t.Fatalf("name = %q", in.Name)
	}
}

func TestGenerateGraph_RetriesOnInvalidJSON(t *testing.T) {
	calls := 0
	uc := NewUsecase(nil, nil, nil, nil).WithLLM(retryFakeLLM{
		replies: []string{
			"根据需求生成如下工作流",
			`{"name":"重试成功","description":"","graph":{"nodes":[{"id":"t","type":"trigger.manual","config":{}}],"edges":[]}}`,
		},
		calls: &calls,
	})
	in, err := uc.GenerateGraph(context.Background(), "做一个巡检工作流")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if in.Name != "重试成功" {
		t.Fatalf("name = %q", in.Name)
	}
}

func TestGenerateGraph_RetriesOnInvalidGraphType(t *testing.T) {
	calls := 0
	badGraph := `{"name":"采购平台巡检","description":"x","graph":{"title":"巡检","type":"inspection_report","nodes":[{"id":"purchase_platform","type":"system","name":"采购管理平台","config":{}}],"edges":[]}}`
	okGraph := `{"name":"采购平台巡检","description":"x","graph":{"nodes":[{"id":"t","type":"trigger.manual","config":{}},{"id":"a","type":"agent","name":"巡检","config":{"persona":"default","instruction":"巡检采购管理平台"}}],"edges":[{"id":"1","source":"t","target":"a"}]}}`
	uc := NewUsecase(nil, nil, nil, nil).WithLLM(retryFakeLLM{
		replies: []string{badGraph, okGraph},
		calls:   &calls,
	})
	in, err := uc.GenerateGraph(context.Background(), "巡检采购管理平台所有设备")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if !strings.Contains(in.GraphJSON, `"agent"`) {
		t.Fatalf("graph = %s", in.GraphJSON)
	}
}

func TestGenRetryUserPrompt_GraphValidate(t *testing.T) {
	badOut := `{"name":"x","graph":{"nodes":[{"id":"purchase_platform","type":"system","config":{}}],"edges":[]}}`
	diag := graphParseDiag{
		stage:     "graph_validate",
		extracted: badOut,
		err:       fmt.Errorf(`graph: unknown node type "system" (node purchase_platform)`),
	}
	prompt := genRetryUserPrompt("巡检采购管理平台", []graphGenFailure{{
		attempt: 1,
		output:  badOut,
		diag:    diag,
	}})
	for _, want := range []string{
		"最近失败尝试",
		"第 1 次尝试",
		"校验阶段：graph_validate",
		"校验错误",
		`unknown node type "system"`,
		"你当时的模型输出",
		"purchase_platform",
		"修正要求",
		"query_systems",
		"trigger.manual",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("retry prompt missing %q\n%s", want, prompt)
		}
	}
}

func TestAppendGraphGenFailure_KeepsLastTwo(t *testing.T) {
	var fs []graphGenFailure
	for i := 1; i <= 5; i++ {
		fs = appendGraphGenFailure(fs, graphGenFailure{attempt: i})
	}
	if len(fs) != maxGraphGenRetryContext {
		t.Fatalf("len = %d, want %d", len(fs), maxGraphGenRetryContext)
	}
	if fs[0].attempt != 4 || fs[1].attempt != 5 {
		t.Fatalf("attempts = %d,%d, want 4,5", fs[0].attempt, fs[1].attempt)
	}
}

func TestGenRetryUserPrompt_OnlyLastTwoRounds(t *testing.T) {
	makeFailure := func(n int) graphGenFailure {
		return graphGenFailure{
			attempt: n,
			output:  fmt.Sprintf(`{"attempt_marker":%d}`, n),
			diag: graphParseDiag{
				stage:     "graph_validate",
				extracted: fmt.Sprintf(`{"attempt_marker":%d}`, n),
				err:       fmt.Errorf("err-%d", n),
			},
		}
	}
	recent := []graphGenFailure{makeFailure(3), makeFailure(4)}
	prompt := genRetryUserPrompt("需求", recent)
	if strings.Contains(prompt, "attempt_marker\":1") || strings.Contains(prompt, "attempt_marker\":2") {
		t.Fatalf("prompt should not contain attempts 1-2:\n%s", prompt)
	}
	for _, want := range []string{`attempt_marker":3`, `attempt_marker":4`, "err-3", "err-4"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestParseGeneratedGraph_RejectsUnknownNodeType(t *testing.T) {
	raw := `{"name":"x","description":"","graph":{"nodes":[{"id":"n","type":"system","config":{}}],"edges":[]}}`
	_, diag := parseGeneratedGraphDiag(raw)
	if diag.err == nil {
		t.Fatal("expected graph_validate error")
	}
	if diag.stage != "graph_validate" {
		t.Fatalf("stage = %q", diag.stage)
	}
	if !strings.Contains(diag.err.Error(), "unknown node type") {
		t.Fatalf("err = %v", diag.err)
	}
}

func TestGenSystemPrompt_ForbidsReportGraphShape(t *testing.T) {
	prompt := genSystemPrompt(nil)
	for _, want := range []string{
		"禁止在 graph 顶层写 title",
		"inspection_report",
		"query_systems",
		"trigger.manual",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestGraphGenAttemptLabel(t *testing.T) {
	if got := graphGenAttemptLabel(1); got != "initial" {
		t.Fatalf("attempt 1 = %q", got)
	}
	if got := graphGenAttemptLabel(2); got != "retry-1" {
		t.Fatalf("attempt 2 = %q", got)
	}
	if got := graphGenAttemptLabel(8); got != "retry-7" {
		t.Fatalf("attempt 8 = %q", got)
	}
}

func TestGenerateGraph_StopsAtMaxAttempts(t *testing.T) {
	calls := 0
	bad := `{"name":"x","description":"","graph":{"nodes":[{"id":"n","type":"system","config":{}}],"edges":[]}}`
	replies := make([]string, maxGraphGenAttempts)
	for i := range replies {
		replies[i] = bad
	}
	uc := NewUsecase(nil, nil, nil, nil).WithLLM(retryFakeLLM{replies: replies, calls: &calls})
	_, err := uc.GenerateGraph(context.Background(), "做一个巡检工作流")
	if err == nil {
		t.Fatal("expected error after max attempts")
	}
	if calls != maxGraphGenAttempts {
		t.Fatalf("calls = %d, want %d", calls, maxGraphGenAttempts)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("after %d attempts", maxGraphGenAttempts)) {
		t.Fatalf("err = %v", err)
	}
}

type retryFakeLLM struct {
	replies []string
	calls   *int
}

func (f retryFakeLLM) RunLLM(_ context.Context, _, _ string) (string, error) {
	return f.next()
}

func (f retryFakeLLM) RunLLMJSON(_ context.Context, _, _ string) (string, error) {
	return f.next()
}

func (f retryFakeLLM) next() (string, error) {
	if f.calls == nil {
		return "", errors.New("calls nil")
	}
	*f.calls++
	if *f.calls > len(f.replies) {
		return "", errors.New("no more replies")
	}
	return f.replies[*f.calls-1], nil
}
