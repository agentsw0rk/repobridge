# Maven Effective Repositories Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve Maven source artifacts using the ordered effective Maven repositories from local project configuration instead of assuming a single Maven Central URL.

**Architecture:** Add a focused Maven repository configuration parser under `internal/registry/maven`, extend `registry.ResolvedPackage` with ordered Maven artifact candidates, and make `source.GitFetcher` try source JARs and POM SCM metadata in repository order. The built-in source resolver threads `cwd` into Maven resolution while keeping existing custom resolver tests compatible.

**Tech Stack:** Go XML decoding, Go `testing`, existing RepoBridge `source`, `registry`, and Maven resolver packages.

---

### Task 1: Maven Repository Configuration Parser

**Files:**
- Create: `internal/registry/maven/repositories.go`
- Test: `internal/registry/maven/repositories_test.go`

- [ ] **Step 1: Write failing repository parser tests**

Create tests that write temporary `pom.xml` and `.m2/settings.xml` files. Cover direct repositories, local parent repositories, active-by-default profiles, settings active profiles, exact mirrors, wildcard mirrors, `external:*`, exclusions, and Maven Central defaults.

- [ ] **Step 2: Run parser tests to verify they fail**

Run: `go test ./internal/registry/maven -run 'TestEffectiveRepositories|TestApplyMirrors'`

Expected: FAIL because parser types and functions do not exist yet.

- [ ] **Step 3: Implement parser**

Implement:

```go
type Repository struct {
    ID  string
    URL string
}

func EffectiveRepositories(cwd string) []Repository
func EffectiveRepositoriesWithSettings(cwd, settingsPath string) []Repository
```

The parser reads local POMs and settings. It applies mirrors after repository collection and preserves order.

- [ ] **Step 4: Run parser tests**

Run: `go test ./internal/registry/maven -run 'TestEffectiveRepositories|TestApplyMirrors'`

Expected: PASS.

### Task 2: Maven Resolver Candidate URLs

**Files:**
- Modify: `internal/registry/registry.go`
- Modify: `internal/registry/maven/maven.go`
- Modify: `internal/registry/maven/maven_test.go`

- [ ] **Step 1: Write failing resolver tests**

Add tests proving `ResolveWithRepositories` returns ordered source/POM candidates and keeps the legacy first URL fields populated.

- [ ] **Step 2: Run resolver tests to verify they fail**

Run: `go test ./internal/registry/maven -run 'TestResolve.*Repositories|TestResolveBuilds'`

Expected: FAIL because candidate fields and `ResolveWithRepositories` do not exist.

- [ ] **Step 3: Implement resolver candidate fields**

Add:

```go
type ArtifactCandidate struct {
    RepositoryID      string
    SourceArchiveURL  string
    SourceMetadataURL string
}
```

Add `ArtifactCandidates []ArtifactCandidate` to `registry.ResolvedPackage`. Have Maven resolution build candidates from ordered repositories.

- [ ] **Step 4: Run resolver tests**

Run: `go test ./internal/registry/maven`

Expected: PASS.

### Task 3: Source Resolver and Fetcher Order

**Files:**
- Modify: `internal/source/source.go`
- Modify: `internal/source/git_fetcher.go`
- Modify: `internal/source/source_test.go`

- [ ] **Step 1: Write failing source tests**

Add tests that:

- Resolve Maven with a `cwd` containing an internal repository and verify the internal URL is used before Central.
- Fetch Maven candidates where the first source JAR is 404 and the second succeeds.
- Fetch Maven candidates where all source JARs are 404 and SCM POM lookup happens in the same repository order.

- [ ] **Step 2: Run source tests to verify they fail**

Run: `go test ./internal/source -run 'Test.*Maven.*Repos|TestGitFetcher.*Candidate'`

Expected: FAIL because `cwd` is not used and the fetcher only knows one Maven URL.

- [ ] **Step 3: Implement source wiring and ordered fetch**

Add a package-level helper for built-in resolution that accepts `cwd`. For Maven, call `maven.EffectiveRepositories(cwd)` and pass the result into `maven.ResolveWithRepositories`. In `GitFetcher`, iterate `ArtifactCandidates` for source JARs, then POM SCM lookup.

- [ ] **Step 4: Run source tests**

Run: `go test ./internal/source`

Expected: PASS.

### Task 4: Documentation and Completion Note

**Files:**
- Modify: `README.md`
- Add: `docs/features/29-custom-registry-urls-done.md`

- [ ] **Step 1: Update README**

Document that Maven resolution reads local Maven repository configuration from `--cwd`, preserves order, applies supported mirrors, and still has explicit limitations.

- [ ] **Step 2: Add done file**

Record implementation summary, deviations from the broader issue, and follow-up work for credentials, remote parents, full Maven model building, and other package managers.

- [ ] **Step 3: Run full verification**

Run:

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
```

Expected: all commands exit 0.
