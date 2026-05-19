package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSourceCacheEnsurePackageFetchesRecordsThenHitsCache(t *testing.T) {
	home := withHome(t)
	clock := func() time.Time {
		return time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	}
	cache := NewSourceCache(WithHome(home), WithClock(clock))
	key := PackageKey{Name: "zod", Registry: "npm", Version: "3.22.4"}
	relativePath := "repos/github.com/colinhacks/zod/3.22.4"
	absolutePath := filepath.Join(home, filepath.FromSlash(relativePath))
	fetches := 0

	first, fromCache, err := cache.EnsurePackage(key, func() (FetchedPackage, error) {
		fetches++
		if err := os.MkdirAll(absolutePath, 0o755); err != nil {
			return FetchedPackage{}, err
		}
		if err := os.WriteFile(filepath.Join(absolutePath, "package.json"), []byte("{}"), 0o644); err != nil {
			return FetchedPackage{}, err
		}
		return FetchedPackage{Path: absolutePath}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if fromCache {
		t.Fatal("first EnsurePackage() fromCache = true, want false")
	}
	if first.Path != absolutePath {
		t.Fatalf("first path = %q, want %q", first.Path, absolutePath)
	}

	second, fromCache, err := cache.EnsurePackage(key, func() (FetchedPackage, error) {
		fetches++
		return FetchedPackage{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fromCache {
		t.Fatal("second EnsurePackage() fromCache = false, want true")
	}
	if second.Path != absolutePath {
		t.Fatalf("second path = %q, want %q", second.Path, absolutePath)
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want 1", fetches)
	}
}

func TestSourceCacheCleanUsesRemoveReferenceRules(t *testing.T) {
	home := withHome(t)
	sharedPath := "repos/github.com/owner/repo/1.0.0"
	if err := WriteSources(
		[]PackageEntry{{
			Name:      "pkg",
			Version:   "1.0.0",
			Registry:  "npm",
			Path:      sharedPath,
			FetchedAt: "2026-05-18T12:00:00Z",
		}},
		[]RepoEntry{{
			Name:      "github.com/owner/repo",
			Version:   "1.0.0",
			Path:      sharedPath,
			FetchedAt: "2026-05-18T12:00:00Z",
		}},
	); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, filepath.FromSlash(sharedPath))
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("# repo"), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := NewSourceCache(WithHome(home))
	result, err := cache.Clean(CleanOptions{
		Kinds:      map[SourceKind]bool{PackageSource: true},
		Registries: map[string]bool{"npm": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || result.Deleted != 0 || result.Kept != 1 {
		t.Fatalf("Clean() = %#v, want removed=1 deleted=0 kept=1", result)
	}
	index, err := ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 0 {
		t.Fatalf("packages = %#v, want empty", index.Packages)
	}
	if len(index.Repos) != 1 {
		t.Fatalf("repos = %#v, want repo retained", index.Repos)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("shared target removed or unreadable: %v", err)
	}
}

func TestSourceCacheRemovePackageRollsBackIndexOnDeleteFailure(t *testing.T) {
	home := withHome(t)
	relativePath := "repos/github.com/owner/repo/1.0.0"
	if err := WriteSources([]PackageEntry{{
		Name:      "pkg",
		Version:   "1.0.0",
		Registry:  "npm",
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := NewSourceCache(
		WithHome(home),
		WithRemoveAll(func(relativePath string) error {
			return errForTest("delete failed")
		}),
	)
	result, err := cache.Remove(RemoveSelector{
		Kind: PackageSource,
		Package: PackageSelector{
			Name:     "pkg",
			Registry: "npm",
		},
	})
	if err == nil {
		t.Fatal("Remove() error = nil, want delete error")
	}
	if !result.RolledBack {
		t.Fatalf("Remove() RolledBack = false, result = %#v", result)
	}
	index, readErr := ReadSources()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(index.Packages) != 1 {
		t.Fatalf("packages = %#v, want restored package", index.Packages)
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Fatalf("target missing after failed delete: %v", statErr)
	}
}

type errForTest string

func (e errForTest) Error() string { return string(e) }

func TestSourceCacheListReturnsAbsolutePaths(t *testing.T) {
	home := withHome(t)
	packagePath := "repos/github.com/owner/pkg/1.0.0"
	repoPath := "repos/github.com/owner/repo/main"
	if err := WriteSources(
		[]PackageEntry{{
			Name:      "pkg",
			Version:   "1.0.0",
			Registry:  "npm",
			Path:      packagePath,
			FetchedAt: "2026-05-18T12:00:00Z",
		}},
		[]RepoEntry{{
			Name:      "github.com/owner/repo",
			Version:   "main",
			Path:      repoPath,
			FetchedAt: "2026-05-18T12:00:00Z",
		}},
	); err != nil {
		t.Fatal(err)
	}

	cache := NewSourceCache(WithHome(home))
	result, err := cache.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("sources = %#v, want two sources", result.Sources)
	}
	if result.Sources[0].Path != filepath.Join(home, filepath.FromSlash(packagePath)) {
		t.Fatalf("package path = %q", result.Sources[0].Path)
	}
	if result.Sources[1].Path != filepath.Join(home, filepath.FromSlash(repoPath)) {
		t.Fatalf("repo path = %q", result.Sources[1].Path)
	}
}
