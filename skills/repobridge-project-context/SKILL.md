---
name: repobridge-project-context
description: Use when an AI agent needs real framework or library source code as read-only reference context for a project. Scans project manifests, lockfiles, and imports, proposes RepoBridge specs, fetches relevant sources with repobridge, and uses the resolved paths for code reading, symbol investigation, and rg/grep-style searches.
---

# RepoBridge Project Context

Use this skill to enrich an agent's local context with source code for the frameworks and libraries a project actually uses. The goal is not to edit dependency code. The goal is to resolve dependency sources into stable local paths and use them as reference material while working on the project.

## Workflow

1. **Find the project root.** Use the current working directory unless the user gives another path.
2. **Ensure RepoBridge is available.** Run `command -v repobridge`. If it is missing, install the pinned GitHub release using the platform-specific asset table below.
3. **Scan dependencies.** Run RepoBridge's project scanner:

   ```bash
   repobridge scan --cwd . --json
   ```

4. **Review the proposed specs.** Prefer the highest-confidence specs from manifests and lockfiles. Use import-derived specs as supporting evidence.
5. **Fetch only useful references.** Do not blindly fetch every transitive dependency. Prioritize frameworks, runtimes, SDKs, ORMs, test frameworks, UI libraries, and libraries related to the task.
6. **Fetch sources with RepoBridge.**

   ```bash
   repobridge scan --cwd <project-root> --fetch --limit 10
   repobridge path --cwd <project-root> <spec>
   ```

7. **Use resolved paths as read-only references.** Search and read them with tools such as `rg`, file reads, and LSP navigation when available:

   ```bash
   rg "createRoot|useEffect" "$(repobridge path --cwd <project-root> react)"
   ```

8. **State what was fetched.** In the final response, mention which frameworks/libraries were resolved and which paths were used when that matters for the task.

## Installing RepoBridge from GitHub Releases

Use this only when `command -v repobridge` fails. Prefer an existing installed binary when available.

Use the pinned release version `v0.2.0`. Pick the asset for the current platform:

| Platform | Asset |
| --- | --- |
| macOS Apple Silicon | `repobridge_v0.2.0_darwin_arm64.tar.gz` |
| macOS Intel | `repobridge_v0.2.0_darwin_amd64.tar.gz` |
| Linux x86_64 | `repobridge_v0.2.0_linux_amd64.tar.gz` |
| Linux arm64 | `repobridge_v0.2.0_linux_arm64.tar.gz` |
| Windows x86_64 | `repobridge_v0.2.0_windows_amd64.zip` |

Download the selected asset from:

```text
https://github.com/ArekCzarnik/RepoBridge/releases/download/v0.2.0/<asset>
```

For macOS/Linux, extract the tarball, copy `repobridge` to a directory on `PATH` such as `$HOME/.local/bin`, make it executable, and verify with `repobridge --version`.

For Windows, extract `repobridge.exe` from the zip file, add its directory to `PATH`, and verify with `repobridge --version`.

## Selection Rules

- Fetch direct dependencies before transitive dependencies.
- Use versions from lockfiles/manifests when available.
- For npm, pass `--cwd <project-root>` so RepoBridge can use local version detection.
- Treat unknown import aliases, workspace packages, relative imports, and standard libraries as project-local or built-in unless a manifest confirms an external package.
- Skip secrets and private registry configuration. Never print tokens.
- Keep dependency source directories read-only. Do not modify cached dependency sources unless the user explicitly asks to inspect or patch a vendored copy.

## Scan Output

`repobridge scan --json` prints JSON with:

- `candidates`: deduplicated RepoBridge specs sorted by confidence.
- `warnings`: files that could not be parsed or ambiguous detections.

Each spec has:

- `spec`: value to pass to RepoBridge.
- `ecosystem`: npm, pypi, go, crates, maven, nuget, or unknown.
- `confidence`: higher means safer to fetch.
- `reasons`: why the spec was proposed.

## Supported Detection

The scanner detects common direct dependencies from:

- JavaScript/TypeScript: `package.json`, `package-lock.json`, imports/requires.
- Python: `requirements.txt`.
- Go: `go.mod`, imports.
- Rust: `Cargo.toml`.
- JVM: `pom.xml`.
- .NET: `.csproj`.

Detection is conservative. If the scanner cannot infer a reliable RepoBridge spec, inspect the manifest manually and decide whether fetching that dependency is worth it.

## Typical Use

For a React project:

```bash
repobridge scan --cwd /path/to/app --json
repobridge scan --cwd /path/to/app --fetch --limit 8
REACT_PATH="$(repobridge path --cwd /path/to/app react)"
rg "useSyncExternalStore" "$REACT_PATH"
```

For a mixed backend project:

```bash
repobridge scan --cwd /path/to/service
repobridge fetch --cwd /path/to/service pypi:fastapi maven:org.springframework:spring-core@6.1.0
```

## Failure Handling

- If `repobridge` is not installed, download the pinned `v0.2.0` asset for the current OS/architecture, install it into a local bin directory, and verify `repobridge --version`.
- If a proposed spec fails, continue with the remaining specs and report the failure.
- If too many dependencies are detected, narrow to the libraries relevant to the user's current task.
- If private repositories fail, ask the user to provide the appropriate token through `GITHUB_TOKEN`, `GITLAB_TOKEN`, or `BITBUCKET_TOKEN`.
