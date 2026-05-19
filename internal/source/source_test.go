package source

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"repobridge/internal/cache"
	"repobridge/internal/git"
	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

type fakeFetcher struct {
	packageResult FetchResult
	repoResult    FetchResult
	packageCalls  int
	repoCalls     int
}

func (f *fakeFetcher) FetchPackage(pkg registry.ResolvedPackage) FetchResult {
	f.packageCalls++
	if f.packageResult.Success || f.packageResult.Error != nil {
		return f.packageResult
	}
	return FetchResult{Error: errors.New("unexpected package fetch")}
}

func (f *fakeFetcher) FetchRepo(displayName, repoURL, gitRef string) FetchResult {
	f.repoCalls++
	if f.repoResult.Success || f.repoResult.Error != nil {
		return f.repoResult
	}
	return FetchResult{Error: errors.New("unexpected repo fetch")}
}

func TestEnsureCachedReturnsExistingPackageCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/github.com/colinhacks/zod/3.22.4"
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(relativePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, filepath.FromSlash(relativePath), "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cache.WriteSources([]cache.PackageEntry{{
		Name:      "zod",
		Version:   "3.22.4",
		Registry:  string(registry.NPM),
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{}

	got, err := EnsureCached("zod@3.22.4", Options{CWD: ".", Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.packageCalls != 0 || fetcher.repoCalls != 0 {
		t.Fatalf("fetcher calls = package:%d repo:%d, want none", fetcher.packageCalls, fetcher.repoCalls)
	}
	wantPath := filepath.Join(home, filepath.FromSlash(relativePath))
	if got.Path != wantPath {
		t.Fatalf("Path = %q, want %q", got.Path, wantPath)
	}
	if !got.FromCache {
		t.Fatal("FromCache = false, want true")
	}
	if got.Name != "zod" || got.Version != "3.22.4" {
		t.Fatalf("name/version = %q/%q, want zod/3.22.4", got.Name, got.Version)
	}
}

func TestEnsureCachedFetchesRepoAndWritesSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/github.com/owner/repo/main"
	fetcher := &fakeFetcher{
		repoResult: FetchResult{
			Package: "github.com/owner/repo",
			Version: "main",
			Path:    relativePath,
			Success: true,
		},
	}

	got, err := EnsureCached("owner/repo@main", Options{Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.repoCalls != 1 {
		t.Fatalf("repo fetch calls = %d, want 1", fetcher.repoCalls)
	}
	if got.FromCache {
		t.Fatal("FromCache = true, want false")
	}
	wantPath := filepath.Join(home, filepath.FromSlash(relativePath))
	if got.Path != wantPath {
		t.Fatalf("Path = %q, want %q", got.Path, wantPath)
	}

	index, err := cache.ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Repos) != 1 {
		t.Fatalf("repos = %#v, want one entry", index.Repos)
	}
	if index.Repos[0].Name != "github.com/owner/repo" || index.Repos[0].Version != "main" || index.Repos[0].Path != relativePath {
		t.Fatalf("repo entry = %#v", index.Repos[0])
	}
}

func TestEnsureCachedInvalidRepoSpecReturnsTypedError(t *testing.T) {
	t.Setenv("REPOBRIDGE_HOME", t.TempDir())

	_, err := EnsureCached("https://example.com/owner/repo", Options{Fetcher: &fakeFetcher{}})
	if err == nil {
		t.Fatal("EnsureCached() error = nil, want InvalidRepoSpecError")
	}
	var invalidErr repobridge.InvalidRepoSpecError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("EnsureCached() error = %T %q, want InvalidRepoSpecError", err, err)
	}
}

func TestFetchPackageWithGitReusesExistingTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/1.2.3")
	if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "packages/pkg"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry:      registry.NPM,
		Name:          "pkg",
		Version:       "1.2.3",
		RepoURL:       "https://github.com/owner/repo",
		RepoDirectory: "packages/pkg",
	})
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !got.Success {
		t.Fatal("Success = false, want true")
	}
	if got.Path != "repos/github.com/owner/repo/1.2.3/packages/pkg" {
		t.Fatalf("Path = %q", got.Path)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git still exists or unexpected error: %v", err)
	}
}

func TestFetchRepoWithGitReusesExistingTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/main")
	if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := FetchRepoWithGit("github.com/owner/repo", "https://github.com/owner/repo", "main")
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !got.Success {
		t.Fatal("Success = false, want true")
	}
	if got.Path != "repos/github.com/owner/repo/main" {
		t.Fatalf("Path = %q", got.Path)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git still exists or unexpected error: %v", err)
	}
}

func TestFetchPackageWithGitClonesAfterClearingEmptyTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/1.2.3")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	cloneCalled := false
	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, gotTarget, version string) git.CloneResult {
		cloneCalled = true
		if gotTarget != target {
			t.Fatalf("target = %q, want %q", gotTarget, target)
		}
		if _, err := os.Stat(gotTarget); !os.IsNotExist(err) {
			t.Fatalf("target exists before clone or unexpected error: %v", err)
		}
		if err := os.MkdirAll(gotTarget, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry: registry.NPM,
		Name:     "pkg",
		Version:  "1.2.3",
		RepoURL:  "https://github.com/owner/repo",
	})
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !cloneCalled {
		t.Fatal("cloneAtTag was not called")
	}
	if !got.Success {
		t.Fatal("Success = false, want true")
	}
}

func TestFetchPackageWithGitClonesWhenTargetOnlyContainsGitDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/1.2.3")
	if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cloneCalled := false
	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, gotTarget, version string) git.CloneResult {
		cloneCalled = true
		if gotTarget != target {
			t.Fatalf("target = %q, want %q", gotTarget, target)
		}
		if _, err := os.Stat(gotTarget); !os.IsNotExist(err) {
			t.Fatalf("target exists before clone or unexpected error: %v", err)
		}
		if err := os.MkdirAll(gotTarget, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry: registry.NPM,
		Name:     "pkg",
		Version:  "1.2.3",
		RepoURL:  "https://github.com/owner/repo",
	})
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !cloneCalled {
		t.Fatal("cloneAtTag was not called")
	}
	if !got.Success {
		t.Fatal("Success = false, want true")
	}
}

func TestFetchPackageWithGitAllowsNestedRepoDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, target, version string) git.CloneResult {
		if err := os.MkdirAll(filepath.Join(target, "packages/pkg"), 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry:      registry.NPM,
		Name:          "pkg",
		Version:       "1.2.3",
		RepoURL:       "https://github.com/owner/repo",
		RepoDirectory: "packages/pkg",
	})
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if got.Path != "repos/github.com/owner/repo/1.2.3/packages/pkg" {
		t.Fatalf("Path = %q", got.Path)
	}
}

func TestFetchPackageWithGitRejectsMissingRepoDirectoryOnReuse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/1.2.3")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry:      registry.NPM,
		Name:          "pkg",
		Version:       "1.2.3",
		RepoURL:       "https://github.com/owner/repo",
		RepoDirectory: "packages/pkg",
	})
	if got.Error == nil {
		t.Fatal("Error = nil, want missing RepoDirectory error")
	}
}

func TestFetchPackageWithGitRejectsMissingRepoDirectoryAfterClone(t *testing.T) {
	t.Setenv("REPOBRIDGE_HOME", t.TempDir())
	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, target, version string) git.CloneResult {
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry:      registry.NPM,
		Name:          "pkg",
		Version:       "1.2.3",
		RepoURL:       "https://github.com/owner/repo",
		RepoDirectory: "packages/pkg",
	})
	if got.Error == nil {
		t.Fatal("Error = nil, want missing RepoDirectory error")
	}
}

func TestFetchPackageWithGitRejectsParentRepoDirectory(t *testing.T) {
	t.Setenv("REPOBRIDGE_HOME", t.TempDir())
	cloneCalled := false
	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, target, version string) git.CloneResult {
		cloneCalled = true
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry:      registry.NPM,
		Name:          "pkg",
		Version:       "1.2.3",
		RepoURL:       "https://github.com/owner/repo",
		RepoDirectory: "../../other",
	})
	if got.Error == nil {
		t.Fatal("Error = nil, want error")
	}
	if cloneCalled {
		t.Fatal("cloneAtTag was called for invalid RepoDirectory")
	}
}

func TestFetchPackageWithGitRejectsAbsoluteRepoDirectory(t *testing.T) {
	t.Setenv("REPOBRIDGE_HOME", t.TempDir())
	cloneCalled := false
	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, target, version string) git.CloneResult {
		cloneCalled = true
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry:      registry.NPM,
		Name:          "pkg",
		Version:       "1.2.3",
		RepoURL:       "https://github.com/owner/repo",
		RepoDirectory: "/tmp/other",
	})
	if got.Error == nil {
		t.Fatal("Error = nil, want error")
	}
	if cloneCalled {
		t.Fatal("cloneAtTag was called for invalid RepoDirectory")
	}
}

func TestFetchRepoWithGitClonesAfterClearingEmptyTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/main")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	cloneCalled := false
	oldCloneAtRef := cloneAtRef
	cloneAtRef = func(repoURL, gotTarget, ref string) git.CloneResult {
		cloneCalled = true
		if gotTarget != target {
			t.Fatalf("target = %q, want %q", gotTarget, target)
		}
		if _, err := os.Stat(gotTarget); !os.IsNotExist(err) {
			t.Fatalf("target exists before clone or unexpected error: %v", err)
		}
		if err := os.MkdirAll(gotTarget, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtRef = oldCloneAtRef })

	got := FetchRepoWithGit("github.com/owner/repo", "https://github.com/owner/repo", "main")
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !cloneCalled {
		t.Fatal("cloneAtRef was not called")
	}
	if !got.Success {
		t.Fatal("Success = false, want true")
	}
}

func TestFetchRepoWithGitClonesWhenTargetOnlyContainsGitDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/main")
	if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cloneCalled := false
	oldCloneAtRef := cloneAtRef
	cloneAtRef = func(repoURL, gotTarget, ref string) git.CloneResult {
		cloneCalled = true
		if gotTarget != target {
			t.Fatalf("target = %q, want %q", gotTarget, target)
		}
		if _, err := os.Stat(gotTarget); !os.IsNotExist(err) {
			t.Fatalf("target exists before clone or unexpected error: %v", err)
		}
		if err := os.MkdirAll(gotTarget, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtRef = oldCloneAtRef })

	got := FetchRepoWithGit("github.com/owner/repo", "https://github.com/owner/repo", "main")
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !cloneCalled {
		t.Fatal("cloneAtRef was not called")
	}
	if !got.Success {
		t.Fatal("Success = false, want true")
	}
}

func TestEnsureCachedIgnoresStalePackageCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	if err := cache.WriteSources([]cache.PackageEntry{{
		Name:      "pkg",
		Version:   "1.2.3",
		Registry:  string(registry.NPM),
		Path:      "repos/github.com/owner/repo/1.2.3",
		FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	oldResolvePackage := resolvePackage
	resolvePackage = func(spec registry.PackageSpec, client *http.Client) (registry.ResolvedPackage, error) {
		return registry.ResolvedPackage{
			Registry: registry.NPM,
			Name:     spec.Name,
			Version:  spec.Version,
			RepoURL:  "https://github.com/owner/repo",
		}, nil
	}
	t.Cleanup(func() { resolvePackage = oldResolvePackage })
	fetcher := &fakeFetcher{
		packageResult: FetchResult{
			Package:  "pkg",
			Version:  "1.2.3",
			Registry: registry.NPM,
			Path:     "repos/github.com/owner/repo/1.2.3",
			Success:  true,
		},
	}

	got, err := EnsureCached("pkg@1.2.3", Options{Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if got.FromCache {
		t.Fatal("FromCache = true, want false")
	}
	if fetcher.packageCalls != 1 {
		t.Fatalf("package fetch calls = %d, want 1", fetcher.packageCalls)
	}
}

func TestEnsureCachedIgnoresEmptyPackageCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/github.com/owner/repo/1.2.3"
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(relativePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cache.WriteSources([]cache.PackageEntry{{
		Name:      "pkg",
		Version:   "1.2.3",
		Registry:  string(registry.NPM),
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	oldResolvePackage := resolvePackage
	resolvePackage = func(spec registry.PackageSpec, client *http.Client) (registry.ResolvedPackage, error) {
		return registry.ResolvedPackage{
			Registry: registry.NPM,
			Name:     spec.Name,
			Version:  spec.Version,
			RepoURL:  "https://github.com/owner/repo",
		}, nil
	}
	t.Cleanup(func() { resolvePackage = oldResolvePackage })
	fetcher := &fakeFetcher{
		packageResult: FetchResult{
			Package:  "pkg",
			Version:  "1.2.3",
			Registry: registry.NPM,
			Path:     relativePath,
			Success:  true,
		},
	}

	got, err := EnsureCached("pkg@1.2.3", Options{Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if got.FromCache {
		t.Fatal("FromCache = true, want false")
	}
	if fetcher.packageCalls != 1 {
		t.Fatalf("package fetch calls = %d, want 1", fetcher.packageCalls)
	}
}

func TestEnsureCachedIgnoresGitOnlyPackageCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/github.com/owner/repo/1.2.3"
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(relativePath), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cache.WriteSources([]cache.PackageEntry{{
		Name:      "pkg",
		Version:   "1.2.3",
		Registry:  string(registry.NPM),
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	oldResolvePackage := resolvePackage
	resolvePackage = func(spec registry.PackageSpec, client *http.Client) (registry.ResolvedPackage, error) {
		return registry.ResolvedPackage{
			Registry: registry.NPM,
			Name:     spec.Name,
			Version:  spec.Version,
			RepoURL:  "https://github.com/owner/repo",
		}, nil
	}
	t.Cleanup(func() { resolvePackage = oldResolvePackage })
	fetcher := &fakeFetcher{
		packageResult: FetchResult{
			Package:  "pkg",
			Version:  "1.2.3",
			Registry: registry.NPM,
			Path:     relativePath,
			Success:  true,
		},
	}

	got, err := EnsureCached("pkg@1.2.3", Options{Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if got.FromCache {
		t.Fatal("FromCache = true, want false")
	}
	if fetcher.packageCalls != 1 {
		t.Fatalf("package fetch calls = %d, want 1", fetcher.packageCalls)
	}
}

func TestEnsureCachedIgnoresStaleRepoCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	if err := cache.WriteSources(nil, []cache.RepoEntry{{
		Name:      "github.com/owner/repo",
		Version:   "main",
		Path:      "repos/github.com/owner/repo/main",
		FetchedAt: "2026-05-18T12:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{
		repoResult: FetchResult{
			Package: "github.com/owner/repo",
			Version: "main",
			Path:    "repos/github.com/owner/repo/main",
			Success: true,
		},
	}

	got, err := EnsureCached("owner/repo@main", Options{Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if got.FromCache {
		t.Fatal("FromCache = true, want false")
	}
	if fetcher.repoCalls != 1 {
		t.Fatalf("repo fetch calls = %d, want 1", fetcher.repoCalls)
	}
}

func TestEnsureCachedIgnoresEmptyRepoCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/github.com/owner/repo/main"
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(relativePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cache.WriteSources(nil, []cache.RepoEntry{{
		Name:      "github.com/owner/repo",
		Version:   "main",
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{
		repoResult: FetchResult{
			Package: "github.com/owner/repo",
			Version: "main",
			Path:    relativePath,
			Success: true,
		},
	}

	got, err := EnsureCached("owner/repo@main", Options{Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if got.FromCache {
		t.Fatal("FromCache = true, want false")
	}
	if fetcher.repoCalls != 1 {
		t.Fatalf("repo fetch calls = %d, want 1", fetcher.repoCalls)
	}
}

func TestEnsureCachedIgnoresGitOnlyRepoCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/github.com/owner/repo/main"
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(relativePath), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cache.WriteSources(nil, []cache.RepoEntry{{
		Name:      "github.com/owner/repo",
		Version:   "main",
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{
		repoResult: FetchResult{
			Package: "github.com/owner/repo",
			Version: "main",
			Path:    relativePath,
			Success: true,
		},
	}

	got, err := EnsureCached("owner/repo@main", Options{Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if got.FromCache {
		t.Fatal("FromCache = true, want false")
	}
	if fetcher.repoCalls != 1 {
		t.Fatalf("repo fetch calls = %d, want 1", fetcher.repoCalls)
	}
}
