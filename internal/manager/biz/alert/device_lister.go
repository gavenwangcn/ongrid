package alert

import (
	"context"

	devicebiz "github.com/ongridio/ongrid/internal/manager/biz/device"
)

// DeviceRepoSystemLister adapts device.Repo for SystemNotifyService.
type DeviceRepoSystemLister struct {
	Repo devicebiz.Repo
}

func (d DeviceRepoSystemLister) ListDistinctSystemNames(ctx context.Context) ([]string, error) {
	if d.Repo == nil {
		return nil, nil
	}
	return d.Repo.ListDistinctSystemNames(ctx)
}

func (d DeviceRepoSystemLister) ListSystemEnvironmentPairs(ctx context.Context) ([]SystemEnvironmentPair, error) {
	if d.Repo == nil {
		return nil, nil
	}
	rows, err := d.Repo.ListSystemEnvironmentPairs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SystemEnvironmentPair, 0, len(rows))
	for _, row := range rows {
		out = append(out, SystemEnvironmentPair{
			SystemName:     row.SystemName,
			EnvironmentTag: row.EnvironmentTag,
		})
	}
	return out, nil
}
