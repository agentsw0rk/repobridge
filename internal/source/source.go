package source

import (
	"fmt"
	"net/http"
	"strings"

	"repobridge/internal/cache"
	"repobridge/internal/lockfile"
	"repobridge/internal/registry"
	"repobridge/internal/registry/crates"
	"repobridge/internal/registry/maven"
	"repobridge/internal/registry/npm"
	"repobridge/internal/registry/nuget"
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
	store := cache.NewSourceStore()
	spec := registry.ParsePackageSpec(input)
	if spec.Name == "" {
		return Outcome{}, fmt.Errorf("package name must not be empty")
	}
	if spec.Registry == registry.NPM && spec.Version == "" {
		spec.Version = lockfile.DetectInstalledVersion(spec.Name, opts.CWD)
	}
	if spec.Version != "" {
		key := cache.PackageKey{Name: spec.Name, Registry: string(spec.Registry), Version: spec.Version}
		if entry, ok, err := store.GetPackage(key); err != nil {
			return Outcome{}, err
		} else if ok {
			return Outcome{
				Path:        entry.Path,
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
	key := cache.PackageKey{Name: resolved.Name, Registry: string(resolved.Registry), Version: resolved.Version}
	if entry, ok, err := store.GetPackage(key); err != nil {
		return Outcome{}, err
	} else if ok {
		return Outcome{
			Path:        entry.Path,
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
	entry, err := store.RecordPackage(key, cache.FetchedPackage{
		Name:     result.Package,
		Registry: string(result.Registry),
		Version:  result.Version,
		Path:     result.Path,
	})
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		Path:        entry.Path,
		Name:        entry.Name,
		Version:     entry.Version,
		SourceLabel: registry.Registry(entry.Registry).Label(),
		Warning:     result.Warning,
	}, nil
}

func ensureRepoCached(input string, opts Options) (Outcome, error) {
	store := cache.NewSourceStore()
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
	key := cache.RepoKey{DisplayName: resolved.DisplayName, Version: resolved.GitRef}
	if entry, ok, err := store.GetRepo(key); err != nil {
		return Outcome{}, err
	} else if ok {
		return Outcome{
			Path:        entry.Path,
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
	entry, err := store.RecordRepo(key, cache.FetchedRepo{
		Name:    result.Package,
		Version: result.Version,
		Path:    result.Path,
	})
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		Path:        entry.Path,
		Name:        entry.Name,
		Version:     entry.Version,
		SourceLabel: entry.Name,
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
	case registry.NuGet:
		return nuget.Resolve(spec.Name, spec.Version, client, "")
	default:
		return registry.ResolvedPackage{}, fmt.Errorf("unsupported registry: %s", spec.Registry)
	}
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
