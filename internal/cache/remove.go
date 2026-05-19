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
	result, err := NewSourceCache(WithRemoveAll(removeAllSafeFunc)).Remove(RemoveSelector{
		Kind: PackageSource,
		Package: PackageSelector{
			Name:     name,
			Registry: registry,
		},
	})
	return result.Matched > 0, result.FilesDeleted > 0, err
}

func RemovePackageSourceVersion(name, registry, version string) (bool, bool, error) {
	result, err := NewSourceCache(WithRemoveAll(removeAllSafeFunc)).Remove(RemoveSelector{
		Kind: PackageSource,
		Package: PackageSelector{
			Name:     name,
			Registry: registry,
			Version:  version,
		},
	})
	return result.Matched > 0, result.FilesDeleted > 0, err
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
	selector := RepoSelector{DisplayName: displayName}
	if version != nil {
		selector.Version = *version
	}
	result, err := NewSourceCache(WithRemoveAll(removeAllSafeFunc)).Remove(RemoveSelector{
		Kind: RepoSource,
		Repo: selector,
	})
	return result.Matched > 0, err
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
