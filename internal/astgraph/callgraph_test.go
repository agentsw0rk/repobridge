package astgraph_test

import (
	"testing"
	"time"

	"repobridge/internal/astgraph"
	"repobridge/internal/source"
)

func TestCallgraphServiceReturnsCalleesForResolvedSymbol(t *testing.T) {
	sourceDir := t.TempDir()
	replaceStoredGraph(t, sourceDir, astgraph.IndexResult{
		SourcePath:    sourceDir,
		SchemaVersion: astgraph.SchemaVersion,
		CompletedAt:   time.Now().UTC(),
		Nodes: []astgraph.GraphNode{
			{ID: "run", Kind: astgraph.NodeKindFunction, Name: "Run", QualifiedName: "main.Run", FilePath: "main.go", Language: astgraph.LanguageGo, StartLine: 3},
			{ID: "helper", Kind: astgraph.NodeKindFunction, Name: "helper", QualifiedName: "main.helper", FilePath: "main.go", Language: astgraph.LanguageGo, StartLine: 7},
		},
		Edges: []astgraph.GraphEdge{
			{SourceNodeID: "run", TargetNodeID: "helper", Kind: astgraph.EdgeKindCalls, FilePath: "main.go", Line: 4},
		},
	})
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewCallgraphService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Callgraph("demo@v1", "Run", astgraph.CallgraphOptions{
		Direction: astgraph.CallgraphDirectionCallees,
		SyncIndex: false,
		Depth:     1,
		Limit:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "demo@v1" || result.Symbol != "Run" || result.Direction != astgraph.CallgraphDirectionCallees {
		t.Fatalf("result header = %#v, want demo Run callees", result)
	}
	if result.Root == nil || result.Root.ID != "run" {
		t.Fatalf("root = %#v, want run", result.Root)
	}
	if len(result.Edges) != 1 || result.Edges[0].From.ID != "run" || result.Edges[0].To.ID != "helper" {
		t.Fatalf("edges = %#v, want run -> helper", result.Edges)
	}
}

func TestCallgraphServiceReportsAmbiguousSymbolMatches(t *testing.T) {
	sourceDir := t.TempDir()
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
	service := astgraph.NewCallgraphService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Callgraph("demo@v1", "Run", astgraph.CallgraphOptions{
		Direction: astgraph.CallgraphDirectionCallers,
		SyncIndex: false,
		Depth:     1,
		Limit:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Root != nil || len(result.Matches) != 2 || len(result.Edges) != 0 {
		t.Fatalf("result = %#v, want ambiguous matches only", result)
	}
}

func TestCallgraphServiceIncludesUnresolvedCallees(t *testing.T) {
	sourceDir := t.TempDir()
	replaceStoredGraph(t, sourceDir, astgraph.IndexResult{
		SourcePath:    sourceDir,
		SchemaVersion: astgraph.SchemaVersion,
		CompletedAt:   time.Now().UTC(),
		Nodes: []astgraph.GraphNode{
			{ID: "run", Kind: astgraph.NodeKindFunction, Name: "Run", QualifiedName: "main.Run", FilePath: "main.go", Language: astgraph.LanguageGo, StartLine: 3},
		},
		Unresolved: []astgraph.UnresolvedReference{
			{FromNodeID: "run", ReferenceName: "fmt.Println", ReferenceKind: astgraph.EdgeKindCalls, FilePath: "main.go", Language: astgraph.LanguageGo, Line: 4},
		},
	})
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewCallgraphService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Callgraph("demo@v1", "Run", astgraph.CallgraphOptions{
		Direction:         astgraph.CallgraphDirectionCallees,
		SyncIndex:         false,
		Depth:             1,
		Limit:             10,
		IncludeUnresolved: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edges) != 1 || !result.Edges[0].Unresolved || result.Edges[0].ReferenceName != "fmt.Println" {
		t.Fatalf("edges = %#v, want unresolved fmt.Println", result.Edges)
	}
}
