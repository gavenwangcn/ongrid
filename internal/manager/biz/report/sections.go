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
			parts = append(parts, mtr("集群态势（CPU/内存/磁盘/网络资源水位与监控覆盖）", "cluster posture (CPU/memory/disk/network resource & coverage)"))
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

// clusterResourceDirective tells the reporter to synthesize all four
// resource dimensions when the cluster section is enabled.
func clusterResourceDirective(sections []string, en bool) string {
	if !SectionEnabled(sections, SectionCluster) {
		return ""
	}
	mtr := func(zh, eng string) string {
		if en {
			return eng
		}
		return zh
	}
	return mtr(
		"集群态势写作要求：narrative 必须对 CPU、内存、磁盘、网络四类资源做总体分析（fleet 汇总）。"+
			"引用 facts.resource 中的 cpu/mem/disk 周期均值与峰值（百分比），以及 net_rx/net_tx 吞吐均值与峰值（bytes/s，物理网卡）。"+
			"逐台明细在 facts.device_resources[]（cpu_count / mem_total_bytes / disk_total_bytes 容量 + 周期利用率）—— narrative 只点出异常或 Top 设备，不要逐台罗列全表；"+
			"需要接口/连通性等定性信息时可只读调用 query_edges(role=network)。"+
			"禁止只写三类资源而忽略网络。\n",
		"CLUSTER POSTURE: In narrative, synthesize ALL four resource dimensions at fleet level — CPU, memory, disk, and network. "+
			"Cite facts.resource cpu/mem/disk period avg/peak (percent) and net_rx/net_tx avg/peak throughput (bytes/s, physical NICs). "+
			"Per-host detail lives in facts.device_resources[] — call out outliers or top devices only, do NOT paste the full table into prose; "+
			"use query_edges(role=network) read-only for qualitative interface/connectivity context. "+
			"Do NOT omit network when discussing resource posture.\n",
	)
}
