package lockfile

import (
	"encoding/json"
	"sort"
	"strings"
)

type packageLockDependency struct {
	Version      string                           `json:"version"`
	Dependencies map[string]packageLockDependency `json:"dependencies"`
}

func versionFromPackageLock(packageName, cwd string) string {
	content, err := readFile(cwd, "package-lock.json")
	if err != nil {
		return ""
	}
	return parsePackageLock(content, packageName)
}

func parsePackageLock(content, packageName string) string {
	var parsed struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
		Dependencies map[string]packageLockDependency `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return ""
	}
	if pkg, ok := parsed.Packages["node_modules/"+packageName]; ok {
		if isRegistryVersion(pkg.Version) {
			return pkg.Version
		}
		return ""
	}
	if version := nestedPackageLockV7Version(parsed.Packages, packageName); version != "" {
		return version
	}
	if dep, ok := parsed.Dependencies[packageName]; ok {
		if isRegistryVersion(dep.Version) {
			return dep.Version
		}
		return ""
	}
	return nestedPackageLockV6Version(parsed.Dependencies, packageName)
}

func nestedPackageLockV7Version(packages map[string]struct {
	Version string `json:"version"`
}, packageName string) string {
	suffix := "/node_modules/" + packageName
	keys := make([]string, 0, len(packages))
	for key := range packages {
		if strings.HasSuffix(key, suffix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if version := packages[key].Version; isRegistryVersion(version) {
			return version
		}
	}
	return ""
}

func nestedPackageLockV6Version(dependencies map[string]packageLockDependency, packageName string) string {
	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dep := dependencies[name]
		if name == packageName {
			if isRegistryVersion(dep.Version) {
				return dep.Version
			}
			continue
		}
		if version := nestedPackageLockV6Version(dep.Dependencies, packageName); version != "" {
			return version
		}
	}
	return ""
}
