package version

import "testing"

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v1.0.1", -1},
		{"v1.2.0", "v1.1.9", 1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.0.0-dev", "v1.0.0", -1},
		{"1.0.0", "v1.0.0", 0},
	}

	for _, tt := range tests {
		if got := CompareSemver(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("v1.0.0", "v1.0.1") {
		t.Error("expected v1.0.1 to be newer than v1.0.0")
	}
	if !IsNewer("v1.0.0-dev", "v1.0.0") {
		t.Error("expected v1.0.0 to be newer than v1.0.0-dev")
	}
	if IsNewer("v1.1.0", "v1.0.9") {
		t.Error("expected v1.0.9 not to be newer than v1.1.0")
	}
}
