package source

import (
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestEnsureCachedReturnsExistingMavenPackageCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/maven/org.jetbrains.kotlin/kotlin-stdlib/2.1.0"
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(relativePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, filepath.FromSlash(relativePath), "KotlinVersion.kt"), []byte("class KotlinVersion"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cache.WriteSources([]cache.PackageEntry{{
		Name:      "org.jetbrains.kotlin:kotlin-stdlib",
		Version:   "2.1.0",
		Registry:  string(registry.Maven),
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{}

	got, err := EnsureCached("maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0", Options{Fetcher: fetcher})
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
	if got.Name != "org.jetbrains.kotlin:kotlin-stdlib" || got.Version != "2.1.0" || got.SourceLabel != "Maven" {
		t.Fatalf("outcome = %#v", got)
	}
}

func TestEnsureCachedReturnsExistingNuGetPackageCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/github.com/serilog/serilog/3.1.1"
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(relativePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, filepath.FromSlash(relativePath), "Serilog.csproj"), []byte("<Project />"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cache.WriteSources([]cache.PackageEntry{{
		Name:      "Serilog",
		Version:   "3.1.1",
		Registry:  string(registry.NuGet),
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{}

	got, err := EnsureCached("nuget:Serilog@3.1.1", Options{Fetcher: fetcher})
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
	if got.Name != "Serilog" || got.Version != "3.1.1" || got.SourceLabel != "NuGet" {
		t.Fatalf("outcome = %#v", got)
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

func TestGitFetcherUsesSourceArchiveBeforeGit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	body := zipBytes(t, map[string]string{
		"src/main/java/App.java": "class App {}",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/demo-1.0.0-sources.jar" {
			t.Fatalf("path = %q, want source archive path", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, target, version string) git.CloneResult {
		t.Fatalf("cloneAtTag was called for archive-backed Maven package")
		return git.CloneResult{}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := GitFetcher{Client: server.Client()}.FetchPackage(registry.ResolvedPackage{
		Registry:          registry.Maven,
		Name:              "org.example:demo",
		Version:           "1.0.0",
		RepoURL:           "https://github.com/owner/repo",
		SourceArchiveURL:  server.URL + "/demo-1.0.0-sources.jar",
		SourceMetadataURL: server.URL + "/demo-1.0.0.pom",
	})
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !got.Success {
		t.Fatal("Success = false, want true")
	}
	if got.Path != "repos/maven/org.example/demo/1.0.0" {
		t.Fatalf("Path = %q", got.Path)
	}
	if got.Version != "1.0.0" || got.Registry != registry.Maven {
		t.Fatalf("result = %#v", got)
	}
	if _, err := os.Stat(filepath.Join(home, "repos/maven/org.example/demo/1.0.0/src/main/java/App.java")); err != nil {
		t.Fatal(err)
	}
}

func TestGitFetcherFallsBackToGitWhenSourceArchiveMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	target := filepath.Join(home, "repos/github.com/owner/repo/1.0.0")
	cloneCalled := false
	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, gotTarget, version string) git.CloneResult {
		cloneCalled = true
		if repoURL != "https://github.com/owner/repo" {
			t.Fatalf("repoURL = %q", repoURL)
		}
		if gotTarget != target {
			t.Fatalf("target = %q, want %q", gotTarget, target)
		}
		if version != "1.0.0" {
			t.Fatalf("clone ref = %q, want 1.0.0", version)
		}
		if err := os.MkdirAll(gotTarget, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := GitFetcher{Client: server.Client()}.FetchPackage(registry.ResolvedPackage{
		Registry:         registry.Maven,
		Name:             "org.example:demo",
		Version:          "1.0.0",
		RepoURL:          "https://github.com/owner/repo",
		GitTag:           "v1.0.0",
		SourceArchiveURL: server.URL + "/missing-sources.jar",
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
	if got.Version != "1.0.0" {
		t.Fatalf("Version = %q, want package version", got.Version)
	}
	if got.Path != "repos/github.com/owner/repo/1.0.0" {
		t.Fatalf("Path = %q", got.Path)
	}
}

func TestGitFetcherResolvesMavenSCMWhenSourceArchiveMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/demo-1.0.0-sources.jar":
			http.NotFound(w, r)
		case "/demo-1.0.0.pom":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<project>
  <scm>
    <connection>scm:git:https://github.com/Owner/repo.git</connection>
  </scm>
</project>`))
		default:
			t.Fatalf("unexpected path = %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	target := filepath.Join(home, "repos/github.com/owner/repo/1.0.0")
	cloneCalled := false
	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, gotTarget, version string) git.CloneResult {
		cloneCalled = true
		if repoURL != "https://github.com/owner/repo" {
			t.Fatalf("repoURL = %q", repoURL)
		}
		if gotTarget != target {
			t.Fatalf("target = %q, want %q", gotTarget, target)
		}
		if version != "1.0.0" {
			t.Fatalf("clone ref = %q, want 1.0.0", version)
		}
		if err := os.MkdirAll(gotTarget, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := GitFetcher{Client: server.Client()}.FetchPackage(registry.ResolvedPackage{
		Registry:          registry.Maven,
		Name:              "org.example:demo",
		Version:           "1.0.0",
		GitTag:            "v1.0.0",
		SourceArchiveURL:  server.URL + "/demo-1.0.0-sources.jar",
		SourceMetadataURL: server.URL + "/demo-1.0.0.pom",
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
	if got.Path != "repos/github.com/owner/repo/1.0.0" {
		t.Fatalf("Path = %q", got.Path)
	}
}

func TestGitFetcherReturnsMavenPOMErrorWhenDeferredSCMFails(t *testing.T) {
	t.Setenv("REPOBRIDGE_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/demo-1.0.0-sources.jar":
			http.NotFound(w, r)
		case "/demo-1.0.0.pom":
			http.Error(w, "unavailable", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected path = %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, target, version string) git.CloneResult {
		t.Fatalf("cloneAtTag was called after POM failure")
		return git.CloneResult{}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := GitFetcher{Client: server.Client()}.FetchPackage(registry.ResolvedPackage{
		Registry:          registry.Maven,
		Name:              "org.example:demo",
		Version:           "1.0.0",
		SourceArchiveURL:  server.URL + "/demo-1.0.0-sources.jar",
		SourceMetadataURL: server.URL + "/demo-1.0.0.pom",
	})
	var statusErr repobridge.HTTPStatusError
	if !errors.As(got.Error, &statusErr) {
		t.Fatalf("Error = %T %[1]v, want HTTPStatusError", got.Error)
	}
	if statusErr.Context != "Maven POM" {
		t.Fatalf("Context = %q, want Maven POM", statusErr.Context)
	}
}

func TestGitFetcherDoesNotResolveMavenSCMForNon404ArchiveError(t *testing.T) {
	t.Setenv("REPOBRIDGE_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/demo-1.0.0-sources.jar" {
			t.Fatalf("unexpected POM lookup or path = %q", r.URL.Path)
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	oldCloneAtTag := cloneAtTag
	cloneAtTag = func(repoURL, target, version string) git.CloneResult {
		t.Fatalf("cloneAtTag was called after non-404 archive failure")
		return git.CloneResult{}
	}
	t.Cleanup(func() { cloneAtTag = oldCloneAtTag })

	got := GitFetcher{Client: server.Client()}.FetchPackage(registry.ResolvedPackage{
		Registry:          registry.Maven,
		Name:              "org.example:demo",
		Version:           "1.0.0",
		SourceArchiveURL:  server.URL + "/demo-1.0.0-sources.jar",
		SourceMetadataURL: server.URL + "/demo-1.0.0.pom",
	})
	var statusErr repobridge.HTTPStatusError
	if !errors.As(got.Error, &statusErr) {
		t.Fatalf("Error = %T %[1]v, want HTTPStatusError", got.Error)
	}
	if statusErr.Context != "source archive" {
		t.Fatalf("Context = %q, want source archive", statusErr.Context)
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

func TestFetchPackageWithGitUsesGitRefBeforeGitTag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/1.2.3")
	commit := "0123456789abcdef0123456789abcdef01234567"

	cloneCalled := false
	oldCloneAtTag := cloneAtTag
	oldCloneAtCommit := cloneAtCommit
	cloneAtTag = func(repoURL, gotTarget, version string) git.CloneResult {
		t.Fatalf("cloneAtTag was called; GitRef should take precedence")
		return git.CloneResult{}
	}
	cloneAtCommit = func(repoURL, gotTarget, gotCommit string) git.CloneResult {
		cloneCalled = true
		if repoURL != "https://github.com/owner/repo" {
			t.Fatalf("repoURL = %q", repoURL)
		}
		if gotTarget != target {
			t.Fatalf("target = %q, want %q", gotTarget, target)
		}
		if gotCommit != commit {
			t.Fatalf("commit = %q, want %q", gotCommit, commit)
		}
		if err := os.MkdirAll(gotTarget, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() {
		cloneAtTag = oldCloneAtTag
		cloneAtCommit = oldCloneAtCommit
	})

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry: registry.NuGet,
		Name:     "Example.Package",
		Version:  "1.2.3",
		RepoURL:  "https://github.com/owner/repo",
		GitTag:   "v1.2.3",
		GitRef:   commit,
	})
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !cloneCalled {
		t.Fatal("cloneAtCommit was not called")
	}
	if !got.Success {
		t.Fatal("Success = false, want true")
	}
	if got.Path != "repos/github.com/owner/repo/1.2.3" {
		t.Fatalf("Path = %q", got.Path)
	}
}

func TestFetchPackageWithGitUsesStrictTagCloneForNuGetWithoutGitRef(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/1.2.3")

	cloneCalled := false
	oldCloneAtTag := cloneAtTag
	oldCloneAtTagStrict := cloneAtTagStrict
	cloneAtTag = func(repoURL, gotTarget, version string) git.CloneResult {
		t.Fatalf("cloneAtTag was called; NuGet should use strict tag clone")
		return git.CloneResult{}
	}
	cloneAtTagStrict = func(repoURL, gotTarget, version string) git.CloneResult {
		cloneCalled = true
		if repoURL != "https://github.com/owner/repo" {
			t.Fatalf("repoURL = %q", repoURL)
		}
		if gotTarget != target {
			t.Fatalf("target = %q, want %q", gotTarget, target)
		}
		if version != "1.2.3" {
			t.Fatalf("version = %q, want normalized tag version", version)
		}
		if err := os.MkdirAll(gotTarget, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	t.Cleanup(func() {
		cloneAtTag = oldCloneAtTag
		cloneAtTagStrict = oldCloneAtTagStrict
	})

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry: registry.NuGet,
		Name:     "Example.Package",
		Version:  "1.2.3",
		RepoURL:  "https://github.com/owner/repo",
		GitTag:   "v1.2.3",
	})
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !cloneCalled {
		t.Fatal("cloneAtTagStrict was not called")
	}
	if !got.Success {
		t.Fatal("Success = false, want true")
	}
	if got.Path != "repos/github.com/owner/repo/1.2.3" {
		t.Fatalf("Path = %q", got.Path)
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
