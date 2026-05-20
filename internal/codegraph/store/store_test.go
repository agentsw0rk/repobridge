package store

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"repobridge/internal/codegraph"
)

func TestStoreReplaceGraphAndSearchByName(t *testing.T) {
	graph, err := Open(filepath.Join(t.TempDir(), ".repobridge-graph"))
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()

	err = graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/repo",
		SchemaVersion: 1,
		CompletedAt:   time.Now(),
		Files: []codegraph.GraphFile{{
			Path:     "main.go",
			Language: codegraph.LanguageGo,
		}},
		Nodes: []codegraph.GraphNode{{
			ID:            "n1",
			Kind:          codegraph.NodeKindFunction,
			Name:          "EnsureCached",
			QualifiedName: "EnsureCached",
			FilePath:      "main.go",
			Language:      codegraph.LanguageGo,
			StartLine:     10,
			EndLine:       20,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := graph.Search(codegraph.SearchQuery{NameFilters: []string{"Ensure"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "EnsureCached" {
		t.Fatalf("results = %#v", results)
	}
}

func TestStoreCallsByNodeReturnsUnresolvedCallNames(t *testing.T) {
	graph := openTestStore(t)

	err := graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/repo",
		SchemaVersion: 1,
		CompletedAt:   time.Now(),
		Nodes: []codegraph.GraphNode{{
			ID:       "n1",
			Kind:     codegraph.NodeKindFunction,
			Name:     "Run",
			FilePath: "main.go",
			Language: codegraph.LanguageGo,
		}},
		Unresolved: []codegraph.UnresolvedReference{
			{FromNodeID: "n1", ReferenceName: "EnsureCached", ReferenceKind: codegraph.EdgeKindCalls},
			{FromNodeID: "n1", ReferenceName: "fmt.Println", ReferenceKind: codegraph.EdgeKindCalls},
			{FromNodeID: "n1", ReferenceName: "EnsureCached", ReferenceKind: codegraph.EdgeKindCalls},
			{FromNodeID: "n1", ReferenceName: "OtherImport", ReferenceKind: codegraph.EdgeKindImports},
			{FromNodeID: "n2", ReferenceName: "FromOtherNode", ReferenceKind: codegraph.EdgeKindCalls},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	calls, err := graph.CallsByNode("n1")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"EnsureCached", "fmt.Println"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestStoreStatusReportsMissingAndComplete(t *testing.T) {
	graph := openTestStore(t)

	status, err := graph.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "missing" {
		t.Fatalf("initial status = %#v, want missing", status)
	}

	completedAt := time.Now().UTC().Truncate(time.Millisecond)
	err = graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/repo",
		SchemaVersion: 1,
		CompletedAt:   completedAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	status, err = graph.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "complete" || status.SourcePath != "/cache/repo" || status.SchemaVersion != 1 {
		t.Fatalf("status = %#v", status)
	}
	if !status.CompletedAt.Equal(completedAt) {
		t.Fatalf("completed at = %s, want %s", status.CompletedAt, completedAt)
	}
}

func TestStoreSearchAppliesFiltersAndSortsDeterministically(t *testing.T) {
	graph := openTestStore(t)

	err := graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/repo",
		SchemaVersion: 1,
		CompletedAt:   time.Now(),
		Nodes: []codegraph.GraphNode{
			{
				ID:            "n1",
				Kind:          codegraph.NodeKindFunction,
				Name:          "EnsureCached",
				QualifiedName: "cache.EnsureCached",
				FilePath:      "internal/cache/cache.go",
				Language:      codegraph.LanguageGo,
				StartLine:     30,
				EndLine:       40,
			},
			{
				ID:            "n2",
				Kind:          codegraph.NodeKindMethod,
				Name:          "EnsureCached",
				QualifiedName: "Resolver.EnsureCached",
				FilePath:      "internal/source/cache.go",
				Language:      codegraph.LanguageGo,
				StartLine:     10,
				EndLine:       20,
			},
			{
				ID:            "n3",
				Kind:          codegraph.NodeKindFunction,
				Name:          "FetchSource",
				QualifiedName: "FetchSource",
				FilePath:      "internal/source/source.go",
				Language:      codegraph.LanguageGo,
				StartLine:     5,
				EndLine:       9,
			},
			{
				ID:            "n4",
				Kind:          codegraph.NodeKindFunction,
				Name:          "EnsureCached",
				QualifiedName: "ensureCached",
				FilePath:      "src/cache.ts",
				Language:      codegraph.LanguageTypeScript,
				StartLine:     1,
				EndLine:       4,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := graph.Search(codegraph.SearchQuery{
		Text:        "cache",
		Kinds:       []codegraph.NodeKind{codegraph.NodeKindFunction},
		Languages:   []codegraph.Language{codegraph.LanguageGo},
		PathFilters: []string{"internal/"},
		NameFilters: []string{"Ensure"},
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := resultNames(results); !reflect.DeepEqual(got, []string{"EnsureCached"}) {
		t.Fatalf("result names = %#v, results = %#v", got, results)
	}
	if results[0].Path != "internal/cache/cache.go" {
		t.Fatalf("result path = %q", results[0].Path)
	}

	results, err = graph.Search(codegraph.SearchQuery{NameFilters: []string{"Ensure"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	got := resultPaths(results)
	want := []string{"internal/cache/cache.go", "internal/source/cache.go", "src/cache.ts"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("result paths = %#v, want %#v", got, want)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	graph, err := Open(filepath.Join(t.TempDir(), ".repobridge-graph"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(graph.Close)
	return graph
}

func resultNames(results []codegraph.SearchResult) []string {
	names := make([]string, 0, len(results))
	for _, result := range results {
		names = append(names, result.Name)
	}
	return names
}

func resultPaths(results []codegraph.SearchResult) []string {
	paths := make([]string, 0, len(results))
	for _, result := range results {
		paths = append(paths, result.Path)
	}
	return paths
}
