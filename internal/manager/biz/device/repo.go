// Package device is the manager/device biz tier. It owns persistence of
// Device rows and the edge_devices M:N junction, plus the upsert
// primitive used by the edge agent register flow.
package device

import (
	"context"

	model "github.com/ongridio/ongrid/internal/manager/model/device"
)

// ListFilter narrows Device.List results.
//
// RolesAny is a bit mask: list rows whose roles bit-set overlaps with
// this mask. Stays sargable in the SQL impl by translating to a finite
// IN-list of stored values (see model.MatchingRoleValues).
// RolesUnknownOnly, when true, narrows to rows with roles == 0 (the
// "未分类" bucket); it is mutually exclusive with RolesAny — set one or
// the other, not both. Online filters by the live online flag.
// Hostname / Name / IPAddress are substring matches.
type ListFilter struct {
	RolesAny         uint8
	RolesUnknownOnly bool
	Online           *bool
	Hostname         string
	Name             string
	SystemName       string
	IPAddress        string
	Limit            int
	Offset           int
}

// SystemEnvironment is one (system_name, environment_tag) bucket in the fleet.
type SystemEnvironment struct {
	SystemName     string
	EnvironmentTag string
}

// Repo is the device persistence contract. The sqlite/mysql implementation
// lives under internal/manager/data/device.
type Repo interface {
	// FindOrCreateByFingerprint returns the existing Device for the
	// (Fingerprint) key or creates a fresh row carrying the provided
	// fields. UserID, Hostname/OS/etc. on the seed are only written on
	// initial create — subsequent calls do NOT overwrite host facts;
	// use UpdateHostFacts for that.
	FindOrCreateByFingerprint(ctx context.Context, seed *model.Device) (*model.Device, error)

	// RebindFingerprint moves a device from oldFP to newFP in place when
	// newFP isn't already taken — migrates a device to a new fingerprint
	// algorithm without orphaning it (device.ID and history preserved).
	// No-op when oldFP == newFP, either side is empty, or newFP already
	// exists (the new device already won; nothing to migrate).
	RebindFingerprint(ctx context.Context, oldFP, newFP string) error

	// UpdateHostFacts overwrites Hostname / OS / Arch / KernelVersion /
	// CPUCount / MemTotalBytes / DiskTotalBytes / OSVersion for the
	// device. Called after a fresh register payload arrives so we always
	// keep the latest facts.
	UpdateHostFacts(ctx context.Context, id uint64, facts HostFacts) error

	// UpdateUsage refreshes the live usage gauges (CPU/Mem/Disk %).
	// Called from the metric ingest path so the device list shows live
	// load without a JOIN onto host_metrics every render.
	UpdateUsage(ctx context.Context, id uint64, u Usage) error

	// UpdateRoles sets the operator-assigned device-roles bit mask.
	// Caller is expected to have already masked the value down to the
	// known-bits envelope (model.RolesAllKnownBits); the DB CHECK
	// constraint is the second line of defense.
	UpdateRoles(ctx context.Context, id uint64, roles uint8) error

	// UpdateNameDescription updates operator-editable display fields.
	UpdateNameDescription(ctx context.Context, id uint64, name, description string) error

	// UpdateHostname overwrites the operator-edited hostname on the device row.
	// Does not change fingerprint or agent-reported facts on next register.
	UpdateHostname(ctx context.Context, id uint64, hostname string) error

	// UpdateOperatorMeta updates operator-assigned system / IP / environment metadata.
	UpdateOperatorMeta(ctx context.Context, id uint64, systemName, deviceIP, environmentTag string) error

	// SetNodeID writes Device.NodeID — the link to the topology
	// `nodes` table. Called from the edge register flow (via NodeMirror)
	// after a fresh device is created or from the topology migration
	// backfill. Idempotent: writing the same node_id twice is a no-op.
	SetNodeID(ctx context.Context, id, nodeID uint64) error

	// MarkOnline / MarkOffline set the device-level online flag and
	// timestamp. Called from the edge online/offline callbacks.
	MarkOnline(ctx context.Context, id uint64) error
	MarkOffline(ctx context.Context, id uint64) error

	// Get returns the row by id; ErrNotFound otherwise.
	Get(ctx context.Context, id uint64) (*model.Device, error)
	// GetMany batch-loads devices by id. Missing ids are simply absent
	// from the returned map; callers must handle the no-row case.
	GetMany(ctx context.Context, ids []uint64) (map[uint64]*model.Device, error)
	// List returns devices matching f. Sorted by id DESC (newest first).
	// Soft-deleted rows excluded.
	List(ctx context.Context, f ListFilter) ([]*model.Device, error)
	// Count returns the total non-soft-deleted device count.
	Count(ctx context.Context) (int64, error)
	// ListDistinctSystemNames returns sorted non-empty operator-assigned
	// system_name values across the fleet.
	ListDistinctSystemNames(ctx context.Context) ([]string, error)
	// ListSystemEnvironmentPairs returns distinct (system_name, environment_tag)
	// pairs seen in the fleet (environment_tag may be empty).
	ListSystemEnvironmentPairs(ctx context.Context) ([]SystemEnvironment, error)
	// Delete hard-deletes a device and does NOT touch junction rows;
	// edge deletion cascades through edge usecase instead.
	Delete(ctx context.Context, id uint64) error

	// DeleteOfflineWithLinkedEdges hard-deletes an offline device, its
	// edge_devices junction rows, and every linked Edge row in one
	// transaction. It must reject online devices (and online linked
	// edges) with ErrConflict so callers never remove a live host.
	DeleteOfflineWithLinkedEdges(ctx context.Context, id uint64) error

	// ReconcileOfflineOrphans flips online=true devices back to offline
	// when none of their linked (non-deleted) edges is online. Heals
	// orphan "ghost" devices — a device whose edge was deleted, or whose
	// host re-registered under a new fingerprint, used to stay online
	// forever because only a tunnel-close (HandleOffline) flipped it.
	// Returns the number of rows flipped. Run periodically by the
	// presence reconciler.
	ReconcileOfflineOrphans(ctx context.Context) (int64, error)
}

// HostFacts is the subset of Device columns updated on register.
type HostFacts struct {
	Hostname       string
	OS             string
	OSVersion      string
	Arch           string
	KernelVersion  string
	CPUCount       int
	MemTotalBytes  uint64
	DiskTotalBytes uint64
	IPAddress      string
}

// Usage is the live percentage gauges for one device.
type Usage struct {
	CPUPct  float32
	MemPct  float32
	DiskPct float32
}
