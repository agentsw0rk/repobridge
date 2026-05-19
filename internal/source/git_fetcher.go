package source

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"repobridge/internal/cache"
	"repobridge/internal/git"
	"repobridge/internal/registry"
)

type GitFetcher struct{}

var cloneAtTag = git.CloneAtTag
var cloneAtRef = git.CloneAtRef

func (GitFetcher) FetchPackage(pkg registry.ResolvedPackage) FetchResult {
	return FetchPackageWithGit(pkg)
}

func (GitFetcher) FetchRepo(displayName, repoURL, gitRef string) FetchResult {
	return FetchRepoWithGit(displayName, repoURL, gitRef)
}

func FetchPackageWithGit(pkg registry.ResolvedPackage) FetchResult {
	displayName := repoDisplayName(pkg.RepoURL)
	if displayName == "" {
		return FetchResult{Error: fmt.Errorf("could not derive repository name from %q", pkg.RepoURL)}
	}
	target, err := cache.RepoPath(displayName, pkg.Version)
	if err != nil {
		return FetchResult{Error: err}
	}
	relativePath, err := packageRelativePath(displayName, pkg.Version, pkg.RepoDirectory)
	if err != nil {
		return FetchResult{Error: err}
	}
	if ok, err := reusableTarget(target); err != nil {
		return FetchResult{Error: err}
	} else if ok {
		if err := ensureRepoDirectoryExists(target, pkg.RepoDirectory); err != nil {
			return FetchResult{Error: err}
		}
		return FetchResult{
			Package:  pkg.Name,
			Version:  pkg.Version,
			Path:     relativePath,
			Success:  true,
			Registry: pkg.Registry,
		}
	}
	clone := cloneAtTag(pkg.RepoURL, target, pkg.Version)
	if clone.Error != nil || !clone.Success {
		return FetchResult{Success: clone.Success, Warning: clone.Warning, Error: clone.Error}
	}
	if err := git.RemoveGitDir(target); err != nil {
		return FetchResult{Error: err}
	}
	if err := ensureRepoDirectoryExists(target, pkg.RepoDirectory); err != nil {
		return FetchResult{Error: err}
	}

	return FetchResult{
		Package:  pkg.Name,
		Version:  pkg.Version,
		Path:     relativePath,
		Success:  true,
		Warning:  clone.Warning,
		Registry: pkg.Registry,
	}
}

func FetchRepoWithGit(displayName, repoURL, gitRef string) FetchResult {
	target, err := cache.RepoPath(displayName, gitRef)
	if err != nil {
		return FetchResult{Error: err}
	}
	if ok, err := reusableTarget(target); err != nil {
		return FetchResult{Error: err}
	} else if ok {
		return FetchResult{
			Package: displayName,
			Version: gitRef,
			Path:    cache.RepoRelativePath(displayName, gitRef),
			Success: true,
		}
	}
	clone := cloneAtRef(repoURL, target, gitRef)
	if clone.Error != nil || !clone.Success {
		return FetchResult{Success: clone.Success, Warning: clone.Warning, Error: clone.Error}
	}
	if err := git.RemoveGitDir(target); err != nil {
		return FetchResult{Error: err}
	}
	return FetchResult{
		Package: displayName,
		Version: gitRef,
		Path:    cache.RepoRelativePath(displayName, gitRef),
		Success: true,
		Warning: clone.Warning,
	}
}

func repoDisplayName(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || strings.EqualFold(part, "-") {
			break
		}
		clean = append(clean, part)
	}
	if len(clean) < 2 {
		return ""
	}
	clean[len(clean)-1] = strings.TrimSuffix(clean[len(clean)-1], ".git")
	if clean[len(clean)-1] == "" {
		return ""
	}
	return strings.ToLower(parsed.Host) + "/" + strings.Join(clean, "/")
}

func packageRelativePath(displayName, version, repoDirectory string) (string, error) {
	base := cache.RepoRelativePath(displayName, version)
	if repoDirectory == "" {
		return base, nil
	}
	normalized := strings.ReplaceAll(repoDirectory, "\\", "/")
	if filepath.IsAbs(filepath.FromSlash(normalized)) {
		return "", fmt.Errorf("repository directory must be relative: %s", repoDirectory)
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", fmt.Errorf("repository directory must not contain '..': %s", repoDirectory)
		}
	}
	relativePath := filepath.ToSlash(filepath.Join(base, filepath.FromSlash(normalized)))
	if relativePath != base && !strings.HasPrefix(relativePath, base+"/") {
		return "", fmt.Errorf("repository directory escapes package source: %s", repoDirectory)
	}
	return relativePath, nil
}

func ensureRepoDirectoryExists(target, repoDirectory string) error {
	if repoDirectory == "" {
		return nil
	}
	path := filepath.Join(target, filepath.FromSlash(repoDirectory))
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("repository directory does not exist: %s", repoDirectory)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("repository directory is not a directory: %s", repoDirectory)
	}
	return nil
}

func reusableTarget(target string) (bool, error) {
	info, err := os.Stat(target)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("cache target exists and is not a directory: %s", target)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		if err := os.Remove(target); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := git.RemoveGitDir(target); err != nil {
		return false, err
	}
	entries, err = os.ReadDir(target)
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		if err := os.Remove(target); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}
