package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"repobridge/internal/repobridge"
)

const (
	envHome     = "REPOBRIDGE_HOME"
	defaultDir  = ".repobridge"
	reposDir    = "repos"
	sourcesFile = "sources.json"
)

type PackageEntry struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Registry  string `json:"registry"`
	Path      string `json:"path"`
	FetchedAt string `json:"fetchedAt"`
}

type RepoEntry struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Path      string `json:"path"`
	FetchedAt string `json:"fetchedAt"`
}

type SourcesIndex struct {
	UpdatedAt string         `json:"updatedAt,omitempty"`
	Packages  []PackageEntry `json:"packages,omitempty"`
	Repos     []RepoEntry    `json:"repos,omitempty"`
}

func Home() (string, error) {
	if home := os.Getenv(envHome); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", repobridge.HomeDirNotFoundError{}
	}
	return filepath.Join(home, defaultDir), nil
}

func ReposDir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, reposDir), nil
}

func AbsolutePath(relativePath string) (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return resolveUnderHome(home, relativePath)
}

func RepoRelativePath(displayName, version string) string {
	return filepath.ToSlash(filepath.Join(reposDir, displayName, version))
}

func RepoPath(displayName, version string) (string, error) {
	if err := validateRelativePath(displayName); err != nil {
		return "", err
	}
	if err := validateRelativePath(version); err != nil {
		return "", err
	}
	return AbsolutePath(filepath.ToSlash(filepath.Join(reposDir, filepath.FromSlash(displayName), filepath.FromSlash(version))))
}

func ReadSources() (SourcesIndex, error) {
	return NewSourceCache().readSources()
}

func ListSources() ([]PackageEntry, []RepoEntry, error) {
	return NewSourceCache().listSources()
}

func WriteSources(packages []PackageEntry, repos []RepoEntry) error {
	return NewSourceCache().writeSources(packages, repos)
}

func repoRootPath(displayName string) (string, error) {
	if err := validateRelativePath(displayName); err != nil {
		return "", err
	}
	return AbsolutePath(filepath.ToSlash(filepath.Join(reposDir, filepath.FromSlash(displayName))))
}

func resolveUnderHome(home, relativePath string) (string, error) {
	if err := validateRelativePath(relativePath); err != nil {
		return "", err
	}
	homeAbs, err := filepath.Abs(home)
	if err != nil {
		return "", err
	}
	joinedAbs, err := filepath.Abs(filepath.Join(homeAbs, filepath.FromSlash(relativePath)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(homeAbs, joinedAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("cache path escapes home: %s", relativePath)
	}
	return joinedAbs, nil
}

func validateRelativePath(path string) error {
	if path == "" {
		return fmt.Errorf("cache path must not be empty")
	}
	fromSlash := filepath.FromSlash(path)
	if filepath.IsAbs(fromSlash) {
		return fmt.Errorf("cache path must be relative: %s", path)
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return fmt.Errorf("cache path must not contain '..': %s", path)
		}
	}
	return nil
}
