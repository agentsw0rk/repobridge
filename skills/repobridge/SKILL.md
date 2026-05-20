---
name: repobridge
description: Use when an AI agent needs to find, inspect, search, or reason about external framework, library, dependency, package, or fetched source code for a project. Prefer this skill before raw network fetches, gh/curl downloads, generic file search, rg, grep, or manually browsing dependency trees. Use RepoBridge context, explore, search, graph, and callgraph commands first.
---

# RepoBridge Project Context

Use this skill to enrich an agent's local context with source code for the frameworks and libraries a project actually uses. The goal is not to edit dependency code. The goal is to resolve dependency sources into stable local paths and use them as reference material while working on the project.

## Source Search Policy

When the user asks about dependency, framework, library, package, or fetched source code, use RepoBridge as the first search path.

- Do not start with raw `rg`, `grep`, `find`, `gh`, `curl`, `wget`, package registry pages, or manual browsing of dependency directories.
- Do not use a plain network fetch to inspect sources that RepoBridge can resolve.
- Use `repobridge scan`, `repobridge path`, `repobridge context`, `repobridge explore`, `repobridge search`, graph inspection, and callgraph commands to resolve and query dependency sources.
- Use `repobridge context` or `repobridge explore` for task-level source investigations before opening large files or scanning whole trees manually.
- Use `repobridge search`, `node`, `callers`, `callees`, or `impact` for exact symbol, route, handler, path, language, and call-flow questions.
- Use `rg` or `grep` only as a documented fallback after RepoBridge graph commands cannot represent the target, for example comments, README text, raw string literals, generated files, config files, or unindexed file types.
- If fallback `rg` or `grep` is used, scope it to `$(repobridge path --cwd <project-root> <spec>)` and state why AST search was insufficient.

For project-local code that is not an external dependency, normal local code tools can still be used. This skill is specifically the default path for dependency and fetched source investigation.

## Workflow

1. **Find the project root.** Use the current working directory unless the user gives another path.
2. **Ensure RepoBridge is available.** Run `command -v repobridge`. If it is missing, install the pinned GitHub release using the platform-specific asset table below. If AST context is needed, verify `repobridge context --help`, `repobridge explore --help`, and `repobridge search --help`; when unavailable, use a newer RepoBridge build that includes codegraph context/search.
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

7. **Build task context from resolved source graphs first.** Prefer `repobridge context` for focused tasks and `repobridge explore` for broader graph explanation. They return bounded entry points, relationships, snippets, related files, warnings, and stats:

   ```bash
   repobridge context --cwd <project-root> react "createRoot render flow" --budget small
   repobridge explore --cwd <project-root> github.com/vercel/next.js "AppRouter cache invalidation" --budget large --depth 2
   ```

8. **Search exact symbols, routes, and calls with RepoBridge.** Prefer `repobridge search` for symbol-, route-, language-, path-, and call-aware investigation because it returns compact AST results instead of broad text snippets:

   ```bash
   repobridge search --cwd <project-root> react "kind:function calls:createRoot"
   repobridge search --cwd <project-root> maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0 "lang:kotlin kind:function"
   repobridge search --cwd <project-root> <spec> "kind:route path:/login"
   repobridge search --cwd <project-root> <spec> "kind:component_route path:/settings"
   ```

9. **Inspect and trace graph evidence before raw text search.** Use `status`, `files`, `node`, `callers`, `callees`, and `impact` when the agent needs exact graph health, source lines, or call-flow evidence.
10. **Use resolved paths as read-only references when needed.** Open specific files returned by RepoBridge commands, use LSP navigation when available, and use `rg` only as a fallback for text that is not represented in the AST graph.
11. **State what was fetched and searched.** In the final response, mention which frameworks/libraries were resolved and which RepoBridge queries or paths were used when that matters for the task.

## Installing RepoBridge from GitHub Releases

Use this only when `command -v repobridge` fails. Prefer an existing installed binary when available.

Use the pinned release version `v0.5.0`. Pick the asset for the current platform:

| Platform | Asset |
| --- | --- |
| macOS Apple Silicon | `repobridge_v0.5.0_darwin_arm64.tar.gz` |
| macOS Intel | `repobridge_v0.5.0_darwin_amd64.tar.gz` |
| Linux x86_64 | `repobridge_v0.5.0_linux_amd64.tar.gz` |
| Linux arm64 | `repobridge_v0.5.0_linux_arm64.tar.gz` |
| Windows x86_64 | `repobridge_v0.5.0_windows_amd64.zip` |

Download the selected asset from:

```text
https://github.com/agentsw0rk/repobridge/releases/download/v0.5.0/<asset>
```

For macOS/Linux, extract the tarball and keep `repobridge` beside the bundled `libobjectbox.*` file in a directory on `PATH` such as `$HOME/.local/bin/repobridge-v0.5.0`. Make `repobridge` executable and verify with `repobridge --version`.

For Windows, extract the zip file and keep `repobridge.exe` beside the bundled `objectbox.dll`. Add that directory to `PATH` and verify with `repobridge --version`.

## Codegraph Context and Search

After successful `path`, `fetch`, or `scan --fetch`, RepoBridge starts background AST indexing for cached sources. `repobridge context`, `explore`, `search`, graph inspection, and callgraph commands build a missing or stale graph synchronously unless `--no-sync-index` is set. Use these graph commands before reading large dependency trees manually.

Use this decision order for source-code questions:

1. `repobridge scan --cwd <project-root> --json` to identify candidate specs when the spec is unknown.
2. `repobridge scan --cwd <project-root> --fetch --limit <N>` or `repobridge fetch --cwd <project-root> <spec>` only to populate the local source cache.
3. `repobridge context --cwd <project-root> <spec> "<task query>"` for focused implementation/debugging context.
4. `repobridge explore --cwd <project-root> <spec> "<symbols or flow>"` for broader graph explanation.
5. `repobridge search --cwd <project-root> <spec> "<query>"` for definitions, functions, methods, classes, imports, framework routes, component routes, paths, languages, or call relationships.
6. `repobridge node`, `callers`, `callees`, or `impact` for exact symbol details and graph traversal.
7. Open only the files and lines returned by RepoBridge.
8. Use scoped fallback `rg` only for non-AST content and explain the fallback.

Context command shape:

```bash
repobridge context --cwd <project-root> [--json] [--budget small|medium|large] [--limit N] [--depth N] <spec> "<query>"
repobridge explore --cwd <project-root> [--json] [--budget small|medium|large] [--limit N] [--depth N] <spec> "<query>"
```

Use `context` for focused task evidence and `explore` for wider source understanding. Output includes `entryPoints`, `relationships`, `snippets`, `relatedFiles`, `warnings`, and `stats`. Budget defaults are bounded; use `small` for most agent prompts, `medium` for normal investigations, and `large` only when the task needs wider context.

Search command shape:

```bash
repobridge search --cwd <project-root> [--json] [--limit N] <spec> "<query>"
```

Query tokens:

| Token | Use |
| --- | --- |
| `kind:<kind>` | Filter node kind: `file`, `module`, `class`, `struct`, `interface`, `function`, `method`, `import`, `route`, `handler`, `component_route`. |
| `lang:<language>` or `language:<language>` | Filter language: `go`, `java`, `kotlin`, `csharp`, `javascript`, `typescript`, `python`, `rust`, `unknown`. |
| `path:<substring>` | Restrict results to file paths; for `route` and `component_route` nodes this also matches route patterns such as `/login`. |
| `name:<substring>` | Match function, method, class, module, or import names. |
| `calls:<symbol>` | Find functions or methods that call a symbol. |
| free text | Matches indexed names, qualified names, paths, and call names. |

The same filters can also be passed as flags when this is clearer:

```bash
repobridge search --cwd <project-root> --json --limit 20 --kind function --lang kotlin --path DockerCompose.kt --calls exec <spec> ""
```

Use `--json` when another tool or the agent needs structured fields such as `kind`, `name`, `qualifiedName`, `language`, `path`, `startLine`, `endLine`, and `calls`.

Graph inspection and callgraph command shapes:

```bash
repobridge status --cwd <project-root> [--json] <spec>
repobridge files --cwd <project-root> [--json] [--path substring] [--limit N] <spec>
repobridge node --cwd <project-root> [--json] [--source-lines N] <spec> <id-or-name>
repobridge callers --cwd <project-root> [--json] [--depth N] [--limit N] [--include-unresolved] <spec> <symbol>
repobridge callees --cwd <project-root> [--json] [--depth N] [--limit N] [--include-unresolved] <spec> <symbol>
repobridge impact --cwd <project-root> [--json] [--depth N] [--limit N] <spec> <symbol>
```

Good query patterns:

```bash
repobridge search --cwd . react@19.0.0 "kind:function name:render"
repobridge search --cwd . pypi:requests==2.32.3 "calls:send lang:python"
repobridge search --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0 "lang:kotlin kind:function"
repobridge search --cwd . github.com/vercel/next.js "path:packages kind:method"
repobridge search --cwd . <spec> "kind:function calls:exec path:DockerCompose.kt"
repobridge search --cwd . <spec> "kind:route path:/api/login"
repobridge search --cwd . <spec> "kind:component_route path:/settings"
repobridge context --cwd . <spec> "POST /api/login" --budget small
repobridge context --cwd . <spec> "auth login session flow" --budget small
repobridge explore --cwd . <spec> "AuthController LoginRepository" --budget medium --depth 2
repobridge node --cwd . <spec> AuthController.login --source-lines 16
repobridge callers --cwd . <spec> login --depth 2 --include-unresolved
```

Route-aware examples:

```bash
repobridge search --cwd . <spring-spec> "kind:route lang:kotlin path:/login"
repobridge search --cwd . <express-spec> "kind:route lang:javascript path:/api/users"
repobridge search --cwd . <react-router-spec> "kind:component_route path:/dashboard"
repobridge search --cwd . <fastapi-spec> "kind:route lang:python path:/items"
repobridge search --cwd . <aspnet-spec> "kind:route lang:csharp path:/api/users"
repobridge search --cwd . <rust-web-spec> "kind:route lang:rust path:/health"
repobridge callers --cwd . <spec> UsersController.Get --depth 1
repobridge context --cwd . <spec> "GET /api/users" --budget small
```

Translate common source-search requests like this:

| User intent | Prefer this |
| --- | --- |
| Find a function or method | `repobridge search --cwd . <spec> "kind:function name:<name>"` or `kind:method name:<name>` |
| Find classes/interfaces | `repobridge search --cwd . <spec> "kind:class name:<name>"` or `kind:interface name:<name>` |
| Find framework routes | `repobridge search --cwd . <spec> "kind:route path:<route-path>"` |
| Find client component routes | `repobridge search --cwd . <spec> "kind:component_route path:<route-path>"` |
| Find route handler evidence | `repobridge callers --cwd . <spec> <HandlerOrController.method> --depth 1` |
| Find callers of a function | `repobridge search --cwd . <spec> "kind:function calls:<symbol>"` |
| Trace callers with graph edges | `repobridge callers --cwd . <spec> <symbol> --depth 2 --include-unresolved` |
| Trace callees from a symbol | `repobridge callees --cwd . <spec> <symbol> --depth 2` |
| Estimate change impact | `repobridge impact --cwd . <spec> <symbol> --depth 2` |
| Get exact node source lines | `repobridge node --cwd . <spec> <id-or-name> --source-lines 16` |
| Build focused task context | `repobridge context --cwd . <spec> "<task query>" --budget small` |
| Explore broader source flow | `repobridge explore --cwd . <spec> "<symbols or flow>" --budget medium --depth 2` |
| Search Kotlin sources | `repobridge search --cwd . <spec> "lang:kotlin <query>"` |
| Restrict to a file or package path | `repobridge search --cwd . <spec> "path:<substring> <query>"` |
| Need structured output for an agent | `repobridge search --cwd . --json --limit 20 <spec> "<query>"` |

Prefer `context`/`explore` when the task asks for source understanding, implementation guidance, debugging context, route flow, or architectural flow. Prefer `search`, `node`, and callgraph commands when the task is about definitions, declarations, functions, methods, classes, framework routes, component routes, handlers, imports, languages, paths, or function calls. Use fallback `rg` on `$(repobridge path --cwd <project-root> <spec>)` for comments, docs, string literals, configuration files, generated code, or patterns outside the current AST extraction.

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
repobridge context --cwd /path/to/app react "useSyncExternalStore subscription flow" --budget small
repobridge search --cwd /path/to/app react "kind:function name:useSyncExternalStore"
```

For a mixed backend project:

```bash
repobridge scan --cwd /path/to/service
repobridge fetch --cwd /path/to/service pypi:fastapi maven:org.springframework:spring-core@6.1.0
repobridge explore --cwd /path/to/service maven:org.springframework:spring-core@6.1.0 "ApplicationContext bean lifecycle" --budget medium
repobridge search --cwd /path/to/service pypi:fastapi "lang:python kind:function"
repobridge search --cwd /path/to/service maven:org.springframework:spring-core@6.1.0 "lang:java kind:class"
repobridge search --cwd /path/to/service <spec> "kind:route path:/login"
repobridge context --cwd /path/to/service <spec> "POST /login" --budget small
```

## Failure Handling

- If `repobridge` is not installed, download the pinned `v0.5.0` asset for the current OS/architecture, install it into a local bin directory, and verify `repobridge --version`.
- If `repobridge context --help`, `repobridge explore --help`, or `repobridge search --help` is unavailable, install or build a newer RepoBridge version that includes codegraph context/search.
- If a proposed spec fails, continue with the remaining specs and report the failure.
- If too many dependencies are detected, narrow to the libraries relevant to the user's current task.
- If private repositories fail, ask the user to provide the appropriate token through `GITHUB_TOKEN`, `GITLAB_TOKEN`, or `BITBUCKET_TOKEN`.
