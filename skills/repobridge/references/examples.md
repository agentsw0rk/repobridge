# RepoBridge Examples

Use these examples as command patterns. Replace specs, paths, and node IDs with values from the current project.

## Current Project Source

```bash
repobridge status --cwd . project:.
repobridge files --cwd . project:. --path internal/cli
repobridge search --cwd . project:. "kind:function name:NewRootCommand"
repobridge context --cwd . project:. "scan command dependency detection flow" --budget small
repobridge callees --cwd . project:. <file-or-class-node-id> --edge contains --limit 20
```

## Kotlin Stdlib Structure

```bash
repobridge status --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20
repobridge search --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 "kind:file path:commonMain/kotlin/collections/ArrayList.kt"
repobridge search --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 "kind:class name:ArrayList lang:kotlin"
repobridge callees --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 commonMain/kotlin/collections/ArrayList.kt --edge contains
repobridge callees --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 <array-list-node-id> --edge contains --limit 20
repobridge callers --cwd . maven:org.jetbrains.kotlin:kotlin-stdlib@2.0.20 <method-node-id> --edge contains
```

## React Project

```bash
repobridge scan --cwd /path/to/app --json
repobridge scan --cwd /path/to/app --fetch --limit 8
repobridge context --cwd /path/to/app react "useSyncExternalStore subscription flow" --budget small
repobridge search --cwd /path/to/app react "kind:function name:useSyncExternalStore"
repobridge callers --cwd /path/to/app react <node-id> --edge contains
```

## Mixed Backend Project

```bash
repobridge scan --cwd /path/to/service
repobridge fetch --cwd /path/to/service pypi:fastapi maven:org.springframework:spring-core@6.1.0
repobridge explore --cwd /path/to/service maven:org.springframework:spring-core@6.1.0 "ApplicationContext bean lifecycle" --budget medium
repobridge search --cwd /path/to/service pypi:fastapi "lang:python kind:function"
repobridge search --cwd /path/to/service maven:org.springframework:spring-core@6.1.0 "lang:java kind:class"
repobridge context --cwd /path/to/service <spec> "POST /login" --budget small
repobridge callees --cwd /path/to/service <spec> <controller-node-id> --edge contains
```

## Route-Aware Search

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
