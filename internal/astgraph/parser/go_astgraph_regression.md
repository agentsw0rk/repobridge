# Go AST Graph Regression Strategy

## Scope

This document describes the Go parser regression series in
`go_astgraph_test.go`. The goal is to expose bugs where RepoBridge can parse Go
syntax but fails to map the syntax into stable AST graph nodes, metadata, and
edges.

The tests are modeled after the Kotlin parser work: every case starts from a
language construct, then asserts the graph shape that downstream search,
context, and callgraph code needs.

## Source Selection

The current matrix combines targeted minimal fixtures with GitHub-derived
patterns:

| Source | Pattern used | Test coverage |
|---|---|---|
| Targeted fixture | Pointer, slice, and map struct fields | `field` nodes, `contains`, `type_of` |
| Targeted fixture | `const (...)` and generic type aliases | `constant` and `type_alias` nodes |
| Targeted fixture | Compact `interface{...}` methods | interface method metadata and containment |
| Targeted fixture | `Load[User](raw)` generic call syntax | unresolved `calls` from enclosing function |
| `gin-gonic/examples`, `versioning/main.go` | `Group`, nested `Group`, and `POST` route calls | route and handler graph edges |
| `gorilla/mux`, `middleware_test.go` and `doc.go` | `HandleFunc(...).Methods(...)` chains | mux route and handler graph edges |
| `kubernetes/kubernetes`, `pkg/proxy/ipvs/testing/util.go` | map/slice type aliases | type alias nodes plus return/instantiation edges |
| `golang/go`, `src/net/net.go` | interface methods with slice params and named multi-returns | method signature metadata |

The GitHub-derived tests intentionally use reduced fixtures instead of copying
large files. This keeps the expected graph small and makes the failing parser
layer obvious.

## Test Shape

Each regression test should include:

1. Minimal Go source code that reproduces one construct family.
2. A call to `ExtractFromSource(path, source, model.LanguageGo)`.
3. Node assertions by `NodeKind`, `Name`, and stable metadata such as
   `QualifiedName`, `ReceiverType`, `ParameterTypes`, and `ReturnType`.
4. Edge assertions for graph semantics: `contains`, `type_of`, `returns`,
   `instantiates`, `handles`, or `middleware`.
5. Unresolved call assertions for call extraction: `FromNodeID`,
   `ReferenceName`, `ReceiverText`, `ArgumentCount`, and `ArgumentTexts`.

Avoid broad fixture files as first-line tests. If a real repository exposes a
bug, shrink it to the smallest valid Go program that still fails.

## AST-to-Graph Expectations

The table below records the expected AST shape and the graph contract that the
tests assert. It is intentionally written in language-parser terms so that the
same checklist can be reused when another grammar exposes a surprising node
kind.

| Go construct | Expected Tree-sitter shape | Expected graph result |
|---|---|---|
| `type Service struct { Repo *Repository }` | `type_declaration` / `type_spec` / `struct_type` with field declaration | `struct Service`, `field Repo`, `Service contains Repo`, `Repo type_of Repository` |
| `type Cache[K comparable] map[K]Item` | `type_declaration` with generic type parameters and alias target | `type_alias Cache` with raw `ReturnType: map[K]Item` |
| `const ( MaxUsers = 10 )` | grouped const declaration | `constant MaxUsers` |
| `type Store interface{ Save(...) error }` | `interface_type` with method elements, including compact formatting | `interface Store`, `method Store.Save`, `Store contains Save` |
| `Read(b []byte) (n int, err error)` | interface method signature with slice parameter and named result tuple | method metadata `ParameterTypes: [byte]`, `ReturnType: int` |
| `Load[User](raw)` | Tree-sitter-Go may report `type_conversion_expression` with `generic_type` | unresolved call `Load` scoped to the enclosing function |
| `authV1.POST("users/add", AddV1User)` | selector call expression, after earlier `Group` assignments | `route POST /v1/users/add`, `handles AddV1User` |
| `HandleFunc("/products", ProductsHandler).Methods("GET")` | chained call expression where outer call is `Methods` | `route GET /products`, `handles ProductsHandler` |

## Current Fixed Issues

| ID | Construct | Reproduction | Expected result | Previous actual result | Priority | Status |
|---|---|---|---|---|---|---|
| GO-AST-001 | Struct fields using `*T`, `[]T`, `map[K]T` | `Repo *Repository`, `Items []Item`, `Lookup map[string]Item` | `field` nodes with `type_of` edges to local target types | Pointer/slice fields were missing; map fields resolved to `map` | High | Fixed |
| GO-AST-002 | Const blocks | `const ( MaxUsers = 10 )` | `constant MaxUsers` node | Constants inside blocks were skipped | Medium | Fixed |
| GO-AST-003 | Generic type aliases | `type Cache[K comparable] map[K]Item` | `type_alias Cache` with full return type | Alias was skipped because the regex only allowed non-generic names | Medium | Fixed |
| GO-AST-004 | Compact interfaces | `type Store interface{ Save(...) error }` | `interface Store` and contained method nodes | `interface{` without a space was skipped | High | Fixed |
| GO-AST-005 | Pointer and tuple-like interface returns | `Find(id ID) (*Item, error)` | `ReturnType: Item` | Return normalization was incomplete | Medium | Fixed |
| GO-AST-006 | Generic call expressions | `Load[User](raw)` | unresolved call `Load` from enclosing function | Tree-sitter-Go produced `type_conversion_expression`, so the Go walker skipped it | High | Fixed |
| GO-AST-007 | Named multi-return signatures | `Read(b []byte) (n int, err error)` | `ReturnType: int` | Return type was recorded as `n int` | Medium | Fixed |
| GO-AST-008 | GitHub-style nested Gin groups | `apiV1 := router.Group("/v1"); authV1.POST("users/add", AddV1User)` | `route POST /v1/users/add` handles `AddV1User` | Covered by route prefix regression | High | Fixed |
| GO-AST-009 | gorilla/mux method chains | `router.HandleFunc("/products", ProductsHandler).Methods("GET")` | `route GET /products` handles `ProductsHandler` | Covered by mux chain regression | High | Fixed |

## Open Parser Gaps

| ID | Construct | Reproduction | Expected result | Current result | Priority | Status |
|---|---|---|---|---|---|---|
| GO-AST-OPEN-001 | Inline compact struct fields | `type Service struct{ Repo *Repository; Items []Item }` | `field Repo` and `field Items` with `type_of` edges | The expanded-kind pass only scans fields on later lines | Medium | Open |
| GO-AST-OPEN-002 | Multi-name struct fields | `type Point struct { X, Y int }` | Either two `field` nodes or a documented single-node policy | Multi-name fields are intentionally skipped to avoid incorrect partial mapping | Low | Open |
| GO-AST-OPEN-003 | Type alias target edges | `type Chain map[string][]Rule` | Optional semantic edge from alias to `Rule` if alias target analysis is enabled | Alias node keeps raw `ReturnType`; no target edge is emitted | Low | Open |
| GO-AST-OPEN-004 | Deep gorilla/mux chains | `r.HandleFunc(...).Host(...).Methods("GET").Schemes("http")` | Route still recognized when `Methods` is not directly on `HandleFunc` | Only the direct `HandleFunc(...).Methods(...)` shape is indexed | Medium | Open |
| GO-AST-OPEN-005 | `chi.Route` nested callbacks | `r.Route("/api", func(r chi.Router) { r.Get("/users", list) })` | Combined `/api/users` route | Callback route scopes are not modeled yet | Medium | Open |

Open items should become failing tests first when they are implemented. Until
then, they remain documented gaps rather than hidden assumptions.

## Transferable Strategy

Use the same matrix for any new language:

| Axis | Question | Evidence |
|---|---|---|
| Declarations | Which syntax creates graph nodes? | `class`, `struct`, `interface`, `function`, `method`, `field`, `constant`, `type_alias` assertions |
| Containment | Which owner contains the node? | `file/module/type/function contains child` edges |
| Type mapping | Which local type relationships matter? | `type_of`, `returns`, `instantiates`, `extends`, `implements` edges |
| Call extraction | Which expressions represent calls? | unresolved references with receiver and arguments |
| Framework mapping | Which calls represent routes, handlers, middleware, or component routes? | framework-specific nodes and edges |
| Grammar traps | Which valid syntax is represented by surprising AST nodes? | reduced fixtures plus comments or failure-list entries |

When adding another language, start with:

1. One targeted fixture for declarations.
2. One fixture for containment.
3. One fixture for type relationships.
4. One fixture for call extraction.
5. One fixture from a real GitHub repository or standard library pattern.
6. A bug table that records fixed and open parser gaps.

Language-specific recommendations:

- Prefer one parser-facing test file per language once the matrix grows beyond
  basic smoke coverage. This keeps grammar regressions searchable.
- Keep real-repository snippets reduced but traceable. Record the source repo,
  path, and pattern in the strategy document.
- Assert graph edges in the same test as the node that motivates them. A node
  without a usable edge is usually not enough for RepoBridge features.
- Document open gaps even when they are not yet tests. Open gaps should include
  a reduced reproduction and a priority so they can be promoted to failing
  tests later.
- For syntax that parser libraries model unexpectedly, record the AST node kind
  in the bug table or AST-to-Graph table. This avoids repeating the same
  investigation when the grammar is upgraded.

The standard workflow is red-green-refactor:

1. Add a small failing test that states the expected graph.
2. Run the targeted parser test and record the observed failure.
3. Fix the parser layer that actually lost the information.
4. Re-run the targeted test, then the full parser package.
5. Update the fixed/open issue table.

## Typical Failure Classes

- The AST contains the construct, but the language walker does not visit that
  node kind.
- The construct is represented as a different AST node than expected.
- Regex-based expanded-kind extraction only covers simple formatting.
- Type normalization strips too little or too much information.
- A graph node is emitted without the edge that downstream graph queries need.
- Call extraction loses receiver, argument text, or enclosing scope.
- Framework-specific route extraction handles direct calls but not grouped,
  chained, or callback-based variants.
