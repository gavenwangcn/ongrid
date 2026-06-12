package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ErrDifyNotConfigured is returned when API key or base URL is missing.
var ErrDifyNotConfigured = errors.New("llm: dify API key or base URL not configured")

// DifyConfig holds credentials and App input defaults for the Dify
// Service API (POST /v1/chat-messages, blocking mode).
type DifyConfig struct {
	APIKey        string
	BaseURL       string
	User          string
	InputsContent string
	InputsOnline  string
	// Model is a display label for metrics / the SPA catalog; Dify apps
	// do not take an OpenAI-style model slug on chat-messages.
	Model   string
	Timeout time.Duration
}

// Configured reports whether the Dify backend should take over LLM routing.
func (c DifyConfig) Configured() bool {
	return strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.BaseURL) != ""
}

// NewDifyClient builds a Client that speaks the Dify Service API.
func NewDifyClient(cfg DifyConfig, budget BudgetChecker, reg *prometheus.Registry) Client {
	log := slog.Default().With(slog.String("component", "llm-dify"))
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if strings.TrimSpace(cfg.User) == "" {
		cfg.User = "ongrid"
	}
	if strings.TrimSpace(cfg.InputsContent) == "" {
		cfg.InputsContent = "输入数据源"
	}
	if strings.TrimSpace(cfg.InputsOnline) == "" {
		cfg.InputsOnline = "1"
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "dify-app"
	}
	return &difyClient{
		cfg:     cfg,
		budget:  budget,
		metrics: newMetrics(reg, log),
		log:     log,
		http:    &http.Client{Timeout: cfg.Timeout},
	}
}

type difyClient struct {
	cfg     DifyConfig
	budget  BudgetChecker
	metrics *metrics
	log     *slog.Logger
	http    *http.Client
}

type difyChatRequest struct {
	Inputs         map[string]string `json:"inputs"`
	Query          string            `json:"query"`
	ResponseMode   string            `json:"response_mode"`
	ConversationID string            `json:"conversation_id"`
	User           string            `json:"user"`
	Files          []any             `json:"files"`
}

type difyChatResponse struct {
	Answer   string `json:"answer"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Metadata struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	} `json:"metadata"`
}

// Chat implements Client via Dify blocking chat-messages.
func (c *difyClient) Chat(ctx context.Context, req ChatReq) (*ChatResp, error) {
	if !c.cfg.Configured() {
		return nil, ErrDifyNotConfigured
	}
	model := firstNonEmpty(req.Model, c.cfg.Model)

	if c.budget != nil {
		if err := c.budget.Check(ctx, req.UserID, estimatePromptTokens(req.Messages)); err != nil {
			c.metrics.requestsTotal.WithLabelValues(model, "budget_exceeded").Inc()
			return nil, err
		}
	}

	if len(req.Tools) > 0 {
		c.log.Debug("dify: omitting tool schemas — Service API has no OpenAI function-calling",
			slog.Int("tool_count", len(req.Tools)))
	}

	query, extraContent := messagesToDifyQuery(req.Messages)
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("llm: dify: no user query in messages")
	}

	inputs := map[string]string{
		"content": c.cfg.InputsContent,
		"online":  c.cfg.InputsOnline,
	}
	if extraContent != "" {
		inputs["content"] = strings.TrimSpace(c.cfg.InputsContent + "\n\n" + extraContent)
	}

	user := c.cfg.User
	if req.UserID > 0 {
		user = strconv.FormatUint(req.UserID, 10)
	}

	body := difyChatRequest{
		Inputs:       inputs,
		Query:        query,
		ResponseMode: "blocking",
		User:         user,
		Files:        []any{},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm: dify: marshal request: %w", err)
	}

	endpoint, err := difyChatEndpoint(c.cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	callCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, c.cfg.Timeout)
		defer cancel()
	}

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("llm: dify: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.APIKey))

	start := time.Now()
	httpResp, err := c.http.Do(httpReq)
	dur := time.Since(start)
	c.metrics.requestSeconds.WithLabelValues(model).Observe(dur.Seconds())
	if err != nil {
		c.metrics.requestsTotal.WithLabelValues(model, "error").Inc()
		return nil, fmt.Errorf("llm: dify: http: %w", err)
	}
	defer httpResp.Body.Close()

	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		c.metrics.requestsTotal.WithLabelValues(model, "error").Inc()
		return nil, fmt.Errorf("llm: dify: read body: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		c.metrics.requestsTotal.WithLabelValues(model, "error").Inc()
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 240 {
			snippet = snippet[:240] + "..."
		}
		return nil, fmt.Errorf("llm: dify: http %d: %s", httpResp.StatusCode, snippet)
	}

	var parsed difyChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		c.metrics.requestsTotal.WithLabelValues(model, "error").Inc()
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 120 {
			snippet = snippet[:120] + "..."
		}
		return nil, fmt.Errorf("llm: dify: decode response: %w (body=%q)", err, snippet)
	}
	if parsed.Code != "" && parsed.Code != "success" {
		c.metrics.requestsTotal.WithLabelValues(model, "error").Inc()
		msg := firstNonEmpty(parsed.Message, parsed.Code)
		return nil, fmt.Errorf("llm: dify: api error: %s", msg)
	}
	if strings.TrimSpace(parsed.Answer) == "" {
		c.metrics.requestsTotal.WithLabelValues(model, "error").Inc()
		return nil, fmt.Errorf("llm: dify: empty answer")
	}

	usage := Usage{
		PromptTokens:     parsed.Metadata.Usage.PromptTokens,
		CompletionTokens: parsed.Metadata.Usage.CompletionTokens,
		TotalTokens:      parsed.Metadata.Usage.TotalTokens,
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	c.metrics.tokensTotal.WithLabelValues(model, "prompt").Add(float64(usage.PromptTokens))
	c.metrics.tokensTotal.WithLabelValues(model, "completion").Add(float64(usage.CompletionTokens))
	c.metrics.requestsTotal.WithLabelValues(model, "success").Inc()

	if c.budget != nil {
		if rerr := c.budget.Record(ctx, req.UserID, usage); rerr != nil {
			c.log.Warn("dify budget record failed", slog.Any("err", rerr))
		}
	}

	c.log.Info("dify chat completion",
		slog.String("model", model),
		slog.Uint64("user_id", req.UserID),
		slog.Int("prompt_tokens", usage.PromptTokens),
		slog.Int("completion_tokens", usage.CompletionTokens),
		slog.Duration("duration", dur),
	)

	return &ChatResp{
		Assistant: Message{Role: "assistant", Content: parsed.Answer},
		Usage:     usage,
	}, nil
}

func difyChatEndpoint(baseURL string) (string, error) {
	s := strings.TrimSpace(baseURL)
	if s == "" {
		return "", fmt.Errorf("llm: dify: base URL empty")
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("llm: dify: invalid base URL %q", baseURL)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/chat-messages"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// messagesToDifyQuery maps OpenAI-shaped history to Dify's query + extra
// context. The last user turn becomes query; earlier system notes are
// folded into inputs.content via the returned extraContent string.
func messagesToDifyQuery(msgs []Message) (query string, extraContent string) {
	var systemParts []string
	var history strings.Builder
	lastUser := ""

	for _, m := range msgs {
		switch m.Role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				systemParts = append(systemParts, m.Content)
			}
		case "user":
			if lastUser != "" {
				history.WriteString("User: ")
				history.WriteString(lastUser)
				history.WriteString("\n")
			}
			lastUser = m.Content
		case "assistant":
			if strings.TrimSpace(m.Content) != "" {
				history.WriteString("Assistant: ")
				history.WriteString(m.Content)
				history.WriteString("\n")
			}
		case "tool":
			if strings.TrimSpace(m.Content) != "" {
				history.WriteString("Tool ")
				history.WriteString(m.ToolName)
				history.WriteString(": ")
				history.WriteString(m.Content)
				history.WriteString("\n")
			}
		}
	}

	query = lastUser
	if history.Len() > 0 {
		extraContent = strings.TrimSpace(history.String())
	}
	if len(systemParts) > 0 {
		sys := strings.Join(systemParts, "\n\n")
		if extraContent == "" {
			extraContent = sys
		} else {
			extraContent = sys + "\n\n" + extraContent
		}
	}
	return query, extraContent
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
