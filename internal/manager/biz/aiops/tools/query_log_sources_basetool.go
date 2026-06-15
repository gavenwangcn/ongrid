package tools

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/ongridio/ongrid/internal/manager/biz/aiops/tools/basetool"
	devicebiz "github.com/ongridio/ongrid/internal/manager/biz/device"
)

// QueryLogSourcesTool is the BaseTool form of query_log_sources.
type QueryLogSourcesTool struct {
	logQuery       LogQuerier
	devices        *devicebiz.Usecase
	pluginConfigs  PluginConfigLister
	log            *slog.Logger
}

// NewQueryLogSourcesTool builds the BaseTool variant.
func NewQueryLogSourcesTool(
	lq LogQuerier,
	devices *devicebiz.Usecase,
	plugins PluginConfigLister,
	log *slog.Logger,
) *QueryLogSourcesTool {
	if log == nil {
		log = slog.Default()
	}
	return &QueryLogSourcesTool{logQuery: lq, devices: devices, pluginConfigs: plugins, log: log}
}

const queryLogSourcesWhenToUse = "When troubleshooting logs for a business system or device and you need to know which systemd units, Docker containers, or file paths exist in Loki BEFORE writing LogQL. " +
	"Typical flow: query_systems(system_name=...) → query_log_sources(system_name=...) → query_logql(logql_selector from result). " +
	"Use when the user names an app/container/unit (e.g. 订单容器) but you do not know the exact Loki label values. " +
	"NOT for log line content (use query_logql). NOT for device discovery without logs (use query_devices / query_systems)."

// Info returns metadata. Class=read.
func (t *QueryLogSourcesTool) Info(_ context.Context) (*basetool.ToolInfo, error) {
	return &basetool.ToolInfo{
		Name:        ToolNameQueryLogSources,
		Description: QueryLogSourcesDescription,
		WhenToUse:   queryLogSourcesWhenToUse,
		Parameters:  QueryLogSourcesSchema,
		Class:       "read",
	}, nil
}

// InvokableRun lists Loki-indexed log sources per device or system.
func (t *QueryLogSourcesTool) InvokableRun(ctx context.Context, argsJSON string, _ ...basetool.InvokeOption) (string, error) {
	reg := &Registry{
		logQuery:       t.logQuery,
		devices:        t.devices,
		pluginConfigs:  t.pluginConfigs,
		log:            t.log,
	}
	res, err := reg.executeQueryLogSources(ctx, json.RawMessage(argsJSON))
	if err != nil {
		return "", err
	}
	return string(res.ResultJSON), nil
}
