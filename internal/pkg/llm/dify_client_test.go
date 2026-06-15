package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDifyChatEndpoint(t *testing.T) {
	got, err := difyChatEndpoint("https://uat.cherygpt.com/v1")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://uat.cherygpt.com/v1/chat-messages"
	if got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}

func TestMessagesToDifyQuery(t *testing.T) {
	q, extra := messagesToDifyQuery([]Message{
		{Role: "system", Content: "你是 SRE"},
		{Role: "user", Content: "第一轮"},
		{Role: "assistant", Content: "收到"},
		{Role: "user", Content: "第二轮"},
	})
	if q != "第二轮" {
		t.Fatalf("query = %q", q)
	}
	if !strings.Contains(extra, "你是 SRE") || !strings.Contains(extra, "第一轮") {
		t.Fatalf("extra = %q", extra)
	}
}

func TestDifyClient_Chat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat-messages" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer app-test") {
			t.Fatalf("auth = %q", r.Header.Get("Authorization"))
		}
		var body difyChatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Query != "你好" {
			t.Fatalf("query = %q", body.Query)
		}
		if body.Inputs["online"] != "1" {
			t.Fatalf("inputs = %#v", body.Inputs)
		}
		_, _ = w.Write([]byte(`{"answer":"你好！","metadata":{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}}`))
	}))
	defer srv.Close()

	c := NewDifyClient(DifyConfig{
		APIKey:  "app-test",
		BaseURL: srv.URL + "/v1",
		User:    "tester",
	}, nil, nil)

	resp, err := c.Chat(context.Background(), ChatReq{
		Messages: []Message{{Role: "user", Content: "你好"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Assistant.Content != "你好！" {
		t.Fatalf("answer = %q", resp.Assistant.Content)
	}
	if resp.Usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}

func TestDifyClient_Chat_SystemContextInInputsContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body difyChatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Query != "第二轮" {
			t.Fatalf("query = %q", body.Query)
		}
		if !strings.Contains(body.Inputs["content"], "你是 SRE") || !strings.Contains(body.Inputs["content"], "第一轮") {
			t.Fatalf("inputs.content should carry system/history: %q", body.Inputs["content"])
		}
		_, _ = w.Write([]byte(`{"answer":"ok","metadata":{"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}}`))
	}))
	defer srv.Close()

	c := NewDifyClient(DifyConfig{
		APIKey:  "app-test",
		BaseURL: srv.URL + "/v1",
	}, nil, nil)

	_, err := c.Chat(context.Background(), ChatReq{
		Messages: []Message{
			{Role: "system", Content: "你是 SRE"},
			{Role: "user", Content: "第一轮"},
			{Role: "assistant", Content: "收到"},
			{Role: "user", Content: "第二轮"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDifyClient_APIErrorJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthorized","message":"bad key"}`))
	}))
	defer srv.Close()

	c := NewDifyClient(DifyConfig{
		APIKey:  "bad",
		BaseURL: srv.URL + "/v1",
	}, nil, nil)

	_, err := c.Chat(context.Background(), ChatReq{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}
