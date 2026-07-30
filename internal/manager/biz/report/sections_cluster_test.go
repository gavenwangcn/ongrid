package report

import "testing"

func TestClusterResourceDirective(t *testing.T) {
	got := clusterResourceDirective([]string{SectionCluster}, false)
	if got == "" {
		t.Fatal("expected directive for cluster section")
	}
	if !containsSubstr(got, "网络") || !containsSubstr(got, "CPU") {
		t.Errorf("directive missing resource dimensions: %q", got)
	}
	if clusterResourceDirective([]string{SectionLogs}, false) != "" {
		t.Error("directive should be empty when cluster disabled")
	}
}

func containsSubstr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})())
}
