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

## Description

`repobridge` is a small Go CLI for turning package or repository specs into local source trees. It supports npm, pypi, crates.io, maven, nuget, and common Git repository hosts.

## Features

- Resolve package specs from npm, pypi, crates.io, maven, and nuget.
- Fetch Git repositories from GitHub, GitLab, and Bitbucket.
- Reuse a stable local cache across repeated agent/tool runs.
- Detect installed npm package versions from `node_modules`, lockfiles, and `package.json`.
- Print machine-friendly paths for downstream automation.

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

## Commands

| Command | Description |
| --- | --- |
| `repobridge fetch <spec...>` | Downloads sources into the cache. |
| `repobridge path <spec...>` | Fetches on cache miss and prints absolute source paths. |
| `repobridge list [--json]` | Lists cached packages and repositories. |
| `repobridge remove <spec...>` | Removes selected cached sources. |
| `repobridge clean` | Removes cached sources, optionally scoped by flags. |

Most commands that resolve package versions accept `--cwd` for lockfile detection. `fetch` also accepts `--quiet`; `path` accepts `--verbose`; `clean` accepts filters such as `--packages`, `--repos`, `--npm`, `--pypi`, `--crates`, `--maven`, and `--nuget`.

## Configuration

| Variable | Description |
| --- | --- |
| `REPOBRIDGE_HOME` | Cache directory. Defaults to `~/.repobridge`. |
| `GITHUB_TOKEN` | Token for GitHub API calls and private GitHub repositories. |
| `GITLAB_TOKEN` | Token for private GitLab repositories. |
| `BITBUCKET_TOKEN` | Token for private Bitbucket repositories. |

The cache contains cloned source trees and a `sources.json` index under `REPOBRIDGE_HOME`. Repository fetches remove `.git` so the cache stores source snapshots rather than nested working trees.

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

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
