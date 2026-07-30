package report

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	model "github.com/ongridio/ongrid/internal/manager/model/report"
)

// Deliverer fans a finished report out to notification channels. It's a
// seam (implemented in main.go over the notify router + channel store)
// so biz/report stays free of the notify / alert imports. A nil
// Deliverer means in-app only — the report is still viewable, just not
// pushed. The locked decision: only ready reports are delivered; a
// failed report is never pushed (no half-baked sends).
type Deliverer interface {
	// Deliver sends summary to each channel id and returns one record
	// per channel. Must not block indefinitely — the caller runs it
	// inline after MarkReady, so the impl bounds each send with a
	// timeout. Errors are captured in the records, not returned.
	Deliver(ctx context.Context, summary DeliverySummary, channelIDs []uint64) []DeliveryRecord
}

// DeliverySummary is the channel-agnostic payload. The concrete
// Deliverer renders it into each channel's native format (markdown
// text for v1; a Feishu interactive card is a future enhancement). The
// DeepLink is the in-app report URL the "view full report" affordance
// points at.
type DeliverySummary struct {
	Title     string
	Headline  string
	Hero      []HeroStat
	Resource  ResourceFacts
	Fleet     FleetFacts
	Logs      LogFacts
	Incidents []KeyIncident
	Actions   ActionsSummary
	Sections  []string
	DeepLink  string // public share URL (/r/{token}); absolute when PublicURL set
	ReportID  string
}

// DeliveryRecord is one channel's delivery outcome, persisted into
// reports.delivery_json and surfaced on the detail page.
type DeliveryRecord struct {
	ChannelID    uint64    `json:"channel_id"`
	ChannelType  string    `json:"channel_type,omitempty"`
	Status       string    `json:"status"` // "sent" | "failed"
	SentAt       time.Time `json:"sent_at"`
	Error        string    `json:"error,omitempty"`
	FallbackUsed bool      `json:"fallback_used,omitempty"`
}

// parseChannelIDsJSON decodes a schedule/task channel_ids_json column.
func parseChannelIDsJSON(raw string) []uint64 {
	if raw == "" {
		return nil
	}
	var ids []uint64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

// encodeChannelIDsJSON encodes channel ids for persistence.
func encodeChannelIDsJSON(ids []uint64) string {
	if len(ids) == 0 {
		return "[]"
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// MarkdownSummary renders the channel-agnostic markdown body senders
// use. Headline + enabled section headers (cluster / logs / alerts),
// then the deep link. Subject carries Title separately — do not repeat
// it here. IM channels receive fleet-level summaries only; per-device
// log sources and incident lists stay in the in-app report.
func (s DeliverySummary) MarkdownSummary() string {
	var b strings.Builder
	if s.Headline != "" {
		b.WriteString(flattenEntities(s.Headline))
	}
	sections := s.Sections
	if len(sections) == 0 {
		sections = allReportSections
	}
	if SectionEnabled(sections, SectionCluster) {
		if s.Resource.Available || s.Fleet.Total > 0 {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			writeClusterPosture(&b, s.Resource, s.Fleet)
		} else if len(s.Hero) > 0 {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			parts := make([]string, 0, len(s.Hero))
			for _, h := range s.Hero {
				v := fmt.Sprintf("%s %s%s", h.Label, formatNum(h.Value), h.Unit)
				if h.DeltaPct != nil {
					arrow := "→"
					if *h.DeltaPct < 0 {
						arrow = "↓"
					} else if *h.DeltaPct > 0 {
						arrow = "↑"
					}
					v += fmt.Sprintf(" %s%.0f%%", arrow, abs(*h.DeltaPct))
				}
				parts = append(parts, v)
			}
			b.WriteString(strings.Join(parts, " · "))
		}
	}
	if SectionEnabled(sections, SectionLogs) {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		writeLogsPosture(&b, s.Logs)
	}
	if SectionEnabled(sections, SectionAlerts) {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		writeAlertsPosture(&b, s.Incidents, s.Actions)
	}
	if s.DeepLink != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("\n[查看完整报告 →](" + s.DeepLink + ")")
	}
	return b.String()
}

func writeClusterPosture(b *strings.Builder, res ResourceFacts, fleet FleetFacts) {
	b.WriteString("\n**集群态势**\n")
	b.WriteString("CPU / 内存 / 磁盘 / 网络 · 监控覆盖\n")
	if res.Available {
		b.WriteString(fmt.Sprintf("CPU · 均 %s%% / 峰 %s%%\n",
			formatNum(res.CPUAvg), formatNum(res.CPUPeak)))
		b.WriteString(fmt.Sprintf("内存 · 均 %s%% / 峰 %s%%\n",
			formatNum(res.MemAvg), formatNum(res.MemPeak)))
		b.WriteString(fmt.Sprintf("磁盘 · 均 %s%% / 峰 %s%%\n",
			formatNum(res.DiskAvg), formatNum(res.DiskPeak)))
		peakBps := res.NetRxPeakBps + res.NetTxPeakBps
		b.WriteString(fmt.Sprintf("网络 · 峰 %s · 入 %s / 出 %s\n",
			formatBytesPerSec(peakBps),
			formatBytesPerSec(res.NetRxAvgBps),
			formatBytesPerSec(res.NetTxAvgBps)))
	}
	if fleet.Total > 0 {
		b.WriteString(fmt.Sprintf("在线设备 · %d / 共 %d 台",
			fleet.Online, fleet.Total))
	} else if !res.Available {
		b.WriteString("本周期资源指标暂无数据")
	}
}

func writeLogsPosture(b *strings.Builder, logs LogFacts) {
	b.WriteString("\n**应用日志**\n")
	b.WriteString("潜在错误（error / panic / fatal）\n")
	if !logs.Available {
		b.WriteString("本周期日志指标暂无数据")
		return
	}
	line := fmt.Sprintf("潜在错误 · %d", logs.TotalErrors)
	if logs.DeltaPct != nil {
		sign := "+"
		if *logs.DeltaPct < 0 {
			sign = ""
		}
		line += fmt.Sprintf("（较上周期 %s%s%%）", sign, formatNum(*logs.DeltaPct))
	} else if logs.PrevTotalErrors > 0 {
		line += fmt.Sprintf("（上周期 %d）", logs.PrevTotalErrors)
	}
	b.WriteString(line)
}

func writeAlertsPosture(b *strings.Builder, incidents []KeyIncident, actions ActionsSummary) {
	b.WriteString("\n**告警与处理**\n")
	b.WriteString("事件、处置与 agent 动作\n")
	resolved := 0
	for _, inc := range incidents {
		if inc.Status == "resolved" {
			resolved++
		}
	}
	mttr := mttrMinutes(incidents)
	totalActions := actions.MutatingTotal + actions.SafeTotal
	b.WriteString(fmt.Sprintf("告警 · %d · 已解决 %d · MTTR %d min",
		len(incidents), resolved, mttr))
	if totalActions > 0 {
		b.WriteString(fmt.Sprintf(" · Agent 动作 %d（变更 %d · 只读 %d）",
			totalActions, actions.MutatingTotal, actions.SafeTotal))
	}
}

func mttrMinutes(incidents []KeyIncident) int {
	var resolved []KeyIncident
	for _, inc := range incidents {
		if inc.Status == "resolved" {
			resolved = append(resolved, inc)
		}
	}
	if len(resolved) == 0 {
		return 0
	}
	sum := 0
	for _, inc := range resolved {
		sum += inc.DurationMin
	}
	return sum / len(resolved)
}

// deliveryFor builds the summary for a ready report by parsing its
// content. Falls back to the stored SummaryText when content can't be
// parsed (never blocks delivery on a content quirk).
func deliveryFor(rpt *model.Report, deepLink string) DeliverySummary {
	s := DeliverySummary{
		Title:    rpt.Title,
		Headline: rpt.SummaryText,
		DeepLink: deepLink,
		ReportID: rpt.ID,
	}
	if rpt.ContentJSON != "" {
		if c, err := ParseContent(rpt.ContentJSON, nil); err == nil {
			s.Hero = c.Hero
			im := deliveryIMFactsFromContent(c)
			s.Sections = im.sections
			s.Resource = im.resource
			s.Fleet = im.fleet
			s.Logs = im.logs
			s.Incidents = im.incidents
			s.Actions = im.actions
			if h := summaryHeadline(&c); h != "" {
				s.Headline = h
			}
		}
	}
	return s
}

type deliveryIMFacts struct {
	sections  []string
	resource  ResourceFacts
	fleet     FleetFacts
	logs      LogFacts
	incidents []KeyIncident
	actions   ActionsSummary
}

// deliveryIMFactsFromContent picks fleet-level stats for IM delivery.
// Scoped reports store metrics under systems[]; legacy reports use top-level
// fields. Multi-system reports aggregate logs/incidents/actions counts.
func deliveryIMFactsFromContent(c Content) deliveryIMFacts {
	out := deliveryIMFacts{
		sections: EffectiveSections(Scope{Sections: c.Metadata.Sections}),
	}
	if len(c.Systems) == 0 {
		out.resource = c.Resource
		out.fleet = c.Fleet
		out.logs = c.Logs
		out.incidents = c.KeyIncidents
		out.actions = c.Actions
		return out
	}
	if len(c.Systems) == 1 {
		sys := c.Systems[0]
		out.resource = sys.Resource
		out.fleet = sys.Fleet
		out.logs = sys.Logs
		out.incidents = sys.KeyIncidents
		out.actions = sys.Actions
		return out
	}
	for _, sys := range c.Systems {
		out.fleet.Total += sys.Fleet.Total
		out.fleet.Online += sys.Fleet.Online
		if sys.Logs.Available {
			out.logs.Available = true
			out.logs.TotalErrors += sys.Logs.TotalErrors
			out.logs.PrevTotalErrors += sys.Logs.PrevTotalErrors
		}
		out.incidents = append(out.incidents, sys.KeyIncidents...)
		out.actions.MutatingTotal += sys.Actions.MutatingTotal
		out.actions.MutatingApproved += sys.Actions.MutatingApproved
		out.actions.SafeTotal += sys.Actions.SafeTotal
	}
	return out
}

// recordDelivery serialises the per-channel records into the report row.
func recordDelivery(rpt *model.Report, records []DeliveryRecord) {
	if len(records) == 0 {
		return
	}
	if b, err := json.Marshal(records); err == nil {
		rpt.DeliveryJSON = string(b)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
