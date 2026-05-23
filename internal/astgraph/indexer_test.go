package astgraph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexerBuildsGraphFromSourceDirectory(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
func helper() {}
func main() { helper() }
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("Files = %#v", result.Files)
	}
	if !hasNode(result.Nodes, NodeKindFunction, "helper") || !hasNode(result.Nodes, NodeKindFunction, "main") {
		t.Fatalf("Nodes = %#v", result.Nodes)
	}
	mainID := nodeIDByName(t, result.Nodes, NodeKindFunction, "main")
	helperID := nodeIDByName(t, result.Nodes, NodeKindFunction, "helper")
	if !hasEdge(result.Edges, mainID, helperID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want main -> helper calls edge", result.Edges)
	}
}

func TestIndexerBuildsContainmentGraphAndKeepsCallEdges(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
type Service struct {
	Name string
}
func helper() {}
func (s Service) Run() { helper() }
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	fileID := nodeIDByName(t, result.Nodes, NodeKindFile, "main.go")
	moduleID := nodeIDByName(t, result.Nodes, NodeKindModule, "main")
	serviceID := nodeIDByName(t, result.Nodes, NodeKindStruct, "Service")
	fieldID := nodeIDByName(t, result.Nodes, NodeKindField, "Name")
	runID := nodeIDByName(t, result.Nodes, NodeKindMethod, "Run")
	helperID := nodeIDByName(t, result.Nodes, NodeKindFunction, "helper")

	for _, edge := range []struct {
		from string
		to   string
		kind EdgeKind
	}{
		{fileID, moduleID, EdgeKindContains},
		{moduleID, serviceID, EdgeKindContains},
		{serviceID, fieldID, EdgeKindContains},
		{serviceID, runID, EdgeKindContains},
		{runID, helperID, EdgeKindCalls},
	} {
		if !hasEdge(result.Edges, edge.from, edge.to, edge.kind) {
			t.Fatalf("Edges = %#v, want %s -> %s %s", result.Edges, edge.from, edge.to, edge.kind)
		}
	}
	if len(result.Edges) <= 1 {
		t.Fatalf("Edges = %#v, want contains edges included in graph edge count", result.Edges)
	}
}

func TestSchemaVersionBumpedForTypeScriptNestJSObjectDecoratorPaths(t *testing.T) {
	if SchemaVersion != 41 {
		t.Fatalf("SchemaVersion = %d, want 41 for TypeScript NestJS object decorator path reindex", SchemaVersion)
	}
}

func TestIndexerResolvesUnambiguousCallsToEdges(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
import "fmt"
func helper() {}
func main() {
	helper()
	fmt.Println("ok")
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	mainID := nodeIDByName(t, result.Nodes, NodeKindFunction, "main")
	helperID := nodeIDByName(t, result.Nodes, NodeKindFunction, "helper")
	if !hasEdge(result.Edges, mainID, helperID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want main -> helper calls edge", result.Edges)
	}
	if hasUnresolved(result.Unresolved, mainID, "helper") {
		t.Fatalf("Unresolved = %#v, helper should have resolved", result.Unresolved)
	}
	if hasUnresolved(result.Unresolved, mainID, "Println") {
		t.Fatalf("Unresolved = %#v, external Println should have been filtered", result.Unresolved)
	}
}

func TestIndexerKeepsAmbiguousCallsUnresolved(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
func main() { helper() }
`)
	writeASTGraphFixture(t, root, "a.go", `package main
func helper() {}
`)
	writeASTGraphFixture(t, root, "b.go", `package main
func helper() {}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	mainID := nodeIDByName(t, result.Nodes, NodeKindFunction, "main")
	if hasEdgeKind(result.Edges, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, ambiguous helper call should not resolve to a calls edge", result.Edges)
	}
	if !hasUnresolved(result.Unresolved, mainID, "helper") {
		t.Fatalf("Unresolved = %#v, want ambiguous helper to remain unresolved", result.Unresolved)
	}
}

func TestIndexerResolvesOverloadedCallsByArgumentCount(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "Box.kt", `class Box {
  fun pick(): Int { return 0 }
  fun pick(value: String): Int { return 1 }
  fun run() { pick("x") }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	pickWithArgID := nodeIDByNameAndParamCount(t, result.Nodes, NodeKindFunction, "pick", 1)
	if !hasEdge(result.Edges, runID, pickWithArgID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want run -> pick(String)", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "pick") {
		t.Fatalf("Unresolved = %#v, pick(String) should have resolved", result.Unresolved)
	}
}

func TestIndexerResolvesJavaScriptCallsWithOmittedOptionalArguments(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "clone.js", `function baseClone(value, bitmask, customizer, key, object, stack) {
  return value;
}
function clone(value) {
  return baseClone(value, 4);
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	cloneID := nodeIDByName(t, result.Nodes, NodeKindFunction, "clone")
	baseCloneID := nodeIDByName(t, result.Nodes, NodeKindFunction, "baseClone")
	if !hasEdge(result.Edges, cloneID, baseCloneID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want clone -> baseClone with omitted optional arguments", result.Edges)
	}
	if hasUnresolved(result.Unresolved, cloneID, "baseClone") {
		t.Fatalf("Unresolved = %#v, baseClone should have resolved", result.Unresolved)
	}
}

func TestIndexerComposesJavaScriptExpressRouterMountsAcrossFiles(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "routes/users.js", `import { Router } from "express"
const api = Router()

function listUsers(req, res) {}

api.get("/users", listUsers)
export default api
`)
	writeASTGraphFixture(t, root, "app.js", `import api from "./routes/users"
const app = express()
app.use("/api", api)
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 2048})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	routeID := nodeIDByName(t, result.Nodes, NodeKindRoute, "GET /api/users")
	handlerID := nodeIDByName(t, result.Nodes, NodeKindHandler, "listUsers")
	if !hasEdge(result.Edges, routeID, handlerID, EdgeKindHandles) {
		t.Fatalf("Edges = %#v, want mounted /api/users route to handle listUsers", result.Edges)
	}
}

func TestIndexerComposesJavaScriptExpressRouterMountsThroughNamedReExports(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "routes/users.js", `import { Router } from "express"
export const api = Router()

function listUsers(req, res) {}

api.get("/users", listUsers)
`)
	writeASTGraphFixture(t, root, "routes/index.js", `export { api as usersRouter } from "./users"
`)
	writeASTGraphFixture(t, root, "app.js", `import { usersRouter } from "./routes"
const app = express()
app.use("/api", usersRouter)
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 2048})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	routeID := nodeIDByName(t, result.Nodes, NodeKindRoute, "GET /api/users")
	handlerID := nodeIDByName(t, result.Nodes, NodeKindHandler, "listUsers")
	if !hasEdge(result.Edges, routeID, handlerID, EdgeKindHandles) {
		t.Fatalf("Edges = %#v, want mounted /api/users route through named re-export to handle listUsers", result.Edges)
	}
}

func TestIndexerComposesPythonFastAPIIncludeRouterPrefixesAcrossFiles(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "routes/users.py", `from fastapi import APIRouter

router = APIRouter(prefix="/api")

@router.get("/users")
def list_users():
    pass
`)
	writeASTGraphFixture(t, root, "main.py", `from routes.users import router

app.include_router(router, prefix="/v1")
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 2048})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	routeID := nodeIDByName(t, result.Nodes, NodeKindRoute, "GET /v1/api/users")
	handlerID := nodeIDByName(t, result.Nodes, NodeKindHandler, "list_users")
	if !hasEdge(result.Edges, routeID, handlerID, EdgeKindHandles) {
		t.Fatalf("Edges = %#v, want included /v1/api/users route to handle list_users", result.Edges)
	}
}

func TestIndexerComposesPythonFastAPIIncludeRouterPrefixesForNamedRouterImports(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "routes/users.py", `from fastapi import APIRouter

users_router = APIRouter(prefix="/api")

@users_router.get("/users")
def list_users():
    pass
`)
	writeASTGraphFixture(t, root, "main.py", `from routes.users import users_router

app.include_router(users_router, prefix="/v1")
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 2048})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	routeID := nodeIDByName(t, result.Nodes, NodeKindRoute, "GET /v1/api/users")
	handlerID := nodeIDByName(t, result.Nodes, NodeKindHandler, "list_users")
	if !hasEdge(result.Edges, routeID, handlerID, EdgeKindHandles) {
		t.Fatalf("Edges = %#v, want included named-router /v1/api/users route to handle list_users", result.Edges)
	}
}

func TestIndexerComposesPythonFastAPIIncludeRouterPrefixesForOnlyImportedRouter(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "routes/api.py", `from fastapi import APIRouter

users_router = APIRouter(prefix="/users")
admin_router = APIRouter(prefix="/admin")

@users_router.get("/")
def list_users():
    pass

@admin_router.get("/")
def list_admins():
    pass
`)
	writeASTGraphFixture(t, root, "main.py", `from routes.api import users_router

app.include_router(users_router, prefix="/v1")
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 2048})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	routeID := nodeIDByName(t, result.Nodes, NodeKindRoute, "GET /v1/users/")
	handlerID := nodeIDByName(t, result.Nodes, NodeKindHandler, "list_users")
	if !hasEdge(result.Edges, routeID, handlerID, EdgeKindHandles) {
		t.Fatalf("Edges = %#v, want included users router route to handle list_users", result.Edges)
	}
	if hasNode(result.Nodes, NodeKindRoute, "GET /v1/admin/") {
		t.Fatalf("Nodes = %#v, admin_router route should not be mounted when only users_router is included", result.Nodes)
	}
}

func TestIndexerComposesPythonFastAPIIncludeRouterPrefixesThroughPackageReExports(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "routes/users.py", `from fastapi import APIRouter

users_router = APIRouter(prefix="/users")

@users_router.get("/")
def list_users():
    pass
`)
	writeASTGraphFixture(t, root, "routes/__init__.py", `from .users import users_router
`)
	writeASTGraphFixture(t, root, "main.py", `from routes import users_router

app.include_router(users_router, prefix="/v1")
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 2048})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	routeID := nodeIDByName(t, result.Nodes, NodeKindRoute, "GET /v1/users/")
	handlerID := nodeIDByName(t, result.Nodes, NodeKindHandler, "list_users")
	if !hasEdge(result.Edges, routeID, handlerID, EdgeKindHandles) {
		t.Fatalf("Edges = %#v, want package re-exported users router route to handle list_users", result.Edges)
	}
}

func TestIndexerResolvesTypeScriptCallsWithOmittedOptionalArguments(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "parse.ts", `function parseValue(value: unknown, options: object, ctx: object) {
  return value;
}
function parse(value: unknown) {
  return parseValue(value);
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	parseID := nodeIDByName(t, result.Nodes, NodeKindFunction, "parse")
	parseValueID := nodeIDByName(t, result.Nodes, NodeKindFunction, "parseValue")
	if !hasEdge(result.Edges, parseID, parseValueID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want parse -> parseValue with omitted optional arguments", result.Edges)
	}
	if hasUnresolved(result.Unresolved, parseID, "parseValue") {
		t.Fatalf("Unresolved = %#v, parseValue should have resolved", result.Unresolved)
	}
}

func TestIndexerClassifiesQualifiedCSharpExternalReceiverCalls(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class App {
  bool Equals(string left, string right) { return false; }
  void Run() {
    StringComparer.OrdinalIgnoreCase.Equals("a", "b");
    System.IO.File.OpenText("data.json");
    String.IsNullOrEmpty("value");
    Assert.Equal<string>("a", "b");
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindMethod, "Run")
	equalsID := nodeIDByName(t, result.Nodes, NodeKindMethod, "Equals")
	if hasEdge(result.Edges, runID, equalsID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, external receiver Equals should not resolve to local App.Equals", result.Edges)
	}
	for _, qualifiedName := range []string{
		"external:StringComparer.OrdinalIgnoreCase.Equals",
		"external:System.IO.File.OpenText",
		"external:System.String.IsNullOrEmpty",
		"external:Assert.Equal",
	} {
		externalID := nodeIDByQualifiedName(t, result.Nodes, NodeKindExternal, qualifiedName)
		if !hasEdge(result.Edges, runID, externalID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want run -> %s", result.Edges, qualifiedName)
		}
	}
	if hasUnresolved(result.Unresolved, runID, "Equals") || hasUnresolved(result.Unresolved, runID, "Equal") {
		t.Fatalf("Unresolved = %#v, external receiver calls should not remain unresolved", result.Unresolved)
	}
}

func TestIndexerClassifiesCSharpStringAndPrimitiveReceiverCallsAsExternal(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class App {
  void Run(string text, int count) {
    text.Substring(1);
    count.ToString();
    "Hello, world!".AsSpan();
    string.Format("{0}", count);
    string.Join(",", new[] { text });
    string.IsNullOrEmpty(text);
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	for _, qualifiedName := range []string{
		"external:System.String.Substring",
		"external:System.Int32.ToString",
		"external:System.String.AsSpan",
		"external:System.String.Format",
		"external:System.String.Join",
		"external:System.String.IsNullOrEmpty",
	} {
		externalID := nodeIDByQualifiedName(t, result.Nodes, NodeKindExternal, qualifiedName)
		if !hasEdge(result.Edges, runID, externalID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want run -> %s", result.Edges, qualifiedName)
		}
	}
	for _, name := range []string{"Substring", "ToString", "AsSpan", "Format", "Join", "IsNullOrEmpty"} {
		if hasUnresolved(result.Unresolved, runID, name) {
			t.Fatalf("Unresolved = %#v, %s should be classified as primitive/string external", result.Unresolved, name)
		}
	}
}

func TestIndexerClassifiesCSharpLinqCallsAsExternalWhenNoLocalCandidate(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.Tests/LinqTests.cs", `class LinqTests {
  void Run(Items items) {
    items.Select(x => x);
    items.ToList();
    items.ToAsyncEnumerable();
  }
}`)
	writeASTGraphFixture(t, root, "App/Custom.cs", `class Custom {
  void Select() {}
  void Run(Custom items) {
    items.Select();
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	testRunID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "LinqTests.Run")
	for _, qualifiedName := range []string{
		"external:System.Linq.Enumerable.Select",
		"external:System.Linq.Enumerable.ToList",
		"external:System.Linq.Enumerable.ToAsyncEnumerable",
	} {
		externalID := nodeIDByQualifiedName(t, result.Nodes, NodeKindExternal, qualifiedName)
		if !hasEdge(result.Edges, testRunID, externalID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want test Run -> %s", result.Edges, qualifiedName)
		}
	}

	customRunID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "Custom.Run")
	customSelectID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "Custom.Select")
	if !hasEdge(result.Edges, customRunID, customSelectID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, local Select should still resolve before LINQ external fallback", result.Edges)
	}
}

func TestIndexerClassifiesUnqualifiedCSharpTestCallsAsExternal(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.Tests/WidgetTests.cs", `class WidgetTests {
  void Run() {
    Equal(1, 1);
    True(true);
    Throws<InvalidOperationException>(() => Run());
    AreEqual("a", "a");
    IsTrue(true);
    Returns(42);
    Single(new[] { 1 });
    OfType<string>(new object[0]);
  }
}`)
	writeASTGraphFixture(t, root, "App/Widget.cs", `class Widget {
  void Run() {
    Equal(1, 1);
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	testRunID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "WidgetTests.Run")
	for _, qualifiedName := range []string{
		"external:test-framework.Equal",
		"external:test-framework.True",
		"external:test-framework.Throws",
		"external:test-framework.AreEqual",
		"external:test-framework.IsTrue",
		"external:test-framework.Returns",
		"external:test-framework.Single",
		"external:test-framework.OfType",
	} {
		externalID := nodeIDByQualifiedName(t, result.Nodes, NodeKindExternal, qualifiedName)
		if !hasEdge(result.Edges, testRunID, externalID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want test Run -> %s", result.Edges, qualifiedName)
		}
	}

	productionRunID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "Widget.Run")
	if !hasUnresolved(result.Unresolved, productionRunID, "Equal") {
		t.Fatalf("Unresolved = %#v, non-test Equal should remain unresolved", result.Unresolved)
	}
}

func TestIndexerResolvesCSharpReceiverCallsFromExplicitLocalTypes(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class Runner {
  void RunAsync() {}
}
class Sink {
  void OnMessage() {}
}
class IMessageSink {
  void OnMessage() {}
}
class App {
  Runner CreateRunner() { return new Runner(); }
  Sink CreateSink() { return new Sink(); }
  void Run() {
    Runner runner = CreateRunner();
    Sink sink = CreateSink();
    IMessageSink messageSink = GetMessageSink();
    runner.RunAsync();
    sink.OnMessage();
    messageSink.OnMessage();
  }
  IMessageSink GetMessageSink() { return new IMessageSink(); }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	for _, qualifiedName := range []string{"Runner.RunAsync", "Sink.OnMessage", "IMessageSink.OnMessage"} {
		targetID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, qualifiedName)
		if !hasEdge(result.Edges, runID, targetID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want run -> %s through explicit local type", result.Edges, qualifiedName)
		}
	}
}

func TestIndexerResolvesCSharpReceiverCallsFromParameterTypes(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class JsonSerializer {
  string Deserialize(JsonReader reader) { return ""; }
}
class App {
  void Run(JsonSerializer serializer, JsonReader reader) {
    serializer.Deserialize(reader);
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	deserializeID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "JsonSerializer.Deserialize")
	if !hasEdge(result.Edges, runID, deserializeID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want serializer.Deserialize to resolve through parameter type", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Deserialize") {
		t.Fatalf("Unresolved = %#v, Deserialize should resolve through parameter type", result.Unresolved)
	}
}

func TestIndexerResolvesCSharpReceiverCallsFromLocalNewVariables(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class JsonSerializer {
  string Deserialize(JsonReader reader) { return ""; }
}
class App {
  void Run(JsonReader reader) {
    var serializer = new JsonSerializer();
    serializer.Deserialize(reader);
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	deserializeID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "JsonSerializer.Deserialize")
	if !hasEdge(result.Edges, runID, deserializeID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want serializer.Deserialize to resolve through local new variable", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Deserialize") {
		t.Fatalf("Unresolved = %#v, Deserialize should resolve through local new variable", result.Unresolved)
	}
}

func TestIndexerResolvesCSharpReceiverCallsFromFactoryReturnTypes(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class JsonSerializer {
  string Deserialize(JsonReader reader) { return ""; }
}
class App {
  JsonSerializer CreateSerializer() { return new JsonSerializer(); }
  void Run(JsonReader reader) {
    var serializer = CreateSerializer();
    serializer.Deserialize(reader);
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	deserializeID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "JsonSerializer.Deserialize")
	if !hasEdge(result.Edges, runID, deserializeID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want serializer.Deserialize to resolve through factory return type", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Deserialize") {
		t.Fatalf("Unresolved = %#v, Deserialize should resolve through factory return type", result.Unresolved)
	}
}

func TestIndexerResolvesCSharpChainedReceiverCallsFromReturnTypes(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class JsonSerializer {
  string Deserialize(JsonReader reader) { return ""; }
}
class App {
  JsonSerializer CreateSerializer() { return new JsonSerializer(); }
  void Run(JsonReader reader) {
    CreateSerializer().Deserialize(reader);
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	deserializeID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "JsonSerializer.Deserialize")
	if !hasEdge(result.Edges, runID, deserializeID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want chained Deserialize to resolve through CreateSerializer return type", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Deserialize") {
		t.Fatalf("Unresolved = %#v, chained Deserialize should resolve through CreateSerializer return type", result.Unresolved)
	}
}

func TestIndexerResolvesCSharpChainedReceiverCallsThroughTypedPrefix(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class Runner {
  void RunAsync() {}
}
class Options {
  Runner Build() { return new Runner(); }
}
class App {
  void Run(Options options) {
    options.Build().RunAsync();
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	buildID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "Options.Build")
	runAsyncID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "Runner.RunAsync")
	if !hasEdge(result.Edges, runID, buildID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want options.Build to resolve through parameter type", result.Edges)
	}
	if !hasEdge(result.Edges, runID, runAsyncID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want options.Build().RunAsync to resolve through Build return type", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "RunAsync") {
		t.Fatalf("Unresolved = %#v, chained RunAsync should resolve through Build return type", result.Unresolved)
	}
}

func TestIndexerResolvesCSharpReceiverCallsFromFieldsAndProperties(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class JsonSerializer {
  string Deserialize(JsonReader reader) { return ""; }
}
class App {
  JsonSerializer serializer;
  JsonSerializer Serializer { get; }
  void Run(JsonReader reader) {
    serializer.Deserialize(reader);
    Serializer.Deserialize(reader);
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	deserializeID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "JsonSerializer.Deserialize")
	if !hasEdge(result.Edges, runID, deserializeID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want field/property receiver calls to resolve", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Deserialize") {
		t.Fatalf("Unresolved = %#v, Deserialize should resolve through field/property type", result.Unresolved)
	}
}

func TestIndexerResolvesCSharpThisAndBaseReceiverCalls(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "App.cs", `class BaseApp {
  void Save() {}
}
class App : BaseApp {
  void Helper() {}
  void Run() {
    this.Helper();
    base.Save();
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	helperID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Helper")
	saveID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "BaseApp.Save")
	if !hasEdge(result.Edges, runID, helperID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want this.Helper to resolve to App.Helper", result.Edges)
	}
	if !hasEdge(result.Edges, runID, saveID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want base.Save to resolve to BaseApp.Save", result.Edges)
	}
}

func TestIndexerResolvesCSharpBaseReceiverCallsFromGenericPartialBase(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "Base.cs", `class BaseApp<T> {
  void Save() {}
}`)
	writeASTGraphFixture(t, root, "App.Base.cs", `partial class App : BaseApp<string>, IDisposable {
}`)
	writeASTGraphFixture(t, root, "App.cs", `partial class App {
  void Run() {
    base.Save();
  }
}`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "App.Run")
	saveID := nodeIDByQualifiedName(t, result.Nodes, NodeKindMethod, "BaseApp.Save")
	if !hasEdge(result.Edges, runID, saveID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want base.Save to resolve through generic partial base type", result.Edges)
	}
}

func TestIndexerResolvesGoCallsWithOmittedVariadicArguments(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
type Option func()
func New(opts ...Option) {}
func Default() {
	New()
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	defaultID := nodeIDByName(t, result.Nodes, NodeKindFunction, "Default")
	newID := nodeIDByName(t, result.Nodes, NodeKindFunction, "New")
	if !hasEdge(result.Edges, defaultID, newID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want Default -> New with omitted variadic arguments", result.Edges)
	}
	if hasUnresolved(result.Unresolved, defaultID, "New") {
		t.Fatalf("Unresolved = %#v, New should have resolved", result.Unresolved)
	}
}

func TestIndexerResolvesGoCompositeLiteralReceiverCalls(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
type Writer struct{}
type JSON struct{ Data any }
type XML struct{ Data any }
func (r JSON) Render(w Writer) error { return nil }
func (r XML) Render(w Writer) error { return nil }
func run(data any, w Writer) {
	(JSON{Data: data}).Render(w)
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	renderID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindMethod, "Render", "JSON")
	if !hasEdge(result.Edges, runID, renderID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want run -> JSON.Render for composite literal receiver", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Render") {
		t.Fatalf("Unresolved = %#v, Render should have resolved", result.Unresolved)
	}
}

func TestIndexerResolvesGoPackageVariablesAndLocalAliases(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "vars.go", `package main
type Request struct{}
type formBinding struct{}
type formPostBinding struct{}

var FormPost = formPostBinding{}

func (formBinding) Bind(req Request) error { return nil }
func (formBinding) Name() string { return "form" }
func (formPostBinding) Bind(req Request) error { return nil }
func (formPostBinding) Name() string { return "form-urlencoded" }
`)
	writeASTGraphFixture(t, root, "main.go", `package main
func run(req Request) {
	FormPost.Bind(req)
	b := FormPost
	b.Name()
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	bindID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindMethod, "Bind", "formPostBinding")
	nameID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindMethod, "Name", "formPostBinding")
	if !hasEdge(result.Edges, runID, bindID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want run -> formPostBinding.Bind through FormPost package variable", result.Edges)
	}
	if !hasEdge(result.Edges, runID, nameID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want run -> formPostBinding.Name through local alias b", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Bind") || hasUnresolved(result.Unresolved, runID, "Name") {
		t.Fatalf("Unresolved = %#v, Bind and Name should have resolved through Go aliases", result.Unresolved)
	}
}

func TestIndexerResolvesGoLocalVariablesFromReturnTypes(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
type Engine struct{}
type RouterGroup struct{}

func New() *Engine { return &Engine{} }
func Default() *Engine { return New() }
func (e *Engine) Group(path string) *RouterGroup { return &RouterGroup{} }
func (e *Engine) ServeHTTP() {}
func (g *RouterGroup) GET(path string) {}

func run() {
	router := Default()
	router.Group("/api").GET("/users")
	router.ServeHTTP()
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	serveID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindMethod, "ServeHTTP", "Engine")
	if !hasEdge(result.Edges, runID, serveID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want run -> Engine.ServeHTTP through inferred Go return type", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "ServeHTTP") {
		t.Fatalf("Unresolved = %#v, ServeHTTP should have resolved through inferred Go return type", result.Unresolved)
	}
}

func TestIndexerClassifiesUnresolvedGoReceiverChainsAsExternal(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
type Request struct{ Header Header }
type Header struct{}
func run(req Request) {
	req.Header.Add("Accept", "application/json")
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	externalID := nodeIDByQualifiedName(t, result.Nodes, NodeKindExternal, "external:req.Header.Add")
	if !hasEdge(result.Edges, runID, externalID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want req.Header.Add classified as external receiver chain", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Add") {
		t.Fatalf("Unresolved = %#v, Add should have been classified as external receiver chain", result.Unresolved)
	}
}

func TestIndexerResolvesGoFunctionValuedVariables(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
func statusColor(code int) string { return "" }

var colorForStatus = statusColor

func run() {
	colorForStatus(200)
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	statusID := nodeIDByName(t, result.Nodes, NodeKindFunction, "statusColor")
	if !hasEdge(result.Edges, runID, statusID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want function-valued variable call to resolve to statusColor", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "colorForStatus") {
		t.Fatalf("Unresolved = %#v, colorForStatus should resolve as a function-valued variable", result.Unresolved)
	}
}

func TestIndexerResolvesGoInterfaceTypedParameters(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
type Request struct{}
type Binding interface {
	Name() string
	Bind(Request) error
}
type formBinding struct{}
type formPostBinding struct{}

func (formBinding) Name() string { return "form" }
func (formBinding) Bind(req Request) error { return nil }
func (formPostBinding) Name() string { return "form" }
func (formPostBinding) Bind(req Request) error { return nil }

func run(b Binding, req Request) {
	b.Name()
	b.Bind(req)
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	nameID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindMethod, "Name", "Binding")
	bindID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindMethod, "Bind", "Binding")
	if !hasEdge(result.Edges, runID, nameID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want b.Name to resolve through interface-typed parameter", result.Edges)
	}
	if !hasEdge(result.Edges, runID, bindID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want b.Bind to resolve through interface-typed parameter", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Name") || hasUnresolved(result.Unresolved, runID, "Bind") {
		t.Fatalf("Unresolved = %#v, interface-typed parameter calls should resolve", result.Unresolved)
	}
}

func TestIndexerFiltersGoBuiltinsAndExternalImportsFromUnresolved(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
import (
  "fmt"
  "github.com/stretchr/testify/assert"
)
func run() {
  values := []string{"a"}
  len(values)
  make([]string, 0)
  string([]byte("x"))
  fmt.Println("ok")
  assert.Equal(nil, 1, 1)
  missing()
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	for _, name := range []string{"len", "make", "string", "Println", "Equal"} {
		if hasUnresolved(result.Unresolved, runID, name) {
			t.Fatalf("Unresolved = %#v, %s should have been filtered as builtin or imported external call", result.Unresolved, name)
		}
	}
	for _, qualifiedName := range []string{
		"external:builtin.len",
		"external:builtin.make",
		"external:builtin.string",
		"external:fmt.Println",
		"external:assert.Equal",
	} {
		externalID := nodeIDByQualifiedName(t, result.Nodes, NodeKind("external"), qualifiedName)
		if !hasEdge(result.Edges, runID, externalID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want run -> %s", result.Edges, qualifiedName)
		}
	}
	if !hasUnresolved(result.Unresolved, runID, "missing") {
		t.Fatalf("Unresolved = %#v, missing internal call should remain unresolved", result.Unresolved)
	}
}

func TestIndexerResolvesKotlinNavigationCallsByReceiverScope(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "Strings.kt", `fun String.substring(startIndex: Int): String { return this }
fun Int.substring(startIndex: Int): Int { return this }
fun String.dropFirst(): String { return this.substring(1) }
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	dropID := nodeIDByName(t, result.Nodes, NodeKindFunction, "dropFirst")
	stringSubstringID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindFunction, "substring", "String")
	if !hasEdge(result.Edges, dropID, stringSubstringID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want String.dropFirst -> String.substring", result.Edges)
	}
	if hasUnresolved(result.Unresolved, dropID, "substring") {
		t.Fatalf("Unresolved = %#v, substring should have resolved", result.Unresolved)
	}
}

func TestIndexerResolvesKotlinTrailingLambdaCallsByArgumentCount(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "Contracts.kt", `fun contract(builder: ContractBuilder.() -> Unit) {}
fun run() { contract { returns() } }
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	contractID := nodeIDByName(t, result.Nodes, NodeKindFunction, "contract")
	if !hasEdge(result.Edges, runID, contractID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want run -> contract", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "contract") {
		t.Fatalf("Unresolved = %#v, contract should have resolved", result.Unresolved)
	}
}

func TestIndexerResolvesKotlinConstructorCallsToClasses(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "Classes.kt", `class NoSuchElementException
annotation class ReplaceWith(val expression: String)
fun run() {
  NoSuchElementException("missing")
  ReplaceWith("value")
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	exceptionID := nodeIDByName(t, result.Nodes, NodeKindClass, "NoSuchElementException")
	replaceWithID := nodeIDByName(t, result.Nodes, NodeKindClass, "ReplaceWith")
	if !hasEdge(result.Edges, runID, exceptionID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want run -> NoSuchElementException", result.Edges)
	}
	if !hasEdge(result.Edges, runID, replaceWithID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want run -> ReplaceWith", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "NoSuchElementException") || hasUnresolved(result.Unresolved, runID, "ReplaceWith") {
		t.Fatalf("Unresolved = %#v, constructor calls should have resolved", result.Unresolved)
	}
}

func TestIndexerResolvesKotlinReceiverCallsFromInterfaceConstructorProperties(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "api/Port.kt", `package api

interface Port {
  fun save(id: String)
}
`)
	writeASTGraphFixture(t, root, "impl/Other.kt", `package impl

class Other {
  fun save(id: String) {}
}
`)
	writeASTGraphFixture(t, root, "svc/Service.kt", `package svc

import api.Port

class Service(private val port: Port) {
  fun handle(id: String) {
    port.save(id)
    helper(id)
    RuntimeException(id)
  }

  private fun helper(id: String) {}
}

class RuntimeException(message: String)
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 2048})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	handleID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindFunction, "handle", "Service")
	saveID := nodeIDByQualifiedName(t, result.Nodes, NodeKindFunction, "api.Port.save")
	helperID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindFunction, "helper", "Service")
	exceptionID := nodeIDByName(t, result.Nodes, NodeKindClass, "RuntimeException")
	if !hasEdge(result.Edges, handleID, saveID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want Service.handle -> Port.save through interface-typed constructor property", result.Edges)
	}
	if !hasEdge(result.Edges, handleID, helperID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want Service.handle -> Service.helper", result.Edges)
	}
	if !hasEdge(result.Edges, handleID, exceptionID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want Service.handle -> RuntimeException constructor", result.Edges)
	}
	if hasUnresolved(result.Unresolved, handleID, "save") {
		t.Fatalf("Unresolved = %#v, port.save should resolve through interface-typed constructor property", result.Unresolved)
	}
}

func TestIndexerResolvesKotlinCallsThroughWildcardImports(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "contracts/Contract.kt", `package kotlin.contracts
fun contract(builder: ContractBuilder.() -> Unit) {}
`)
	writeASTGraphFixture(t, root, "demo/Use.kt", `package demo
import kotlin.contracts.*
fun run() { contract { returns() } }
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	contractID := nodeIDByQualifiedName(t, result.Nodes, NodeKindFunction, "kotlin.contracts.contract")
	if !hasEdge(result.Edges, runID, contractID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want demo.run -> kotlin.contracts.contract", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "contract") {
		t.Fatalf("Unresolved = %#v, contract should have resolved through wildcard import", result.Unresolved)
	}
}

func TestIndexerPrefersKotlinSamePackageTargets(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "a/Box.kt", `package a
class Box
`)
	writeASTGraphFixture(t, root, "a/Use.kt", `package a
fun run() { Box() }
`)
	writeASTGraphFixture(t, root, "b/Box.kt", `package b
class Box
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindFunction, "a.run")
	boxID := nodeIDByQualifiedName(t, result.Nodes, NodeKindClass, "a.Box")
	if !hasEdge(result.Edges, runID, boxID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want a.run -> a.Box", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "Box") {
		t.Fatalf("Unresolved = %#v, Box should have resolved to same package", result.Unresolved)
	}
}

func TestIndexerResolvesKotlinDefaultImports(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "kotlin/Errors.kt", `package kotlin
class NoSuchElementException
class IllegalArgumentException
`)
	writeASTGraphFixture(t, root, "kotlin/collections/Lists.kt", `package kotlin.collections
class ArrayList
`)
	writeASTGraphFixture(t, root, "demo/Use.kt", `package demo
fun run() {
  NoSuchElementException("missing")
  IllegalArgumentException("bad")
  ArrayList()
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByQualifiedName(t, result.Nodes, NodeKindFunction, "demo.run")
	exceptionID := nodeIDByQualifiedName(t, result.Nodes, NodeKindClass, "kotlin.NoSuchElementException")
	argumentID := nodeIDByQualifiedName(t, result.Nodes, NodeKindClass, "kotlin.IllegalArgumentException")
	arrayListID := nodeIDByQualifiedName(t, result.Nodes, NodeKindClass, "kotlin.collections.ArrayList")
	for _, targetID := range []string{exceptionID, argumentID, arrayListID} {
		if !hasEdge(result.Edges, runID, targetID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want demo.run -> %s", result.Edges, targetID)
		}
	}
	for _, name := range []string{"NoSuchElementException", "IllegalArgumentException", "ArrayList"} {
		if hasUnresolved(result.Unresolved, runID, name) {
			t.Fatalf("Unresolved = %#v, %s should have resolved through Kotlin default imports", result.Unresolved, name)
		}
	}
}

func TestIndexerResolvesRustEnumVariantConstructors(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "lib.rs", `enum Token {
    Str(&'static str),
    I32(i32),
}

fn emit() {
    Token::Str("value");
    Token::I32(1);
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	emitID := nodeIDByName(t, result.Nodes, NodeKindFunction, "emit")
	strID := nodeIDByQualifiedName(t, result.Nodes, NodeKindClass, "Token::Str")
	i32ID := nodeIDByQualifiedName(t, result.Nodes, NodeKindClass, "Token::I32")
	for _, targetID := range []string{strID, i32ID} {
		if !hasEdge(result.Edges, emitID, targetID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want emit -> %s", result.Edges, targetID)
		}
	}
	if hasUnresolved(result.Unresolved, emitID, "Str") || hasUnresolved(result.Unresolved, emitID, "I32") {
		t.Fatalf("Unresolved = %#v, enum variants should resolve", result.Unresolved)
	}
}

func TestIndexerResolvesRustImplCallsByReceiverType(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "lib.rs", `struct Formatter;

impl Formatter {
    fn write_str(&mut self, value: &str) {}
}

fn run(formatter: &mut Formatter) {
    formatter.write_str("value");
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	writeStrID := nodeIDByNameAndReceiver(t, result.Nodes, NodeKindMethod, "write_str", "Formatter")
	if !hasEdge(result.Edges, runID, writeStrID, EdgeKindCalls) {
		t.Fatalf("Edges = %#v, want run -> Formatter::write_str", result.Edges)
	}
	if hasUnresolved(result.Unresolved, runID, "write_str") {
		t.Fatalf("Unresolved = %#v, write_str should resolve through typed Rust receiver", result.Unresolved)
	}
}

func TestIndexerClassifiesRustPreludeAndReceiverCallsAsExternal(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "lib.rs", `fn run(values: Vec<i32>) {
    Some(1);
    Ok(1);
    Err(());
    Box::new(1);
    values.iter().map(|value| value);
    missing();
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	for _, name := range []string{"Some", "Ok", "Err", "new", "iter", "map"} {
		if hasUnresolved(result.Unresolved, runID, name) {
			t.Fatalf("Unresolved = %#v, %s should have been classified as a Rust external call", result.Unresolved, name)
		}
	}
	for _, qualifiedName := range []string{
		"external:rust.prelude.Some",
		"external:rust.prelude.Ok",
		"external:rust.prelude.Err",
		"external:Box::new",
		"external:values.iter",
		"external:values.iter.map",
	} {
		externalID := nodeIDByQualifiedName(t, result.Nodes, NodeKindExternal, qualifiedName)
		if !hasEdge(result.Edges, runID, externalID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want run -> %s", result.Edges, qualifiedName)
		}
	}
	if !hasUnresolved(result.Unresolved, runID, "missing") {
		t.Fatalf("Unresolved = %#v, missing internal call should remain unresolved", result.Unresolved)
	}
}

func TestIndexerClassifiesRustImportedCallsAsExternal(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "lib.rs", `use serde_test::{assert_de_tokens, assert_ser_tokens, Token};

fn run() {
    assert_de_tokens(&[Token::Str("value"), Token::I32(1)]);
    assert_ser_tokens(&[Token::Str("value")]);
    missing();
}
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := nodeIDByName(t, result.Nodes, NodeKindFunction, "run")
	for _, name := range []string{"assert_de_tokens", "assert_ser_tokens", "Str", "I32"} {
		if hasUnresolved(result.Unresolved, runID, name) {
			t.Fatalf("Unresolved = %#v, %s should have been classified through Rust imports", result.Unresolved, name)
		}
	}
	for _, qualifiedName := range []string{
		"external:serde_test::assert_de_tokens",
		"external:serde_test::assert_ser_tokens",
		"external:serde_test::Token::Str",
		"external:serde_test::Token::I32",
	} {
		externalID := nodeIDByQualifiedName(t, result.Nodes, NodeKindExternal, qualifiedName)
		if !hasEdge(result.Edges, runID, externalID, EdgeKindCalls) {
			t.Fatalf("Edges = %#v, want run -> %s", result.Edges, qualifiedName)
		}
	}
	if !hasUnresolved(result.Unresolved, runID, "missing") {
		t.Fatalf("Unresolved = %#v, missing internal call should remain unresolved", result.Unresolved)
	}
}

func TestIndexerSetsResultAndFileMetadata(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "main.go", `package main
func helper() {}
func main() { helper() }
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 1024})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}

	if result.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", result.SchemaVersion, SchemaVersion)
	}
	if result.SourcePath != root {
		t.Fatalf("SourcePath = %q, want %q", result.SourcePath, root)
	}
	if result.StartedAt.IsZero() || result.CompletedAt.IsZero() {
		t.Fatalf("StartedAt = %s, CompletedAt = %s, want non-zero", result.StartedAt, result.CompletedAt)
	}
	if len(result.Files) != 1 {
		t.Fatalf("Files = %#v", result.Files)
	}
	file := result.Files[0]
	if file.ContentHash == "" {
		t.Fatalf("ContentHash = %q, want non-empty", file.ContentHash)
	}
	if file.NodeCount != countNodesForPath(result.Nodes, file.Path) {
		t.Fatalf("NodeCount = %d, nodes for path = %d", file.NodeCount, countNodesForPath(result.Nodes, file.Path))
	}
}

func TestIndexerSkipsOversizedFile(t *testing.T) {
	root := t.TempDir()
	writeASTGraphFixture(t, root, "large.go", `package main
func helper() {}
func main() { helper() }
`)

	indexer := NewIndexer(IndexOptions{MaxFileSize: 8})
	result, err := indexer.Index(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 0 {
		t.Fatalf("Files = %#v, want none", result.Files)
	}
	if len(result.Nodes) != 0 {
		t.Fatalf("Nodes = %#v, want none", result.Nodes)
	}
}

func writeASTGraphFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasNode(nodes []GraphNode, kind NodeKind, name string) bool {
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return true
		}
	}
	return false
}

func countNodesForPath(nodes []GraphNode, path string) int {
	count := 0
	for _, node := range nodes {
		if node.FilePath == path {
			count++
		}
	}
	return count
}

func nodeIDByName(t *testing.T, nodes []GraphNode, kind NodeKind, name string) string {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return node.ID
		}
	}
	t.Fatalf("node %s %s not found in %#v", kind, name, nodes)
	return ""
}

func nodeIDByNameAndParamCount(t *testing.T, nodes []GraphNode, kind NodeKind, name string, parameterCount int) string {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name && node.ParameterCount == parameterCount {
			return node.ID
		}
	}
	t.Fatalf("node %s %s with %d params not found in %#v", kind, name, parameterCount, nodes)
	return ""
}

func nodeIDByNameAndReceiver(t *testing.T, nodes []GraphNode, kind NodeKind, name, receiver string) string {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name && node.ReceiverType == receiver {
			return node.ID
		}
	}
	t.Fatalf("node %s %s with receiver %s not found in %#v", kind, name, receiver, nodes)
	return ""
}

func nodeIDByQualifiedName(t *testing.T, nodes []GraphNode, kind NodeKind, qualifiedName string) string {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.QualifiedName == qualifiedName {
			return node.ID
		}
	}
	t.Fatalf("node %s qualified %s not found in %#v", kind, qualifiedName, nodes)
	return ""
}

func hasEdge(edges []GraphEdge, sourceID, targetID string, kind EdgeKind) bool {
	for _, edge := range edges {
		if edge.SourceNodeID == sourceID && edge.TargetNodeID == targetID && edge.Kind == kind {
			return true
		}
	}
	return false
}

func hasEdgeKind(edges []GraphEdge, kind EdgeKind) bool {
	for _, edge := range edges {
		if edge.Kind == kind {
			return true
		}
	}
	return false
}

func hasUnresolved(refs []UnresolvedReference, fromID, name string) bool {
	for _, ref := range refs {
		if ref.FromNodeID == fromID && ref.ReferenceName == name {
			return true
		}
	}
	return false
}
