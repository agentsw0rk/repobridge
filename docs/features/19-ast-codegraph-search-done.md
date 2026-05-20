# Feature 19 Done: AST Codegraph Search

## Summary

RepoBridge now starts asynchronous AST codegraph indexing after successful `path`, `fetch`, and `scan --fetch` cache outcomes. Graph data is stored beside each cached source under `.repobridge-graph/` using ObjectBox. The new `repobridge search` command searches symbol names, languages, paths, node kinds, and function call references, and builds a missing graph synchronously before searching.

## Deviations

- Kotlin extraction is warning-only if the available Kotlin Tree-sitter Go binding is incompatible with the selected `go-tree-sitter` version.
- Search filtering is implemented through the store API with deterministic in-process scoring before deeper ObjectBox query optimization.

## Open Follow-ups

- Add a long-running file watcher for editable source trees.
- Add richer cross-file call resolution.
- Add framework-aware route nodes after the core graph model stabilizes.
