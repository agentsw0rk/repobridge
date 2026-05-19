package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func PackageInfo(name, registry string) (*PackageEntry, error) {
	packages, _, err := ListSources()
	if err != nil {
		return nil, err
	}
	for _, pkg := range packages {
		if pkg.Name == name && pkg.Registry == registry {
			copy := pkg
			return &copy, nil
		}
	}
	return nil, nil
}

func RepoInfo(displayName string) (*RepoEntry, error) {
	_, repos, err := ListSources()
	if err != nil {
		return nil, err
	}
	for _, repo := range repos {
		if repo.Name == displayName {
			copy := repo
			return &copy, nil
		}
	}
	return nil, nil
}

func ExtractRepoBasePath(fullPath string) string {
	parts := strings.Split(filepath.ToSlash(fullPath), "/")
	if len(parts) >= 4 && parts[0] == reposDir {
		return strings.Join(parts[:4], "/")
	}
	return fullPath
}

func RemovePackageSource(name, registry string) (bool, bool, error) {
	return removePackageSource(name, registry, nil)
}

func RemovePackageSourceVersion(name, registry, version string) (bool, bool, error) {
	return removePackageSource(name, registry, &version)
}

func removePackageSource(name, registry string, version *string) (bool, bool, error) {
	packages, repos, err := ListSources()
	if err != nil {
		return false, false, err
	}
	var pkg *PackageEntry
	for i := range packages {
		if packageMatches(packages[i], name, registry, version) {
			pkg = &packages[i]
			break
		}
	}
	if pkg == nil {
		return false, false, nil
	}
	target := packageRemovalTarget(pkg.Path, version)
	filtered := make([]PackageEntry, 0, len(packages))
	for _, other := range packages {
		if packageMatches(other, name, registry, version) {
			continue
		}
		filtered = append(filtered, other)
	}
	stillReferenced := packageRemovalTargetReferenced(target, version, filtered, repos)
	if stillReferenced {
		if err := WriteSources(filtered, repos); err != nil {
			return true, false, err
		}
		return true, false, nil
	}
	if err := preflightRemoveSafe(target); err != nil {
		return false, false, err
	}
	if err := WriteSources(filtered, repos); err != nil {
		return true, false, err
	}
	if err := removeAllSafeFunc(target); err != nil {
		if restoreErr := WriteSources(packages, repos); restoreErr != nil {
			return true, true, fmt.Errorf("delete failed: %w; restore failed: %v", err, restoreErr)
		}
		return true, true, err
	}
	cleanupEmptyParents(target)
	return true, true, nil
}

func packageRemovalTarget(path string, version *string) string {
	if version != nil {
		return path
	}
	return ExtractRepoBasePath(path)
}

func packageRemovalTargetReferenced(target string, version *string, packages []PackageEntry, repos []RepoEntry) bool {
	if version != nil {
		return relatedPathReferenced(target, packages, repos)
	}
	for _, other := range packages {
		if ExtractRepoBasePath(other.Path) == target {
			return true
		}
	}
	return repoRootReferenced(target, repos)
}

func packageMatches(pkg PackageEntry, name, registry string, version *string) bool {
	if pkg.Name != name || pkg.Registry != registry {
		return false
	}
	return version == nil || pkg.Version == *version
}

func relatedPathReferenced(path string, packages []PackageEntry, repos []RepoEntry) bool {
	for _, pkg := range packages {
		if pathsRelated(path, pkg.Path) {
			return true
		}
	}
	for _, repo := range repos {
		if pathsRelated(path, repo.Path) {
			return true
		}
	}
	return false
}

func pathsRelated(a, b string) bool {
	a = strings.Trim(filepath.ToSlash(a), "/")
	b = strings.Trim(filepath.ToSlash(b), "/")
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func RemoveRepoSource(displayName string, version *string) (bool, error) {
	if version != nil {
		if _, err := RepoPath(displayName, *version); err != nil {
			return false, err
		}
	} else {
		if _, err := repoRootPath(displayName); err != nil {
			return false, err
		}
	}
	packages, repos, err := ListSources()
	if err != nil {
		return false, err
	}
	filteredRepos := make([]RepoEntry, 0, len(repos))
	removedRepos := make([]RepoEntry, 0)
	for _, repo := range repos {
		matchesName := repo.Name == displayName
		matchesVersion := version == nil || repo.Version == *version
		if matchesName && matchesVersion {
			removedRepos = append(removedRepos, repo)
			continue
		}
		filteredRepos = append(filteredRepos, repo)
	}
	if len(removedRepos) == 0 {
		return false, nil
	}
	deleted := map[string]bool{}
	toDelete := make([]string, 0, len(removedRepos))
	for _, repo := range removedRepos {
		if deleted[repo.Path] || repoPathReferenced(repo.Path, packages, filteredRepos) {
			continue
		}
		toDelete = append(toDelete, repo.Path)
		deleted[repo.Path] = true
	}
	for _, path := range toDelete {
		if err := preflightRemoveSafe(path); err != nil {
			return true, err
		}
	}
	if err := WriteSources(packages, filteredRepos); err != nil {
		return true, err
	}
	for _, path := range toDelete {
		if err := removeAllSafeFunc(path); err != nil {
			restoreRepos, restoreBuildErr := restoreExistingRepoEntries(filteredRepos, removedRepos)
			if restoreBuildErr != nil {
				return true, fmt.Errorf("delete failed: %w; restore failed: %v", err, restoreBuildErr)
			}
			if restoreErr := WriteSources(packages, restoreRepos); restoreErr != nil {
				return true, fmt.Errorf("delete failed: %w; restore failed: %v", err, restoreErr)
			}
			return true, err
		}
		cleanupEmptyParents(path)
	}
	return true, nil
}

func restoreExistingRepoEntries(filteredRepos, removedRepos []RepoEntry) ([]RepoEntry, error) {
	restored := append([]RepoEntry{}, filteredRepos...)
	for _, repo := range removedRepos {
		exists, err := cachePathExists(repo.Path)
		if err != nil {
			return nil, err
		}
		if exists {
			restored = append(restored, repo)
		}
	}
	return restored, nil
}

func cleanupEmptyParents(relativePath string) {
	parts := strings.Split(filepath.ToSlash(relativePath), "/")
	for i := len(parts) - 1; i >= 1; i-- {
		dir, err := AbsolutePath(strings.Join(parts[:i], "/"))
		if err != nil {
			return
		}
		info, err := os.Lstat(dir)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		_ = os.Remove(dir)
	}
}

func repoRootReferenced(base string, repos []RepoEntry) bool {
	for _, repo := range repos {
		if repo.Path == base || ExtractRepoBasePath(repo.Path) == base {
			return true
		}
	}
	return false
}

func repoPathReferenced(path string, packages []PackageEntry, repos []RepoEntry) bool {
	for _, repo := range repos {
		if pathsRelated(path, repo.Path) {
			return true
		}
	}
	for _, pkg := range packages {
		if pathsRelated(path, pkg.Path) {
			return true
		}
	}
	return false
}

func cachePathExists(relativePath string) (bool, error) {
	path, err := AbsolutePath(relativePath)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func preflightRemoveSafe(relativePath string) error {
	if _, err := AbsolutePath(relativePath); err != nil {
		return err
	}
	return rejectSymlinkComponents(relativePath)
}

var removeAllSafeFunc = removeAllSafe

func removeAllSafe(relativePath string) error {
	target, err := AbsolutePath(relativePath)
	if err != nil {
		return err
	}
	if err := rejectSymlinkComponents(relativePath); err != nil {
		return err
	}
	return os.RemoveAll(target)
}

func rejectSymlinkComponents(relativePath string) error {
	home, err := Home()
	if err != nil {
		return err
	}
	current, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	for _, part := range strings.Split(filepath.ToSlash(relativePath), "/") {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to remove cache path through symlink: %s", current)
		}
	}
	return nil
}
