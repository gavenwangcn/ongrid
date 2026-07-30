package report

import "testing"

func TestEffectiveSectionsLegacyAll(t *testing.T) {
	got := EffectiveSections(Scope{})
	if len(got) != 3 {
		t.Fatalf("legacy scope = %v, want 3 sections", got)
	}
}

func TestNormalizeSectionsDailyDefault(t *testing.T) {
	got := NormalizeSections([]string{"cluster", "logs", "cluster", "nope"})
	want := []string{"cluster", "logs"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("normalize = %v, want %v", got, want)
	}
}

func TestTrimContentForSections(t *testing.T) {
	c := &Content{
		Resource:     ResourceFacts{Available: true, CPUAvg: 1},
		Fleet:        FleetFacts{Total: 2},
		Logs:         LogFacts{Available: true, TotalErrors: 3},
		KeyIncidents: []KeyIncident{{ID: 1}},
		Actions:      ActionsSummary{SafeTotal: 4},
	}
	TrimContentForSections(c, []string{SectionCluster})
	if c.Logs.Available || len(c.KeyIncidents) > 0 || c.Actions.SafeTotal != 0 {
		t.Fatalf("cluster-only trim left logs/alerts: logs=%+v incidents=%d actions=%+v", c.Logs, len(c.KeyIncidents), c.Actions)
	}
	if !c.Resource.Available || c.Fleet.Total != 2 {
		t.Fatalf("cluster facts cleared unexpectedly: resource=%+v fleet=%+v", c.Resource, c.Fleet)
	}
}
