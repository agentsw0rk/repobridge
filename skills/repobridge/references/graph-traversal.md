# RepoBridge Graph Traversal

Use this for exact node details, call flow, impact, and structure containment.

After successful `path`, `fetch`, or `scan --fetch`, RepoBridge starts background AST indexing for cached sources. Graph commands build a missing or stale graph synchronously unless `--no-sync-index` is set.

## Command Shapes

```bash
repobridge status --cwd <project-root> [--json] <spec>
repobridge files --cwd <project-root> [--json] [--path substring] [--limit N] <spec>
repobridge node --cwd <project-root> [--json] [--source-lines N] <spec> <id-or-name>
repobridge callers --cwd <project-root> [--json] [--depth N] [--edge kind] [--limit N] [--include-unresolved] <spec> <symbol>
repobridge callees --cwd <project-root> [--json] [--depth N] [--edge kind] [--limit N] [--include-unresolved] <spec> <symbol>
repobridge impact --cwd <project-root> [--json] [--depth N] [--edge kind] [--limit N] <spec> <symbol>
```

## Edge Kinds

| Edge | Use |
| --- | --- |
| `contains` | Structure navigation: file, module, type, member, and nested declaration hierarchy. |
| `calls` | Call-only traversal. |
| `imports` | Import relationships. |
| `handles` | Framework route handler relationships when indexed. |
| `routes_to` | Component/client route relationships when indexed. |
| `middleware` | Middleware route relationships when indexed. |

Multiple `--edge` flags can be used when an investigation needs more than one relationship kind.

## Structure Navigation

Use `--edge contains` when the user asks what a file, module, class, interface, struct, enum, trait, function, or method contains.

`file` nodes represent source files. `module` nodes represent statically visible packages or namespaces when available. `contains` edges connect local declarations to their primary parent.

If a name is ambiguous, use `search --json` or `node` to get the stable node ID, then pass that ID to `callers` or `callees`.

```bash
repobridge callees --cwd . <spec> <file-path-or-file-node-id> --edge contains
repobridge callees --cwd . <spec> <class-node-id> --edge contains --limit 50
repobridge callers --cwd . <spec> <node-id> --edge contains
```

## Flow Mapping

| User intent | Prefer this |
| --- | --- |
| Get exact source lines | `repobridge node --cwd . <spec> <id-or-name> --source-lines 16` |
| Trace callers | `repobridge callers --cwd . <spec> <symbol> --depth 2 --include-unresolved` |
| Trace callees | `repobridge callees --cwd . <spec> <symbol> --depth 2` |
| Estimate change impact | `repobridge impact --cwd . <spec> <symbol> --depth 2` |
| List what a file contains | `repobridge callees --cwd . <spec> <file-path-or-file-node-id> --edge contains` |
| List methods/properties on a class | `repobridge callees --cwd . <spec> <class-node-id> --edge contains --limit 50` |
| Find a node's structural parent | `repobridge callers --cwd . <spec> <node-id> --edge contains` |
