package astgraph

import (
	"reflect"
	"testing"
)

func TestGraphQueriesSearchFiltersByResolvedAndUnresolvedCalls(t *testing.T) {
	queries := NewGraphQueries(GraphSnapshot{
		SourcePath: "/cache/repo",
		Nodes: []GraphNode{
			{
				ID:            "run",
				Kind:          NodeKindFunction,
				Name:          "Run",
				QualifiedName: "app.Run",
				FilePath:      "app.go",
				Language:      LanguageGo,
				StartLine:     10,
				EndLine:       20,
			},
			{
				ID:            "save",
				Kind:          NodeKindFunction,
				Name:          "Save",
				QualifiedName: "store.Save",
				FilePath:      "store.go",
				Language:      LanguageGo,
				StartLine:     4,
				EndLine:       8,
			},
		},
		Edges: []GraphEdge{
			{SourceNodeID: "run", TargetNodeID: "save", Kind: EdgeKindCalls},
		},
		Unresolved: []UnresolvedReference{
			{FromNodeID: "run", ReferenceName: "fmt.Println", ReferenceKind: EdgeKindCalls},
		},
	}, QueryOptions{SourceLabel: "demo@v1"})

	results, err := queries.Search(SearchQuery{
		Text:  "Run",
		Calls: []string{"println"},
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %#v, want one result", results)
	}
	got := results[0]
	if got.Source != "demo@v1" || got.ID != "run" || got.Name != "Run" {
		t.Fatalf("result = %#v", got)
	}
	wantCalls := []string{"Save", "fmt.Println"}
	if !reflect.DeepEqual(got.Calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", got.Calls, wantCalls)
	}
}
