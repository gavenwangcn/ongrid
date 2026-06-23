package alert

import (
	"context"
	"testing"

	bizsetting "github.com/ongridio/ongrid/internal/manager/biz/setting"
	settingmodel "github.com/ongridio/ongrid/internal/manager/model/setting"
	"github.com/ongridio/ongrid/internal/pkg/notify"
)

func TestNotifyWebhookSettingsServiceDefaultCurl(t *testing.T) {
	t.Parallel()
	repo := newMemSettingRepo()
	svc := NewNotifyWebhookSettingsService(bizsetting.New(repo, nil))

	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.SendMode != string(notify.WebhookSendModeCurl) {
		t.Fatalf("SendMode = %q, want curl", got.SendMode)
	}
	if mode := svc.ResolveSendMode(context.Background()); mode != notify.WebhookSendModeCurl {
		t.Fatalf("ResolveSendMode = %q, want curl", mode)
	}
}

func TestNotifyWebhookSettingsServiceSetHTTP(t *testing.T) {
	t.Parallel()
	repo := newMemSettingRepo()
	svc := NewNotifyWebhookSettingsService(bizsetting.New(repo, nil))

	got, err := svc.Set(context.Background(), string(notify.WebhookSendModeHTTP))
	if err != nil {
		t.Fatal(err)
	}
	if got.SendMode != string(notify.WebhookSendModeHTTP) {
		t.Fatalf("Set returned %q", got.SendMode)
	}
	row, err := repo.Get(context.Background(), settingmodel.CategoryAlert, settingmodel.KeyNotifyWebhookSendMode)
	if err != nil {
		t.Fatal(err)
	}
	if row.Value != string(notify.WebhookSendModeHTTP) {
		t.Fatalf("stored value = %q", row.Value)
	}
}

func TestNotifyWebhookSettingsServiceRejectInvalid(t *testing.T) {
	t.Parallel()
	svc := NewNotifyWebhookSettingsService(bizsetting.New(newMemSettingRepo(), nil))
	if _, err := svc.Set(context.Background(), "wget"); err == nil {
		t.Fatal("expected error for invalid send_mode")
	}
}
