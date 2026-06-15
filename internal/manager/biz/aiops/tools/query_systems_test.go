package tools

import (
	"testing"
	"time"

	devicemodel "github.com/ongridio/ongrid/internal/manager/model/device"
)

func TestAggregateSystemsCountsAndOptionalDevices(t *testing.T) {
	now := time.Now()
	devices := []*devicemodel.Device{
		{ID: 1, Name: "a", SystemName: "订单", Online: true, LastSeenAt: &now},
		{ID: 2, Name: "b", SystemName: "订单", Online: false},
		{ID: 3, Name: "c", SystemName: "支付", Online: true},
		{ID: 4, Name: "d", SystemName: "", Online: true},
	}

	summaries := aggregateSystems(devices, false, 50)
	if len(summaries) != 3 {
		t.Fatalf("systems = %d, want 3", len(summaries))
	}
	// Empty system_name sorts last.
	if summaries[len(summaries)-1].SystemName != "" {
		t.Fatalf("last system = %q, want empty unassigned bucket", summaries[len(summaries)-1].SystemName)
	}

	var order, pay *SystemSummary
	for i := range summaries {
		switch summaries[i].SystemName {
		case "订单":
			order = &summaries[i]
		case "支付":
			pay = &summaries[i]
		}
	}
	if order == nil || order.DeviceCount != 2 || order.OnlineCount != 1 || order.OfflineCount != 1 {
		t.Fatalf("订单 bucket = %+v", order)
	}
	if pay == nil || pay.DeviceCount != 1 {
		t.Fatalf("支付 bucket = %+v", pay)
	}

	withDevices := aggregateSystems(devices, true, 1)
	for _, s := range withDevices {
		if s.SystemName == "订单" && len(s.Devices) != 1 {
			t.Fatalf("devices per system limit: got %d", len(s.Devices))
		}
	}
}
