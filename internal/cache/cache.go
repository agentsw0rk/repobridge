package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	home, err := Home()
	if err != nil {
		return SourcesIndex{}, err
	}
	path := filepath.Join(home, sourcesFile)
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return SourcesIndex{}, nil
	}
	if err != nil {
		return SourcesIndex{}, err
	}
	var index SourcesIndex
	if err := json.Unmarshal(content, &index); err != nil {
		bak := filepath.Join(home, "sources.json.bak")
		fmt.Fprintf(os.Stderr, "Warning: %s is corrupt (%v), backing up to %s\n", path, err, bak)
		if err := os.WriteFile(bak, content, 0o644); err != nil {
			return SourcesIndex{}, err
		}
		return SourcesIndex{}, nil
	}
	return index, nil
}

func ListSources() ([]PackageEntry, []RepoEntry, error) {
	index, err := ReadSources()
	if err != nil {
		return nil, nil, err
	}
	return index.Packages, index.Repos, nil
}

func WriteSources(packages []PackageEntry, repos []RepoEntry) error {
	home, err := Home()
	if err != nil {
		return err
	}
	path := filepath.Join(home, sourcesFile)
	if len(packages) == 0 && len(repos) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	index := SourcesIndex{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Packages:  packages,
		Repos:     repos,
	}
	content, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(home, ".sources.json.tmp")
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
