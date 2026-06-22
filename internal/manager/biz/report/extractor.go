package report

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ongridio/ongrid/internal/pkg/llm"
)

// ContentLLM is the narrow seam for the structured extraction pass.
type ContentLLM interface {
	Chat(ctx context.Context, req llm.ChatReq) (*llm.ChatResp, error)
}

// ExtractContentReq inputs the Pass-2 extractor.
type ExtractContentReq struct {
	RawOutput  string
	Locale     string
	Provider   string
	Model      string
	ParseError string
}

// ContentExtractor runs a temperature-0 LLM pass to coerce arbitrary worker
// output into validated ContentJSON. Mirrors alert investigator extractStructured.
type ContentExtractor struct {
	llm     ContentLLM
	timeout time.Duration
	log     *slog.Logger
}

// NewContentExtractor builds an extractor. llm nil → Extract returns error.
func NewContentExtractor(client ContentLLM, log *slog.Logger) *ContentExtractor {
	if log == nil {
		log = slog.Default()
	}
	return &ContentExtractor{
		llm:     client,
		timeout: 60 * time.Second,
		log:     log.With(slog.String("comp", "report-extractor")),
	}
}

// WithTimeout overrides the per-call deadline (0 → 60s default).
func (e *ContentExtractor) WithTimeout(d time.Duration) *ContentExtractor {
	if d > 0 {
		e.timeout = d
	}
	return e
}

// Extract coerces raw worker output into ContentJSON via a dedicated LLM call.
func (e *ContentExtractor) Extract(ctx context.Context, req ExtractContentReq) (*Content, error) {
	if e == nil || e.llm == nil {
		return nil, fmt.Errorf("report: content extractor not configured")
	}
	raw := strings.TrimSpace(req.RawOutput)
	if raw == "" {
		return nil, fmt.Errorf("report: content extractor: empty input")
	}

	prompt := buildExtractorPrompt(raw, req.ParseError, req.Locale)
	cctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	chatReq := llm.ChatReq{
		Model:          req.Model,
		Provider:       req.Provider,
		Sampling:       llm.SamplingParams{Temperature: llm.PtrFloat32(0)},
		ResponseFormat: llm.JSONSchemaFormat("report_content", llmOutputJSONSchema),
		Messages: []llm.Message{
			{Role: "system", Content: contentExtractorSystemPrompt},
			{Role: "user", Content: prompt},
		},
	}

	e.log.Info("report content extract start",
		slog.String("provider", req.Provider),
		slog.String("model", req.Model),
		slog.Int("raw_bytes", len(raw)),
	)

	resp, err := e.llm.Chat(cctx, chatReq)
	if err != nil && isResponseFormatUnsupported(err) {
		e.log.Info("report content extract: json_schema unsupported, retry json_object",
			slog.Any("err", err),
		)
		chatReq.ResponseFormat = llm.JSONObjectFormat()
		resp, err = e.llm.Chat(cctx, chatReq)
	}
	if err != nil {
		return nil, fmt.Errorf("report: content extractor LLM: %w", err)
	}

	out := strings.TrimSpace(resp.Assistant.Content)
	jsonBlob := extractJSON(out)
	if jsonBlob == "" {
		return nil, fmt.Errorf("report: content extractor: no JSON in response")
	}

	content, err := ParseContent(jsonBlob, e.log)
	if err != nil {
		return nil, fmt.Errorf("report: content extractor parse: %w", err)
	}
	e.log.Info("report content extract ok",
		slog.String("headline", truncate(content.Narrative.Headline, 120)),
		slog.Int("paragraphs", len(content.Narrative.Paragraphs)),
		slog.Int("advice", len(content.Advice)),
	)
	return content, nil
}

func buildExtractorPrompt(rawOutput, parseError, locale string) string {
	var b strings.Builder
	b.WriteString("# Required output schema\n\n")
	b.WriteString(RequiredLLMOutputSchema())
	b.WriteString("\n\n# Draft report output (may be wrong shape)\n\n")
	b.WriteString(rawOutput)
	if parseError != "" {
		b.WriteString("\n\n# Parser error from the draft\n\n")
		b.WriteString(parseError)
	}
	b.WriteString("\n\n# Task\n\nRewrite the draft into the required schema. Output JSON only.\n")
	if d := localeDirective(locale); d != "" {
		b.WriteString("\n")
		b.WriteString(d)
		b.WriteString("\n")
	}
	return b.String()
}

func buildSchemaCorrectionPrompt(factsPrompt, prevOutput, parseError, locale string) string {
	var b strings.Builder
	b.WriteString("Your previous report output did not match the required ContentJSON schema.\n\n")
	if parseError != "" {
		b.WriteString("Validation error:\n")
		b.WriteString(parseError)
		b.WriteString("\n\n")
	}
	b.WriteString("Required schema (output ONLY this JSON shape, nothing else):\n\n")
	b.WriteString(RequiredLLMOutputSchema())
	b.WriteString("\n\nYour previous output (wrong shape — rewrite, do not append):\n\n")
	b.WriteString(truncate(prevOutput, 4000))
	b.WriteString("\n\n")
	b.WriteString(factsPrompt)
	if d := localeDirective(locale); d != "" {
		b.WriteString("\n")
		b.WriteString(d)
		b.WriteString("\n")
	}
	b.WriteString("\n\nOutput the corrected ContentJSON only.\n")
	return b.String()
}

// isResponseFormatUnsupported heuristically detects providers that reject
// json_schema response_format so we can fall back to json_object.
func isResponseFormatUnsupported(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"response_format",
		"json_schema",
		"unsupported",
		"not supported",
		"invalid request",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
