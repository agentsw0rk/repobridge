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
	if !hasUnresolved(result.Unresolved, mainID, "Println") {
		t.Fatalf("Unresolved = %#v, want external Println to remain unresolved", result.Unresolved)
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
