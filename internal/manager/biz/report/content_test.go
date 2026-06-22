package report

import (
	"strings"
	"testing"
)

func sampleContent() *Content {
	d := -12.0
	return &Content{
		Version: ContentVersion,
		Hero: []HeroStat{
			{Key: "incidents", Label: "Incidents", Value: 23, DeltaPct: &d, Sparkline: []int{4, 7, 3}},
			{Key: "mttr_minutes", Label: "MTTR", Value: 47, Unit: "min"},
		},
		Narrative: Narrative{
			Headline: "本周整体平稳",
			Paragraphs: []Paragraph{
				{Text: "{{entity:edge:7|db-prod-3}} 三次突破 30%，触发 {{entity:incident:1234|I-1234}}。"},
			},
		},
		KeyIncidents: []KeyIncident{
			{ID: 1234, Title: "db-prod-3 IO 饱和", Severity: "warning", DurationMin: 47, Status: "resolved"},
		},
		Actions: ActionsSummary{MutatingTotal: 11, MutatingApproved: 11, SafeTotal: 47},
		Advice:  []Advice{{Text: "把 {{entity:edge:7|db-prod-3}} backup 挪到 03:00"}},
	}
}

func TestParseContent_RoundTrip(t *testing.T) {
	raw := sampleContent().MustJSON()
	got, err := ParseContent(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Narrative.Headline != "本周整体平稳" {
		t.Errorf("headline lost: %q", got.Narrative.Headline)
	}
	if len(got.Hero) != 2 || got.Hero[0].DeltaPct == nil || *got.Hero[0].DeltaPct != -12 {
		t.Errorf("hero delta lost: %+v", got.Hero)
	}
}

func TestValidate_RejectsMissingHeadline(t *testing.T) {
	c := sampleContent()
	c.Narrative.Headline = "  "
	if err := c.Validate(); err == nil {
		t.Error("expected error for blank headline")
	}
}

func TestValidate_RejectsHeroMissingKey(t *testing.T) {
	c := sampleContent()
	c.Hero[1].Key = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error for hero without key")
	}
}

func TestValidate_AllowsCalmReport(t *testing.T) {
	// A 0-incident calm report: empty incidents/advice is fine, spine intact.
	c := &Content{
		Version:   ContentVersion,
		Hero:      []HeroStat{{Key: "incidents", Label: "Incidents", Value: 0}},
		Narrative: Narrative{Headline: "本周无异常，一切平稳"},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("calm report should validate: %v", err)
	}
}

func TestRenderMarkdown_FlattensEntities(t *testing.T) {
	md := sampleContent().RenderMarkdown("周报 · 2026 W23", "zh")
	if strings.Contains(md, "{{entity:") {
		t.Errorf("entity tokens not flattened:\n%s", md)
	}
	if !strings.Contains(md, "db-prod-3") {
		t.Errorf("entity display name lost:\n%s", md)
	}
	if !strings.Contains(md, "# 周报 · 2026 W23") {
		t.Errorf("title missing:\n%s", md)
	}
	// zh locale → Chinese section titles.
	if !strings.Contains(md, "## 监控覆盖") {
		t.Errorf("zh section title missing:\n%s", md)
	}
}

// TestRenderMarkdown_Locale verifies the markdown section titles follow
// the report locale (feedback_ai_output_locale) — en yields English
// headings, never the Chinese defaults.
func TestRenderMarkdown_Locale(t *testing.T) {
	en := sampleContent().RenderMarkdown("Weekly · 2026 W23", "en")
	for _, want := range []string{"## Monitoring coverage"} {
		if !strings.Contains(en, want) {
			t.Errorf("en markdown missing %q:\n%s", want, en)
		}
	}
	for _, banned := range []string{"## 监控覆盖", "## 使用情况", "## 知识资产新增", "## New assets", "## Usage", "## 资源使用"} {
		if strings.Contains(en, banned) {
			t.Errorf("en markdown leaked Chinese heading %q", banned)
		}
	}
}

func TestParseContent_LegacyFlatShape(t *testing.T) {
	// Mirrors production failure: LLM returned flat headline + sections instead
	// of narrative.{headline,paragraphs}.
	raw := `{
		"headline": "电子招标管理平台-EBD 本周期运行整体平稳",
		"period": "2026-06-21 — 2026-06-22",
		"granularity": "daily",
		"scope": "系统「电子招标管理平台-EBD」",
		"hero": [{"key":"devices","label":"监控设备","value":5}],
		"sections": [
			{"title":"总体概况","body":"本周期内设备共 5 台，全部在线。"},
			{"title":"日志","body":"潜在错误 60 条。"}
		],
		"advice": ["优先排查 uip 500","核查 Kong Error"]
	}`
	got, err := ParseContent(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Narrative.Headline != "电子招标管理平台-EBD 本周期运行整体平稳" {
		t.Errorf("headline: %q", got.Narrative.Headline)
	}
	if len(got.Narrative.Paragraphs) != 2 {
		t.Fatalf("paragraphs: %+v", got.Narrative.Paragraphs)
	}
	if !strings.Contains(got.Narrative.Paragraphs[0].Text, "总体概况") {
		t.Errorf("first paragraph: %q", got.Narrative.Paragraphs[0].Text)
	}
	if len(got.Advice) != 2 || got.Advice[0].Text != "优先排查 uip 500" {
		t.Errorf("advice: %+v", got.Advice)
	}
}

func TestParseContent_LegacyFlatShapePreservesCanonicalNarrative(t *testing.T) {
	raw := `{"version":"1","hero":[{"key":"incidents","label":"Incidents","value":0}],
		"narrative":{"headline":"canonical"},
		"headline":"ignored flat headline"}`
	got, err := ParseContent(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Narrative.Headline != "canonical" {
		t.Errorf("canonical narrative should win: %q", got.Narrative.Headline)
	}
}

func TestParseContent_LegacyTitleOnlySectionsUsesSummary(t *testing.T) {
	// gpt-5.5 shape: sections are outline titles; prose lives in summary[].
	raw := `{
		"headline": "人力资源-EHR系统本周期整体运行平稳，主要风险集中在日志错误。",
		"sections": [
			{"title":"资源概况"},
			{"title":"设备与在线状态"},
			{"title":"日志风险"}
		],
		"summary": [
			"本周期 CPU 均值 3.1%，内存均值 68.4%，资源负载较低。",
			"监控设备 1 台全部在线。",
			"潜在错误 848 条，较上周期 +0.4%，主要来自 RabbitMQ exporter。"
		],
		"hero": [{"key":"devices","label":"监控设备","value":1}]
	}`
	got, err := ParseContent(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Narrative.Paragraphs) != 3 {
		t.Fatalf("paragraphs = %d, want 3 from summary: %+v", len(got.Narrative.Paragraphs), got.Narrative.Paragraphs)
	}
	if !strings.Contains(got.Narrative.Paragraphs[0].Text, "CPU 均值 3.1%") {
		t.Errorf("first paragraph should come from summary, got %q", got.Narrative.Paragraphs[0].Text)
	}
	for _, p := range got.Narrative.Paragraphs {
		if p.Text == "资源概况" || p.Text == "设备与在线状态" {
			t.Errorf("title-only section leaked as paragraph: %q", p.Text)
		}
	}
}

func TestParseContent_LegacyNarrativeHeadlineWithSummary(t *testing.T) {
	raw := `{
		"version":"1",
		"hero":[{"key":"devices","label":"监控设备","value":1}],
		"narrative":{"headline":"headline in narrative"},
		"summary": ["段落一：资源平稳。", "段落二：日志需关注。"]
	}`
	got, err := ParseContent(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Narrative.Headline != "headline in narrative" {
		t.Errorf("headline: %q", got.Narrative.Headline)
	}
	if len(got.Narrative.Paragraphs) != 2 {
		t.Fatalf("paragraphs: %+v", got.Narrative.Paragraphs)
	}
}

func TestParseContent_LegacySectionsWithContentField(t *testing.T) {
	raw := `{
		"headline": "本周平稳",
		"sections": [
			{"title":"资源概况","content":"CPU 均值 2.3%。"},
			{"title":"日志","content":"潜在错误 28 条。"}
		],
		"hero": [{"key":"devices","label":"监控设备","value":4}]
	}`
	got, err := ParseContent(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Narrative.Paragraphs) != 2 {
		t.Fatalf("paragraphs: %+v", got.Narrative.Paragraphs)
	}
	if !strings.Contains(got.Narrative.Paragraphs[0].Text, "CPU 均值 2.3%") {
		t.Errorf("content field not parsed: %q", got.Narrative.Paragraphs[0].Text)
	}
}

func TestParseContent_AdviceStringArray(t *testing.T) {
	raw := `{"version":"1","hero":[{"key":"incidents","label":"Incidents","value":0}],
		"narrative":{"headline":"本周平稳"},
		"advice":["关注磁盘容量","检查离线设备"]}`
	got, err := ParseContent(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Advice) != 2 || got.Advice[0].Text != "关注磁盘容量" || got.Advice[1].Text != "检查离线设备" {
		t.Errorf("advice string[] coerce: %+v", got.Advice)
	}
}

func TestParseContent_AdviceSingleString(t *testing.T) {
	raw := `{"version":"1","hero":[{"key":"incidents","label":"Incidents","value":0}],
		"narrative":{"headline":"本周平稳"},
		"advice":"单条建议"}`
	got, err := ParseContent(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Advice) != 1 || got.Advice[0].Text != "单条建议" {
		t.Errorf("advice string coerce: %+v", got.Advice)
	}
}

func TestParseContent_AdviceSingleObject(t *testing.T) {
	raw := `{"version":"1","hero":[{"key":"incidents","label":"Incidents","value":0}],
		"narrative":{"headline":"本周平稳"},
		"advice":{"text":"对象形式"}}`
	got, err := ParseContent(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Advice) != 1 || got.Advice[0].Text != "对象形式" {
		t.Errorf("advice object coerce: %+v", got.Advice)
	}
}

func TestFlattenEntities(t *testing.T) {
	cases := map[string]string{
		"plain text":                              "plain text",
		"{{entity:edge:7|db-prod-3}} down":        "db-prod-3 down",
		"a {{entity:incident:1|I-1}} b {{entity:edge:2|n2}} c": "a I-1 b n2 c",
		"{{entity:malformed":                      "{{entity:malformed", // no closer → left as-is
	}
	for in, want := range cases {
		if got := flattenEntities(in); got != want {
			t.Errorf("flatten(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseScope(t *testing.T) {
	s := ParseScope(`{"edge_ids":[7,9],"severity_min":"warning","system_name":"prod"}`)
	if len(s.EdgeIDs) != 2 || s.SeverityMin != "warning" || s.SystemName != "prod" {
		t.Errorf("scope parse: %+v", s)
	}
	// Empty / malformed → zero scope (full coverage), no panic.
	if got := ParseScope(""); len(got.EdgeIDs) != 0 {
		t.Errorf("empty scope should be zero: %+v", got)
	}
	if got := ParseScope("not json"); len(got.EdgeIDs) != 0 {
		t.Errorf("malformed scope should degrade to zero: %+v", got)
	}
}
