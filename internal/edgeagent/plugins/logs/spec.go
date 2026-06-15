package logs

import (
	"strings"
)

const (
	specEnableJournald   = "enable_journald"
	specEnableDockerAPI  = "enable_docker_api"
	specDockerSocket     = "docker_socket"
	specFilePaths        = "file_paths"
	specJournaldUnits    = "journald_units"
	specExtraLabels      = "extra_labels"
	defaultDockerSocket  = "/var/run/docker.sock"
)

func specBool(spec map[string]interface{}, key string, defaultVal bool) bool {
	if spec == nil {
		return defaultVal
	}
	v, ok := spec[key]
	if !ok {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

func specString(spec map[string]interface{}, key, defaultVal string) string {
	if spec == nil {
		return defaultVal
	}
	v, ok := spec[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return defaultVal
	}
	return strings.TrimSpace(s)
}

func enableDockerAPI(spec map[string]interface{}) bool {
	return specBool(spec, specEnableDockerAPI, false)
}

func dockerSocket(spec map[string]interface{}) string {
	return specString(spec, specDockerSocket, defaultDockerSocket)
}

// needsPromtail reports whether promtail should run for this config snapshot.
// When only enable_docker_api is on (no journald, no file_paths), promtail has
// nothing to scrape.
func needsPromtail(spec map[string]interface{}) bool {
	if spec == nil {
		return true // default journald on
	}
	enableJournald := specBool(spec, specEnableJournald, true)
	paths := stringSlice(spec, specFilePaths)
	if enableDockerAPI(spec) {
		paths = filterDockerFilePaths(paths)
	}
	if !enableJournald && len(paths) == 0 {
		return false
	}
	return true
}

// filterDockerFilePaths drops file_paths that target docker json-file logs on
// disk — those are collected via Docker API when enable_docker_api is set.
func filterDockerFilePaths(paths []string) []string {
	if len(paths) == 0 {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if looksLikeDockerContainerLogPath(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func looksLikeDockerContainerLogPath(p string) bool {
	p = strings.ToLower(p)
	return strings.Contains(p, "/containers/") &&
		(strings.Contains(p, "json.log") || strings.Contains(p, "*.log"))
}
