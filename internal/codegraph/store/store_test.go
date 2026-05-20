package store

import (
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
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

func TestStoreFilesReturnsStoredFileMetadata(t *testing.T) {
	graph := openTestStore(t)
	modifiedAt := time.Now().UTC().Truncate(time.Millisecond)
	indexedAt := modifiedAt.Add(time.Second)

	err := graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/repo",
		SchemaVersion: 1,
		CompletedAt:   indexedAt,
		Files: []codegraph.GraphFile{{
			Path:        "main.go",
			Language:    codegraph.LanguageGo,
			ContentHash: "abc123",
			Size:        42,
			ModifiedAt:  modifiedAt,
			IndexedAt:   indexedAt,
			NodeCount:   3,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	files, err := graph.Files()
	if err != nil {
		t.Fatal(err)
	}
	want := []codegraph.GraphFile{{
		Path:        "main.go",
		Language:    codegraph.LanguageGo,
		ContentHash: "abc123",
		Size:        42,
		ModifiedAt:  modifiedAt,
		IndexedAt:   indexedAt,
		NodeCount:   3,
	}}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %#v, want %#v", files, want)
	}
}

func TestStoreReplacePersistsWarningsInStatusErrorText(t *testing.T) {
	graph := openTestStore(t)

	err := graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/repo",
		SchemaVersion: 1,
		CompletedAt:   time.Now(),
		Warnings:      []string{"main.go: unsupported syntax", "src/app.ts: extract failed"},
	})
	if err != nil {
		t.Fatal(err)
	}

	status, err := graph.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "complete" {
		t.Fatalf("status = %#v, want complete", status)
	}
	if got, want := status.ErrorText, "main.go: unsupported syntax\nsrc/app.ts: extract failed"; got != want {
		t.Fatalf("error text = %q, want %q", got, want)
	}
}

func TestStoreMarkFailedUpdatesStatusWithoutRemovingGraphData(t *testing.T) {
	graph := openTestStore(t)

	err := graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/repo",
		SchemaVersion: 1,
		CompletedAt:   time.Now(),
		Nodes: []codegraph.GraphNode{{
			ID:            "n1",
			Kind:          codegraph.NodeKindFunction,
			Name:          "Existing",
			QualifiedName: "Existing",
			FilePath:      "main.go",
			Language:      codegraph.LanguageGo,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	startedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := graph.MarkFailed("/cache/repo", startedAt, errors.New("boom")); err != nil {
		t.Fatal(err)
	}

	status, err := graph.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "failed" || status.SourcePath != "/cache/repo" || status.SchemaVersion != codegraph.SchemaVersion {
		t.Fatalf("status = %#v, want failed current schema for /cache/repo", status)
	}
	if status.ErrorText != "boom" {
		t.Fatalf("error text = %q, want boom", status.ErrorText)
	}

	results, err := graph.Search(codegraph.SearchQuery{NameFilters: []string{"Existing"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "Existing" {
		t.Fatalf("results after MarkFailed = %#v, want existing graph data", results)
	}
}

func TestStoreReplaceRollsBackInvalidReplacement(t *testing.T) {
	graph := openTestStore(t)

	completedAt := time.Now().UTC().Truncate(time.Millisecond)
	err := graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/repo",
		SchemaVersion: 1,
		CompletedAt:   completedAt,
		Nodes: []codegraph.GraphNode{{
			ID:            "stable-existing",
			Kind:          codegraph.NodeKindFunction,
			Name:          "Existing",
			QualifiedName: "cache.Existing",
			FilePath:      "internal/cache/cache.go",
			Language:      codegraph.LanguageGo,
			StartLine:     10,
			EndLine:       20,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/replacement",
		SchemaVersion: 2,
		CompletedAt:   completedAt.Add(time.Minute),
		Nodes: []codegraph.GraphNode{
			{
				ID:            "duplicate-stable-id",
				Kind:          codegraph.NodeKindFunction,
				Name:          "ReplacementOne",
				QualifiedName: "cache.ReplacementOne",
				FilePath:      "internal/cache/replacement.go",
				Language:      codegraph.LanguageGo,
				StartLine:     1,
				EndLine:       2,
			},
			{
				ID:            "duplicate-stable-id",
				Kind:          codegraph.NodeKindFunction,
				Name:          "ReplacementTwo",
				QualifiedName: "cache.ReplacementTwo",
				FilePath:      "internal/cache/replacement.go",
				Language:      codegraph.LanguageGo,
				StartLine:     3,
				EndLine:       4,
			},
		},
	})
	if err == nil {
		t.Fatal("Replace returned nil error for duplicate stable IDs")
	}

	status, err := graph.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "complete" || status.SourcePath != "/cache/repo" || status.SchemaVersion != 1 {
		t.Fatalf("status after failed replace = %#v", status)
	}

	results, err := graph.Search(codegraph.SearchQuery{NameFilters: []string{"Existing"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultQualifiedNames(results); !reflect.DeepEqual(got, []string{"cache.Existing"}) {
		t.Fatalf("existing results after failed replace = %#v, want cache.Existing", got)
	}

	results, err = graph.Search(codegraph.SearchQuery{NameFilters: []string{"Replacement"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("replacement results after failed replace = %#v, want none", results)
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

func TestStoreSearchSortsTiedResultsByStableFields(t *testing.T) {
	graph := openTestStore(t)

	err := graph.Replace(codegraph.IndexResult{
		SourcePath:    "/cache/repo",
		SchemaVersion: 1,
		CompletedAt:   time.Now(),
		Nodes: []codegraph.GraphNode{
			{
				ID:            "stable-4",
				Kind:          codegraph.NodeKindMethod,
				Name:          "Shared",
				QualifiedName: "pkg.Shared",
				FilePath:      "internal/shared.go",
				Language:      codegraph.LanguageTypeScript,
				StartLine:     10,
				EndLine:       20,
			},
			{
				ID:            "stable-3",
				Kind:          codegraph.NodeKindMethod,
				Name:          "Shared",
				QualifiedName: "pkg.Shared",
				FilePath:      "internal/shared.go",
				Language:      codegraph.LanguageGo,
				StartLine:     10,
				EndLine:       30,
			},
			{
				ID:            "stable-2",
				Kind:          codegraph.NodeKindFunction,
				Name:          "Shared",
				QualifiedName: "pkg.Shared",
				FilePath:      "internal/shared.go",
				Language:      codegraph.LanguageGo,
				StartLine:     10,
				EndLine:       30,
			},
			{
				ID:            "stable-5",
				Kind:          codegraph.NodeKindMethod,
				Name:          "Shared",
				QualifiedName: "pkg.Shared",
				FilePath:      "internal/shared.go",
				Language:      codegraph.LanguageTypeScript,
				StartLine:     10,
				EndLine:       10,
			},
			{
				ID:            "stable-1",
				Kind:          codegraph.NodeKindFunction,
				Name:          "Shared",
				QualifiedName: "alpha.Shared",
				FilePath:      "internal/shared.go",
				Language:      codegraph.LanguageGo,
				StartLine:     10,
				EndLine:       50,
			},
			{
				ID:            "stable-0",
				Kind:          codegraph.NodeKindFunction,
				Name:          "Shared",
				QualifiedName: "pkg.Shared",
				FilePath:      "internal/shared.go",
				Language:      codegraph.LanguageGo,
				StartLine:     10,
				EndLine:       30,
			},
		},
		Unresolved: []codegraph.UnresolvedReference{
			{FromNodeID: "stable-2", ReferenceName: "stable-2-call", ReferenceKind: codegraph.EdgeKindCalls},
			{FromNodeID: "stable-0", ReferenceName: "stable-0-call", ReferenceKind: codegraph.EdgeKindCalls},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := graph.Search(codegraph.SearchQuery{NameFilters: []string{"Shared"}, Limit: 4})
	if err != nil {
		t.Fatal(err)
	}

	got := resultQualifiedNamesKindsLanguagesEndLines(results)
	want := []string{
		"alpha.Shared|function|go|50|",
		"pkg.Shared|function|go|30|stable-0-call",
		"pkg.Shared|function|go|30|stable-2-call",
		"pkg.Shared|method|go|30|",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tied result order = %#v, want %#v", got, want)
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

func resultQualifiedNames(results []codegraph.SearchResult) []string {
	names := make([]string, 0, len(results))
	for _, result := range results {
		names = append(names, result.QualifiedName)
	}
	return names
}

func resultQualifiedNamesKindsLanguagesEndLines(results []codegraph.SearchResult) []string {
	values := make([]string, 0, len(results))
	for _, result := range results {
		values = append(values, result.QualifiedName+"|"+string(result.Kind)+"|"+string(result.Language)+"|"+strconv.Itoa(result.EndLine)+"|"+strings.Join(result.Calls, ","))
	}
	return values
}
