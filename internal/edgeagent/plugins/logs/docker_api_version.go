package logs

import (
	"fmt"
	"strconv"
	"strings"
)

// dockerAPIPreferences lists Engine API versions edge knows how to speak,
// newest first. We only use list-containers + container-logs — both exist
// since very old API versions; the split is driven by daemon min/max policy
// (Docker 29+ requires clients ≥ 1.44).
var dockerAPIPreferences = []string{"1.44", "1.41"}

const dockerAPIFallback = "v1.41"

// chooseDockerAPIVersion picks a path version within [minVer, maxVer].
// Empty min/max are treated as legacy daemons that only expose ApiVersion.
func chooseDockerAPIVersion(minVer, maxVer string) string {
	minVer = normalizeAPIVersion(minVer)
	maxVer = normalizeAPIVersion(maxVer)
	if maxVer == "" {
		maxVer = "1.41"
	}
	if minVer == "" {
		minVer = "1.12"
	}
	for _, pref := range dockerAPIPreferences {
		if apiVersionWithin(minVer, maxVer, pref) {
			return "v" + pref
		}
	}
	// No preferred version fits — use the newest the daemon still accepts.
	return "v" + maxVer
}

func normalizeAPIVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	return v
}

func apiVersionWithin(minVer, maxVer, target string) bool {
	target = normalizeAPIVersion(target)
	return compareAPIVersion(minVer, target) <= 0 && compareAPIVersion(target, maxVer) <= 0
}

// compareAPIVersion compares major.minor API strings. Returns -1/0/1.
func compareAPIVersion(a, b string) int {
	am, an, aok := parseAPIVersion(normalizeAPIVersion(a))
	bm, bn, bok := parseAPIVersion(normalizeAPIVersion(b))
	if !aok || !bok {
		return strings.Compare(normalizeAPIVersion(a), normalizeAPIVersion(b))
	}
	if am != bm {
		return am - bm
	}
	return an - bn
}

func parseAPIVersion(v string) (major, minor int, ok bool) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func formatAPIVersionPath(v string) string {
	v = normalizeAPIVersion(v)
	if v == "" {
		return dockerAPIFallback
	}
	return "v" + v
}

func dockerVersionURL() string {
	return "http://docker/version"
}

func dockerVersionProbeError(status int, body string) error {
	if len(body) > 256 {
		body = body[:256] + "…"
	}
	return fmt.Errorf("docker version probe %d: %s", status, body)
}
