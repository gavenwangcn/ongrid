package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	devicebiz "github.com/ongridio/ongrid/internal/manager/biz/device"
	edgebiz "github.com/ongridio/ongrid/internal/manager/biz/edge"
	devicemodel "github.com/ongridio/ongrid/internal/manager/model/device"
	"github.com/ongridio/ongrid/internal/pkg/errs"
	"github.com/ongridio/ongrid/internal/pkg/logquery"
)

type logSourcesFakeQuerier struct {
	fakeLogQuerier
	series []map[string]string
	match  string
}

func (f *logSourcesFakeQuerier) Series(_ context.Context, matches []string, _, _ time.Time) ([]map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(matches) > 0 {
		f.match = matches[0]
	}
	return f.series, nil
}

type logSourcesDeviceRepo struct {
	mu   sync.Mutex
	byID map[uint64]*devicemodel.Device
}

func (r *logSourcesDeviceRepo) List(_ context.Context, f devicebiz.ListFilter) ([]*devicemodel.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*devicemodel.Device, 0)
	for _, d := range r.byID {
		if f.SystemName != "" && d.SystemName != f.SystemName {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *logSourcesDeviceRepo) FindOrCreateByFingerprint(context.Context, *devicemodel.Device) (*devicemodel.Device, error) {
	return nil, errs.ErrNotWiredYet
}
func (r *logSourcesDeviceRepo) RebindFingerprint(context.Context, string, string) error { return errs.ErrNotWiredYet }
func (r *logSourcesDeviceRepo) UpdateHostFacts(context.Context, uint64, devicebiz.HostFacts) error {
	return errs.ErrNotWiredYet
}
func (r *logSourcesDeviceRepo) UpdateUsage(context.Context, uint64, devicebiz.Usage) error {
	return errs.ErrNotWiredYet
}
func (r *logSourcesDeviceRepo) UpdateRoles(context.Context, uint64, uint8) error { return errs.ErrNotWiredYet }
func (r *logSourcesDeviceRepo) UpdateNameDescription(context.Context, uint64, string, string) error {
	return errs.ErrNotWiredYet
}
func (r *logSourcesDeviceRepo) UpdateHostname(context.Context, uint64, string) error { return errs.ErrNotWiredYet }
func (r *logSourcesDeviceRepo) UpdateOperatorMeta(context.Context, uint64, string, string, string) error {
	return errs.ErrNotWiredYet
}
func (r *logSourcesDeviceRepo) SetNodeID(context.Context, uint64, uint64) error { return errs.ErrNotWiredYet }
func (r *logSourcesDeviceRepo) MarkOnline(context.Context, uint64) error       { return errs.ErrNotWiredYet }
func (r *logSourcesDeviceRepo) MarkOffline(context.Context, uint64) error      { return errs.ErrNotWiredYet }
func (r *logSourcesDeviceRepo) GetMany(context.Context, []uint64) (map[uint64]*devicemodel.Device, error) {
	return nil, errs.ErrNotWiredYet
}
func (r *logSourcesDeviceRepo) Count(context.Context) (int64, error) { return 0, errs.ErrNotWiredYet }

func (r *logSourcesDeviceRepo) Get(_ context.Context, id uint64) (*devicemodel.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.byID[id]
	if !ok {
		return nil, errs.ErrNotFound
	}
	return d, nil
}

func (r *logSourcesDeviceRepo) Create(context.Context, *devicemodel.Device) (*devicemodel.Device, error) {
	return nil, errs.ErrNotWiredYet
}
func (r *logSourcesDeviceRepo) Update(context.Context, *devicemodel.Device) (*devicemodel.Device, error) {
	return nil, errs.ErrNotWiredYet
}
func (r *logSourcesDeviceRepo) Delete(context.Context, uint64) error { return errs.ErrNotWiredYet }
func (r *logSourcesDeviceRepo) UpsertByHostname(context.Context, *devicemodel.Device) (*devicemodel.Device, error) {
	return nil, errs.ErrNotWiredYet
}

type logSourcesEdgeDeviceRepo struct {
	edgeForDevice map[uint64]uint64
}

func (r *logSourcesEdgeDeviceRepo) Link(context.Context, uint64, uint64, devicemodel.EdgeDeviceRelationType) error {
	return nil
}
func (r *logSourcesEdgeDeviceRepo) Unlink(context.Context, uint64, uint64, devicemodel.EdgeDeviceRelationType) error {
	return nil
}
func (r *logSourcesEdgeDeviceRepo) DeleteAllForEdge(context.Context, uint64) error { return nil }
func (r *logSourcesEdgeDeviceRepo) LookupHostDevice(context.Context, uint64) (uint64, error) {
	return 0, errs.ErrNotFound
}
func (r *logSourcesEdgeDeviceRepo) LookupEdgeForDevice(_ context.Context, deviceID uint64, t devicemodel.EdgeDeviceRelationType) (uint64, error) {
	if t != devicemodel.EdgeDeviceRelationHost {
		return 0, errs.ErrNotFound
	}
	if eid, ok := r.edgeForDevice[deviceID]; ok {
		return eid, nil
	}
	return 0, errs.ErrNotFound
}
func (r *logSourcesEdgeDeviceRepo) ListDevicesForEdge(context.Context, uint64) ([]*devicemodel.EdgeDevice, error) {
	return nil, nil
}

func TestClassifyLokiSeriesUnitsContainersFiles(t *testing.T) {
	labelIDs := []string{"42", "7"}
	series := []map[string]string{
		{"device_id": "42", "unit": "nginx.service", "ongrid_source": "journald"},
		{"device_id": "42", "container": "order-api", "container_id": "abc123", "ongrid_source": "docker_api"},
		{"device_id": "42", "ongrid_source": "file:/var/log/app.log", "filename": "/var/log/app.log"},
	}
	c := classifyLokiSeries(labelIDs, series)

	if len(c.units) != 1 {
		t.Fatalf("units = %d, want 1", len(c.units))
	}
	if c.units["nginx.service"].LogQLSelector != "{device_id=~\"42|7\",unit=\"nginx.service\"}" {
		t.Fatalf("unit selector = %q", c.units["nginx.service"].LogQLSelector)
	}
	if len(c.containers) != 1 {
		t.Fatalf("containers = %d, want 1", len(c.containers))
	}
	if c.containers["order-api"].LogQLSelector != "{device_id=~\"42|7\",container=\"order-api\"}" {
		t.Fatalf("container selector = %q", c.containers["order-api"].LogQLSelector)
	}
	if len(c.files) != 1 {
		t.Fatalf("files = %d, want 1", len(c.files))
	}
}

func TestParseLogLookbackCapsAtRetention(t *testing.T) {
	d, err := parseLogLookback("168h")
	if err != nil {
		t.Fatal(err)
	}
	if d != maxLogLookback {
		t.Fatalf("lookback = %v, want %v", d, maxLogLookback)
	}
}

func TestDeviceLogStreamSelector(t *testing.T) {
	if got := deviceLogStreamSelector([]string{"42"}); got != "{device_id=\"42\"}" {
		t.Fatalf("single = %q", got)
	}
	if got := deviceLogStreamSelector([]string{"42", "7"}); got != "{device_id=~\"42|7\"}" {
		t.Fatalf("multi = %q", got)
	}
}

func TestQueryLogSources_ExecuteByDeviceID(t *testing.T) {
	lq := &logSourcesFakeQuerier{
		series: []map[string]string{
			{"device_id": "10", "container": "order-svc", "ongrid_source": "docker_api"},
		},
	}
	repo := &logSourcesDeviceRepo{byID: map[uint64]*devicemodel.Device{
		10: {ID: 10, Name: "node-a", SystemName: "订单中心", Online: true},
	}}
	links := &logSourcesEdgeDeviceRepo{edgeForDevice: map[uint64]uint64{10: 99}}
	devUC := devicebiz.NewUsecase(repo, links, slog.Default())

	reg := &Registry{
		logQuery: lq,
		devices:  devUC,
		log:      slog.Default(),
	}

	out, err := reg.executeQueryLogSources(context.Background(), json.RawMessage(`{"device_ids":[10]}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if lq.match != "{device_id=~\"10|99\"}" {
		t.Fatalf("series match = %q", lq.match)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.ResultJSON, &payload); err != nil {
		t.Fatal(err)
	}
	devs, ok := payload["devices"].([]any)
	if !ok || len(devs) != 1 {
		t.Fatalf("devices payload = %v", payload["devices"])
	}
}

func TestQueryLogSources_RegisteredWithLoki(t *testing.T) {
	lq := &logSourcesFakeQuerier{series: []map[string]string{}}
	repo := &logSourcesDeviceRepo{byID: map[uint64]*devicemodel.Device{}}
	devUC := devicebiz.NewUsecase(repo, &logSourcesEdgeDeviceRepo{}, slog.Default())
	reg := NewRegistry(nil, nil, devUC, nil, lq, nil, nil, slog.Default())
	if !containsName(schemaNames(reg.Schemas()), ToolNameQueryLogSources) {
		t.Fatalf("query_log_sources not registered")
	}
	bag := reg.BuildBaseTools()
	found := false
	for _, tool := range bag.AllTools() {
		info, err := tool.Info(context.Background())
		if err == nil && info != nil && info.Name == ToolNameQueryLogSources {
			found = true
		}
	}
	if !found {
		t.Fatal("query_log_sources missing from BaseTool bag")
	}
}

func TestLogsConfiguredFromPluginRows(t *testing.T) {
	rows := []edgebiz.PluginRow{{
		PluginName: "logs",
		Enabled:    true,
		Spec: map[string]interface{}{
			"enable_docker_api": true,
			"file_paths":        []interface{}{"/var/log/app.log"},
			"journald_units":    []interface{}{"nginx.service"},
		},
	}}
	cfg := logsConfiguredFromPluginRows(rows)
	if cfg == nil || !cfg.EnableDockerAPI || len(cfg.FilePaths) != 1 || len(cfg.JournaldUnits) != 1 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

var _ LogQuerier = (*logSourcesFakeQuerier)(nil)
var _ = logquery.QueryRangeResult{}
