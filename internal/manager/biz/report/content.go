package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
)

// ContentJSON is the structured report body the reporter agent
// produces and the SPA renders (HLD-014 §ContentJSON). It is the source
// of truth for the rich in-app view; ContentMD is rendered from it as
// the export / IM / search fallback.
//
// Anti-hallucination contract: the numeric fields (Hero values/deltas/
// sparklines, the per-tool action counts) are computed in pure SQL by
// the FactsCollector and handed to the LLM. The generator (PR-2b)
// OVERWRITES Content.Hero and Content.Actions with the collected facts
// after the LLM returns, so a model that fiddles a number can't leak it
// into the report. The LLM owns only Narrative / KeyIncidents ordering
// commentary / Advice.
type Content struct {
	Version   string    `json:"version"`
	Hero      []HeroStat `json:"hero"`
	Narrative Narrative  `json:"narrative"`
	// Resource / Fleet / Changes are facts-injected (the generator
	// overwrites them from ReportFacts post-LLM); the agent never
	// produces them. Reuse the facts types — same JSON shape.
	Resource     ResourceFacts  `json:"resource"`
	Fleet        FleetFacts     `json:"fleet"`
	KeyIncidents []KeyIncident  `json:"key_incidents"`
	Actions      ActionsSummary `json:"actions_summary"`
	Changes      []ChangeFact   `json:"changes,omitempty"`
	// Legacy fields — no longer collected or rendered in new reports.
	Assets       AssetFacts     `json:"assets,omitempty"`
	Usage        UsageFacts     `json:"usage,omitempty"`
	Logs         LogFacts       `json:"logs"`
	Advice       []Advice       `json:"advice"`
	Metadata     ContentMeta    `json:"metadata"`
}

// HeroStat is one big-number card. Value/DeltaPct/Sparkline are SQL-
// computed (never LLM). Unit is optional ("min", "%"); empty = bare
// count. DeltaPct is the period-over-period change; nil = no prior
// period to compare (rendered as "new" rather than an arrow).
type HeroStat struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Value     float64  `json:"value"`
	Unit      string   `json:"unit,omitempty"`
	DeltaPct  *float64 `json:"delta_pct,omitempty"`
	Sparkline []int    `json:"sparkline,omitempty"`
}

// Narrative is the LLM's prose. Paragraph.Text may embed entity tokens
// `{{entity:kind:id|name}}` that the SPA renders as clickable chips;
// Entities lists them for the renderer's convenience (markdown export
// strips the token syntax to the display name).
type Narrative struct {
	Headline   string      `json:"headline"`
	Paragraphs []Paragraph `json:"paragraphs"`
}

type Paragraph struct {
	Text     string         `json:"text"`
	Entities []EntityRef     `json:"entities,omitempty"`
}

type EntityRef struct {
	Key  string `json:"key"`  // "edge:7" | "incident:1234"
	Name string `json:"name"` // display name
}

// KeyIncident is a compact incident reference for the report's incident
// list. Sourced from facts (ids, durations, status are SQL-true); the
// LLM may set RootCauseSnippet from the RCA report when one exists.
type KeyIncident struct {
	ID               uint64 `json:"id"`
	Title            string `json:"title"`
	Severity         string `json:"severity"`
	DurationMin      int    `json:"duration_min"`
	Status           string `json:"status"`
	RootCauseSnippet string `json:"root_cause_snippet,omitempty"`
}

// ActionsSummary is the agent-transparency panel. All counts SQL-true.
type ActionsSummary struct {
	MutatingTotal    int          `json:"mutating_total"`
	MutatingApproved int          `json:"mutating_approved"`
	SafeTotal        int          `json:"safe_total"`
	ByTool           []ToolCount  `json:"by_tool,omitempty"`
}

type ToolCount struct {
	Tool  string `json:"tool"`
	Count int    `json:"count"`
}

// Advice is one forward-looking recommendation. LLM-authored. Text may
// embed entity tokens like Narrative.
type Advice struct {
	Text string `json:"text"`
}

type ContentMeta struct {
	PeriodStart string   `json:"period_start"`
	PeriodEnd   string   `json:"period_end"`
	DataSources []string `json:"data_sources,omitempty"`
}

// ContentVersion is the schema version stamped into freshly generated
// reports; lets the SPA branch on shape if the schema evolves.
const ContentVersion = "1"

// ParseContent unmarshals a ContentJSON blob and validates it. Used by
// the generator (PR-2b) to check the LLM output before persisting.
// log is optional; when set, parse/normalize failures emit structured
// diagnostics (field snippets + JSON preview) for production triage.
func ParseContent(raw string, log *slog.Logger) (*Content, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if log != nil {
			log.Warn("parse content: empty input")
		}
		return nil, fmt.Errorf("report: content unmarshal: empty input")
	}
	if log != nil {
		log.Debug("parse content start", slog.Int("bytes", len(raw)))
	}

	normalized, shapeChanged, adviceChanged, err := normalizeContentJSON(raw, log)
	if err != nil {
		logContentParseFailure(log, raw, normalized, err, "normalize")
		return nil, fmt.Errorf("report: content normalize: %w", err)
	}

	var c Content
	if err := json.Unmarshal([]byte(normalized), &c); err != nil {
		logContentParseFailure(log, raw, normalized, err, "unmarshal")
		return nil, fmt.Errorf("report: content unmarshal: %w", err)
	}
	if err := c.Validate(); err != nil {
		if log != nil {
			log.Warn("parse content validate failed",
				slog.Any("err", err),
				slog.String("headline", c.Narrative.Headline),
				slog.Int("hero_count", len(c.Hero)),
				slog.Int("advice_count", len(c.Advice)),
				slog.String("json_preview", truncate(raw, 800)),
			)
		}
		return nil, err
	}
	if log != nil {
		log.Debug("parse content ok",
			slog.String("version", c.Version),
			slog.String("headline", truncate(c.Narrative.Headline, 120)),
			slog.Int("paragraphs", len(c.Narrative.Paragraphs)),
			slog.Int("advice", len(c.Advice)),
			slog.Bool("shape_normalized", shapeChanged),
			slog.Bool("advice_normalized", adviceChanged),
		)
	}
	return &c, nil
}

// normalizeContentJSON coerces common LLM shape drift (flat headline/
// sections, string advice, nested report_meta/*_overview) into the
// ContentJSON schema before unmarshal.
func normalizeContentJSON(raw string, log *slog.Logger) (string, bool, bool, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return raw, false, false, nil
	}
	altChanged, err := normalizeContentAlternateSchema(top, log)
	if err != nil {
		return raw, false, false, err
	}
	shapeChanged, err := normalizeContentShape(top, log)
	if err != nil {
		return raw, altChanged, false, err
	}
	adviceChanged, err := normalizeContentAdviceOnMap(top, log)
	if err != nil {
		return raw, shapeChanged || altChanged, false, err
	}
	heroChanged, err := normalizeContentHeroOnMap(top, log)
	if err != nil {
		return raw, shapeChanged || altChanged, adviceChanged, err
	}
	if !shapeChanged && !adviceChanged && !heroChanged && !altChanged {
		return raw, false, false, nil
	}
	b, err := json.Marshal(top)
	if err != nil {
		return raw, shapeChanged || altChanged, adviceChanged, err
	}
	return string(b), shapeChanged || altChanged, adviceChanged, nil
}

// nestedSectionOrder is the preferred merge order when Claude-style models
// emit one object per report theme (resource_overview, log_analysis, …).
var nestedSectionOrder = []string{
	"executive_summary",
	"resource_overview",
	"monitoring_coverage",
	"monitoring_status",
	"fleet_overview",
	"log_analysis",
	"logs_overview",
	"application_logs",
	"changes_overview",
	"alert_summary",
	"incidents_overview",
}

// normalizeContentAlternateSchema lifts report_meta / hero_metrics /
// *_overview nested objects into ContentJSON narrative + hero + advice.
func normalizeContentAlternateSchema(top map[string]json.RawMessage, log *slog.Logger) (bool, error) {
	changed := false

	if _, hasHero := top["hero"]; !hasHero {
		if hm, ok := top["hero_metrics"]; ok {
			top["hero"] = hm
			delete(top, "hero_metrics")
			changed = true
		}
	}
	if liftAdviceFromAlternateKeys(top) {
		changed = true
	}

	existingHeadline, existingParas := narrativeFields(top["narrative"])
	headline := existingHeadline
	if headline == "" {
		headline = jsonStringField(top["headline"])
	}
	if headline == "" {
		headline = headlineFromNestedSections(top)
	}
	if headline == "" {
		headline = jsonStringFieldFromObject(top["report_meta"], "title")
	}

	paras := existingParas
	if len(paras) == 0 {
		if nested := paragraphsFromNestedSections(top); len(nested) > 0 {
			paras = nested
		}
	}

	if headline == "" {
		return changed, nil
	}
	if existingHeadline != "" && len(existingParas) > 0 {
		return changed, nil
	}

	narrative, err := json.Marshal(map[string]any{
		"headline":   headline,
		"paragraphs": paras,
	})
	if err != nil {
		return changed, err
	}
	if log != nil {
		log.Info("parse content: normalized alternate report schema",
			slog.String("headline", truncate(headline, 120)),
			slog.Int("paragraphs", len(paras)),
			slog.String("hint", "LLM returned report_meta/hero_metrics/nested sections instead of ContentJSON"),
		)
	}
	top["narrative"] = narrative
	delete(top, "headline")
	changed = true
	return changed, nil
}

func liftAdviceFromAlternateKeys(top map[string]json.RawMessage) bool {
	if _, ok := top["advice"]; ok {
		return false
	}
	for _, key := range []string{"recommendations", "action_items"} {
		if raw, ok := top[key]; ok {
			top["advice"] = raw
			delete(top, key)
			return true
		}
	}
	if raw, ok := top["advice_section"]; ok {
		if items := jsonRawFieldFromObject(raw, "items"); items != nil {
			top["advice"] = items
			delete(top, "advice_section")
			return true
		}
		if recs := jsonRawFieldFromObject(raw, "recommendations"); recs != nil {
			top["advice"] = recs
			delete(top, "advice_section")
			return true
		}
	}
	return false
}

func headlineFromNestedSections(top map[string]json.RawMessage) string {
	for _, key := range nestedSectionOrder {
		if raw, ok := top[key]; ok {
			if h := jsonStringFieldFromObject(raw, "headline"); h != "" {
				return h
			}
			if h := jsonStringFieldFromObject(raw, "title"); h != "" {
				return h
			}
		}
	}
	for key, raw := range top {
		if isNestedSectionSkipKey(key) {
			continue
		}
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || raw[0] != '{' {
			continue
		}
		if h := jsonStringFieldFromObject(raw, "headline"); h != "" {
			return h
		}
	}
	return ""
}

func paragraphsFromNestedSections(top map[string]json.RawMessage) []Paragraph {
	seen := make(map[string]bool, len(nestedSectionOrder))
	var out []Paragraph
	appendFrom := func(key string, raw json.RawMessage) {
		if seen[key] {
			return
		}
		seen[key] = true
		if paras := paragraphsFromNestedSection(raw); len(paras) > 0 {
			out = append(out, paras...)
		}
	}
	for _, key := range nestedSectionOrder {
		if raw, ok := top[key]; ok {
			appendFrom(key, raw)
		}
	}
	for key, raw := range top {
		if isNestedSectionSkipKey(key) || seen[key] {
			continue
		}
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || raw[0] != '{' {
			continue
		}
		appendFrom(key, raw)
	}
	return out
}

type nestedSection struct {
	Headline  string          `json:"headline"`
	Title     string          `json:"title"`
	Narrative string          `json:"narrative"`
	Summary   string          `json:"summary"`
	Content   string          `json:"content"`
	Body      string          `json:"body"`
	Text      string          `json:"text"`
	Items     json.RawMessage `json:"items"`
}

func paragraphsFromNestedSection(raw json.RawMessage) []Paragraph {
	var sec nestedSection
	if err := json.Unmarshal(raw, &sec); err != nil {
		return nil
	}
	text := firstNonEmpty(
		strings.TrimSpace(sec.Narrative),
		strings.TrimSpace(sec.Content),
		strings.TrimSpace(sec.Body),
		strings.TrimSpace(sec.Text),
		strings.TrimSpace(sec.Summary),
	)
	if text == "" && len(bytes.TrimSpace(sec.Items)) > 0 {
		var items []string
		if json.Unmarshal(sec.Items, &items) == nil {
			parts := make([]string, 0, len(items))
			for _, it := range items {
				it = strings.TrimSpace(it)
				if it != "" {
					parts = append(parts, it)
				}
			}
			if len(parts) > 0 {
				text = strings.Join(parts, "\n")
			}
		}
	}
	if text == "" {
		return nil
	}
	title := firstNonEmpty(strings.TrimSpace(sec.Headline), strings.TrimSpace(sec.Title))
	if title != "" && !strings.Contains(text, title) {
		text = title + "\n\n" + text
	}
	return []Paragraph{{Text: text}}
}

func isNestedSectionSkipKey(key string) bool {
	switch key {
	case "version", "narrative", "hero", "hero_metrics", "advice", "recommendations",
		"action_items", "advice_section", "report_meta", "metadata", "key_incidents",
		"actions_summary", "resource", "fleet", "logs", "changes", "assets", "usage",
		"headline", "sections", "summary", "paragraphs", "period", "granularity", "scope":
		return true
	default:
		return false
	}
}

func jsonStringFieldFromObject(raw json.RawMessage, field string) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	return jsonStringField(obj[field])
}

func jsonRawFieldFromObject(raw json.RawMessage, field string) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	v, ok := obj[field]
	if !ok {
		return nil
	}
	return v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// normalizeContentShape lifts a legacy flat LLM shape (top-level headline
// + sections / summary) into narrative.headline / narrative.paragraphs.
// Also back-fills narrative.paragraphs when the model put headline under
// narrative but parked prose in summary or title-only sections.
func normalizeContentShape(top map[string]json.RawMessage, log *slog.Logger) (bool, error) {
	existingHeadline, existingParas := narrativeFields(top["narrative"])
	headline := existingHeadline
	if headline == "" {
		headline = jsonStringField(top["headline"])
	}
	if headline == "" {
		return false, nil
	}
	if existingHeadline != "" && len(existingParas) > 0 {
		return false, nil
	}

	paras := existingParas
	legacyParas, err := paragraphsFromLegacy(top)
	if err != nil {
		return false, err
	}
	if len(paras) == 0 {
		paras = legacyParas
	}

	needsFix := existingHeadline == "" || len(existingParas) == 0 && len(legacyParas) > 0
	if !needsFix {
		return false, nil
	}

	narrative, err := json.Marshal(map[string]any{
		"headline":   headline,
		"paragraphs": paras,
	})
	if err != nil {
		return false, err
	}
	if log != nil {
		log.Info("parse content: normalized narrative shape",
			slog.String("headline", truncate(headline, 120)),
			slog.Int("paragraphs", len(paras)),
			slog.String("hint", "LLM returned flat headline/sections/summary instead of narrative.{headline,paragraphs}"),
		)
	}
	top["narrative"] = narrative
	delete(top, "headline")
	delete(top, "sections")
	delete(top, "paragraphs")
	delete(top, "summary")
	delete(top, "period")
	delete(top, "granularity")
	delete(top, "scope")
	return true, nil
}

func narrativeFields(raw json.RawMessage) (string, []Paragraph) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}
	var n struct {
		Headline   string      `json:"headline"`
		Paragraphs []Paragraph `json:"paragraphs"`
	}
	if err := json.Unmarshal(raw, &n); err != nil {
		return "", nil
	}
	return strings.TrimSpace(n.Headline), n.Paragraphs
}

func jsonStringField(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '"' {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func paragraphsFromLegacy(top map[string]json.RawMessage) ([]Paragraph, error) {
	if raw, ok := top["paragraphs"]; ok {
		var paras []Paragraph
		if err := json.Unmarshal(raw, &paras); err == nil && len(paras) > 0 {
			return paras, nil
		}
	}
	if paras, ok, err := paragraphsFromSectionsField(top["sections"]); err != nil {
		return nil, err
	} else if ok {
		return paras, nil
	}
	if paras := paragraphsFromSummaryField(top["summary"]); len(paras) > 0 {
		return paras, nil
	}
	return nil, nil
}

type sectionLegacy struct {
	Title   string          `json:"title"`
	Body    string          `json:"body"`
	Text    string          `json:"text"`
	Content string          `json:"content"`
	Items   json.RawMessage `json:"items"`
}

func (s sectionLegacy) combineText() (text string, hadBody bool) {
	text = strings.TrimSpace(s.Text)
	if text != "" {
		return text, true
	}
	body := strings.TrimSpace(s.Body)
	if body == "" {
		body = strings.TrimSpace(s.Content)
	}
	if body == "" && len(bytes.TrimSpace(s.Items)) > 0 {
		var items []string
		if json.Unmarshal(s.Items, &items) == nil {
			parts := make([]string, 0, len(items))
			for _, it := range items {
				it = strings.TrimSpace(it)
				if it != "" {
					parts = append(parts, it)
				}
			}
			if len(parts) > 0 {
				body = strings.Join(parts, "\n")
			}
		}
	}
	title := strings.TrimSpace(s.Title)
	switch {
	case title != "" && body != "":
		return title + "\n\n" + body, true
	case body != "":
		return body, true
	default:
		return title, false
	}
}

func paragraphsFromSectionsField(raw json.RawMessage) ([]Paragraph, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, false, nil
	}
	var sections []sectionLegacy
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, false, fmt.Errorf("sections: %w", err)
	}
	out := make([]Paragraph, 0, len(sections))
	hadBody := false
	for _, s := range sections {
		text, body := s.combineText()
		if body {
			hadBody = true
		}
		if text != "" {
			out = append(out, Paragraph{Text: text})
		}
	}
	if len(out) == 0 || !hadBody {
		return nil, false, nil
	}
	return out, true, nil
}

func paragraphsFromSummaryField(raw json.RawMessage) []Paragraph {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		return []Paragraph{{Text: s}}
	case '[':
		var arr []string
		if err := json.Unmarshal(raw, &arr); err != nil || len(arr) == 0 {
			return nil
		}
		out := make([]Paragraph, 0, len(arr))
		for _, s := range arr {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, Paragraph{Text: s})
			}
		}
		return out
	default:
		return nil
	}
}

// normalizeContentAdviceOnMap rewrites the top-level advice field when the
// LLM returns a string, a string array, or a lone object instead of
// [{"text":"..."}].
func normalizeContentAdviceOnMap(top map[string]json.RawMessage, log *slog.Logger) (bool, error) {
	adviceRaw, ok := top["advice"]
	if !ok {
		return false, nil
	}
	normalized, changed, err := normalizeAdviceJSON(adviceRaw)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	if log != nil {
		log.Info("parse content: normalized advice field",
			slog.String("from", truncate(string(adviceRaw), 400)),
			slog.String("to", truncate(string(normalized), 400)),
			slog.String("hint", "LLM returned string or string[] instead of [{\"text\":...}]"),
		)
	}
	top["advice"] = normalized
	return true, nil
}

// normalizeContentHeroOnMap fills missing hero keys/labels. gpt-5.x often
// emits label+value cards without the stable key the SPA expects; the
// generator overwrites hero from facts anyway, but ParseContent validates
// before that merge.
func normalizeContentHeroOnMap(top map[string]json.RawMessage, log *slog.Logger) (bool, error) {
	raw, ok := top["hero"]
	if !ok {
		return false, nil
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false, nil
	}
	var cards []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cards); err != nil {
		return false, nil // leave unmarshal to report the shape error
	}
	changed := false
	for i := range cards {
		key := jsonStringField(cards[i]["key"])
		label := jsonStringField(cards[i]["label"])
		if key == "" && label != "" {
			key = heroKeyFromLabel(label, i)
			b, err := json.Marshal(key)
			if err != nil {
				return false, err
			}
			cards[i]["key"] = b
			changed = true
		}
		if label == "" && key != "" {
			b, err := json.Marshal(key)
			if err != nil {
				return false, err
			}
			cards[i]["label"] = b
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	normalized, err := json.Marshal(cards)
	if err != nil {
		return false, err
	}
	if log != nil {
		log.Info("parse content: normalized hero field",
			slog.Int("cards", len(cards)),
			slog.String("hint", "LLM returned hero cards without key; derived from label"),
		)
	}
	top["hero"] = normalized
	return true, nil
}

func heroKeyFromLabel(label string, index int) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return fmt.Sprintf("hero_%d", index)
	}
	// Align with facts collector labels → stable keys.
	known := map[string]string{
		"监控设备":    "devices",
		"CPU 均值":  "cpu_avg",
		"内存 均值":   "mem_avg",
		"磁盘 峰值":   "disk_peak",
		"潜在错误":    "log_errors",
		"Incidents": "incidents",
		"Agent 动作":  "actions",
		"在线设备":    "online",
	}
	if k, ok := known[label]; ok {
		return k
	}
	var b strings.Builder
	prevUnderscore := false
	for _, r := range label {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			prevUnderscore = false
			continue
		}
		if !prevUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	s := strings.Trim(b.String(), "_")
	if s != "" {
		return s
	}
	return fmt.Sprintf("hero_%d", index)
}

// normalizeAdviceJSON coerces common LLM advice shapes into []Advice.
func normalizeAdviceJSON(raw json.RawMessage) (json.RawMessage, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return raw, false, nil
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, err
		}
		out, err := json.Marshal([]Advice{{Text: s}})
		return out, true, err
	case '{':
		var adv Advice
		if err := json.Unmarshal(raw, &adv); err != nil {
			return nil, false, err
		}
		out, err := json.Marshal([]Advice{adv})
		return out, true, err
	case '[':
		var elems []json.RawMessage
		if err := json.Unmarshal(raw, &elems); err != nil {
			return nil, false, err
		}
		changed := false
		out := make([]Advice, 0, len(elems))
		for i, elem := range elems {
			elem = bytes.TrimSpace(elem)
			if len(elem) > 0 && elem[0] == '"' {
				var s string
				if err := json.Unmarshal(elem, &s); err != nil {
					return nil, false, fmt.Errorf("advice[%d]: %w", i, err)
				}
				out = append(out, Advice{Text: s})
				changed = true
				continue
			}
			var adv Advice
			if err := json.Unmarshal(elem, &adv); err != nil {
				return nil, false, fmt.Errorf("advice[%d]: %w", i, err)
			}
			out = append(out, adv)
		}
		if !changed {
			return raw, false, nil
		}
		normalized, err := json.Marshal(out)
		return normalized, true, err
	default:
		return nil, false, fmt.Errorf("unexpected JSON type %q", string(raw[:min(16, len(raw))]))
	}
}

func logContentParseFailure(log *slog.Logger, original, normalized string, err error, stage string) {
	if log == nil {
		return
	}
	attrs := []any{
		slog.String("stage", stage),
		slog.Any("err", err),
		slog.Int("raw_bytes", len(original)),
		slog.Bool("advice_normalized", original != normalized),
	}
	if advice := extractJSONField(original, "advice"); advice != "" {
		attrs = append(attrs, slog.String("advice_field", truncate(advice, 500)))
	}
	if narrative := extractJSONField(original, "narrative"); narrative != "" {
		attrs = append(attrs, slog.String("narrative_field", truncate(narrative, 300)))
	}
	attrs = append(attrs, slog.String("json_preview", truncate(original, 1000)))
	log.Warn("parse content failed", attrs...)
}

func extractJSONField(raw, key string) string {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return ""
	}
	v, ok := probe[key]
	if !ok {
		return ""
	}
	return string(v)
}

// Validate enforces the minimal shape the SPA depends on. Lenient on
// optional sections (a calm report may have empty KeyIncidents/Advice)
// but strict on the spine: a headline must exist and hero cards must
// each carry a key + label so the grid renders.
func (c *Content) Validate() error {
	if strings.TrimSpace(c.Narrative.Headline) == "" {
		return fmt.Errorf("report: content missing narrative.headline")
	}
	for i, h := range c.Hero {
		if h.Key == "" || h.Label == "" {
			return fmt.Errorf("report: hero[%d] missing key/label", i)
		}
	}
	return nil
}

// MustJSON serialises content; panics only on a programming error
// (unmarshalable types can't occur with these concrete structs).
func (c *Content) MustJSON() string {
	b, err := json.Marshal(c)
	if err != nil {
		panic(fmt.Sprintf("report: content marshal: %v", err))
	}
	return string(b)
}

// RenderMarkdown produces the ContentMD fallback from structured
// content. Used for export, IM plain-text fallback, and full-text
// search. Entity tokens are flattened to their display name.
func (c *Content) RenderMarkdown(title, locale string) string {
	en := locale == "en"
	mtr := func(zh, eng string) string {
		if en {
			return eng
		}
		return zh
	}
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")

	if c.Narrative.Headline != "" {
		b.WriteString("## " + c.Narrative.Headline + "\n\n")
	}
	for _, p := range c.Narrative.Paragraphs {
		b.WriteString(flattenEntities(p.Text) + "\n\n")
	}

	if c.Resource.Available {
		b.WriteString("## " + mtr("资源使用（周期 均值 / 峰值）", "Resource usage (period avg / peak)") + "\n\n")
		avg, peak := mtr("均", "avg"), mtr("峰", "peak")
		b.WriteString(fmt.Sprintf("- CPU: %s %.1f%% · %s %.1f%%\n", avg, c.Resource.CPUAvg, peak, c.Resource.CPUPeak))
		b.WriteString(fmt.Sprintf("- %s: %s %.1f%% · %s %.1f%%\n", mtr("内存", "Memory"), avg, c.Resource.MemAvg, peak, c.Resource.MemPeak))
		b.WriteString(fmt.Sprintf("- %s: %s %.1f%% · %s %.1f%%\n\n", mtr("磁盘", "Disk"), avg, c.Resource.DiskAvg, peak, c.Resource.DiskPeak))
	}

	b.WriteString("## " + mtr("监控覆盖", "Monitoring coverage") + "\n\n")
	b.WriteString(fmt.Sprintf("- %s\n", mtr(
		fmt.Sprintf("监控设备 %d 台 · 在线 %d 台", c.Fleet.Total, c.Fleet.Online),
		fmt.Sprintf("%d devices · %d online", c.Fleet.Total, c.Fleet.Online))))
	if len(c.Fleet.Roles) > 0 {
		var roles []string
		for r, n := range c.Fleet.Roles {
			roles = append(roles, fmt.Sprintf("%s ×%d", r, n))
		}
		b.WriteString("- " + mtr("角色", "Roles") + ": " + strings.Join(roles, " · ") + "\n")
	}
	b.WriteString("\n")

	if c.Logs.Available {
		b.WriteString("## " + mtr("应用日志（潜在错误）", "Application logs (potential errors)") + "\n\n")
		b.WriteString(fmt.Sprintf("- %s\n", mtr(
			fmt.Sprintf("潜在错误 %d 条", c.Logs.TotalErrors),
			fmt.Sprintf("%d potential errors", c.Logs.TotalErrors),
		)))
		if c.Logs.DeltaPct != nil {
			b.WriteString(fmt.Sprintf("- %s\n", mtr(
				fmt.Sprintf("较上周期 %+.0f%%", *c.Logs.DeltaPct),
				fmt.Sprintf("%+.0f%% vs prior period", *c.Logs.DeltaPct),
			)))
		}
		if len(c.Logs.TopSources) > 0 {
			b.WriteString("- " + mtr("主要来源", "Top sources") + ":\n")
			curKind := ""
			for _, s := range c.Logs.TopSources {
				if s.Kind != curKind {
					curKind = s.Kind
					b.WriteString(fmt.Sprintf("  [%s]\n", logSourceKindLabel(curKind, mtr)))
				}
				label := s.DisplayName
				if label == "" {
					label = s.Name
				}
				if s.DeviceName != "" {
					label = s.DeviceName + "/" + label
				}
				detail := fmt.Sprintf("%d", s.Count)
				if s.OngridSource != "" {
					detail += " · " + s.OngridSource
				}
				if s.SampleLine != "" {
					detail += " · " + truncateReportSample(s.SampleLine, 120)
				}
				b.WriteString(fmt.Sprintf("  - %s: %s\n", label, detail))
			}
		}
		b.WriteString("\n")
	}

	if len(c.KeyIncidents) > 0 {
		b.WriteString("## " + mtr("告警与处置", "Alerts & response") + "\n\n")
		for _, ki := range c.KeyIncidents {
			b.WriteString(fmt.Sprintf("- I-%d %s (%s, %dm, %s)\n",
				ki.ID, ki.Title, ki.Severity, ki.DurationMin, ki.Status))
		}
		b.WriteString(fmt.Sprintf("- %s\n\n", mtr(
			fmt.Sprintf("Agent 动作: mutating %d（批准 %d）· 只读 %d", c.Actions.MutatingTotal, c.Actions.MutatingApproved, c.Actions.SafeTotal),
			fmt.Sprintf("Agent actions: mutating %d (approved %d) · read-only %d", c.Actions.MutatingTotal, c.Actions.MutatingApproved, c.Actions.SafeTotal))))
	}

	if len(c.Changes) > 0 {
		b.WriteString("## " + mtr("变更记录", "Changes") + "\n\n")
		for _, ch := range c.Changes {
			b.WriteString(fmt.Sprintf("- %s %s %s\n", ch.At.Format("01-02 15:04"), ch.Action, ch.ResourceName))
		}
		b.WriteString("\n")
	}

	if len(c.Advice) > 0 {
		b.WriteString("## " + mtr("建议", "Recommendations") + "\n\n")
		for _, a := range c.Advice {
			b.WriteString("- " + flattenEntities(a.Text) + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

func logSourceKindLabel(kind string, mtr func(zh, en string) string) string {
	switch kind {
	case "container":
		return mtr("容器", "Container")
	case "unit":
		return mtr("Unit", "Unit")
	case "file":
		return mtr("文件", "File")
	default:
		return mtr("其他", "Other")
	}
}

func truncateReportSample(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// formatNum prints an integer without trailing .0, else a 1-decimal.
func formatNum(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}

// flattenEntities rewrites `{{entity:kind:id|name}}` → `name` for the
// markdown fallback (chips are an SPA-only affordance).
func flattenEntities(s string) string {
	for {
		start := strings.Index(s, "{{entity:")
		if start < 0 {
			return s
		}
		end := strings.Index(s[start:], "}}")
		if end < 0 {
			return s // malformed; leave as-is
		}
		end += start
		token := s[start+2 : end] // entity:kind:id|name
		name := token
		if bar := strings.LastIndex(token, "|"); bar >= 0 {
			name = token[bar+1:]
		}
		s = s[:start] + name + s[end+2:]
	}
}
