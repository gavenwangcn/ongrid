package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	bizreport "github.com/ongridio/ongrid/internal/manager/biz/report"
	"github.com/ongridio/ongrid/internal/pkg/logquery"
)

type fakeReportLoki struct {
	queryRangeFn func(ctx context.Context, opts logquery.QueryRangeOptions) (*logquery.QueryRangeResult, error)
}

func (f *fakeReportLoki) QueryRange(ctx context.Context, opts logquery.QueryRangeOptions) (*logquery.QueryRangeResult, error) {
	if f.queryRangeFn != nil {
		return f.queryRangeFn(ctx, opts)
	}
	return nil, nil
}

func TestFactsCollector_Logs_SystemScope(t *testing.T) {
	db := newFactsDB(t)
	ctx := context.Background()
	period := bizreport.Period{
		Start: mustParse(t, "2026-06-01T00:00:00Z"),
		End:   mustParse(t, "2026-06-08T00:00:00Z"),
	}
	prev := bizreport.Period{
		Start: mustParse(t, "2026-05-25T00:00:00Z"),
		End:   mustParse(t, "2026-06-01T00:00:00Z"),
	}
	db.Exec(`INSERT INTO devices VALUES (7, true, 1, 'prod-a', NULL), (9, false, 2, 'prod-b', NULL)`)

	vector := func(v float64) *logquery.QueryRangeResult {
		raw, _ := json.Marshal([]map[string]interface{}{
			{"metric": map[string]string{}, "value": []interface{}{"1", fmtFloat(v)}},
		})
		return &logquery.QueryRangeResult{ResultType: "vector", Result: raw}
	}
	topVector := func() *logquery.QueryRangeResult {
		raw, _ := json.Marshal([]map[string]interface{}{
			{
				"metric": map[string]string{"device_id": "7", "container": "order-api"},
				"value":  []interface{}{"1", "12"},
			},
		})
		return &logquery.QueryRangeResult{ResultType: "vector", Result: raw}
	}
	matrix := func() *logquery.QueryRangeResult {
		raw, _ := json.Marshal([]map[string]interface{}{
			{"metric": map[string]string{}, "values": [][]interface{}{{"1", "3"}, {"2", "5"}}},
		})
		return &logquery.QueryRangeResult{ResultType: "matrix", Result: raw}
	}

	loki := &fakeReportLoki{
		queryRangeFn: func(_ context.Context, opts logquery.QueryRangeOptions) (*logquery.QueryRangeResult, error) {
			if stringsContains(opts.Query, "topk") {
				if !stringsContains(opts.Query, "device_id=~\"7\"") {
					t.Errorf("topk query missing scoped device_id: %s", opts.Query)
				}
				return topVector(), nil
			}
			if stringsContains(opts.Query, "sum(sum_over_time") {
				if !stringsContains(opts.Query, "device_id=~\"7\"") {
					t.Errorf("total query missing scoped device_id: %s", opts.Query)
				}
				return vector(42), nil
			}
			if stringsContains(opts.Query, "sum(count_over_time") {
				if !stringsContains(opts.Query, "device_id=~\"7\"") {
					t.Errorf("sparkline query missing scoped device_id: %s", opts.Query)
				}
				return matrix(), nil
			}
			return vector(0), nil
		},
	}

	fc := NewFactsCollector(db, nil, loki, nil)
	facts, err := fc.Collect(ctx, period, prev, bizreport.Scope{SystemName: "prod-a"})
	if err != nil {
		t.Fatal(err)
	}
	if !facts.Logs.Available {
		t.Fatalf("logs.available = false, want true")
	}
	if facts.Logs.TotalErrors != 42 {
		t.Errorf("total_errors = %d, want 42", facts.Logs.TotalErrors)
	}
	if len(facts.Logs.DailySparkline) != 2 {
		t.Errorf("sparkline = %v, want 2 points", facts.Logs.DailySparkline)
	}
	if len(facts.Logs.TopSources) != 1 || facts.Logs.TopSources[0].Name != "order-api" {
		t.Errorf("top sources = %+v", facts.Logs.TopSources)
	}
	if facts.Logs.SystemName != "prod-a" {
		t.Errorf("system_name = %q, want prod-a", facts.Logs.SystemName)
	}
}

func fmtFloat(v float64) string {
	return fmt.Sprintf("%g", v)
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
