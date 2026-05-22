---
name: repobridge
description: Use when an AI agent needs to search, inspect, locate, or reason about source code in external libraries, dependencies, frameworks, packages, vendored sources, cached sources, or fetched repositories; especially for symbols, files, routes, call graphs, structure, comments, README text, docs, examples, or source snippets.
---

# RepoBridge Project Context

Use RepoBridge as the first path for external dependency and fetched source investigation. The goal is to resolve real library sources into stable local paths and query their AST graph before falling back to broad text search.

For project-local code that is not an external dependency, normal local code tools still apply.

## Default Source Search Rule

For dependency, framework, package, vendored, cached, or fetched source-code search, use RepoBridge first.

This includes:

- finding files, symbols, classes, functions, methods, imports, routes, handlers, modules, and call relationships
- inspecting source structure with graph edges
- searching comments, README text, docs, examples, generated source, config files, raw string literals, or source snippets inside a resolved dependency source tree

Use normal local code tools first only when the target is the current project's own source code and no external dependency, vendored source, cached source, or fetched repository is involved.

## Core Policy

- Do not start dependency-source work with raw `rg`, `grep`, `find`, `gh`, `curl`, `wget`, registry pages, or manual browsing.
- Use RepoBridge to scan, fetch, resolve paths, build context, search AST nodes, and traverse graph edges.
- Prefer `context` or `explore` for task-level source understanding.
- Prefer `search`, `node`, `callers`, `callees`, and `impact` for exact symbols, files, modules, routes, containment, and call-flow evidence.
- For raw text in dependency sources, such as comments, README text, docs, examples, config files, generated source, or string literals, resolve the source path with RepoBridge first, then run scoped text search inside that path.
- Use fallback `rg` only for non-AST content after RepoBridge has resolved the source path.
- If fallback search is needed, scope it to `$(repobridge path --cwd <project-root> <spec>)` and state why AST search was insufficient.

## Minimal Workflow

1. Find the project root, usually the current working directory.
2. Check `command -v repobridge`.
3. If the spec is unknown, run `repobridge scan --cwd <project-root> --json`.
4. Fetch only task-relevant direct dependencies with `repobridge scan --cwd <project-root> --fetch --limit <N>` or `repobridge fetch --cwd <project-root> <spec>`.
5. Use `repobridge context` or `repobridge explore` before opening large dependency trees manually.
6. Use `repobridge search` for exact indexed nodes.
7. Use `repobridge node`, `callers`, `callees`, or `impact` for exact source lines and graph traversal.
8. Open only returned files and line ranges; then fall back to scoped `rg` only when needed.

## Quick Commands

```bash
repobridge scan --cwd . --json
repobridge scan --cwd . --fetch --limit 10
repobridge context --cwd . <spec> "auth login flow" --budget small
repobridge explore --cwd . <spec> "controller repository flow" --budget medium --depth 2
repobridge search --cwd . <spec> "kind:function name:<name>"
repobridge callees --cwd . <spec> <node-id> --edge contains --limit 20
```

## Decision Table

| Need | Use |
| --- | --- |
| Unknown dependency spec | `scan --json`; see [detection](references/detection.md). |
| Fetch or resolve source path | `scan --fetch`, `fetch`, `path`; see [workflow](references/workflow.md). |
| Focused implementation/debugging context | `context --budget small`; see [workflow](references/workflow.md). |
| Broader framework/library flow | `explore --budget medium --depth 2`; see [workflow](references/workflow.md). |
| Exact function, class, route, file, module, import, or caller query | `search`; see [search syntax](references/search-syntax.md). |
| Source lines for a known node | `node --source-lines N`; see [graph traversal](references/graph-traversal.md). |
| Call flow, impact, or structural containment | `callers`, `callees`, `impact`; see [graph traversal](references/graph-traversal.md). |
| Concrete command examples | See [examples](references/examples.md). |
| Missing tool, failed spec, private repo, or too many deps | See [failure handling](references/failure-handling.md). |

## Search Hints

Use `repobridge search --cwd . <spec> "<query>"` with compact query tokens:

| Token | Use |
| --- | --- |
| `kind:<kind>` | `file`, `module`, `class`, `struct`, `interface`, `function`, `method`, `import`, `route`, `handler`, `component_route`. |
| `lang:<language>` | `go`, `java`, `kotlin`, `csharp`, `javascript`, `typescript`, `python`, `rust`, `unknown`. |
| `path:<substring>` | Restrict by file path; route nodes also match route paths. |
| `name:<substring>` | Match names and qualified names. |
| `calls:<symbol>` | Find functions or methods that call a symbol. |

For structure navigation, use `--edge contains` with `callers` or `callees`.

```bash
repobridge search --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 "kind:file path:ArrayList.kt"
repobridge search --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 "kind:class name:ArrayList lang:kotlin"
repobridge callees --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 commonMain/kotlin/collections/ArrayList.kt --edge contains
repobridge callers --cwd . <spec> <method-node-id> --edge contains
```

## Common Mistakes

- Fetching every transitive dependency instead of the few relevant direct dependencies.
- Reading dependency directories manually before trying `context`, `explore`, or `search`.
- Using broad `rg` for questions already represented in the AST graph.
- Passing ambiguous names to graph commands instead of resolving a stable node ID with `search --json` or `node`.
- Forgetting `--cwd <project-root>`, especially for local version detection.
