# RepoBridge Detection

Use this when deciding which dependency specs to fetch.

## Scan Output

```bash
repobridge scan --cwd <project-root> --json
```

JSON output includes:

| Field | Meaning |
| --- | --- |
| `candidates` | Deduplicated RepoBridge specs sorted by confidence. |
| `warnings` | Files that could not be parsed or ambiguous detections. |

Each candidate has:

| Field | Meaning |
| --- | --- |
| `spec` | Value to pass to RepoBridge. |
| `ecosystem` | `npm`, `pypi`, `go`, `crates`, `maven`, `nuget`, or `unknown`. |
| `confidence` | Higher means safer to fetch. |
| `reasons` | Why the spec was proposed. |

## Supported Detection

The scanner detects common direct dependencies from:

| Ecosystem | Sources |
| --- | --- |
| JavaScript/TypeScript | `package.json`, `package-lock.json`, imports/requires. |
| Python | `requirements.txt`. |
| Go | `go.mod`, imports. |
| Rust | `Cargo.toml`. |
| JVM | `pom.xml`. |
| .NET | `.csproj`. |

## Selection Rules

- Fetch direct dependencies before transitive dependencies.
- Use versions from lockfiles and manifests when available.
- For npm, pass `--cwd <project-root>` so RepoBridge can use local version detection.
- Treat unknown import aliases, workspace packages, relative imports, and standard libraries as project-local or built-in unless a manifest confirms an external package.
- Skip secrets and private registry configuration. Never print tokens.
- Keep dependency source directories read-only. Do not modify cached dependency sources unless the user explicitly asks to inspect or patch a vendored copy.

Detection is conservative. If the scanner cannot infer a reliable RepoBridge spec, inspect the manifest manually and decide whether fetching that dependency is useful for the task.
