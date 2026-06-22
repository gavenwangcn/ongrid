package alert

import (
	"context"
	"testing"
	"time"

	bizsetting "github.com/ongridio/ongrid/internal/manager/biz/setting"
	model "github.com/ongridio/ongrid/internal/manager/model/alert"
	settingmodel "github.com/ongridio/ongrid/internal/manager/model/setting"
	devicemodel "github.com/ongridio/ongrid/internal/manager/model/device"
	"github.com/ongridio/ongrid/internal/pkg/errs"
)

type memSettingRepo struct {
	rows map[string]*settingmodel.Setting
}

func newMemSettingRepo() *memSettingRepo {
	return &memSettingRepo{rows: make(map[string]*settingmodel.Setting)}
}

func (r *memSettingRepo) key(cat, key string) string { return cat + "|" + key }

func (r *memSettingRepo) Get(_ context.Context, category, key string) (*settingmodel.Setting, error) {
	row, ok := r.rows[r.key(category, key)]
	if !ok {
		return nil, errs.ErrNotFound
	}
	cp := *row
	return &cp, nil
}

func (r *memSettingRepo) Set(_ context.Context, category, key, value string, sensitive bool) (*settingmodel.Setting, error) {
	row := &settingmodel.Setting{Category: category, Key: key, Value: value, Sensitive: sensitive, UpdatedAt: time.Now()}
	r.rows[r.key(category, key)] = row
	return row, nil
}

func (r *memSettingRepo) List(_ context.Context, category string) ([]*settingmodel.Setting, error) {
	return nil, nil
}

func (r *memSettingRepo) Delete(_ context.Context, category, key string) error {
	delete(r.rows, r.key(category, key))
	return nil
}

func TestDBChannelResolverSystemBindings(t *testing.T) {
	src := &fakeChannelLister{rows: []*model.Channel{
		{ID: 1, Name: "global-feishu", Enabled: true, MatchSeverityMin: "warning"},
		{ID: 2, Name: "ehr-feishu", Enabled: true},
		{ID: 3, Name: "ebd-feishu", Enabled: true},
	}}
	repo := newMemSettingRepo()
	if _, err := repo.Set(context.Background(), settingmodel.CategoryAlert, settingmodel.KeySystemNotifyBindings, `{"人力资源-EHR系统":[2]}`, false); err != nil {
		t.Fatal(err)
	}
	svc := NewSystemNotifyService(bizsetting.New(repo, nil), nil, nil)

	r := NewDBChannelResolver(src, []string{"fallback"})
	devID := uint64(7)
	r.SetDeviceLookup(DeviceTargetFromGetter(func(_ context.Context, id uint64) (*devicemodel.Device, error) {
		if id != devID {
			t.Fatalf("device id = %d", id)
		}
		return &devicemodel.Device{SystemName: "人力资源-EHR系统", EnvironmentTag: "prod"}, nil
	}))
	r.SetSystemNotify(svc)

	got := channelNames(r.ChannelsFor(context.Background(), &model.Incident{
		Severity: "warning",
		DeviceID: &devID,
	}))
	if len(got) != 1 || got[0] != "ehr-feishu" {
		t.Fatalf("EHR incident channels = %v, want [ehr-feishu]", got)
	}
	if contains(got, "global-feishu") {
		t.Fatal("system binding should not fall through to global channels")
	}
}

func TestDBChannelResolverSystemEnvironmentBindings(t *testing.T) {
	src := &fakeChannelLister{rows: []*model.Channel{
		{ID: 2, Name: "ehr-prod-feishu", Enabled: true},
		{ID: 4, Name: "ehr-test-feishu", Enabled: true},
	}}
	repo := newMemSettingRepo()
	raw := `{"人力资源-EHR系统":{"prod":[2],"test":[4]}}`
	if _, err := repo.Set(context.Background(), settingmodel.CategoryAlert, settingmodel.KeySystemNotifyBindings, raw, false); err != nil {
		t.Fatal(err)
	}
	svc := NewSystemNotifyService(bizsetting.New(repo, nil), nil, nil)
	r := NewDBChannelResolver(src, []string{"fallback"})
	devID := uint64(9)
	r.SetDeviceLookup(DeviceTargetFromGetter(func(_ context.Context, id uint64) (*devicemodel.Device, error) {
		return &devicemodel.Device{SystemName: "人力资源-EHR系统", EnvironmentTag: "prod"}, nil
	}))
	r.SetSystemNotify(svc)

	got := channelNames(r.ChannelsFor(context.Background(), &model.Incident{
		Severity: "warning",
		DeviceID: &devID,
	}))
	if len(got) != 1 || got[0] != "ehr-prod-feishu" {
		t.Fatalf("prod incident channels = %v", got)
	}
}

func TestSystemNotifyServiceSetAndResolve(t *testing.T) {
	repo := newMemSettingRepo()
	ch := &fakeNotifyCatalog{rows: map[uint64]*model.Channel{
		5: {ID: 5, Name: "ehr", Enabled: true},
		6: {ID: 6, Name: "ehr-prod", Enabled: true},
		7: {ID: 7, Name: "ehr-test", Enabled: true},
	}}
	svc := NewSystemNotifyService(bizsetting.New(repo, nil), nil, ch)
	ctx := context.Background()

	_, err := svc.Set(ctx, []SystemNotifyBinding{{SystemName: "人力资源-EHR系统", ChannelIDs: []uint64{5}}})
	if err != nil {
		t.Fatal(err)
	}
	ids := svc.ChannelIDsForTarget(ctx, "人力资源-EHR系统", "prod")
	if len(ids) != 1 || ids[0] != 5 {
		t.Fatalf("wildcard ids = %v", ids)
	}

	_, err = svc.Set(ctx, []SystemNotifyBinding{
		{SystemName: "人力资源-EHR系统", EnvironmentTag: "prod", ChannelIDs: []uint64{6}},
		{SystemName: "人力资源-EHR系统", EnvironmentTag: "test", ChannelIDs: []uint64{7}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := svc.ChannelIDsForTarget(ctx, "人力资源-EHR系统", "prod"); len(got) != 1 || got[0] != 6 {
		t.Fatalf("prod ids = %v", got)
	}
	if got := svc.ChannelIDsForTarget(ctx, "人力资源-EHR系统", "dev"); len(got) != 0 {
		t.Fatalf("dev should not match, got %v", got)
	}
}

type fakeNotifyCatalog struct {
	rows map[uint64]*model.Channel
}

func (f *fakeNotifyCatalog) ListNotificationChannels(context.Context) ([]*model.Channel, error) {
	out := make([]*model.Channel, 0, len(f.rows))
	for _, ch := range f.rows {
		out = append(out, ch)
	}
	return out, nil
}

func (f *fakeNotifyCatalog) GetChannelByID(_ context.Context, id uint64) (*model.Channel, error) {
	ch, ok := f.rows[id]
	if !ok {
		return nil, errs.ErrNotFound
	}
	return ch, nil
}
