# Feature 38: Expanded Graph Kinds Done

## Summary

RepoBridge now supports expanded AST graph node and edge kinds for richer impact analysis. The graph model includes fields, properties, constants, enums, enum members, traits, protocols, and type aliases, plus semantic edges for extends, implements, references, type_of, returns, instantiates, and overrides.

The CLI accepts repeatable `--edge` filters on `callers`, `callees`, and `impact`, and search query parsing accepts the new node kinds. The graph schema version was bumped from 25 to 26 so existing graph caches are rebuilt through the current lifecycle stale-index path.

Parser extraction was extended for Go, Java, Kotlin, C#, TypeScript, and Rust using conservative syntactic patterns layered onto the existing Tree-sitter extraction.

## Deviations

- The implementation stays syntactic and local. It does not add full semantic type checking, external type placeholder nodes, Roslyn, `go/types`, or Rust Analyzer integration.
- Unresolved type relationships are omitted instead of creating noisy placeholder edges. Feature 39 remains the right place for richer external classification.
- Swift protocol extraction is represented in the model as `protocol`, but Swift source scanning is not implemented in this repository yet.

## Remaining Technical Debt

- Expanded kind extraction is intentionally conservative and should be refined as real-world fixtures expose grammar variants.
- `overrides` and `references` are available as stable edge kinds, but broad extraction is deferred until a reliable language-specific pattern is needed.
- Type resolution is local to the indexed graph and does not follow imports or package/module boundaries.

