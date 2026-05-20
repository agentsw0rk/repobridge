package codegraph_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"repobridge/internal/cache"
	"repobridge/internal/codegraph"
	"repobridge/internal/codegraph/store"
	"repobridge/internal/source"
)

type fakeSourceResolver struct {
	outcome source.Outcome
	opts    []source.Options
}

func (f *fakeSourceResolver) EnsureCached(spec string, opts source.Options) (source.Outcome, error) {
	f.opts = append(f.opts, opts)
	return f.outcome, nil
}

func TestSearchServiceIndexesWhenGraphMissing(t *testing.T) {
	sourceDir := t.TempDir()
	writeCodegraphFixture(t, sourceDir, "main.go", `package main
func main() { helper() }
func helper() {}
`)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := codegraph.NewSearchService(codegraph.SearchServiceOptions{Resolver: resolver})

	results, err := service.Search("demo@v1", `calls:helper`, codegraph.SearchOptions{SyncIndex: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("results empty, want caller of helper")
	}
	if results[0].Source != "demo@v1" {
		t.Fatalf("result source = %q, want demo@v1", results[0].Source)
	}
}

func TestSearchServiceNoSyncReturnsClearErrorWhenGraphMissing(t *testing.T) {
	sourceDir := t.TempDir()
	writeCodegraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := codegraph.NewSearchService(codegraph.SearchServiceOptions{Resolver: resolver})

	_, err := service.Search("demo@v1", `main`, codegraph.SearchOptions{SyncIndex: false, Limit: 10})
	if err == nil {
		t.Fatal("Search() error = nil, want missing graph error")
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "missing") && !strings.Contains(message, "incomplete") {
		t.Fatalf("Search() error = %q, want missing/incomplete graph message", err)
	}
	if !strings.Contains(message, "sync") {
		t.Fatalf("Search() error = %q, want sync index guidance", err)
	}
}

func TestSearchServiceUsesCompleteGraphWithoutReindexing(t *testing.T) {
	sourceDir := t.TempDir()
	writeCodegraphFixture(t, sourceDir, "main.go", `package main
func Different() {}
`)
	replaceStoredGraph(t, sourceDir, codegraph.IndexResult{
		SourcePath:    sourceDir,
		SchemaVersion: codegraph.SchemaVersion,
		CompletedAt:   time.Now().UTC(),
		Nodes: []codegraph.GraphNode{{
			ID:            "stored-function",
			Kind:          codegraph.NodeKindFunction,
			Name:          "StoredFunction",
			QualifiedName: "StoredFunction",
			FilePath:      "stored.go",
			Language:      codegraph.LanguageGo,
			StartLine:     3,
			EndLine:       5,
		}},
	})

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := codegraph.NewSearchService(codegraph.SearchServiceOptions{Resolver: resolver})

	results, err := service.Search("demo@v1", `name:StoredFunction`, codegraph.SearchOptions{SyncIndex: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "StoredFunction" {
		t.Fatalf("results = %#v, want pre-populated StoredFunction", results)
	}
	if results[0].Path != "stored.go" {
		t.Fatalf("result path = %q, want stored.go from existing graph", results[0].Path)
	}
}

func TestSearchServicePassesCWDToResolver(t *testing.T) {
	sourceDir := t.TempDir()
	writeCodegraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := codegraph.NewSearchService(codegraph.SearchServiceOptions{Resolver: resolver})

	_, err := service.Search("demo@v1", `main`, codegraph.SearchOptions{CWD: "/workspace/project", SyncIndex: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolver.opts) != 1 {
		t.Fatalf("resolver calls = %d, want 1", len(resolver.opts))
	}
	if resolver.opts[0].CWD != "/workspace/project" {
		t.Fatalf("resolver CWD = %q, want /workspace/project", resolver.opts[0].CWD)
	}
}

func replaceStoredGraph(t *testing.T, sourceDir string, result codegraph.IndexResult) {
	t.Helper()

	graphDir, err := cache.GraphDirForSource(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := store.Open(graphDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Replace(result); err != nil {
		graph.Close()
		t.Fatal(err)
	}
	graph.Close()
}

func writeCodegraphFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
