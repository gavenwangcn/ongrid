// generate.go — natural-language → flow graph. Turns a one-line description
// into a runnable workflow using the live tool catalog, so users don't have to
// hand-place nodes.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ongridio/ongrid/internal/pkg/errs"
)

// GenLLM is the one-shot completion seam used for generation (reuses the same
// runner the LLM node uses).
type GenLLM interface {
	RunLLM(ctx context.Context, system, user string) (string, error)
}

// jsonGenLLM is optional — when implemented, GenerateGraph requests
// json_object output from providers that support it.
type jsonGenLLM interface {
	RunLLMJSON(ctx context.Context, system, user string) (string, error)
}

// WithLLM wires the generation LLM. Returns the usecase for chaining.
func (u *Usecase) WithLLM(l GenLLM) *Usecase { u.llm = l; return u }

// GenerateGraph asks the model to turn `prompt` into a flow graph, using the
// live tool catalog so it only references tools that exist. Returns a
// CreateInput ready for Create. Best-effort: the graph is validated before
// returning.
func (u *Usecase) GenerateGraph(ctx context.Context, prompt string) (CreateInput, error) {
	start := time.Now()
	log := u.log.With(slog.String("comp", "flow-generate"))

	if u.llm == nil {
		return CreateInput{}, fmt.Errorf("%w: workflow generation not wired", errs.ErrNotWiredYet)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return CreateInput{}, fmt.Errorf("%w: prompt required", errs.ErrInvalid)
	}
	tools := u.ListTools()
	system := genSystemPrompt(tools)
	_, jsonMode := u.llm.(jsonGenLLM)

	log.Info("flow generate start",
		slog.Int("prompt_bytes", len(prompt)),
		slog.String("prompt_preview", truncateOut(prompt, 200)),
		slog.Int("tool_count", len(tools)),
		slog.Bool("json_mode", jsonMode),
		slog.Int("system_prompt_bytes", len(system)),
	)

	out, err := u.callGraphLLM(ctx, log, system, prompt, "initial", jsonMode)
	if err != nil {
		log.Warn("flow generate llm failed",
			slog.String("attempt", "initial"),
			slog.Duration("total_duration", time.Since(start)),
			slog.Any("err", err),
		)
		return CreateInput{}, err
	}
	in, parseDiag := parseGeneratedGraphDiag(out)
	retried := false
	if parseDiag.err != nil {
		log.Warn("flow generate parse failed, retrying",
			slog.String("attempt", "initial"),
			slog.String("stage", parseDiag.stage),
			slog.Int("output_bytes", len(out)),
			slog.Int("extracted_bytes", len(parseDiag.extracted)),
			slog.String("output_preview", truncateOut(out, 600)),
			slog.String("extracted_preview", truncateOut(parseDiag.extracted, 800)),
			slog.String("output_first_rune", firstRuneLabel(out)),
			slog.Bool("output_starts_with_brace", strings.HasPrefix(strings.TrimSpace(out), "{")),
			slog.Int("brace_index", strings.IndexByte(out, '{')),
			slog.Any("err", parseDiag.err),
		)
		retryPrompt := prompt + "\n\n你上次返回的不是合法 JSON。请只输出一个 JSON 对象，包含 name、description、graph 三个字段，不要任何解释或 markdown。"
		retried = true
		out, err = u.callGraphLLM(ctx, log, system, retryPrompt, "retry", jsonMode)
		if err != nil {
			log.Warn("flow generate llm failed",
				slog.String("attempt", "retry"),
				slog.Duration("total_duration", time.Since(start)),
				slog.Any("err", err),
			)
			return CreateInput{}, err
		}
		in, parseDiag = parseGeneratedGraphDiag(out)
		if parseDiag.err != nil {
			log.Warn("flow generate parse failed after retry",
				slog.String("attempt", "retry"),
				slog.String("stage", parseDiag.stage),
				slog.Int("output_bytes", len(out)),
				slog.Int("extracted_bytes", len(parseDiag.extracted)),
				slog.String("output_preview", truncateOut(out, 600)),
				slog.String("extracted_preview", truncateOut(parseDiag.extracted, 800)),
				slog.String("output_first_rune", firstRuneLabel(out)),
				slog.Bool("output_starts_with_brace", strings.HasPrefix(strings.TrimSpace(out), "{")),
				slog.Int("brace_index", strings.IndexByte(out, '{')),
				slog.Duration("total_duration", time.Since(start)),
				slog.Any("err", parseDiag.err),
			)
			return CreateInput{}, fmt.Errorf("%w: model did not return valid JSON: %v", errs.ErrInvalid, parseDiag.err)
		}
	}
	nodes, edges := graphCounts(in.GraphJSON)
	log.Info("flow generate done",
		slog.String("name", in.Name),
		slog.Int("description_bytes", len(in.Description)),
		slog.Int("graph_bytes", len(in.GraphJSON)),
		slog.Int("node_count", nodes),
		slog.Int("edge_count", edges),
		slog.Bool("retried", retried),
		slog.Duration("total_duration", time.Since(start)),
	)
	return in, nil
}

func (u *Usecase) callGraphLLM(ctx context.Context, log *slog.Logger, system, user, attempt string, jsonMode bool) (string, error) {
	start := time.Now()
	log.Info("flow generate llm call start",
		slog.String("attempt", attempt),
		slog.Bool("json_mode", jsonMode),
		slog.Int("user_prompt_bytes", len(user)),
		slog.String("user_prompt_preview", truncateOut(user, 200)),
	)

	var out string
	var err error
	if jsonMode {
		out, err = u.llm.(jsonGenLLM).RunLLMJSON(ctx, system, user)
	} else {
		out, err = u.llm.RunLLM(ctx, system, user)
	}
	if err != nil {
		log.Warn("flow generate llm call error",
			slog.String("attempt", attempt),
			slog.Bool("json_mode", jsonMode),
			slog.Duration("duration", time.Since(start)),
			slog.Any("err", err),
		)
		return "", err
	}

	extracted := stripCodeFences(out)
	log.Info("flow generate llm call done",
		slog.String("attempt", attempt),
		slog.Bool("json_mode", jsonMode),
		slog.Duration("duration", time.Since(start)),
		slog.Int("output_bytes", len(out)),
		slog.Int("extracted_bytes", len(extracted)),
		slog.String("output_preview", truncateOut(out, 600)),
		slog.String("extracted_preview", truncateOut(extracted, 800)),
		slog.String("output_first_rune", firstRuneLabel(out)),
		slog.Bool("output_starts_with_brace", strings.HasPrefix(strings.TrimSpace(out), "{")),
		slog.Int("brace_index", strings.IndexByte(out, '{')),
	)
	return out, nil
}

type graphParseDiag struct {
	err       error
	stage     string
	extracted string
}

func parseGeneratedGraphDiag(out string) (CreateInput, graphParseDiag) {
	extracted := stripCodeFences(out)
	var gen struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Graph       json.RawMessage `json:"graph"`
	}
	if err := json.Unmarshal([]byte(extracted), &gen); err != nil {
		return CreateInput{}, graphParseDiag{err: err, stage: "json_unmarshal", extracted: extracted}
	}
	graph := strings.TrimSpace(string(gen.Graph))
	if graph == "" || graph == "null" {
		return CreateInput{}, graphParseDiag{err: fmt.Errorf("model returned no graph"), stage: "empty_graph", extracted: extracted}
	}
	if _, err := ParseGraph(graph); err != nil {
		return CreateInput{}, graphParseDiag{err: err, stage: "graph_validate", extracted: extracted}
	}
	name := strings.TrimSpace(gen.Name)
	if name == "" {
		name = "AI 生成的工作流"
	}
	return CreateInput{Name: name, Description: strings.TrimSpace(gen.Description), GraphJSON: graph},
		graphParseDiag{extracted: extracted}
}

func parseGeneratedGraph(out string) (CreateInput, error) {
	in, diag := parseGeneratedGraphDiag(out)
	if diag.err != nil {
		return CreateInput{}, diag.err
	}
	return in, nil
}

func graphCounts(graphJSON string) (nodes, edges int) {
	var g struct {
		Nodes []json.RawMessage `json:"nodes"`
		Edges []json.RawMessage `json:"edges"`
	}
	if err := json.Unmarshal([]byte(graphJSON), &g); err != nil {
		return 0, 0
	}
	return len(g.Nodes), len(g.Edges)
}

func firstRuneLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "<empty>"
	}
	r, _ := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return "<invalid>"
	}
	return fmt.Sprintf("U+%04X %q", r, r)
}

// stripCodeFences removes markdown fences and trims to the outermost JSON
// object. Models often prepend Chinese prose or wrap JSON in ``` fences.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		}
		if j := strings.Index(rest, "```"); j >= 0 {
			rest = rest[:j]
		}
		s = strings.TrimSpace(rest)
	}
	if i := strings.IndexByte(s, '{'); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndexByte(s, '}'); j >= 0 && j+1 < len(s) {
		s = s[:j+1]
	}
	return strings.TrimSpace(s)
}

func truncateOut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func genSystemPrompt(tools []ToolMeta) string {
	var b strings.Builder
	b.WriteString(`你是 ongrid 工作流生成器。把用户的自然语言需求转成一个可运行的工作流图。

只输出一个 JSON 对象，不要任何解释、不要 markdown 代码围栏：
{"name":"<简短工作流名>","description":"<一句话说明>","graph":{"nodes":[...],"edges":[...]}}

## 图规则
- nodes: [{"id":"<短id>","type":"<节点类型>","name":"<简短中文名>","config":{...}}]，edges: [{"id":"<短id>","source":"<id>","target":"<id>","sourcePort":"<可选>"}]
- 每个节点都要给一个简短、能一眼看懂的 name（如「拉取设备摘要」「分析风险」「生成HTML」「托管网页」），它会显示在画布和运行记录里——不要省略，也不要只用单字母 id 当名字。
- 必须有且仅有一个触发器节点做起点（默认 trigger.manual）。
- 节点间用边连，下游用 {{nodes.<上游id>.output.<字段>}} 引用上游输出（写在 config 的字符串值里）。

## 节点类型与 config
- trigger.manual: 手动触发，config {}
- trigger.cron: 定时，config {"schedule":"0 9 * * *"}
- trigger.alert_fired: 告警触发，config {"rule":"<规则名包含,可空>"}；可引用 {{trigger.incident_id}}
- tool: 调工具，config {"tool":"<工具名>","args":{...}}；输出 {{nodes.<id>.output.result}}
- llm: 一次 LLM，config {"system":"...","prompt":"...支持{{}}"}；输出 {{nodes.<id>.output.answer}}。要结构化加 "output_schema":<JSONSchema>，则可引 output.structured.<字段>
- agent: 自主 agent，config {"persona":"default","instruction":"...支持{{}}"}；输出 output.answer
- condition: 分支，config {"expr":"{{nodes.x.output.structured.severity}} == \"critical\""}；两个出口端口 true/false，对应边写 "sourcePort":"true" 或 "false"
- notify: 发通知，config {"channel_ids":[1],"title":"...","message":"...支持{{}}"}
- http_request: HTTP，config {"method":"GET","url":"...","headers":{},"body":""}；输出 output.status / output.body
- transform: 字段映射，config {"fields":{"<新名>":"{{...}}"}}
- set: 变量，config {"name":"...","value":"{{...}}"}

## 报告网页范式（用户要"生成报告/网页/可视化"时）
最后用 serve_page：先用 llm 节点生成完整 HTML（system 写明"只输出 <!DOCTYPE html> 开头的 HTML、不要解释不要代码围栏"），再 tool serve_page，args {"html":"{{nodes.<llm_id>.output.answer}}","title":"..."}。

## 可用工具（tool 节点的 tool 名 + 必填参数；只能用这里的名字）
`)
	for _, t := range tools {
		desc := t.DescriptionZh
		if desc == "" {
			desc = t.Description
		}
		req := requiredParams(t.Parameters)
		line := "- " + t.Name
		if desc != "" {
			line += "：" + oneLine(desc)
		}
		if len(req) > 0 {
			line += "（必填: " + strings.Join(req, ", ") + "）"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(`
## 示例（用户："巡检设备1的负载，生成网页报告"）
{"name":"设备负载巡检报告","description":"取负载 → AI 生成 HTML → 托管网页","graph":{"nodes":[{"id":"t","type":"trigger.manual","name":"手动触发","config":{}},{"id":"a","type":"tool","name":"拉取设备负载","config":{"tool":"get_host_load","args":{"device_ids":[1]}}},{"id":"b","type":"llm","name":"生成报告HTML","config":{"system":"你是网页生成器，只输出完整HTML(<!DOCTYPE html>开头)，不要解释不要代码围栏","prompt":"根据负载数据生成一个报告网页：{{nodes.a.output.result}}"}},{"id":"c","type":"tool","name":"托管网页","config":{"tool":"serve_page","args":{"html":"{{nodes.b.output.answer}}","title":"设备负载报告"}}}],"edges":[{"id":"1","source":"t","target":"a"},{"id":"2","source":"a","target":"b"},{"id":"3","source":"b","target":"c"}]}}

只输出 JSON。工具名必须用上面列出的，参数符合其 schema。`)
	return b.String()
}

func requiredParams(schema json.RawMessage) []string {
	if len(schema) == 0 {
		return nil
	}
	var s struct {
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(schema, &s)
	return s.Required
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 120 {
		// trim on a rune boundary
		r := []rune(s)
		if len(r) > 80 {
			r = r[:80]
		}
		s = string(r) + "…"
	}
	return strings.TrimSpace(s)
}
