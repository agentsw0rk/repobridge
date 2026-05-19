package source

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"repobridge/internal/cache"
	"repobridge/internal/lockfile"
	"repobridge/internal/registry"
	"repobridge/internal/registry/crates"
	"repobridge/internal/registry/maven"
	"repobridge/internal/registry/npm"
	"repobridge/internal/registry/pypi"
	"repobridge/internal/registry/repo"
	"repobridge/internal/repobridge"
)

type Outcome struct {
	Path        string
	Name        string
	Version     string
	SourceLabel string
	FromCache   bool
	Warning     string
}

type Options struct {
	CWD     string
	Verbose bool
	Client  *http.Client
	Fetcher Fetcher
}

type Fetcher interface {
	FetchPackage(registry.ResolvedPackage) FetchResult
	FetchRepo(displayName, repoURL, gitRef string) FetchResult
}

type FetchResult struct {
	Package  string
	Version  string
	Path     string
	Success  bool
	Warning  string
	Error    error
	Registry registry.Registry
}

func EnsureCached(spec string, opts Options) (Outcome, error) {
	switch registry.DetectInputType(spec) {
	case registry.RepoInput:
		return ensureRepoCached(spec, opts)
	default:
		return ensurePackageCached(spec, opts)
	}
}

func ensurePackageCached(input string, opts Options) (Outcome, error) {
	spec := registry.ParsePackageSpec(input)
	if spec.Name == "" {
		return Outcome{}, fmt.Errorf("package name must not be empty")
	}
	if spec.Registry == registry.NPM && spec.Version == "" {
		spec.Version = lockfile.DetectInstalledVersion(spec.Name, opts.CWD)
	}
	if spec.Version != "" {
		if entry, err := cachedPackage(spec.Name, string(spec.Registry), spec.Version); err != nil {
			return Outcome{}, err
		} else if entry != nil {
			abs, err := cache.AbsolutePath(entry.Path)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{
				Path:        abs,
				Name:        entry.Name,
				Version:     entry.Version,
				SourceLabel: registry.Registry(entry.Registry).Label(),
				FromCache:   true,
			}, nil
		}
	}

	resolved, err := resolvePackage(spec, opts.Client)
	if err != nil {
		return Outcome{}, err
	}
	if entry, err := cachedPackage(resolved.Name, string(resolved.Registry), resolved.Version); err != nil {
		return Outcome{}, err
	} else if entry != nil {
		abs, err := cache.AbsolutePath(entry.Path)
		if err != nil {
			return Outcome{}, err
		}
		return Outcome{
			Path:        abs,
			Name:        entry.Name,
			Version:     entry.Version,
			SourceLabel: registry.Registry(entry.Registry).Label(),
			FromCache:   true,
		}, nil
	}

	fetcher := opts.Fetcher
	if fetcher == nil {
		fetcher = GitFetcher{Client: opts.Client}
	}
	result := fetcher.FetchPackage(resolved)
	if err := fetchError(result); err != nil {
		return Outcome{}, err
	}
	if result.Package == "" {
		result.Package = resolved.Name
	}
	if result.Version == "" {
		result.Version = resolved.Version
	}
	if result.Registry == "" {
		result.Registry = resolved.Registry
	}
	relativePath, err := relativeCachePath(result.Path)
	if err != nil {
		return Outcome{}, err
	}
	if err := upsertPackage(result.Package, string(result.Registry), result.Version, relativePath); err != nil {
		return Outcome{}, err
	}
	abs, err := cache.AbsolutePath(relativePath)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		Path:        abs,
		Name:        result.Package,
		Version:     result.Version,
		SourceLabel: result.Registry.Label(),
		Warning:     result.Warning,
	}, nil
}

func ensureRepoCached(input string, opts Options) (Outcome, error) {
	spec, ok := repo.ParseSpec(input)
	if !ok {
		return Outcome{}, repobridge.InvalidRepoSpecError{Spec: input}
	}
	resolved := repo.Resolved{
		GitRef:      spec.Ref,
		RepoURL:     fmt.Sprintf("https://%s/%s/%s", spec.Host, spec.Owner, spec.Repo),
		DisplayName: fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo),
	}
	if resolved.GitRef == "" {
		var err error
		resolved, err = repo.Resolve(spec, opts.Client)
		if err != nil {
			return Outcome{}, err
		}
	}
	if entry, err := cachedRepo(resolved.DisplayName, resolved.GitRef); err != nil {
		return Outcome{}, err
	} else if entry != nil {
		abs, err := cache.AbsolutePath(entry.Path)
		if err != nil {
			return Outcome{}, err
		}
		return Outcome{
			Path:        abs,
			Name:        entry.Name,
			Version:     entry.Version,
			SourceLabel: entry.Name,
			FromCache:   true,
		}, nil
	}

	fetcher := opts.Fetcher
	if fetcher == nil {
		fetcher = GitFetcher{Client: opts.Client}
	}
	result := fetcher.FetchRepo(resolved.DisplayName, resolved.RepoURL, resolved.GitRef)
	if err := fetchError(result); err != nil {
		return Outcome{}, err
	}
	if result.Package == "" {
		result.Package = resolved.DisplayName
	}
	if result.Version == "" {
		result.Version = resolved.GitRef
	}
	relativePath, err := relativeCachePath(result.Path)
	if err != nil {
		return Outcome{}, err
	}
	if err := upsertRepo(result.Package, result.Version, relativePath); err != nil {
		return Outcome{}, err
	}
	abs, err := cache.AbsolutePath(relativePath)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		Path:        abs,
		Name:        result.Package,
		Version:     result.Version,
		SourceLabel: result.Package,
		Warning:     result.Warning,
	}, nil
}

var resolvePackage = defaultResolvePackage

func defaultResolvePackage(spec registry.PackageSpec, client *http.Client) (registry.ResolvedPackage, error) {
	if err := registry.SupportedRegistry(spec.Registry); err != nil {
		return registry.ResolvedPackage{}, err
	}
	switch spec.Registry {
	case registry.NPM:
		return npm.Resolve(spec.Name, spec.Version, client, "")
	case registry.PyPI:
		return pypi.Resolve(spec.Name, spec.Version, client, "")
	case registry.Crates:
		return crates.Resolve(spec.Name, spec.Version, client, "")
	case registry.Maven:
		return maven.Resolve(spec.Name, spec.Version, client, "")
	default:
		return registry.ResolvedPackage{}, fmt.Errorf("unsupported registry: %s", spec.Registry)
	}
}

func cachedPackage(name, registryName, version string) (*cache.PackageEntry, error) {
	packages, _, err := cache.ListSources()
	if err != nil {
		return nil, err
	}
	for _, entry := range packages {
		if entry.Name == name && entry.Registry == registryName && entry.Version == version {
			ok, err := cacheEntryPathExists(entry.Path)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			copy := entry
			return &copy, nil
		}
	}
	return nil, nil
}

func cachedRepo(displayName, version string) (*cache.RepoEntry, error) {
	_, repos, err := cache.ListSources()
	if err != nil {
		return nil, err
	}
	for _, entry := range repos {
		if entry.Name == displayName && entry.Version == version {
			ok, err := cacheEntryPathExists(entry.Path)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			copy := entry
			return &copy, nil
		}
	}
	return nil, nil
}

func cacheEntryPathExists(relativePath string) (bool, error) {
	abs, err := cache.AbsolutePath(relativePath)
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

func upsertPackage(name, registryName, version, relativePath string) error {
	packages, repos, err := cache.ListSources()
	if err != nil {
		return err
	}
	entry := cache.PackageEntry{
		Name:      name,
		Version:   version,
		Registry:  registryName,
		Path:      relativePath,
		FetchedAt: cacheNow(),
	}
	for i := range packages {
		if packages[i].Name == name && packages[i].Registry == registryName && packages[i].Version == version {
			packages[i] = entry
			return cache.WriteSources(packages, repos)
		}
	}
	packages = append(packages, entry)
	return cache.WriteSources(packages, repos)
}

func upsertRepo(displayName, version, relativePath string) error {
	packages, repos, err := cache.ListSources()
	if err != nil {
		return err
	}
	entry := cache.RepoEntry{
		Name:      displayName,
		Version:   version,
		Path:      relativePath,
		FetchedAt: cacheNow(),
	}
	for i := range repos {
		if repos[i].Name == displayName && repos[i].Version == version {
			repos[i] = entry
			return cache.WriteSources(packages, repos)
		}
	}
	repos = append(repos, entry)
	return cache.WriteSources(packages, repos)
}

func cacheNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func fetchError(result FetchResult) error {
	if result.Error != nil {
		return result.Error
	}
	if !result.Success {
		return fmt.Errorf("fetch failed")
	}
	if strings.TrimSpace(result.Path) == "" {
		return fmt.Errorf("fetch result missing path")
	}
	return nil
}

func relativeCachePath(path string) (string, error) {
	if !filepath.IsAbs(filepath.FromSlash(path)) {
		relative := filepath.ToSlash(path)
		if _, err := cache.AbsolutePath(relative); err != nil {
			return "", err
		}
		return relative, nil
	}
	home, err := cache.Home()
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(home, path)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("cache path is outside REPOBRIDGE_HOME: %s", path)
	}
	relative := filepath.ToSlash(rel)
	if _, err := cache.AbsolutePath(relative); err != nil {
		return "", err
	}
	return relative, nil
}
