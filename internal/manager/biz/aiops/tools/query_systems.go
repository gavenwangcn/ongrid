package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	devicebiz "github.com/ongridio/ongrid/internal/manager/biz/device"
	devicemodel "github.com/ongridio/ongrid/internal/manager/model/device"
)

// ToolNameQuerySystems is the LLM-visible name for business-system inventory.
const ToolNameQuerySystems = "query_systems"

// QuerySystemsDescription tells the model when to prefer this over query_devices.
const QuerySystemsDescription = "List operator-assigned business systems (system_name) and how many devices belong to each, or drill into one system to see its devices. " +
	"Use when the question is about which systems exist, fleet grouping by business system, or devices under a named system. " +
	"For a flat device list filtered by role/online/name without grouping, use query_devices instead."

// QuerySystemsSchema is the JSON Schema for query_systems arguments.
var QuerySystemsSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "system_name": {
      "type": "string",
      "description": "When set, return only this business system and its devices (exact match on operator-assigned system_name)."
    },
    "include_devices": {
      "type": "boolean",
      "description": "When listing all systems, embed device rows under each system. Default false (counts only)."
    },
    "devices_per_system": {
      "type": "integer",
      "minimum": 1,
      "maximum": 500,
      "description": "Max devices returned per system when include_devices is true (default 50)."
    }
  }
}`)

// QuerySystemsArgs is the typed argument object for query_systems.
type QuerySystemsArgs struct {
	SystemName         string `json:"system_name,omitempty"`
	IncludeDevices     bool   `json:"include_devices,omitempty"`
	DevicesPerSystem   int    `json:"devices_per_system,omitempty"`
}

// SystemSummary is one business system bucket in query_systems output.
type SystemSummary struct {
	SystemName   string     `json:"system_name"`
	DeviceCount  int        `json:"device_count"`
	OnlineCount  int        `json:"online_count"`
	OfflineCount int        `json:"offline_count"`
	Devices      []EdgeRow  `json:"devices,omitempty"`
}

const querySystemsCallTimeout = 10 * time.Second

func deviceToEdgeRow(d *devicemodel.Device) EdgeRow {
	return EdgeRow{
		ID:             d.ID,
		Name:           d.Name,
		Hostname:       d.Hostname,
		SystemName:     d.SystemName,
		DeviceIP:       d.DeviceIP,
		EnvironmentTag: d.EnvironmentTag,
		Online:         d.Online,
		Roles:          devicemodel.DecodeRoles(d.Roles),
		LastSeenAt:     d.LastSeenAt,
	}
}

func aggregateSystems(devices []*devicemodel.Device, includeDevices bool, perSystemLimit int) []SystemSummary {
	if perSystemLimit <= 0 {
		perSystemLimit = 50
	}
	if perSystemLimit > 500 {
		perSystemLimit = 500
	}

	type bucket struct {
		summary SystemSummary
		all     []*devicemodel.Device
	}
	byName := map[string]*bucket{}
	order := make([]string, 0)

	for _, d := range devices {
		key := strings.TrimSpace(d.SystemName)
		b, ok := byName[key]
		if !ok {
			b = &bucket{summary: SystemSummary{SystemName: key}}
			byName[key] = b
			order = append(order, key)
		}
		b.all = append(b.all, d)
		b.summary.DeviceCount++
		if d.Online {
			b.summary.OnlineCount++
		} else {
			b.summary.OfflineCount++
		}
	}

	sort.Slice(order, func(i, j int) bool {
		// Named systems first (alphabetically), empty/unassigned last.
		if order[i] == "" && order[j] != "" {
			return true
		}
		if order[i] != "" && order[j] == "" {
			return false
		}
		return order[i] < order[j]
	})

	out := make([]SystemSummary, 0, len(order))
	for _, key := range order {
		b := byName[key]
		if includeDevices {
			rows := make([]EdgeRow, 0, min(len(b.all), perSystemLimit))
			for i, d := range b.all {
				if i >= perSystemLimit {
					break
				}
				rows = append(rows, deviceToEdgeRow(d))
			}
			b.summary.Devices = rows
		}
		out = append(out, b.summary)
	}
	return out
}

func (r *Registry) executeQuerySystems(ctx context.Context, args json.RawMessage) (ExecuteResult, error) {
	if r.devices == nil {
		return ExecuteResult{}, fmt.Errorf("query_systems: device usecase not configured")
	}
	var in QuerySystemsArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return ExecuteResult{}, fmt.Errorf("query_systems: bad args: %w", err)
	}
	if in.DevicesPerSystem <= 0 {
		in.DevicesPerSystem = 50
	}

	callCtx, cancel := context.WithTimeout(ctx, querySystemsCallTimeout)
	defer cancel()

	f := devicebiz.ListFilter{Limit: 5000}
	if strings.TrimSpace(in.SystemName) != "" {
		f.SystemName = strings.TrimSpace(in.SystemName)
		in.IncludeDevices = true
	}

	all, err := r.devices.List(callCtx, f)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("query_systems: list: %w", err)
	}

	systems := aggregateSystems(all, in.IncludeDevices || f.SystemName != "", in.DevicesPerSystem)
	payload := map[string]any{
		"systems": systems,
		"count":   len(systems),
	}
	if f.SystemName != "" && len(systems) == 0 {
		payload["hint"] = fmt.Sprintf("no devices with system_name=%q (system may be unused or name mismatch)", f.SystemName)
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("query_systems: marshal: %w", err)
	}
	return ExecuteResult{ResultJSON: out}, nil
}
