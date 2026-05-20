<p align="center">
  <img src="logo.png" alt="RepoBridge logo" width="560">
</p>

<h1 align="center">RepoBridge</h1>

<p align="center">
  <strong>Fetch package and repository source code into stable local paths for coding agents and developer tooling.</strong>
</p>

<p align="center">
  <a href="https://github.com/arkadiuszczarnik/repobridge"><img alt="Repository" src="https://img.shields.io/badge/github-repobridge-181717?logo=github"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white">
  <img alt="Cobra" src="https://img.shields.io/badge/Cobra-1.9.1-6F42C1">
  <img alt="Registries" src="https://img.shields.io/badge/registries-npm%20%7C%20pypi%20%7C%20crates.io%20%7C%20Maven%20%7C%20NuGet-2F855A">
  <img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-blue">
</p>

## Introduction

`repobridge` is a small Go CLI for turning package or repository specs into local source trees and searchable AST code graphs. It supports npm, pypi, crates.io, maven, nuget, and common git repository hosts.

## Features

- Resolve package specs from npm, pypi, crates.io, maven, and nuget.
- Fetch Git repositories from GitHub, GitLab, and Bitbucket.
- Scan a project for dependency source specs from manifests, lockfiles, and imports.
- Build local Tree-sitter AST graphs for cached sources and search them by symbol, kind, path, language, and calls.
- Index Spring Java/Kotlin routes and handlers as graph nodes, including HTTP method, route pattern, file, and line.
- Inspect cached code graphs with `status`, `files`, and `node` without scanning source trees again.
- Traverse call graphs with `callers`, `callees`, and `impact` for focused agent investigations.
- Build task-oriented agent context with `context` and broader graph explanations with `explore`.
- Reuse a stable local cache across repeated agent/tool runs.
- Detect installed npm package versions from `node_modules`, lockfiles, and `package.json`.
- Print machine-friendly paths for downstream automation.

The E2E benchmark compares `repobridge search` with comparable `rg -C 3` output on a pinned Kotlin repository. The current run shows that structured AST search returns much smaller outputs for LLM context while preserving code-level intent such as function and call matching.

| Task | Results | RepoBridge tokens | rg tokens | Estimated reduction |
| --- | ---: | ---: | ---: | ---: |
| Find Kotlin up function | 2 | 224 | 444 | 49.5% |
| Find functions calling exec | 9 | 928 | 2575 | 64.0% |
| Find functions calling ps | 3 | 320 | 1310 | 75.6% |
| Find docker compose resolver | 1 | 96 | 576 | 83.3% |

## Requirements

- Go 1.22 or newer
- `git` available on `PATH`

## Installation

Build or install from the repository root:

```bash
go install ./cmd/repobridge
```

For a local binary:

```bash
go build -o ./bin/repobridge ./cmd/repobridge
./bin/repobridge --version
```

## Quick Start

Fetch source and print its cache path:

```bash
repobridge path react
repobridge path pypi:requests==2.32.3
repobridge path crates:serde@1.0.217
repobridge path maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0
repobridge path nuget:Newtonsoft.Json@13.0.3
repobridge path dotnet:Serilog@3.1.1
repobridge path github.com/vercel/next.js
```

Use `fetch` when you only need to populate the cache:

```bash
repobridge fetch react@19.0.0 vercel/next.js
```

Scan a project for dependencies that can be fetched as source references:

```bash
repobridge scan --cwd .
repobridge scan --cwd . --json
repobridge scan --cwd . --fetch --limit 10
```

Search cached source graphs:

```bash
repobridge search react@19.0.0 "kind:function name:render"
repobridge search pypi:requests==2.32.3 "calls:send lang:python"
repobridge search maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0 "lang:kotlin kind:function"
repobridge search github.com/vercel/next.js "path:packages kind:method"
repobridge search github.com/acme/service "kind:route path:/login"
```

Inspect graph health, indexed files, and exact nodes:

```bash
repobridge status react@19.0.0
repobridge files react@19.0.0 --path packages/react-dom --limit 10
repobridge node react@19.0.0 createRoot --source-lines 20
```

Trace call relationships:

```bash
repobridge callers react@19.0.0 createRoot --depth 2
repobridge callers github.com/acme/service AuthController.login --depth 1
repobridge callees react@19.0.0 createRoot --include-unresolved
repobridge impact react@19.0.0 createRoot --json
```

Build task context for an agent:

```bash
repobridge context react@19.0.0 "createRoot render flow" --budget small
repobridge context github.com/acme/service "POST /login" --budget small
repobridge explore github.com/vercel/next.js "AppRouter cache invalidation" --budget large --depth 2
```

Inspect and clean cached sources:

```bash
repobridge list
repobridge list --json
repobridge remove react
repobridge clean --repos
```

## Supported Inputs

| Input | Example |
| --- | --- |
| npm package | `react`, `react@19.0.0`, `@scope/package@1.2.3` |
| pypi package | `pypi:requests`, `pypi:requests==2.32.3` |
| crates.io package | `crates:serde`, `crates:serde@1.0.217` |
| maven artifact | `maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0` |
| NuGet package | `nuget:Newtonsoft.Json`, `nuget:Newtonsoft.Json@13.0.3`, `dotnet:Serilog@3.1.1` |
| GitHub shorthand | `vercel/next.js` |
| Repository host | `github.com/vercel/next.js`, `gitlab.com/group/project` |
| Full URL | `https://github.com/vercel/next.js` |

Package inputs default to npm. Use a registry prefix for non-npm packages.

Maven inputs use explicit `groupId:artifactId@version` coordinates. RepoBridge downloads the published `*-sources.jar` from maven first; when no source JAR exists, it tries to clone a Git repository from SCM metadata in the artifact POM.

NuGet inputs use package IDs with an optional explicit version. Without a version, RepoBridge selects the latest stable NuGet version. RepoBridge downloads the `.nupkg` only to read `.nuspec` repository metadata, then fetches the matching Git repository by commit or version tag. It does not cache package binaries as source.

AST codegraph indexing currently parses Go, Java, Kotlin, C#, JavaScript, TypeScript, Python, and Rust sources with Tree-sitter. Java and Kotlin parsing also recognizes Spring `@RequestMapping`, `@GetMapping`, `@PostMapping`, `@PutMapping`, `@DeleteMapping`, and `@PatchMapping` routes, stores `route` and `handler` nodes, and links them with `handles` edges. Indexing runs in the background after successful `path`, `fetch`, and `scan --fetch` commands. `search`, graph inspection, callgraph, `context`, and `explore` rebuild a missing or stale graph synchronously before returning results unless `--no-sync-index` is set.

## Commands

| Command | Description |
| --- | --- |
| `repobridge fetch <spec...>` | Downloads sources into the cache. |
| `repobridge path <spec...>` | Fetches on cache miss and prints absolute source paths. |
| `repobridge scan` | Scans a project and proposes dependency source specs. |
| `repobridge search <spec> <query>` | Searches the local AST graph for a cached source, including framework routes such as `kind:route path:/login`, building the graph synchronously if needed. |
| `repobridge status <spec>` | Shows graph path, freshness status, schema version, file/node/edge counts, and warnings. |
| `repobridge files <spec>` | Lists files stored in the AST graph without walking the source tree again. |
| `repobridge node <spec> <id-or-name>` | Shows one symbol's kind, qualified name, location, calls, and optional source lines. |
| `repobridge callers <spec> <symbol>` | Finds functions, methods, or routes that call or handle a symbol. |
| `repobridge callees <spec> <symbol>` | Finds functions or methods called by a symbol. |
| `repobridge impact <spec> <symbol>` | Traverses incoming call/import relationships to estimate change impact. |
| `repobridge context <spec> <query>` | Returns focused task context with entry points, relationships, snippets, related files, warnings, and stats. |
| `repobridge explore <spec> <query>` | Returns broader graph exploration context with the same bounded output shape. |
| `repobridge list [--json]` | Lists cached packages and repositories. |
| `repobridge remove <spec...>` | Removes selected cached sources. |
| `repobridge clean` | Removes cached sources, optionally scoped by flags. |

Most commands that resolve package versions accept `--cwd` for lockfile detection. `fetch` also accepts `--quiet`; `path` accepts `--verbose`; `scan` accepts `--json`, `--fetch`, `--limit`, and `--no-imports`; `search` accepts `--json`, `--limit`, `--kind`, `--lang`, `--path`, `--calls`, and `--no-sync-index`; `status`, `files`, `node`, `callers`, `callees`, `impact`, `context`, and `explore` accept `--json` and `--no-sync-index`; `files` adds `--path` and `--limit`; `node` adds `--source-lines`; callgraph commands add `--depth`, `--kind`, `--lang`, `--path`, `--limit`, and `--include-unresolved`; `context` and `explore` add `--budget`, `--limit`, and `--depth`; `clean` accepts filters such as `--packages`, `--repos`, `--npm`, `--pypi`, `--crates`, `--maven`, and `--nuget`.

## Configuration

| Variable | Description |
| --- | --- |
| `REPOBRIDGE_HOME` | Cache directory. Defaults to `~/.repobridge`. |
| `GITHUB_TOKEN` | Token for GitHub API calls and private GitHub repositories. |
| `GITLAB_TOKEN` | Token for private GitLab repositories. |
| `BITBUCKET_TOKEN` | Token for private Bitbucket repositories. |

The cache contains cloned source trees and a `sources.json` index under `REPOBRIDGE_HOME`. Repository fetches remove `.git` so the cache stores source snapshots rather than nested working trees.

After successful `path`, `fetch`, and `scan --fetch` calls, RepoBridge starts background AST indexing for the cached source. Graph data is stored beside the source in `.repobridge-graph/`; command output remains unchanged. The first `search`, `status`, `files`, `node`, `callers`, `callees`, `impact`, `context`, or `explore` call builds a missing or stale graph synchronously unless `--no-sync-index` is set.

## Development

Run the CLI from source:

```bash
go run ./cmd/repobridge --version
```

Run checks before submitting changes:

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
```

Tests are colocated with implementation files as `*_test.go`. Lockfile fixtures live in `internal/lockfile/testdata/`.

Project scanning also has an opt-in end-to-end test that clones pinned public GitHub projects into the ignored `e2e/` directory and verifies scanner output across Maven, Gradle, NuGet, JavaScript, PyPI, and crates projects:

```bash
go test -tags=e2e ./test/e2e-scan -count=1
```

Codegraph search has an opt-in end-to-end test that checks the real `repobridge search` CLI against a pinned Kotlin repository and writes a Markdown output-budget report comparing `repobridge search` with `rg -C 3`:

```bash
go test -tags=e2e ./test/e2e-search -count=1 -v
```

The report is written to `e2e/search/results/search-output-budget.md`.

Use verbose mode to see each repository, commit, sparse checkout path, expected specs, and scanner results:

```bash
go test -tags=e2e ./test/e2e-scan -count=1 -v
```

Set `REPOBRIDGE_E2E_REFRESH=1` to delete and re-fetch the local E2E checkouts.

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
