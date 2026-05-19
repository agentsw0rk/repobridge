---
layout: default
title: RepoBridge Documentation
description: Product and functional documentation for RepoBridge
---

RepoBridge is a Go CLI that resolves package and repository specifications into stable local source-code paths. It is built for coding agents, developer tools, and developers who need quick local access to the source code behind a dependency.

## What Problem Does RepoBridge Solve?

Many development workflows need more than package names. They need the source code behind a dependency. RepoBridge handles package-registry and repository-host resolution, downloads the matching sources, and stores them in a reusable local cache.

RepoBridge is useful when you need to:

- Find source code for npm, pypi, crates.io, maven, and nuget packages.
- Bring GitHub, GitLab, and Bitbucket repositories into stable local paths.
- Provide coding agents with machine-friendly path output.
- Speed up repeated tool runs by reusing already cached sources.
- Detect local npm versions from `node_modules`, lockfiles, and `package.json`.

## Core Features

### Fetch Sources for Packages

RepoBridge accepts package specifications and resolves them to the matching source location. On a cache miss, it fetches the source. On a cache hit, it reuses the existing local path.

Examples:

```bash
repobridge path react
repobridge path react@19.0.0
repobridge path pypi:requests==2.32.3
repobridge path crates:serde@1.0.217
repobridge path maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0
repobridge path nuget:Newtonsoft.Json@13.0.3
```

When no npm version is provided, RepoBridge first tries to detect the locally installed version. It supports `node_modules`, `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, and `package.json`.

### Fetch Sources for Repositories

RepoBridge also accepts repository specifications. If no explicit reference is provided, RepoBridge resolves the repository host's default branch.

Examples:

```bash
repobridge path vercel/next.js
repobridge path github.com/vercel/next.js
repobridge path gitlab.com/group/project
repobridge path bitbucket.org/workspace/repo
repobridge path https://github.com/vercel/next.js/tree/canary
```

### Preload the Cache

Use `fetch` to load one or more sources into the cache ahead of time. This is useful when an agent or CI workflow should later work only with local paths.

```bash
repobridge fetch react@19.0.0 vercel/next.js
repobridge fetch --quiet pypi:requests==2.32.3
```

### Print Paths for Automation

The `path` command is designed for downstream tools. It prints one absolute path to the local source copy for each input.

```bash
SOURCE_PATH="$(repobridge path react)"
ls "$SOURCE_PATH"
```

Use `--verbose` to show additional fetch information.

```bash
repobridge path --verbose react
```

### Inspect and Clean the Cache

RepoBridge stores fetched sources under `REPOBRIDGE_HOME`, or under `~/.repobridge` when the variable is not set. The cache contains source-code snapshots and a `sources.json` index.

```bash
repobridge list
repobridge list --json
repobridge remove react
repobridge remove github.com/vercel/next.js
repobridge clean
```

`clean` can be limited to source types or registries:

```bash
repobridge clean --packages
repobridge clean --repos
repobridge clean --npm
repobridge clean --pypi
repobridge clean --crates
repobridge clean --maven
repobridge clean --nuget
```

## Supported Inputs

| Type | Examples | Behavior |
| --- | --- | --- |
| npm | `react`, `react@19.0.0`, `@scope/pkg@1.2.3` | The default registry when no prefix is used. Without a version, RepoBridge uses a locally detected version or registry resolution. |
| pypi | `pypi:requests`, `pypi:requests==2.32.3`, `python:requests@2.32.3` | Package metadata is read from pypi and normalized to a repository source. |
| crates.io | `crates:serde`, `cargo:serde@1.0.217`, `rust:serde@1.0.217` | RepoBridge uses crates.io metadata and matching Git references. |
| maven | `maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0`, `java:group:artifact@1.0.0` | RepoBridge prefers `*-sources.jar`; if no source JAR exists, it tries SCM metadata fallback. |
| nuget | `nuget:Newtonsoft.Json`, `nuget:Newtonsoft.Json@13.0.3`, `dotnet:Serilog@3.1.1` | RepoBridge reads `.nuspec` repository metadata from the package and fetches the matching Git source. |
| GitHub | `vercel/next.js`, `github:vercel/next.js`, `github.com/vercel/next.js` | Without a ref, the default branch is resolved through the host API. |
| GitLab | `gitlab:group/project`, `gitlab.com/group/subgroup/project` | Project paths with subgroups are supported. |
| Bitbucket | `bitbucket:workspace/repo`, `bitbucket.org/workspace/repo` | Without a ref, the default branch is resolved through the Bitbucket API. |
| URL | `https://github.com/vercel/next.js/tree/canary` | Supported host URLs are normalized; tree refs are detected. |

## CLI Reference

| Command | Purpose | Important Options |
| --- | --- | --- |
| `repobridge path <spec...>` | Fetches sources when needed and prints absolute local paths. | `--cwd`, `--verbose` |
| `repobridge fetch <spec...>` | Loads sources into the cache without using paths as the primary output. | `--cwd`, `--quiet` |
| `repobridge list` | Lists cached packages and repositories. | `--json` |
| `repobridge remove <spec...>` | Removes selected cache entries. | Alias: `rm` |
| `repobridge clean` | Removes cache entries by type or registry. | `--packages`, `--repos`, `--npm`, `--pypi`, `--crates`, `--maven`, `--nuget` |

## Installation and Requirements

RepoBridge requires Go 1.22 or newer and `git` on the `PATH`.

Install from the repository:

```bash
go install ./cmd/repobridge
```

Build a local binary:

```bash
go build -o ./bin/repobridge ./cmd/repobridge
./bin/repobridge --version
```

## Configuration

| Variable | Meaning |
| --- | --- |
| `REPOBRIDGE_HOME` | Cache directory. Defaults to `~/.repobridge`. |
| `GITHUB_TOKEN` | Token for GitHub API calls and private GitHub repositories. |
| `GITLAB_TOKEN` | Token for private GitLab repositories. |
| `BITBUCKET_TOKEN` | Token for private Bitbucket repositories. |

Tokens should be provided through the environment only. They do not belong in source code, logs, or cache contents.

## Common Workflows

### Analyze the Source Code of a Dependency

1. Provide the package:

   ```bash
   repobridge path react@19.0.0
   ```

2. Use the printed path in an editor, agent, or analysis tool.
3. Later runs reuse the same cache entry.

### Prepare Sources for an Agent Run

1. Preload all required sources:

   ```bash
   repobridge fetch react@19.0.0 pypi:requests==2.32.3 vercel/next.js
   ```

2. During the agent run, call only `repobridge path <spec>`.
3. The agent receives stable local paths instead of changing remote URLs.

### Isolate the Cache Before Risky Tests

1. Set a temporary cache directory:

   ```bash
   export REPOBRIDGE_HOME="$(mktemp -d)"
   ```

2. Run commands:

   ```bash
   repobridge fetch react
   repobridge clean --npm
   ```

3. Remove the temporary directory after the test run.

## Current Limits

RepoBridge currently documents and supports the public registry and repository sources exposed by the CLI. Some extensions are not available yet:

- Custom or private registry URLs for npm, pypi, crates.io, maven, and nuget.
- Checksum and signature verification for downloaded archives.
- maven snapshot metadata and automatic latest-version resolution for maven.
- Local version detection for Gradle, maven, and .NET projects.
- JSON output for `path` and `fetch`.
- Configuration file support for default registries or cache policies.

## Troubleshooting

### `git` Is Not Found

**Symptom:** Fetching repository sources fails.

**Solution:**

1. Check whether `git` is installed:

   ```bash
   git --version
   ```

2. Make sure `git` is available on the `PATH`.
3. Run the RepoBridge command again.

### API Limits or Private Repositories

**Symptom:** Repository resolution fails for GitHub, GitLab, or Bitbucket.

**Solution:**

1. Set the matching token as an environment variable:

   ```bash
   export GITHUB_TOKEN="..."
   export GITLAB_TOKEN="..."
   export BITBUCKET_TOKEN="..."
   ```

2. Run the command again.
3. Do not put tokens in shell history, logs, or project files.

### The Wrong npm Version Is Loaded

**Symptom:** `repobridge path react` does not use the expected project version.

**Solution:**

1. Pass the project directory with `--cwd`:

   ```bash
   repobridge path --cwd /path/to/project react
   ```

2. Check whether `node_modules`, `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, or `package.json` contains the expected version.
3. If a specific version is required, provide it explicitly:

   ```bash
   repobridge path react@19.0.0
   ```

### The Cache Needs Targeted Cleanup

**Symptom:** An old or no longer needed source copy remains in the cache.

**Solution:**

1. Inspect cache contents:

   ```bash
   repobridge list
   ```

2. Remove a single source:

   ```bash
   repobridge remove react
   ```

3. Or clean entries by type or registry:

   ```bash
   repobridge clean --repos
   repobridge clean --nuget
   ```
