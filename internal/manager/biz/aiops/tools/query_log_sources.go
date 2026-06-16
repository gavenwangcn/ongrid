package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	devicebiz "github.com/ongridio/ongrid/internal/manager/biz/device"
	edgebiz "github.com/ongridio/ongrid/internal/manager/biz/edge"
	edgemodel "github.com/ongridio/ongrid/internal/manager/model/edge"
)

// ToolNameQueryLogSources is the LLM-visible name for Loki log-source inventory.
const ToolNameQueryLogSources = "query_log_sources"

// QueryLogSourcesDescription tells the model when to prefer this over query_logql.
const QueryLogSourcesDescription = "List application log sources (systemd units, Docker containers, file paths) indexed in Loki for one or more devices or a business system. " +
	"Returns suggested LogQL selectors so follow-up query_logql calls can target a specific container/unit/file. " +
	"Use BEFORE query_logql when the user names a system, device, or app container but you do not yet know the exact Loki labels. " +
	"For raw log lines or error counts, call query_logql with the returned logql_selector."

// QueryLogSourcesSchema is the JSON Schema for query_log_sources arguments.
var QueryLogSourcesSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "device_ids": {
      "type": "array",
      "items": {"type": "integer"},
      "minItems": 1,
      "maxItems": 16,
      "description": "Host device ids to inspect. Combine with system_name only when you need an explicit subset."
    },
    "system_name": {
      "type": "string",
      "description": "Business system name (operator-assigned system_name). When set, all devices in that system are queried unless device_ids narrows the set."
    },
    "lookback": {
      "type": "string",
      "description": "How far back to scan Loki index (duration, default 24h). Supports hours (72h), days (7d). Max 31d."
    },
    "include_configured": {
      "type": "boolean",
      "description": "When true (default), merge edge logs plugin config (file_paths, journald_units, docker API flag) even if not yet indexed in Loki."
    }
  }
}`)

// QueryLogSourcesArgs is the typed argument object.
type QueryLogSourcesArgs struct {
	DeviceIDs          []uint64 `json:"device_ids,omitempty"`
	SystemName         string   `json:"system_name,omitempty"`
	Lookback           string   `json:"lookback,omitempty"`
	IncludeConfigured  *bool    `json:"include_configured,omitempty"`
}

// LogSourceUnit is one indexed systemd unit log stream.
type LogSourceUnit struct {
	Unit          string `json:"unit"`
	LogQLSelector string `json:"logql_selector"`
	Indexed       bool   `json:"indexed"`
}

// LogSourceContainer is one indexed Docker container log stream.
type LogSourceContainer struct {
	Container     string `json:"container"`
	ContainerID   string `json:"container_id,omitempty"`
	LogQLSelector string `json:"logql_selector"`
	Indexed       bool   `json:"indexed"`
}

// LogSourceFile is one file / journald / docker_api stream keyed by ongrid_source or filename.
type LogSourceFile struct {
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	OngridSource  string `json:"ongrid_source,omitempty"`
	Filename      string `json:"filename,omitempty"`
	LogQLSelector string `json:"logql_selector"`
	Indexed       bool   `json:"indexed"`
}

// LogSourcesConfigured is the edge logs plugin spec slice (planned collection).
type LogSourcesConfigured struct {
	LogsPluginEnabled bool     `json:"logs_plugin_enabled"`
	EnableJournald    bool     `json:"enable_journald"`
	EnableDockerAPI   bool     `json:"enable_docker_api"`
	JournaldUnits     []string `json:"journald_units,omitempty"`
	FilePaths         []string `json:"file_paths,omitempty"`
}

// LogSourcesHints gives ready-made LogQL examples for error triage.
type LogSourcesHints struct {
	RecentErrorsExample string `json:"recent_errors_example,omitempty"`
	ErrorCountExample   string `json:"error_count_5m_example,omitempty"`
}

// DeviceLogSources is the per-device inventory block.
type DeviceLogSources struct {
	DeviceID      uint64               `json:"device_id"`
	Name          string               `json:"name,omitempty"`
	SystemName    string               `json:"system_name,omitempty"`
	LogLabelIDs   []string             `json:"log_label_ids"`
	Units         []LogSourceUnit      `json:"units"`
	Containers    []LogSourceContainer `json:"containers"`
	Files         []LogSourceFile      `json:"files"`
	Configured    *LogSourcesConfigured `json:"configured,omitempty"`
	Hints         LogSourcesHints      `json:"hints"`
	Error         string               `json:"error,omitempty"`
}

const (
	queryLogSourcesCallTimeout = 45 * time.Second
	maxLogLookback             = 31 * 24 * time.Hour
	defaultLogLookback         = 24 * time.Hour
)

func parseLogLookback(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultLogLookback, nil
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("invalid lookback duration %q", s)
		}
		d := time.Duration(days) * 24 * time.Hour
		if d > maxLogLookback {
			return maxLogLookback, nil
		}
		return d, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid lookback duration %q", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("lookback must be positive")
	}
	if d > maxLogLookback {
		return maxLogLookback, nil
	}
	return d, nil
}

// logLabelIDsForDevice returns device_id plus legacy edge_id strings used
// as Loki device_id label on older promtail configs.
func logLabelIDsForDevice(ctx context.Context, devices *devicebiz.Usecase, deviceID uint64) []string {
	ids := make([]string, 0, 2)
	seen := map[string]struct{}{}
	add := func(id uint64) {
		if id == 0 {
			return
		}
		s := fmt.Sprintf("%d", id)
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		ids = append(ids, s)
	}
	add(deviceID)
	if devices != nil && devices.Links() != nil {
		edgeID, err := devices.LookupEdgeForDevice(ctx, deviceID)
		if err == nil {
			add(edgeID)
		}
	}
	return ids
}

func deviceLogStreamSelector(labelIDs []string) string {
	if len(labelIDs) == 0 {
		return "{device_id=\"__no_match__\"}"
	}
	if len(labelIDs) == 1 {
		return fmt.Sprintf("{device_id=\"%s\"}", labelIDs[0])
	}
	return fmt.Sprintf("{device_id=~\"%s\"}", strings.Join(labelIDs, "|"))
}

func logQLWithDevice(labelIDs []string, extra string) string {
	base := deviceLogStreamSelector(labelIDs)
	if extra == "" {
		return base
	}
	inner := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(extra), "}"), "{")
	if inner == "" {
		return base
	}
	// base is `{device_id=...}` or `{device_id=~...}` — inject extra matchers.
	return strings.TrimSuffix(base, "}") + "," + inner + "}"
}

type classifiedSources struct {
	units      map[string]LogSourceUnit
	containers map[string]LogSourceContainer
	files      map[string]LogSourceFile
}

func newClassifiedSources() *classifiedSources {
	return &classifiedSources{
		units:      make(map[string]LogSourceUnit),
		containers: make(map[string]LogSourceContainer),
		files:      make(map[string]LogSourceFile),
	}
}

func classifyLokiSeries(labelIDs []string, series []map[string]string) *classifiedSources {
	out := newClassifiedSources()
	for _, labels := range series {
		if len(labels) == 0 {
			continue
		}
		src := strings.TrimSpace(labels["ongrid_source"])
		container := strings.TrimSpace(labels["container"])
		containerID := strings.TrimSpace(labels["container_id"])
		unit := strings.TrimSpace(labels["unit"])
		filename := strings.TrimSpace(labels["filename"])

		switch {
		case container != "" || src == "docker_api":
			name := container
			if name == "" {
				name = containerID
			}
			if name == "" {
				continue
			}
			key := name
			sel := logQLWithDevice(labelIDs, fmt.Sprintf("container=\"%s\"", escapeLogQLLabel(name)))
			out.containers[key] = LogSourceContainer{
				Container:     name,
				ContainerID:   containerID,
				LogQLSelector: sel,
				Indexed:       true,
			}
		case unit != "":
			sel := logQLWithDevice(labelIDs, fmt.Sprintf("unit=\"%s\"", escapeLogQLLabel(unit)))
			out.units[unit] = LogSourceUnit{
				Unit:          unit,
				LogQLSelector: sel,
				Indexed:       true,
			}
		case strings.HasPrefix(src, "file:"):
			path := strings.TrimPrefix(src, "file:")
			key := "file:" + path
			sel := logQLWithDevice(labelIDs, fmt.Sprintf("ongrid_source=\"file:%s\"", escapeLogQLLabel(path)))
			out.files[key] = LogSourceFile{
				Path:          path,
				Kind:          "file_path",
				OngridSource:  src,
				Filename:      filename,
				LogQLSelector: sel,
				Indexed:       true,
			}
		case src == "journald":
			key := "journald"
			if unit != "" {
				key = "journald:" + unit
			}
			sel := logQLWithDevice(labelIDs, "ongrid_source=\"journald\"")
			if unit != "" {
				sel = logQLWithDevice(labelIDs, fmt.Sprintf("unit=\"%s\"", escapeLogQLLabel(unit)))
			}
			out.files[key] = LogSourceFile{
				Path:          key,
				Kind:          "journald",
				OngridSource:  src,
				LogQLSelector: sel,
				Indexed:       true,
			}
		case filename != "":
			key := "filename:" + filename
			sel := logQLWithDevice(labelIDs, fmt.Sprintf("filename=\"%s\"", escapeLogQLLabel(filename)))
			out.files[key] = LogSourceFile{
				Path:          filename,
				Kind:          "filename",
				Filename:      filename,
				LogQLSelector: sel,
				Indexed:       true,
			}
		case src != "":
			key := "ongrid_source:" + src
			sel := logQLWithDevice(labelIDs, fmt.Sprintf("ongrid_source=\"%s\"", escapeLogQLLabel(src)))
			out.files[key] = LogSourceFile{
				Path:          src,
				Kind:          "ongrid_source",
				OngridSource:  src,
				LogQLSelector: sel,
				Indexed:       true,
			}
		}
	}
	return out
}

func escapeLogQLLabel(s string) string {
	return strings.ReplaceAll(s, "\\", "\\\\")
}

func mergeConfiguredSources(classified *classifiedSources, labelIDs []string, cfg *LogSourcesConfigured) {
	if classified == nil || cfg == nil {
		return
	}
	for _, u := range cfg.JournaldUnits {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := classified.units[u]; ok {
			continue
		}
		classified.units[u] = LogSourceUnit{
			Unit:          u,
			LogQLSelector: logQLWithDevice(labelIDs, fmt.Sprintf("unit=\"%s\"", escapeLogQLLabel(u))),
			Indexed:       false,
		}
	}
	for _, p := range cfg.FilePaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := "file:" + p
		if _, ok := classified.files[key]; ok {
			continue
		}
		classified.files[key] = LogSourceFile{
			Path:          p,
			Kind:          "file_path",
			OngridSource:  "file:" + p,
			LogQLSelector: logQLWithDevice(labelIDs, fmt.Sprintf("ongrid_source=\"file:%s\"", escapeLogQLLabel(p))),
			Indexed:       false,
		}
	}
}

func sortedUnits(m map[string]LogSourceUnit) []LogSourceUnit {
	out := make([]LogSourceUnit, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Unit < out[j].Unit })
	return out
}

func sortedContainers(m map[string]LogSourceContainer) []LogSourceContainer {
	out := make([]LogSourceContainer, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Container < out[j].Container })
	return out
}

func sortedFiles(m map[string]LogSourceFile) []LogSourceFile {
	out := make([]LogSourceFile, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func buildLogSourceHints(labelIDs []string, classified *classifiedSources) LogSourcesHints {
	base := deviceLogStreamSelector(labelIDs)
	h := LogSourcesHints{
		RecentErrorsExample: base + " |~ \"(?i)(error|panic|fatal)\"",
		ErrorCountExample:   "sum(count_over_time(" + base + " |~ \"(?i)error\"[5m]))",
	}
	if classified != nil && len(classified.containers) == 1 {
		c := sortedContainers(classified.containers)[0]
		h.RecentErrorsExample = c.LogQLSelector + " |~ \"(?i)(error|panic|fatal)\""
		h.ErrorCountExample = "sum(count_over_time(" + c.LogQLSelector + " |~ \"(?i)error\"[5m]))"
	}
	return h
}

func logsConfiguredFromPluginRows(rows []edgebiz.PluginRow) *LogSourcesConfigured {
	for _, row := range rows {
		if row.PluginName != edgemodel.PluginNameLogs {
			continue
		}
		cfg := &LogSourcesConfigured{
			LogsPluginEnabled: row.Enabled,
			EnableJournald:    true,
			EnableDockerAPI:   false,
		}
		spec := row.Spec
		if v, ok := spec["enable_journald"].(bool); ok {
			cfg.EnableJournald = v
		}
		if v, ok := spec["enable_docker_api"].(bool); ok {
			cfg.EnableDockerAPI = v
		}
		cfg.JournaldUnits = stringSliceFromSpec(spec, "journald_units")
		cfg.FilePaths = stringSliceFromSpec(spec, "file_paths")
		return cfg
	}
	return nil
}

func stringSliceFromSpec(spec map[string]interface{}, key string) []string {
	raw, ok := spec[key]
	if !ok {
		return nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func resolveLogSourceDeviceIDs(ctx context.Context, devices *devicebiz.Usecase, in QueryLogSourcesArgs) ([]uint64, error) {
	seen := make(map[uint64]struct{})
	var ids []uint64
	add := func(id uint64) {
		if id == 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, id := range in.DeviceIDs {
		add(id)
	}
	systemName := strings.TrimSpace(in.SystemName)
	if systemName != "" && devices != nil {
		all, err := devices.List(ctx, devicebiz.ListFilter{SystemName: systemName, Limit: 5000})
		if err != nil {
			return nil, err
		}
		if len(all) == 0 {
			return ids, nil
		}
		if len(in.DeviceIDs) == 0 {
			for _, d := range all {
				add(d.ID)
			}
		} else {
			allowed := make(map[uint64]struct{})
			for _, d := range all {
				allowed[d.ID] = struct{}{}
			}
			filtered := make([]uint64, 0, len(ids))
			for _, id := range ids {
				if _, ok := allowed[id]; ok {
					filtered = append(filtered, id)
				}
			}
			ids = filtered
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("query_log_sources: no devices matched (set device_ids or system_name)")
	}
	if len(ids) > 16 {
		return nil, fmt.Errorf("query_log_sources: too many devices (%d); max 16 per call", len(ids))
	}
	return ids, nil
}

func (r *Registry) queryLogSourcesForDevice(
	ctx context.Context,
	deviceID uint64,
	start, end time.Time,
	includeConfigured bool,
) DeviceLogSources {
	entry := DeviceLogSources{DeviceID: deviceID}
	labelIDs := logLabelIDsForDevice(ctx, r.devices, deviceID)
	entry.LogLabelIDs = labelIDs

	if r.devices != nil {
		if d, err := r.devices.Get(ctx, deviceID); err == nil && d != nil {
			entry.Name = d.Name
			entry.SystemName = d.SystemName
		}
	}

	if r.logQuery == nil {
		entry.Error = "loki client not configured"
		return entry
	}

	selector := deviceLogStreamSelector(labelIDs)
	series, err := r.logQuery.Series(ctx, []string{selector}, start, end)
	if err != nil {
		entry.Error = fmt.Sprintf("loki series: %v", err)
		return entry
	}

	classified := classifyLokiSeries(labelIDs, series)

	if includeConfigured && r.pluginConfigs != nil && r.devices != nil {
		edgeID, eerr := r.devices.LookupEdgeForDevice(ctx, deviceID)
		if eerr == nil && edgeID != 0 {
			if rows, lerr := r.pluginConfigs.ListForUI(ctx, edgeID); lerr == nil {
				cfg := logsConfiguredFromPluginRows(rows)
				if cfg != nil {
					entry.Configured = cfg
					mergeConfiguredSources(classified, labelIDs, cfg)
				}
			}
		}
	}

	entry.Units = sortedUnits(classified.units)
	entry.Containers = sortedContainers(classified.containers)
	entry.Files = sortedFiles(classified.files)
	entry.Hints = buildLogSourceHints(labelIDs, classified)
	return entry
}

func (r *Registry) executeQueryLogSources(ctx context.Context, args json.RawMessage) (ExecuteResult, error) {
	if r.logQuery == nil {
		return ExecuteResult{}, fmt.Errorf("query_log_sources: log query client not configured")
	}
	if r.devices == nil {
		return ExecuteResult{}, fmt.Errorf("query_log_sources: device usecase not configured")
	}

	var in QueryLogSourcesArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return ExecuteResult{}, fmt.Errorf("query_log_sources: bad args: %w", err)
	}

	lookback, err := parseLogLookback(in.Lookback)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("query_log_sources: %w", err)
	}
	includeConfigured := true
	if in.IncludeConfigured != nil {
		includeConfigured = *in.IncludeConfigured
	}

	callCtx, cancel := context.WithTimeout(ctx, queryLogSourcesCallTimeout)
	defer cancel()

	deviceIDs, err := resolveLogSourceDeviceIDs(callCtx, r.devices, in)
	if err != nil {
		return ExecuteResult{}, err
	}

	end := time.Now().UTC()
	start := end.Add(-lookback)

	devicesOut := make([]DeviceLogSources, 0, len(deviceIDs))
	for _, id := range deviceIDs {
		devicesOut = append(devicesOut, r.queryLogSourcesForDevice(callCtx, id, start, end, includeConfigured))
	}

	payload := map[string]any{
		"window": map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   end.Format(time.RFC3339),
		},
		"lookback": lookback.String(),
		"devices":  devicesOut,
		"count":    len(devicesOut),
	}
	if strings.TrimSpace(in.SystemName) != "" {
		payload["system_name"] = strings.TrimSpace(in.SystemName)
	}
	payload["workflow"] = "1) query_systems/query_devices to pick devices → 2) query_log_sources → 3) query_logql with returned logql_selector"

	out, err := json.Marshal(payload)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("query_log_sources: marshal: %w", err)
	}
	return ExecuteResult{ResultJSON: out}, nil
}
