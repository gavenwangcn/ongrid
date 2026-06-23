package alert

import (
	"context"
	"fmt"
	"strings"

	bizsetting "github.com/ongridio/ongrid/internal/manager/biz/setting"
	settingmodel "github.com/ongridio/ongrid/internal/manager/model/setting"
	"github.com/ongridio/ongrid/internal/pkg/errs"
	"github.com/ongridio/ongrid/internal/pkg/notify"
)

// NotifyWebhookSettingsService reads/writes the global webhook send mode
// (curl vs Go net/http) under system_settings.alert.webhook_send_mode.
type NotifyWebhookSettingsService struct {
	settings *bizsetting.Service
}

// NotifyWebhookSettingsView is the wire shape for GET/PUT
// /v1/alert-settings/webhook-send-mode.
type NotifyWebhookSettingsView struct {
	SendMode string `json:"send_mode"`
}

func NewNotifyWebhookSettingsService(settings *bizsetting.Service) *NotifyWebhookSettingsService {
	return &NotifyWebhookSettingsService{settings: settings}
}

func (s *NotifyWebhookSettingsService) Get(ctx context.Context) (NotifyWebhookSettingsView, error) {
	if s == nil || s.settings == nil {
		return NotifyWebhookSettingsView{}, errs.ErrNotWiredYet
	}
	v, ok, err := s.settings.Get(ctx, settingmodel.CategoryAlert, settingmodel.KeyNotifyWebhookSendMode)
	if err != nil {
		return NotifyWebhookSettingsView{}, err
	}
	if !ok || v == "" {
		return NotifyWebhookSettingsView{SendMode: string(notify.WebhookSendModeCurl)}, nil
	}
	return NotifyWebhookSettingsView{SendMode: string(notify.NormalizeWebhookSendMode(v))}, nil
}

func (s *NotifyWebhookSettingsService) Set(ctx context.Context, sendMode string) (NotifyWebhookSettingsView, error) {
	if s == nil || s.settings == nil {
		return NotifyWebhookSettingsView{}, errs.ErrNotWiredYet
	}
	raw := strings.TrimSpace(sendMode)
	switch raw {
	case string(notify.WebhookSendModeCurl), string(notify.WebhookSendModeHTTP):
	default:
		return NotifyWebhookSettingsView{}, fmt.Errorf("%w: send_mode must be curl or http", errs.ErrInvalid)
	}
	if err := s.settings.Set(ctx, settingmodel.CategoryAlert, settingmodel.KeyNotifyWebhookSendMode, raw, false); err != nil {
		return NotifyWebhookSettingsView{}, err
	}
	return NotifyWebhookSettingsView{SendMode: raw}, nil
}

// ResolveSendMode returns the effective mode for one delivery.
func (s *NotifyWebhookSettingsService) ResolveSendMode(ctx context.Context) notify.WebhookSendMode {
	if s == nil {
		return notify.WebhookSendModeCurl
	}
	out, err := s.Get(ctx)
	if err != nil {
		return notify.WebhookSendModeCurl
	}
	return notify.NormalizeWebhookSendMode(out.SendMode)
}
