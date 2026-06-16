package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	bizreport "github.com/ongridio/ongrid/internal/manager/biz/report"
	"github.com/ongridio/ongrid/internal/pkg/logquery"
)

// logErrorLineFilter matches the Logs UI shortcut「潜在错误」.
const logErrorLineFilter = "|~ \"(?i)(error|panic|fatal)\""

// logTopPerKind caps top sources per kind so one noisy exporter cannot
// fill the entire report list.
const logTopPerKind = 3

const logSampleLineMaxLen = 240

// LogQuerier is the narrow Loki surface for report log facts.
type LogQuerier interface {
	QueryRange(ctx context.Context, opts logquery.QueryRangeOptions) (*logquery.QueryRangeResult, error)
}

func (c *FactsCollector) collectLogs(ctx context.Context, period, prev bizreport.Period, scope bizreport.Scope) bizreport.LogFacts {
	out := bizreport.LogFacts{
		QueryPattern: "{ongrid_source=~\".+\"} " + logErrorLineFilter,
	}
	if strings.TrimSpace(scope.SystemName) != "" {
		out.SystemName = strings.TrimSpace(scope.SystemName)
	}
	if c.loki == nil {
		c.log.Warn("report logs skipped: loki client not configured")
		return out
	}
	if scopedEmpty(scope) {
		c.log.Warn("report logs skipped: system scope matched no devices",
			slog.String("system_name", strings.TrimSpace(scope.SystemName)),
		)
		return out
	}

	deviceIDs := scopedDeviceIDs(scope)
	selector := logErrorStreamSelector(deviceIDs)
	out.QueryPattern = selector + " " + logErrorLineFilter

	periodDur := period.End.Sub(period.Start)
	if periodDur < time.Hour {
		periodDur = time.Hour
	}
	countWindow := logErrorCountWindow(periodDur)

	totalExpr := logErrorCountExpr(selector, countWindow)
	prevDur := prev.End.Sub(prev.Start)
	if prevDur < time.Hour {
		prevDur = time.Hour
	}
	prevExpr := logErrorCountExpr(selector, logErrorCountWindow(prevDur))

	start := time.Now()
	total, err := c.lokiScalarAt(ctx, totalExpr, period.End)
	if err != nil {
		c.log.Warn("report logs total query failed", slog.Any("err", err), slog.String("expr", truncateLogExpr(totalExpr)))
		return out
	}
	prevTotal, perr := c.lokiScalarAt(ctx, prevExpr, prev.End)
	if perr != nil {
		c.log.Warn("report logs prev total query failed", slog.Any("err", perr))
		prevTotal = 0
	}

	sparkStep := logSparklineStep(periodDur)
	sparkWindow := formatLogQLDuration(sparkStep)
	sparkExpr := logErrorCountExpr(selector, sparkWindow)
	sparkline, serr := c.lokiSparkline(ctx, sparkExpr, period.Start, period.End, sparkStep)
	if serr != nil {
		c.log.Warn("report logs sparkline query failed", slog.Any("err", serr))
	}

	topSources, terr := c.collectLogTopSources(ctx, selector, countWindow, period)
	if terr != nil {
		c.log.Warn("report logs top sources query failed", slog.Any("err", terr))
	}

	out.Available = true
	out.TotalErrors = int(total)
	out.PrevTotalErrors = int(prevTotal)
	out.DeltaPct = deltaPct(float64(out.TotalErrors), float64(out.PrevTotalErrors))
	out.DailySparkline = sparkline
	out.TopSources = topSources

	c.log.Info("report logs collect done",
		slog.Duration("duration", time.Since(start)),
		slog.Int("total_errors", out.TotalErrors),
		slog.Int("prev_total_errors", out.PrevTotalErrors),
		slog.Int("sparkline_points", len(out.DailySparkline)),
		slog.Int("top_sources", len(out.TopSources)),
		slog.String("system_name", out.SystemName),
	)
	return out
}

func logErrorStreamSelector(deviceIDs []uint64) string {
	if len(deviceIDs) == 0 {
		return "{ongrid_source=~\".+\"}"
	}
	return fmt.Sprintf("{device_id=~\"%s\",ongrid_source=~\".+\"}", joinDeviceIDs(deviceIDs))
}

func joinDeviceIDs(ids []uint64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(id, 10)
	}
	return strings.Join(parts, "|")
}

func logErrorCountExpr(selector, window string) string {
	return fmt.Sprintf(`sum(count_over_time(%s %s [%s]))`, selector, logErrorLineFilter, window)
}

func logErrorTopByKindExpr(selector, window string, topN int) string {
	inner := fmt.Sprintf(
		`sum by (device_id, container, unit, ongrid_source, service_name) (count_over_time(%s %s [%s]))`,
		selector, logErrorLineFilter, window,
	)
	return fmt.Sprintf(`topk(%d, %s)`, topN, inner)
}

func logErrorKindSelector(baseSelector, kind string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(baseSelector), "{"), "}")
	switch kind {
	case "container":
		return fmt.Sprintf("{%s,container=~\".+\"}", inner)
	case "unit":
		return fmt.Sprintf("{%s,unit=~\".+\",container!~\".+\"}", inner)
	case "file":
		return fmt.Sprintf("{%s,ongrid_source=~\"file:.*\"}", inner)
	case "other":
		// Streams with ongrid_source but no container/unit labels (e.g. legacy sources).
		return fmt.Sprintf("{%s,ongrid_source=~\".+\",container!~\".+\",unit!~\".+\",ongrid_source!~\"file:.*\"}", inner)
	default:
		return baseSelector
	}
}

func (c *FactsCollector) collectLogTopSources(ctx context.Context, baseSelector, window string, period bizreport.Period) ([]bizreport.LogErrorSource, error) {
	kinds := []string{"container", "unit", "file", "other"}
	var entries []lokiVectorEntry
	for _, kind := range kinds {
		sel := logErrorKindSelector(baseSelector, kind)
		expr := logErrorTopByKindExpr(sel, window, logTopPerKind)
		part, err := c.lokiVectorAt(ctx, expr, period.End)
		if err != nil {
			return nil, err
		}
		entries = append(entries, part...)
	}
	nameByID := c.deviceNamesForLogEntries(ctx, entries)
	built := buildLogTopSources(entries, nameByID)
	return c.enrichLogSourcesWithSamples(ctx, period, built), nil
}

// logErrorCountWindow is the lookback for a single count_over_time covering
// the whole report period. Loki does not support Prom-style
// sum_over_time(count_over_time(...)[range:step]) subqueries.
func logErrorCountWindow(periodDur time.Duration) string {
	periodDur = periodDur.Round(time.Hour)
	if periodDur < time.Hour {
		periodDur = time.Hour
	}
	const max = 31 * 24 * time.Hour
	if periodDur > max {
		periodDur = max
	}
	return formatLogQLDuration(periodDur)
}

func logSparklineStep(periodDur time.Duration) time.Duration {
	if periodDur <= 48*time.Hour {
		return time.Hour
	}
	return 24 * time.Hour
}

func formatLogQLDuration(d time.Duration) string {
	if d >= 24*time.Hour && d%24*time.Hour == 0 {
		return fmt.Sprintf("%dh", d/(24*time.Hour))
	}
	return d.String()
}

func (c *FactsCollector) lokiScalarAt(ctx context.Context, expr string, at time.Time) (float64, error) {
	entries, err := c.lokiVectorAt(ctx, expr, at)
	if err != nil {
		return 0, err
	}
	var sum float64
	for _, e := range entries {
		sum += e.Value
	}
	return sum, nil
}

func (c *FactsCollector) lokiVectorAt(ctx context.Context, expr string, at time.Time) ([]lokiVectorEntry, error) {
	res, err := c.loki.QueryRange(ctx, logquery.QueryRangeOptions{
		Query: expr,
		Start: at.Add(-60 * time.Second),
		End:   at,
		Step:  30 * time.Second,
		Limit: 5000,
	})
	if err != nil {
		return nil, err
	}
	return decodeLokiVector(res)
}

func (c *FactsCollector) lokiSparkline(ctx context.Context, expr string, start, end time.Time, step time.Duration) ([]int, error) {
	res, err := c.loki.QueryRange(ctx, logquery.QueryRangeOptions{
		Query: expr,
		Start: start,
		End:   end,
		Step:  step,
		Limit: 5000,
	})
	if err != nil {
		return nil, err
	}
	points, err := decodeLokiMatrixSumByTime(res)
	if err != nil {
		return nil, err
	}
	out := make([]int, len(points))
	for i, v := range points {
		out[i] = int(v)
	}
	return out, nil
}

type lokiVectorEntry struct {
	Labels map[string]string
	Value  float64
}

func decodeLokiVector(res *logquery.QueryRangeResult) ([]lokiVectorEntry, error) {
	if res == nil {
		return nil, nil
	}
	switch res.ResultType {
	case "vector":
		type promEntry struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage   `json:"value"`
		}
		var raw []promEntry
		if err := json.Unmarshal(res.Result, &raw); err != nil {
			return nil, fmt.Errorf("decode loki vector: %w", err)
		}
		out := make([]lokiVectorEntry, 0, len(raw))
		for _, ent := range raw {
			valStr := ""
			if len(ent.Value) >= 2 {
				_ = json.Unmarshal(ent.Value[1], &valStr)
			}
			v, _ := strconv.ParseFloat(strings.Trim(valStr, `"`), 64)
			out = append(out, lokiVectorEntry{Labels: ent.Metric, Value: v})
		}
		return out, nil
	case "matrix":
		type lokiEntry struct {
			Metric map[string]string   `json:"metric"`
			Values [][]json.RawMessage `json:"values"`
		}
		var raw []lokiEntry
		if err := json.Unmarshal(res.Result, &raw); err != nil {
			return nil, fmt.Errorf("decode loki matrix as vector: %w", err)
		}
		out := make([]lokiVectorEntry, 0, len(raw))
		for _, s := range raw {
			if len(s.Values) == 0 {
				continue
			}
			last := s.Values[len(s.Values)-1]
			if len(last) < 2 {
				continue
			}
			var valStr string
			_ = json.Unmarshal(last[1], &valStr)
			v, _ := strconv.ParseFloat(strings.Trim(valStr, `"`), 64)
			out = append(out, lokiVectorEntry{Labels: s.Metric, Value: v})
		}
		return out, nil
	default:
		return nil, nil
	}
}

func decodeLokiMatrixSumByTime(res *logquery.QueryRangeResult) ([]float64, error) {
	if res == nil || res.ResultType != "matrix" {
		return nil, nil
	}
	type lokiEntry struct {
		Values [][]json.RawMessage `json:"values"`
	}
	var raw []lokiEntry
	if err := json.Unmarshal(res.Result, &raw); err != nil {
		return nil, fmt.Errorf("decode loki matrix: %w", err)
	}
	timeSums := make(map[int64]float64)
	for _, s := range raw {
		for _, pt := range s.Values {
			if len(pt) < 2 {
				continue
			}
			var tsStr, valStr string
			_ = json.Unmarshal(pt[0], &tsStr)
			_ = json.Unmarshal(pt[1], &valStr)
			ts, _ := strconv.ParseInt(tsStr, 10, 64)
			v, _ := strconv.ParseFloat(strings.Trim(valStr, `"`), 64)
			timeSums[ts] += v
		}
	}
	if len(timeSums) == 0 {
		return nil, nil
	}
	times := make([]int64, 0, len(timeSums))
	for ts := range timeSums {
		times = append(times, ts)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	out := make([]float64, len(times))
	for i, ts := range times {
		out[i] = timeSums[ts]
	}
	return out, nil
}

func buildLogTopSources(entries []lokiVectorEntry, nameByID map[uint64]string) []bizreport.LogErrorSource {
	out := make([]bizreport.LogErrorSource, 0, len(entries))
	for _, e := range entries {
		if e.Value <= 0 {
			continue
		}
		kind, name := logSourceKindName(e.Labels)
		if name == "" {
			continue
		}
		var deviceID uint64
		if idStr := strings.TrimSpace(e.Labels["device_id"]); idStr != "" {
			deviceID, _ = strconv.ParseUint(idStr, 10, 64)
		}
		ongridSource := strings.TrimSpace(e.Labels["ongrid_source"])
		out = append(out, bizreport.LogErrorSource{
			DeviceID:     deviceID,
			DeviceName:   nameByID[deviceID],
			Kind:         kind,
			Name:         name,
			DisplayName:  logSourceDisplayName(e.Labels, kind, name),
			OngridSource: ongridSource,
			Count:        int(e.Value),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return logSourceKindOrder(out[i].Kind) < logSourceKindOrder(out[j].Kind)
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func logSourceKindOrder(kind string) int {
	switch kind {
	case "container":
		return 0
	case "unit":
		return 1
	case "file":
		return 2
	default:
		return 3
	}
}

func logSourceKindName(labels map[string]string) (kind, name string) {
	if c := strings.TrimSpace(labels["container"]); c != "" {
		return "container", c
	}
	if u := strings.TrimSpace(labels["unit"]); u != "" {
		return "unit", u
	}
	if src := strings.TrimSpace(labels["ongrid_source"]); src != "" {
		if strings.HasPrefix(src, "file:") {
			return "file", strings.TrimPrefix(src, "file:")
		}
		return "other", src
	}
	return "other", "unknown"
}

func logSourceDisplayName(labels map[string]string, kind, canonical string) string {
	svc := strings.TrimSpace(labels["service_name"])
	if svc != "" && svc != "unknown_service" {
		if short := shortenK8sLogName(svc); short != "" && short != canonical {
			return short
		}
		if svc != canonical {
			return svc
		}
	}
	if kind == "container" {
		if short := shortenK8sLogName(canonical); short != "" {
			return short
		}
	}
	return canonical
}

func shortenK8sLogName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "k8s_") {
		parts := strings.SplitN(s, "_", 3)
		if len(parts) >= 2 && parts[1] != "" {
			return truncateLogSample(parts[1], 64)
		}
	}
	if len(s) > 64 {
		return s[:64] + "…"
	}
	return s
}

func (c *FactsCollector) enrichLogSourcesWithSamples(ctx context.Context, period bizreport.Period, sources []bizreport.LogErrorSource) []bizreport.LogErrorSource {
	if len(sources) == 0 {
		return sources
	}
	for i := range sources {
		labels := logLabelsForSource(sources[i])
		if len(labels) == 0 {
			continue
		}
		line, err := c.lokiSampleLine(ctx, logStreamSelector(labels), period.Start, period.End)
		if err != nil {
			c.log.Warn("report logs sample query failed",
				slog.Any("err", err),
				slog.String("kind", sources[i].Kind),
				slog.String("name", sources[i].Name),
			)
			continue
		}
		sources[i].SampleLine = line
	}
	return sources
}

func logLabelsForSource(s bizreport.LogErrorSource) map[string]string {
	labels := make(map[string]string)
	if s.DeviceID != 0 {
		labels["device_id"] = strconv.FormatUint(s.DeviceID, 10)
	}
	switch s.Kind {
	case "container":
		labels["container"] = s.Name
	case "unit":
		labels["unit"] = s.Name
	}
	if src := strings.TrimSpace(s.OngridSource); src != "" {
		labels["ongrid_source"] = src
	} else if s.Kind == "file" {
		labels["ongrid_source"] = "file:" + s.Name
	}
	return labels
}

func logStreamSelector(labels map[string]string) string {
	var pairs []string
	if id := strings.TrimSpace(labels["device_id"]); id != "" {
		pairs = append(pairs, fmt.Sprintf("device_id=\"%s\"", escapeLogQLLabel(id)))
	}
	if c := strings.TrimSpace(labels["container"]); c != "" {
		pairs = append(pairs, fmt.Sprintf("container=\"%s\"", escapeLogQLLabel(c)))
	}
	if u := strings.TrimSpace(labels["unit"]); u != "" {
		pairs = append(pairs, fmt.Sprintf("unit=\"%s\"", escapeLogQLLabel(u)))
	}
	if src := strings.TrimSpace(labels["ongrid_source"]); src != "" {
		pairs = append(pairs, fmt.Sprintf("ongrid_source=\"%s\"", escapeLogQLLabel(src)))
	}
	if len(pairs) == 0 {
		return "{ongrid_source=~\".+\"}"
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

func escapeLogQLLabel(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "\"", "\\\"")
}

func (c *FactsCollector) lokiSampleLine(ctx context.Context, selector string, start, end time.Time) (string, error) {
	res, err := c.loki.QueryRange(ctx, logquery.QueryRangeOptions{
		Query:     selector + " " + logErrorLineFilter,
		Start:     start,
		End:       end,
		Limit:     1,
		Direction: "backward",
	})
	if err != nil {
		return "", err
	}
	return decodeLokiFirstLine(res), nil
}

func decodeLokiFirstLine(res *logquery.QueryRangeResult) string {
	if res == nil || res.ResultType != "streams" {
		return ""
	}
	type stream struct {
		Values [][]json.RawMessage `json:"values"`
	}
	var raw []stream
	if err := json.Unmarshal(res.Result, &raw); err != nil || len(raw) == 0 {
		return ""
	}
	for _, s := range raw {
		if len(s.Values) == 0 {
			continue
		}
		last := s.Values[0]
		if len(last) < 2 {
			continue
		}
		var line string
		if err := json.Unmarshal(last[1], &line); err != nil {
			continue
		}
		return truncateLogSample(strings.TrimSpace(line), logSampleLineMaxLen)
	}
	return ""
}

func truncateLogSample(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func (c *FactsCollector) deviceNamesForLogEntries(ctx context.Context, entries []lokiVectorEntry) map[uint64]string {
	seen := make(map[uint64]struct{})
	var ids []uint64
	for _, e := range entries {
		idStr := strings.TrimSpace(e.Labels["device_id"])
		if idStr == "" {
			continue
		}
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return c.deviceNames(ctx, ids)
}

func (c *FactsCollector) deviceNames(ctx context.Context, ids []uint64) map[uint64]string {
	out := make(map[uint64]string)
	if len(ids) == 0 {
		return out
	}
	type row struct {
		ID   uint64
		Name string
	}
	var rows []row
	err := c.db.WithContext(ctx).Table("devices").
		Select("id, name").
		Where("id IN ?", ids).
		Find(&rows).Error
	if err != nil {
		return out
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out
}
