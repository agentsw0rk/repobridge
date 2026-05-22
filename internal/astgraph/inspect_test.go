package astgraph_test

import (
	"strings"
	"testing"
	"time"

	"repobridge/internal/astgraph"
	"repobridge/internal/cache"
	"repobridge/internal/source"
)

func TestInspectStatusNoSyncReportsMissingGraph(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewInspectService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	status, err := service.Status("demo@v1", astgraph.GraphInspectOptions{SyncIndex: false})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "missing" || status.Source != "demo@v1" || status.SourcePath != sourceDir {
		t.Fatalf("status = %#v, want missing demo status", status)
	}
	if !strings.HasSuffix(status.GraphPath, ".repobridge-graph") {
		t.Fatalf("graph path = %q, want graph dir", status.GraphPath)
	}
}

func TestInspectStatusSyncIndexesMissingGraph(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func main() { helper() }
func helper() {}
`)
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewInspectService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	status, err := service.Status("demo@v1", astgraph.GraphInspectOptions{SyncIndex: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "ready" {
		t.Fatalf("status = %#v, want ready", status)
	}
	if status.Counts.Files != 1 || status.Counts.Nodes == 0 {
		t.Fatalf("counts = %#v, want indexed files and nodes", status.Counts)
	}
}

func TestInspectStatusIncludesSourceKind(t *testing.T) {
	sourceDir := t.TempDir()
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "project:.", SourceKind: "project"},
	}
	service := astgraph.NewInspectService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	status, err := service.Status("project:.", astgraph.GraphInspectOptions{SyncIndex: false})
	if err != nil {
		t.Fatal(err)
	}
	if status.SourceKind != "project" {
		t.Fatalf("SourceKind = %q, want project", status.SourceKind)
	}
}

func TestInspectStatusNoSyncReportsStaleGraph(t *testing.T) {
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
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewInspectService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	status, err := service.Status("demo@v1", astgraph.GraphInspectOptions{SyncIndex: false})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "stale" {
		t.Fatalf("status = %#v, want stale", status)
	}
}

func TestInspectFilesFiltersAndLimitsStoredFiles(t *testing.T) {
	sourceDir := t.TempDir()
	completedAt := time.Now().UTC()
	replaceStoredGraph(t, sourceDir, astgraph.IndexResult{
		SourcePath:    sourceDir,
		SchemaVersion: astgraph.SchemaVersion,
		CompletedAt:   completedAt,
		Files: []astgraph.GraphFile{
			{Path: "cmd/main.go", Language: astgraph.LanguageGo, NodeCount: 1},
			{Path: "internal/cache/cache.go", Language: astgraph.LanguageGo, NodeCount: 2},
			{Path: "internal/source/source.go", Language: astgraph.LanguageGo, NodeCount: 3},
		},
	})
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewInspectService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	files, err := service.Files("demo@v1", astgraph.GraphInspectOptions{
		SyncIndex:  false,
		PathFilter: "internal/",
		Limit:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files.Files) != 1 || files.Files[0].Path != "internal/cache/cache.go" {
		t.Fatalf("files = %#v, want first internal file", files)
	}
}

func TestInspectNodeReturnsCallsAndSourceSnippet(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main

func Run() {
	helper()
}

func helper() {}
`)
	result, err := astgraph.NewIndexer(astgraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	replaceStoredGraph(t, sourceDir, result)
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewInspectService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	node, err := service.Node("demo@v1", "Run", astgraph.GraphInspectOptions{SyncIndex: false, SourceLines: 2})
	if err != nil {
		t.Fatal(err)
	}
	if node.Node == nil {
		t.Fatalf("node = %#v, want single Run node", node)
	}
	if node.Node.Name != "Run" || node.Node.Path != "main.go" {
		t.Fatalf("node = %#v, want Run in main.go", node.Node)
	}
	if len(node.Node.Source) != 2 || node.Node.Source[0].Line != 3 || !strings.Contains(node.Node.Source[0].Text, "func Run") {
		t.Fatalf("source = %#v, want first two function lines", node.Node.Source)
	}
	if len(node.Node.Calls) != 1 || node.Node.Calls[0] != "helper" {
		t.Fatalf("calls = %#v, want helper", node.Node.Calls)
	}
}

func TestInspectNodeReportsAmbiguousNameMatches(t *testing.T) {
	sourceDir := t.TempDir()
	graphDir, err := cache.GraphDirForSource(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = graphDir
	replaceStoredGraph(t, sourceDir, astgraph.IndexResult{
		SourcePath:    sourceDir,
		SchemaVersion: astgraph.SchemaVersion,
		CompletedAt:   time.Now().UTC(),
		Nodes: []astgraph.GraphNode{
			{ID: "run-1", Kind: astgraph.NodeKindFunction, Name: "Run", QualifiedName: "main.Run", FilePath: "main.go", Language: astgraph.LanguageGo},
			{ID: "run-2", Kind: astgraph.NodeKindMethod, Name: "Run", QualifiedName: "worker.Run", FilePath: "worker.go", Language: astgraph.LanguageGo},
		},
	})
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewInspectService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Node("demo@v1", "Run", astgraph.GraphInspectOptions{SyncIndex: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Node != nil || len(result.Matches) != 2 {
		t.Fatalf("result = %#v, want two ambiguous matches", result)
	}
}
