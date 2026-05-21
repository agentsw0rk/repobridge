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
	if len(result.Edges) != 0 {
		t.Fatalf("Edges = %#v, ambiguous helper call should not resolve", result.Edges)
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

func hasUnresolved(refs []UnresolvedReference, fromID, name string) bool {
	for _, ref := range refs {
		if ref.FromNodeID == fromID && ref.ReferenceName == name {
			return true
		}
	}
	return false
}
