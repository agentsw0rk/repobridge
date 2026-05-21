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
