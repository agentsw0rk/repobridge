# Structure Graph Containment Design

## Context

Feature 43 adds a persisted structure graph to the existing AST graph. The codebase already has `NodeKindFile`, `NodeKindModule`, and `EdgeKindContains`, and ObjectBox stores edges generically by string kind, so no ObjectBox entity migration is required. The missing pieces are file/module node creation, containment edge generation, and tests that prove the graph can be queried and persisted with the new edges.

Context7 check:

- `github.com/tree-sitter/go-tree-sitter` supports the current parser pattern: create a parser, set a language, parse source, walk `RootNode`, inspect `Kind`, `NamedChild`, `ChildByFieldName`, and read `StartPosition`/`EndPosition`.
- ObjectBox Go requires `go generate` when entity fields change. This feature does not change entity fields, so the existing `EdgeEntity.Kind` string storage can persist `contains`.
- Go `testing` subtests and `t.TempDir` remain the right local test mechanism.

## Approach

Containment will be built as a parser post-processing pass after all existing language-specific extraction and expanded-kind extraction has run. This keeps callgraph, type, route, and resolver code unchanged while centralizing primary-parent selection in one place.

For each parsed source file, the parser will:

- create one stable `file` node with `QualifiedName` equal to the relative file path;
- create a `module` node when a static language scope is cheap and deterministic to detect, such as Go package, Java package, Kotlin package, C# namespace, Python module path, or Rust module file stem;
- add `file --contains--> module` when a module node exists;
- add exactly one primary `contains` parent for every non-external local node except file/module nodes;
- prefer explicit receiver ownership for methods, fields, properties, enum members, handlers, and constants when `ReceiverType` points to a local type;
- otherwise use the deepest local enclosing node by line range;
- fall back to module or file for top-level nodes.

## Parser Changes

Existing walkers already set real ranges for many functions and methods. A small expansion is needed so class-like nodes also get useful ranges from Tree-sitter where available:

- Java: append class, interface, and enum nodes from AST declarations before recursing.
- JavaScript/TypeScript: append class declarations from AST declarations.
- Python: append class definitions from AST declarations.
- Existing expanded-kind code remains as a fallback and for fields/properties/type aliases/enum members.

The post-processor will deduplicate nodes and edges using the existing stable IDs and edge identity. It will not add `contains` edges to `external` nodes.

## Schema

`SchemaVersion` will be bumped so existing graph stores are treated as stale and reindexed. ObjectBox model files do not need regeneration because no entity shape changes.

## Testing

Parser tests will cover:

- file node creation;
- top-level containment for Go imports/functions/types;
- type containment for fields, methods, enum members, and nested classes across representative languages;
- no containment edges for external nodes;
- stable JSON/string edge kind behavior through the existing edge model.

Indexer tests will cover:

- `GraphCounts.Edges` includes `contains`;
- schema version bump;
- existing call edges still exist alongside containment edges.

The full `go test ./...` suite remains the final verification.

