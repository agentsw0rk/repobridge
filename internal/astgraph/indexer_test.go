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
	if len(result.Unresolved) == 0 {
		t.Fatalf("Unresolved = %#v, want helper call", result.Unresolved)
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
