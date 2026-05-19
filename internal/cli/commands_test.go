package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"repobridge/internal/cache"
	"repobridge/internal/source"
)

func executeForTest(args ...string) (string, string, error) {
	return executeForTestWithOptions(Options{}, args...)
}

func executeForTestWithOptions(opts Options, args ...string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	opts.Version = "test-version"
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	cmd := NewRootCommand(opts)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

type ensureCall struct {
	spec string
	opts source.Options
}

type fakeApp struct {
	outcomes map[string]source.Outcome
	calls    []ensureCall
}

func (a *fakeApp) EnsureCached(spec string, opts source.Options) (source.Outcome, error) {
	a.calls = append(a.calls, ensureCall{spec: spec, opts: opts})
	return a.outcomes[spec], nil
}

func withHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", dir)
	return dir
}

func TestRootVersion(t *testing.T) {
	stdout, _, err := executeForTest("--version")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "test-version") {
		t.Fatalf("stdout = %q, want version", stdout)
	}
}

func TestRemoveAlias(t *testing.T) {
	_, _, err := executeForTest("rm")
	if err == nil {
		t.Fatal("Execute() error = nil, want missing arg error")
	}
	if !strings.Contains(err.Error(), "requires at least 1 arg") {
		t.Fatalf("error = %v, want missing arg error", err)
	}
}

func TestFetchRequiresArgs(t *testing.T) {
	_, _, err := executeForTest("fetch")
	if err == nil {
		t.Fatal("Execute() error = nil, want missing arg error")
	}
}

func TestListEmpty(t *testing.T) {
	withHome(t)

	stdout, _, err := executeForTest("list")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "No sources cached yet." {
		t.Fatalf("stdout = %q, want empty cache message", stdout)
	}
}

func TestCleanEmpty(t *testing.T) {
	withHome(t)

	stdout, _, err := executeForTest("clean")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "Cleaned 0 source(s)" {
		t.Fatalf("stdout = %q, want zero clean summary", stdout)
	}
}

func TestPathEnsuresCachedAndPrintsOnlyAbsolutePath(t *testing.T) {
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"zod@3.22.4": {Path: filepath.Join(t.TempDir(), "zod")},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "path", "--cwd", "/tmp/project", "zod@3.22.4")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout != app.outcomes["zod@3.22.4"].Path+"\n" {
		t.Fatalf("stdout = %q, want only path", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if len(app.calls) != 1 {
		t.Fatalf("calls = %#v, want one call", app.calls)
	}
	if app.calls[0].spec != "zod@3.22.4" || app.calls[0].opts.CWD != "/tmp/project" || app.calls[0].opts.Verbose {
		t.Fatalf("call = %#v, want spec, cwd, non-verbose", app.calls[0])
	}
}

func TestFetchEnsuresCachedAndSummarizesOutcomes(t *testing.T) {
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"zod@3.22.4":     {Name: "zod", Version: "3.22.4", SourceLabel: "npm", Path: "/cache/zod"},
		"left-pad@1.3.0": {Name: "left-pad", Version: "1.3.0", SourceLabel: "npm", Path: "/cache/left-pad", FromCache: true},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "fetch", "--cwd", "/tmp/project", "zod@3.22.4", "left-pad@1.3.0")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"Fetched zod@3.22.4 from npm",
		"Cached left-pad@1.3.0 from npm",
		"Fetched 1 source(s), 1 already cached",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if len(app.calls) != 2 {
		t.Fatalf("calls = %#v, want two calls", app.calls)
	}
	for _, call := range app.calls {
		if call.opts.CWD != "/tmp/project" || !call.opts.Verbose {
			t.Fatalf("call = %#v, want cwd and verbose", call)
		}
	}
}

func TestFetchDisplaysMavenLabel(t *testing.T) {
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"maven:org.example:lib@1.0.0": {
			Name:        "org.example:lib",
			Version:     "1.0.0",
			SourceLabel: "Maven",
			Path:        "/cache/lib",
		},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "fetch", "maven:org.example:lib@1.0.0")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Fetched org.example:lib@1.0.0 from Maven") {
		t.Fatalf("stdout = %q, want Maven fetch line", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestFetchDisplaysNuGetLabel(t *testing.T) {
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"nuget:Newtonsoft.Json@13.0.3": {
			Name:        "Newtonsoft.Json",
			Version:     "13.0.3",
			SourceLabel: "NuGet",
			Path:        "/cache/Newtonsoft.Json",
		},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "fetch", "nuget:Newtonsoft.Json@13.0.3")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Fetched Newtonsoft.Json@13.0.3 from NuGet") {
		t.Fatalf("stdout = %q, want NuGet fetch line", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestListJSONUsesCacheIndex(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{{
		Name: "zod", Version: "3.22.4", Registry: "npm",
		Path: "repos/github.com/colinhacks/zod/3.22.4", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	repos := []cache.RepoEntry{{
		Name: "github.com/owner/repo", Version: "main",
		Path: "repos/github.com/owner/repo/main", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := cache.WriteSources(packages, repos); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("list", "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got cache.SourcesIndex
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(got.Packages) != 1 || got.Packages[0].Name != "zod" {
		t.Fatalf("packages = %#v", got.Packages)
	}
	if len(got.Repos) != 1 || got.Repos[0].Name != "github.com/owner/repo" {
		t.Fatalf("repos = %#v", got.Repos)
	}
}

func TestRemovePackageUsesCacheHelper(t *testing.T) {
	withHome(t)
	if err := cache.WriteSources([]cache.PackageEntry{{
		Name: "zod", Version: "3.22.4", Registry: "npm",
		Path: "repos/github.com/colinhacks/zod/3.22.4", FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("remove", "zod")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Removed zod from npm") {
		t.Fatalf("stdout = %q, want removed package message", stdout)
	}
	info, err := cache.PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Fatalf("PackageInfo() = %#v, want nil", info)
	}
}

func TestRemoveVersionedPackageKeepsOtherVersions(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{
		{Name: "zod", Version: "3.22.4", Registry: "npm", Path: "repos/github.com/colinhacks/zod/3.22.4"},
		{Name: "zod", Version: "4.0.0", Registry: "npm", Path: "repos/github.com/colinhacks/zod/4.0.0"},
	}
	if err := cache.WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("remove", "zod@3.22.4")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Removed zod@3.22.4 from npm") {
		t.Fatalf("stdout = %q, want versioned removed package message", stdout)
	}
	index, err := cache.ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 1 || index.Packages[0].Version != "4.0.0" {
		t.Fatalf("packages = %#v, want only zod 4.0.0", index.Packages)
	}
}

func TestCleanRejectsReposWithRegistryFilter(t *testing.T) {
	withHome(t)

	_, _, err := executeForTest("clean", "--repos", "--npm")
	if err == nil {
		t.Fatal("Execute() error = nil, want conflicting flags error")
	}
	if !strings.Contains(err.Error(), "--repos cannot be combined with registry filters") {
		t.Fatalf("error = %v, want conflicting flags error", err)
	}
}

func TestSubcommandsUseActiveCobraWriters(t *testing.T) {
	withHome(t)
	cmd := NewRootCommand(Options{Version: "test-version"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "No sources cached yet." {
		t.Fatalf("stdout = %q, want subcommand output in command writer", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestCleanRegistryFilter(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{
		{Name: "zod", Version: "3.22.4", Registry: "npm", Path: "repos/github.com/colinhacks/zod/3.22.4"},
		{Name: "requests", Version: "2.31.0", Registry: "pypi", Path: "repos/github.com/psf/requests/v2.31.0"},
	}
	repos := []cache.RepoEntry{{
		Name: "github.com/owner/repo", Version: "main", Path: "repos/github.com/owner/repo/main",
	}}
	if err := cache.WriteSources(packages, repos); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("clean", "--npm")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "Cleaned 1 source(s)" {
		t.Fatalf("stdout = %q, want one-source clean summary", stdout)
	}
	npmInfo, err := cache.PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if npmInfo != nil {
		t.Fatalf("npm package = %#v, want nil", npmInfo)
	}
	pypiInfo, err := cache.PackageInfo("requests", "pypi")
	if err != nil {
		t.Fatal(err)
	}
	if pypiInfo == nil {
		t.Fatal("pypi package missing after npm clean")
	}
	repoInfo, err := cache.RepoInfo("github.com/owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if repoInfo == nil {
		t.Fatal("repo missing after npm clean")
	}
}

func TestCleanMavenRegistryFilter(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{
		{Name: "zod", Version: "3.22.4", Registry: "npm", Path: "repos/github.com/colinhacks/zod/3.22.4"},
		{Name: "org.example:lib", Version: "1.0.0", Registry: "maven", Path: "repos/maven/org.example/lib/1.0.0"},
	}
	if err := cache.WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("clean", "--maven")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "Cleaned 1 source(s)" {
		t.Fatalf("stdout = %q, want one-source clean summary", stdout)
	}
	mavenInfo, err := cache.PackageInfo("org.example:lib", "maven")
	if err != nil {
		t.Fatal(err)
	}
	if mavenInfo != nil {
		t.Fatalf("maven package = %#v, want nil", mavenInfo)
	}
	npmInfo, err := cache.PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if npmInfo == nil {
		t.Fatal("npm package missing after Maven clean")
	}
}

func TestCleanNuGetRegistryFilter(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{
		{Name: "zod", Version: "3.22.4", Registry: "npm", Path: "repos/github.com/colinhacks/zod/3.22.4"},
		{Name: "Newtonsoft.Json", Version: "13.0.3", Registry: "nuget", Path: "repos/github.com/JamesNK/Newtonsoft.Json/13.0.3"},
	}
	if err := cache.WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("clean", "--nuget")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "Cleaned 1 source(s)" {
		t.Fatalf("stdout = %q, want one-source clean summary", stdout)
	}
	nugetInfo, err := cache.PackageInfo("Newtonsoft.Json", "nuget")
	if err != nil {
		t.Fatal(err)
	}
	if nugetInfo != nil {
		t.Fatalf("nuget package = %#v, want nil", nugetInfo)
	}
	npmInfo, err := cache.PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if npmInfo == nil {
		t.Fatal("npm package missing after NuGet clean")
	}
}
