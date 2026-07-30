package report

import (
	"context"
	"strings"
	"testing"
	"time"

	model "github.com/ongridio/ongrid/internal/manager/model/report"
)

func TestMarkdownSummary(t *testing.T) {
	d := -12.0
	s := DeliverySummary{
		Title:    "周报 · 2026 W23",
		Headline: "本周整体平稳，主要风险在 {{entity:edge:7|db-prod-3}}",
		Hero: []HeroStat{
			{Key: "incidents", Label: "Incidents", Value: 23, DeltaPct: &d},
			{Key: "mttr", Label: "MTTR", Value: 47, Unit: "min"},
		},
		DeepLink: "https://ongrid.example/reports/abc",
	}
	md := s.MarkdownSummary()
	if strings.Contains(md, "**周报") {
		t.Errorf("title should not repeat in body (subject carries it):\n%s", md)
	}
	if !strings.Contains(md, "Incidents 23 ↓12%") {
		t.Errorf("hero fallback line wrong:\n%s", md)
	}
	if !strings.Contains(md, "MTTR 47min") {
		t.Errorf("mttr missing:\n%s", md)
	}
	// Entity token flattened in the IM summary.
	if strings.Contains(md, "{{entity") {
		t.Errorf("entity not flattened:\n%s", md)
	}
	if !strings.Contains(md, "db-prod-3") {
		t.Errorf("entity name lost:\n%s", md)
	}
	if !strings.Contains(md, "https://ongrid.example/reports/abc") {
		t.Errorf("deep link missing:\n%s", md)
	}
}

func TestMarkdownSummaryClusterPosture(t *testing.T) {
	s := DeliverySummary{
		Headline: "采购管理平台本日资源整体处于中等负载水位。",
		Sections: allReportSections,
		Resource: ResourceFacts{
			Available:    true,
			CPUAvg:       6.1,
			CPUPeak:      79.7,
			MemAvg:       31.4,
			MemPeak:      49.6,
			DiskAvg:      20.8,
			DiskPeak:     24.1,
			NetRxAvgBps:  115.2 * 1024,
			NetTxAvgBps:  277.2 * 1024,
			NetRxPeakBps: 600 * 1024,
			NetTxPeakBps: 900 * 1024,
		},
		Fleet: FleetFacts{Total: 5, Online: 5},
		Logs: LogFacts{
			Available:       true,
			TotalErrors:     42,
			PrevTotalErrors: 30,
			DeltaPct:        ptrFloat64(40),
		},
		Incidents: []KeyIncident{
			{ID: 1, Status: "resolved", DurationMin: 30},
			{ID: 2, Status: "open", DurationMin: 10},
		},
		Actions: ActionsSummary{MutatingTotal: 2, SafeTotal: 3},
		DeepLink: "http://10.1.1.41:30088/r/tok",
	}
	md := s.MarkdownSummary()
	for _, want := range []string{
		"采购管理平台本日资源整体处于中等负载水位",
		"**集群态势**",
		"CPU · 均 6.1% / 峰 79.7%",
		"在线设备 · 5 / 共 5 台",
		"**应用日志**",
		"潜在错误 · 42",
		"较上周期 +40%",
		"**告警与处理**",
		"告警 · 2 · 已解决 1 · MTTR 30 min",
		"Agent 动作 5",
		"查看完整报告",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestMarkdownSummaryRespectsSections(t *testing.T) {
	s := DeliverySummary{
		Headline: "仅集群",
		Sections: []string{SectionCluster},
		Resource: ResourceFacts{Available: true, CPUAvg: 1, CPUPeak: 2},
		Fleet:    FleetFacts{Total: 1, Online: 1},
		Logs:     LogFacts{Available: true, TotalErrors: 99},
		Incidents: []KeyIncident{{ID: 1}},
	}
	md := s.MarkdownSummary()
	if !strings.Contains(md, "集群态势") {
		t.Fatalf("expected cluster block:\n%s", md)
	}
	if strings.Contains(md, "应用日志") || strings.Contains(md, "告警与处理") {
		t.Fatalf("logs/alerts should be omitted when not in sections:\n%s", md)
	}
}

func ptrFloat64(v float64) *float64 { return &v }

// recordingDeliverer captures what it was asked to deliver.
type recordingDeliverer struct {
	gotSummary  DeliverySummary
	gotChannels []uint64
	called      bool
}

func (d *recordingDeliverer) Deliver(_ context.Context, s DeliverySummary, ids []uint64) []DeliveryRecord {
	d.called = true
	d.gotSummary = s
	d.gotChannels = ids
	recs := make([]DeliveryRecord, 0, len(ids))
	for _, id := range ids {
		recs = append(recs, DeliveryRecord{ChannelID: id, Status: "sent", SentAt: time.Now()})
	}
	return recs
}

func TestDeliverySummaryExcludesDeviceDetail(t *testing.T) {
	s := DeliverySummary{
		Title:     "周报",
		Headline:  "本周资源平稳",
		Sections:  allReportSections,
		Resource:  ResourceFacts{Available: true, CPUAvg: 12, CPUPeak: 20},
		Fleet:     FleetFacts{Total: 3, Online: 3},
		Logs:      LogFacts{Available: true, TotalErrors: 5},
		Incidents: []KeyIncident{{ID: 1, Status: "open", DurationMin: 5}},
		DeepLink:  "https://h/r/tok",
	}
	md := s.MarkdownSummary()
	if strings.Contains(md, "device_resources") || strings.Contains(md, "设备资源明细") {
		t.Errorf("IM summary must not include per-device detail:\n%s", md)
	}
	if strings.Contains(md, "Top error") || strings.Contains(md, "错误来源") {
		t.Errorf("IM summary must not include log source detail:\n%s", md)
	}
	for _, want := range []string{"集群态势", "应用日志", "告警与处理"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected section %q in:\n%s", want, md)
		}
	}
}

func TestGenerator_DeliversReadyReportToChannels(t *testing.T) {
	rpt := pendingReport()
	sid := uint64(1)
	rpt.ScheduleID = &sid
	repo := newGenTestRepo(rpt)
	// Owning schedule with two channels.
	repo.schedules[1] = &model.ReportSchedule{
		ID: 1, CreatedBy: 42, Kind: model.KindWeekly, Timezone: "UTC",
		ChannelIDsJSON: "[12,7]",
	}
	llmOut := `{"version":"1","hero":[],"narrative":{"headline":"本周平稳"},"actions_summary":{}}`
	deliverer := &recordingDeliverer{}
	gen := NewWorkerGenerator(repo, fakeFacts{facts: sampleFacts()}, &fakeSpawner{result: llmOut}, GeneratorConfig{PublicURL: "https://h"}, nil).
		WithDeliverer(deliverer)

	gen.Generate(context.Background(), rpt.ID)

	if !deliverer.called {
		t.Fatal("deliverer not called for ready report")
	}
	if len(deliverer.gotChannels) != 2 || deliverer.gotChannels[0] != 12 {
		t.Errorf("channels = %v, want [12 7]", deliverer.gotChannels)
	}
	if !strings.HasPrefix(deliverer.gotSummary.DeepLink, "https://h/r/") {
		t.Errorf("deep link = %q, want public share link under /r/", deliverer.gotSummary.DeepLink)
	}
	// Delivery records persisted.
	got, _ := repo.GetReport(context.Background(), rpt.ID)
	if got.ShareToken == nil || *got.ShareToken == "" {
		t.Fatal("share token should be minted for delivery")
	}
	if deliverer.gotSummary.DeepLink != "https://h/r/"+*got.ShareToken {
		t.Errorf("deep link = %q, want token %q", deliverer.gotSummary.DeepLink, *got.ShareToken)
	}
	if got.DeliveryJSON == "" || !strings.Contains(got.DeliveryJSON, `"status":"sent"`) {
		t.Errorf("delivery_json not persisted: %q", got.DeliveryJSON)
	}
}

func TestGenerator_NoDeliveryWhenNoChannels(t *testing.T) {
	rpt := pendingReport()
	sid := uint64(1)
	rpt.ScheduleID = &sid
	repo := newGenTestRepo(rpt)
	repo.schedules[1] = &model.ReportSchedule{ID: 1, Kind: model.KindWeekly, Timezone: "UTC", ChannelIDsJSON: "[]"}
	deliverer := &recordingDeliverer{}
	gen := NewWorkerGenerator(repo, fakeFacts{facts: sampleFacts()},
		&fakeSpawner{result: `{"version":"1","narrative":{"headline":"ok"}}`}, GeneratorConfig{}, nil).
		WithDeliverer(deliverer)

	gen.Generate(context.Background(), rpt.ID)
	if deliverer.called {
		t.Error("deliverer should not be called when no channels configured")
	}
}

func TestGenerator_NoDeliveryForManualReport(t *testing.T) {
	rpt := pendingReport() // ScheduleID nil, no oneoff task ref
	repo := newGenTestRepo(rpt)
	deliverer := &recordingDeliverer{}
	gen := NewWorkerGenerator(repo, fakeFacts{facts: sampleFacts()},
		&fakeSpawner{result: `{"version":"1","narrative":{"headline":"ok"}}`}, GeneratorConfig{}, nil).
		WithDeliverer(deliverer)

	gen.Generate(context.Background(), rpt.ID)
	if deliverer.called {
		t.Error("ad-hoc report without oneoff task should not deliver")
	}
}

func TestGenerator_DeliversOneoffTaskReport(t *testing.T) {
	rpt := pendingReport()
	rpt.TaskID = "oneoff:task-1"
	repo := newGenTestRepo(rpt)
	repo.tasks["task-1"] = &model.Task{
		ID:             "task-1",
		Kind:           model.TaskKindOneoff,
		ChannelIDsJSON: "[12,7]",
	}
	deliverer := &recordingDeliverer{}
	gen := NewWorkerGenerator(repo, fakeFacts{facts: sampleFacts()},
		&fakeSpawner{result: `{"version":"1","narrative":{"headline":"ok"}}`}, GeneratorConfig{PublicURL: "https://h"}, nil).
		WithDeliverer(deliverer)

	gen.Generate(context.Background(), rpt.ID)
	if !deliverer.called {
		t.Fatal("oneoff task report with channels should deliver")
	}
	if len(deliverer.gotChannels) != 2 || deliverer.gotChannels[0] != 12 {
		t.Errorf("channels = %v, want [12 7]", deliverer.gotChannels)
	}
}

func TestGenerator_DeliversRunNowScheduleReport(t *testing.T) {
	rpt := pendingReport()
	rpt.TaskID = "report-schedule:1" // run-now: schedule_id nil, task_id stamped
	repo := newGenTestRepo(rpt)
	repo.schedules[1] = &model.ReportSchedule{
		ID: 1, CreatedBy: 42, Kind: model.KindWeekly, Timezone: "UTC",
		ChannelIDsJSON: "[12]",
	}
	deliverer := &recordingDeliverer{}
	gen := NewWorkerGenerator(repo, fakeFacts{facts: sampleFacts()},
		&fakeSpawner{result: `{"version":"1","narrative":{"headline":"ok"}}`}, GeneratorConfig{}, nil).
		WithDeliverer(deliverer)

	gen.Generate(context.Background(), rpt.ID)
	if !deliverer.called {
		t.Fatal("run-now schedule report should deliver via task_id back-ref")
	}
	if len(deliverer.gotChannels) != 1 || deliverer.gotChannels[0] != 12 {
		t.Errorf("channels = %v, want [12]", deliverer.gotChannels)
	}
}

func TestGenerator_NoDeliveryForFailedReport(t *testing.T) {
	rpt := pendingReport()
	sid := uint64(1)
	rpt.ScheduleID = &sid
	repo := newGenTestRepo(rpt)
	repo.schedules[1] = &model.ReportSchedule{ID: 1, Kind: model.KindWeekly, Timezone: "UTC", ChannelIDsJSON: "[12]"}
	deliverer := &recordingDeliverer{}
	// Spawn error → failed report.
	gen := NewWorkerGenerator(repo, fakeFacts{facts: sampleFacts()},
		&fakeSpawner{result: "not json"}, GeneratorConfig{}, nil).
		WithDeliverer(deliverer)

	gen.Generate(context.Background(), rpt.ID)
	got, _ := repo.GetReport(context.Background(), rpt.ID)
	if got.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %q", got.Status)
	}
	if deliverer.called {
		t.Error("failed report must not be delivered (locked decision)")
	}
}
