package report

import (
	"fmt"
	"strings"
)

func renderDeviceResourcesMarkdown(b *strings.Builder, dr DeviceResourceFacts, mtr func(zh, eng string) string) {
	if len(dr.Devices) == 0 {
		return
	}
	b.WriteString("### " + mtr("设备资源明细", "Per-device resources") + "\n\n")
	avg, peak := mtr("均", "avg"), mtr("峰", "peak")
	for _, d := range dr.Devices {
		status := mtr("离线", "offline")
		if d.Online {
			status = mtr("在线", "online")
		}
		line := fmt.Sprintf("- **%s** (#%d, %s)", d.Name, d.DeviceID, status)
		if d.CPUCount > 0 {
			line += fmt.Sprintf(" · CPU %d%s", d.CPUCount, mtr("核", " cores"))
		}
		if d.MemTotalBytes > 0 {
			line += fmt.Sprintf(" · %s %s", mtr("内存", "mem"), formatBytesSize(float64(d.MemTotalBytes)))
		}
		if d.DiskTotalBytes > 0 {
			line += fmt.Sprintf(" · %s %s", mtr("磁盘", "disk"), formatBytesSize(float64(d.DiskTotalBytes)))
		}
		if d.CPUAvg > 0 || d.CPUPeak > 0 {
			line += fmt.Sprintf(" · CPU %s %.1f%% / %s %.1f%%", avg, d.CPUAvg, peak, d.CPUPeak)
		}
		if d.MemAvg > 0 || d.MemPeak > 0 {
			line += fmt.Sprintf(" · %s %s %.1f%% / %s %.1f%%", mtr("内存", "mem"), avg, d.MemAvg, peak, d.MemPeak)
		}
		if d.DiskAvg > 0 || d.DiskPeak > 0 {
			line += fmt.Sprintf(" · %s %s %.1f%% / %s %.1f%%", mtr("磁盘", "disk"), avg, d.DiskAvg, peak, d.DiskPeak)
		}
		if d.NetRxAvgBps > 0 || d.NetRxPeakBps > 0 || d.NetTxAvgBps > 0 || d.NetTxPeakBps > 0 {
			line += fmt.Sprintf(" · %s rx %s/%s tx %s/%s", mtr("网络", "net"),
				formatBytesPerSec(d.NetRxAvgBps), formatBytesPerSec(d.NetRxPeakBps),
				formatBytesPerSec(d.NetTxAvgBps), formatBytesPerSec(d.NetTxPeakBps))
		}
		if d.DownsizeSuggest && d.DownsizeHint != "" {
			line += " · **" + mtr("降配建议", "Downsize") + "**: " + d.DownsizeHint
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
}
