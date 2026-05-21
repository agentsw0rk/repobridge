# Expanded Graph Kinds Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add expanded AST graph node and edge kinds for fields, properties, constants, enums, aliases, traits, type relationships, and instantiation queries.

**Architecture:** Extend the existing `internal/astgraph` model, query, store, CLI, and Tree-sitter parser path. Keep extraction syntactic and conservative, relying on local graph-node resolution instead of introducing semantic typecheckers.

**Tech Stack:** Go 1.22+, Cobra, Tree-sitter Go bindings, ObjectBox Go, Go `testing`.

---

### Task 1: Model, Query, Store, And CLI Edge Filters

**Files:**
- Modify: `internal/astgraph/model/types.go`
- Modify: `internal/astgraph/types.go`
- Modify: `internal/astgraph/query.go`
- Modify: `internal/astgraph/indexer.go`
- Modify: `internal/astgraph/store/store.go`
- Modify: `internal/cli/commands.go`
- Modify: `internal/cli/commands_test.go`
- Modify: `internal/astgraph/query_test.go`
- Modify: `internal/astgraph/store/store_test.go`

- [ ] **Step 1: Write failing query/model tests**

Add assertions that `ParseSearchQuery` accepts `kind:field`, `kind:property`, `kind:constant`, `kind:enum`, `kind:enum_member`, `kind:trait`, `kind:protocol`, and `kind:type_alias`.

- [ ] **Step 2: Write failing CLI/store tests**

Add a command test proving `callers --edge implements --edge instantiates` reaches `astgraph.CallgraphOptions.EdgeKinds`. Add a store test with mixed `calls` and `implements` edges proving `CallgraphQuery.EdgeKinds` restricts traversal to the requested edge kind.

- [ ] **Step 3: Verify red**

Run:

```bash
go test ./internal/astgraph ./internal/astgraph/store ./internal/cli
```

Expected: failures for missing node kind constants, missing edge filter fields, or unfiltered traversal.

- [ ] **Step 4: Implement model and CLI plumbing**

Add the new node and edge constants, expose aliases in `internal/astgraph/types.go`, add `EdgeKinds []EdgeKind` to `CallgraphOptions` and `CallgraphQuery`, parse repeated CLI `--edge` values, and pass them to the store.

- [ ] **Step 5: Implement store edge filtering**

Make `callgraphEdgeKindAllowed` honor explicit edge filters before default direction behavior. Preserve existing defaults when no `--edge` is supplied. Include new semantic edges in default `impact`.

- [ ] **Step 6: Bump schema**

Change `SchemaVersion` from `25` to `26` in `internal/astgraph/indexer.go`.

- [ ] **Step 7: Verify green**

Run:

```bash
go test ./internal/astgraph ./internal/astgraph/store ./internal/cli
```

Expected: targeted tests pass.

### Task 2: Parser Helpers For Type Nodes And Relationships

**Files:**
- Modify: `internal/astgraph/parser/queries.go`
- Modify: `internal/astgraph/parser/extractor_test.go`

- [ ] **Step 1: Write failing parser tests for each language**

Add compact fixtures that assert representative nodes and edges:

- Go: `field`, `constant`, `type_alias`, `type_of`, `returns`, `instantiates`.
- Java: `field`, `enum`, `enum_member`, `extends`, `implements`.
- Kotlin: `property`, `constant`, `type_alias`, `enum`, `enum_member`, `implements`.
- C#: `field`, `property`, `constant`, `enum`, `enum_member`, `extends`, `implements`.
- TypeScript: `property`, `type_alias`, `enum`, `enum_member`, `extends`, `implements`.
- Rust: `field`, `constant`, `type_alias`, `enum`, `enum_member`, `trait`, `implements`, `instantiates`.

- [ ] **Step 2: Verify red**

Run:

```bash
go test ./internal/astgraph/parser
```

Expected: parser tests fail because new nodes and edges are not extracted yet.

- [ ] **Step 3: Add shared helper functions**

Add helpers that append typed nodes, build member qualified names, resolve local type nodes by name/qualified name, and add local edges only when both endpoint IDs are known.

- [ ] **Step 4: Implement Go extraction**

Use source-line and existing Tree-sitter traversal helpers to extract struct fields, constants, type aliases, return-type edges, field type edges, and local constructor/compound literal instantiation edges.

- [ ] **Step 5: Implement Java/Kotlin/C#/TypeScript extraction**

Use existing class/method traversal branches and direct child scans to extract class fields/properties/constants/enums/type aliases and inheritance lists. Keep relationship edges local and conservative.

- [ ] **Step 6: Implement Rust extraction**

Extend existing Rust enum/struct/impl handling so enum variants become `enum_member`, trait declarations become `trait`, type aliases become `type_alias`, struct fields become `field`, and `impl Trait for Type` emits `implements`.

- [ ] **Step 7: Verify green**

Run:

```bash
go test ./internal/astgraph/parser
```

Expected: parser extraction tests pass.

### Task 3: Integration And Completion

**Files:**
- Modify: `docs/features/38-expanded-graph-kinds-done.md`
- Potentially modify: docs or tests touched by Tasks 1-2

- [ ] **Step 1: Run focused graph tests**

Run:

```bash
go test ./internal/astgraph/... ./internal/cli
```

Expected: all astgraph and CLI tests pass.

- [ ] **Step 2: Run repository verification**

Run:

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
```

Expected: formatting completes, all tests pass, vet reports no issues.

- [ ] **Step 3: Write done document**

Create `docs/features/38-expanded-graph-kinds-done.md` with implementation summary, deviations, and remaining technical debt.

- [ ] **Step 4: Final status**

Run:

```bash
git status --short
```

Expected: only intentional feature files are modified.

