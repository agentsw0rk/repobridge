package astgraph_test

import (
	"strings"
	"testing"
	"time"

	"repobridge/internal/astgraph"
	"repobridge/internal/source"
)

func TestContextServiceBuildsFocusedContext(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "auth.go", `package auth

func Login() {
	createSession()
}

func createSession() {}
`)
	index, err := astgraph.NewIndexer(astgraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	index.CompletedAt = time.Now().UTC()
	index.Nodes = []astgraph.GraphNode{
		{ID: "login", Kind: astgraph.NodeKindFunction, Name: "Login", QualifiedName: "auth.Login", FilePath: "auth.go", Language: astgraph.LanguageGo, StartLine: 3, EndLine: 5, Signature: "func Login()"},
		{ID: "session", Kind: astgraph.NodeKindFunction, Name: "createSession", QualifiedName: "auth.createSession", FilePath: "auth.go", Language: astgraph.LanguageGo, StartLine: 7, EndLine: 7, Signature: "func createSession()"},
	}
	index.Edges = []astgraph.GraphEdge{
		{SourceNodeID: "login", TargetNodeID: "session", Kind: astgraph.EdgeKindCalls, FilePath: "auth.go", Line: 4},
	}
	replaceStoredGraph(t, sourceDir, index)
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewContextService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Context("demo@v1", "login flow", astgraph.ContextOptions{
		Mode:      astgraph.ContextModeContext,
		Budget:    "small",
		SyncIndex: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "demo@v1" || result.Mode != astgraph.ContextModeContext || result.Budget.Name != "small" {
		t.Fatalf("result header = %#v", result)
	}
	if len(result.EntryPoints) != 1 || result.EntryPoints[0].Name != "Login" {
		t.Fatalf("entry points = %#v, want Login", result.EntryPoints)
	}
	if len(result.Relationships) != 1 || result.Relationships[0].From.ID != "login" || result.Relationships[0].To.ID != "session" {
		t.Fatalf("relationships = %#v, want login -> session", result.Relationships)
	}
	if len(result.Snippets) != 1 || result.Snippets[0].Path != "auth.go" {
		t.Fatalf("snippets = %#v, want auth.go", result.Snippets)
	}
	if len(result.Snippets[0].Lines) == 0 || !strings.Contains(result.Snippets[0].Lines[0].Text, "func Login") {
		t.Fatalf("snippet lines = %#v, want Login source", result.Snippets[0].Lines)
	}
	if len(result.RelatedFiles) != 1 || result.RelatedFiles[0].Path != "auth.go" {
		t.Fatalf("related files = %#v, want auth.go", result.RelatedFiles)
	}
}

func TestContextServiceExploreUsesConcreteSymbolAndLargeBudget(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "service.go", `package auth

func AuthServiceLogin() {
	SessionRepositorySave()
}

func SessionRepositorySave() {}
`)
	index, err := astgraph.NewIndexer(astgraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	index.CompletedAt = time.Now().UTC()
	index.Nodes = []astgraph.GraphNode{
		{ID: "login", Kind: astgraph.NodeKindFunction, Name: "AuthServiceLogin", QualifiedName: "AuthService.login", FilePath: "service.go", Language: astgraph.LanguageGo, StartLine: 3, EndLine: 5},
		{ID: "save", Kind: astgraph.NodeKindFunction, Name: "SessionRepositorySave", QualifiedName: "SessionRepository.save", FilePath: "service.go", Language: astgraph.LanguageGo, StartLine: 7, EndLine: 7},
	}
	index.Edges = []astgraph.GraphEdge{
		{SourceNodeID: "login", TargetNodeID: "save", Kind: astgraph.EdgeKindCalls, FilePath: "service.go", Line: 4},
	}
	replaceStoredGraph(t, sourceDir, index)
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := astgraph.NewContextService(astgraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Context("demo@v1", "AuthService.login SessionRepository.save", astgraph.ContextOptions{
		Mode:      astgraph.ContextModeExplore,
		Budget:    "large",
		SyncIndex: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != astgraph.ContextModeExplore || result.Budget.Name != "large" {
		t.Fatalf("mode/budget = %s %#v", result.Mode, result.Budget)
	}
	if len(result.EntryPoints) != 2 {
		t.Fatalf("entry points = %#v, want two concrete symbols", result.EntryPoints)
	}
	if len(result.Relationships) != 1 {
		t.Fatalf("relationships = %#v, want relationship map", result.Relationships)
	}
}
