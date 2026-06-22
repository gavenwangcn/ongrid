package alert

import (
	"context"

	alertmodel "github.com/ongridio/ongrid/internal/manager/model/alert"
)

// RepoNotifyCatalog adapts bizalert.Repo for SystemNotifyService.
type RepoNotifyCatalog struct {
	Repo Repo
}

func (c RepoNotifyCatalog) ListNotificationChannels(ctx context.Context) ([]*alertmodel.Channel, error) {
	if c.Repo == nil {
		return nil, nil
	}
	return c.Repo.ListChannels(ctx, ChannelFilter{Limit: 500})
}

func (c RepoNotifyCatalog) GetChannelByID(ctx context.Context, id uint64) (*alertmodel.Channel, error) {
	if c.Repo == nil {
		return nil, nil
	}
	return c.Repo.GetChannelByID(ctx, id)
}
