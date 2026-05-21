# Structure Graph Containment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist `contains` edges that describe file, module, type, member, and nested declaration hierarchy in RepoBridge AST graphs.

**Architecture:** Add a parser post-processing pass that creates file/module nodes and assigns one primary containment parent per local node. Extend AST extraction where needed so class-like nodes have ranges, then bump graph schema to force reindexing.

**Tech Stack:** Go 1.26, Go `testing`, Tree-sitter Go bindings, ObjectBox Go string edge persistence.

---

### Task 1: Parser Containment Tests

**Files:**
- Modify: `internal/astgraph/parser/extractor_test.go`

- [ ] **Step 1: Write failing tests**

Add tests that parse small Go, Kotlin, TypeScript, Python, Java, C#, and Rust snippets and assert:

- `file` node exists for each source path.
- `file` or `module` contains top-level nodes.
- class/type nodes contain methods, properties, fields, enum members, and nested types.
- no `contains` edge targets an `external` node.

- [ ] **Step 2: Run parser tests to verify RED**

Run: `go test ./internal/astgraph/parser -run 'TestExtractFromSource.*Contain'`

Expected: tests fail because file nodes and `contains` edges are not produced yet.

### Task 2: Parser Containment Implementation

**Files:**
- Modify: `internal/astgraph/parser/extractor.go`
- Create: `internal/astgraph/parser/containment.go`
- Modify: `internal/astgraph/parser/queries.go`

- [ ] **Step 1: Implement containment post-processing**

Create `appendContainmentGraph(path, source, language, result)` and call it at the end of `ExtractFromSource` after `appendExpandedGraphKinds`.

The helper should:

- append a stable file node;
- append a stable module node when detectable;
- deduplicate nodes and edges;
- choose receiver parent, deepest range parent, then module/file fallback;
- add `EdgeKindContains` edges with provenance `parser:containment`.

- [ ] **Step 2: Add AST-ranged class-like nodes**

Update language walkers to append declaration nodes for Java class/interface/enum, JavaScript/TypeScript class, and Python class definitions so nested containment has usable ranges.

- [ ] **Step 3: Run parser tests to verify GREEN**

Run: `go test ./internal/astgraph/parser -run 'TestExtractFromSource.*Contain'`

Expected: parser containment tests pass.

### Task 3: Indexer and Schema Tests

**Files:**
- Modify: `internal/astgraph/indexer_test.go`
- Modify: `internal/astgraph/indexer.go`

- [ ] **Step 1: Write failing indexer tests**

Add tests that verify an indexed source directory includes file nodes and `contains` edges, call edges remain present, and `SchemaVersion` is bumped from the previous value.

- [ ] **Step 2: Run indexer tests to verify RED**

Run: `go test ./internal/astgraph -run 'TestIndexer.*Contain|TestSchemaVersion'`

Expected: tests fail until schema version and containment counts are implemented.

- [ ] **Step 3: Bump schema version and adjust counts expectations**

Increment `SchemaVersion` in `internal/astgraph/indexer.go`. Existing persistence should accept `contains` because edges are stored by string kind.

- [ ] **Step 4: Run indexer tests to verify GREEN**

Run: `go test ./internal/astgraph -run 'TestIndexer.*Contain|TestSchemaVersion'`

Expected: indexer tests pass.

### Task 4: Full Verification and Feature Done Note

**Files:**
- Add: `docs/features/43-structure-graph-containment-done.md`

- [ ] **Step 1: Run formatting**

Run: `gofmt -w ./cmd ./internal`

- [ ] **Step 2: Run full tests**

Run: `go test ./...`

Expected: all packages pass.

- [ ] **Step 3: Write done note**

Create `docs/features/43-structure-graph-containment-done.md` with summary, deviations, open questions, and test commands.

