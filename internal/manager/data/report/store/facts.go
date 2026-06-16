package store

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	bizreport "github.com/ongridio/ongrid/internal/manager/biz/report"
	"github.com/ongridio/ongrid/internal/pkg/promquery"
)

// FactsCollector implements bizreport.FactsCollector. It computes the
// deterministic report inputs from two sources:
//   - Prometheus (period avg/peak fleet resource trends) via an injected
//     promquery.Client — the report leads with these so a calm period
//     still has substance.
//   - SQL over devices / alert_incidents / chat_mutating_proposals /
//     audit_logs for monitoring coverage, alerts, and the change log.
//
// No LLM, no mutation. A failure in any one source degrades to its zero
// value (e.g. Prometheus down → Resource.Available=false) rather than
// aborting the whole report.
type FactsCollector struct {
	db   *gorm.DB
	prom PromQuerier
	loki LogQuerier
	log  *slog.Logger
}

// PromQuerier is the narrow Prometheus surface. Implemented by
// *promquery.Client. nil = resource trends unavailable (report still
// renders the SQL-derived sections).
type PromQuerier interface {
	Query(ctx context.Context, expr string, ts time.Time) (*promquery.InstantResult, error)
	QueryRange(ctx context.Context, expr string, start, end time.Time, step time.Duration) (*promquery.InstantResult, error)
}

func NewFactsCollector(db *gorm.DB, prom PromQuerier, loki LogQuerier, log *slog.Logger) *FactsCollector {
	if log == nil {
		log = slog.Default()
	}
	return &FactsCollector{
		db:   db,
		prom: prom,
		loki: loki,
		log:  log.With(slog.String("comp", "report-facts")),
	}
}

var _ bizreport.FactsCollector = (*FactsCollector)(nil)

var severityRank = map[string]int{"info": 0, "warning": 1, "critical": 2}

func (c *FactsCollector) Collect(ctx context.Context, period, prev bizreport.Period, scope bizreport.Scope) (*bizreport.ReportFacts, error) {
	start := time.Now()
	c.log.Info("report facts collect start",
		slog.Time("period_start", period.Start),
		slog.Time("period_end", period.End),
		slog.String("system_name", strings.TrimSpace(scope.SystemName)),
		slog.Bool("prom_wired", c.prom != nil),
		slog.Bool("loki_wired", c.loki != nil),
	)

	scope, err := c.resolveScope(ctx, scope)
	if err != nil {
		c.log.Error("report facts scope resolve failed", slog.Any("err", err))
		return nil, err
	}
	c.log.Info("report facts scope resolved",
		slog.String("system_name", strings.TrimSpace(scope.SystemName)),
		slog.Any("device_ids", scopedDeviceIDs(scope)),
		slog.Bool("scoped_empty", scopedEmpty(scope)),
	)

	facts := &bizreport.ReportFacts{
		Period:      period,
		PrevPeriod:  prev,
		AlertCounts: map[string]int{},
	}

	incidents, err := c.collectIncidents(ctx, period, scope)
	if err != nil {
		c.log.Error("report facts incidents failed", slog.Any("err", err))
		return nil, err
	}
	facts.Incidents = incidents
	facts.Actions = c.collectActions(ctx, period)
	facts.AlertCounts = c.collectAlertCounts(ctx, period, scope)
	facts.Fleet = c.collectFleet(ctx, scope)
	facts.Changes = c.collectChanges(ctx, period)
	facts.Assets = c.collectAssets(ctx, period)
	facts.Usage = c.collectUsage(ctx, period)
	facts.Resource = c.collectResource(ctx, period, scope)
	facts.Logs = c.collectLogs(ctx, period, prev, scope)
	facts.Hero = c.buildHero(period, prev, scope, incidents, facts.Actions, facts.Fleet, facts.Resource, facts.Logs, c.countIncidents(ctx, prev, scope))

	c.log.Info("report facts collect done",
		slog.Duration("duration", time.Since(start)),
		slog.Int("incidents", len(facts.Incidents)),
		slog.Int("fleet_total", facts.Fleet.Total),
		slog.Int("fleet_online", facts.Fleet.Online),
		slog.Bool("resource_available", facts.Resource.Available),
		slog.Float64("cpu_avg", facts.Resource.CPUAvg),
		slog.Float64("cpu_peak", facts.Resource.CPUPeak),
		slog.Float64("mem_avg", facts.Resource.MemAvg),
		slog.Float64("mem_peak", facts.Resource.MemPeak),
		slog.Float64("disk_avg", facts.Resource.DiskAvg),
		slog.Float64("disk_peak", facts.Resource.DiskPeak),
		slog.Bool("logs_available", facts.Logs.Available),
		slog.Int("log_errors", facts.Logs.TotalErrors),
	)
	return facts, nil
}

// --- resource trends (Prometheus period avg/peak) ---

// resourceMetricExprs holds the inner PromQL (per-device utilization) and
// subquery-wrapped fleet aggregates used for period avg/peak.
type resourceMetricExprs struct {
	inner    string
	avgExpr  string
	peakExpr string
}

func buildResourceExprs(durStr string, deviceIDs []uint64) map[string]resourceMetricExprs {
	devSel := promDeviceIDSelector(deviceIDs)
	devLabels := promDeviceIDLabels(deviceIDs)
	cpu := `(100 * (1 - avg by (device_id) (rate(node_cpu_seconds_total{mode="idle"` + devSel + `}[5m]))))`
	mem := `(100 * (1 - (sum by (device_id) (node_memory_MemAvailable_bytes{` + devLabels + `}) / sum by (device_id) (node_memory_MemTotal_bytes{` + devLabels + `}))))`
	disk := `(100 - 100 * (sum by (device_id) (node_filesystem_avail_bytes{fstype!~"tmpfs|overlay"` + devSel + `}) / sum by (device_id) (node_filesystem_size_bytes{fstype!~"tmpfs|overlay"` + devSel + `})))`
	mk := func(inner string) resourceMetricExprs {
		return resourceMetricExprs{
			inner:    inner,
			avgExpr:  "avg(avg_over_time(" + inner + "[" + durStr + ":5m]))",
			peakExpr: "max(max_over_time(" + inner + "[" + durStr + ":5m]))",
		}
	}
	return map[string]resourceMetricExprs{
		"cpu":  mk(cpu),
		"mem":  mk(mem),
		"disk": mk(disk),
	}
}

func (c *FactsCollector) collectResource(ctx context.Context, p bizreport.Period, scope bizreport.Scope) bizreport.ResourceFacts {
	start := time.Now()
	var r bizreport.ResourceFacts
	if c.prom == nil {
		c.log.Warn("report resource skipped: prometheus client not configured")
		return r
	}
	if scopedEmpty(scope) {
		c.log.Warn("report resource skipped: system scope matched no devices",
			slog.String("system_name", strings.TrimSpace(scope.SystemName)),
		)
		return r
	}
	deviceIDs := scopedDeviceIDs(scope)
	dur := p.End.Sub(p.Start)
	if dur <= 0 || dur > 30*24*time.Hour {
		dur = 7 * 24 * time.Hour
	}
	durStr := strconv.FormatInt(int64(dur.Hours()), 10) + "h"
	exprs := buildResourceExprs(durStr, deviceIDs)

	c.log.Info("report resource collect start",
		slog.Time("period_start", p.Start),
		slog.Time("period_end", p.End),
		slog.String("subquery_window", durStr),
		slog.Any("device_ids", deviceIDs),
		slog.String("system_name", strings.TrimSpace(scope.SystemName)),
	)

	any := false
	collectMetric := func(key string, e resourceMetricExprs) {
		a, aok := c.scalarAt(ctx, key, "avg_subquery", e.avgExpr, p.End)
		pk, pok := c.scalarAt(ctx, key, "peak_subquery", e.peakExpr, p.End)
		if !aok && !pok {
			c.log.Warn("report resource subquery failed, trying query_range fallback",
				slog.String("metric", key),
				slog.String("inner_expr", truncateLogExpr(e.inner)),
			)
			ra, rp, rok := c.fleetAvgPeakFromRange(ctx, key, e.inner, p.Start, p.End)
			if rok {
				a, pk, aok, pok = ra, rp, true, true
				c.log.Info("report resource query_range fallback ok",
					slog.String("metric", key),
					slog.Float64("avg", ra),
					slog.Float64("peak", rp),
				)
			} else {
				c.log.Warn("report resource query_range fallback failed",
					slog.String("metric", key),
					slog.String("inner_expr", truncateLogExpr(e.inner)),
				)
			}
		}
		if aok || pok {
			any = true
		}
		switch key {
		case "cpu":
			if aok {
				r.CPUAvg = a
			}
			if pok {
				r.CPUPeak = pk
			}
		case "mem":
			if aok {
				r.MemAvg = a
			}
			if pok {
				r.MemPeak = pk
			}
		case "disk":
			if aok {
				r.DiskAvg = a
			}
			if pok {
				r.DiskPeak = pk
			}
		}
		c.log.Info("report resource metric",
			slog.String("metric", key),
			slog.Bool("avg_ok", aok),
			slog.Bool("peak_ok", pok),
			slog.Float64("avg", a),
			slog.Float64("peak", pk),
		)
	}
	for key, e := range exprs {
		collectMetric(key, e)
	}
	r.Available = any
	c.log.Info("report resource collect done",
		slog.Bool("available", r.Available),
		slog.Duration("duration", time.Since(start)),
	)
	return r
}

// fleetAvgPeakFromRange mirrors avg(avg_over_time(...)) / max(max_over_time(...))
// by computing per-series avg/peak over the range matrix, then aggregating
// across device_id series. Used when Prometheus subquery instant queries fail
// (timeout / unsupported) while Monitor-style query_range still works.
func (c *FactsCollector) fleetAvgPeakFromRange(ctx context.Context, metric, innerExpr string, start, end time.Time) (avg, peak float64, ok bool) {
	step := 5 * time.Minute
	c.log.Debug("report resource query_range",
		slog.String("metric", metric),
		slog.Time("start", start),
		slog.Time("end", end),
		slog.String("step", step.String()),
		slog.String("expr", truncateLogExpr(innerExpr)),
	)
	res, err := c.prom.QueryRange(ctx, innerExpr, start, end, step)
	if err != nil {
		c.log.Warn("report resource query_range error",
			slog.String("metric", metric),
			slog.Any("err", err),
			slog.String("expr", truncateLogExpr(innerExpr)),
		)
		return 0, 0, false
	}
	if res == nil {
		c.log.Warn("report resource query_range nil result",
			slog.String("metric", metric),
		)
		return 0, 0, false
	}
	if res.ResultType != "matrix" {
		c.log.Warn("report resource query_range unexpected result type",
			slog.String("metric", metric),
			slog.String("result_type", res.ResultType),
			slog.String("result_preview", truncateLogExpr(string(res.Result))),
		)
		return 0, 0, false
	}
	var series []struct {
		Metric map[string]string   `json:"metric"`
		Values [][]json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(res.Result, &series); err != nil {
		c.log.Warn("report resource query_range decode failed",
			slog.String("metric", metric),
			slog.Any("err", err),
		)
		return 0, 0, false
	}
	if len(series) == 0 {
		c.log.Warn("report resource query_range empty matrix",
			slog.String("metric", metric),
		)
		return 0, 0, false
	}
	var deviceAvgs, devicePeaks []float64
	for _, s := range series {
		var sum float64
		var count int
		var maxV float64
		for _, v := range s.Values {
			f, parsed := decodePromSampleValue(v)
			if !parsed {
				continue
			}
			sum += f
			count++
			if count == 1 || f > maxV {
				maxV = f
			}
		}
		if count == 0 {
			continue
		}
		deviceAvgs = append(deviceAvgs, sum/float64(count))
		devicePeaks = append(devicePeaks, maxV)
	}
	if len(deviceAvgs) == 0 {
		c.log.Warn("report resource query_range no parseable samples",
			slog.String("metric", metric),
			slog.Int("series", len(series)),
		)
		return 0, 0, false
	}
	var avgSum float64
	for _, v := range deviceAvgs {
		avgSum += v
	}
	fleetAvg := avgSum / float64(len(deviceAvgs))
	fleetPeak := devicePeaks[0]
	for _, v := range devicePeaks[1:] {
		if v > fleetPeak {
			fleetPeak = v
		}
	}
	return fleetAvg, fleetPeak, true
}

func decodePromSampleValue(v []json.RawMessage) (float64, bool) {
	if len(v) != 2 {
		return 0, false
	}
	return parseQuotedFloat(v[1])
}

// scalarAt runs an instant query and extracts the single numeric value
// from a scalar or single-element vector result. Returns ok=false on
// error / empty / unparseable so the caller degrades gracefully.
func (c *FactsCollector) scalarAt(ctx context.Context, metric, kind, expr string, ts time.Time) (float64, bool) {
	c.log.Debug("report resource instant query",
		slog.String("metric", metric),
		slog.String("kind", kind),
		slog.Time("at", ts),
		slog.String("expr", truncateLogExpr(expr)),
	)
	res, err := c.prom.Query(ctx, expr, ts)
	if err != nil {
		c.log.Warn("report resource instant query error",
			slog.String("metric", metric),
			slog.String("kind", kind),
			slog.Any("err", err),
			slog.String("expr", truncateLogExpr(expr)),
		)
		return 0, false
	}
	if res == nil {
		c.log.Warn("report resource instant query nil result",
			slog.String("metric", metric),
			slog.String("kind", kind),
		)
		return 0, false
	}
	switch res.ResultType {
	case "scalar":
		var pair []json.RawMessage
		if json.Unmarshal(res.Result, &pair) == nil && len(pair) == 2 {
			if f, ok := parseQuotedFloat(pair[1]); ok {
				return f, true
			}
		}
	case "vector":
		var vec []struct {
			Value []json.RawMessage `json:"value"`
		}
		if json.Unmarshal(res.Result, &vec) == nil && len(vec) > 0 && len(vec[0].Value) == 2 {
			if f, ok := parseQuotedFloat(vec[0].Value[1]); ok {
				return f, true
			}
		}
		if len(vec) == 0 {
			c.log.Warn("report resource instant query empty vector",
				slog.String("metric", metric),
				slog.String("kind", kind),
				slog.String("expr", truncateLogExpr(expr)),
			)
			return 0, false
		}
	}
	c.log.Warn("report resource instant query unparseable",
		slog.String("metric", metric),
		slog.String("kind", kind),
		slog.String("result_type", res.ResultType),
		slog.String("result_preview", truncateLogExpr(string(res.Result))),
	)
	return 0, false
}

func truncateLogExpr(s string) string {
	const max = 280
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func parseQuotedFloat(raw json.RawMessage) (float64, bool) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f != f { // NaN guard
		return 0, false
	}
	if f < 0 {
		f = 0
	}
	return f, true
}

// --- fleet coverage (devices table) ---

func (c *FactsCollector) collectFleet(ctx context.Context, scope bizreport.Scope) bizreport.FleetFacts {
	var f bizreport.FleetFacts
	if scopedEmpty(scope) {
		return f
	}
	type row struct {
		Online bool
		Roles  uint8
	}
	var rows []row
	q := c.db.WithContext(ctx).Table("devices").
		Select("online, roles").
		Where("deleted_at IS NULL")
	if ids := scopedDeviceIDs(scope); len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	if err := q.Find(&rows).Error; err != nil {
		return f
	}
	f.Roles = map[string]int{}
	roleNames := map[uint8]string{1: "server", 2: "storage", 4: "network", 8: "database"}
	for _, r := range rows {
		f.Total++
		if r.Online {
			f.Online++
		}
		matched := false
		for bit, name := range roleNames {
			if r.Roles&bit != 0 {
				f.Roles[name]++
				matched = true
			}
		}
		if !matched {
			f.Roles["unknown"]++
		}
	}
	if len(f.Roles) == 0 {
		f.Roles = nil
	}
	return f
}

// --- change log (audit_logs) ---

// changeActions are the audit actions worth surfacing in a report (the
// product-side mutations), excluding read / login noise.
var changeActions = []string{
	"rule_create", "rule_update", "rule_delete",
	"setting_update", "setting_delete",
	"device_update", "device_delete",
	"channel_create", "channel_update", "channel_delete",
	"repo_create", "repo_delete",
	"user_create", "user_update", "user_delete",
}

func (c *FactsCollector) collectChanges(ctx context.Context, p bizreport.Period) []bizreport.ChangeFact {
	type row struct {
		OccurredAt   time.Time
		Action       string
		ResourceType string
		ResourceName string
		UserEmail    string
	}
	var rows []row
	_ = c.db.WithContext(ctx).
		Table("audit_logs").
		Select("occurred_at, action, resource_type, resource_name, user_email").
		Where("occurred_at >= ? AND occurred_at < ?", p.Start, p.End).
		Where("action IN ?", changeActions).
		Where("status = ?", "success").
		Order("occurred_at DESC").
		Limit(20).
		Find(&rows).Error
	out := make([]bizreport.ChangeFact, 0, len(rows))
	for _, r := range rows {
		out = append(out, bizreport.ChangeFact{
			At:           r.OccurredAt,
			Action:       r.Action,
			ResourceType: r.ResourceType,
			ResourceName: r.ResourceName,
			Actor:        r.UserEmail,
		})
	}
	return out
}

// --- assets created this period (user_agents / installed_skills / knowledge_repos) ---

func (c *FactsCollector) collectAssets(ctx context.Context, p bizreport.Period) bizreport.AssetFacts {
	var a bizreport.AssetFacts
	count := func(table, tsCol string) int {
		var n int64
		_ = c.db.WithContext(ctx).Table(table).
			Where(tsCol+" >= ? AND "+tsCol+" < ?", p.Start, p.End).
			Count(&n).Error
		return int(n)
	}
	a.NewAgents = count("user_agents", "created_at")
	a.NewSkills = count("installed_skills", "installed_at")
	a.NewRepos = count("knowledge_repos", "created_at")
	return a
}

// --- usage (chat sessions + LLM token spend) ---

func (c *FactsCollector) collectUsage(ctx context.Context, p bizreport.Period) bizreport.UsageFacts {
	var u bizreport.UsageFacts
	var sessions int64
	_ = c.db.WithContext(ctx).Table("chat_sessions").
		Where("created_at >= ? AND created_at < ?", p.Start, p.End).
		Where("kind = ?", "user").
		Count(&sessions).Error
	u.Sessions = int(sessions)

	type tokRow struct {
		Prompt     int64
		Completion int64
	}
	var tok tokRow
	_ = c.db.WithContext(ctx).Table("chat_messages").
		Select("COALESCE(SUM(prompt_tokens),0) as prompt, COALESCE(SUM(completion_tokens),0) as completion").
		Where("created_at >= ? AND created_at < ?", p.Start, p.End).
		Scan(&tok).Error
	u.PromptTokens = tok.Prompt
	u.CompletionTokens = tok.Completion
	return u
}

// --- incidents / actions / alerts (unchanged from prior) ---

type incidentRow struct {
	ID           uint64
	RuleName     string
	Severity     string
	Status       string
	DeviceID     *uint64
	FirstFiredAt time.Time
	ResolvedAt   *time.Time
}

func (c *FactsCollector) collectIncidents(ctx context.Context, p bizreport.Period, scope bizreport.Scope) ([]bizreport.IncidentFact, error) {
	q := c.db.WithContext(ctx).
		Table("alert_incidents").
		Select("id, rule_name, severity, status, device_id, first_fired_at, resolved_at").
		Where("deleted_at IS NULL").
		Where("first_fired_at >= ? AND first_fired_at < ?", p.Start, p.End)
	q = applyEdgeScope(q, scope)
	q = applySeverityScope(q, scope)

	var rows []incidentRow
	if err := q.Order("first_fired_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]bizreport.IncidentFact, 0, len(rows))
	for _, r := range rows {
		f := bizreport.IncidentFact{
			ID:          r.ID,
			Title:       r.RuleName,
			Severity:    r.Severity,
			Status:      r.Status,
			DurationMin: durationMinutes(r.FirstFiredAt, r.ResolvedAt, p.End),
		}
		if r.DeviceID != nil {
			f.DeviceID = *r.DeviceID
		}
		out = append(out, f)
	}
	return out, nil
}

func (c *FactsCollector) collectActions(ctx context.Context, p bizreport.Period) bizreport.ActionsSummary {
	var sum bizreport.ActionsSummary
	type propRow struct {
		ToolName string
		Decision string
		Cnt      int
	}
	var props []propRow
	_ = c.db.WithContext(ctx).
		Table("chat_mutating_proposals").
		Select("tool_name, decision, COUNT(*) as cnt").
		Where("created_at >= ? AND created_at < ?", p.Start, p.End).
		Group("tool_name, decision").
		Find(&props).Error
	byTool := map[string]int{}
	for _, pr := range props {
		sum.MutatingTotal += pr.Cnt
		if pr.Decision == "approve" {
			sum.MutatingApproved += pr.Cnt
		}
		byTool[pr.ToolName] += pr.Cnt
	}
	for tool, cnt := range byTool {
		sum.ByTool = append(sum.ByTool, bizreport.ToolCount{Tool: tool, Count: cnt})
	}
	sortToolCounts(sum.ByTool)
	var safe int64
	_ = c.db.WithContext(ctx).Table("audit_logs").
		Where("occurred_at >= ? AND occurred_at < ?", p.Start, p.End).
		Where("status = ?", "success").
		Count(&safe).Error
	sum.SafeTotal = int(safe)
	return sum
}

func (c *FactsCollector) collectAlertCounts(ctx context.Context, p bizreport.Period, scope bizreport.Scope) map[string]int {
	type sevRow struct {
		Severity string
		Cnt      int
	}
	q := c.db.WithContext(ctx).Table("alert_incidents").
		Select("severity, COUNT(*) as cnt").
		Where("deleted_at IS NULL").
		Where("first_fired_at >= ? AND first_fired_at < ?", p.Start, p.End)
	q = applyEdgeScope(q, scope)
	var rows []sevRow
	_ = q.Group("severity").Find(&rows).Error
	out := map[string]int{}
	for _, r := range rows {
		out[r.Severity] = r.Cnt
	}
	return out
}

func (c *FactsCollector) countIncidents(ctx context.Context, p bizreport.Period, scope bizreport.Scope) int {
	q := c.db.WithContext(ctx).Table("alert_incidents").
		Where("deleted_at IS NULL").
		Where("first_fired_at >= ? AND first_fired_at < ?", p.Start, p.End)
	q = applyEdgeScope(q, scope)
	var n int64
	_ = q.Count(&n).Error
	return int(n)
}

// buildHero leads with non-incident signals so calm-period cards still
// carry meaning: devices monitored, fleet CPU/mem avg, disk peak. Falls
// back to incident-count when Prometheus has no resource data.
func (c *FactsCollector) buildHero(p, prev bizreport.Period, scope bizreport.Scope, incidents []bizreport.IncidentFact, actions bizreport.ActionsSummary, fleet bizreport.FleetFacts, res bizreport.ResourceFacts, logs bizreport.LogFacts, prevIncidents int) []bizreport.HeroStat {
	hero := []bizreport.HeroStat{
		{Key: "devices", Label: "监控设备", Value: float64(fleet.Total)},
	}
	if res.Available {
		hero = append(hero,
			bizreport.HeroStat{Key: "cpu_avg", Label: "CPU 均值", Value: round1(res.CPUAvg), Unit: "%"},
			bizreport.HeroStat{Key: "mem_avg", Label: "内存 均值", Value: round1(res.MemAvg), Unit: "%"},
			bizreport.HeroStat{Key: "disk_peak", Label: "磁盘 峰值", Value: round1(res.DiskPeak), Unit: "%"},
		)
	} else if logs.Available {
		hero = append(hero,
			bizreport.HeroStat{Key: "log_errors", Label: "潜在错误", Value: float64(logs.TotalErrors), DeltaPct: logs.DeltaPct, Sparkline: logs.DailySparkline},
			bizreport.HeroStat{Key: "incidents", Label: "Incidents", Value: float64(len(incidents)), DeltaPct: deltaPct(float64(len(incidents)), float64(prevIncidents))},
			bizreport.HeroStat{Key: "actions", Label: "Agent 动作", Value: float64(actions.MutatingTotal + actions.SafeTotal)},
		)
	} else {
		// No prom data — fall back to alert/action counts so the card row
		// isn't half-empty.
		hero = append(hero,
			bizreport.HeroStat{Key: "incidents", Label: "Incidents", Value: float64(len(incidents)), DeltaPct: deltaPct(float64(len(incidents)), float64(prevIncidents))},
			bizreport.HeroStat{Key: "actions", Label: "Agent 动作", Value: float64(actions.MutatingTotal + actions.SafeTotal)},
			bizreport.HeroStat{Key: "online", Label: "在线设备", Value: float64(fleet.Online)},
		)
	}
	return hero
}

// --- scope resolution (system_name → device ids) ---

func (c *FactsCollector) resolveScope(ctx context.Context, scope bizreport.Scope) (bizreport.Scope, error) {
	name := strings.TrimSpace(scope.SystemName)
	if name == "" || len(scope.EdgeIDs) > 0 {
		return scope, nil
	}
	ids, err := c.deviceIDsForSystem(ctx, name)
	if err != nil {
		return scope, err
	}
	scope.EdgeIDs = ids
	return scope, nil
}

func (c *FactsCollector) deviceIDsForSystem(ctx context.Context, name string) ([]uint64, error) {
	type idRow struct {
		ID uint64
	}
	var rows []idRow
	err := c.db.WithContext(ctx).Table("devices").
		Select("id").
		Where("deleted_at IS NULL").
		Where("system_name = ?", name).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]uint64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out, nil
}

func scopedDeviceIDs(scope bizreport.Scope) []uint64 {
	return scope.EdgeIDs
}

func scopedEmpty(scope bizreport.Scope) bool {
	return strings.TrimSpace(scope.SystemName) != "" && len(scope.EdgeIDs) == 0
}

func promDeviceIDSelector(ids []uint64) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(id, 10)
	}
	return ",device_id=~\"" + strings.Join(parts, "|") + "\""
}

func promDeviceIDLabels(ids []uint64) string {
	sel := promDeviceIDSelector(ids)
	return strings.TrimPrefix(sel, ",")
}

// --- helpers ---

func applyEdgeScope(q *gorm.DB, scope bizreport.Scope) *gorm.DB {
	if scopedEmpty(scope) {
		return q.Where("1 = 0")
	}
	if ids := scopedDeviceIDs(scope); len(ids) > 0 {
		return q.Where("device_id IN ?", ids)
	}
	return q
}

func applySeverityScope(q *gorm.DB, scope bizreport.Scope) *gorm.DB {
	if scope.SeverityMin == "" {
		return q
	}
	min, ok := severityRank[scope.SeverityMin]
	if !ok {
		return q
	}
	allowed := make([]string, 0, 3)
	for sev, rank := range severityRank {
		if rank >= min {
			allowed = append(allowed, sev)
		}
	}
	return q.Where("severity IN ?", allowed)
}

func durationMinutes(start time.Time, resolved *time.Time, periodEnd time.Time) int {
	end := periodEnd
	if resolved != nil {
		end = *resolved
	}
	d := end.Sub(start)
	if d < 0 {
		return 0
	}
	return int(d.Minutes())
}

func deltaPct(cur, prev float64) *float64 {
	if prev == 0 {
		return nil
	}
	d := (cur - prev) / prev * 100
	return &d
}

func round1(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10
}

func sortToolCounts(tc []bizreport.ToolCount) {
	for i := 1; i < len(tc); i++ {
		for j := i; j > 0 && tc[j].Count > tc[j-1].Count; j-- {
			tc[j], tc[j-1] = tc[j-1], tc[j]
		}
	}
}
