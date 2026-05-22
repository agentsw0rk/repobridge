package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repobridge/internal/cache"
	"repobridge/internal/git"
	"repobridge/internal/registry"
	"repobridge/internal/registry/repo"
	"repobridge/internal/repobridge"
)

type fakeFetcher struct {
	packageResult   FetchResult
	repoResult      FetchResult
	packageCalls    int
	repoCalls       int
	packageRequests []registry.ResolvedPackage
}

func (f *fakeFetcher) FetchPackage(pkg registry.ResolvedPackage) FetchResult {
	f.packageCalls++
	f.packageRequests = append(f.packageRequests, pkg)
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

type fakeResolver struct {
	packageResult registry.ResolvedPackage
	packageErr    error
	packageCalls  int
	packageSpecs  []registry.PackageSpec
	repoResult    repo.Resolved
	repoErr       error
	repoCalls     int
}

func (f *fakeResolver) ResolvePackage(ctx context.Context, spec registry.PackageSpec, client *http.Client) (registry.ResolvedPackage, error) {
	f.packageCalls++
	f.packageSpecs = append(f.packageSpecs, spec)
	if f.packageErr != nil {
		return registry.ResolvedPackage{}, f.packageErr
	}
	result := f.packageResult
	if result.Name == "" {
		result.Name = spec.Name
	}
	if result.Version == "" {
		result.Version = spec.Version
	}
	if result.Registry == "" {
		result.Registry = spec.Registry
	}
	return result, nil
}

func (f *fakeResolver) ResolveRepo(ctx context.Context, spec repo.Spec, client *http.Client) (repo.Resolved, error) {
	f.repoCalls++
	if f.repoErr != nil {
		return repo.Resolved{}, f.repoErr
	}
	return f.repoResult, nil
}

type fakeInstalledVersions struct {
	version string
	calls   int
	reg     registry.Registry
	name    string
	cwd     string
}

func (f *fakeInstalledVersions) InstalledVersion(reg registry.Registry, name, cwd string) string {
	f.calls++
	f.reg = reg
	f.name = name
	f.cwd = cwd
	return f.version
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

func TestEnsureCachedResolvesProjectSpecWithoutFetching(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantProject, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{}

	got, err := EnsureCached("project:.", Options{CWD: project, Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.packageCalls != 0 || fetcher.repoCalls != 0 {
		t.Fatalf("fetcher calls = package:%d repo:%d, want none", fetcher.packageCalls, fetcher.repoCalls)
	}
	if got.Path != wantProject {
		t.Fatalf("Path = %q, want %q", got.Path, wantProject)
	}
	if got.Name != "project:." || got.SourceLabel != "project" || got.SourceKind != "project" {
		t.Fatalf("outcome = %#v, want project metadata", got)
	}
	if got.GraphPath == "" {
		t.Fatalf("GraphPath empty, outcome = %#v", got)
	}
	if !strings.HasPrefix(got.GraphPath, filepath.Join(home, "projects")+string(os.PathSeparator)) {
		t.Fatalf("GraphPath = %q, want under %q", got.GraphPath, filepath.Join(home, "projects"))
	}
	if strings.HasPrefix(got.GraphPath, wantProject) {
		t.Fatalf("GraphPath = %q, must not be inside project %q", got.GraphPath, wantProject)
	}
}

func TestEnsureCachedResolvesProjectRelativeSubtree(t *testing.T) {
	project := t.TempDir()
	subtree := filepath.Join(project, "internal", "cli")
	if err := os.MkdirAll(subtree, 0o755); err != nil {
		t.Fatal(err)
	}
	wantSubtree, err := filepath.EvalSymlinks(subtree)
	if err != nil {
		t.Fatal(err)
	}

	got, err := EnsureCached("project:./internal/cli", Options{CWD: project})
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != wantSubtree {
		t.Fatalf("Path = %q, want %q", got.Path, wantSubtree)
	}
}

func TestEnsureCachedResolvesAbsoluteProjectOutsideCWD(t *testing.T) {
	project := t.TempDir()
	outside := t.TempDir()
	wantOutside, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}

	got, err := EnsureCached("project:"+outside, Options{CWD: project})
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != wantOutside {
		t.Fatalf("Path = %q, want %q", got.Path, wantOutside)
	}
	if got.Name != "project:"+filepath.ToSlash(wantOutside) {
		t.Fatalf("Name = %q, want absolute project spec", got.Name)
	}
}

func TestEnsureCachedRejectsProjectTraversalOutsideCWD(t *testing.T) {
	project := t.TempDir()
	_, err := EnsureCached("project:../outside", Options{CWD: project})
	if err == nil {
		t.Fatal("EnsureCached(project traversal) error = nil, want error")
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

func TestEnsureCachedResolvesMavenUsingRepositoriesFromCWD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	t.Setenv("HOME", t.TempDir())
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "pom.xml"), []byte(`<project>
  <repositories>
    <repository>
      <id>internal</id>
      <url>https://repo.example.com/internal</url>
    </repository>
  </repositories>
</project>`), 0o644); err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{packageResult: FetchResult{
		Package:  "org.example:demo",
		Version:  "1.0.0",
		Registry: registry.Maven,
		Path:     "repos/maven/org.example/demo/1.0.0",
		Success:  true,
	}}

	_, err := EnsureCached("maven:org.example:demo@1.0.0", Options{CWD: project, Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.packageCalls != 1 {
		t.Fatalf("package calls = %d, want 1", fetcher.packageCalls)
	}
	got := fetcher.packageRequests[0]
	if len(got.ArtifactCandidates) < 2 {
		t.Fatalf("ArtifactCandidates = %#v, want internal plus Maven Central", got.ArtifactCandidates)
	}
	if got.ArtifactCandidates[0].RepositoryID != "internal" {
		t.Fatalf("first repository = %q, want internal", got.ArtifactCandidates[0].RepositoryID)
	}
	if got.ArtifactCandidates[0].SourceArchiveURL != "https://repo.example.com/internal/org/example/demo/1.0.0/demo-1.0.0-sources.jar" {
		t.Fatalf("first source URL = %q", got.ArtifactCandidates[0].SourceArchiveURL)
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

func TestGitFetcherTriesMavenSourceCandidatesInOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	body := zipBytes(t, map[string]string{
		"src/main/java/App.java": "class App {}",
	})
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/repo-one/demo-1.0.0-sources.jar":
			http.NotFound(w, r)
		case "/repo-two/demo-1.0.0-sources.jar":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		default:
			t.Fatalf("unexpected path = %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	got := GitFetcher{Client: server.Client()}.FetchPackage(registry.ResolvedPackage{
		Registry: registry.Maven,
		Name:     "org.example:demo",
		Version:  "1.0.0",
		ArtifactCandidates: []registry.ArtifactCandidate{
			{RepositoryID: "one", SourceArchiveURL: server.URL + "/repo-one/demo-1.0.0-sources.jar"},
			{RepositoryID: "two", SourceArchiveURL: server.URL + "/repo-two/demo-1.0.0-sources.jar"},
		},
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
	if len(paths) != 2 || paths[0] != "/repo-one/demo-1.0.0-sources.jar" || paths[1] != "/repo-two/demo-1.0.0-sources.jar" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestGitFetcherTriesMavenPOMCandidatesInOrderAfterSourcesMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/repo-one/demo-1.0.0-sources.jar", "/repo-two/demo-1.0.0-sources.jar":
			http.NotFound(w, r)
		case "/repo-one/demo-1.0.0.pom":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<project></project>`))
		case "/repo-two/demo-1.0.0.pom":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<project><scm><connection>scm:git:https://github.com/Owner/repo.git</connection></scm></project>`))
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
		Registry: registry.Maven,
		Name:     "org.example:demo",
		Version:  "1.0.0",
		GitTag:   "v1.0.0",
		ArtifactCandidates: []registry.ArtifactCandidate{
			{
				RepositoryID:      "one",
				SourceArchiveURL:  server.URL + "/repo-one/demo-1.0.0-sources.jar",
				SourceMetadataURL: server.URL + "/repo-one/demo-1.0.0.pom",
			},
			{
				RepositoryID:      "two",
				SourceArchiveURL:  server.URL + "/repo-two/demo-1.0.0-sources.jar",
				SourceMetadataURL: server.URL + "/repo-two/demo-1.0.0.pom",
			},
		},
	})
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !cloneCalled {
		t.Fatal("cloneAtTag was not called")
	}
	wantPaths := []string{
		"/repo-one/demo-1.0.0-sources.jar",
		"/repo-two/demo-1.0.0-sources.jar",
		"/repo-one/demo-1.0.0.pom",
		"/repo-two/demo-1.0.0.pom",
	}
	if len(paths) != len(wantPaths) {
		t.Fatalf("paths = %#v, want %#v", paths, wantPaths)
	}
	for i := range wantPaths {
		if paths[i] != wantPaths[i] {
			t.Fatalf("paths = %#v, want %#v", paths, wantPaths)
		}
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

func TestAcquirerEnsureUsesInjectedPackageResolver(t *testing.T) {
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
	resolver := &fakeResolver{
		packageResult: registry.ResolvedPackage{
			Registry: registry.NPM,
			Name:     "pkg",
			Version:  "1.2.3",
			RepoURL:  "https://github.com/owner/repo",
		},
	}
	fetcher := &fakeFetcher{
		packageResult: FetchResult{
			Package:  "pkg",
			Version:  "1.2.3",
			Registry: registry.NPM,
			Path:     "repos/github.com/owner/repo/1.2.3",
			Success:  true,
		},
	}

	got, err := NewAcquirer(WithResolver(resolver), WithFetcher(fetcher)).Ensure(context.Background(), Request{Spec: "pkg@1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.packageCalls != 1 {
		t.Fatalf("resolver package calls = %d, want 1", resolver.packageCalls)
	}
	if fetcher.packageCalls != 1 {
		t.Fatalf("package fetch calls = %d, want 1", fetcher.packageCalls)
	}
	if got.FromCache {
		t.Fatal("FromCache = true, want false")
	}
}

func TestAcquirerEnsureUsesInjectedInstalledVersionDetector(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/github.com/owner/repo/1.2.3"
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(relativePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, filepath.FromSlash(relativePath), "package.json"), []byte("{}"), 0o644); err != nil {
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
	resolver := &fakeResolver{}
	fetcher := &fakeFetcher{}
	versions := &fakeInstalledVersions{version: "1.2.3"}

	got, err := NewAcquirer(
		WithInstalledVersionDetector(versions),
		WithResolver(resolver),
		WithFetcher(fetcher),
	).Ensure(context.Background(), Request{Spec: "pkg", CWD: "/work/project"})
	if err != nil {
		t.Fatal(err)
	}
	if versions.calls != 1 || versions.reg != registry.NPM || versions.name != "pkg" || versions.cwd != "/work/project" {
		t.Fatalf("installed version lookup = calls:%d registry:%q name:%q cwd:%q", versions.calls, versions.reg, versions.name, versions.cwd)
	}
	if resolver.packageCalls != 0 || fetcher.packageCalls != 0 {
		t.Fatalf("package resolver/fetcher calls = %d/%d, want none", resolver.packageCalls, fetcher.packageCalls)
	}
	if !got.FromCache {
		t.Fatal("FromCache = false, want true")
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
	resolver := &fakeResolver{
		packageResult: registry.ResolvedPackage{
			Registry: registry.NPM,
			Name:     "pkg",
			Version:  "1.2.3",
			RepoURL:  "https://github.com/owner/repo",
		},
	}
	fetcher := &fakeFetcher{
		packageResult: FetchResult{
			Package:  "pkg",
			Version:  "1.2.3",
			Registry: registry.NPM,
			Path:     "repos/github.com/owner/repo/1.2.3",
			Success:  true,
		},
	}

	got, err := NewAcquirer(WithResolver(resolver), WithFetcher(fetcher)).Ensure(context.Background(), Request{Spec: "pkg@1.2.3"})
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
	resolver := &fakeResolver{
		packageResult: registry.ResolvedPackage{
			Registry: registry.NPM,
			Name:     "pkg",
			Version:  "1.2.3",
			RepoURL:  "https://github.com/owner/repo",
		},
	}
	fetcher := &fakeFetcher{
		packageResult: FetchResult{
			Package:  "pkg",
			Version:  "1.2.3",
			Registry: registry.NPM,
			Path:     relativePath,
			Success:  true,
		},
	}

	got, err := NewAcquirer(WithResolver(resolver), WithFetcher(fetcher)).Ensure(context.Background(), Request{Spec: "pkg@1.2.3"})
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
	resolver := &fakeResolver{
		packageResult: registry.ResolvedPackage{
			Registry: registry.NPM,
			Name:     "pkg",
			Version:  "1.2.3",
			RepoURL:  "https://github.com/owner/repo",
		},
	}
	fetcher := &fakeFetcher{
		packageResult: FetchResult{
			Package:  "pkg",
			Version:  "1.2.3",
			Registry: registry.NPM,
			Path:     relativePath,
			Success:  true,
		},
	}

	got, err := NewAcquirer(WithResolver(resolver), WithFetcher(fetcher)).Ensure(context.Background(), Request{Spec: "pkg@1.2.3"})
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
