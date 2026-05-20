package source

import (
	"context"
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

type Request struct {
	Spec string
	CWD  string
}

type Acquirer struct {
	client          *http.Client
	fetcher         Fetcher
	resolver        Resolver
	versionDetector InstalledVersionDetector
	store           *cache.SourceStore
}

type Option func(*Acquirer)

type Resolver interface {
	ResolvePackage(context.Context, registry.PackageSpec, *http.Client) (registry.ResolvedPackage, error)
	ResolveRepo(context.Context, repo.Spec, *http.Client) (repo.Resolved, error)
}

type InstalledVersionDetector interface {
	InstalledVersion(registry.Registry, string, string) string
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

func NewAcquirer(opts ...Option) *Acquirer {
	acquirer := &Acquirer{
		resolver:        defaultResolver{},
		versionDetector: lockfileVersionDetector{},
		store:           cache.NewSourceStore(),
	}
	for _, opt := range opts {
		opt(acquirer)
	}
	return acquirer
}

func WithHTTPClient(client *http.Client) Option {
	return func(acquirer *Acquirer) {
		acquirer.client = client
	}
}

func WithFetcher(fetcher Fetcher) Option {
	return func(acquirer *Acquirer) {
		acquirer.fetcher = fetcher
	}
}

func WithResolver(resolver Resolver) Option {
	return func(acquirer *Acquirer) {
		if resolver != nil {
			acquirer.resolver = resolver
		}
	}
}

func WithInstalledVersionDetector(detector InstalledVersionDetector) Option {
	return func(acquirer *Acquirer) {
		if detector != nil {
			acquirer.versionDetector = detector
		}
	}
}

func EnsureCached(spec string, opts Options) (Outcome, error) {
	acquirer := NewAcquirer(
		WithHTTPClient(opts.Client),
		WithFetcher(opts.Fetcher),
	)
	return acquirer.Ensure(context.Background(), Request{Spec: spec, CWD: opts.CWD})
}

func (a *Acquirer) Ensure(ctx context.Context, req Request) (Outcome, error) {
	switch registry.DetectInputType(req.Spec) {
	case registry.RepoInput:
		return a.ensureRepoCached(ctx, req.Spec)
	default:
		return a.ensurePackageCached(ctx, req.Spec, req.CWD)
	}
}

func (a *Acquirer) ensurePackageCached(ctx context.Context, input, cwd string) (Outcome, error) {
	spec := registry.ParsePackageSpec(input)
	if spec.Name == "" {
		return Outcome{}, fmt.Errorf("package name must not be empty")
	}
	if spec.Registry == registry.NPM && spec.Version == "" {
		spec.Version = a.versionDetector.InstalledVersion(spec.Registry, spec.Name, cwd)
	}
	if spec.Version != "" {
		key := cache.PackageKey{Name: spec.Name, Registry: string(spec.Registry), Version: spec.Version}
		if entry, ok, err := a.store.GetPackage(key); err != nil {
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

	resolved, err := a.resolver.ResolvePackage(ctx, spec, a.client)
	if err != nil {
		return Outcome{}, err
	}
	key := cache.PackageKey{Name: resolved.Name, Registry: string(resolved.Registry), Version: resolved.Version}
	if entry, ok, err := a.store.GetPackage(key); err != nil {
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

	fetcher := a.fetcherOrDefault()
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
	entry, err := a.store.RecordPackage(key, cache.FetchedPackage{
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

func (a *Acquirer) ensureRepoCached(ctx context.Context, input string) (Outcome, error) {
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
		resolved, err = a.resolver.ResolveRepo(ctx, spec, a.client)
		if err != nil {
			return Outcome{}, err
		}
	}
	key := cache.RepoKey{DisplayName: resolved.DisplayName, Version: resolved.GitRef}
	if entry, ok, err := a.store.GetRepo(key); err != nil {
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

	fetcher := a.fetcherOrDefault()
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
	entry, err := a.store.RecordRepo(key, cache.FetchedRepo{
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

func (a *Acquirer) fetcherOrDefault() Fetcher {
	if a.fetcher != nil {
		return a.fetcher
	}
	return GitFetcher{Client: a.client}
}

type defaultResolver struct{}

func (defaultResolver) ResolvePackage(ctx context.Context, spec registry.PackageSpec, client *http.Client) (registry.ResolvedPackage, error) {
	return defaultResolvePackage(spec, client)
}

func (defaultResolver) ResolveRepo(ctx context.Context, spec repo.Spec, client *http.Client) (repo.Resolved, error) {
	return repo.Resolve(spec, client)
}

type lockfileVersionDetector struct{}

func (lockfileVersionDetector) InstalledVersion(reg registry.Registry, name, cwd string) string {
	if reg != registry.NPM {
		return ""
	}
	return lockfile.DetectInstalledVersion(name, cwd)
}

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
