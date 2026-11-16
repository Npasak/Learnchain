package version

import (
	"runtime"
	"strconv"
	"strings"
)

// Build-time variables injected via -ldflags.
var (
	Version   = "v1.2.0"
	Commit    = "local"
	BuildDate = "unknown"
)

// Info describes the running application binary.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// GetInfo returns metadata about the current build.
func GetInfo() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// CompareSemver compares two semantic version strings (e.g. "v1.2.3" vs "1.3.0").
// Returns -1 if a < b, 0 if a == b, and 1 if a > b.
func CompareSemver(a, b string) int {
	pa, preA := parseSemver(a)
	pb, preB := parseSemver(b)

	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}

	// Same MAJOR.MINOR.PATCH: release > pre-release.
	if preA == "" && preB != "" {
		return 1
	}
	if preA != "" && preB == "" {
		return -1
	}
	return strings.Compare(preA, preB)
}

// IsNewer reports whether latest is strictly newer than current.
func IsNewer(current, latest string) bool {
	return CompareSemver(current, latest) < 0
}

func parseSemver(raw string) ([3]int, string) {
	var nums [3]int
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")

	var pre string
	if idx := strings.IndexAny(s, "-+"); idx != -1 {
		pre = s[idx+1:]
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err == nil && n >= 0 {
			nums[i] = n
		}
	}
	return nums, pre
}
