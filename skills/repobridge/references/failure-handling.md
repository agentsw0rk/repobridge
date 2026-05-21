# RepoBridge Failure Handling

Use this when RepoBridge cannot complete the preferred path.

## Missing Tool

If `command -v repobridge` fails, state that RepoBridge is unavailable and use the best local fallback for the task.

## Missing Command

If `repobridge context --help`, `repobridge explore --help`, or `repobridge search --help` is unavailable, use the best available RepoBridge command before falling back.

## Failed Specs

If a proposed spec fails, continue with the remaining specs and report the failure. Do not let one failed dependency block unrelated source investigation.

## Too Many Dependencies

If too many dependencies are detected, narrow to the libraries relevant to the user's current task. Prefer direct dependencies, frameworks, SDKs, ORMs, test frameworks, UI libraries, and packages mentioned by the user.

## Private Repositories

If private repositories fail, ask the user to provide the appropriate token through `GITHUB_TOKEN`, `GITLAB_TOKEN`, or `BITBUCKET_TOKEN`. Do not print tokens or private registry configuration.

## Fallback Search

Use scoped fallback search only after RepoBridge cannot represent the target:

```bash
rg <pattern> "$(repobridge path --cwd <project-root> <spec>)"
```

Good fallback targets include comments, docs, string literals, generated code, configuration files, and unsupported file types. State why AST search was insufficient.
