---
name: repobridge-project-context
description: Use when an AI agent needs real framework or library source code as read-only reference context for a project. Scans project manifests, lockfiles, and imports, proposes RepoBridge specs, fetches relevant sources with repobridge, and uses AST codegraph search, resolved paths, file reads, and focused fallback rg searches for investigation.
---

# RepoBridge Project Context

Use this skill to enrich an agent's local context with source code for the frameworks and libraries a project actually uses. The goal is not to edit dependency code. The goal is to resolve dependency sources into stable local paths and use them as reference material while working on the project.

## Workflow

1. **Find the project root.** Use the current working directory unless the user gives another path.
2. **Ensure RepoBridge is available.** Run `command -v repobridge`. If it is missing, install the pinned GitHub release using the platform-specific asset table below. If AST search is needed, also verify `repobridge search --help`; when unavailable, use a newer RepoBridge build that includes codegraph search.
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

7. **Search resolved source graphs first.** Prefer `repobridge search` for symbol-, language-, path-, and call-aware investigation because it returns compact AST results instead of broad text snippets:

   ```bash
   repobridge search --cwd <project-root> react "kind:function calls:createRoot"
   repobridge search --cwd <project-root> maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0 "lang:kotlin kind:function"
   ```

8. **Use resolved paths as read-only references when needed.** Read files directly, use LSP navigation when available, and use `rg` only as a fallback for text that is not represented in the AST graph.
9. **State what was fetched and searched.** In the final response, mention which frameworks/libraries were resolved and which search queries or paths were used when that matters for the task.

## Installing RepoBridge from GitHub Releases

Use this only when `command -v repobridge` fails. Prefer an existing installed binary when available.

Use the pinned release version `v0.3.0`. Pick the asset for the current platform:

| Platform | Asset |
| --- | --- |
| macOS Apple Silicon | `repobridge_v0.3.0_darwin_arm64.tar.gz` |
| macOS Intel | `repobridge_v0.3.0_darwin_amd64.tar.gz` |
| Linux x86_64 | `repobridge_v0.3.0_linux_amd64.tar.gz` |
| Linux arm64 | `repobridge_v0.3.0_linux_arm64.tar.gz` |
| Windows x86_64 | `repobridge_v0.3.0_windows_amd64.zip` |

Download the selected asset from:

```text
https://github.com/agentsw0rk/repobridge/releases/download/v0.3.0/<asset>
```

For macOS/Linux, extract the tarball and keep `repobridge` beside the bundled `libobjectbox.*` file in a directory on `PATH` such as `$HOME/.local/bin/repobridge-v0.3.0`. Make `repobridge` executable and verify with `repobridge --version`.

For Windows, extract the zip file and keep `repobridge.exe` beside the bundled `objectbox.dll`. Add that directory to `PATH` and verify with `repobridge --version`.

## Codegraph Search

After successful `path`, `fetch`, or `scan --fetch`, RepoBridge starts background AST indexing for cached sources. `repobridge search` builds a missing or stale graph synchronously unless `--no-sync-index` is set. Use search before reading large dependency trees manually.

Search command shape:

```bash
repobridge search --cwd <project-root> [--json] [--limit N] <spec> "<query>"
```

Query tokens:

| Token | Use |
| --- | --- |
| `kind:<kind>` | Filter node kind: `file`, `module`, `class`, `struct`, `interface`, `function`, `method`, `import`. |
| `lang:<language>` or `language:<language>` | Filter language: `go`, `java`, `kotlin`, `csharp`, `javascript`, `typescript`, `python`, `rust`, `unknown`. |
| `path:<substring>` | Restrict results to paths containing the substring. |
| `name:<substring>` | Match function, method, class, module, or import names. |
| `calls:<symbol>` | Find functions or methods that call a symbol. |
| free text | Matches indexed names, qualified names, paths, and call names. |

The same filters can also be passed as flags when this is clearer:

```bash
repobridge search --cwd <project-root> --json --limit 20 --kind function --lang kotlin --path DockerCompose.kt --calls exec <spec> ""
```

Use `--json` when another tool or the agent needs structured fields such as `kind`, `name`, `qualifiedName`, `language`, `path`, `startLine`, `endLine`, and `calls`.

Good query patterns:

```bash
repobridge search --cwd . react@19.0.0 "kind:function name:render"
repobridge search --cwd . pypi:requests==2.32.3 "calls:send lang:python"
repobridge search --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0 "lang:kotlin kind:function"
repobridge search --cwd . github.com/vercel/next.js "path:packages kind:method"
repobridge search --cwd . <spec> "kind:function calls:exec path:DockerCompose.kt"
```

Prefer search when the task is about definitions, declarations, functions, methods, classes, imports, languages, paths, or function calls. Use fallback `rg` on `$(repobridge path --cwd <project-root> <spec>)` for comments, docs, string literals, configuration files, generated code, or patterns outside the current AST extraction.

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
repobridge search --cwd /path/to/app react "kind:function name:useSyncExternalStore"
```

For a mixed backend project:

```bash
repobridge scan --cwd /path/to/service
repobridge fetch --cwd /path/to/service pypi:fastapi maven:org.springframework:spring-core@6.1.0
repobridge search --cwd /path/to/service pypi:fastapi "lang:python kind:function"
repobridge search --cwd /path/to/service maven:org.springframework:spring-core@6.1.0 "lang:java kind:class"
```

## Failure Handling

- If `repobridge` is not installed, download the pinned `v0.3.0` asset for the current OS/architecture, install it into a local bin directory, and verify `repobridge --version`.
- If `repobridge search --help` is unavailable, install or build a newer RepoBridge version that includes codegraph search.
- If a proposed spec fails, continue with the remaining specs and report the failure.
- If too many dependencies are detected, narrow to the libraries relevant to the user's current task.
- If private repositories fail, ask the user to provide the appropriate token through `GITHUB_TOKEN`, `GITLAB_TOKEN`, or `BITBUCKET_TOKEN`.
