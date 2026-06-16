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
	bucket, window := logErrorAggWindow(periodDur)

	totalExpr := fmt.Sprintf(
		`sum(sum_over_time(count_over_time(%s %s [%s])[%s:%s]))`,
		selector, logErrorLineFilter, bucket, window, bucket,
	)
	prevDur := prev.End.Sub(prev.Start)
	if prevDur < time.Hour {
		prevDur = time.Hour
	}
	prevBucket, prevWindow := logErrorAggWindow(prevDur)
	prevExpr := fmt.Sprintf(
		`sum(sum_over_time(count_over_time(%s %s [%s])[%s:%s]))`,
		selector, logErrorLineFilter, prevBucket, prevWindow, prevBucket,
	)

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
	sparkExpr := fmt.Sprintf(`sum(count_over_time(%s %s [%s]))`, selector, logErrorLineFilter, sparkWindow)
	sparkline, serr := c.lokiSparkline(ctx, sparkExpr, period.Start, period.End, sparkStep)
	if serr != nil {
		c.log.Warn("report logs sparkline query failed", slog.Any("err", serr))
	}

	topExpr := fmt.Sprintf(
		`topk(10, sum by (device_id, container, unit, ongrid_source) (sum_over_time(count_over_time(%s %s [1h])[%s:1h])))`,
		selector, logErrorLineFilter, window,
	)
	topEntries, terr := c.lokiVectorAt(ctx, topExpr, period.End)
	if terr != nil {
		c.log.Warn("report logs top sources query failed", slog.Any("err", terr))
	}

	nameByID := c.deviceNamesForLogEntries(ctx, topEntries)
	topSources := buildLogTopSources(topEntries, nameByID)

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

func logErrorAggWindow(periodDur time.Duration) (bucket, window string) {
	periodDur = periodDur.Round(time.Hour)
	if periodDur < time.Hour {
		periodDur = time.Hour
	}
	switch {
	case periodDur <= 48*time.Hour:
		return "1h", formatLogQLDuration(periodDur)
	case periodDur <= 8*24*time.Hour:
		b := 6 * time.Hour
		if periodDur%b != 0 {
			b = time.Hour
		}
		return formatLogQLDuration(b), formatLogQLDuration(periodDur)
	default:
		return "24h", formatLogQLDuration(periodDur)
	}
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
		out = append(out, bizreport.LogErrorSource{
			DeviceID:   deviceID,
			DeviceName: nameByID[deviceID],
			Kind:       kind,
			Name:       name,
			Count:      int(e.Value),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
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
