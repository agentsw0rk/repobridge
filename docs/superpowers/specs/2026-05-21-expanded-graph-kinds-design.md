# Expanded Graph Kinds Design

## Problem

RepoBridge's AST graph currently models functions, methods, classes, structs, interfaces, imports, variables, routes, and handlers. That is enough for callgraph and route questions, but it loses important impact-analysis concepts such as fields, properties, constants, enum members, type aliases, traits, inheritance, implementation, type usage, return types, and object construction.

## Scope

This feature adds syntactic graph detail using the existing Tree-sitter based parser and ObjectBox-backed graph store. It does not add a full semantic typechecker, Roslyn integration, `go/types`, Rust Analyzer, or perfect override resolution.

The implementation covers Go, Java, Kotlin, C#, TypeScript, and Rust where the concept exists in the language grammar and can be recognized statically from source syntax.

## Graph Model

Add these node kinds:

- `field`
- `property`
- `constant`
- `enum`
- `enum_member`
- `trait`
- `protocol`
- `type_alias`

Add these edge kinds:

- `extends`
- `implements`
- `references`
- `type_of`
- `returns`
- `instantiates`
- `overrides`

Existing node and edge kinds stay valid. Search query parsing and JSON output serialize the new string values without special compatibility code.

The graph schema version is bumped from `25` so old graph caches are considered stale by the existing lifecycle check and get rebuilt automatically.

## Parser Behavior

Parser extraction remains additive and conservative:

- Go: struct fields become `field`; constants become `constant`; type aliases become `type_alias`; typed fields and return types create `type_of` and `returns` when the target type resolves locally; `T{}` and `T(...)` constructor-like expressions create `instantiates` when the target is a local type.
- Java: fields become `field`; enum declarations become `enum`; enum constants become `enum_member`; `extends` and `implements` clauses create graph edges when targets resolve locally; return types create `returns`.
- Kotlin: `val`/`var` properties become `property` or `constant` for top-level `const val`; `typealias` becomes `type_alias`; enum classes and entries become `enum`/`enum_member`; superclass and interface lists produce `extends`/`implements` when resolvable.
- C#: fields become `field`; properties become `property`; `const` and readonly fields become `constant`; enums and enum members are extracted; base lists produce `extends`/`implements` using local resolution where possible.
- TypeScript: interface/type aliases/classes/enums and enum members are extracted where present; property signatures and class fields become `property` or `field`; `extends`/`implements` clauses are linked when the target is local.
- Rust: `struct` fields become `field`; `const` items become `constant`; enum declarations become `enum`; variants become `enum_member`; `trait` items become `trait`; `type` aliases become `type_alias`; `impl Trait for Type` produces `implements`; tuple/unit struct and enum variant construction produces `instantiates` where the target resolves.

For unresolved type relationships, the parser may leave the source node without an edge rather than producing noisy placeholder nodes. Feature 39 will handle broader external classification later.

## CLI And Store

`repobridge search` already supports `kind:` and `--kind`; it is extended by allowing the new node kinds.

`repobridge callers`, `repobridge callees`, and `repobridge impact` gain repeatable `--edge` filters. Without `--edge`, existing defaults remain:

- `callers` and `callees`: `calls`, `handles`, and `routes_to`, plus new semantic edges that are useful for graph traversal only when explicitly requested.
- `impact`: existing impact edges plus the new semantic relationship edges.

With `--edge`, traversal is restricted to the requested edge kinds. This makes examples such as `--edge instantiates` and `--edge implements` stable.

## Testing

Tests cover:

- model/query parsing for all new node kinds;
- CLI propagation of repeated `--edge` values;
- store callgraph traversal filtered by edge kind;
- schema version bump behavior through existing lifecycle tests;
- parser extraction fixtures for Go, Java, Kotlin, C#, TypeScript, and Rust.

Targeted tests are run first during TDD, followed by `go test ./...`.

