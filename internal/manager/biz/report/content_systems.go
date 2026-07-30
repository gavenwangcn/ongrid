package report

import (
	"fmt"
	"strings"
)

func renderSystemMarkdownBlock(b *strings.Builder, sys SystemContentBlock, mtr func(zh, eng string) string) {
	b.WriteString("## " + sys.SystemName + "\n\n")
	if sys.Narrative.Headline != "" {
		b.WriteString("### " + sys.Narrative.Headline + "\n\n")
	}
	for _, p := range sys.Narrative.Paragraphs {
		b.WriteString(flattenEntities(p.Text) + "\n\n")
	}
	if sys.Resource.Available {
		b.WriteString("### " + mtr("资源使用", "Resource usage") + "\n\n")
		avg, peak := mtr("均", "avg"), mtr("峰", "peak")
		b.WriteString(fmt.Sprintf("- CPU: %s %.1f%% · %s %.1f%%\n", avg, sys.Resource.CPUAvg, peak, sys.Resource.CPUPeak))
		b.WriteString(fmt.Sprintf("- %s: %s %.1f%% · %s %.1f%%\n", mtr("内存", "Memory"), avg, sys.Resource.MemAvg, peak, sys.Resource.MemPeak))
		b.WriteString(fmt.Sprintf("- %s: %s %.1f%% · %s %.1f%%\n\n", mtr("磁盘", "Disk"), avg, sys.Resource.DiskAvg, peak, sys.Resource.DiskPeak))
	}
	b.WriteString(fmt.Sprintf("- %s\n\n", mtr(
		fmt.Sprintf("监控设备 %d 台 · 在线 %d 台", sys.Fleet.Total, sys.Fleet.Online),
		fmt.Sprintf("%d devices · %d online", sys.Fleet.Total, sys.Fleet.Online),
	)))
	if sys.Logs.Available {
		b.WriteString(fmt.Sprintf("- %s\n\n", mtr(
			fmt.Sprintf("潜在错误 %d 条", sys.Logs.TotalErrors),
			fmt.Sprintf("%d potential errors", sys.Logs.TotalErrors),
		)))
	}
	if len(sys.KeyIncidents) > 0 {
		b.WriteString("### " + mtr("告警", "Alerts") + "\n\n")
		for _, ki := range sys.KeyIncidents {
			b.WriteString(fmt.Sprintf("- I-%d %s (%s, %dm, %s)\n", ki.ID, ki.Title, ki.Severity, ki.DurationMin, ki.Status))
		}
		b.WriteString("\n")
	}
	if len(sys.Advice) > 0 {
		b.WriteString("### " + mtr("建议", "Recommendations") + "\n\n")
		for _, a := range sys.Advice {
			b.WriteString("- " + flattenEntities(a.Text) + "\n")
		}
		b.WriteString("\n")
	}
}
