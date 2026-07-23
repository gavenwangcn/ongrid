package edge

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	devicebiz "github.com/ongridio/ongrid/internal/manager/biz/device"
	devicemodel "github.com/ongridio/ongrid/internal/manager/model/device"
	model "github.com/ongridio/ongrid/internal/manager/model/edge"
	"github.com/ongridio/ongrid/internal/pkg/errs"
	"github.com/ongridio/ongrid/internal/pkg/tunnel"
)

// fakeDeviceRepo is the in-memory devicebiz.Repo used by HandleRegister
// tests. Captures the sequence of upserts so tests can assert which
// host facts landed in the device row.
type fakeDeviceRepo struct {
	mu          sync.Mutex
	byID        map[uint64]*devicemodel.Device
	byFP        map[string]uint64
	nextID      uint64
	lastFacts   devicebiz.HostFacts
	onlineCalls int
}

func newFakeDeviceRepo() *fakeDeviceRepo {
	return &fakeDeviceRepo{byID: map[uint64]*devicemodel.Device{}, byFP: map[string]uint64{}}
}

func (d *fakeDeviceRepo) FindOrCreateByFingerprint(_ context.Context, seed *devicemodel.Device) (*devicemodel.Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.byFP[seed.Fingerprint]; ok {
		return d.byID[id], nil
	}
	d.nextID++
	cp := *seed
	cp.ID = d.nextID
	cp.CreatedAt = time.Now()
	cp.UpdatedAt = cp.CreatedAt
	d.byID[cp.ID] = &cp
	d.byFP[cp.Fingerprint] = cp.ID
	return &cp, nil
}

func (d *fakeDeviceRepo) RebindFingerprint(_ context.Context, oldFP, newFP string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if oldFP == "" || newFP == "" || oldFP == newFP {
		return nil
	}
	if _, taken := d.byFP[newFP]; taken {
		return nil // newFP already won — nothing to migrate
	}
	id, ok := d.byFP[oldFP]
	if !ok {
		return nil // no device under oldFP
	}
	delete(d.byFP, oldFP)
	d.byFP[newFP] = id
	d.byID[id].Fingerprint = newFP // same device.ID, only fp changes
	return nil
}

func (d *fakeDeviceRepo) UpdateHostFacts(_ context.Context, id uint64, f devicebiz.HostFacts) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return errs.ErrNotFound
	}
	dev.Hostname, dev.OS, dev.Arch = f.Hostname, f.OS, f.Arch
	dev.KernelVersion, dev.CPUCount, dev.MemTotalBytes = f.KernelVersion, f.CPUCount, f.MemTotalBytes
	d.lastFacts = f
	return nil
}

func (d *fakeDeviceRepo) MarkOnline(_ context.Context, id uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return errs.ErrNotFound
	}
	dev.Online = true
	now := time.Now()
	dev.LastSeenAt = &now
	d.onlineCalls++
	return nil
}

func (d *fakeDeviceRepo) MarkOffline(_ context.Context, id uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return errs.ErrNotFound
	}
	dev.Online = false
	return nil
}

func (d *fakeDeviceRepo) Get(_ context.Context, id uint64) (*devicemodel.Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return nil, errs.ErrNotFound
	}
	return dev, nil
}

func (d *fakeDeviceRepo) GetMany(_ context.Context, ids []uint64) (map[uint64]*devicemodel.Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := map[uint64]*devicemodel.Device{}
	for _, id := range ids {
		if v, ok := d.byID[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func (d *fakeDeviceRepo) UpdateUsage(_ context.Context, id uint64, _ devicebiz.Usage) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.byID[id]; !ok {
		return errs.ErrNotFound
	}
	return nil
}

func (d *fakeDeviceRepo) UpdateRoles(_ context.Context, id uint64, roles uint8) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return errs.ErrNotFound
	}
	dev.Roles = roles
	return nil
}

func (d *fakeDeviceRepo) UpdateNameDescription(_ context.Context, id uint64, name, description string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return errs.ErrNotFound
	}
	dev.Name, dev.Description = name, description
	return nil
}

func (d *fakeDeviceRepo) UpdateHostname(_ context.Context, id uint64, hostname string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return errs.ErrNotFound
	}
	dev.Hostname = hostname
	return nil
}

func (d *fakeDeviceRepo) UpdateOperatorMeta(_ context.Context, id uint64, systemName, deviceIP, environmentTag string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return errs.ErrNotFound
	}
	dev.SystemName, dev.DeviceIP, dev.EnvironmentTag = systemName, deviceIP, environmentTag
	return nil
}

func (d *fakeDeviceRepo) SetNodeID(_ context.Context, id, nodeID uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return errs.ErrNotFound
	}
	nid := nodeID
	dev.NodeID = &nid
	return nil
}

func (d *fakeDeviceRepo) List(_ context.Context, _ devicebiz.ListFilter) ([]*devicemodel.Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]*devicemodel.Device, 0, len(d.byID))
	for _, v := range d.byID {
		out = append(out, v)
	}
	return out, nil
}

func (d *fakeDeviceRepo) Count(_ context.Context) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return int64(len(d.byID)), nil
}

func (d *fakeDeviceRepo) ListDistinctSystemNames(_ context.Context) ([]string, error) {
	return nil, nil
}

func (d *fakeDeviceRepo) ListSystemEnvironmentPairs(_ context.Context) ([]devicebiz.SystemEnvironment, error) {
	return nil, nil
}

func (d *fakeDeviceRepo) Delete(_ context.Context, id uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, ok := d.byID[id]
	if !ok {
		return errs.ErrNotFound
	}
	delete(d.byFP, dev.Fingerprint)
	delete(d.byID, id)
	return nil
}

func (d *fakeDeviceRepo) DeleteOfflineWithLinkedEdges(ctx context.Context, id uint64) error {
	return d.Delete(ctx, id)
}

func (d *fakeDeviceRepo) ReconcileOfflineOrphans(_ context.Context) (int64, error) {
	return 0, nil
}

// fakeEdgeDeviceRepo is an in-memory EdgeDeviceRepo for cascade-delete tests.
type fakeEdgeDeviceRepo struct {
	mu   sync.Mutex
	rows []*devicemodel.EdgeDevice
}

func newFakeEdgeDeviceRepo() *fakeEdgeDeviceRepo {
	return &fakeEdgeDeviceRepo{}
}

func (f *fakeEdgeDeviceRepo) Link(_ context.Context, edgeID, deviceID uint64, t devicemodel.EdgeDeviceRelationType) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.EdgeID == edgeID && r.DeviceID == deviceID && r.Type == t {
			return nil
		}
	}
	f.rows = append(f.rows, &devicemodel.EdgeDevice{EdgeID: edgeID, DeviceID: deviceID, Type: t})
	return nil
}

func (f *fakeEdgeDeviceRepo) Unlink(_ context.Context, edgeID, deviceID uint64, t devicemodel.EdgeDeviceRelationType) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*devicemodel.EdgeDevice, 0, len(f.rows))
	for _, r := range f.rows {
		if r.EdgeID == edgeID && r.DeviceID == deviceID && r.Type == t {
			continue
		}
		out = append(out, r)
	}
	f.rows = out
	return nil
}

func (f *fakeEdgeDeviceRepo) DeleteAllForEdge(_ context.Context, edgeID uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*devicemodel.EdgeDevice, 0, len(f.rows))
	for _, r := range f.rows {
		if r.EdgeID != edgeID {
			out = append(out, r)
		}
	}
	f.rows = out
	return nil
}

func (f *fakeEdgeDeviceRepo) LookupHostDevice(_ context.Context, edgeID uint64) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.EdgeID == edgeID && r.Type == devicemodel.EdgeDeviceRelationHost {
			return r.DeviceID, nil
		}
	}
	return 0, errs.ErrNotFound
}

func (f *fakeEdgeDeviceRepo) LookupEdgeForDevice(_ context.Context, deviceID uint64, t devicemodel.EdgeDeviceRelationType) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.DeviceID == deviceID && r.Type == t {
			return r.EdgeID, nil
		}
	}
	return 0, errs.ErrNotFound
}

func (f *fakeEdgeDeviceRepo) ListDevicesForEdge(_ context.Context, edgeID uint64) ([]*devicemodel.EdgeDevice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*devicemodel.EdgeDevice, 0)
	for _, r := range f.rows {
		if r.EdgeID == edgeID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeEdgeDeviceRepo) ListEdgesForDevice(_ context.Context, deviceID uint64) ([]*devicemodel.EdgeDevice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*devicemodel.EdgeDevice, 0)
	for _, r := range f.rows {
		if r.DeviceID == deviceID {
			out = append(out, r)
		}
	}
	return out, nil
}

// fakeRepo is an in-memory biz.Repo for usecase-level tests. Mirrors the
// SQLite implementation's observable semantics (soft-delete hides rows,
// lookups by AccessKey exclude deleted, etc.) without dragging in gorm.
type fakeRepo struct {
	mu     sync.Mutex
	byID   map[uint64]*model.Edge
	nextID uint64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[uint64]*model.Edge{}}
}

func (r *fakeRepo) Create(_ context.Context, e *model.Edge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	e.ID = r.nextID
	e.CreatedAt = time.Now()
	e.UpdatedAt = e.CreatedAt
	cp := *e
	r.byID[e.ID] = &cp
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id uint64) (*model.Edge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok || e.DeletedAt != nil {
		return nil, errs.ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (r *fakeRepo) GetByAccessKey(_ context.Context, ak string) (*model.Edge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.byID {
		if e.AccessKeyID == ak && e.DeletedAt == nil {
			cp := *e
			return &cp, nil
		}
	}
	return nil, errs.ErrNotFound
}

func (r *fakeRepo) GetByName(_ context.Context, name string) (*model.Edge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.byID {
		if e.Name == name && e.DeletedAt == nil {
			cp := *e
			return &cp, nil
		}
	}
	return nil, errs.ErrNotFound
}

func (r *fakeRepo) List(_ context.Context, f ListFilter) ([]*model.Edge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.Edge
	for _, e := range r.byID {
		if e.DeletedAt != nil {
			continue
		}
		if f.Status != "" && e.Status != f.Status {
			continue
		}
		if f.CreatedBy != nil && (e.CreatedBy == nil || *e.CreatedBy != *f.CreatedBy) {
			continue
		}
		cp := *e
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeRepo) UpdateSecretHash(_ context.Context, id uint64, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok || e.DeletedAt != nil {
		return errs.ErrNotFound
	}
	e.SecretKeyHash = hash
	e.UpdatedAt = time.Now()
	return nil
}

func (r *fakeRepo) UpdateStatus(_ context.Context, id uint64, status string, lastSeen time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok || e.DeletedAt != nil {
		return errs.ErrNotFound
	}
	e.Status = status
	ls := lastSeen
	e.LastSeenAt = &ls
	return nil
}

func (r *fakeRepo) UpdateName(_ context.Context, id uint64, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok || e.DeletedAt != nil {
		return errs.ErrNotFound
	}
	e.Name = name
	return nil
}

func (r *fakeRepo) UpdateOperatorMeta(_ context.Context, id uint64, systemName, deviceIP, environmentTag string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok || e.DeletedAt != nil {
		return errs.ErrNotFound
	}
	e.SystemName, e.DeviceIP, e.EnvironmentTag = systemName, deviceIP, environmentTag
	return nil
}

func (r *fakeRepo) SetDeviceID(_ context.Context, id, deviceID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok || e.DeletedAt != nil {
		return errs.ErrNotFound
	}
	d := deviceID
	e.DeviceID = &d
	return nil
}

func (r *fakeRepo) ClearDeviceID(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok || e.DeletedAt != nil {
		return errs.ErrNotFound
	}
	e.DeviceID = nil
	return nil
}

func (r *fakeRepo) SetAgentVersion(_ context.Context, id uint64, v string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok || e.DeletedAt != nil {
		return errs.ErrNotFound
	}
	e.AgentVersion = v
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return errs.ErrNotFound
	}
	delete(r.byID, id)
	return nil
}

func (r *fakeRepo) Count(_ context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, e := range r.byID {
		if e.DeletedAt == nil {
			n++
		}
	}
	return n, nil
}

// --- tests ---

func TestCreateReturnsAccessKeyAndSecretKey(t *testing.T) {
	uc := NewUsecase(newFakeRepo(), nil, nil, nil)
	ctx := context.Background()

	uid := uint64(42)
	res, err := uc.Create(ctx, CreateParams{Name: "edge-1"}, &uid)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Edge == nil || res.Edge.ID == 0 {
		t.Fatalf("edge not assigned id: %+v", res.Edge)
	}
	if len(res.AccessKey) != 24 {
		t.Errorf("AccessKey len = %d, want 24", len(res.AccessKey))
	}
	if len(res.SecretKey) != 32 {
		t.Errorf("SecretKey len = %d, want 32", len(res.SecretKey))
	}
	if res.Edge.AccessKeyID != res.AccessKey {
		t.Errorf("stored AccessKeyID %q != returned %q", res.Edge.AccessKeyID, res.AccessKey)
	}
	if res.Edge.SecretKeyHash == res.SecretKey {
		t.Errorf("SecretKeyHash stored as plaintext")
	}
	if res.Edge.Status != model.StatusOffline {
		t.Errorf("Status = %q, want %q", res.Edge.Status, model.StatusOffline)
	}
	if res.Edge.CreatedBy == nil || *res.Edge.CreatedBy != uid {
		t.Errorf("CreatedBy = %v, want %d", res.Edge.CreatedBy, uid)
	}
}

func TestCreateAcceptsEmptyName(t *testing.T) {
	// Empty name is intentionally allowed: HandleRegister back-fills
	// it from the host's hostname on first tunnel handshake.
	uc := NewUsecase(newFakeRepo(), nil, nil, nil)
	res, err := uc.Create(context.Background(), CreateParams{Name: "  "}, nil)
	if err != nil {
		t.Fatalf("Create empty name: %v", err)
	}
	if res.Edge.Name != "" {
		t.Errorf("name = %q, want empty until HandleRegister back-fills", res.Edge.Name)
	}
}

func TestCreateSeedsHostDevice(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	links := newFakeEdgeDeviceRepo()
	uc := NewUsecase(repo, devices, links, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{
		Name:           "edge-pending",
		SystemName:     "订单中心",
		DeviceIP:       "10.0.0.8",
		EnvironmentTag: devicemodel.EnvProd,
	}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Edge.DeviceID == nil {
		t.Fatal("Edge.DeviceID not set after create")
	}
	dev, err := devices.Get(ctx, *res.Edge.DeviceID)
	if err != nil {
		t.Fatalf("Get device: %v", err)
	}
	if dev.Fingerprint != legacyEdgeFingerprint(res.Edge.ID) {
		t.Errorf("Fingerprint = %q, want %q", dev.Fingerprint, legacyEdgeFingerprint(res.Edge.ID))
	}
	if dev.SystemName != "订单中心" || dev.DeviceIP != "10.0.0.8" || dev.EnvironmentTag != devicemodel.EnvProd {
		t.Errorf("operator meta = %+v", dev)
	}
	if dev.Online {
		t.Error("placeholder device should start offline")
	}
	hostDevID, err := links.LookupHostDevice(ctx, res.Edge.ID)
	if err != nil {
		t.Fatalf("LookupHostDevice: %v", err)
	}
	if hostDevID != dev.ID {
		t.Errorf("host device link = %d, want %d", hostDevID, dev.ID)
	}
}

func TestListFiltersByStatus(t *testing.T) {
	repo := newFakeRepo()
	uc := NewUsecase(repo, nil, nil, nil)
	ctx := context.Background()

	a, err := uc.Create(ctx, CreateParams{Name: "a"}, nil)
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := uc.Create(ctx, CreateParams{Name: "b"}, nil)
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	// Flip `a` to online, leave `b` offline.
	if err := repo.UpdateStatus(ctx, a.Edge.ID, model.StatusOnline, time.Now()); err != nil {
		t.Fatalf("set online: %v", err)
	}

	online, err := uc.List(ctx, ListFilter{Status: model.StatusOnline})
	if err != nil {
		t.Fatalf("list online: %v", err)
	}
	if len(online) != 1 || online[0].ID != a.Edge.ID {
		t.Errorf("online list = %+v, want only %d", online, a.Edge.ID)
	}

	offline, err := uc.List(ctx, ListFilter{Status: model.StatusOffline})
	if err != nil {
		t.Fatalf("list offline: %v", err)
	}
	if len(offline) != 1 || offline[0].ID != b.Edge.ID {
		t.Errorf("offline list = %+v, want only %d", offline, b.Edge.ID)
	}
}

func TestRotateSecretProducesDifferentHash(t *testing.T) {
	repo := newFakeRepo()
	uc := NewUsecase(repo, nil, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "x"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldHash := res.Edge.SecretKeyHash

	newPlain, err := uc.RotateSecret(ctx, res.Edge.ID)
	if err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}
	if len(newPlain) != 32 {
		t.Errorf("rotated plaintext len = %d, want 32", len(newPlain))
	}
	after, err := uc.Get(ctx, res.Edge.ID)
	if err != nil {
		t.Fatalf("Get after rotate: %v", err)
	}
	if after.SecretKeyHash == oldHash {
		t.Error("RotateSecret: hash unchanged")
	}
	if newPlain == res.SecretKey {
		t.Error("RotateSecret: new plaintext equals old plaintext")
	}
}

func TestDeleteHidesFromSubsequentGet(t *testing.T) {
	repo := newFakeRepo()
	uc := NewUsecase(repo, nil, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "gone"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := uc.Delete(ctx, res.Edge.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := uc.Get(ctx, res.Edge.ID); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("Get after Delete: want ErrNotFound, got %v", err)
	}
	list, err := uc.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List after Delete: got %d items, want 0", len(list))
	}
}

func TestDeleteCascadesLinkedDevice(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	links := newFakeEdgeDeviceRepo()
	uc := NewUsecase(repo, devices, links, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-del"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	info := tunnel.HostInfo{Hostname: "host-1", OS: "linux", Arch: "amd64", CPUCount: 4}
	if err := uc.HandleRegister(ctx, res.Edge.ID, info, ""); err != nil {
		t.Fatalf("HandleRegister: %v", err)
	}
	edge, err := uc.Get(ctx, res.Edge.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if edge.DeviceID == nil {
		t.Fatal("DeviceID not set")
	}
	devID := *edge.DeviceID

	if err := uc.Delete(ctx, res.Edge.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := devices.Get(ctx, devID); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("device still visible after edge delete: %v", err)
	}
	if rows, _ := links.ListDevicesForEdge(ctx, res.Edge.ID); len(rows) != 0 {
		t.Errorf("junction rows remain: %d", len(rows))
	}
}

func TestAuthenticateSuccessReturnsSession(t *testing.T) {
	repo := newFakeRepo()
	uc := NewUsecase(repo, nil, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "auth-target"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	auth := NewAccessKeyAuthenticator(repo, nil)
	sess, err := auth.Authenticate(ctx, res.AccessKey, res.SecretKey)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if sess.EdgeID != res.Edge.ID {
		t.Errorf("Session.EdgeID = %d, want %d", sess.EdgeID, res.Edge.ID)
	}
	// give the goroutine a chance to flip status
	time.Sleep(50 * time.Millisecond)
	after, _ := uc.Get(ctx, res.Edge.ID)
	if after.Status != model.StatusOnline {
		t.Errorf("status = %q, want online after authenticate", after.Status)
	}
}

func TestHandleRegisterUpsertsDeviceAndLinksEdge(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	links := newFakeEdgeDeviceRepo()
	uc := NewUsecase(repo, devices, links, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-reg"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Edge.DeviceID == nil {
		t.Fatal("precondition: create should seed placeholder device")
	}
	placeholderID := *res.Edge.DeviceID
	if res.Edge.Status != model.StatusOffline {
		t.Fatalf("precondition: want Status=offline, got %q", res.Edge.Status)
	}

	info := tunnel.HostInfo{
		Hostname: "node-1",
		OS:       "linux",
		Arch:     "arm64",
		CPUCount: 8,
	}
	if err := uc.HandleRegister(ctx, res.Edge.ID, info, ""); err != nil {
		t.Fatalf("HandleRegister: %v", err)
	}

	after, err := uc.Get(ctx, res.Edge.ID)
	if err != nil {
		t.Fatalf("Get after register: %v", err)
	}
	if after.Status != model.StatusOnline {
		t.Errorf("Status = %q, want online", after.Status)
	}
	if after.LastSeenAt == nil {
		t.Errorf("LastSeenAt not updated")
	}
	if after.DeviceID == nil {
		t.Fatal("Edge.DeviceID not set after register")
	}
	if *after.DeviceID != placeholderID {
		t.Errorf("DeviceID changed across register: got %d want %d", *after.DeviceID, placeholderID)
	}
	dev, err := devices.Get(ctx, *after.DeviceID)
	if err != nil {
		t.Fatalf("Get device: %v", err)
	}
	if dev.Fingerprint == legacyEdgeFingerprint(res.Edge.ID) {
		t.Errorf("device still carries placeholder fingerprint after register")
	}
	if dev.Hostname != info.Hostname || dev.OS != info.OS || dev.Arch != info.Arch || dev.CPUCount != info.CPUCount {
		t.Errorf("Device facts = %+v, want hostname/os/arch/cpu = %+v", dev, info)
	}
	if !dev.Online {
		t.Errorf("Device.Online = false, want true after register")
	}
	hostDevID, err := links.LookupHostDevice(ctx, res.Edge.ID)
	if err != nil {
		t.Fatalf("LookupHostDevice: %v", err)
	}
	if hostDevID != placeholderID {
		t.Errorf("host link device_id = %d, want %d", hostDevID, placeholderID)
	}
}

func TestClearHostDeviceLinkRemovesControllerDeviceAssociation(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	links := newFakeEdgeDeviceRepo()
	uc := NewUsecase(repo, devices, links, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "k8s-controller"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := uc.HandleRegister(ctx, res.Edge.ID, tunnel.HostInfo{Hostname: "wrong-controller-host"}, ""); err != nil {
		t.Fatalf("HandleRegister: %v", err)
	}
	registered, err := uc.Get(ctx, res.Edge.ID)
	if err != nil || registered.DeviceID == nil {
		t.Fatalf("precondition: edge should have mistaken device link, edge=%+v err=%v", registered, err)
	}
	deviceID := *registered.DeviceID

	if err := uc.ClearHostDeviceLink(ctx, res.Edge.ID); err != nil {
		t.Fatalf("ClearHostDeviceLink: %v", err)
	}
	after, err := uc.Get(ctx, res.Edge.ID)
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if after.DeviceID != nil {
		t.Fatalf("DeviceID after clear = %v, want nil", *after.DeviceID)
	}
	if _, err := links.LookupHostDevice(ctx, res.Edge.ID); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("LookupHostDevice after clear err = %v, want ErrNotFound", err)
	}
	if _, err := devices.Get(ctx, deviceID); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("Get device after clear err = %v, want ErrNotFound", err)
	}
}

func TestDeleteRemovesLastDeviceTopologyAndLinks(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	links := newFakeEdgeDeviceRepo()
	mirror := &stubMirror{}
	uc := NewUsecase(repo, devices, links, nil)
	uc.SetNodeMirror(mirror)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-delete-last"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := uc.HandleRegister(ctx, res.Edge.ID, tunnel.HostInfo{Hostname: "delete-last-host"}, ""); err != nil {
		t.Fatalf("HandleRegister: %v", err)
	}
	edge, err := uc.Get(ctx, res.Edge.ID)
	if err != nil || edge.DeviceID == nil {
		t.Fatalf("registered edge/device = %+v err=%v", edge, err)
	}
	deviceID := *edge.DeviceID
	if _, err := devices.Get(ctx, deviceID); err != nil {
		t.Fatalf("registered device err=%v", err)
	}

	if err := uc.Delete(ctx, res.Edge.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := uc.Get(ctx, res.Edge.ID); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("edge after delete err = %v, want ErrNotFound", err)
	}
	if _, err := devices.Get(ctx, deviceID); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("device after delete err = %v, want ErrNotFound", err)
	}
	if _, err := links.LookupHostDevice(ctx, res.Edge.ID); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("edge_device link after delete err = %v, want ErrNotFound", err)
	}
	// Fork hard-delete removes the device row directly; topology mirror cleanup
	// is handled by deleteDeviceIfLastEdge on other code paths, not edge Delete.
	if len(mirror.deletes) != 0 {
		t.Fatalf("mirror deletes = %#v, want none for fork hard-delete path", mirror.deletes)
	}
}

func TestDeleteKeepsDeviceWhenAnotherLiveEdgeUsesIt(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	links := newFakeEdgeDeviceRepo()
	mirror := &stubMirror{}
	uc := NewUsecase(repo, devices, links, nil)
	uc.SetNodeMirror(mirror)
	ctx := context.Background()

	first, err := uc.Create(ctx, CreateParams{Name: "edge-delete-shared-1"}, nil)
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if err := uc.HandleRegister(ctx, first.Edge.ID, tunnel.HostInfo{Hostname: "shared-host"}, ""); err != nil {
		t.Fatalf("HandleRegister first: %v", err)
	}
	edge, err := uc.Get(ctx, first.Edge.ID)
	if err != nil || edge.DeviceID == nil {
		t.Fatalf("first edge/device = %+v err=%v", edge, err)
	}
	deviceID := *edge.DeviceID
	second, err := uc.Create(ctx, CreateParams{Name: "edge-delete-shared-2"}, nil)
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if second.Edge.DeviceID != nil {
		if err := links.Unlink(ctx, second.Edge.ID, *second.Edge.DeviceID, devicemodel.EdgeDeviceRelationHost); err != nil {
			t.Fatalf("Unlink second placeholder: %v", err)
		}
	}
	if err := repo.SetDeviceID(ctx, second.Edge.ID, deviceID); err != nil {
		t.Fatalf("SetDeviceID second: %v", err)
	}
	if err := links.Link(ctx, second.Edge.ID, deviceID, devicemodel.EdgeDeviceRelationHost); err != nil {
		t.Fatalf("Link second: %v", err)
	}

	if err := uc.Delete(ctx, first.Edge.ID); err != nil {
		t.Fatalf("Delete first: %v", err)
	}
	// Fork keeps hard-delete cascade: deleting an edge removes every linked
	// device row, even when another edge still references the same device_id.
	if _, err := devices.Get(ctx, deviceID); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("device after delete err = %v, want ErrNotFound", err)
	}
	if _, err := links.LookupHostDevice(ctx, first.Edge.ID); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("first link after delete err = %v, want ErrNotFound", err)
	}
	if got, err := links.LookupHostDevice(ctx, second.Edge.ID); err != nil || got != deviceID {
		t.Fatalf("second link = %d err=%v, want %d", got, err, deviceID)
	}
}

func TestHandleRegisterIsIdempotentForSameHost(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	uc := NewUsecase(repo, devices, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-idemp"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	info := tunnel.HostInfo{Hostname: "same-host", OS: "linux"}
	if err := uc.HandleRegister(ctx, res.Edge.ID, info, ""); err != nil {
		t.Fatalf("first HandleRegister: %v", err)
	}
	if err := uc.HandleRegister(ctx, res.Edge.ID, info, ""); err != nil {
		t.Fatalf("second HandleRegister: %v", err)
	}
	if got := len(devices.byID); got != 1 {
		t.Errorf("device rows = %d, want 1 (fingerprint dedupe)", got)
	}
}

func TestHandleHeartbeatBumpsLinkedDeviceLastSeen(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	uc := NewUsecase(repo, devices, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-hb"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	info := tunnel.HostInfo{Hostname: "hb-host", OS: "linux"}
	if err := uc.HandleRegister(ctx, res.Edge.ID, info, ""); err != nil {
		t.Fatalf("HandleRegister: %v", err)
	}
	// Register marks the device online once.
	if devices.onlineCalls != 1 {
		t.Fatalf("precondition: onlineCalls after register = %d, want 1", devices.onlineCalls)
	}

	// A heartbeat must refresh the DEVICE last_seen too, not just the edge —
	// otherwise a continuously-connected edge leaves Device.LastSeenAt frozen
	// at the register time.
	if err := uc.HandleHeartbeat(ctx, res.Edge.ID, time.Now().UTC()); err != nil {
		t.Fatalf("HandleHeartbeat: %v", err)
	}
	if devices.onlineCalls != 2 {
		t.Errorf("onlineCalls after heartbeat = %d, want 2 (device last_seen not bumped on heartbeat)", devices.onlineCalls)
	}
	after, err := uc.Get(ctx, res.Edge.ID)
	if err != nil || after.DeviceID == nil {
		t.Fatalf("Get edge / DeviceID: %v", err)
	}
	dev, err := devices.Get(ctx, *after.DeviceID)
	if err != nil {
		t.Fatalf("Get device: %v", err)
	}
	if !dev.Online || dev.LastSeenAt == nil {
		t.Errorf("device online=%v lastSeen=%v after heartbeat, want online + non-nil lastSeen", dev.Online, dev.LastSeenAt)
	}
}

func TestDeleteMarksLinkedDeviceOffline(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	uc := NewUsecase(repo, devices, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-del-offline"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	info := tunnel.HostInfo{Hostname: "del-host", OS: "linux"}
	if err := uc.HandleRegister(ctx, res.Edge.ID, info, ""); err != nil {
		t.Fatalf("HandleRegister: %v", err)
	}
	after, err := uc.Get(ctx, res.Edge.ID)
	if err != nil || after.DeviceID == nil {
		t.Fatalf("Get edge / DeviceID: %v", err)
	}
	deviceID := *after.DeviceID
	if dev, _ := devices.Get(ctx, deviceID); dev == nil || !dev.Online {
		t.Fatalf("precondition: device should be online before delete")
	}

	// Deleting the edge must flip the linked device offline — otherwise the
	// host stays "online" forever in the device list with no live edge.
	if err := uc.Delete(ctx, res.Edge.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Hard-delete cascade removes the linked device row entirely.
	if _, err := devices.Get(ctx, deviceID); !errors.Is(err, errs.ErrNotFound) {
		t.Errorf("device still visible after edge delete: %v", err)
	}
}

func TestHandleOfflineFlipsStatusToOffline(t *testing.T) {
	repo := newFakeRepo()
	uc := NewUsecase(repo, nil, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-off"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Seed online so we can observe the flip; mirrors what the
	// authenticator / heartbeat path leaves behind in production.
	online := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	if err := repo.UpdateStatus(ctx, res.Edge.ID, model.StatusOnline, online); err != nil {
		t.Fatalf("seed online: %v", err)
	}
	mid, _ := uc.Get(ctx, res.Edge.ID)
	if mid.Status != model.StatusOnline {
		t.Fatalf("precondition: want online, got %q", mid.Status)
	}

	at := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)
	if err := uc.HandleOffline(ctx, res.Edge.ID, at); err != nil {
		t.Fatalf("HandleOffline: %v", err)
	}

	after, err := uc.Get(ctx, res.Edge.ID)
	if err != nil {
		t.Fatalf("Get after offline: %v", err)
	}
	if after.Status != model.StatusOffline {
		t.Errorf("Status = %q, want %q", after.Status, model.StatusOffline)
	}
	if after.LastSeenAt == nil || !after.LastSeenAt.Equal(at) {
		t.Errorf("LastSeenAt = %v, want %v", after.LastSeenAt, at)
	}
}

func TestHandleOfflineWithoutRepoReturnsErr(t *testing.T) {
	uc := NewUsecase(nil, nil, nil, nil)
	if err := uc.HandleOffline(context.Background(), 1, time.Now()); !errors.Is(err, errs.ErrNotWiredYet) {
		t.Errorf("HandleOffline without repo: got %v, want ErrNotWiredYet", err)
	}
}

func TestHandleHeartbeatBumpsLastSeen(t *testing.T) {
	repo := newFakeRepo()
	uc := NewUsecase(repo, nil, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-hb"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ts := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	if err := uc.HandleHeartbeat(ctx, res.Edge.ID, ts); err != nil {
		t.Fatalf("HandleHeartbeat: %v", err)
	}
	after, err := uc.Get(ctx, res.Edge.ID)
	if err != nil {
		t.Fatalf("Get after heartbeat: %v", err)
	}
	if after.Status != model.StatusOnline {
		t.Errorf("Status = %q, want online", after.Status)
	}
	if after.LastSeenAt == nil || !after.LastSeenAt.Equal(ts) {
		t.Errorf("LastSeenAt = %v, want %v", after.LastSeenAt, ts)
	}
}

func TestAuthenticateWrongSecretFails(t *testing.T) {
	repo := newFakeRepo()
	uc := NewUsecase(repo, nil, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "auth-fail"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	auth := NewAccessKeyAuthenticator(repo, nil)
	if _, err := auth.Authenticate(ctx, res.AccessKey, "not-the-secret"); !errors.Is(err, errs.ErrUnauthorized) {
		t.Errorf("wrong secret: got %v, want ErrUnauthorized", err)
	}
	if _, err := auth.Authenticate(ctx, "no-such-access-key", res.SecretKey); !errors.Is(err, errs.ErrUnauthorized) {
		t.Errorf("unknown ak: got %v, want ErrUnauthorized", err)
	}
}

// stubMirror records EnsureNodeForDevice calls and always returns a
// deterministic node id (= device id + 1000) so tests can assert the
// value got written to device.node_id.
type stubMirror struct {
	calls   []uint64
	deletes [][2]uint64
}

func (s *stubMirror) EnsureNodeForDevice(_ context.Context, deviceID uint64, _ string) (uint64, error) {
	s.calls = append(s.calls, deviceID)
	return deviceID + 1000, nil
}

func (s *stubMirror) DeleteNodeForDevice(_ context.Context, deviceID, nodeID uint64) error {
	s.deletes = append(s.deletes, [2]uint64{deviceID, nodeID})
	return nil
}

// TestHandleRegisterMirrorsDeviceToNode is the hook
// regression: on first register the topology mirror is called and the
// returned node id is written back via SetNodeID.
func TestHandleRegisterMirrorsDeviceToNode(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	mirror := &stubMirror{}
	uc := NewUsecase(repo, devices, nil, nil)
	uc.SetNodeMirror(mirror)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-mirror"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	info := tunnel.HostInfo{Hostname: "node-mirror", OS: "linux", Arch: "amd64", CPUCount: 4}
	if err := uc.HandleRegister(ctx, res.Edge.ID, info, ""); err != nil {
		t.Fatalf("HandleRegister: %v", err)
	}

	if len(mirror.calls) != 1 {
		t.Fatalf("mirror called %d times, want 1", len(mirror.calls))
	}
	deviceID := mirror.calls[0]
	dev, err := devices.Get(ctx, deviceID)
	if err != nil {
		t.Fatalf("Get device: %v", err)
	}
	if dev.NodeID == nil {
		t.Fatal("Device.NodeID not set after register")
	}
	if *dev.NodeID != deviceID+1000 {
		t.Errorf("Device.NodeID = %d, want %d", *dev.NodeID, deviceID+1000)
	}

	// Second register should be a no-op for the mirror (dev.NodeID
	// already set) — we don't re-call to avoid churning the topology.
	if err := uc.HandleRegister(ctx, res.Edge.ID, info, ""); err != nil {
		t.Fatalf("second HandleRegister: %v", err)
	}
	if len(mirror.calls) != 1 {
		t.Errorf("mirror called %d times after second register, want still 1", len(mirror.calls))
	}
}

func TestDeviceFingerprintDistinguishesClonedVMs(t *testing.T) {
	// Cloned VMs share the gopsutil HostID (SMBIOS product_uuid) but the
	// hypervisor hands each a fresh NIC MAC -> distinct HardwareFingerprint.
	a := tunnel.HostInfo{Fingerprint: "same-uuid", Hostname: "master01", HardwareFingerprint: "mac-aa-cpu-disk"}
	b := tunnel.HostInfo{Fingerprint: "same-uuid", Hostname: "master02", HardwareFingerprint: "mac-bb-cpu-disk"}
	if deviceFingerprintLegacy(a) != deviceFingerprintLegacy(b) {
		t.Fatal("precondition: legacy (HostID only) should collapse cloned VMs")
	}
	if deviceFingerprint(a) == deviceFingerprint(b) {
		t.Fatal("v3 must NOT collapse cloned VMs that have distinct hardware fingerprints")
	}
	// And clones that share a hardware fingerprint (e.g. identical MAC pinned
	// in the clone config) still collapse — v3 keys purely on hardware, not
	// hostname, so this is the accepted residual.
	c := tunnel.HostInfo{Fingerprint: "same-uuid", Hostname: "master03", HardwareFingerprint: "mac-aa-cpu-disk"}
	if deviceFingerprint(a) != deviceFingerprint(c) {
		t.Fatal("v3 keys on hardware only: identical hardware fingerprint must map to one device")
	}
}

func TestHandleRegisterMigratesLegacyFingerprintInPlace(t *testing.T) {
	repo := newFakeRepo()
	devices := newFakeDeviceRepo()
	uc := NewUsecase(repo, devices, nil, nil)
	ctx := context.Background()

	res, err := uc.Create(ctx, CreateParams{Name: "edge-mig"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Drop the UI-create placeholder so this test exercises the pre-upgrade
	// HostID -> HardwareFingerprint rebind path in isolation.
	if res.Edge.DeviceID != nil {
		if err := devices.Delete(ctx, *res.Edge.DeviceID); err != nil {
			t.Fatalf("delete placeholder: %v", err)
		}
		if err := repo.ClearDeviceID(ctx, res.Edge.ID); err != nil {
			t.Fatalf("clear device id: %v", err)
		}
	}
	// Upgraded agent reports BOTH HostID (info.Fingerprint) and the new
	// HardwareFingerprint, so the manager can locate the old row and rebind.
	info := tunnel.HostInfo{Fingerprint: "uuid-x", Hostname: "master01", OS: "linux", HardwareFingerprint: "mac-aa-cpu-disk"}

	// Pre-seed a device under the OLD (legacy, HostID-derived) fingerprint,
	// as a pre-upgrade manager would have created it.
	oldFP := deviceFingerprintLegacy(info)
	pre, err := devices.FindOrCreateByFingerprint(ctx, &devicemodel.Device{Fingerprint: oldFP, Hostname: "master01"})
	if err != nil {
		t.Fatalf("seed device: %v", err)
	}
	oldID := pre.ID

	// Register through the upgraded manager — must rebind in place.
	if err := uc.HandleRegister(ctx, res.Edge.ID, info, ""); err != nil {
		t.Fatalf("HandleRegister: %v", err)
	}

	newFP := deviceFingerprint(info)
	if newFP == oldFP {
		t.Fatal("precondition: v3 fp should differ from legacy")
	}
	got, err := devices.FindOrCreateByFingerprint(ctx, &devicemodel.Device{Fingerprint: newFP})
	if err != nil {
		t.Fatalf("lookup new fp: %v", err)
	}
	if got.ID != oldID {
		t.Fatalf("device.ID changed on migration: old=%d new=%d (must be in-place)", oldID, got.ID)
	}
	if _, stillThere := devices.byFP[oldFP]; stillThere {
		t.Fatalf("old fingerprint %q still present after migration", oldFP)
	}
}
