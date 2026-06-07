package backend

import (
	"runtime"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.4", "v1.2.3", true},
		{"1.2.4", "1.2.3", true},
		{"v1.3.0", "v1.2.9", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.2", "v1.2.3", false},
		// A final release is newer than its pre-release.
		{"v1.2.3", "v1.2.3-alpha", true},
		{"v1.2.3-alpha", "v1.2.3", false},
		// Pre-release ordering.
		{"v1.2.3-rc.2", "v1.2.3-rc.1", true},
		{"v1.2.3-rc.1", "v1.2.3-rc.2", false},
		// Dev / unknown current builds must never be considered outdated.
		{"v1.2.3", "dev", false},
		{"v1.2.3", "", false},
		// Invalid latest tag is ignored.
		{"not-a-version", "v1.2.3", false},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestGetAssetCandidates(t *testing.T) {
	got := getAssetCandidates()
	if runtime.GOOS != "linux" {
		if got != nil {
			t.Errorf("expected nil candidates on %s, got %v", runtime.GOOS, got)
		}
		return
	}
	switch runtime.GOARCH {
	case "amd64", "arm64", "386", "arm":
		if len(got) == 0 {
			t.Errorf("expected asset candidates for linux/%s, got none", runtime.GOARCH)
		}
	}
}
