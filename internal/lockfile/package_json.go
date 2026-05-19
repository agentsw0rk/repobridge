package lockfile

import (
	"encoding/json"
	"strings"
)

func versionFromPackageJSON(packageName, cwd string) string {
	content, err := readFile(cwd, "package.json")
	if err != nil {
		return ""
	}
	return parsePackageJSONVersion(content, packageName)
}

func parsePackageJSONVersion(content, packageName string) string {
	var parsed struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return ""
	}
	for _, deps := range []map[string]string{
		parsed.Dependencies,
		parsed.DevDependencies,
		parsed.PeerDependencies,
		parsed.OptionalDependencies,
	} {
		version := deps[packageName]
		if version == "" {
			continue
		}
		stripped := packageJSONVersionSpec(version)
		if isRegistryVersion(stripped) && isSimpleVersion(stripped) {
			return stripped
		}
	}
	return ""
}

func packageJSONVersionSpec(version string) string {
	version = strings.TrimSpace(version)
	switch {
	case strings.HasPrefix(version, ">="):
		return strings.TrimSpace(strings.TrimPrefix(version, ">="))
	case strings.HasPrefix(version, "^"):
		return strings.TrimSpace(strings.TrimPrefix(version, "^"))
	case strings.HasPrefix(version, "~"):
		return strings.TrimSpace(strings.TrimPrefix(version, "~"))
	case strings.HasPrefix(version, "<") || strings.HasPrefix(version, ">") || strings.HasPrefix(version, "="):
		return ""
	default:
		return version
	}
}

func isSimpleVersion(version string) bool {
	if version == "" || version[0] < '0' || version[0] > '9' {
		return false
	}
	return strings.IndexFunc(version, func(r rune) bool {
		return !((r >= '0' && r <= '9') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			r == '.' || r == '-' || r == '_' || r == '+')
	}) == -1 && !strings.ContainsAny(version, "xX")
}

func packageJSONFieldVersion(content string) string {
	var parsed struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return ""
	}
	return parsed.Version
}
