package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	bizsetting "github.com/ongridio/ongrid/internal/manager/biz/setting"
	alertmodel "github.com/ongridio/ongrid/internal/manager/model/alert"
	devicemodel "github.com/ongridio/ongrid/internal/manager/model/device"
	settingmodel "github.com/ongridio/ongrid/internal/manager/model/setting"
	"github.com/ongridio/ongrid/internal/pkg/errs"
)

// DeviceTargetResolver resolves a host device's business system and environment.
type DeviceTargetResolver interface {
	TargetForDevice(ctx context.Context, deviceID uint64) (systemName, environmentTag string, err error)
}

// DeviceGetter adapts a Get(ctx, id) device repo method.
type DeviceGetter func(ctx context.Context, id uint64) (*devicemodel.Device, error)

// DeviceTargetFromGetter wraps a device Get function as DeviceTargetResolver.
func DeviceTargetFromGetter(get DeviceGetter) DeviceTargetResolver {
	if get == nil {
		return nil
	}
	return deviceTargetGetter{get: get}
}

type deviceTargetGetter struct {
	get DeviceGetter
}

func (d deviceTargetGetter) TargetForDevice(ctx context.Context, deviceID uint64) (string, string, error) {
	if deviceID == 0 {
		return "", "", nil
	}
	dev, err := d.get(ctx, deviceID)
	if err != nil || dev == nil {
		return "", "", err
	}
	return strings.TrimSpace(dev.SystemName), normalizeEnvironmentTag(dev.EnvironmentTag), nil
}

// SystemEnvironmentPair is one (system_name, environment_tag) seen in the fleet.
type SystemEnvironmentPair struct {
	SystemName     string `json:"system_name"`
	EnvironmentTag string `json:"environment_tag"`
}

// SystemDeviceLister lists distinct operator-assigned system_name values.
type SystemDeviceLister interface {
	ListDistinctSystemNames(ctx context.Context) ([]string, error)
	ListSystemEnvironmentPairs(ctx context.Context) ([]SystemEnvironmentPair, error)
}

// notifyChannelCatalog lists and validates notification channel rows.
type notifyChannelCatalog interface {
	ListNotificationChannels(ctx context.Context) ([]*alertmodel.Channel, error)
	GetChannelByID(ctx context.Context, id uint64) (*alertmodel.Channel, error)
}

// SystemNotifyService manages per-system notification channel bindings.
type SystemNotifyService struct {
	settings *bizsetting.Service
	systems  SystemDeviceLister
	channels notifyChannelCatalog
}

// SystemNotifyBinding is one row in the settings UI.
type SystemNotifyBinding struct {
	SystemName     string   `json:"system_name"`
	EnvironmentTag string   `json:"environment_tag,omitempty"`
	ChannelIDs     []uint64 `json:"channel_ids"`
}

// SystemNotifyChannelDTO is a channel row shown in the picker.
type SystemNotifyChannelDTO struct {
	ID      uint64 `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

// SystemNotifyView is returned by GET /v1/alert-settings/system-notify.
type SystemNotifyView struct {
	Systems             []string                 `json:"systems"`
	SystemEnvironments  []SystemEnvironmentPair  `json:"system_environments"`
	Bindings            []SystemNotifyBinding    `json:"bindings"`
	Channels            []SystemNotifyChannelDTO `json:"channels"`
	EnvironmentTags     []string                 `json:"environment_tags"`
}

// NewSystemNotifyService wires system→channel bindings. systems and
// channels may be nil (empty lists in GET; validation on PUT is
// channel-id only when catalog is available).
func NewSystemNotifyService(settings *bizsetting.Service, systems SystemDeviceLister, channels notifyChannelCatalog) *SystemNotifyService {
	return &SystemNotifyService{settings: settings, systems: systems, channels: channels}
}

// Get returns known systems, current bindings, and the channel catalog.
func (s *SystemNotifyService) Get(ctx context.Context) (SystemNotifyView, error) {
	out := SystemNotifyView{
		Bindings:        []SystemNotifyBinding{},
		Channels:        []SystemNotifyChannelDTO{},
		Systems:         []string{},
		SystemEnvironments: []SystemEnvironmentPair{},
		EnvironmentTags: knownEnvironmentTags(),
	}
	if s.systems != nil {
		names, err := s.systems.ListDistinctSystemNames(ctx)
		if err != nil {
			return out, err
		}
		out.Systems = names
		pairs, err := s.systems.ListSystemEnvironmentPairs(ctx)
		if err != nil {
			return out, err
		}
		out.SystemEnvironments = pairs
	}
	if s.channels != nil {
		rows, err := s.channels.ListNotificationChannels(ctx)
		if err != nil {
			return out, err
		}
		for _, ch := range rows {
			if ch == nil {
				continue
			}
			out.Channels = append(out.Channels, SystemNotifyChannelDTO{
				ID:      ch.ID,
				Name:    ch.Name,
				Type:    ch.ChannelType,
				Enabled: ch.Enabled,
			})
		}
	}
	bindings, err := s.loadBindings(ctx)
	if err != nil {
		return out, err
	}
	out.Bindings = bindingsToList(bindings, out.Systems, out.SystemEnvironments)
	return out, nil
}

// systemEnvBindings maps system_name → environment_tag → channel ids.
// environment_tag "" means all environments for that system.
type systemEnvBindings map[string]map[string][]uint64

// Set replaces all system→channel bindings.
func (s *SystemNotifyService) Set(ctx context.Context, bindings []SystemNotifyBinding) (SystemNotifyView, error) {
	if s.settings == nil {
		return SystemNotifyView{}, fmt.Errorf("%w: system notify settings unavailable", errs.ErrNotWiredYet)
	}
	normalized := make(systemEnvBindings)
	for _, b := range bindings {
		name := strings.TrimSpace(b.SystemName)
		if name == "" {
			continue
		}
		env := normalizeEnvironmentTag(b.EnvironmentTag)
		if env != "" && !devicemodel.IsValidEnvironmentTag(env) {
			return SystemNotifyView{}, fmt.Errorf("%w: invalid environment_tag %q", errs.ErrInvalid, b.EnvironmentTag)
		}
		ids := uniqueUint64(b.ChannelIDs)
		if len(ids) == 0 {
			continue
		}
		if err := s.validateChannelIDs(ctx, ids); err != nil {
			return SystemNotifyView{}, err
		}
		if normalized[name] == nil {
			normalized[name] = map[string][]uint64{}
		}
		normalized[name][env] = ids
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return SystemNotifyView{}, err
	}
	if len(normalized) == 0 {
		_ = s.settings.Delete(ctx, settingmodel.CategoryAlert, settingmodel.KeySystemNotifyBindings)
	} else if err := s.settings.Set(ctx, settingmodel.CategoryAlert, settingmodel.KeySystemNotifyBindings, string(raw), false); err != nil {
		return SystemNotifyView{}, err
	}
	return s.Get(ctx)
}

// ChannelIDsForTarget returns pinned channel ids for system + environment.
// Empty environmentTag matches bindings stored under "" (all envs) when no
// exact env binding exists.
func (s *SystemNotifyService) ChannelIDsForTarget(ctx context.Context, systemName, environmentTag string) []uint64 {
	if s == nil || s.settings == nil {
		return nil
	}
	systemName = strings.TrimSpace(systemName)
	if systemName == "" {
		return nil
	}
	environmentTag = normalizeEnvironmentTag(environmentTag)
	m, err := s.loadBindingsMap(ctx)
	if err != nil {
		return nil
	}
	envMap := m[systemName]
	if len(envMap) == 0 {
		return nil
	}
	if ids := envMap[environmentTag]; len(ids) > 0 {
		return append([]uint64(nil), ids...)
	}
	if environmentTag != "" {
		if ids := envMap[""]; len(ids) > 0 {
			return append([]uint64(nil), ids...)
		}
	}
	return nil
}

// ChannelIDsForSystem is kept for tests / legacy callers.
func (s *SystemNotifyService) ChannelIDsForSystem(ctx context.Context, systemName string) []uint64 {
	return s.ChannelIDsForTarget(ctx, systemName, "")
}

func (s *SystemNotifyService) loadBindings(ctx context.Context) (systemEnvBindings, error) {
	m, err := s.loadBindingsMap(ctx)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return systemEnvBindings{}, nil
	}
	return m, nil
}

func (s *SystemNotifyService) loadBindingsMap(ctx context.Context) (systemEnvBindings, error) {
	if s.settings == nil {
		return systemEnvBindings{}, nil
	}
	raw, ok, err := s.settings.Get(ctx, settingmodel.CategoryAlert, settingmodel.KeySystemNotifyBindings)
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return systemEnvBindings{}, err
	}
	return parseSystemEnvBindings(raw)
}

func parseSystemEnvBindings(raw string) (systemEnvBindings, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return nil, fmt.Errorf("system notify bindings: %w", err)
	}
	out := make(systemEnvBindings, len(top))
	for system, val := range top {
		system = strings.TrimSpace(system)
		if system == "" {
			continue
		}
		val = bytesTrim(val)
		if len(val) == 0 {
			continue
		}
		switch val[0] {
		case '[':
			var ids []uint64
			if err := json.Unmarshal(val, &ids); err != nil {
				return nil, fmt.Errorf("system notify bindings for %q: %w", system, err)
			}
			out[system] = map[string][]uint64{"": ids}
		case '{':
			var envMap map[string][]uint64
			if err := json.Unmarshal(val, &envMap); err != nil {
				return nil, fmt.Errorf("system notify bindings for %q: %w", system, err)
			}
			normalized := make(map[string][]uint64, len(envMap))
			for env, ids := range envMap {
				normalized[normalizeEnvironmentTag(env)] = ids
			}
			out[system] = normalized
		default:
			return nil, fmt.Errorf("system notify bindings for %q: unexpected JSON shape", system)
		}
	}
	return out, nil
}

func bytesTrim(raw json.RawMessage) json.RawMessage {
	return json.RawMessage(strings.TrimSpace(string(raw)))
}

func (s *SystemNotifyService) validateChannelIDs(ctx context.Context, ids []uint64) error {
	if s.channels == nil {
		return nil
	}
	for _, id := range ids {
		ch, err := s.channels.GetChannelByID(ctx, id)
		if err != nil || ch == nil {
			return fmt.Errorf("%w: notification channel %d is not available", errs.ErrInvalid, id)
		}
	}
	return nil
}

func bindingsToList(m systemEnvBindings, systems []string, pairs []SystemEnvironmentPair) []SystemNotifyBinding {
	seen := make(map[string]struct{})
	out := make([]SystemNotifyBinding, 0)

	add := func(name, env string) {
		key := bindingKey(name, env)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		var ids []uint64
		if envMap := m[name]; envMap != nil {
			ids = append([]uint64(nil), envMap[env]...)
		}
		out = append(out, SystemNotifyBinding{
			SystemName:     name,
			EnvironmentTag: env,
			ChannelIDs:     ids,
		})
	}

	for _, name := range systems {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		envs := environmentsForSystem(name, pairs)
		if len(envs) == 0 {
			add(name, "")
			continue
		}
		for _, env := range envs {
			add(name, env)
		}
		add(name, "")
	}
	for name, envMap := range m {
		for env, ids := range envMap {
			key := bindingKey(name, env)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, SystemNotifyBinding{
				SystemName:     name,
				EnvironmentTag: env,
				ChannelIDs:     append([]uint64(nil), ids...),
			})
		}
	}
	return out
}

func environmentsForSystem(system string, pairs []SystemEnvironmentPair) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, p := range pairs {
		if strings.TrimSpace(p.SystemName) != system {
			continue
		}
		env := normalizeEnvironmentTag(p.EnvironmentTag)
		if env == "" {
			continue
		}
		if _, ok := seen[env]; ok {
			continue
		}
		seen[env] = struct{}{}
		out = append(out, env)
	}
	return out
}

func bindingKey(system, env string) string {
	return system + "\x00" + env
}

func normalizeEnvironmentTag(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func knownEnvironmentTags() []string {
	return []string{"dev", "test", "prod"}
}

func uniqueUint64(in []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(in))
	out := make([]uint64, 0, len(in))
	for _, id := range in {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
