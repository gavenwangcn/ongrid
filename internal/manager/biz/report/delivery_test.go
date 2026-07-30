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
	if !strings.Contains(md, "**周报 · 2026 W23**") {
		t.Errorf("title missing:\n%s", md)
	}
	if !strings.Contains(md, "Incidents 23 ↓12%") {
		t.Errorf("hero line wrong:\n%s", md)
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
