# NuGet Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add NuGet package support that reads `.nuspec` repository metadata from `.nupkg` files and fetches the matching Git source by commit or version tag.

**Architecture:** NuGet becomes a first-class registry in `internal/registry`, with resolver logic isolated in `internal/registry/nuget`. The resolver uses the NuGet v3 service index, registration metadata, and flat-container package content to determine the package version and repository metadata. Source fetching stays Git-based: NuGet never caches extracted package binaries as source, and it never falls back to a repository default branch.

**Tech Stack:** Go 1.22, standard `net/http`, `encoding/json`, `encoding/xml`, `archive/zip`, existing `registry`, `source`, `git`, `cache`, `httpx`, and Cobra CLI packages.

---

## File Structure

- Modify `internal/registry/registry.go`: add `NuGet`, prefixes, parser support, `SupportedRegistry`, and `ResolvedPackage.GitRef`.
- Modify `internal/registry/registry_test.go`: cover NuGet/dotnet detection and parsing.
- Create `internal/registry/nuget/nuget.go`: NuGet v3 resolver, service index parsing, version selection, `.nupkg` download, `.nuspec` parsing, repository normalization.
- Create `internal/registry/nuget/nuget_test.go`: resolver behavior with `httptest` and generated `.nupkg` ZIPs.
- Modify `internal/git/git.go`: add strict clone helpers for commits and no-default-branch tag probing.
- Modify `internal/git/git_test.go`: cover commit clone behavior and no-default-branch tag failure.
- Modify `internal/source/git_fetcher.go`: prefer `ResolvedPackage.GitRef`, allow NuGet tag probing without default-branch fallback.
- Modify `internal/source/source.go`: route NuGet to the new resolver.
- Modify `internal/source/source_test.go`: cover NuGet cache hit and GitRef propagation.
- Modify `internal/cli/commands.go`: add `--nuget` clean filter.
- Modify `internal/cli/commands_test.go`: cover `clean --nuget` and NuGet output label.
- Modify `README.md`: document NuGet support and examples.
- Create `docs/features/17-nuget-sources-done.md`: completion note after implementation.

## Task 1: Add NuGet Registry Parsing

**Files:**
- Modify: `internal/registry/registry.go`
- Modify: `internal/registry/registry_test.go`

- [ ] **Step 1: Write failing registry tests**

Add these rows to `TestDetectRegistry` in `internal/registry/registry_test.go`:

```go
{"nuget:Newtonsoft.Json", NuGet, "Newtonsoft.Json"},
{"dotnet:Serilog", NuGet, "Serilog"},
```

Add these rows to `TestParsePackageSpec`:

```go
{"nuget:Newtonsoft.Json@13.0.3", NuGet, "Newtonsoft.Json", "13.0.3"},
{"dotnet:Serilog@3.1.1", NuGet, "Serilog", "3.1.1"},
{"nuget:Example.Package@2.0.0-beta.1", NuGet, "Example.Package", "2.0.0-beta.1"},
```

Add these rows to `TestDetectInputType`:

```go
"nuget:Newtonsoft.Json@13.0.3": PackageInput,
"dotnet:Serilog@3.1.1":        PackageInput,
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/registry
```

Expected: FAIL because `NuGet` is undefined or the detector returns `NPM`.

- [ ] **Step 3: Implement NuGet registry parsing**

In `internal/registry/registry.go`, add the registry constant:

```go
const (
	NPM    Registry = "npm"
	PyPI   Registry = "pypi"
	Crates Registry = "crates"
	Maven  Registry = "maven"
	NuGet  Registry = "nuget"
)
```

Add the label case:

```go
case NuGet:
	return "NuGet"
```

Extend `ResolvedPackage` with a real ref field:

```go
type ResolvedPackage struct {
	Registry          Registry
	Name              string
	Version           string
	RepoURL           string
	RepoDirectory     string
	GitTag            string
	GitRef            string
	SourceArchiveURL  string
	SourceMetadataURL string
}
```

Add prefixes:

```go
{"nuget:", NuGet},
{"dotnet:", NuGet},
```

Route parsing:

```go
case NuGet:
	return ParseNuGetSpec(spec)
```

Add parser:

```go
func ParseNuGetSpec(spec string) (string, string) {
	trimmed := strings.TrimSpace(spec)
	if at := strings.LastIndex(trimmed, "@"); at > 0 {
		return strings.TrimSpace(trimmed[:at]), strings.TrimSpace(trimmed[at+1:])
	}
	return trimmed, ""
}
```

Allow the registry:

```go
case NPM, PyPI, Crates, Maven, NuGet:
	return nil
```

- [ ] **Step 4: Run registry tests**

Run:

```bash
go test ./internal/registry
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/registry/registry.go internal/registry/registry_test.go
git commit -m "Add NuGet registry parsing"
```

## Task 2: Add Strict Git Fetching Primitives

**Files:**
- Modify: `internal/git/git.go`
- Modify: `internal/git/git_test.go`

- [ ] **Step 1: Write failing Git tests**

Add tests to `internal/git/git_test.go`:

```go
func TestCloneAtCommitFetchesExactCommit(t *testing.T) {
	old := gitRunner
	t.Cleanup(func() { gitRunner = old })
	var calls [][]string
	gitRunner = func(env []string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{}, args...))
		return nil, nil
	}
	target := t.TempDir() + "/target"

	result := CloneAtCommit("https://github.com/owner/repo", target, "0123456789abcdef0123456789abcdef01234567")
	if result.Error != nil || !result.Success {
		t.Fatalf("result = %#v", result)
	}
	want := [][]string{
		{"clone", "--no-checkout", "--depth", "1", "https://github.com/owner/repo", target},
		{"-C", target, "fetch", "--depth", "1", "origin", "0123456789abcdef0123456789abcdef01234567"},
		{"-C", target, "checkout", "--detach", "0123456789abcdef0123456789abcdef01234567"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestCloneAtTagStrictDoesNotCloneDefaultBranch(t *testing.T) {
	old := gitRunner
	t.Cleanup(func() { gitRunner = old })
	target := t.TempDir() + "/target"
	var calls [][]string
	gitRunner = func(env []string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{}, args...))
		return []byte("fatal: couldn't find remote ref"), errors.New("exit status 128")
	}

	result := CloneAtTagStrict("https://github.com/owner/repo", target, "1.2.3")
	if result.Error == nil {
		t.Fatal("error = nil, want missing tag error")
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v, want only v1.2.3 and 1.2.3 tag attempts", calls)
	}
}
```

Ensure imports include:

```go
import (
	"errors"
	"reflect"
	"testing"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/git
```

Expected: FAIL because `CloneAtCommit` and `CloneAtTagStrict` are undefined.

- [ ] **Step 3: Implement strict Git helpers**

In `internal/git/git.go`, add:

```go
func CloneAtCommit(repoURL, target, commit string) CloneResult {
	if err := ensureTargetAvailable(target); err != nil {
		return CloneResult{Error: err}
	}
	if !commitSHARE.MatchString(commit) {
		return CloneResult{Error: fmt.Errorf("clone commit %q is not a valid commit SHA", commit)}
	}
	if err := runGitClone(repoURL, target, "--no-checkout", "--depth", "1"); err != nil {
		return CloneResult{Error: err}
	}
	if err := runGitInTarget(repoURL, target, "fetch", "--depth", "1", "origin", commit); err != nil {
		removeFailedClone(target)
		return CloneResult{Error: err}
	}
	if err := runGitInTarget(repoURL, target, "checkout", "--detach", commit); err != nil {
		removeFailedClone(target)
		return CloneResult{Error: err}
	}
	return CloneResult{Success: true}
}

func CloneAtTagStrict(repoURL, target, version string) CloneResult {
	if err := ensureTargetAvailable(target); err != nil {
		return CloneResult{Error: err}
	}
	for _, tag := range []string{"v" + version, version} {
		if err := runGitClone(repoURL, target, "--depth", "1", "--branch", tag); err == nil {
			return CloneResult{Success: true}
		} else if isMissingRefError(err) {
			removeFailedClone(target)
		} else {
			return CloneResult{Error: err}
		}
	}
	return CloneResult{Error: fmt.Errorf("could not find tag v%s or %s", version, version)}
}

func runGitInTarget(repoURL, target string, args ...string) error {
	env := authConfigEnv(repoURL)
	cmdArgs := append([]string{"-C", target}, args...)
	output, err := gitRunner(env, cmdArgs...)
	if err != nil {
		return fmt.Errorf("git command failed: %s\n%s", redactSecrets(err.Error()), redactSecrets(string(output)))
	}
	return nil
}
```

- [ ] **Step 4: Run Git tests**

Run:

```bash
go test ./internal/git
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "Add strict Git source refs"
```

## Task 3: Implement NuGet Resolver Tests and Core Resolver

**Files:**
- Create: `internal/registry/nuget/nuget.go`
- Create: `internal/registry/nuget/nuget_test.go`

- [ ] **Step 1: Write failing resolver tests**

Create `internal/registry/nuget/nuget_test.go` with these tests:

```go
package nuget

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

func TestResolveUsesRepositoryCommitFromNuspec(t *testing.T) {
	nupkg := nugetPackage(t, "Newtonsoft.Json.nuspec", `<package><metadata><id>Newtonsoft.Json</id><version>13.0.3</version><repository type="git" url="https://github.com/JamesNK/Newtonsoft.Json.git" commit="0123456789abcdef0123456789abcdef01234567" /></metadata></package>`)
	server := nugetServer(t, map[string][]byte{
		"/v3/index.json": []byte(`{"resources":[{"@id":"` + "BASE" + `/flat/","@type":"PackageBaseAddress/3.0.0"},{"@id":"` + "BASE" + `/registration/","@type":"RegistrationsBaseUrl/3.6.0"}]}`),
		"/flat/newtonsoft.json/index.json": []byte(`{"versions":["13.0.1","13.0.3"]}`),
		"/flat/newtonsoft.json/13.0.3/newtonsoft.json.13.0.3.nupkg": nupkg,
	})

	got, err := Resolve("Newtonsoft.Json", "13.0.3", server.Client(), server.URL+"/v3/index.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.Registry != registry.NuGet || got.Name != "Newtonsoft.Json" || got.Version != "13.0.3" {
		t.Fatalf("resolved = %#v", got)
	}
	if got.RepoURL != "https://github.com/JamesNK/Newtonsoft.Json" {
		t.Fatalf("RepoURL = %q", got.RepoURL)
	}
	if got.GitRef != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("GitRef = %q", got.GitRef)
	}
	if got.GitTag != "" {
		t.Fatalf("GitTag = %q, want empty when commit exists", got.GitTag)
	}
}

func TestResolveLatestStableIgnoresPrerelease(t *testing.T) {
	nupkg := nugetPackage(t, "Serilog.nuspec", `<package><metadata><id>Serilog</id><version>3.1.1</version><repository type="git" url="https://github.com/serilog/serilog.git" /></metadata></package>`)
	server := nugetServer(t, map[string][]byte{
		"/v3/index.json": []byte(`{"resources":[{"@id":"` + "BASE" + `/flat/","@type":"PackageBaseAddress/3.0.0"},{"@id":"` + "BASE" + `/registration/","@type":"RegistrationsBaseUrl/3.6.0"}]}`),
		"/flat/serilog/index.json": []byte(`{"versions":["3.0.0","3.2.0-beta.1","3.1.1"]}`),
		"/flat/serilog/3.1.1/serilog.3.1.1.nupkg": nupkg,
	})

	got, err := Resolve("Serilog", "", server.Client(), server.URL+"/v3/index.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "3.1.1" || got.GitTag != "v3.1.1" {
		t.Fatalf("resolved = %#v", got)
	}
}

func TestResolveExplicitPrerelease(t *testing.T) {
	nupkg := nugetPackage(t, "Example.Package.nuspec", `<package><metadata><id>Example.Package</id><version>2.0.0-beta.1</version><repository type="git" url="https://github.com/owner/repo.git" /></metadata></package>`)
	server := nugetServer(t, map[string][]byte{
		"/v3/index.json": []byte(`{"resources":[{"@id":"` + "BASE" + `/flat/","@type":"PackageBaseAddress/3.0.0"},{"@id":"` + "BASE" + `/registration/","@type":"RegistrationsBaseUrl/3.6.0"}]}`),
		"/flat/example.package/index.json": []byte(`{"versions":["2.0.0-beta.1"]}`),
		"/flat/example.package/2.0.0-beta.1/example.package.2.0.0-beta.1.nupkg": nupkg,
	})

	got, err := Resolve("Example.Package", "2.0.0-beta.1", server.Client(), server.URL+"/v3/index.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "2.0.0-beta.1" || got.GitTag != "v2.0.0-beta.1" {
		t.Fatalf("resolved = %#v", got)
	}
}

func TestResolveMissingPackage(t *testing.T) {
	server := nugetServer(t, map[string][]byte{
		"/v3/index.json": []byte(`{"resources":[{"@id":"` + "BASE" + `/flat/","@type":"PackageBaseAddress/3.0.0"},{"@id":"` + "BASE" + `/registration/","@type":"RegistrationsBaseUrl/3.6.0"}]}`),
	})

	_, err := Resolve("Missing.Package", "", server.Client(), server.URL+"/v3/index.json")
	var notFound repobridge.PackageNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("error = %T %[1]v, want PackageNotFoundError", err)
	}
}

func TestResolveRejectsMissingRepository(t *testing.T) {
	nupkg := nugetPackage(t, "NoRepo.nuspec", `<package><metadata><id>NoRepo</id><version>1.0.0</version></metadata></package>`)
	server := nugetServer(t, map[string][]byte{
		"/v3/index.json": []byte(`{"resources":[{"@id":"` + "BASE" + `/flat/","@type":"PackageBaseAddress/3.0.0"},{"@id":"` + "BASE" + `/registration/","@type":"RegistrationsBaseUrl/3.6.0"}]}`),
		"/flat/norepo/index.json": []byte(`{"versions":["1.0.0"]}`),
		"/flat/norepo/1.0.0/norepo.1.0.0.nupkg": nupkg,
	})

	_, err := Resolve("NoRepo", "1.0.0", server.Client(), server.URL+"/v3/index.json")
	var noRepo repobridge.NoRepoURLError
	if !errors.As(err, &noRepo) {
		t.Fatalf("error = %T %[1]v, want NoRepoURLError", err)
	}
}

func nugetPackage(t *testing.T, name, nuspec string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(nuspec)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func nugetServer(t *testing.T, responses map[string][]byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		body = []byte(strings.ReplaceAll(string(body), "BASE", "http://"+r.Host))
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}
```

- [ ] **Step 2: Run resolver tests and verify failure**

Run:

```bash
go test ./internal/registry/nuget
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement NuGet resolver**

Create `internal/registry/nuget/nuget.go`:

```go
package nuget

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"repobridge/internal/httpx"
	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

const DefaultServiceIndex = "https://api.nuget.org/v3/index.json"

const maxNupkgBytes int64 = 256 * 1024 * 1024
const maxNuspecBytes int64 = 2 * 1024 * 1024

type serviceIndex struct {
	Resources []serviceResource `json:"resources"`
}

type serviceResource struct {
	ID   string `json:"@id"`
	Type string `json:"@type"`
}

type versionsResponse struct {
	Versions []string `json:"versions"`
}

type nuspecPackage struct {
	Metadata nuspecMetadata `xml:"metadata"`
}

type nuspecMetadata struct {
	ID         string           `xml:"id"`
	Version    string           `xml:"version"`
	Repository nuspecRepository `xml:"repository"`
}

type nuspecRepository struct {
	Type   string `xml:"type,attr"`
	URL    string `xml:"url,attr"`
	Branch string `xml:"branch,attr"`
	Commit string `xml:"commit,attr"`
}

func Resolve(name, version string, client *http.Client, serviceIndexURL string) (registry.ResolvedPackage, error) {
	if client == nil {
		client = httpx.NewClient()
	}
	if serviceIndexURL == "" {
		serviceIndexURL = DefaultServiceIndex
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return registry.ResolvedPackage{}, fmt.Errorf("NuGet package name must not be empty")
	}

	resources, err := loadResources(client, serviceIndexURL)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	flatBase, err := resourceURL(resources, "PackageBaseAddress/")
	if err != nil {
		return registry.ResolvedPackage{}, err
	}

	versions, err := loadVersions(client, flatBase, name)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	resolvedVersion, err := resolveVersion(name, version, versions)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}

	tmp, err := downloadNupkg(client, flatBase, name, resolvedVersion)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	defer os.Remove(tmp)

	nuspec, err := readNuspec(tmp)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	repoURL := registry.NormalizeRepoURL(normalizeGitURL(nuspec.Metadata.Repository.URL))
	if strings.ToLower(nuspec.Metadata.Repository.Type) != "git" || repoURL == "" {
		return registry.ResolvedPackage{}, repobridge.NoRepoURLError{Message: fmt.Sprintf("No usable Git repository URL found for NuGet package %q", name+"@"+resolvedVersion)}
	}

	canonicalName := firstNonEmpty(strings.TrimSpace(nuspec.Metadata.ID), name)
	canonicalVersion := firstNonEmpty(strings.TrimSpace(nuspec.Metadata.Version), resolvedVersion)
	resolved := registry.ResolvedPackage{
		Registry: registry.NuGet,
		Name:     canonicalName,
		Version:  canonicalVersion,
		RepoURL:  repoURL,
	}
	if commit := strings.TrimSpace(nuspec.Metadata.Repository.Commit); commit != "" {
		resolved.GitRef = commit
	} else {
		resolved.GitTag = "v" + canonicalVersion
	}
	return resolved, nil
}

func loadResources(client *http.Client, serviceIndexURL string) ([]serviceResource, error) {
	resp, err := client.Get(serviceIndexURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, repobridge.HTTPStatusError{Context: "NuGet service index", Status: resp.Status}
	}
	var data serviceIndex
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Resources, nil
}

func resourceURL(resources []serviceResource, typePrefix string) (string, error) {
	for _, resource := range resources {
		if strings.HasPrefix(resource.Type, typePrefix) && strings.TrimSpace(resource.ID) != "" {
			return strings.TrimRight(resource.ID, "/") + "/", nil
		}
	}
	return "", fmt.Errorf("NuGet service index is missing resource %s", typePrefix)
}

func loadVersions(client *http.Client, flatBase, name string) ([]string, error) {
	target := strings.TrimRight(flatBase, "/") + "/" + strings.ToLower(url.PathEscape(name)) + "/index.json"
	resp, err := client.Get(target)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, repobridge.PackageNotFoundError{Name: name, Registry: "NuGet"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, repobridge.HTTPStatusError{Context: "NuGet versions", Status: resp.Status}
	}
	var data versionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Versions, nil
}

func resolveVersion(name, requested string, versions []string) (string, error) {
	if requested != "" {
		for _, version := range versions {
			if strings.EqualFold(version, requested) {
				return version, nil
			}
		}
		return "", repobridge.VersionNotFoundError{Message: fmt.Sprintf("Version %q not found for NuGet package %q", requested, name)}
	}
	for i := len(versions) - 1; i >= 0; i-- {
		if !isPrerelease(versions[i]) {
			return versions[i], nil
		}
	}
	return "", repobridge.VersionNotFoundError{Message: fmt.Sprintf("No stable version found for NuGet package %q", name)}
}

func isPrerelease(version string) bool {
	return strings.Contains(version, "-")
}

func downloadNupkg(client *http.Client, flatBase, name, version string) (string, error) {
	lowerName := strings.ToLower(name)
	lowerVersion := strings.ToLower(version)
	escapedName := strings.ToLower(url.PathEscape(lowerName))
	escapedVersion := strings.ToLower(url.PathEscape(lowerVersion))
	fileName := path.Base(escapedName) + "." + path.Base(escapedVersion) + ".nupkg"
	target := strings.TrimRight(flatBase, "/") + "/" + escapedName + "/" + escapedVersion + "/" + fileName
	resp, err := client.Get(target)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", repobridge.VersionNotFoundError{Message: fmt.Sprintf("NuGet package content not found for %q", name+"@"+version)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", repobridge.HTTPStatusError{Context: "NuGet package content", Status: resp.Status}
	}
	if resp.ContentLength > maxNupkgBytes {
		return "", fmt.Errorf("NuGet package exceeds maximum size of %d bytes", maxNupkgBytes)
	}
	tmp, err := os.CreateTemp("", "repobridge-nuget-*.nupkg")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	written, err := io.Copy(tmp, io.LimitReader(resp.Body, maxNupkgBytes+1))
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if written > maxNupkgBytes {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("NuGet package exceeds maximum size of %d bytes", maxNupkgBytes)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func readNuspec(path string) (nuspecPackage, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nuspecPackage{}, err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			continue
		}
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
			return nuspecPackage{}, fmt.Errorf("NuGet package contains unsafe path: %s", entry.Name)
		}
		if !strings.EqualFold(path.Ext(name), ".nuspec") {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return nuspecPackage{}, err
		}
		content, readErr := io.ReadAll(io.LimitReader(rc, maxNuspecBytes+1))
		closeErr := rc.Close()
		if readErr != nil {
			return nuspecPackage{}, readErr
		}
		if closeErr != nil {
			return nuspecPackage{}, closeErr
		}
		if int64(len(content)) > maxNuspecBytes {
			return nuspecPackage{}, fmt.Errorf("NuGet nuspec exceeds maximum size of %d bytes", maxNuspecBytes)
		}
		var parsed nuspecPackage
		if err := xml.Unmarshal(content, &parsed); err != nil {
			return nuspecPackage{}, err
		}
		return parsed, nil
	}
	return nuspecPackage{}, fmt.Errorf("NuGet package does not contain a .nuspec file")
}

func normalizeGitURL(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "git+")
	value = strings.TrimSuffix(value, ".git")
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
```

- [ ] **Step 4: Run resolver tests**

Run:

```bash
gofmt -w internal/registry/nuget
go test ./internal/registry/nuget
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/registry/nuget
git commit -m "Add NuGet resolver"
```

## Task 4: Wire NuGet Into Source Fetching

**Files:**
- Modify: `internal/source/source.go`
- Modify: `internal/source/git_fetcher.go`
- Modify: `internal/source/source_test.go`

- [ ] **Step 1: Write failing source tests**

In `internal/source/source_test.go`, add a NuGet cache-hit test:

```go
func TestEnsureCachedReturnsExistingNuGetPackageCacheEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/github.com/jamesnk/Newtonsoft.Json/13.0.3"
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(relativePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, filepath.FromSlash(relativePath), "README.md"), []byte("Json.NET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cache.WriteSources([]cache.PackageEntry{{
		Name:      "Newtonsoft.Json",
		Version:   "13.0.3",
		Registry:  string(registry.NuGet),
		Path:      relativePath,
		FetchedAt: "2026-05-19T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}

	got, err := EnsureCached("nuget:Newtonsoft.Json@13.0.3", Options{Fetcher: &fakeFetcher{}})
	if err != nil {
		t.Fatal(err)
	}
	if !got.FromCache || got.SourceLabel != "NuGet" {
		t.Fatalf("outcome = %#v", got)
	}
}
```

Add a GitRef propagation test near Git fetch tests:

```go
func TestFetchPackageWithGitUsesGitRefBeforeTag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	target := filepath.Join(home, "repos/github.com/owner/repo/1.0.0")

	oldCloneAtRef := cloneAtRef
	oldCloneAtTag := cloneAtTag
	cloneAtRef = func(repoURL, gotTarget, ref string) git.CloneResult {
		if repoURL != "https://github.com/owner/repo" || gotTarget != target || ref != "0123456789abcdef0123456789abcdef01234567" {
			t.Fatalf("clone args = %q %q %q", repoURL, gotTarget, ref)
		}
		if err := os.MkdirAll(gotTarget, 0o755); err != nil {
			t.Fatal(err)
		}
		return git.CloneResult{Success: true}
	}
	cloneAtTag = func(repoURL, target, version string) git.CloneResult {
		t.Fatal("cloneAtTag should not be called when GitRef is present")
		return git.CloneResult{}
	}
	t.Cleanup(func() {
		cloneAtRef = oldCloneAtRef
		cloneAtTag = oldCloneAtTag
	})

	got := FetchPackageWithGit(registry.ResolvedPackage{
		Registry: registry.NuGet,
		Name:     "Example.Package",
		Version:  "1.0.0",
		RepoURL:  "https://github.com/owner/repo",
		GitRef:   "0123456789abcdef0123456789abcdef01234567",
	})
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if !got.Success || got.Registry != registry.NuGet {
		t.Fatalf("result = %#v", got)
	}
}
```

- [ ] **Step 2: Run source tests and verify failure**

Run:

```bash
go test ./internal/source
```

Expected: FAIL because NuGet is not routed and `GitRef` is not used.

- [ ] **Step 3: Wire resolver and GitRef**

In `internal/source/source.go`, import NuGet:

```go
"repobridge/internal/registry/nuget"
```

Add switch case:

```go
case registry.NuGet:
	return nuget.Resolve(spec.Name, spec.Version, client, "")
```

In `internal/source/git_fetcher.go`, add function variables:

```go
var cloneAtCommit = git.CloneAtCommit
var cloneAtTagStrict = git.CloneAtTagStrict
```

Update `FetchPackageWithGit` clone selection:

```go
var clone git.CloneResult
switch {
case pkg.GitRef != "":
	clone = cloneAtCommit(pkg.RepoURL, target, pkg.GitRef)
case pkg.Registry == registry.NuGet:
	cloneRef := pkg.Version
	if pkg.GitTag != "" {
		cloneRef = strings.TrimPrefix(pkg.GitTag, "v")
	}
	clone = cloneAtTagStrict(pkg.RepoURL, target, cloneRef)
default:
	cloneRef := pkg.Version
	if pkg.GitTag != "" {
		cloneRef = strings.TrimPrefix(pkg.GitTag, "v")
	}
	clone = cloneAtTag(pkg.RepoURL, target, cloneRef)
}
if clone.Error != nil || !clone.Success {
	return FetchResult{Success: clone.Success, Warning: clone.Warning, Error: clone.Error}
}
```

- [ ] **Step 4: Run source tests**

Run:

```bash
gofmt -w internal/source
go test ./internal/source
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/source/source.go internal/source/git_fetcher.go internal/source/source_test.go
git commit -m "Wire NuGet source fetching"
```

## Task 5: Add CLI Clean Filter and Labels

**Files:**
- Modify: `internal/cli/commands.go`
- Modify: `internal/cli/commands_test.go`

- [ ] **Step 1: Write failing CLI tests**

Add to `internal/cli/commands_test.go`:

```go
func TestFetchDisplaysNuGetLabel(t *testing.T) {
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"nuget:Newtonsoft.Json@13.0.3": {
			Name:        "Newtonsoft.Json",
			Version:     "13.0.3",
			SourceLabel: "NuGet",
			Path:        "/cache/newtonsoft",
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

func TestCleanNuGetRegistryFilter(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{
		{Name: "zod", Version: "3.22.4", Registry: "npm", Path: "repos/github.com/colinhacks/zod/3.22.4"},
		{Name: "Newtonsoft.Json", Version: "13.0.3", Registry: "nuget", Path: "repos/github.com/jamesnk/Newtonsoft.Json/13.0.3"},
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
		t.Fatalf("NuGet package = %#v, want nil", nugetInfo)
	}
	npmInfo, err := cache.PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if npmInfo == nil {
		t.Fatal("npm package missing after NuGet clean")
	}
}
```

- [ ] **Step 2: Run CLI tests and verify failure**

Run:

```bash
go test ./internal/cli
```

Expected: FAIL because `--nuget` is unknown.

- [ ] **Step 3: Implement CLI filter**

In `internal/cli/commands.go`, add a variable in `newCleanCommand`:

```go
var nugetOnly bool
```

Pass it to selected registries:

```go
registries: selectedRegistries(npmOnly, pypiOnly, cratesOnly, mavenOnly, nugetOnly),
```

Add flag:

```go
cmd.Flags().BoolVar(&nugetOnly, "nuget", false, "only remove NuGet package sources")
```

Change function signature:

```go
func selectedRegistries(npmOnly, pypiOnly, cratesOnly, mavenOnly, nugetOnly bool) map[string]bool {
```

Add:

```go
if nugetOnly {
	registries[string(registry.NuGet)] = true
}
```

- [ ] **Step 4: Run CLI tests**

Run:

```bash
gofmt -w internal/cli
go test ./internal/cli
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/commands.go internal/cli/commands_test.go
git commit -m "Add NuGet CLI cache filter"
```

## Task 6: Documentation and Completion Note

**Files:**
- Modify: `README.md`
- Modify: `docs/features/00-feature-set-overview.md`
- Create: `docs/features/17-nuget-sources-done.md`

- [ ] **Step 1: Update README**

In `README.md`, update registry badge text from:

```html
<img alt="Registries" src="https://img.shields.io/badge/registries-npm%20%7C%20pypi%20%7C%20crates.io%20%7C%20Maven-2F855A">
```

to:

```html
<img alt="Registries" src="https://img.shields.io/badge/registries-npm%20%7C%20pypi%20%7C%20crates.io%20%7C%20Maven%20%7C%20NuGet-2F855A">
```

Change feature text to include NuGet:

```markdown
- Resolve package specs from npm, pypi, crates.io, maven, and NuGet.
```

Add quick-start examples:

```bash
repobridge path nuget:Newtonsoft.Json@13.0.3
repobridge path dotnet:Serilog@3.1.1
```

Add supported input row:

```markdown
| NuGet package | `nuget:Newtonsoft.Json`, `nuget:Newtonsoft.Json@13.0.3`, `dotnet:Serilog@3.1.1` |
```

Add NuGet behavior paragraph:

```markdown
NuGet inputs use package IDs with an optional explicit version. Without a version, RepoBridge selects the latest stable NuGet version. RepoBridge downloads the `.nupkg` only to read `.nuspec` repository metadata, then fetches the matching Git repository by commit or version tag. It does not cache package binaries as source.
```

Update clean flag note to include `--nuget`.

- [ ] **Step 2: Update feature overview**

In `docs/features/00-feature-set-overview.md`, change Feature 17 status from `Geplant` to `Fertig` after implementation is complete.

Add NuGet to supported inputs:

```markdown
| NuGet | `nuget:Newtonsoft.Json`, `nuget:Newtonsoft.Json@13.0.3`, `dotnet:Serilog@3.1.1` | `.nupkg` wird nur für `.nuspec`-Metadaten gelesen; Source kommt aus Git-Repository-Metadaten. |
```

Update CLI clean row to include `--nuget`.

Update tech stack supported package sources:

```markdown
| Unterstützte Paketquellen | npm, PyPI, crates.io, Maven Central, NuGet |
```

- [ ] **Step 3: Create done note**

Create `docs/features/17-nuget-sources-done.md`:

```markdown
# NuGet Sources Done

## Summary

RepoBridge now supports NuGet package inputs such as:

```bash
repobridge path nuget:Newtonsoft.Json@13.0.3
repobridge path dotnet:Serilog@3.1.1
```

The implementation adds NuGet registry detection, NuGet v3 service-index resolution, latest-stable version selection, temporary `.nupkg` download for `.nuspec` repository metadata, Git commit-first source fetching, strict version-tag fallback without default-branch cloning, NuGet cache indexing, `clean --nuget`, and README documentation.

## Deviations

- NuGet package binaries are not cached as source. The `.nupkg` is used only to read `.nuspec` metadata.
- Prerelease versions are selected only when explicitly requested.
- Default-branch fallback is disabled for NuGet to keep package source resolution tied to the selected package version.

## Open Questions And Technical Debt

- Custom/private NuGet feeds are not supported yet.
- Local .NET project and lockfile version detection is not implemented yet.
- `.snupkg` symbol package inspection is not implemented yet.
- NuGet package signatures and checksums are not verified yet.
```

- [ ] **Step 4: Run documentation grep**

Run:

```bash
rg -n "NuGet|nuget|dotnet" README.md docs/features/00-feature-set-overview.md docs/features/17-nuget-sources-done.md
```

Expected: Output includes NuGet examples and `clean --nuget`.

- [ ] **Step 5: Commit**

```bash
git add README.md docs/features/00-feature-set-overview.md docs/features/17-nuget-sources-done.md
git commit -m "Document NuGet source support"
```

## Task 7: Full Verification

**Files:**
- No new source files beyond prior tasks.

- [ ] **Step 1: Format all Go code**

Run:

```bash
gofmt -w ./cmd ./internal
```

Expected: no output.

- [ ] **Step 2: Run all tests**

Run:

```bash
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Run vet**

Run:

```bash
go vet ./...
```

Expected: no output and exit code 0.

- [ ] **Step 4: Build CLI**

Run:

```bash
go build -o ./bin/repobridge ./cmd/repobridge
```

Expected: binary created at `./bin/repobridge`.

- [ ] **Step 5: Manual smoke test with temporary cache**

Run:

```bash
tmp_home="$(mktemp -d)"
REPOBRIDGE_HOME="$tmp_home" ./bin/repobridge fetch --quiet nuget:Newtonsoft.Json@13.0.3
REPOBRIDGE_HOME="$tmp_home" ./bin/repobridge list
rm -rf "$tmp_home"
```

Expected: command exits 0 if NuGet metadata points to a cloneable commit/tag in the current network environment. If upstream metadata lacks a matching tag and no commit, record the exact error and use a package with commit metadata for the smoke test.

- [ ] **Step 6: Commit verification-only fixes if needed**

If verification exposed a necessary code or documentation correction, commit it:

```bash
git add <changed-files>
git commit -m "Stabilize NuGet source support"
```

If no fixes were needed, do not create an empty commit.
