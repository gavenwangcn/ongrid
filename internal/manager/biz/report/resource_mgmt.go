package report

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	bizsetting "github.com/ongridio/ongrid/internal/manager/biz/setting"
	settingmodel "github.com/ongridio/ongrid/internal/manager/model/setting"
	"github.com/ongridio/ongrid/internal/pkg/errs"
)

const gib = 1024 * 1024 * 1024

// ResourceMgmtConfig controls downsize hints in cluster posture reports.
// Thresholds compare period peak utilization against host capacity.
type ResourceMgmtConfig struct {
	Enabled        bool    `json:"enabled"`
	CPUPeakMaxPct   float64 `json:"cpu_peak_max_pct"`
	MemPeakMaxPct  float64 `json:"mem_peak_max_pct"`
	MinCPUCount    int     `json:"min_cpu_count"`
	MinMemGB       float64 `json:"min_mem_gb"`
}

// DefaultResourceMgmtConfig is used when settings are unset.
func DefaultResourceMgmtConfig() ResourceMgmtConfig {
	return ResourceMgmtConfig{
		Enabled:       true,
		CPUPeakMaxPct: 20,
		MemPeakMaxPct: 30,
		MinCPUCount:   8,
		MinMemGB:      16,
	}
}

// Normalize clamps invalid values and applies defaults for zero fields.
func (c ResourceMgmtConfig) Normalize() ResourceMgmtConfig {
	def := DefaultResourceMgmtConfig()
	if c.CPUPeakMaxPct <= 0 {
		c.CPUPeakMaxPct = def.CPUPeakMaxPct
	}
	if c.MemPeakMaxPct <= 0 {
		c.MemPeakMaxPct = def.MemPeakMaxPct
	}
	if c.MinCPUCount <= 0 {
		c.MinCPUCount = def.MinCPUCount
	}
	if c.MinMemGB <= 0 {
		c.MinMemGB = def.MinMemGB
	}
	return c
}

// ResourceMgmtConfigService persists report resource-management thresholds.
type ResourceMgmtConfigService struct {
	settings *bizsetting.Service
}

func NewResourceMgmtConfigService(settings *bizsetting.Service) *ResourceMgmtConfigService {
	return &ResourceMgmtConfigService{settings: settings}
}

func (s *ResourceMgmtConfigService) Get(ctx context.Context) (ResourceMgmtConfig, error) {
	if s.settings == nil {
		return DefaultResourceMgmtConfig(), nil
	}
	raw, ok, err := s.settings.Get(ctx, settingmodel.CategoryReport, settingmodel.KeyReportResourceMgmt)
	if err != nil {
		return ResourceMgmtConfig{}, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return DefaultResourceMgmtConfig(), nil
	}
	var c ResourceMgmtConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return DefaultResourceMgmtConfig(), nil
	}
	return c.Normalize(), nil
}

func (s *ResourceMgmtConfigService) Set(ctx context.Context, cfg ResourceMgmtConfig) (ResourceMgmtConfig, error) {
	if s.settings == nil {
		return ResourceMgmtConfig{}, fmt.Errorf("%w: report resource-mgmt settings unavailable", errs.ErrNotWiredYet)
	}
	cfg = cfg.Normalize()
	b, err := json.Marshal(cfg)
	if err != nil {
		return ResourceMgmtConfig{}, err
	}
	if err := s.settings.Set(ctx, settingmodel.CategoryReport, settingmodel.KeyReportResourceMgmt, string(b), false); err != nil {
		return ResourceMgmtConfig{}, err
	}
	return cfg, nil
}

// ApplyResourceMgmt annotates device_resources with downsize hints.
func ApplyResourceMgmt(facts *ReportFacts, cfg ResourceMgmtConfig) {
	if facts == nil || !cfg.Enabled {
		return
	}
	cfg = cfg.Normalize()
	minMemBytes := uint64(cfg.MinMemGB * float64(gib))
	apply := func(devs *DeviceResourceFacts) {
		if devs == nil {
			return
		}
		for i := range devs.Devices {
			hint, ok := downsizeHint(&devs.Devices[i], cfg, minMemBytes)
			if ok {
				devs.Devices[i].DownsizeSuggest = true
				devs.Devices[i].DownsizeHint = hint
			}
		}
	}
	apply(&facts.DeviceResources)
	for i := range facts.Systems {
		apply(&facts.Systems[i].DeviceResources)
	}
}

func downsizeHint(d *DeviceResourceStat, cfg ResourceMgmtConfig, minMemBytes uint64) (string, bool) {
	if d == nil || d.CPUCount <= cfg.MinCPUCount || d.MemTotalBytes <= minMemBytes {
		return "", false
	}
	// Require observed peaks — zero means no Prom data for the window.
	if d.CPUPeak <= 0 || d.MemPeak <= 0 {
		return "", false
	}
	if d.CPUPeak >= cfg.CPUPeakMaxPct || d.MemPeak >= cfg.MemPeakMaxPct {
		return "", false
	}
	return fmt.Sprintf(
		"周期 CPU 峰值 %.1f%%、内存峰值 %.1f%% 持续低于阈值（<%g%% / <%g%%），且规格为 %d 核 / %s 内存，可考虑降配",
		d.CPUPeak, d.MemPeak, cfg.CPUPeakMaxPct, cfg.MemPeakMaxPct, d.CPUCount, formatBytesSize(float64(d.MemTotalBytes)),
	), true
}
