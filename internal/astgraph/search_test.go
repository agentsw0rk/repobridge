package astgraph_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"repobridge/internal/astgraph"
	"repobridge/internal/astgraph/store"
	"repobridge/internal/cache"
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
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func main() { helper() }
func helper() {}
`)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	results, err := service.Search("demo@v1", `calls:helper`, astgraph.SearchOptions{SyncIndex: true, Limit: 10})
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
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	_, err := service.Search("demo@v1", `main`, astgraph.SearchOptions{SyncIndex: false, Limit: 10})
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

func TestSearchServiceRequiresExplicitStoreOpener(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{Resolver: resolver})

	_, err := service.Search("demo@v1", `main`, astgraph.SearchOptions{SyncIndex: false, Limit: 10})
	if err == nil {
		t.Fatal("Search() error = nil, want store opener required error")
	}
	if got, want := err.Error(), "astgraph store opener is required"; got != want {
		t.Fatalf("Search() error = %q, want %q", got, want)
	}
}

func TestSearchServiceUsesCompleteGraphWithoutReindexing(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func Different() {}
`)
	result, err := astgraph.NewIndexer(astgraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	result.Nodes = []astgraph.GraphNode{{
		ID:            "stored-function",
		Kind:          astgraph.NodeKindFunction,
		Name:          "StoredFunction",
		QualifiedName: "StoredFunction",
		FilePath:      "main.go",
		Language:      astgraph.LanguageGo,
		StartLine:     3,
		EndLine:       5,
	}}
	replaceStoredGraph(t, sourceDir, result)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	results, err := service.Search("demo@v1", `name:StoredFunction`, astgraph.SearchOptions{SyncIndex: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "StoredFunction" {
		t.Fatalf("results = %#v, want pre-populated StoredFunction", results)
	}
	if results[0].Path != "main.go" {
		t.Fatalf("result path = %q, want main.go from existing graph", results[0].Path)
	}
}

func TestSearchServiceReindexesStaleGraphWhenSyncEnabled(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func OldName() {}
`)
	result, err := astgraph.NewIndexer(astgraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	replaceStoredGraph(t, sourceDir, result)

	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func NewName() {}
`)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	results, err := service.Search("demo@v1", `name:NewName`, astgraph.SearchOptions{SyncIndex: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "NewName" {
		t.Fatalf("results = %#v, want reindexed NewName", results)
	}
}

func TestSearchServiceNoSyncReturnsClearErrorWhenGraphStale(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func OldName() {}
`)
	result, err := astgraph.NewIndexer(astgraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	replaceStoredGraph(t, sourceDir, result)

	writeASTGraphFixture(t, sourceDir, "extra.go", `package main
func Extra() {}
`)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	_, err = service.Search("demo@v1", `name:OldName`, astgraph.SearchOptions{SyncIndex: false, Limit: 10})
	if err == nil {
		t.Fatal("Search() error = nil, want stale graph error")
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "stale") {
		t.Fatalf("Search() error = %q, want stale graph message", err)
	}
	if !strings.Contains(message, "sync") {
		t.Fatalf("Search() error = %q, want sync index guidance", err)
	}
}

func TestSearchServiceTreatsRemovedSourceFileAsStale(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)
	writeASTGraphFixture(t, sourceDir, "gone.go", `package main
func Gone() {}
`)
	result, err := astgraph.NewIndexer(astgraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	replaceStoredGraph(t, sourceDir, result)

	if err := os.Remove(filepath.Join(sourceDir, "gone.go")); err != nil {
		t.Fatal(err)
	}

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	_, err = service.Search("demo@v1", `name:Gone`, astgraph.SearchOptions{SyncIndex: false, Limit: 10})
	if err == nil {
		t.Fatal("Search() error = nil, want stale graph error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "stale") {
		t.Fatalf("Search() error = %q, want stale graph message", err)
	}
}

func TestSearchServiceDetectsCompleteGraphWithMismatchedFilesAsStale(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func Different() {}
`)
	replaceStoredGraph(t, sourceDir, astgraph.IndexResult{
		SourcePath:    sourceDir,
		SchemaVersion: astgraph.SchemaVersion,
		CompletedAt:   time.Now().UTC(),
		Nodes: []astgraph.GraphNode{{
			ID:            "stored-function",
			Kind:          astgraph.NodeKindFunction,
			Name:          "StoredFunction",
			QualifiedName: "StoredFunction",
			FilePath:      "stored.go",
			Language:      astgraph.LanguageGo,
			StartLine:     3,
			EndLine:       5,
		}},
	})

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1", SourceLabel: "repo"},
	}
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	_, err := service.Search("demo@v1", `name:StoredFunction`, astgraph.SearchOptions{SyncIndex: false, Limit: 10})
	if err == nil {
		t.Fatal("Search() error = nil, want stale graph error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "stale") {
		t.Fatalf("Search() error = %q, want stale graph message", err)
	}
}

func TestSearchServicePassesCWDToResolver(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)

	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	_, err := service.Search("demo@v1", `main`, astgraph.SearchOptions{CWD: "/workspace/project", SyncIndex: true, Limit: 10})
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

func TestSearchServiceUsesSnapshotQueryEngine(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "app.go", `package main
func Run() {}
`)
	files, err := astgraph.NewIndexer(astgraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	store := &lifecycleStore{
		status: astgraph.GraphStatus{Status: "complete", SchemaVersion: astgraph.SchemaVersion},
		files:  files.Files,
		snapshot: astgraph.GraphSnapshot{
			SourcePath: sourceDir,
			Nodes: []astgraph.GraphNode{
				{ID: "run", Kind: astgraph.NodeKindFunction, Name: "Run", QualifiedName: "app.Run", FilePath: "app.go", Language: astgraph.LanguageGo, StartLine: 2, EndLine: 2},
				{ID: "save", Kind: astgraph.NodeKindFunction, Name: "Save", QualifiedName: "store.Save", FilePath: "store.go", Language: astgraph.LanguageGo, StartLine: 1, EndLine: 1},
			},
			Edges: []astgraph.GraphEdge{
				{SourceNodeID: "run", TargetNodeID: "save", Kind: astgraph.EdgeKindCalls},
			},
			Unresolved: []astgraph.UnresolvedReference{
				{FromNodeID: "run", ReferenceName: "fmt.Println", ReferenceKind: astgraph.EdgeKindCalls},
			},
		},
	}
	lifecycle := newTestLifecycle(sourceDir, store, files.Files)
	service := astgraph.NewSearchService(astgraph.SearchServiceOptions{Lifecycle: lifecycle})

	results, err := service.Search("demo@v1", `calls:println`, astgraph.SearchOptions{SyncIndex: false, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "Run" {
		t.Fatalf("results = %#v, want Run from snapshot query engine", results)
	}
}

func replaceStoredGraph(t *testing.T, sourceDir string, result astgraph.IndexResult) {
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

func openGraphStore(dir string) (astgraph.GraphStore, error) {
	return store.Open(dir)
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
