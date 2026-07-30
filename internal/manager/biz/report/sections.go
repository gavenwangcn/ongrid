package report

import "strings"

// Report section keys stored in ScopeJSON.sections and Content.metadata.sections.
const (
	SectionCluster = "cluster" // 集群态势 — resource + fleet + coverage narrative
	SectionLogs    = "logs"    // 应用日志 — Loki error trends
	SectionAlerts  = "alerts"  // 告警与处理 — incidents + agent actions
)

var allReportSections = []string{SectionCluster, SectionLogs, SectionAlerts}

// EffectiveSections returns which sections to collect/render. Empty
// scope.Sections means legacy behaviour: all sections (keeps historical
// daily/weekly reports intact). When sections is non-empty, only those
// keys are enabled.
func EffectiveSections(scope Scope) []string {
	if len(scope.Sections) == 0 {
		out := make([]string, len(allReportSections))
		copy(out, allReportSections)
		return out
	}
	return NormalizeSections(scope.Sections)
}

// NormalizeSections dedupes and filters to known keys, preserving order.
func NormalizeSections(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if !IsValidSection(s) {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func IsValidSection(s string) bool {
	switch s {
	case SectionCluster, SectionLogs, SectionAlerts:
		return true
	default:
		return false
	}
}

func SectionEnabled(sections []string, want string) bool {
	for _, s := range sections {
		if s == want {
			return true
		}
	}
	return false
}

// DefaultDailySections is the UI default for newly created daily reports.
func DefaultDailySections() []string {
	return []string{SectionCluster}
}

func dataSourcesForSections(sections []string) []string {
	var out []string
	if SectionEnabled(sections, SectionCluster) {
		out = append(out, "prometheus", "devices")
	}
	if SectionEnabled(sections, SectionLogs) {
		out = append(out, "loki")
	}
	if SectionEnabled(sections, SectionAlerts) {
		out = append(out, "incidents", "audit_log", "proposals")
	}
	return out
}

func sectionDirective(sections []string, en bool) string {
	if len(sections) == 0 || len(sections) == len(allReportSections) {
		return ""
	}
	mtr := func(zh, eng string) string {
		if en {
			return eng
		}
		return zh
	}
	var parts []string
	for _, s := range sections {
		switch s {
		case SectionCluster:
			parts = append(parts, mtr("集群态势（资源水位与监控覆盖）", "cluster posture (resource & coverage)"))
		case SectionLogs:
			parts = append(parts, mtr("应用日志（潜在错误）", "application logs (potential errors)"))
		case SectionAlerts:
			parts = append(parts, mtr("告警与处理（事件与 agent 动作）", "alerts & response (incidents & agent actions)"))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if en {
		return "Include narrative/advice ONLY for these report sections (ignore facts for omitted sections): " + strings.Join(parts, "; ") + ".\n"
	}
	return "仅围绕以下报告模块撰写 narrative/advice（未选模块的事实可忽略）：" + strings.Join(parts, "；") + "。\n"
}
