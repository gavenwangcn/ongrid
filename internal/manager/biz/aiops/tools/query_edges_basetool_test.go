package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	devicebiz "github.com/ongridio/ongrid/internal/manager/biz/device"
	edgebiz "github.com/ongridio/ongrid/internal/manager/biz/edge"
	devicemodel "github.com/ongridio/ongrid/internal/manager/model/device"
	edgemodel "github.com/ongridio/ongrid/internal/manager/model/edge"
	"github.com/ongridio/ongrid/internal/pkg/errs"
)

type queryEdgesFakeDeviceRepo struct {
	devices []*devicemodel.Device
}

func (r *queryEdgesFakeDeviceRepo) FindOrCreateByFingerprint(context.Context, *devicemodel.Device) (*devicemodel.Device, error) {
	return nil, errs.ErrNotFound
}
func (r *queryEdgesFakeDeviceRepo) RebindFingerprint(context.Context, string, string) error { return nil }
func (r *queryEdgesFakeDeviceRepo) UpdateHostFacts(context.Context, uint64, devicebiz.HostFacts) error {
	return nil
}
func (r *queryEdgesFakeDeviceRepo) MarkOnline(context.Context, uint64) error  { return nil }
func (r *queryEdgesFakeDeviceRepo) MarkOffline(context.Context, uint64) error { return nil }
func (r *queryEdgesFakeDeviceRepo) Get(context.Context, uint64) (*devicemodel.Device, error) {
	return nil, errs.ErrNotFound
}
func (r *queryEdgesFakeDeviceRepo) GetMany(context.Context, []uint64) (map[uint64]*devicemodel.Device, error) {
	return nil, nil
}
func (r *queryEdgesFakeDeviceRepo) UpdateUsage(context.Context, uint64, devicebiz.Usage) error { return nil }
func (r *queryEdgesFakeDeviceRepo) UpdateRoles(context.Context, uint64, uint8) error           { return nil }
func (r *queryEdgesFakeDeviceRepo) UpdateNameDescription(context.Context, uint64, string, string) error {
	return nil
}
func (r *queryEdgesFakeDeviceRepo) UpdateHostname(context.Context, uint64, string) error { return nil }
func (r *queryEdgesFakeDeviceRepo) UpdateOperatorMeta(context.Context, uint64, string, string, string) error {
	return nil
}
func (r *queryEdgesFakeDeviceRepo) SetNodeID(context.Context, uint64, uint64) error { return nil }
func (r *queryEdgesFakeDeviceRepo) List(_ context.Context, _ devicebiz.ListFilter) ([]*devicemodel.Device, error) {
	return append([]*devicemodel.Device(nil), r.devices...), nil
}
func (r *queryEdgesFakeDeviceRepo) Count(context.Context) (int64, error) { return int64(len(r.devices)), nil }
func (r *queryEdgesFakeDeviceRepo) Delete(context.Context, uint64) error { return nil }

func TestQueryEdgesTool_Info(t *testing.T) {
	uc := edgebiz.NewUsecase(newFakeEdgeRepo(), nil, nil, slog.Default())
	tool := NewQueryEdgesTool(nil, uc, nil)
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != ToolNameQueryEdges {
		t.Errorf("Name = %q", info.Name)
	}
	if !strings.Contains(strings.ToLower(info.WhenToUse), "not") {
		t.Errorf("WhenToUse needs reverse guard")
	}
}

func TestQueryEdgesTool_LegacyEdgeFallback(t *testing.T) {
	// devices nil → falls back to edges path.
	e1 := &edgemodel.Edge{
		ID: 1, Name: "alpha", Status: "online",
		SystemName: "订单中心", DeviceIP: "10.0.0.1", EnvironmentTag: "prod",
	}
	e2 := &edgemodel.Edge{ID: 2, Name: "beta", Status: "offline"}
	uc := edgebiz.NewUsecase(newFakeEdgeRepo(e1, e2), nil, nil, slog.Default())
	tool := NewQueryEdgesTool(nil, uc, nil)

	out, err := tool.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	devs, _ := got["devices"].([]any)
	if len(devs) == 0 {
		t.Errorf("expected devices in response")
	}
	first, _ := devs[0].(map[string]any)
	if first["system_name"] != "订单中心" || first["device_ip"] != "10.0.0.1" || first["environment_tag"] != "prod" {
		t.Errorf("legacy edge row missing metadata: %#v", first)
	}
}

func TestQueryEdgesTool_DeviceMetadata(t *testing.T) {
	d := &devicemodel.Device{
		ID: 10, Name: "dft-lc-ebd", Hostname: "dft-lc-ebd-测试",
		SystemName: "订单中心", DeviceIP: "192.168.1.10", EnvironmentTag: "test",
		Online: true, Roles: devicemodel.RoleBitServer,
	}
	devUC := devicebiz.NewUsecase(&queryEdgesFakeDeviceRepo{devices: []*devicemodel.Device{d}}, nil, slog.Default())
	tool := NewQueryEdgesTool(devUC, nil, nil)

	out, err := tool.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var got struct {
		Devices []EdgeRow `json:"devices"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(got.Devices))
	}
	row := got.Devices[0]
	if row.SystemName != "订单中心" || row.DeviceIP != "192.168.1.10" || row.EnvironmentTag != "test" {
		t.Errorf("metadata = system:%q ip:%q env:%q", row.SystemName, row.DeviceIP, row.EnvironmentTag)
	}
}

func TestQueryEdgesTool_BadArgs(t *testing.T) {
	uc := edgebiz.NewUsecase(newFakeEdgeRepo(), nil, nil, slog.Default())
	tool := NewQueryEdgesTool(nil, uc, nil)
	if _, err := tool.InvokableRun(context.Background(), `not json`); err == nil {
		t.Errorf("expected error for non-JSON")
	}
}

func TestQueryEdgesTool_NilDeps(t *testing.T) {
	tool := NewQueryEdgesTool(nil, nil, nil)
	_, err := tool.InvokableRun(context.Background(), `{}`)
	if err == nil {
		t.Errorf("expected error when both devices and edges are nil")
	}
}
