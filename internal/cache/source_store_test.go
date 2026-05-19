package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSourceStoreGetPackageReturnsUsableHit(t *testing.T) {
	home := withHome(t)
	relativePath := "repos/github.com/colinhacks/zod/3.22.4"
	if err := WriteSources([]PackageEntry{{
		Name:      "zod",
		Version:   "3.22.4",
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

	store := NewSourceStore(WithHome(home))
	got, ok, err := store.GetPackage(PackageKey{Name: "zod", Registry: "npm", Version: "3.22.4"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("GetPackage() ok = false, want true")
	}
	if got.Path != target {
		t.Fatalf("Path = %q, want %q", got.Path, target)
	}
	if got.RelativePath != relativePath {
		t.Fatalf("RelativePath = %q, want %q", got.RelativePath, relativePath)
	}
}

func TestSourceStoreGetPackageIgnoresGitOnlyHit(t *testing.T) {
	home := withHome(t)
	relativePath := "repos/github.com/colinhacks/zod/3.22.4"
	if err := WriteSources([]PackageEntry{{
		Name:      "zod",
		Version:   "3.22.4",
		Registry:  "npm",
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	store := NewSourceStore(WithHome(home))
	if got, ok, err := store.GetPackage(PackageKey{Name: "zod", Registry: "npm", Version: "3.22.4"}); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("GetPackage() = %#v, true; want nil, false", got)
	}
}

func TestSourceStoreRecordPackageNormalizesAbsolutePathAndReplacesEntry(t *testing.T) {
	home := withHome(t)
	oldPath := "repos/github.com/colinhacks/zod/3.22.3"
	if err := WriteSources([]PackageEntry{{
		Name:      "zod",
		Version:   "3.22.4",
		Registry:  "npm",
		Path:      oldPath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time {
		return time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	}
	store := NewSourceStore(WithHome(home), WithClock(clock))
	relativePath := "repos/github.com/colinhacks/zod/3.22.4"
	absolutePath := filepath.Join(home, filepath.FromSlash(relativePath))

	recorded, err := store.RecordPackage(
		PackageKey{Name: "zod", Registry: "npm", Version: "3.22.4"},
		FetchedPackage{Path: absolutePath},
	)
	if err != nil {
		t.Fatal(err)
	}
	if recorded.Path != absolutePath {
		t.Fatalf("Path = %q, want %q", recorded.Path, absolutePath)
	}
	if recorded.RelativePath != relativePath {
		t.Fatalf("RelativePath = %q, want %q", recorded.RelativePath, relativePath)
	}

	index, err := ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 1 {
		t.Fatalf("packages = %#v, want one package", index.Packages)
	}
	got := index.Packages[0]
	if got.Path != relativePath || got.FetchedAt != "2026-05-19T12:00:00Z" {
		t.Fatalf("package entry = %#v", got)
	}
}

func TestSourceStoreRecordPackageRejectsPathOutsideHome(t *testing.T) {
	home := withHome(t)
	store := NewSourceStore(WithHome(home))

	_, err := store.RecordPackage(
		PackageKey{Name: "zod", Registry: "npm", Version: "3.22.4"},
		FetchedPackage{Path: filepath.Join(t.TempDir(), "zod")},
	)
	if err == nil {
		t.Fatal("RecordPackage() error = nil, want error")
	}
}

func TestSourceStoreGetRepoReturnsUsableHit(t *testing.T) {
	home := withHome(t)
	relativePath := "repos/github.com/owner/repo/main"
	if err := WriteSources(nil, []RepoEntry{{
		Name:      "github.com/owner/repo",
		Version:   "main",
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("# repo"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewSourceStore(WithHome(home))
	got, ok, err := store.GetRepo(RepoKey{DisplayName: "github.com/owner/repo", Version: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("GetRepo() ok = false, want true")
	}
	if got.Path != target {
		t.Fatalf("Path = %q, want %q", got.Path, target)
	}
}

func TestSourceStoreRecordRepoNormalizesRelativePath(t *testing.T) {
	home := withHome(t)
	clock := func() time.Time {
		return time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	}
	store := NewSourceStore(WithHome(home), WithClock(clock))
	relativePath := "repos/github.com/owner/repo/main"

	recorded, err := store.RecordRepo(
		RepoKey{DisplayName: "github.com/owner/repo", Version: "main"},
		FetchedRepo{Path: relativePath},
	)
	if err != nil {
		t.Fatal(err)
	}
	if recorded.Path != filepath.Join(home, filepath.FromSlash(relativePath)) {
		t.Fatalf("Path = %q", recorded.Path)
	}

	index, err := ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Repos) != 1 {
		t.Fatalf("repos = %#v, want one repo", index.Repos)
	}
	got := index.Repos[0]
	if got.Path != relativePath || got.FetchedAt != "2026-05-19T12:00:00Z" {
		t.Fatalf("repo entry = %#v", got)
	}
}
