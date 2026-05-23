package updatecheck

import (
	"fmt"
	"strconv"
	"strings"
)

type semanticVersion struct {
	major int
	minor int
	patch int
}

func IsReleaseVersion(version string) bool {
	_, err := parseVersion(version)
	return err == nil
}

func CompareVersions(a, b string) (int, error) {
	av, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	bv, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	switch {
	case av.major != bv.major:
		return compareInt(av.major, bv.major), nil
	case av.minor != bv.minor:
		return compareInt(av.minor, bv.minor), nil
	default:
		return compareInt(av.patch, bv.patch), nil
	}
}

func parseVersion(version string) (semanticVersion, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("invalid release version %q", version)
	}
	for _, part := range parts {
		if !isDecimalDigits(part) {
			return semanticVersion{}, fmt.Errorf("invalid release version %q", version)
		}
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid release version %q", version)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid release version %q", version)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid release version %q", version)
	}
	return semanticVersion{major: major, minor: minor, patch: patch}, nil
}

func isDecimalDigits(component string) bool {
	if component == "" {
		return false
	}
	for _, r := range component {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareInt(a, b int) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}
