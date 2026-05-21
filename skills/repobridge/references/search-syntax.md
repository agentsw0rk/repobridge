# RepoBridge Search Syntax

Use this for exact AST search queries.

## Shape

```bash
repobridge search --cwd <project-root> [--json] [--limit N] <spec> "<query>"
```

Use `--json` when another tool or agent needs structured fields such as `kind`, `name`, `qualifiedName`, `language`, `path`, `startLine`, `endLine`, and `calls`.

## Query Tokens

| Token | Use |
| --- | --- |
| `kind:<kind>` | Filter node kind: `file`, `module`, `class`, `struct`, `interface`, `function`, `method`, `import`, `route`, `handler`, `component_route`. |
| `lang:<language>` or `language:<language>` | Filter language: `go`, `java`, `kotlin`, `csharp`, `javascript`, `typescript`, `python`, `rust`, `unknown`. |
| `path:<substring>` | Restrict results to file paths; route nodes also match route patterns such as `/login`. |
| `name:<substring>` | Match function, method, class, module, import, or qualified names. |
| `calls:<symbol>` | Find functions or methods that call a symbol. |
| free text | Matches indexed names, qualified names, paths, and call names. |

The same filters can be passed as flags when this is clearer:

```bash
repobridge search --cwd <project-root> --json --limit 20 --kind function --lang kotlin --path DockerCompose.kt --calls exec <spec> ""
```

## Intent Mapping

| User intent | Prefer this |
| --- | --- |
| Find a function or method | `repobridge search --cwd . <spec> "kind:function name:<name>"` or `kind:method name:<name>` |
| Find classes/interfaces | `repobridge search --cwd . <spec> "kind:class name:<name>"` or `kind:interface name:<name>` |
| Find files or modules | `repobridge search --cwd . <spec> "kind:file path:<file>"` or `kind:module name:<package>` |
| Find framework routes | `repobridge search --cwd . <spec> "kind:route path:<route-path>"` |
| Find client component routes | `repobridge search --cwd . <spec> "kind:component_route path:<route-path>"` |
| Find callers by indexed call names | `repobridge search --cwd . <spec> "kind:function calls:<symbol>"` |
| Search Kotlin sources | `repobridge search --cwd . <spec> "lang:kotlin <query>"` |
| Restrict to a file or package path | `repobridge search --cwd . <spec> "path:<substring> <query>"` |

## Examples

```bash
repobridge search --cwd . react@19.0.0 "kind:function name:render"
repobridge search --cwd . pypi:requests==2.32.3 "calls:send lang:python"
repobridge search --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 "kind:file path:ArrayList.kt"
repobridge search --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 "kind:class name:ArrayList lang:kotlin"
repobridge search --cwd . github.com/vercel/next.js "path:packages kind:method"
repobridge search --cwd . <spec> "kind:route path:/api/login"
repobridge search --cwd . <spec> "kind:component_route path:/settings"
```
