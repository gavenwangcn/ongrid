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

func TestExtractWAFMetadata(t *testing.T) {
	body := `<!DOCTYPE html><html><head><meta http-equiv="Server" content="CloudWAF" />
<title>访问被拦截！</title></head><body>
<script>var eventId = "evt-abc123def456";</script>
<p>事件ID：WAF-20260615-001</p></body></html>`
	hdr := http.Header{}
	hdr.Set("Server", "CloudWAF")
	hdr.Set("X-Request-Id", "req-trace-99")

	meta := extractWAFMetadata(body, hdr)
	if meta["event_id"] != "evt-abc123def456" {
		t.Fatalf("event_id = %q, want evt-abc123def456", meta["event_id"])
	}
	if meta["waf_vendor"] != "CloudWAF" {
		t.Fatalf("waf_vendor = %q", meta["waf_vendor"])
	}
	if meta["header_server"] != "CloudWAF" {
		t.Fatalf("header_server = %q", meta["header_server"])
	}
	if meta["header_x-request-id"] != "req-trace-99" {
		t.Fatalf("header_x-request-id = %q", meta["header_x-request-id"])
	}
}

func TestDifyClient_WAF418_EventIDInError(t *testing.T) {
	wafBody := `<!DOCTYPE html><html><head><meta http-equiv="Server" content="CloudWAF" />
<title>访问被拦截！</title></head><body onload="block()">
<input type="hidden" name="eventId" value="waf-event-7f3a2b1c"/></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "CloudWAF")
		w.WriteHeader(http.StatusTeapot) // 418
		_, _ = w.Write([]byte(wafBody))
	}))
	defer srv.Close()

	c := NewDifyClient(DifyConfig{
		APIKey:  "app-test",
		BaseURL: srv.URL + "/v1",
	}, nil, nil)

	_, err := c.Chat(context.Background(), ChatReq{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "418") {
		t.Fatalf("err should mention 418: %v", err)
	}
	if !strings.Contains(err.Error(), "waf_event_id=waf-event-7f3a2b1c") {
		t.Fatalf("err should include waf_event_id: %v", err)
	}
}
