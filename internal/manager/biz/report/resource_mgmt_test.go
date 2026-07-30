package report

import "testing"

func TestDownsizeHint(t *testing.T) {
	cfg := DefaultResourceMgmtConfig()
	minMem := uint64(cfg.MinMemGB * float64(gib))
	d := &DeviceResourceStat{
		CPUCount:      16,
		MemTotalBytes: 32 * gib,
		CPUPeak:       12,
		MemPeak:       22,
	}
	hint, ok := downsizeHint(d, cfg, minMem)
	if !ok || hint == "" {
		t.Fatalf("expected downsize hint, got ok=%v hint=%q", ok, hint)
	}
	d.CPUPeak = 25
	if _, ok := downsizeHint(d, cfg, minMem); ok {
		t.Fatal("high cpu peak should not suggest downsize")
	}
	d.CPUPeak = 12
	d.CPUCount = 4
	if _, ok := downsizeHint(d, cfg, minMem); ok {
		t.Fatal("low core count should not suggest downsize")
	}
}

func TestApplyResourceMgmtPerSystem(t *testing.T) {
	facts := &ReportFacts{
		Systems: []SystemFactsBlock{{
			DeviceResources: DeviceResourceFacts{
				Devices: []DeviceResourceStat{{
					DeviceID: 1, CPUCount: 16, MemTotalBytes: 32 * gib,
					CPUPeak: 5, MemPeak: 10,
				}},
			},
		}},
	}
	ApplyResourceMgmt(facts, DefaultResourceMgmtConfig())
	if !facts.Systems[0].DeviceResources.Devices[0].DownsizeSuggest {
		t.Fatal("expected downsize on system block device")
	}
}
