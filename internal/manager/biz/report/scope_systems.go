package report

import (
	"sort"
	"strings"
)

const UnassignedSystemLabel = "未分类"

// NormalizeScope merges legacy system_name into system_names and keeps
// fields deduplicated and sorted.
func NormalizeScope(s Scope) Scope {
	if len(s.SystemNames) == 0 {
		if name := strings.TrimSpace(s.SystemName); name != "" {
			s.SystemNames = []string{name}
		}
	} else {
		seen := make(map[string]struct{}, len(s.SystemNames))
		out := make([]string, 0, len(s.SystemNames))
		for _, raw := range s.SystemNames {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
		sort.Strings(out)
		s.SystemNames = out
	}
	if len(s.SystemNames) == 1 {
		s.SystemName = s.SystemNames[0]
	} else {
		s.SystemName = ""
	}
	return s
}

// ExplicitSystemNames returns operator-selected systems after normalization.
// Empty means "all systems" (not yet expanded to distinct names).
func ExplicitSystemNames(s Scope) []string {
	return append([]string(nil), NormalizeScope(s).SystemNames...)
}

// ScopeForSystem clones scope narrowed to one business system.
func ScopeForSystem(base Scope, systemName string) Scope {
	s := NormalizeScope(base)
	s.SystemName = strings.TrimSpace(systemName)
	s.SystemNames = nil
	if s.SystemName != "" {
		s.SystemNames = []string{s.SystemName}
	}
	s.EdgeIDs = nil
	return s
}
