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

func TestCallgraphServiceDefaultsContainmentTraversalDeepEnoughForKotlinFile(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "demo/Service.kt", `package demo
import kotlin.collections.List

class Service {
  fun run() { helper() }
  fun helper() {}
}
`)
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewCallgraphService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Callgraph("demo@v1", "demo/Service.kt", astgraph.CallgraphOptions{
		Direction: astgraph.CallgraphDirectionCallees,
		SyncIndex: true,
		Limit:     20,
		EdgeKinds: []astgraph.EdgeKind{astgraph.EdgeKindContains},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !callgraphContainsTarget(result.Edges, "demo.Service.run") {
		t.Fatalf("edges = %#v, want file containment traversal to include Service.run", result.Edges)
	}
}

func TestCallgraphServiceDefaultsContainmentTraversalDeepEnoughForKotlinClassCallees(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "api/Port.kt", `package api

interface Port {
  fun save(id: String)
}
`)
	writeASTGraphFixture(t, sourceDir, "svc/Service.kt", `package svc

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
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewCallgraphService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Callgraph("demo@v1", "Service", astgraph.CallgraphOptions{
		Direction: astgraph.CallgraphDirectionCallees,
		SyncIndex: true,
		Limit:     20,
		EdgeKinds: []astgraph.EdgeKind{astgraph.EdgeKindContains, astgraph.EdgeKindCalls},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !callgraphContainsTarget(result.Edges, "api.Port.save") {
		t.Fatalf("edges = %#v, want class containment traversal to include injected interface Port.save callee", result.Edges)
	}
}

func callgraphContainsTarget(edges []astgraph.CallgraphEdge, qualifiedName string) bool {
	for _, edge := range edges {
		if edge.To.QualifiedName == qualifiedName {
			return true
		}
	}
	return false
}
