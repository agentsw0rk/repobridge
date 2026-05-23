# Multi-Language AST Graph Regression Strategy

## Scope

This document applies the Go AST graph regression approach to the other
RepoBridge parser languages: JavaScript, TypeScript, Python, Rust, Java,
Kotlin, and C#.

The common rule is the same as for Go: a parser test is useful only when it
proves the graph contract used by search, context, callgraph, and route lookup.
That means tests should assert nodes, metadata, and edges, not just successful
parsing.

## Iterative Research Process

This file is the running notebook for multi-language AST parser research. Each
iteration should be small enough to finish in one interaction and should leave
the repository with either new regression tests, a documented open issue, or
both.

Use this loop:

1. Pick one language and one construct family.
2. Select evidence from one of three sources: targeted fixture, standard
   framework documentation, or real GitHub source.
3. Reduce the example to the smallest valid snippet that still represents the
   construct.
4. Parse it through `ExtractFromSource` or, when cross-file behavior matters,
   through an indexer-level test.
5. Compare expected graph contract against actual nodes, metadata, unresolved
   references, and edges.
6. Record the result in the iteration log and update the fixed/open tables.
7. If the behavior should be supported now, add a failing test first, fix the
   parser layer that loses the information, then rerun the language subset.
8. If the behavior is intentionally deferred, keep it in the open findings or
   next-hypotheses table with a concrete reproduction.

The minimum verification command for parser-only work is:

```bash
go test ./internal/astgraph/parser
```

When a change can affect indexing, containment, search, or stored edges, also
run:

```bash
go test ./internal/astgraph/...
```

Use `go test ./...` as a repository-level signal, but do not treat unrelated
non-parser failures as evidence against parser behavior. Record them in the
iteration log.

## Documentation Schema

Every iteration entry should include:

| Field | Meaning |
|---|---|
| Iteration ID | Stable label such as `2026-05-23-route-dsl-pass-1` |
| Languages | Languages tested in the interaction |
| Constructs | Syntax families under test |
| Sources | Targeted fixture, docs, or GitHub repo/path |
| Expected graph | Required nodes, metadata, unresolved calls, and edges |
| Actual result | Observed parser output or test failure |
| Outcome | `covered`, `fixed`, `open`, `deferred`, or `needs-indexer-test` |
| Next hypothesis | What to test next and why |

Open findings must include language, construct, reproduction, expected
node/edge structure, actual behavior, priority, and status.

## Current Regression Coverage

| Language | Existing coverage | Test location |
|---|---|---|
| JavaScript | function declarations, unresolved calls, Express routes, middleware, React Router JSX and object routes | `extractor_test.go` |
| TypeScript | function declarations, Express routes, classes, interfaces, properties, type aliases, enums, inheritance and implementation edges | `extractor_test.go` |
| Python | functions, calls, Flask/FastAPI-style decorators, Django `path`/`re_path`, nested class containment | `extractor_test.go` |
| Rust | functions, constructors, enum variants, impl methods, receiver calls, imports, Axum-like routes, traits, structs, fields, impl edges | `extractor_test.go` |
| Java | methods, calls, Spring routes, request mappings, classes, interfaces, enums, fields, extends/implements/returns edges | `extractor_test.go` |
| Kotlin | package-qualified names, imports, classes, constructor calls, Spring routes, lambdas, extension receivers, navigation calls, function-typed parameters | `extractor_test.go` |
| C# | methods, calls, normalized generic calls, ASP.NET controller routes, minimal API routes, properties, constants, fields, inheritance and implementation edges | `extractor_test.go` |

## Source Selection

Use the same source strategy as the Go regression series:

| Source type | When to use | Example pattern |
|---|---|---|
| Targeted fixture | The parser layer or graph edge is small and easy to isolate | Kotlin trailing lambda call attribution |
| Standard library or framework docs | A framework syntax is canonical and stable | ASP.NET `[HttpGet]`, Spring `@GetMapping`, React Router `<Route>` |
| Real GitHub source | A real project exposes formatting, chaining, or callback syntax not covered by small fixtures | Express route arrays, Axum router chains, Spring annotated controllers |

Keep real-source cases reduced. Record the upstream project and exact shape in
this document or a language-specific companion document before adding the test.

## AST-to-Graph Matrix

| Language | Construct family | Expected AST/grammar area | Expected graph result |
|---|---|---|---|
| JavaScript | `app.post('/api/login', loginHandler)` | `call_expression` with member expression callee | `route POST /api/login`, `handler loginHandler`, `handles` edge |
| JavaScript | `<Route path="/home" element={<Home />} />` | JSX opening/self-closing element | `component_route /home`, `handler Home`, `routes_to` edge |
| TypeScript | `class Service extends Base implements Sink` | class declaration with heritage clauses | `class Service`, `extends Base`, `implements Sink` |
| TypeScript | `profile: Profile` | class property signature | `property Service.profile`, `type_of Profile` |
| Python | `@app.post("/login") def login_view()` | decorated function definition | `route POST /login`, `handler login_view`, `handles` edge |
| Python | `urlpatterns = [path("users/", user_view)]` | call inside list assignment | `route ANY /users/`, `handler user_view`, `handles` edge |
| Rust | `#[get("/health")] async fn health_check()` | attribute item followed by function item | `route GET /health`, `handler health_check`, `handles` edge |
| Rust | `impl Sink for Service {}` | impl item | `struct Service`, `trait Sink`, `implements` edge |
| Java | `@RequestMapping("/api") class C { @PostMapping("/login") void login() {} }` | annotations on class and method | `route POST /api/login`, `handler C.login`, `handles` edge |
| Java | `class Service extends Base implements Sink` | class declaration header | `extends` and `implements` edges |
| Kotlin | `runCatching { port.save(id) }` | call expression with trailing lambda | unresolved call `save` scoped to the enclosing function |
| Kotlin | `fun String.words(limit: Int): List<String>` | function declaration with receiver type | function node `String.words`, `ReceiverType: String`, return metadata |
| C# | `[Route("api/[controller]")] class UsersController { [HttpGet("{id}")] IActionResult Get(...) {} }` | attributes on class and method | `route GET /api/users/{id}`, `handler Get`, `handles` edge |
| C# | `serializer.Deserialize<IList<RootObject>>(reader)` | invocation with generic name | unresolved call `Deserialize`, receiver `serializer` |

## Fixed and Covered Issues

| ID | Language | Construct | Reproduction | Expected result | Previous or guarded actual result | Priority | Status |
|---|---|---|---|---|---|---|---|
| JS-AST-001 | JavaScript | Express route calls | `app.post('/api/login', loginHandler)` | route and handler with `handles` edge | Guarded against being recorded as only an unresolved call | High | Covered |
| JS-AST-002 | JavaScript | React Router JSX | `<Route path="/home" element={<Home />} />` | component route to `Home` | Guards JSX route extraction | Medium | Covered |
| TS-AST-001 | TypeScript | Class property type relationships | `profile: Profile` | `property` node and `type_of Profile` | Guards expanded-kind extraction | Medium | Covered |
| TS-AST-002 | TypeScript | Class header inheritance | `extends Base implements Sink` | `extends` and `implements` edges | Guards class header parsing | Medium | Covered |
| TS-AST-003 | TypeScript | NestJS method object-literal path arrays | `@Get({ path: [':id', 'me'] })` | routes `GET /users/:id` and `GET /users/me` | Guards method decorator object-literal extraction | Medium | Covered |
| PY-AST-001 | Python | Decorated framework routes | `@app.post("/login")` | route and handler with `handles` edge | Guards decorated-definition suppression and route extraction | High | Covered |
| PY-AST-002 | Python | Django URL patterns | `path("users/", user_view)` | route and handler with `handles` edge | Guards list-contained route calls | Medium | Covered |
| RS-AST-001 | Rust | Attribute routes | `#[get("/health")] async fn health_check()` | route and handler with `handles` edge | Guards pending attribute-to-function mapping | High | Covered |
| RS-AST-002 | Rust | Impl relationships | `impl Sink for Service {}` | `Service implements Sink` | Guards expanded impl edge extraction | Medium | Covered |
| JAVA-AST-001 | Java | Spring class and method route prefixes | `@RequestMapping("/api")` + `@PostMapping("/login")` | route `POST /api/login` | Guards annotation prefix composition | High | Covered |
| JAVA-AST-002 | Java | Inheritance and field types | `extends Base implements Sink`, `Profile profile` | `extends`, `implements`, `type_of` edges | Guards expanded-kind parsing | Medium | Covered |
| KT-AST-001 | Kotlin | Trailing lambda call attribution | `runCatching { port.save(id) }` | call `save` attributed to enclosing function | Previously fixed by Kotlin lambda owner logic | High | Fixed |
| KT-AST-002 | Kotlin | Escaping lambdas | `fun makeHandler() = { cleanup() }` | no call attributed to `makeHandler` | Guards over-attribution | High | Covered |
| KT-AST-003 | Kotlin | Extension receiver metadata | `fun String.words(...): List<String>` | receiver and return metadata | Guards receiver extraction | Medium | Covered |
| CS-AST-001 | C# | Generic invocation normalization | `serializer.Deserialize<IList<RootObject>>(reader)` | call name `Deserialize`, receiver `serializer` | Guards generic-name normalization | High | Covered |
| CS-AST-002 | C# | ASP.NET route token expansion | `[Route("api/[controller]")]` + `[HttpGet("{id}")]` | route `GET /api/users/{id}` | Guards controller token expansion | High | Covered |

## Former Open Parser Gaps Fixed in This Pass

| ID | Language | Construct | Reproduction | Expected result | Current result | Priority | Status |
|---|---|---|---|---|---|---|---|
| JS-AST-OPEN-001 | JavaScript | Express routers through exported local aliases | `const api = Router(); api.get(...); export default api` | route associated with the router alias in the parsed file | Parser already handled the selector shape; regression now locks it | Medium | Fixed |
| TS-AST-OPEN-001 | TypeScript | Decorator-based controllers | `@Controller('/users') class C { @Get(':id') get() {} }` | route and handler graph for NestJS-style controllers | NestJS decorators were not modeled | Medium | Fixed |
| TS-AST-OPEN-002 | TypeScript | NestJS static constants and path arrays | `const BASE='/users'; @Controller(BASE); @Get([':id','me'])` | routes `GET /users/:id` and `GET /users/me` | Constant controller prefix was dropped and only the first path array element was emitted | Medium | Fixed |
| TS-AST-OPEN-003 | TypeScript | NestJS guards/interceptors around multiline route decorators | `@UseGuards(...); @Get(\n':id'\n); @UseInterceptors(...)` | route `GET /users/:id` handles `getUser` | Multiline route decorator arguments were read as an empty route path | Medium | Fixed |
| TS-AST-OPEN-004 | TypeScript | NestJS object-literal controller decorators | `@Controller({ version: '1', path: 'users' }); @Get([':id','me'])` and `@Controller({ version: '1' })` | routes use the named `path` property when present; version-only metadata does not become a path prefix | Object-literal decorators were parsed by first quoted string, producing `/1/...` routes | Medium | Fixed |
| PY-AST-OPEN-001 | Python | Class-based views | `app.add_url_rule('/x', view_func=View.as_view(...))` | route to class-based handler | Only direct decorator/Django path shapes were modeled | Medium | Fixed |
| PY-AST-OPEN-002 | Python | FastAPI `APIRouter` prefixes | `router = APIRouter(prefix="/api"); @router.get("/users")` | route `GET /api/users` handles `list_users` | Decorator route was found, but router prefix was dropped | High | Fixed |
| RS-AST-OPEN-001 | Rust | Deep Axum router nesting | `Router::new().nest("/api", Router::new().route(...))` | combined nested route graph | Nested router expression composition was shallow | Medium | Fixed |
| RS-AST-OPEN-002 | Rust | Axum method router chains | `route("/users", get(list_users).post(create_user))` | separate `GET /users` and `POST /users` routes | Only the outer `post(create_user)` route was emitted | Medium | Fixed |
| JAVA-AST-OPEN-001 | Java | Meta-annotations and composed Spring annotations | custom annotation wrapping `@GetMapping` | route resolved through meta-annotation | Annotation definitions were not resolved locally | Low | Fixed |
| JAVA-AST-OPEN-002 | Java | Spring multi-method `@RequestMapping` | `@RequestMapping(value="/users", method={GET,POST})` | separate `GET /users` and `POST /users` routes | Only the first method in the annotation text was emitted | Medium | Fixed |
| KT-AST-OPEN-001 | Kotlin | Coroutine builders as deferred execution | `launch { doWork() }` | nested `doWork` is not attributed to the enclosing setup function | It was intentionally over-attributed before this pass | Low | Fixed |
| KT-AST-OPEN-002 | Kotlin | Ktor nested route DSL | `routing { route("/api") { get("/users") { listUsers() } } }` | route `GET /api/users` handles `listUsers` | Ktor DSL route scopes were not modeled | High | Fixed |
| CS-AST-OPEN-001 | C# | Endpoint route groups | `app.MapGroup("/api").MapGet("/users", Handler)` | combined minimal API route | Minimal API group chaining was not modeled | Medium | Fixed |
| CS-AST-OPEN-002 | C# | Nested endpoint route groups | `api.MapGroup("/v1"); v1.MapGet("/users", Handler)` | route `GET /api/v1/users` handles `Handler` | Child group prefix lost the parent group prefix | Medium | Fixed |

The entries above are kept under their original IDs to preserve traceability
from the former open-gap list to the tests in
`TestExtractFromSourceFixesDocumentedOpenLanguageGaps`.

## Former Open Indexer Gaps Fixed in This Pass

| ID | Language | Construct | Reproduction | Expected result | Current result | Priority | Status |
|---|---|---|---|---|---|---|---|
| JS-IDX-OPEN-001 | JavaScript | Cross-file Express router ownership | `routes/users.js` exports `api`; `app.js` imports it and calls `app.use('/api', api)` | indexer composes mounted prefix into target router routes | `GET /api/users` is emitted with a `handles` edge to `listUsers` | Medium | Fixed |
| JS-IDX-OPEN-002 | JavaScript | Express router ownership through named re-export barrels | `routes/users.js` exports `api`; `routes/index.js` re-exports it as `usersRouter`; `app.js` imports `{ usersRouter }` and mounts it | indexer follows named import and re-export to the source router file | `GET /api/users` is emitted with a `handles` edge to `listUsers` | Medium | Fixed |
| PY-IDX-OPEN-001 | Python | Cross-file FastAPI router includes | `routes/users.py` exports `router`; `main.py` imports it and calls `app.include_router(router, prefix="/v1")` | indexer composes application include prefix with router-local prefixes | `GET /v1/api/users` is emitted with a `handles` edge to `list_users` | High | Fixed |
| PY-IDX-OPEN-002 | Python | FastAPI include_router with noncanonical router variable names | `users_router = APIRouter(...); from routes.users import users_router; app.include_router(users_router, prefix="/v1")` | indexer recognizes imported APIRouter variable names from the target module | `GET /v1/api/users` is emitted with a `handles` edge to `list_users` | Medium | Fixed |
| PY-IDX-OPEN-003 | Python | FastAPI include_router with multiple routers in one module | `users_router` and `admin_router` in the same file; only `users_router` is imported and included | indexer mounts only routes declared on the imported router variable | `GET /v1/users/` is emitted and `GET /v1/admin/` is not emitted | High | Fixed |
| PY-IDX-OPEN-004 | Python | FastAPI package-level router re-exports | `routes/__init__.py` re-exports `users_router`; `main.py` imports it with `from routes import users_router` | indexer resolves the package barrel to the source router file | `GET /v1/users/` is emitted with a `handles` edge to `list_users` | Medium | Fixed |

The JavaScript parser still keeps `ExtractFromSource` single-file. Cross-file
router ownership is now handled by the indexer after all files have been
parsed, using local default/CommonJS imports and Express `use(prefix, alias)`
mounts. FastAPI application-level `include_router(..., prefix=...)`
composition is also handled after all Python files have been parsed, using
local `from module import ...` imports whose imported symbol is assigned from
`APIRouter(...)` in the target module. When a module defines multiple routers,
the include pass now matches route decorators to the imported router variable
before cloning mounted route nodes.
Named JavaScript imports and one or more local re-export barrels are resolved
before Express router mount composition.

## Current Open Findings

| ID | Language | Construct | Reproduction | Expected node/edge structure | Actual behavior | Priority | Status |
|---|---|---|---|---|---|---|---|
| None currently known | - | - | - | - | - | - | - |

The table should stay present even when parser-level open findings are empty.
When an iteration discovers a new parser failure, add it here before fixing it
unless the failing test and fix happen in the same interaction.

## Iteration Log

### 2026-05-23-route-and-dsl-gap-pass-1

Languages tested:

- JavaScript
- TypeScript
- Python
- Rust
- Java
- Kotlin
- C#

Constructs:

- Local/exported Express router alias
- NestJS-style TypeScript decorators
- Flask class-based `add_url_rule`
- Nested Axum `nest(... route(...))`
- Spring composed/meta-annotations
- Kotlin deferred coroutine builders
- ASP.NET Minimal API route groups

Sources:

- Reduced targeted fixtures in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps`.
- Existing Go/Kotlin parser work used as reference for expected graph
  contracts: focused fixture, explicit expected graph shape, red-green parser
  fix, status table update.

Expected graph:

- Framework routes become `route` nodes.
- Handler symbols become `handler` nodes.
- Route-to-handler relationships use `handles`.
- Deferred Kotlin builder lambdas do not attribute nested calls to the enclosing
  synchronous owner.
- C# and Rust route prefixes compose into the final route pattern when the
  prefix and nested route are in the same parsed snippet.

Actual result before fixes:

- JavaScript local router alias already produced the expected route graph.
- TypeScript NestJS decorators produced class/method nodes but no route.
- Python `add_url_rule` produced no route.
- Rust nested Axum emitted separate `/api` and `/users` routes, not
  `/api/users`.
- Java Spring meta-annotation produced a method node, not a route.
- Kotlin `launch { doWork() }` over-attributed `doWork` to the enclosing
  function.
- C# `MapGroup("/api")` did not prefix the nested `MapGet`.

Outcome:

- Parser regressions were added and fixed for all parser-level gaps above.
- JavaScript cross-file module ownership was split out as `JS-IDX-OPEN-001`
  because it requires indexer-level research rather than a single-file parser
  expectation.

Verification:

- `go test ./internal/astgraph/parser` passed.
- `go test ./internal/astgraph/...` passed.
- `go test ./...` still fails on unrelated
  `TestInstallAgentWritesBundledSkill` in `internal/cli`.

Next hypothesis:

- Move from single-file parser fixtures to one indexer-level JavaScript router
  module test.
- Add another parser iteration around language-specific type systems rather
  than route DSLs.

### 2026-05-23-javascript-indexer-router-mount-pass-1

Languages tested:

- JavaScript

Constructs:

- Cross-file Express router mounting with a default import.
- `app.use("/api", api)` prefix composition against routes declared in
  `routes/users.js`.

Sources:

- Reduced targeted fixture in
  `TestIndexerComposesJavaScriptExpressRouterMountsAcrossFiles`.
- The fixture mirrors common Express module layout: router file exports a local
  `Router()` alias, application file imports and mounts it.

Expected graph:

- `routes/users.js` still produces the parser-level route `GET /users`.
- The indexer adds a composed route node `GET /api/users`.
- The composed route keeps a `handles` edge to the existing `listUsers`
  handler node.

Actual result before fix:

- Parser output contained `USE /api -> api` in `app.js`.
- Parser output contained `GET /users -> listUsers` in `routes/users.js`.
- No graph node connected the mounted prefix to the imported router file.

Outcome:

- Added an indexer post-pass for JavaScript Express router mounts.
- The post-pass scans indexed JavaScript sources for local default/CommonJS
  imports and `use(prefix, alias)` mounts, resolves the imported file, and
  emits composed route nodes for routes in the mounted router file.
- `SchemaVersion` was bumped to `29` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph -run TestIndexerComposesJavaScriptExpressRouterMountsAcrossFiles` failed because `GET /api/users` was absent.
- Green check: `go test ./internal/astgraph -run 'TestIndexerComposesJavaScriptExpressRouterMountsAcrossFiles|TestSchemaVersionBumpedForJavaScriptRouterMountComposition'` passed.

Next hypothesis:

- Move to `H-PY-001` or `H-KT-001` for nested router/DSL prefix stacks.
- Add a future JavaScript pressure test for named imports and re-exported
  router modules if real repositories show that pattern.

### 2026-05-23-python-fastapi-apirouter-prefix-pass-1

Languages tested:

- Python

Constructs:

- FastAPI `APIRouter(prefix="/api")` variable assignment.
- Decorator route on the same router alias: `@router.get("/users")`.

Sources:

- Reduced targeted fixture in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps/python fastapi apirouter prefix`.
- The fixture mirrors the canonical FastAPI router module pattern, where a
  module-level router owns a prefix and function decorators add subpaths.

Expected graph:

- Route node `GET /api/users`.
- Handler node `list_users`.
- `GET /api/users` has a `handles` edge to `list_users`.

Actual result before fix:

- Handler node `list_users` was emitted.
- Route node `GET /users` was emitted.
- The parser lost the `APIRouter(prefix="/api")` assignment before processing
  the decorator receiver `router`.

Outcome:

- Added parser-side prefix extraction for local `APIRouter(prefix=...)`
  assignments.
- Python decorator route extraction now combines a matching router receiver
  prefix with the decorator path.

Verification:

- Red check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/python_fastapi_apirouter_prefix'` failed because `GET /api/users` was absent and `GET /users` was emitted.
- Green check: the same command passed after the parser fix.

Next hypothesis:

- Add a follow-up Python indexer test for `app.include_router(router,
  prefix="/v1")`, which composes an application-level prefix with the router
  module prefix.
- Continue with `H-KT-001` for Kotlin Ktor nested route DSL scope stacks.

### 2026-05-23-kotlin-ktor-route-dsl-pass-1

Languages tested:

- Kotlin

Constructs:

- Ktor `routing { ... }` DSL blocks.
- Nested `route("/api")` prefix blocks.
- HTTP verb blocks such as `get("/users") { listUsers() }`.

Sources:

- Reduced targeted fixture in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps/kotlin ktor nested route dsl`.
- The fixture mirrors the common Ktor module shape
  `fun Application.module() { routing { route(...) { get(...) { ... } } } }`.

Expected graph:

- Route node `GET /api/users`.
- Handler node `listUsers`.
- `GET /api/users` has a `handles` edge to `listUsers`.

Actual result before fix:

- Kotlin function nodes `Application.module` and `listUsers` were emitted.
- No Ktor route node or route-to-handler edge was emitted.
- Existing Kotlin lambda logic only controlled call attribution; it did not
  translate Ktor DSL blocks into graph routes.

Outcome:

- Added a Kotlin Ktor route post-pass with a small scope stack over source
  lines.
- The pass composes nested `route(...)` prefixes with HTTP verb blocks and
  records the first handler call in the endpoint block.
- `SchemaVersion` was bumped to `31` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/kotlin_ktor_nested_route_dsl'` failed because `GET /api/users` was absent.
- Green check: the same command passed after the parser fix.

Next hypothesis:

- Add a Ktor pressure test for multiple HTTP verb blocks inside the same
  `route(...)` scope.
- Continue with `H-RS-001` for Axum method routers such as
  `get(list).post(create)`.

### 2026-05-23-rust-axum-method-router-chain-pass-1

Languages tested:

- Rust

Constructs:

- Axum `Router::new().route(...)`.
- Method-router chains in the second route argument:
  `get(list_users).post(create_user)`.

Sources:

- Reduced targeted fixture in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps/rust axum method router chain`.
- The fixture mirrors Axum's common pattern of attaching several HTTP methods
  to the same path with chained method routers.

Expected graph:

- Route node `GET /users` with a `handles` edge to `list_users`.
- Route node `POST /users` with a `handles` edge to `create_user`.

Actual result before fix:

- Route node `POST /users` was emitted.
- Handler node `create_user` was emitted.
- The inner `get(list_users)` call was lost because route extraction only used
  the outermost method-router call.

Outcome:

- Added Axum handler-chain extraction for method-router route arguments.
- The parser now emits one route per HTTP method call in
  `get(...).post(...).put(...)` style chains.
- `SchemaVersion` was bumped to `32` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/rust_axum_method_router_chain'` failed because `GET /users` was absent.
- Green check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/rust_axum_method_router_chain|TestExtractFromSourceFixesDocumentedOpenLanguageGaps/rust_nested_axum_router'` passed.

Next hypothesis:

- Add a Rust pressure test for Axum layers or services around handlers, for
  example `get(list_users).layer(...)`.
- Continue with `H-CS-001` for nested ASP.NET `MapGroup` chains.

### 2026-05-23-csharp-nested-mapgroup-pass-1

Languages tested:

- C#

Constructs:

- ASP.NET Minimal API `MapGroup` prefix assignment.
- Nested group assignment: `var v1 = api.MapGroup("/v1")`.
- Endpoint on nested group: `v1.MapGet("/users", ListUsers)`.

Sources:

- Reduced targeted fixture in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps/csharp nested minimal api route groups`.
- The fixture mirrors the common Minimal API pattern of building grouped
  endpoint prefixes in steps.

Expected graph:

- Route node `GET /api/v1/users`.
- Handler node `ListUsers`.
- `GET /api/v1/users` has a `handles` edge to `ListUsers`.

Actual result before fix:

- Route node `GET /v1/users` was emitted.
- Handler node `ListUsers` was emitted.
- The parser stored `v1`'s direct `MapGroup("/v1")` prefix but did not compose
  it with the parent receiver `api`.

Outcome:

- `csharpRouteGroupPrefixes` now records the receiver for each `MapGroup`
  assignment and composes the child prefix with any known parent group prefix.
- `SchemaVersion` was bumped to `33` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/csharp_nested_minimal_api_route_groups'` failed because `GET /api/v1/users` was absent and `GET /v1/users` was emitted.
- Green check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/csharp_nested_minimal_api_route_groups|TestExtractFromSourceFixesDocumentedOpenLanguageGaps/csharp_minimal_api_route_group'` passed.

Next hypothesis:

- Continue with `H-JAVA-001` for Spring `@RequestMapping(method={GET,POST})`
  multi-method arrays.
- Add a future C# pressure test for endpoint filters chained after grouped
  endpoints.

### 2026-05-23-java-spring-multi-method-pass-1

Languages tested:

- Java

Constructs:

- Spring `@RequestMapping`.
- Multi-method annotation argument:
  `method={RequestMethod.GET, RequestMethod.POST}`.

Sources:

- Reduced targeted fixture in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps/java spring request mapping method array`.
- The fixture mirrors Spring controllers that map one handler method to several
  HTTP methods.

Expected graph:

- Route node `GET /users` with a `handles` edge to `UsersController.users`.
- Route node `POST /users` with a `handles` edge to `UsersController.users`.

Actual result before fix:

- Route node `GET /users` was emitted.
- Route node `POST /users` was absent.
- `requestMappingMethod` returned only the first method found in the annotation
  text.

Outcome:

- Added `springHTTPMethods` and `requestMappingMethods`.
- Spring route extraction now emits a route for each HTTP method in
  `RequestMapping` method arrays.
- `SchemaVersion` was bumped to `34` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/java_spring_request_mapping_method_array'` failed because `POST /users` was absent.
- Green check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/java_spring_request_mapping_method_array|TestExtractFromSourceFindsSpringJavaRequestMappingMethod'` passed.

Next hypothesis:

- Add a future Java pressure test for path arrays and method arrays combined,
  for example `value={"/users","/members"}` with `{GET,POST}`.
- The original hypothesis table is now exhausted; continue with per-language
  recommendations or real GitHub fixtures for deeper coverage.

### 2026-05-23-typescript-nestjs-static-routes-pass-1

Languages tested:

- TypeScript

Constructs:

- NestJS `@Controller(BASE)` where `BASE` is a local string constant.
- NestJS path arrays in method decorators: `@Get([':id', 'me'])`.

Sources:

- Reduced targeted fixture in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript nestjs constants and path arrays`.
- The fixture mirrors NestJS controllers that centralize route prefixes in
  constants and map one handler to several static paths.

Expected graph:

- Route node `GET /users/:id` with a `handles` edge to `getUser`.
- Route node `GET /users/me` with a `handles` edge to `getUser`.

Actual result before fix:

- Route node `GET /:id` was emitted.
- Route node `GET /users/me` was absent.
- The TypeScript decorator post-pass did not resolve local constants and only
  used the first quoted decorator path.

Outcome:

- Added local string constant extraction for the TypeScript decorator post-pass.
- Decorator path extraction now handles multiple quoted values and emits one
  route per path.
- `SchemaVersion` was bumped to `35` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_constants_and_path_arrays'` failed because `GET /users/:id` and `GET /users/me` were not both emitted.
- Green check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_constants_and_path_arrays|TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_decorator_controller'` passed.

Next hypothesis:

- Add real NestJS fixtures with guards/interceptors to verify decorators around
  route decorators do not break handler association.
- The original hypothesis table is now fully fixed; future iterations should be
  selected from the per-language recommendations.

### 2026-05-23-python-fastapi-include-router-pass-1

Languages tested:

- Python

Constructs:

- FastAPI router module with `router = APIRouter(prefix="/api")`.
- Application-level `app.include_router(router, prefix="/v1")` in another
  module.

Sources:

- Reduced targeted indexer fixture in
  `TestIndexerComposesPythonFastAPIIncludeRouterPrefixesAcrossFiles`.

Expected graph:

- Route node `GET /v1/api/users`.
- The included route keeps a `handles` edge to `list_users`.

Actual result before fix:

- The parser emitted the router-local route `GET /api/users`.
- The indexer did not compose the app-level include prefix, so
  `GET /v1/api/users` was absent.

Outcome:

- Added a Python FastAPI indexer post-pass that resolves
  `from module import router` aliases and composes `include_router` prefixes
  with FastAPI route nodes from the imported module.
- `SchemaVersion` was bumped to `36` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph -run TestIndexerComposesPythonFastAPIIncludeRouterPrefixesAcrossFiles` failed because `GET /v1/api/users` was absent.
- Green check: `go test ./internal/astgraph -run 'TestIndexerComposesPythonFastAPIIncludeRouterPrefixesAcrossFiles|TestSchemaVersionBumpedForPythonFastAPIIncludeRouterPrefixes'` passed.

Next hypothesis:

- Add a real FastAPI fixture with multiple routers or aliased router imports to
  verify import resolution beyond the basic `router` export.

### 2026-05-23-javascript-named-router-reexport-pass-1

Languages tested:

- JavaScript

Constructs:

- Express router declared in one module: `export const api = Router()`.
- Barrel re-export: `export { api as usersRouter } from "./users"`.
- Named import mount: `import { usersRouter } from "./routes"` followed by
  `app.use("/api", usersRouter)`.

Sources:

- Reduced targeted indexer fixture in
  `TestIndexerComposesJavaScriptExpressRouterMountsThroughNamedReExports`.

Expected graph:

- Route node `GET /api/users`.
- The mounted route keeps a `handles` edge to `listUsers`.

Actual result before fix:

- The parser emitted the source route `GET /users`.
- The app module emitted only `USE /api -> usersRouter`.
- The indexer did not resolve named imports or re-export barrels, so
  `GET /api/users` was absent.

Outcome:

- Added JavaScript named import parsing for router mount ownership.
- Added local named re-export resolution for barrel modules before composing
  Express mount prefixes.
- `SchemaVersion` was bumped to `37` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph -run TestIndexerComposesJavaScriptExpressRouterMountsThroughNamedReExports` failed because `GET /api/users` was absent.
- Green check: `go test ./internal/astgraph -run 'TestIndexerComposesJavaScriptExpressRouterMountsThroughNamedReExports|TestSchemaVersionBumpedForJavaScriptNamedRouterReExports'` passed.

Next hypothesis:

- Add a real Express project fixture with nested router barrels and multiple
  mounted routers to verify that unrelated re-exports do not produce duplicate
  mounted route nodes.

### 2026-05-23-typescript-nestjs-multiline-decorator-pass-1

Languages tested:

- TypeScript

Constructs:

- NestJS controller with class-level `@UseGuards`.
- Method-level `@UseGuards`, `@UseInterceptors`, and a multiline `@Get(...)`
  route decorator.

Sources:

- Reduced targeted fixture in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript nestjs guards interceptors and multiline route decorator`.

Expected graph:

- Route node `GET /users/:id`.
- The route has a `handles` edge to `getUser`.

Actual result before fix:

- The handler was detected.
- The route was emitted as `GET users/`, because the line-based decorator
  post-pass treated `@Get(` as a decorator with an empty route argument and did
  not normalize the slashless controller prefix before combining paths.

Outcome:

- TypeScript decorator extraction now collects multiline decorator calls before
  route analysis.
- TypeScript decorator route prefix and method paths are normalized before
  being combined.
- `SchemaVersion` was bumped to `38` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_guards_interceptors_and_multiline_route_decorator'` failed because `GET /users/:id` was absent.
- Green check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_guards_interceptors_and_multiline_route_decorator'` passed.

Next hypothesis:

- Add a real NestJS fixture with `@Controller({ path: ..., version: ... })` and
  method arrays to verify object-literal decorator arguments.

### 2026-05-23-python-fastapi-named-router-import-pass-1

Languages tested:

- Python

Constructs:

- FastAPI router variable named `users_router`.
- Cross-file import with `from routes.users import users_router`.
- Application-level `app.include_router(users_router, prefix="/v1")`.

Sources:

- Reduced targeted indexer fixture in
  `TestIndexerComposesPythonFastAPIIncludeRouterPrefixesForNamedRouterImports`.

Expected graph:

- Route node `GET /v1/api/users`.
- The included route keeps a `handles` edge to `list_users`.

Actual result before fix:

- The parser emitted the router-local route `GET /api/users`.
- The indexer did not compose the application prefix because it only treated
  imported symbols named `router` as FastAPI routers.

Outcome:

- Python FastAPI include-router import resolution now reads `APIRouter(...)`
  assignment names from the imported module and accepts those names when
  mapping `include_router` aliases to route files.
- `SchemaVersion` was bumped to `39` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph -run TestIndexerComposesPythonFastAPIIncludeRouterPrefixesForNamedRouterImports` failed because `GET /v1/api/users` was absent.
- Green check: `go test ./internal/astgraph -run 'TestIndexerComposesPythonFastAPIIncludeRouterPrefixesForNamedRouterImports|TestSchemaVersionBumpedForPythonFastAPINamedRouterImports'` passed.

Next hypothesis:

- Add a FastAPI fixture with multiple routers in one module to verify that an
  included router does not accidentally mount unrelated routes from the same
  file.

### 2026-05-23-python-fastapi-multiple-router-filter-pass-1

Languages tested:

- Python

Constructs:

- One module defining `users_router = APIRouter(...)` and
  `admin_router = APIRouter(...)`.
- Application import of only `users_router`.
- Application-level `app.include_router(users_router, prefix="/v1")`.

Sources:

- Reduced targeted indexer fixture in
  `TestIndexerComposesPythonFastAPIIncludeRouterPrefixesForOnlyImportedRouter`.

Expected graph:

- Route node `GET /v1/users/` with a `handles` edge to `list_users`.
- No mounted route node `GET /v1/admin/`, because `admin_router` was not
  imported or included by the application.

Actual result before fix:

- The indexer mounted all FastAPI routes from the imported file, including the
  unrelated `admin_router` route.

Outcome:

- FastAPI route composition now tracks which APIRouter variable owns each
  route decorator and filters mounted routes by the imported router symbol.
- Route decorator line matching now uses the router-name capture offset, so
  leading whitespace or blank lines do not shift router ownership to the wrong
  line.
- `SchemaVersion` was bumped to `40` so existing graph caches are rebuilt.

Verification:

- Red check: `go test ./internal/astgraph -run TestIndexerComposesPythonFastAPIIncludeRouterPrefixesForOnlyImportedRouter` failed because `GET /v1/admin/` was incorrectly emitted.
- Green check: `go test ./internal/astgraph -run 'TestIndexerComposesPythonFastAPIIncludeRouterPrefixesForOnlyImportedRouter|TestSchemaVersionBumpedForPythonFastAPIMultipleRouterFiltering'` passed.

Follow-up coverage:

- FastAPI package-level re-export fixtures are covered by
  `TestIndexerComposesPythonFastAPIIncludeRouterPrefixesThroughPackageReExports`.
- Green check: `go test ./internal/astgraph -run TestIndexerComposesPythonFastAPIIncludeRouterPrefixesThroughPackageReExports -count=1` passed.

Next hypothesis:

- Add FastAPI wildcard or aliased package-barrel imports if that shape appears
  in real projects.

### 2026-05-23-typescript-nestjs-object-decorator-pass-1

Languages tested:

- TypeScript

Constructs:

- NestJS controller decorator with an object-literal argument:
  `@Controller({ version: '1', path: 'users' })`.
- Version-only controller metadata: `@Controller({ version: '1' })`.
- NestJS method decorator with a path array: `@Get([':id', 'me'])`.
- NestJS method decorator with an object-literal path array:
  `@Get({ path: [':id', 'me'] })`.

Sources:

- Reduced targeted fixture in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript nestjs controller object path and method arrays`.
- Reduced targeted fixture in
  `TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript nestjs method object path array`.
- The fixture mirrors NestJS controllers that combine version metadata with a
  route prefix object.

Expected graph:

- Route node `GET /users/:id` with a `handles` edge to `getUser`.
- Route node `GET /users/me` with a `handles` edge to `getUser`.
- Route node `GET /health` for version-only controller metadata.
- Method object-literal path arrays also emit both method routes under the
  controller prefix.
- No route nodes under `/1/...`, because `version` is metadata, not the route
  prefix.

Actual result before fix:

- The parser emitted `GET /1/:id`, `GET /1/me`, and `GET /1/health`.
- `typescriptDecoratorPatterns` read all quoted strings and treated the first
  one as the route path, so `version: '1'` won over `path: 'users'`.

Outcome:

- TypeScript decorator pattern extraction now detects object-literal decorator
  arguments and reads only named `path` or `value` properties.
- Object-literal decorators without a path/value property now produce no route
  prefix instead of falling back to unrelated metadata strings.
- Method object-literal path arrays are covered by an explicit regression guard.
- Positional strings, path arrays, and string constants remain supported.
- `SchemaVersion` was bumped to `41` so existing graph caches are rebuilt.

Verification:

- Red checks:
  - `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_controller_object_path_and_method_arrays' -count=1` failed because `GET /users/:id` was absent and `/1/...` routes were emitted.
  - `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_controller_object_version_without_path' -count=1` failed because `GET /health` was absent and `GET /1/health` was emitted.
- Green check: `go test ./internal/astgraph/parser -run 'TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_controller_object_path_and_method_arrays|TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_controller_object_version_without_path|TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_method_object_path_array|TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_constants_and_path_arrays|TestExtractFromSourceFixesDocumentedOpenLanguageGaps/typescript_nestjs_guards_interceptors_and_multiline_route_decorator' -count=1` passed.

Next hypothesis:

- Add a real NestJS fixture that mixes controller object-literal route prefixes,
  method object-literal path arrays, guards, and interceptors in one class.

## Per-Language Recommendations

| Language | Next regression source to add | Why |
|---|---|---|
| JavaScript | Real Express project fixture with nested router barrels and multiple mounted routers | Single-level named re-export barrels are covered; denser routing indexes may still create duplicate or ambiguous ownership |
| TypeScript | Real NestJS controller mixing object-literal controller prefixes, method object-literal arrays, guards, and interceptors | Reduced cases are covered independently; dense real controllers may still expose decorator association gaps |
| Python | FastAPI wildcard or aliased package-barrel imports from real projects | Direct package re-exports are covered; wildcard and alias forms may still hide router ownership |
| Rust | Real Axum nested router file with state/layers | Nested prefixes and method-router chains are covered; layers and services may still hide handlers |
| Java | Spring controller combining path arrays and method arrays | Multi-method routes are covered; cross product of path arrays and method arrays needs pressure |
| Kotlin | Ktor multiple verb blocks and nested `route` chains from real projects | Basic Ktor route DSL graphing is covered; denser routing tables may still expose scope-stack gaps |
| C# | ASP.NET Minimal API endpoint filters on grouped endpoints | Nested MapGroup prefix composition is covered; filters may still hide endpoint declarations |

## Next Test Hypotheses

| ID | Language | Hypothesis | Candidate snippet/source | Expected graph | Priority | Status |
|---|---|---|---|---|---|---|
| H-JS-001 | JavaScript | Cross-file router mount requires indexer-level source integration, not parser-only extraction | `routes/users.js` exports `api`; `app.js` imports it and calls `app.use('/api', api)` | `GET /api/users` route with `handles` edge to `listUsers` | Medium | Fixed |
| H-JS-IDX-002 | JavaScript | Named imports through barrel modules may hide Express router ownership | `export { api as usersRouter } from "./users"` and `import { usersRouter } from "./routes"` | `GET /api/users` route with `handles` edge to `listUsers` | Medium | Fixed |
| H-TS-001 | TypeScript | NestJS decorators with arrays or constants may not resolve static paths | `@Controller(BASE)`, `@Get([':id', 'me'])` | One or more route nodes with resolved or explicitly unresolved path metadata | Medium | Fixed |
| H-TS-002 | TypeScript | Guards/interceptors plus multiline route decorators may break handler association or route path extraction | `@UseGuards(...); @Get(\n':id'\n); @UseInterceptors(...)` | `GET /users/:id` handles `getUser` | Medium | Fixed |
| H-TS-003 | TypeScript | NestJS object-literal controller decorators may confuse metadata strings with route path strings | `@Controller({ version: '1', path: 'users' })`; `@Controller({ version: '1' })` | `GET /users/:id`, `GET /users/me`, and `GET /health`; not `/1/...` routes | Medium | Fixed |
| H-TS-004 | TypeScript | NestJS object-literal method decorators may hide path arrays behind named object properties | `@Get({ path: [':id', 'me'] })` | `GET /users/:id` and `GET /users/me` handle `getUser` | Medium | Covered |
| H-PY-001 | Python | FastAPI `APIRouter(prefix="/api")` routes do not compose prefixes yet | `router = APIRouter(prefix="/api"); @router.get("/users")` | `GET /api/users` handles function | High | Fixed |
| H-PY-IDX-001 | Python | FastAPI application-level include prefixes need indexer-level source integration | `from routes.users import router`; `app.include_router(router, prefix="/v1")` | `GET /v1/api/users` route with `handles` edge to `list_users` | High | Fixed |
| H-PY-IDX-002 | Python | FastAPI include_router may miss routers whose variable name is not `router` | `users_router = APIRouter(...); from routes.users import users_router` | `GET /v1/api/users` route with `handles` edge to `list_users` | Medium | Fixed |
| H-PY-IDX-003 | Python | FastAPI include_router may mount unrelated routes when several routers live in one source file | `users_router` and `admin_router` in one file, only `users_router` included | only `GET /v1/users/` is mounted; `GET /v1/admin/` is not mounted | High | Fixed |
| H-PY-IDX-004 | Python | FastAPI package-level re-export imports may hide router ownership | `routes/__init__.py` uses `from .users import users_router`; `main.py` uses `from routes import users_router` | `GET /v1/users/` route with `handles` edge to `list_users` | Medium | Fixed |
| H-RS-001 | Rust | Axum route handlers wrapped in layers or method routers may hide the handler | `route("/users", get(list).post(create))` | `GET /users` and `POST /users` route edges | Medium | Fixed |
| H-JAVA-001 | Java | Spring `@RequestMapping(method={GET,POST})` may collapse multi-method routes | method array in annotation | separate `GET` and `POST` route nodes or documented policy | Medium | Fixed |
| H-KT-001 | Kotlin | Ktor nested route DSL likely needs scope-stack handling | `routing { route("/api") { get("/users") { list() } } }` | `GET /api/users` route and handler/call relationship | High | Fixed |
| H-CS-001 | C# | Nested `MapGroup` chains may only use the first group prefix | `api.MapGroup("/v1").MapGet("/users", Handler)` | `GET /api/v1/users` handles `Handler` | Medium | Fixed |

## Maintenance Rules

1. Add a reduced parser test before changing extraction logic.
2. Record the construct in this document with source, expected graph, and
   status.
3. If a case is intentionally unsupported, document it as an open gap with a
   priority.
4. Run the language-specific test subset and `go test ./internal/astgraph/...`.
5. Keep broad `extractor_test.go` smoke coverage, but move growing language
   matrices into language-specific test files when they become hard to scan.
