package flow

import (
	"context"
	"errors"
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
