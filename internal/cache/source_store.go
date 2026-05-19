package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SourceCache struct {
	home          string
	clock         func() time.Time
	removeAllFunc func(relativePath string) error
}

type SourceStore = SourceCache

type SourceStoreOption func(*SourceCache)

func WithHome(home string) SourceStoreOption {
	return func(s *SourceCache) {
		s.home = home
	}
}

func WithClock(clock func() time.Time) SourceStoreOption {
	return func(s *SourceCache) {
		s.clock = clock
	}
}

func WithRemoveAll(removeAll func(relativePath string) error) SourceStoreOption {
	return func(s *SourceCache) {
		s.removeAllFunc = removeAll
	}
}

func NewSourceCache(opts ...SourceStoreOption) *SourceCache {
	store := &SourceCache{
		clock:         time.Now,
		removeAllFunc: removeAllSafe,
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

func NewSourceStore(opts ...SourceStoreOption) *SourceStore {
	return NewSourceCache(opts...)
}

type PackageKey struct {
	Name     string
	Registry string
	Version  string
}

type RepoKey struct {
	DisplayName string
	Version     string
}

type CachedPackage struct {
	Name         string
	Registry     string
	Version      string
	Path         string
	RelativePath string
	FetchedAt    string
}

type CachedRepo struct {
	Name         string
	Version      string
	Path         string
	RelativePath string
	FetchedAt    string
}

type FetchedPackage struct {
	Name     string
	Registry string
	Version  string
	Path     string
}

type FetchedRepo struct {
	Name    string
	Version string
	Path    string
}

type SourceKind string

const (
	PackageSource SourceKind = "package"
	RepoSource    SourceKind = "repo"
)

type PackageSelector struct {
	Name     string
	Registry string
	Version  string
}

type RepoSelector struct {
	DisplayName string
	Version     string
}

type RemoveSelector struct {
	Kind    SourceKind
	Package PackageSelector
	Repo    RepoSelector
}

type RemoveResult struct {
	Matched      int
	IndexRemoved int
	FilesDeleted int
	FilesKept    int
	RolledBack   bool
}

type CleanOptions struct {
	Kinds      map[SourceKind]bool
	Registries map[string]bool
}

type CleanResult struct {
	Removed int
	Deleted int
	Kept    int
}

type ListOptions struct {
	Kind       SourceKind
	Registries map[string]bool
}

type ListedSource struct {
	Kind         SourceKind
	Name         string
	Registry     string
	Version      string
	Path         string
	RelativePath string
	FetchedAt    string
}

type ListResult struct {
	Sources []ListedSource
	Index   SourcesIndex
}

func (s *SourceCache) GetPackage(key PackageKey) (*CachedPackage, bool, error) {
	packages, _, err := s.listSources()
	if err != nil {
		return nil, false, err
	}
	for _, entry := range packages {
		if entry.Name != key.Name || entry.Registry != key.Registry || entry.Version != key.Version {
			continue
		}
		ok, err := s.cacheEntryPathExists(entry.Path)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			continue
		}
		abs, err := s.absolutePath(entry.Path)
		if err != nil {
			return nil, false, err
		}
		return &CachedPackage{
			Name:         entry.Name,
			Registry:     entry.Registry,
			Version:      entry.Version,
			Path:         abs,
			RelativePath: entry.Path,
			FetchedAt:    entry.FetchedAt,
		}, true, nil
	}
	return nil, false, nil
}

func (s *SourceCache) RecordPackage(key PackageKey, fetched FetchedPackage) (*CachedPackage, error) {
	name := firstNonEmptyString(fetched.Name, key.Name)
	registry := firstNonEmptyString(fetched.Registry, key.Registry)
	version := firstNonEmptyString(fetched.Version, key.Version)
	relativePath, err := s.relativeCachePath(fetched.Path)
	if err != nil {
		return nil, err
	}
	packages, repos, err := s.listSources()
	if err != nil {
		return nil, err
	}
	entry := PackageEntry{
		Name:      name,
		Version:   version,
		Registry:  registry,
		Path:      relativePath,
		FetchedAt: s.nowString(),
	}
	replaced := false
	for i := range packages {
		if packages[i].Name == key.Name && packages[i].Registry == key.Registry && packages[i].Version == key.Version {
			packages[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		packages = append(packages, entry)
	}
	if err := s.writeSources(packages, repos); err != nil {
		return nil, err
	}
	abs, err := s.absolutePath(relativePath)
	if err != nil {
		return nil, err
	}
	return &CachedPackage{
		Name:         entry.Name,
		Registry:     entry.Registry,
		Version:      entry.Version,
		Path:         abs,
		RelativePath: entry.Path,
		FetchedAt:    entry.FetchedAt,
	}, nil
}

func (s *SourceCache) EnsurePackage(key PackageKey, fetch func() (FetchedPackage, error)) (*CachedPackage, bool, error) {
	if entry, ok, err := s.GetPackage(key); err != nil {
		return nil, false, err
	} else if ok {
		return entry, true, nil
	}
	fetched, err := fetch()
	if err != nil {
		return nil, false, err
	}
	entry, err := s.RecordPackage(key, fetched)
	return entry, false, err
}

func (s *SourceCache) GetRepo(key RepoKey) (*CachedRepo, bool, error) {
	_, repos, err := s.listSources()
	if err != nil {
		return nil, false, err
	}
	for _, entry := range repos {
		if entry.Name != key.DisplayName || entry.Version != key.Version {
			continue
		}
		ok, err := s.cacheEntryPathExists(entry.Path)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			continue
		}
		abs, err := s.absolutePath(entry.Path)
		if err != nil {
			return nil, false, err
		}
		return &CachedRepo{
			Name:         entry.Name,
			Version:      entry.Version,
			Path:         abs,
			RelativePath: entry.Path,
			FetchedAt:    entry.FetchedAt,
		}, true, nil
	}
	return nil, false, nil
}

func (s *SourceCache) RecordRepo(key RepoKey, fetched FetchedRepo) (*CachedRepo, error) {
	name := firstNonEmptyString(fetched.Name, key.DisplayName)
	version := firstNonEmptyString(fetched.Version, key.Version)
	relativePath, err := s.relativeCachePath(fetched.Path)
	if err != nil {
		return nil, err
	}
	packages, repos, err := s.listSources()
	if err != nil {
		return nil, err
	}
	entry := RepoEntry{
		Name:      name,
		Version:   version,
		Path:      relativePath,
		FetchedAt: s.nowString(),
	}
	replaced := false
	for i := range repos {
		if repos[i].Name == key.DisplayName && repos[i].Version == key.Version {
			repos[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		repos = append(repos, entry)
	}
	if err := s.writeSources(packages, repos); err != nil {
		return nil, err
	}
	abs, err := s.absolutePath(relativePath)
	if err != nil {
		return nil, err
	}
	return &CachedRepo{
		Name:         entry.Name,
		Version:      entry.Version,
		Path:         abs,
		RelativePath: entry.Path,
		FetchedAt:    entry.FetchedAt,
	}, nil
}

func (s *SourceCache) EnsureRepo(key RepoKey, fetch func() (FetchedRepo, error)) (*CachedRepo, bool, error) {
	if entry, ok, err := s.GetRepo(key); err != nil {
		return nil, false, err
	} else if ok {
		return entry, true, nil
	}
	fetched, err := fetch()
	if err != nil {
		return nil, false, err
	}
	entry, err := s.RecordRepo(key, fetched)
	return entry, false, err
}

func (s *SourceCache) Remove(sel RemoveSelector) (RemoveResult, error) {
	switch sel.Kind {
	case PackageSource:
		return s.removePackage(sel.Package)
	case RepoSource:
		return s.removeRepo(sel.Repo)
	default:
		return RemoveResult{}, fmt.Errorf("unknown source kind: %s", sel.Kind)
	}
}

func (s *SourceCache) Clean(opts CleanOptions) (CleanResult, error) {
	packages, repos, err := s.listSources()
	if err != nil {
		return CleanResult{}, err
	}
	cleanPackages := len(opts.Kinds) == 0 || opts.Kinds[PackageSource]
	cleanRepos := len(opts.Kinds) == 0 || opts.Kinds[RepoSource]
	if len(opts.Registries) > 0 {
		cleanPackages = true
		cleanRepos = false
	}
	result := CleanResult{}
	if cleanPackages {
		type packageKey struct {
			name     string
			registry string
		}
		seen := map[packageKey]bool{}
		for _, pkg := range packages {
			if len(opts.Registries) > 0 && !opts.Registries[pkg.Registry] {
				continue
			}
			key := packageKey{name: pkg.Name, registry: pkg.Registry}
			if seen[key] {
				result.Removed++
				continue
			}
			seen[key] = true
			removed, err := s.Remove(RemoveSelector{
				Kind: PackageSource,
				Package: PackageSelector{
					Name:     pkg.Name,
					Registry: pkg.Registry,
				},
			})
			if err != nil {
				return result, err
			}
			result.Removed += removed.IndexRemoved
			result.Deleted += removed.FilesDeleted
			result.Kept += removed.FilesKept
		}
	}
	if cleanRepos {
		type repoKey struct {
			name    string
			version string
		}
		seen := map[repoKey]bool{}
		for _, repo := range repos {
			key := repoKey{name: repo.Name, version: repo.Version}
			if seen[key] {
				continue
			}
			seen[key] = true
			removed, err := s.Remove(RemoveSelector{
				Kind: RepoSource,
				Repo: RepoSelector{
					DisplayName: repo.Name,
					Version:     repo.Version,
				},
			})
			if err != nil {
				return result, err
			}
			result.Removed += removed.IndexRemoved
			result.Deleted += removed.FilesDeleted
			result.Kept += removed.FilesKept
		}
	}
	return result, nil
}

func (s *SourceCache) List(opts ListOptions) (ListResult, error) {
	index, err := s.readSources()
	if err != nil {
		return ListResult{}, err
	}
	result := ListResult{Index: index}
	if opts.Kind == "" || opts.Kind == PackageSource {
		for _, pkg := range index.Packages {
			if len(opts.Registries) > 0 && !opts.Registries[pkg.Registry] {
				continue
			}
			abs, err := s.absolutePath(pkg.Path)
			if err != nil {
				return ListResult{}, err
			}
			result.Sources = append(result.Sources, ListedSource{
				Kind:         PackageSource,
				Name:         pkg.Name,
				Registry:     pkg.Registry,
				Version:      pkg.Version,
				Path:         abs,
				RelativePath: pkg.Path,
				FetchedAt:    pkg.FetchedAt,
			})
		}
	}
	if opts.Kind == "" || opts.Kind == RepoSource {
		for _, repo := range index.Repos {
			abs, err := s.absolutePath(repo.Path)
			if err != nil {
				return ListResult{}, err
			}
			result.Sources = append(result.Sources, ListedSource{
				Kind:         RepoSource,
				Name:         repo.Name,
				Version:      repo.Version,
				Path:         abs,
				RelativePath: repo.Path,
				FetchedAt:    repo.FetchedAt,
			})
		}
	}
	return result, nil
}

func (s *SourceCache) removePackage(sel PackageSelector) (RemoveResult, error) {
	packages, repos, err := s.listSources()
	if err != nil {
		return RemoveResult{}, err
	}
	var version *string
	if sel.Version != "" {
		version = &sel.Version
	}
	var pkg *PackageEntry
	for i := range packages {
		if packageMatches(packages[i], sel.Name, sel.Registry, version) {
			pkg = &packages[i]
			break
		}
	}
	if pkg == nil {
		return RemoveResult{}, nil
	}
	target := packageRemovalTarget(pkg.Path, version)
	filtered := make([]PackageEntry, 0, len(packages))
	removedEntries := 0
	for _, other := range packages {
		if packageMatches(other, sel.Name, sel.Registry, version) {
			removedEntries++
			continue
		}
		filtered = append(filtered, other)
	}
	result := RemoveResult{Matched: removedEntries, IndexRemoved: removedEntries}
	if packageRemovalTargetReferenced(target, version, filtered, repos) {
		if err := s.writeSources(filtered, repos); err != nil {
			return result, err
		}
		result.FilesKept = 1
		return result, nil
	}
	if err := s.preflightRemoveSafe(target); err != nil {
		return RemoveResult{Matched: removedEntries}, err
	}
	if err := s.writeSources(filtered, repos); err != nil {
		return result, err
	}
	if err := s.removeAll(target); err != nil {
		result.RolledBack = true
		if restoreErr := s.writeSources(packages, repos); restoreErr != nil {
			return result, fmt.Errorf("delete failed: %w; restore failed: %v", err, restoreErr)
		}
		return result, err
	}
	s.cleanupEmptyParents(target)
	result.FilesDeleted = 1
	return result, nil
}

func (s *SourceCache) removeRepo(sel RepoSelector) (RemoveResult, error) {
	if sel.Version != "" {
		if _, err := RepoPath(sel.DisplayName, sel.Version); err != nil {
			return RemoveResult{}, err
		}
	} else {
		if _, err := repoRootPath(sel.DisplayName); err != nil {
			return RemoveResult{}, err
		}
	}
	packages, repos, err := s.listSources()
	if err != nil {
		return RemoveResult{}, err
	}
	filteredRepos := make([]RepoEntry, 0, len(repos))
	removedRepos := make([]RepoEntry, 0)
	for _, repo := range repos {
		matchesName := repo.Name == sel.DisplayName
		matchesVersion := sel.Version == "" || repo.Version == sel.Version
		if matchesName && matchesVersion {
			removedRepos = append(removedRepos, repo)
			continue
		}
		filteredRepos = append(filteredRepos, repo)
	}
	if len(removedRepos) == 0 {
		return RemoveResult{}, nil
	}
	result := RemoveResult{Matched: len(removedRepos), IndexRemoved: len(removedRepos)}
	deleted := map[string]bool{}
	toDelete := make([]string, 0, len(removedRepos))
	for _, repo := range removedRepos {
		if deleted[repo.Path] || repoPathReferenced(repo.Path, packages, filteredRepos) {
			result.FilesKept++
			continue
		}
		toDelete = append(toDelete, repo.Path)
		deleted[repo.Path] = true
	}
	for _, path := range toDelete {
		if err := s.preflightRemoveSafe(path); err != nil {
			return result, err
		}
	}
	if err := s.writeSources(packages, filteredRepos); err != nil {
		return result, err
	}
	for _, path := range toDelete {
		if err := s.removeAll(path); err != nil {
			result.RolledBack = true
			restoreRepos, restoreBuildErr := s.restoreExistingRepoEntries(filteredRepos, removedRepos)
			if restoreBuildErr != nil {
				return result, fmt.Errorf("delete failed: %w; restore failed: %v", err, restoreBuildErr)
			}
			if restoreErr := s.writeSources(packages, restoreRepos); restoreErr != nil {
				return result, fmt.Errorf("delete failed: %w; restore failed: %v", err, restoreErr)
			}
			return result, err
		}
		s.cleanupEmptyParents(path)
		result.FilesDeleted++
	}
	return result, nil
}

func (s *SourceCache) listSources() ([]PackageEntry, []RepoEntry, error) {
	index, err := s.readSources()
	if err != nil {
		return nil, nil, err
	}
	return index.Packages, index.Repos, nil
}

func (s *SourceCache) readSources() (SourcesIndex, error) {
	home, err := s.homeDir()
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

func (s *SourceCache) writeSources(packages []PackageEntry, repos []RepoEntry) error {
	home, err := s.homeDir()
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
		UpdatedAt: s.nowString(),
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

func (s *SourceCache) cacheEntryPathExists(relativePath string) (bool, error) {
	abs, err := s.absolutePath(relativePath)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name() != ".git" {
			return true, nil
		}
	}
	return false, nil
}

func (s *SourceCache) cachePathExists(relativePath string) (bool, error) {
	path, err := s.absolutePath(relativePath)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *SourceCache) preflightRemoveSafe(relativePath string) error {
	if _, err := s.absolutePath(relativePath); err != nil {
		return err
	}
	return s.rejectSymlinkComponents(relativePath)
}

func (s *SourceCache) removeAll(relativePath string) error {
	if s.removeAllFunc != nil {
		return s.removeAllFunc(relativePath)
	}
	return removeAllSafe(relativePath)
}

func (s *SourceCache) rejectSymlinkComponents(relativePath string) error {
	home, err := s.homeDir()
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

func (s *SourceCache) cleanupEmptyParents(relativePath string) {
	parts := strings.Split(filepath.ToSlash(relativePath), "/")
	for i := len(parts) - 1; i >= 1; i-- {
		dir, err := s.absolutePath(strings.Join(parts[:i], "/"))
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

func (s *SourceCache) restoreExistingRepoEntries(filteredRepos, removedRepos []RepoEntry) ([]RepoEntry, error) {
	restored := append([]RepoEntry{}, filteredRepos...)
	for _, repo := range removedRepos {
		exists, err := s.cachePathExists(repo.Path)
		if err != nil {
			return nil, err
		}
		if exists {
			restored = append(restored, repo)
		}
	}
	return restored, nil
}

func (s *SourceCache) relativeCachePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("cache path must not be empty")
	}
	if !filepath.IsAbs(filepath.FromSlash(path)) {
		relative := filepath.ToSlash(path)
		if _, err := s.absolutePath(relative); err != nil {
			return "", err
		}
		return relative, nil
	}
	home, err := s.homeDir()
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(home, path)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cache path is outside REPOBRIDGE_HOME: %s", path)
	}
	relative := filepath.ToSlash(rel)
	if _, err := s.absolutePath(relative); err != nil {
		return "", err
	}
	return relative, nil
}

func (s *SourceCache) absolutePath(relativePath string) (string, error) {
	home, err := s.homeDir()
	if err != nil {
		return "", err
	}
	return resolveUnderHome(home, relativePath)
}

func (s *SourceCache) homeDir() (string, error) {
	if s.home != "" {
		return s.home, nil
	}
	return Home()
}

func (s *SourceCache) nowString() string {
	return s.clock().UTC().Format(time.RFC3339)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
