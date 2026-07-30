package report

import (
	"context"
	"encoding/json"
	"time"
)

// ReportFacts is the deterministic, pre-computed input handed to the
// reporter agent (HLD-014 §数据收集). The agent narrates these facts —
// it never computes the numbers. Redesigned (2026-06-06 review): the
// report leads with fleet resource trends + monitoring coverage rather
// than being incident-only, so a calm period still carries substance.
// Every number comes from Prometheus (period avg/peak) or a SQL query;
// the agent's only freedom is prose (narrative + advice).
type ReportFacts struct {
	Period     Period `json:"-"`
	PrevPeriod Period `json:"-"`

	// Hero is the pre-computed big-number set. Redesigned to lead with
	// non-incident signals (devices monitored, fleet CPU/mem avg, disk
	// peak) so the cards are meaningful even when nothing fired.
	Hero []HeroStat `json:"hero"`

	// Resource is the fleet-aggregate resource trend over the period
	// (avg + peak), from Prometheus. Aggregated across devices — not
	// per-device (per-device reads as noise; review decision). Covers
	// CPU / memory / disk utilization and network rx/tx throughput.
	Resource ResourceFacts `json:"resource"`

	// Fleet is the monitoring-coverage snapshot: how many devices, how
	// many online, role breakdown.
	Fleet FleetFacts `json:"fleet"`

	// Incidents / Actions / AlertCounts stay, but the report de-
	// emphasizes them (a secondary section, not the lead).
	Incidents   []IncidentFact `json:"incidents"`
	Actions     ActionsSummary `json:"actions"`
	AlertCounts map[string]int `json:"alert_counts"`

	// Changes is the period's product-side change log (audit_logs):
	// rule / setting / device / channel / user edits. "Who changed what."
	Changes []ChangeFact `json:"changes"`

	// Assets / Usage are legacy ContentJSON fields kept for backward
	// compatibility with older reports; no longer collected or displayed.
	Assets AssetFacts `json:"assets,omitempty"`
	Usage  UsageFacts `json:"usage,omitempty"`

	// Logs is application log error analytics from Loki (potential errors
	// matching the Logs UI shortcut: error|panic|fatal). Scoped to the
	// same devices as the report (system_name / edge_ids).
	Logs LogFacts `json:"logs"`

	// DeviceResources holds per-host CPU / memory / disk / network
	// utilization for the report scope. In-app only — IM delivery stays
	// fleet-summary (hero + headline).
	DeviceResources DeviceResourceFacts `json:"device_resources,omitempty"`

	// NetworkDevices is deprecated; kept for older reports. New reports
	// use device_resources.
	NetworkDevices NetworkDeviceFacts `json:"network_devices,omitempty"`

	// Systems holds per-business-system facts when the report is scoped
	// to one or more systems (or all systems discovered from devices).
	// When populated, top-level Resource/Fleet/Logs are empty — render
	// and narrate per block, not fleet-aggregated.
	Systems []SystemFactsBlock `json:"systems,omitempty"`
}

// SystemFactsBlock is the deterministic input for one business system.
type SystemFactsBlock struct {
	SystemName  string         `json:"system_name"`
	Hero        []HeroStat     `json:"hero,omitempty"`
	Resource    ResourceFacts  `json:"resource"`
	Fleet       FleetFacts     `json:"fleet"`
	Incidents   []IncidentFact `json:"incidents,omitempty"`
	Actions     ActionsSummary `json:"actions,omitempty"`
	AlertCounts map[string]int `json:"alert_counts,omitempty"`
	Logs            LogFacts            `json:"logs"`
	DeviceResources DeviceResourceFacts `json:"device_resources,omitempty"`
	NetworkDevices  NetworkDeviceFacts  `json:"network_devices,omitempty"`
}

// LogFacts aggregates potential application errors from Loki over the
// report period. Available=false when Loki is unreachable, not wired, or
// the scope matched no devices.
type LogFacts struct {
	Available       bool             `json:"available"`
	TotalErrors     int              `json:"total_errors"`
	PrevTotalErrors int              `json:"prev_total_errors,omitempty"`
	DeltaPct        *float64         `json:"delta_pct,omitempty"`
	DailySparkline  []int            `json:"daily_sparkline,omitempty"`
	TopSources      []LogErrorSource `json:"top_sources,omitempty"`
	QueryPattern    string           `json:"query_pattern,omitempty"`
	SystemName      string           `json:"system_name,omitempty"`
}

// LogErrorSource is one high-volume error log stream in the period.
type LogErrorSource struct {
	DeviceID     uint64 `json:"device_id,omitempty"`
	DeviceName   string `json:"device_name,omitempty"`
	Kind         string `json:"kind"` // container | unit | file | other
	Name         string `json:"name"`
	DisplayName  string `json:"display_name,omitempty"`
	OngridSource string `json:"ongrid_source,omitempty"`
	Count        int    `json:"count"`
	SampleLine   string `json:"sample_line,omitempty"`
}

// AssetFacts counts platform assets created within the period.
type AssetFacts struct {
	NewAgents int `json:"new_agents"`
	NewSkills int `json:"new_skills"`
	NewRepos  int `json:"new_repos"`
}

// UsageFacts is the platform-usage summary over the period.
type UsageFacts struct {
	Sessions         int   `json:"sessions"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

// ResourceFacts is the fleet resource trend over the period. Available
// is false when Prometheus is unreachable or has no data for the window
// (old period past retention, or no metrics yet) — the renderer then
// shows a "no data" note instead of fake zeros.
type ResourceFacts struct {
	Available bool    `json:"available"`
	CPUAvg    float64 `json:"cpu_avg"`
	CPUPeak   float64 `json:"cpu_peak"`
	MemAvg    float64 `json:"mem_avg"`
	MemPeak   float64 `json:"mem_peak"`
	DiskAvg   float64 `json:"disk_avg"`
	DiskPeak  float64 `json:"disk_peak"`
	// Network throughput (bytes/s, physical interfaces only — excludes
	// lo / veth / docker). Fleet-aggregate avg/peak across scoped devices.
	NetRxAvgBps  float64 `json:"net_rx_avg_bps,omitempty"`
	NetRxPeakBps float64 `json:"net_rx_peak_bps,omitempty"`
	NetTxAvgBps  float64 `json:"net_tx_avg_bps,omitempty"`
	NetTxPeakBps float64 `json:"net_tx_peak_bps,omitempty"`
}

// NetworkDeviceFacts summarizes network-role hosts in scope.
type NetworkDeviceFacts struct {
	Available bool                `json:"available"`
	Devices   []NetworkDeviceStat `json:"devices,omitempty"`
}

// NetworkDeviceStat is one network-role host's period throughput.
type NetworkDeviceStat struct {
	DeviceID     uint64  `json:"device_id"`
	Name         string  `json:"name,omitempty"`
	Online       bool    `json:"online"`
	NetRxAvgBps  float64 `json:"net_rx_avg_bps,omitempty"`
	NetRxPeakBps float64 `json:"net_rx_peak_bps,omitempty"`
	NetTxAvgBps  float64 `json:"net_tx_avg_bps,omitempty"`
	NetTxPeakBps float64 `json:"net_tx_peak_bps,omitempty"`
}

// DeviceResourceFacts is per-host resource utilization in scope.
type DeviceResourceFacts struct {
	Available bool                `json:"available"`
	Devices   []DeviceResourceStat `json:"devices,omitempty"`
}

// DeviceResourceStat is one host's capacity plus period avg/peak for CPU,
// memory, disk (%), and network throughput (bytes/s).
type DeviceResourceStat struct {
	DeviceID     uint64  `json:"device_id"`
	Name         string  `json:"name,omitempty"`
	Online       bool    `json:"online"`
	CPUCount     int     `json:"cpu_count,omitempty"`
	MemTotalBytes  uint64 `json:"mem_total_bytes,omitempty"`
	DiskTotalBytes uint64 `json:"disk_total_bytes,omitempty"`
	CPUAvg       float64 `json:"cpu_avg,omitempty"`
	CPUPeak      float64 `json:"cpu_peak,omitempty"`
	MemAvg       float64 `json:"mem_avg,omitempty"`
	MemPeak      float64 `json:"mem_peak,omitempty"`
	DiskAvg      float64 `json:"disk_avg,omitempty"`
	DiskPeak     float64 `json:"disk_peak,omitempty"`
	NetRxAvgBps  float64 `json:"net_rx_avg_bps,omitempty"`
	NetRxPeakBps float64 `json:"net_rx_peak_bps,omitempty"`
	NetTxAvgBps  float64 `json:"net_tx_avg_bps,omitempty"`
	NetTxPeakBps float64 `json:"net_tx_peak_bps,omitempty"`
}

// FleetFacts is the monitoring-coverage snapshot at period end.
type FleetFacts struct {
	Total  int            `json:"total"`
	Online int            `json:"online"`
	Roles  map[string]int `json:"roles,omitempty"` // role name → device count
}

// IncidentFact is one incident's SQL-true facts. DurationMin is
// resolved_at - first_fired_at (or now - first_fired_at if still open).
type IncidentFact struct {
	ID          uint64 `json:"id"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	DeviceID    uint64 `json:"device_id,omitempty"`
	DurationMin int    `json:"duration_min"`
}

// ChangeFact is one product-side change from the audit log within the
// period — the "who changed what" signal that's useful regardless of
// incidents.
type ChangeFact struct {
	At           time.Time `json:"at"`
	Action       string    `json:"action"`        // e.g. rule_update
	ResourceType string    `json:"resource_type"` // e.g. alert_rule
	ResourceName string    `json:"resource_name,omitempty"`
	Actor        string    `json:"actor,omitempty"` // user email
}

// Scope is the parsed ReportSchedule.ScopeJSON. v1 honours EdgeIDs,
// SystemName, EnvironmentTag, and SeverityMin; FleetTags is parsed but a no-op until
// G.2.6 edge tags.
type Scope struct {
	FleetTags      []string `json:"fleet_tags,omitempty"`
	EdgeIDs        []uint64 `json:"edge_ids,omitempty"`
	SystemName     string   `json:"system_name,omitempty"`
	SystemNames    []string `json:"system_names,omitempty"`
	EnvironmentTag string   `json:"environment_tag,omitempty"`
	SeverityMin    string   `json:"severity_min,omitempty"`
	// Sections lists enabled report body modules (cluster/logs/alerts).
	// Empty → all sections (legacy). Daily reports created from the SPA
	// stamp an explicit list; default is cluster-only.
	Sections []string `json:"sections,omitempty"`
}

// ParseScope reads a ScopeJSON blob. Empty / "{}" / invalid → zero
// Scope (full coverage) rather than an error.
func ParseScope(raw string) Scope {
	var s Scope
	if raw == "" {
		return s
	}
	_ = json.Unmarshal([]byte(raw), &s)
	return NormalizeScope(s)
}

// FactsCollector runs the pure-data collection. Implemented by
// data/report/store.FactsCollector. Period and prev are pre-computed by
// the Usecase (PeriodFor); scope filters the queries.
type FactsCollector interface {
	Collect(ctx context.Context, period, prev Period, scope Scope) (*ReportFacts, error)
}
