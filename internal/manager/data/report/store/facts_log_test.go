package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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
				"metric": map[string]string{
					"device_id":     "7",
					"container":     "k8s_order-api_order-api-abc_default_0",
					"ongrid_source": "docker_api",
					"service_name":  "k8s_order-api_order-api-abc_default_0",
				},
				"value": []interface{}{"1", "12"},
			},
		})
		return &logquery.QueryRangeResult{ResultType: "vector", Result: raw}
	}
	sampleStreams := func() *logquery.QueryRangeResult {
		raw, _ := json.Marshal([]map[string]interface{}{
			{
				"stream": map[string]string{"container": "k8s_order-api_order-api-abc_default_0"},
				"values": [][]string{{"1781567980561768822", "level=error msg=test failure"}},
			},
		})
		return &logquery.QueryRangeResult{ResultType: "streams", Result: raw}
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
				if stringsContains(opts.Query, "container=~\".+\"") {
					return topVector(), nil
				}
				return vector(0), nil
			}
			if opts.Limit == 1 && opts.Direction == "backward" && opts.Step == 0 {
				return sampleStreams(), nil
			}
			if opts.Step >= 24*time.Hour {
				if !stringsContains(opts.Query, "device_id=~\"7\"") {
					t.Errorf("sparkline query missing scoped device_id: %s", opts.Query)
				}
				return matrix(), nil
			}
			if stringsContains(opts.Query, "sum(count_over_time") {
				if !stringsContains(opts.Query, "device_id=~\"7\"") {
					t.Errorf("total query missing scoped device_id: %s", opts.Query)
				}
				return vector(42), nil
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
	if len(facts.Logs.TopSources) != 1 {
		t.Fatalf("top sources = %+v", facts.Logs.TopSources)
	}
	top := facts.Logs.TopSources[0]
	if top.Kind != "container" {
		t.Errorf("kind = %q, want container", top.Kind)
	}
	if top.DisplayName != "order-api" {
		t.Errorf("display_name = %q, want order-api", top.DisplayName)
	}
	if top.OngridSource != "docker_api" {
		t.Errorf("ongrid_source = %q, want docker_api", top.OngridSource)
	}
	if top.SampleLine != "level=error msg=test failure" {
		t.Errorf("sample_line = %q", top.SampleLine)
	}
	if facts.Logs.SystemName != "prod-a" {
		t.Errorf("system_name = %q, want prod-a", facts.Logs.SystemName)
	}
}

func TestBuildLogTopSources_PerKindOrder(t *testing.T) {
	entries := []lokiVectorEntry{
		{Labels: map[string]string{"device_id": "1", "unit": "sshd.service", "ongrid_source": "journald"}, Value: 5},
		{Labels: map[string]string{"device_id": "1", "container": "k8s_app_app-1_ns_0", "ongrid_source": "docker_api"}, Value: 100},
		{Labels: map[string]string{"device_id": "1", "ongrid_source": "file:/var/log/app.log"}, Value: 3},
	}
	out := buildLogTopSources(entries, map[uint64]string{1: "host-a"})
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0].Kind != "container" || out[1].Kind != "unit" || out[2].Kind != "file" {
		t.Errorf("order = %s, %s, %s", out[0].Kind, out[1].Kind, out[2].Kind)
	}
	if out[0].OngridSource != "docker_api" {
		t.Errorf("container ongrid_source = %q", out[0].OngridSource)
	}
	if out[2].Name != "/var/log/app.log" {
		t.Errorf("file name = %q", out[2].Name)
	}
}

func TestLogSourceDisplayName(t *testing.T) {
	got := logSourceDisplayName(
		map[string]string{"service_name": "k8s_order-api_order-api-abc_default_0"},
		"container",
		"k8s_order-api_order-api-abc_default_0",
	)
	if got != "order-api" {
		t.Errorf("display_name = %q, want order-api", got)
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
