package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/ongridio/ongrid/internal/manager/biz/aiops/tools/basetool"
	devicebiz "github.com/ongridio/ongrid/internal/manager/biz/device"
)

// QuerySystemsTool is the BaseTool form of query_systems.
type QuerySystemsTool struct {
	devices *devicebiz.Usecase
	log     *slog.Logger
}

// NewQuerySystemsTool builds the BaseTool variant.
func NewQuerySystemsTool(devices *devicebiz.Usecase, log *slog.Logger) *QuerySystemsTool {
	if log == nil {
		log = slog.Default()
	}
	return &QuerySystemsTool{devices: devices, log: log}
}

const querySystemsWhenToUse = "When the user asks which business systems exist, how devices are grouped by system_name, or wants devices under a specific system (e.g. 订单中心). " +
	"Use BEFORE drilling into metrics/logs for a whole system. " +
	"NOT for flat device filters by role/online (use query_devices). " +
	"NOT for deployment topology (use get_topology)."

// Info returns metadata. Class=read.
func (t *QuerySystemsTool) Info(_ context.Context) (*basetool.ToolInfo, error) {
	return &basetool.ToolInfo{
		Name:        ToolNameQuerySystems,
		Description: QuerySystemsDescription,
		WhenToUse:   querySystemsWhenToUse,
		Parameters:  QuerySystemsSchema,
		Class:       "read",
	}, nil
}

// InvokableRun lists systems and optional per-system devices.
func (t *QuerySystemsTool) InvokableRun(ctx context.Context, argsJSON string, _ ...basetool.InvokeOption) (string, error) {
	reg := &Registry{devices: t.devices, log: t.log}
	res, err := reg.executeQuerySystems(ctx, json.RawMessage(argsJSON))
	if err != nil {
		return "", err
	}
	return string(res.ResultJSON), nil
}
