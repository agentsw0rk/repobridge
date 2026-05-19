# Repository Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `internal/repoidentity` as the single boundary for repository parsing, display names, clone URLs, cache keys, and remove identities, then migrate source, CLI, and registry URL normalization to it.

**Architecture:** Introduce a small pure parser around `Repository{Host, ProjectPath}` and a resolver that calls a `MetadataProvider` only when a repo input has no explicit ref. Keep `remove` offline by using parser-only identity. Use current cache key/path behavior for compatibility and keep provider HTTP behavior delegated to the existing `internal/registry/repo` resolver during the first migration.

**Tech Stack:** Go 1.x, standard `testing`, Cobra CLI tests, existing `internal/cache`, `internal/source`, `internal/registry`, and `internal/registry/repo` packages.

---

## File Structure

- Create `internal/repoidentity/repoidentity.go`: domain types, parser, resolver, provider adapter, value methods.
- Create `internal/repoidentity/repoidentity_test.go`: boundary tests for parser, resolver, URL normalization, and offline remove.
- Modify `internal/source/source.go`: use `repoidentity.Resolver.ForCache` in repo fetch flow.
- Modify `internal/cli/commands.go`: use `repoidentity.Parser.ForRemove` in `removeSource`.
- Modify `internal/registry/repo_url.go`: delegate URL normalization to `repoidentity.DefaultParser().NormalizeURL`.
- Modify tests only where behavior moves to the new boundary and existing assertions need adapting.

---

### Task 1: Add Parser Boundary

**Files:**
- Create: `internal/repoidentity/repoidentity.go`
- Create: `internal/repoidentity/repoidentity_test.go`

- [ ] **Step 1: Write the failing parser boundary tests**

Create `internal/repoidentity/repoidentity_test.go`:

```go
package repoidentity

import "testing"

func TestParserParsesSupportedRepoInputs(t *testing.T) {
	parser := DefaultParser()
	tests := []struct {
		name        string
		input       string
		host        Host
		projectPath string
		ref         string
		displayName string
		repoURL     string
	}{
		{
			name:        "github shorthand with at ref",
			input:       "owner/repo@main",
			host:        GitHub,
			projectPath: "owner/repo",
			ref:         "main",
			displayName: "github.com/owner/repo",
			repoURL:     "https://github.com/owner/repo",
		},
		{
			name:        "github prefix with hash ref",
			input:       "github:owner/repo#main",
			host:        GitHub,
			projectPath: "owner/repo",
			ref:         "main",
			displayName: "github.com/owner/repo",
			repoURL:     "https://github.com/owner/repo",
		},
		{
			name:        "github tree url",
			input:       "https://github.com/owner/repo/tree/main",
			host:        GitHub,
			projectPath: "owner/repo",
			ref:         "main",
			displayName: "github.com/owner/repo",
			repoURL:     "https://github.com/owner/repo",
		},
		{
			name:        "gitlab nested project with at ref",
			input:       "gitlab.com/group/subgroup/project@v1",
			host:        GitLab,
			projectPath: "group/subgroup/project",
			ref:         "v1",
			displayName: "gitlab.com/group/subgroup/project",
			repoURL:     "https://gitlab.com/group/subgroup/project",
		},
		{
			name:        "gitlab nested tree url",
			input:       "https://gitlab.com/group/subgroup/project/-/tree/main",
			host:        GitLab,
			projectPath: "group/subgroup/project",
			ref:         "main",
			displayName: "gitlab.com/group/subgroup/project",
			repoURL:     "https://gitlab.com/group/subgroup/project",
		},
		{
			name:        "bitbucket prefix",
			input:       "bitbucket:team/project@release",
			host:        Bitbucket,
			projectPath: "team/project",
			ref:         "release",
			displayName: "bitbucket.org/team/project",
			repoURL:     "https://bitbucket.org/team/project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parser.Parse(tt.input)
			if !ok {
				t.Fatalf("Parse(%q) failed", tt.input)
			}
			if got.Host != tt.host || got.ProjectPath != tt.projectPath || got.ExplicitRef != tt.ref {
				t.Fatalf("Parse() = %#v", got)
			}
			if got.DisplayName() != tt.displayName {
				t.Fatalf("DisplayName() = %q, want %q", got.DisplayName(), tt.displayName)
			}
			if got.RepoURL() != tt.repoURL {
				t.Fatalf("RepoURL() = %q, want %q", got.RepoURL(), tt.repoURL)
			}
		})
	}
}

func TestParserRejectsPackageAndInvalidRepoInputs(t *testing.T) {
	parser := DefaultParser()
	for _, input := range []string{
		"",
		"@scope/pkg",
		"https://example.com/owner/repo",
		"https://github.com/owner/repo/issues/1",
		"https://github.com/owner/repo%2Fissues",
	} {
		t.Run(input, func(t *testing.T) {
			if got, ok := parser.Parse(input); ok {
				t.Fatalf("Parse(%q) = %#v, want rejected", input, got)
			}
		})
	}
}
```

- [ ] **Step 2: Run parser tests to verify they fail**

Run:

```bash
go test ./internal/repoidentity -run 'TestParser' -count=1
```

Expected: FAIL because package `internal/repoidentity` or symbols such as `DefaultParser` do not exist.

- [ ] **Step 3: Implement minimal parser and value methods**

Create `internal/repoidentity/repoidentity.go`:

```go
package repoidentity

import (
	"net/url"
	"path/filepath"
	"strings"

	"repobridge/internal/repobridge"
)

type Host string

const (
	GitHub    Host = "github.com"
	GitLab    Host = "gitlab.com"
	Bitbucket Host = "bitbucket.org"
)

type Repository struct {
	Host        Host
	ProjectPath string
}

type ParsedRepository struct {
	Repository
	ExplicitRef string
}

type Parser struct {
	DefaultHost Host
	Hosts       map[Host]struct{}
}

func DefaultParser() Parser {
	return Parser{
		DefaultHost: GitHub,
		Hosts: map[Host]struct{}{
			GitHub:    {},
			GitLab:    {},
			Bitbucket: {},
		},
	}
}

func (p Parser) Parse(input string) (ParsedRepository, bool) {
	input = strings.TrimSpace(input)
	if input == "" || strings.HasPrefix(input, "@") {
		return ParsedRepository{}, false
	}
	if strings.HasPrefix(strings.ToLower(input), "http://") || strings.HasPrefix(strings.ToLower(input), "https://") {
		return p.parseURL(input)
	}
	host := p.DefaultHost
	lower := strings.ToLower(input)
	for prefix, h := range map[string]Host{
		"github:":    GitHub,
		"gitlab:":    GitLab,
		"bitbucket:": Bitbucket,
	} {
		if strings.HasPrefix(lower, prefix) {
			host = h
			input = input[len(prefix):]
			break
		}
	}
	for h := range p.hosts() {
		prefix := string(h) + "/"
		if strings.HasPrefix(strings.ToLower(input), prefix) {
			host = h
			input = input[len(prefix):]
			break
		}
	}
	ref := ""
	if at := strings.Index(input, "@"); at > 0 {
		ref = input[at+1:]
		input = input[:at]
	} else if hash := strings.Index(input, "#"); hash > 0 {
		ref = input[hash+1:]
		input = input[:hash]
	}
	projectPath := strings.Trim(strings.TrimSuffix(input, ".git"), "/")
	if !p.validProjectPath(host, projectPath) {
		return ParsedRepository{}, false
	}
	return ParsedRepository{Repository: Repository{Host: host, ProjectPath: projectPath}, ExplicitRef: ref}, true
}

func (p Parser) parseURL(raw string) (ParsedRepository, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ParsedRepository{}, false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ParsedRepository{}, false
	}
	host := Host(strings.ToLower(parsed.Host))
	if _, ok := p.hosts()[host]; !ok {
		return ParsedRepository{}, false
	}
	escapedPath := parsed.EscapedPath()
	if strings.Contains(strings.ToLower(escapedPath), "%2f") {
		return ParsedRepository{}, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	projectPath, ref, ok := p.projectPathFromURL(host, parts)
	if !ok {
		return ParsedRepository{}, false
	}
	if parsed.Fragment != "" {
		ref = parsed.Fragment
	}
	return ParsedRepository{Repository: Repository{Host: host, ProjectPath: projectPath}, ExplicitRef: ref}, true
}

func (p Parser) projectPathFromURL(host Host, parts []string) (string, string, bool) {
	if len(parts) < 2 {
		return "", "", false
	}
	if host == GitLab {
		for i, part := range parts {
			if part == "" {
				return "", "", false
			}
			if part == "-" {
				if i < 2 || i+2 >= len(parts) || (parts[i+1] != "tree" && parts[i+1] != "blob") || parts[i+2] == "" {
					return "", "", false
				}
				projectPath := strings.TrimSuffix(strings.Join(parts[:i], "/"), ".git")
				return projectPath, parts[i+2], projectPath != ""
			}
		}
		projectPath := strings.TrimSuffix(strings.Join(parts, "/"), ".git")
		return projectPath, "", p.validProjectPath(host, projectPath)
	}
	repoName := strings.TrimSuffix(parts[1], ".git")
	if parts[0] == "" || repoName == "" {
		return "", "", false
	}
	if len(parts) == 2 {
		return parts[0] + "/" + repoName, "", true
	}
	if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") && parts[3] != "" {
		return parts[0] + "/" + repoName, parts[3], true
	}
	return "", "", false
}

func (p Parser) validProjectPath(host Host, projectPath string) bool {
	if _, ok := p.hosts()[host]; !ok {
		return false
	}
	parts := strings.Split(projectPath, "/")
	minParts := 2
	if len(parts) < minParts {
		return false
	}
	if host != GitLab && len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			return false
		}
	}
	return true
}

func (p Parser) hosts() map[Host]struct{} {
	if len(p.Hosts) > 0 {
		return p.Hosts
	}
	return DefaultParser().Hosts
}

func (r Repository) DisplayName() string {
	return string(r.Host) + "/" + r.ProjectPath
}

func (r Repository) RepoURL() string {
	return "https://" + r.DisplayName()
}

func (r Repository) CacheRootRelativePath() string {
	return filepath.ToSlash(filepath.Join("repos", filepath.FromSlash(r.DisplayName())))
}

func (p Parser) ForRemove(input string) (ParsedRepository, error) {
	parsed, ok := p.Parse(input)
	if !ok {
		return ParsedRepository{}, repobridge.InvalidRepoSpecError{Spec: input}
	}
	return parsed, nil
}
```

- [ ] **Step 4: Run parser tests to verify they pass**

Run:

```bash
go test ./internal/repoidentity -run 'TestParser' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit parser boundary**

Run:

```bash
git add internal/repoidentity/repoidentity.go internal/repoidentity/repoidentity_test.go
git commit -m "Add repository identity parser"
```

---

### Task 2: Add Resolver And Cache Identity

**Files:**
- Modify: `internal/repoidentity/repoidentity.go`
- Modify: `internal/repoidentity/repoidentity_test.go`

- [ ] **Step 1: Write failing resolver tests**

Update the import block in `internal/repoidentity/repoidentity_test.go` to include `context`, then append the following code after the existing parser tests:

```go
type recordingProvider struct {
	host  Host
	calls int
	meta  ProviderMetadata
	err   error
}

func (p *recordingProvider) Host() Host {
	return p.host
}

func (p *recordingProvider) LookupRepository(ctx context.Context, repo Repository) (ProviderMetadata, error) {
	p.calls++
	return p.meta, p.err
}

func TestResolverForCacheUsesExplicitRefWithoutProviderCall(t *testing.T) {
	provider := &recordingProvider{host: GitHub, meta: ProviderMetadata{DefaultBranch: "trunk"}}
	resolver := NewResolver(DefaultParser(), provider)

	got, err := resolver.ForCache(context.Background(), "owner/repo@main")
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
	if got.DisplayName() != "github.com/owner/repo" || got.GitRef != "main" || got.RepoURL != "https://github.com/owner/repo" {
		t.Fatalf("ForCache() = %#v", got)
	}
	if got.CacheKey().DisplayName != "github.com/owner/repo" || got.CacheKey().Version != "main" {
		t.Fatalf("CacheKey() = %#v", got.CacheKey())
	}
	if got.CacheRelativePath() != "repos/github.com/owner/repo/main" {
		t.Fatalf("CacheRelativePath() = %q", got.CacheRelativePath())
	}
}

func TestResolverForCacheResolvesMissingRefThroughProvider(t *testing.T) {
	provider := &recordingProvider{host: GitHub, meta: ProviderMetadata{DefaultBranch: "trunk"}}
	resolver := NewResolver(DefaultParser(), provider)

	got, err := resolver.ForCache(context.Background(), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if got.GitRef != "trunk" {
		t.Fatalf("GitRef = %q, want trunk", got.GitRef)
	}
}

func TestParserForRemoveStaysOfflineAndPreservesEmptyRef(t *testing.T) {
	parser := DefaultParser()
	got, err := parser.ForRemove("gitlab.com/group/subgroup/project")
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName() != "gitlab.com/group/subgroup/project" {
		t.Fatalf("DisplayName() = %q", got.DisplayName())
	}
	if got.ExplicitRef != "" {
		t.Fatalf("ExplicitRef = %q, want empty", got.ExplicitRef)
	}
}
```

- [ ] **Step 2: Run resolver tests to verify they fail**

Run:

```bash
go test ./internal/repoidentity -run 'TestResolver|TestParserForRemove' -count=1
```

Expected: FAIL because `ProviderMetadata`, `NewResolver`, `ForCache`, `CacheKey`, and `CacheRelativePath` are not implemented.

- [ ] **Step 3: Implement resolver, provider interface, and cache value methods**

Extend `internal/repoidentity/repoidentity.go`:

```go
import (
	"context"

	"repobridge/internal/cache"
	"repobridge/internal/repobridge"
)

type ResolvedRepository struct {
	Repository
	GitRef string
	RepoURL string
}

type ProviderMetadata struct {
	DefaultBranch        string
	CanonicalProjectPath string
	CloneURL             string
}

type MetadataProvider interface {
	Host() Host
	LookupRepository(ctx context.Context, repo Repository) (ProviderMetadata, error)
}

type Resolver struct {
	Parser      Parser
	Providers   map[Host]MetadataProvider
	FallbackRef string
}

func NewResolver(parser Parser, providers ...MetadataProvider) Resolver {
	if parser.DefaultHost == "" {
		parser = DefaultParser()
	}
	resolver := Resolver{
		Parser:      parser,
		Providers:   map[Host]MetadataProvider{},
		FallbackRef: "main",
	}
	for _, provider := range providers {
		if provider != nil {
			resolver.Providers[provider.Host()] = provider
		}
	}
	return resolver
}

func (r Resolver) Resolve(ctx context.Context, input string) (ResolvedRepository, error) {
	parsed, ok := r.Parser.Parse(input)
	if !ok {
		return ResolvedRepository{}, repobridge.InvalidRepoSpecError{Spec: input}
	}
	return r.ResolveParsed(ctx, parsed)
}

func (r Resolver) ForCache(ctx context.Context, input string) (ResolvedRepository, error) {
	return r.Resolve(ctx, input)
}

func (r Resolver) ResolveParsed(ctx context.Context, parsed ParsedRepository) (ResolvedRepository, error) {
	resolved := ResolvedRepository{
		Repository: parsed.Repository,
		GitRef:     parsed.ExplicitRef,
		RepoURL:    parsed.RepoURL(),
	}
	if resolved.GitRef != "" {
		return resolved, nil
	}
	provider := r.Providers[parsed.Host]
	if provider == nil {
		resolved.GitRef = firstNonEmpty(r.FallbackRef, "main")
		return resolved, nil
	}
	meta, err := provider.LookupRepository(ctx, parsed.Repository)
	if err != nil {
		return ResolvedRepository{}, err
	}
	if meta.CanonicalProjectPath != "" {
		resolved.ProjectPath = meta.CanonicalProjectPath
	}
	resolved.GitRef = firstNonEmpty(meta.DefaultBranch, r.FallbackRef, "main")
	resolved.RepoURL = firstNonEmpty(meta.CloneURL, resolved.Repository.RepoURL())
	return resolved, nil
}

func (r ResolvedRepository) CacheKey() cache.RepoKey {
	return cache.RepoKey{DisplayName: r.DisplayName(), Version: r.GitRef}
}

func (r ResolvedRepository) CacheRelativePath() string {
	return filepath.ToSlash(filepath.Join("repos", filepath.FromSlash(r.DisplayName()), filepath.FromSlash(r.GitRef)))
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

Merge the new imports into the existing import block instead of creating a second import block.

- [ ] **Step 4: Run resolver tests to verify they pass**

Run:

```bash
go test ./internal/repoidentity -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit resolver boundary**

Run:

```bash
git add internal/repoidentity/repoidentity.go internal/repoidentity/repoidentity_test.go
git commit -m "Add repository identity resolver"
```

---

### Task 3: Add Production Provider Adapter

**Files:**
- Modify: `internal/repoidentity/repoidentity.go`
- Modify: `internal/repoidentity/repoidentity_test.go`

- [ ] **Step 1: Write failing provider adapter test**

Append to `internal/repoidentity/repoidentity_test.go`:

```go
func TestRepoProviderWrapsExistingResolver(t *testing.T) {
	provider := RepoProvider{}

	gotHost := provider.Host()
	if gotHost != GitHub {
		t.Fatalf("Host() = %s, want %s", gotHost, GitHub)
	}
}
```

This test is intentionally small because existing provider HTTP behavior remains covered by `internal/registry/repo` tests. The adapter only needs to expose the production provider shape.

- [ ] **Step 2: Run adapter test to verify it fails**

Run:

```bash
go test ./internal/repoidentity -run TestRepoProviderWrapsExistingResolver -count=1
```

Expected: FAIL because `RepoProvider` does not exist.

- [ ] **Step 3: Implement production provider adapter**

Extend `internal/repoidentity/repoidentity.go`:

```go
import (
	"net/http"

	regrepo "repobridge/internal/registry/repo"
)

type RepoProvider struct {
	Client *http.Client
	HostValue Host
}

func (p RepoProvider) Host() Host {
	if p.HostValue != "" {
		return p.HostValue
	}
	return GitHub
}

func DefaultProviders(client *http.Client) []MetadataProvider {
	return []MetadataProvider{
		RepoProvider{Client: client, HostValue: GitHub},
		RepoProvider{Client: client, HostValue: GitLab},
		RepoProvider{Client: client, HostValue: Bitbucket},
	}
}

func (p RepoProvider) LookupRepository(ctx context.Context, identity Repository) (ProviderMetadata, error) {
	_ = ctx
	spec := regrepo.Spec{
		Host:  string(identity.Host),
		Owner: firstPathSegment(identity.ProjectPath),
		Repo:  remainingPath(identity.ProjectPath),
	}
	resolved, err := regrepo.Resolve(spec, p.Client)
	if err != nil {
		return ProviderMetadata{}, err
	}
	return ProviderMetadata{
		DefaultBranch: resolved.GitRef,
		CloneURL:      resolved.RepoURL,
	}, nil
}

func firstPathSegment(path string) string {
	parts := strings.SplitN(path, "/", 2)
	return parts[0]
}

func remainingPath(path string) string {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 1 {
		return ""
	}
	return parts[1]
}
```

Merge imports into the existing import block. Keep the adapter thin; do not reimplement provider HTTP behavior here.

- [ ] **Step 4: Run adapter tests**

Run:

```bash
go test ./internal/repoidentity -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit provider adapter**

Run:

```bash
git add internal/repoidentity/repoidentity.go internal/repoidentity/repoidentity_test.go
git commit -m "Add repository identity provider adapter"
```

---

### Task 4: Migrate Source Repo Fetch Flow

**Files:**
- Modify: `internal/source/source.go`
- Modify: `internal/source/source_test.go`

- [ ] **Step 1: Write failing source workflow test for GitLab nested repo**

Add to `internal/source/source_test.go` near the existing `EnsureCached` repo tests:

```go
func TestEnsureCachedFetchesGitLabNestedRepoWithExplicitRef(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/gitlab.com/group/subgroup/project/v1"
	fetcher := &fakeFetcher{
		repoResult: FetchResult{
			Package: "gitlab.com/group/subgroup/project",
			Version: "v1",
			Path:    relativePath,
			Success: true,
		},
	}

	got, err := EnsureCached("gitlab.com/group/subgroup/project@v1", Options{Fetcher: fetcher})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.repoCalls != 1 {
		t.Fatalf("repo fetch calls = %d, want 1", fetcher.repoCalls)
	}
	if got.Name != "gitlab.com/group/subgroup/project" || got.Version != "v1" {
		t.Fatalf("outcome = %#v", got)
	}
	if got.Path != filepath.Join(home, filepath.FromSlash(relativePath)) {
		t.Fatalf("Path = %q", got.Path)
	}
}
```

- [ ] **Step 2: Run the new source test to verify current behavior**

Run:

```bash
go test ./internal/source -run TestEnsureCachedFetchesGitLabNestedRepoWithExplicitRef -count=1
```

Expected: FAIL if existing flow disagrees on GitLab nested identity, or PASS if existing parser already handles this exact case. If it passes, keep the test as characterization and continue the migration because the goal is architectural consolidation.

- [ ] **Step 3: Update `ensureRepoCached` to use repoidentity**

In `internal/source/source.go`:

- Add import: `repobridge/internal/repoidentity`
- Remove direct import of `repobridge/internal/registry/repo` if no longer used.
- Replace the initial parse/resolve block in `ensureRepoCached` with:

```go
	resolver := repoidentity.NewResolver(repoidentity.DefaultParser(), repoidentity.DefaultProviders(opts.Client)...)
	resolvedRepo, err := resolver.ForCache(context.Background(), input)
	if err != nil {
		return Outcome{}, err
	}
	key := resolvedRepo.CacheKey()
```

- Use these values for cache lookup and fetch:

```go
	result := fetcher.FetchRepo(resolvedRepo.DisplayName(), resolvedRepo.RepoURL, resolvedRepo.GitRef)
```

- Use these values when normalizing empty fetch result fields:

```go
	if result.Package == "" {
		result.Package = resolvedRepo.DisplayName()
	}
	if result.Version == "" {
		result.Version = resolvedRepo.GitRef
	}
```

Ensure `context` is imported.

- [ ] **Step 4: Run source tests**

Run:

```bash
go test ./internal/source -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit source migration**

Run:

```bash
git add internal/source/source.go internal/source/source_test.go
git commit -m "Use repository identity in source fetch"
```

---

### Task 5: Migrate CLI Remove Flow

**Files:**
- Modify: `internal/cli/commands.go`
- Modify: `internal/cli/commands_test.go`

- [ ] **Step 1: Write failing CLI remove test for GitLab nested repo**

Add to `internal/cli/commands_test.go` near remove tests:

```go
func TestRemoveGitLabNestedRepoUsesRepositoryIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)
	relativePath := "repos/gitlab.com/group/subgroup/project/v1"
	if err := cache.WriteSources(nil, []cache.RepoEntry{{
		Name:      "gitlab.com/group/subgroup/project",
		Version:   "v1",
		Path:      relativePath,
		FetchedAt: "2026-05-18T12:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCommand(Options{Version: "test"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"remove", "gitlab.com/group/subgroup/project@v1"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(errOut.String(), "No cached source") {
		t.Fatalf("stderr = %q", errOut.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists or stat error = %v", err)
	}
}
```

If imports are missing, add `bytes`, `os`, `path/filepath`, and `strings` only if the file does not already import them.

- [ ] **Step 2: Run the new CLI test**

Run:

```bash
go test ./internal/cli -run TestRemoveGitLabNestedRepoUsesRepositoryIdentity -count=1
```

Expected: FAIL if current remove path cannot find nested GitLab identity, or PASS as characterization. Keep the test either way.

- [ ] **Step 3: Update `removeSource` to use repoidentity**

In `internal/cli/commands.go`:

- Add import: `repobridge/internal/repoidentity`
- Remove direct import of `repobridge/internal/registry/repo` if unused.
- Replace the repo branch inside `removeSource` with:

```go
	if registry.DetectInputType(spec) == registry.RepoInput {
		parsed, err := repoidentity.DefaultParser().ForRemove(spec)
		if err != nil {
			return false, err
		}
		result, err := store.Remove(cache.RemoveSelector{
			Kind: cache.RepoSource,
			Repo: cache.RepoSelector{
				DisplayName: parsed.DisplayName(),
				Version:     parsed.ExplicitRef,
			},
		})
		removed := result.Matched > 0
		if removed && err == nil {
			fmt.Fprintf(out, "Removed %s\n", parsed.DisplayName())
		}
		return removed, err
	}
```

- [ ] **Step 4: Run CLI tests**

Run:

```bash
go test ./internal/cli -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit CLI migration**

Run:

```bash
git add internal/cli/commands.go internal/cli/commands_test.go
git commit -m "Use repository identity for remove"
```

---

### Task 6: Delegate Registry URL Normalization

**Files:**
- Modify: `internal/registry/repo_url.go`
- Modify: `internal/registry/registry_test.go`

- [ ] **Step 1: Add registry test for existing URL normalization through new boundary**

In `internal/registry/registry_test.go`, add:

```go
func TestNormalizeRepoURLDelegatesGitLabNestedPathRules(t *testing.T) {
	got := NormalizeRepoURL("https://gitlab.com/group/subgroup/project/-/tree/main")
	want := "https://gitlab.com/group/subgroup/project"
	if got != want {
		t.Fatalf("NormalizeRepoURL() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run registry normalization tests**

Run:

```bash
go test ./internal/registry -run 'TestNormalizeRepoURL' -count=1
```

Expected: PASS before migration as characterization.

- [ ] **Step 3: Replace `NormalizeRepoURL` internals**

In `internal/registry/repo_url.go`, replace the implementation with:

```go
package registry

import "repobridge/internal/repoidentity"

func NormalizeRepoURL(value string) string {
	normalized, ok := repoidentity.DefaultParser().NormalizeURL(value)
	if !ok {
		return ""
	}
	return normalized
}
```

Then implement `NormalizeURL` in `internal/repoidentity/repoidentity.go`:

```go
func (p Parser) NormalizeURL(input string) (string, bool) {
	parsed, ok := p.Parse(input)
	if !ok {
		return "", false
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(input)), "http://") &&
		!strings.HasPrefix(strings.ToLower(strings.TrimSpace(input)), "https://") {
		return "", false
	}
	return parsed.RepoURL(), true
}
```

- [ ] **Step 4: Run registry and repoidentity tests**

Run:

```bash
go test ./internal/repoidentity ./internal/registry -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit registry normalization migration**

Run:

```bash
git add internal/registry/repo_url.go internal/registry/registry_test.go internal/repoidentity/repoidentity.go
git commit -m "Delegate repository URL normalization"
```

---

### Task 7: Final Verification And Cleanup

**Files:**
- Review: `internal/repoidentity/repoidentity.go`
- Review: `internal/source/source.go`
- Review: `internal/cli/commands.go`
- Review: `internal/registry/repo_url.go`

- [ ] **Step 1: Run formatting**

Run:

```bash
gofmt -w ./internal/repoidentity ./internal/source ./internal/cli ./internal/registry
```

Expected: command exits 0.

- [ ] **Step 2: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run vet**

Run:

```bash
go vet ./...
```

Expected: PASS.

- [ ] **Step 4: Inspect diff for unrelated edits**

Run:

```bash
git status --short
git diff --stat
```

Expected: only repository identity implementation and related tests are modified.

- [ ] **Step 5: Commit final formatting or cleanup if needed**

If Step 1 changed files not already committed, run:

```bash
git add internal/repoidentity internal/source internal/cli internal/registry
git commit -m "Format repository identity migration"
```

If Step 1 did not change files, do not create an empty commit.
