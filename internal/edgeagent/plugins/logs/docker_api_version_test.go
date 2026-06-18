package logs

import "testing"

func TestChooseDockerAPIVersion(t *testing.T) {
	cases := []struct {
		min, max string
		want     string
	}{
		{"1.44", "1.52", "v1.44"},
		{"1.24", "1.41", "v1.41"},
		{"1.24", "1.43", "v1.41"},
		{"1.44", "1.44", "v1.44"},
		{"", "1.41", "v1.41"},
		{"1.12", "1.40", "v1.40"},
	}
	for _, tc := range cases {
		got := chooseDockerAPIVersion(tc.min, tc.max)
		if got != tc.want {
			t.Errorf("chooseDockerAPIVersion(%q, %q) = %q, want %q", tc.min, tc.max, got, tc.want)
		}
	}
}

func TestCompareAPIVersion(t *testing.T) {
	if compareAPIVersion("1.44", "1.41") <= 0 {
		t.Error("1.44 should be > 1.41")
	}
	if compareAPIVersion("1.41", "1.44") >= 0 {
		t.Error("1.41 should be < 1.44")
	}
	if compareAPIVersion("1.41", "1.41") != 0 {
		t.Error("1.41 should equal 1.41")
	}
}
