package logs

import (
	"strings"
	"testing"

	"github.com/ongridio/ongrid/internal/edgeagent/plugins"
)

func TestNeedsPromtail(t *testing.T) {
	if !needsPromtail(nil) {
		t.Fatal("default spec should need promtail (journald on)")
	}
	spec := map[string]interface{}{
		specEnableJournald:  false,
		specEnableDockerAPI: true,
	}
	if needsPromtail(spec) {
		t.Fatal("docker api only should not need promtail")
	}
	spec[specFilePaths] = []interface{}{"/var/log/app.log"}
	if !needsPromtail(spec) {
		t.Fatal("file_paths should need promtail")
	}
}

func TestFilterDockerFilePaths(t *testing.T) {
	in := []string{
		"/kingdee/docker/containers/*/*-json.log",
		"/var/log/nginx/access.log",
	}
	out := filterDockerFilePaths(in)
	if len(out) != 1 || out[0] != "/var/log/nginx/access.log" {
		t.Fatalf("filter = %v", out)
	}
}

func TestParseDockerTimestampLine(t *testing.T) {
	ts, msg := parseDockerTimestampLine("2024-06-15T12:00:01.123456789Z hello")
	if msg != "hello" {
		t.Fatalf("msg = %q", msg)
	}
	if ts.Year() != 2024 {
		t.Fatalf("ts = %v", ts)
	}
	_, msg2 := parseDockerTimestampLine("plain line")
	if msg2 != "plain line" {
		t.Fatalf("plain = %q", msg2)
	}
}

func TestRenderSkipsDockerPathsWhenDockerAPI(t *testing.T) {
	cfg := plugins.PluginConfig{
		Enabled:  true,
		EdgeID:   1,
		Endpoint: "http://manager/loki/api/v1/push",
		Spec: map[string]interface{}{
			specEnableJournald:  false,
			specEnableDockerAPI: true,
			specFilePaths: []interface{}{
				"/kingdee/docker/containers/*/*-json.log",
				"/var/log/app.log",
			},
		},
	}
	out, err := render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	body := string(out)
	if strings.Contains(body, "kingdee/docker") {
		t.Fatal("docker path should be stripped from promtail when docker api enabled")
	}
	if !strings.Contains(body, "/var/log/app.log") {
		t.Fatal("non-docker file path should remain")
	}
}

func TestRenderDockerAPIOnlyNoSyslogFallback(t *testing.T) {
	cfg := plugins.PluginConfig{
		Enabled:  true,
		EdgeID:   1,
		Endpoint: "http://manager/loki/api/v1/push",
		Spec: map[string]interface{}{
			specEnableJournald:  false,
			specEnableDockerAPI: true,
		},
	}
	out, err := render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	body := string(out)
	if strings.Contains(body, "/var/log/syslog") {
		t.Fatal("docker api only should not fall back to syslog scrape")
	}
	if strings.Contains(body, "job_name: journald") {
		t.Fatal("journald should be off")
	}
}
