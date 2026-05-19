package lockfile

import (
	"os"
	"path/filepath"
	"strings"
)

func DetectInstalledVersion(packageName, cwd string) string {
	if cwd == "" {
		cwd = "."
	}
	if version := versionFromNodeModules(packageName, cwd); version != "" {
		return version
	}
	if version := versionFromPackageLock(packageName, cwd); version != "" {
		return version
	}
	if version := versionFromPNPMLock(packageName, cwd); version != "" {
		return version
	}
	if version := versionFromYarnLock(packageName, cwd); version != "" {
		return version
	}
	return versionFromPackageJSON(packageName, cwd)
}

func versionFromNodeModules(packageName, cwd string) string {
	content, err := os.ReadFile(filepath.Join(cwd, "node_modules", filepath.FromSlash(packageName), "package.json"))
	if err != nil {
		return ""
	}
	return packageJSONFieldVersion(string(content))
}

func stripVersionPrefix(version string) string {
	return strings.TrimLeft(version, "^~>=<")
}

func isRegistryVersion(version string) bool {
	return version != "" && version != "0.0.0-use.local" && !strings.Contains(version, ":")
}

func readFile(cwd, name string) (string, error) {
	content, err := os.ReadFile(filepath.Join(cwd, name))
	if err != nil {
		return "", err
	}
	return string(content), nil
}
