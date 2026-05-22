# RepoBridge Workflow

Use this when the main skill's minimal workflow is not enough.

## Availability

```bash
command -v repobridge
repobridge context --help
repobridge explore --help
repobridge search --help
```

If a newer command is unavailable, use the best available RepoBridge command before falling back to local file search.

## Scan

```bash
repobridge scan --cwd <project-root> --json
```

Review high-confidence direct dependency candidates from manifests and lockfiles first. Import-derived candidates are supporting evidence, not automatic fetch targets.

For the current repository itself, use `project:.` directly instead of scanning for a dependency spec:

```bash
repobridge search --cwd <project-root> project:. "kind:function name:<name>"
repobridge context --cwd <project-root> project:. "<task query>" --budget small
```

## Fetch

```bash
repobridge scan --cwd <project-root> --fetch --limit 10
repobridge fetch --cwd <project-root> <spec>
repobridge path --cwd <project-root> <spec>
```

Fetch only references that matter for the current task: frameworks, runtimes, SDKs, ORMs, test frameworks, UI libraries, and libraries connected to the user request.

## Build Context

```bash
repobridge context --cwd <project-root> [--json] [--budget small|medium|large] [--limit N] [--depth N] <spec> "<query>"
repobridge explore --cwd <project-root> [--json] [--budget small|medium|large] [--limit N] [--depth N] <spec> "<query>"
```

Use `context` for focused task evidence and `explore` for wider source understanding. Output includes entry points, relationships, snippets, related files, warnings, and stats.

Budget guidance:

| Budget | Use |
| --- | --- |
| `small` | Most implementation, debugging, and agent prompts. |
| `medium` | Normal investigations with a few related flows. |
| `large` | Wider context only when the task needs it. |

## Read Files Last

Open only the files and line ranges returned by RepoBridge. Use LSP navigation when available. Use scoped `rg` only for content outside the AST graph.
