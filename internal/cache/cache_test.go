package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func withHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", dir)
	return dir
}

func TestHomeUsesEnv(t *testing.T) {
	dir := withHome(t)
	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("Home() = %q, want %q", got, dir)
	}
}

func TestWriteAndReadSources(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{{
		Name: "zod", Version: "3.22.4", Registry: "npm",
		Path: "repos/github.com/colinhacks/zod/3.22.4", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	repos := []RepoEntry{{
		Name: "github.com/owner/repo", Version: "main",
		Path: "repos/github.com/owner/repo/main", FetchedAt: "2026-05-18T12:00:00Z",
	}}

	if err := WriteSources(packages, repos); err != nil {
		t.Fatal(err)
	}
	index, err := ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 1 || index.Packages[0].Name != "zod" {
		t.Fatalf("packages = %#v", index.Packages)
	}
	if len(index.Repos) != 1 || index.Repos[0].Name != "github.com/owner/repo" {
		t.Fatalf("repos = %#v", index.Repos)
	}
	if _, err := os.Stat(filepath.Join(dir, "sources.json")); err != nil {
		t.Fatal(err)
	}
}

func TestReadCorruptSourcesCreatesBackup(t *testing.T) {
	dir := withHome(t)
	if err := os.WriteFile(filepath.Join(dir, "sources.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 0 || len(index.Repos) != 0 {
		t.Fatalf("index = %#v, want empty", index)
	}
	if _, err := os.Stat(filepath.Join(dir, "sources.json.bak")); err != nil {
		t.Fatal(err)
	}
}

func TestAbsolutePath(t *testing.T) {
	dir := withHome(t)
	got, err := AbsolutePath("repos/github.com/a/b/main")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "repos/github.com/a/b/main")
	if got != want {
		t.Fatalf("AbsolutePath() = %q, want %q", got, want)
	}
}

func TestAbsolutePathRejectsTraversal(t *testing.T) {
	withHome(t)
	for _, path := range []string{"../victim", "/tmp/victim", "repos/a/../b"} {
		if _, err := AbsolutePath(path); err == nil {
			t.Fatalf("AbsolutePath(%q) error = nil, want error", path)
		}
	}
}

func TestRepoPathRejectsTraversalSegments(t *testing.T) {
	withHome(t)
	if _, err := RepoPath("github.com/o/../r", "main"); err == nil {
		t.Fatal(`RepoPath("github.com/o/../r", "main") error = nil, want error`)
	}
	if _, err := RepoPath("github.com/o/r", "v1/../main"); err == nil {
		t.Fatal(`RepoPath("github.com/o/r", "v1/../main") error = nil, want error`)
	}
}

func TestWriteSourcesRemovesEmptyIndex(t *testing.T) {
	dir := withHome(t)
	if err := os.WriteFile(filepath.Join(dir, "sources.json"), []byte(`{"packages":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteSources(nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sources.json")); !os.IsNotExist(err) {
		t.Fatalf("sources.json exists or unexpected error: %v", err)
	}
}

func TestRemovePackageSourceRemovesIndexEntry(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{{
		Name: "zod", Version: "3.22.4", Registry: "npm",
		Path: "repos/github.com/colinhacks/zod/3.22.4", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(dir, "repos/github.com/colinhacks/zod")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, repoRemoved, err := RemovePackageSource("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if !removed || !repoRemoved {
		t.Fatalf("RemovePackageSource() = %v, %v, want true, true", removed, repoRemoved)
	}
	info, err := PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Fatalf("PackageInfo() = %#v, want nil", info)
	}
	if _, err := os.Stat(repoDir); !os.IsNotExist(err) {
		t.Fatalf("repo dir exists or unexpected error: %v", err)
	}
}

func TestRemovePackageSourceVersionRemovesOnlyMatchingVersion(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{
		{
			Name: "zod", Version: "3.22.4", Registry: "npm",
			Path: "repos/github.com/colinhacks/zod/3.22.4", FetchedAt: "2026-05-18T12:00:00Z",
		},
		{
			Name: "zod", Version: "4.0.0", Registry: "npm",
			Path: "repos/github.com/colinhacks/zod/4.0.0", FetchedAt: "2026-05-18T12:00:00Z",
		},
	}
	if err := WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}
	removedDir := filepath.Join(dir, "repos/github.com/colinhacks/zod/3.22.4")
	keptDir := filepath.Join(dir, "repos/github.com/colinhacks/zod/4.0.0")
	if err := os.MkdirAll(removedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(removedDir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(keptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keptDir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, repoRemoved, err := RemovePackageSourceVersion("zod", "npm", "3.22.4")
	if err != nil {
		t.Fatal(err)
	}
	if !removed || !repoRemoved {
		t.Fatalf("RemovePackageSourceVersion() = %v, %v, want true, true", removed, repoRemoved)
	}
	index, err := ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 1 || index.Packages[0].Version != "4.0.0" {
		t.Fatalf("packages = %#v, want only version 4.0.0", index.Packages)
	}
	if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
		t.Fatalf("removed version dir exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(keptDir); err != nil {
		t.Fatal(err)
	}
}

func TestRemovePackageSourceVersionKeepsRootWhenRemainingPackageIsNested(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{
		{
			Name: "pkg-a", Version: "1.2.3", Registry: "npm",
			Path: "repos/github.com/o/r/1.2.3", FetchedAt: "2026-05-18T12:00:00Z",
		},
		{
			Name: "pkg-b", Version: "1.2.3", Registry: "npm",
			Path: "repos/github.com/o/r/1.2.3/packages/pkg-b", FetchedAt: "2026-05-18T12:00:00Z",
		},
	}
	if err := WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}
	versionRoot := filepath.Join(dir, "repos/github.com/o/r/1.2.3")
	nestedPackageDir := filepath.Join(versionRoot, "packages/pkg-b")
	if err := os.MkdirAll(nestedPackageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionRoot, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedPackageDir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, repoRemoved, err := RemovePackageSourceVersion("pkg-a", "npm", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if !removed || repoRemoved {
		t.Fatalf("RemovePackageSourceVersion() = %v, %v, want true, false", removed, repoRemoved)
	}
	index, err := ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 1 || index.Packages[0].Name != "pkg-b" {
		t.Fatalf("packages = %#v, want only pkg-b", index.Packages)
	}
	if _, err := os.Stat(versionRoot); err != nil {
		t.Fatalf("version root missing: %v", err)
	}
	if _, err := os.Stat(nestedPackageDir); err != nil {
		t.Fatalf("nested package dir missing: %v", err)
	}
}

func TestRemovePackageSourceKeepsSharedRepoButRemovesIndexEntry(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{
		{
			Name: "pkg-a", Version: "1.0.0", Registry: "npm",
			Path: "repos/github.com/o/r/1.0.0/pkg-a", FetchedAt: "2026-05-18T12:00:00Z",
		},
		{
			Name: "pkg-b", Version: "1.0.0", Registry: "npm",
			Path: "repos/github.com/o/r/1.0.0/pkg-b", FetchedAt: "2026-05-18T12:00:00Z",
		},
	}
	if err := WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}
	baseDir := filepath.Join(dir, "repos/github.com/o/r")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, repoRemoved, err := RemovePackageSource("pkg-a", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if !removed || repoRemoved {
		t.Fatalf("RemovePackageSource() = %v, %v, want true, false", removed, repoRemoved)
	}
	removedInfo, err := PackageInfo("pkg-a", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if removedInfo != nil {
		t.Fatalf("PackageInfo(pkg-a) = %#v, want nil", removedInfo)
	}
	otherInfo, err := PackageInfo("pkg-b", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if otherInfo == nil {
		t.Fatal("PackageInfo(pkg-b) = nil, want entry")
	}
	if _, err := os.Stat(baseDir); err != nil {
		t.Fatal(err)
	}
}

func TestRemovePackageSourceKeepsRepoEntrySharedPath(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{{
		Name: "pkg", Version: "1.0.0", Registry: "npm",
		Path: "repos/github.com/o/r/1.0.0/pkg", FetchedAt: "2026-05-18T12:00:00Z",
	}, {
		Name: "other", Version: "1.0.0", Registry: "npm",
		Path: "repos/github.com/other/repo/1.0.0/other", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	repos := []RepoEntry{{
		Name: "github.com/o/r", Version: "1.0.0",
		Path: "repos/github.com/o/r/1.0.0", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(packages, repos); err != nil {
		t.Fatal(err)
	}
	baseDir := filepath.Join(dir, "repos/github.com/o/r")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, repoRemoved, err := RemovePackageSource("pkg", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if !removed || repoRemoved {
		t.Fatalf("RemovePackageSource() = %v, %v, want true, false", removed, repoRemoved)
	}
	pkgInfo, err := PackageInfo("pkg", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if pkgInfo != nil {
		t.Fatalf("PackageInfo() = %#v, want nil", pkgInfo)
	}
	repoInfo, err := RepoInfo("github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	if repoInfo == nil {
		t.Fatal("RepoInfo() = nil, want entry")
	}
	if _, err := os.Stat(baseDir); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRepoSourceRemovesIndexEntry(t *testing.T) {
	dir := withHome(t)
	repos := []RepoEntry{{
		Name: "github.com/o/r", Version: "main",
		Path: "repos/github.com/o/r/main", FetchedAt: "2026-05-18T12:00:00Z",
	}, {
		Name: "github.com/other/repo", Version: "main",
		Path: "repos/github.com/other/repo/main", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(nil, repos); err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(dir, "repos/github.com/o/r")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, err := RemoveRepoSource("github.com/o/r", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("RemoveRepoSource() = false, want true")
	}
	info, err := RepoInfo("github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Fatalf("RepoInfo() = %#v, want nil", info)
	}
	if _, err := os.Stat(repoDir); !os.IsNotExist(err) {
		t.Fatalf("repo dir exists or unexpected error: %v", err)
	}
}

func TestRemoveRepoSourceKeepsPackageSharedPath(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{{
		Name: "pkg", Version: "main", Registry: "npm",
		Path: "repos/github.com/o/r/main/pkg", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	repos := []RepoEntry{{
		Name: "github.com/o/r", Version: "main",
		Path: "repos/github.com/o/r/main", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(packages, repos); err != nil {
		t.Fatal(err)
	}
	baseDir := filepath.Join(dir, "repos/github.com/o/r")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, err := RemoveRepoSource("github.com/o/r", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("RemoveRepoSource() = false, want true")
	}
	repoInfo, err := RepoInfo("github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	if repoInfo != nil {
		t.Fatalf("RepoInfo() = %#v, want nil", repoInfo)
	}
	pkgInfo, err := PackageInfo("pkg", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if pkgInfo == nil {
		t.Fatal("PackageInfo() = nil, want entry")
	}
	if _, err := os.Stat(baseDir); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRepoSourceDeletesVersionPathWhenPackageReferencesDifferentVersion(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{{
		Name: "pkg", Version: "v1", Registry: "npm",
		Path: "repos/github.com/o/r/v1/pkg", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	repos := []RepoEntry{{
		Name: "github.com/o/r", Version: "main",
		Path: "repos/github.com/o/r/main", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(packages, repos); err != nil {
		t.Fatal(err)
	}
	mainDir := filepath.Join(dir, "repos/github.com/o/r/main")
	packageDir := filepath.Join(dir, "repos/github.com/o/r/v1/pkg")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "README.md"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}

	version := "main"
	removed, err := RemoveRepoSource("github.com/o/r", &version)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("RemoveRepoSource() = false, want true")
	}
	if _, err := os.Stat(mainDir); !os.IsNotExist(err) {
		t.Fatalf("main repo dir exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(packageDir); err != nil {
		t.Fatal(err)
	}
	repoInfo, err := RepoInfo("github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	if repoInfo != nil {
		t.Fatalf("RepoInfo() = %#v, want nil", repoInfo)
	}
	pkgInfo, err := PackageInfo("pkg", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if pkgInfo == nil {
		t.Fatal("PackageInfo() = nil, want entry")
	}
}

func TestRemoveRepoSourceRejectsTraversal(t *testing.T) {
	dir := withHome(t)
	victim := filepath.Join(filepath.Dir(dir), "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := RemoveRepoSource("../victim", nil); err == nil {
		t.Fatal("RemoveRepoSource() error = nil, want error")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal(err)
	}
}

func TestRemovePackageSourceWriteFailurePreservesIndexAndFiles(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{{
		Name: "pkg", Version: "1.0.0", Registry: "npm",
		Path: "repos/github.com/o/r/1.0.0/pkg", FetchedAt: "2026-05-18T12:00:00Z",
	}, {
		Name: "other", Version: "1.0.0", Registry: "npm",
		Path: "repos/github.com/other/repo/1.0.0/other", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(dir, "repos/github.com/o/r")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".sources.json.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, err := RemovePackageSource("pkg", "npm"); err == nil {
		t.Fatal("RemovePackageSource() error = nil, want write error")
	}
	info, err := PackageInfo("pkg", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("PackageInfo() = nil, want entry preserved after write failure")
	}
	if _, err := os.Stat(targetDir); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRepoSourceWriteFailurePreservesIndexAndFiles(t *testing.T) {
	dir := withHome(t)
	repos := []RepoEntry{{
		Name: "github.com/o/r", Version: "main",
		Path: "repos/github.com/o/r/main", FetchedAt: "2026-05-18T12:00:00Z",
	}, {
		Name: "github.com/other/repo", Version: "main",
		Path: "repos/github.com/other/repo/main", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(nil, repos); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(dir, "repos/github.com/o/r/main")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "README.md"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".sources.json.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}

	version := "main"
	if _, err := RemoveRepoSource("github.com/o/r", &version); err == nil {
		t.Fatal("RemoveRepoSource() error = nil, want write error")
	}
	info, err := RepoInfo("github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("RepoInfo() = nil, want entry preserved after write failure")
	}
	if _, err := os.Stat(targetDir); err != nil {
		t.Fatal(err)
	}
}

func TestRemovePackageSourceDeleteFailureRestoresIndex(t *testing.T) {
	dir := withHome(t)
	packages := []PackageEntry{{
		Name: "pkg", Version: "1.0.0", Registry: "npm",
		Path: "repos/github.com/o/r/1.0.0/pkg", FetchedAt: "2026-05-18T12:00:00Z",
	}, {
		Name: "other", Version: "1.0.0", Registry: "npm",
		Path: "repos/github.com/other/repo/1.0.0/other", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(dir, "repos/github.com/o/r")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	removeErr := errors.New("delete failed")
	originalRemoveAllSafe := removeAllSafeFunc
	removeAllSafeFunc = func(relativePath string) error {
		if relativePath == "repos/github.com/o/r" {
			return removeErr
		}
		return originalRemoveAllSafe(relativePath)
	}
	t.Cleanup(func() { removeAllSafeFunc = originalRemoveAllSafe })

	if _, _, err := RemovePackageSource("pkg", "npm"); !errors.Is(err, removeErr) {
		t.Fatalf("RemovePackageSource() error = %v, want %v", err, removeErr)
	}
	info, err := PackageInfo("pkg", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("PackageInfo() = nil, want entry restored after delete failure")
	}
	if _, err := os.Stat(targetDir); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRepoSourceDeleteFailureRestoresIndex(t *testing.T) {
	dir := withHome(t)
	repos := []RepoEntry{{
		Name: "github.com/o/r", Version: "main",
		Path: "repos/github.com/o/r/main", FetchedAt: "2026-05-18T12:00:00Z",
	}, {
		Name: "github.com/other/repo", Version: "main",
		Path: "repos/github.com/other/repo/main", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(nil, repos); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(dir, "repos/github.com/o/r/main")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	removeErr := errors.New("delete failed")
	originalRemoveAllSafe := removeAllSafeFunc
	removeAllSafeFunc = func(relativePath string) error {
		if relativePath == "repos/github.com/o/r/main" {
			return removeErr
		}
		return originalRemoveAllSafe(relativePath)
	}
	t.Cleanup(func() { removeAllSafeFunc = originalRemoveAllSafe })

	version := "main"
	if _, err := RemoveRepoSource("github.com/o/r", &version); !errors.Is(err, removeErr) {
		t.Fatalf("RemoveRepoSource() error = %v, want %v", err, removeErr)
	}
	info, err := RepoInfo("github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("RepoInfo() = nil, want entry restored after delete failure")
	}
	if _, err := os.Stat(targetDir); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRepoSourcePartialDeleteFailureDoesNotRestoreDeletedPath(t *testing.T) {
	dir := withHome(t)
	repos := []RepoEntry{{
		Name: "github.com/o/r", Version: "v1",
		Path: "repos/github.com/o/r/v1", FetchedAt: "2026-05-18T12:00:00Z",
	}, {
		Name: "github.com/o/r", Version: "v2",
		Path: "repos/github.com/o/r/v2", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(nil, repos); err != nil {
		t.Fatal(err)
	}
	v1Dir := filepath.Join(dir, "repos/github.com/o/r/v1")
	v2Dir := filepath.Join(dir, "repos/github.com/o/r/v2")
	if err := os.MkdirAll(v1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v1Dir, "README.md"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(v2Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v2Dir, "README.md"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	removeErr := errors.New("delete failed")
	originalRemoveAllSafe := removeAllSafeFunc
	removeAllSafeFunc = func(relativePath string) error {
		if relativePath == "repos/github.com/o/r/v2" {
			return removeErr
		}
		return originalRemoveAllSafe(relativePath)
	}
	t.Cleanup(func() { removeAllSafeFunc = originalRemoveAllSafe })

	if _, err := RemoveRepoSource("github.com/o/r", nil); !errors.Is(err, removeErr) {
		t.Fatalf("RemoveRepoSource() error = %v, want %v", err, removeErr)
	}
	if _, err := os.Stat(v1Dir); !os.IsNotExist(err) {
		t.Fatalf("v1 dir exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(v2Dir); err != nil {
		t.Fatal(err)
	}
	index, err := ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range index.Repos {
		if repo.Path == "repos/github.com/o/r/v1" {
			t.Fatalf("index restored deleted path: %#v", index.Repos)
		}
	}
	foundV2 := false
	for _, repo := range index.Repos {
		if repo.Path == "repos/github.com/o/r/v2" {
			foundV2 = true
		}
	}
	if !foundV2 {
		t.Fatalf("index = %#v, want v2 entry retained for retry", index.Repos)
	}
}

func TestRemovePackageSourceUnsafeDeletionPreservesIndexEntry(t *testing.T) {
	dir := withHome(t)
	outside := filepath.Join(filepath.Dir(dir), "outside")
	marker := filepath.Join(outside, "marker")
	if err := os.MkdirAll(filepath.Join(outside, "github.com/o/r/1.0.0/pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "repos")); err != nil {
		t.Fatal(err)
	}
	packages := []PackageEntry{{
		Name: "pkg", Version: "1.0.0", Registry: "npm",
		Path: "repos/github.com/o/r/1.0.0/pkg", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}

	if _, _, err := RemovePackageSource("pkg", "npm"); err == nil {
		t.Fatal("RemovePackageSource() error = nil, want error")
	}
	info, err := PackageInfo("pkg", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("PackageInfo() = nil, want entry preserved after failed deletion")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRejectsSymlinkParent(t *testing.T) {
	dir := withHome(t)
	outside := filepath.Join(filepath.Dir(dir), "outside")
	marker := filepath.Join(outside, "marker")
	if err := os.MkdirAll(filepath.Join(outside, "github.com/o/r/main"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "repos")); err != nil {
		t.Fatal(err)
	}
	repos := []RepoEntry{{
		Name: "github.com/o/r", Version: "main",
		Path: "repos/github.com/o/r/main", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := WriteSources(nil, repos); err != nil {
		t.Fatal(err)
	}

	if _, err := RemoveRepoSource("github.com/o/r", nil); err == nil {
		t.Fatal("RemoveRepoSource() error = nil, want error")
	}
	info, err := RepoInfo("github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("RepoInfo() = nil, want entry preserved after failed deletion")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal(err)
	}
}

func TestReadSourcesReturnsReadError(t *testing.T) {
	dir := withHome(t)
	if err := os.MkdirAll(filepath.Join(dir, "sources.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSources(); err == nil {
		t.Fatal("ReadSources() error = nil, want error")
	}
}

func TestSourcesJSONShape(t *testing.T) {
	withHome(t)
	if err := WriteSources([]PackageEntry{{Name: "zod", Version: "1.0.0", Registry: "npm", Path: "repos/x", FetchedAt: "now"}}, nil); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(mustIndexPath(t))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["updatedAt"]; !ok {
		t.Fatalf("updatedAt missing in %s", content)
	}
}

func mustIndexPath(t *testing.T) string {
	t.Helper()
	home, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, "sources.json")
}
