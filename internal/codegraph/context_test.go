package codegraph_test

import (
	"strings"
	"testing"
	"time"

	"repobridge/internal/codegraph"
	"repobridge/internal/source"
)

func TestContextServiceBuildsFocusedContext(t *testing.T) {
	sourceDir := t.TempDir()
	writeCodegraphFixture(t, sourceDir, "auth.go", `package auth

func Login() {
	createSession()
}

func createSession() {}
`)
	index, err := codegraph.NewIndexer(codegraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	index.CompletedAt = time.Now().UTC()
	index.Nodes = []codegraph.GraphNode{
		{ID: "login", Kind: codegraph.NodeKindFunction, Name: "Login", QualifiedName: "auth.Login", FilePath: "auth.go", Language: codegraph.LanguageGo, StartLine: 3, EndLine: 5, Signature: "func Login()"},
		{ID: "session", Kind: codegraph.NodeKindFunction, Name: "createSession", QualifiedName: "auth.createSession", FilePath: "auth.go", Language: codegraph.LanguageGo, StartLine: 7, EndLine: 7, Signature: "func createSession()"},
	}
	index.Edges = []codegraph.GraphEdge{
		{SourceNodeID: "login", TargetNodeID: "session", Kind: codegraph.EdgeKindCalls, FilePath: "auth.go", Line: 4},
	}
	replaceStoredGraph(t, sourceDir, index)
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := codegraph.NewContextService(codegraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Context("demo@v1", "login flow", codegraph.ContextOptions{
		Mode:      codegraph.ContextModeContext,
		Budget:    "small",
		SyncIndex: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "demo@v1" || result.Mode != codegraph.ContextModeContext || result.Budget.Name != "small" {
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
	writeCodegraphFixture(t, sourceDir, "service.go", `package auth

func AuthServiceLogin() {
	SessionRepositorySave()
}

func SessionRepositorySave() {}
`)
	index, err := codegraph.NewIndexer(codegraph.IndexOptions{}).Index(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	index.CompletedAt = time.Now().UTC()
	index.Nodes = []codegraph.GraphNode{
		{ID: "login", Kind: codegraph.NodeKindFunction, Name: "AuthServiceLogin", QualifiedName: "AuthService.login", FilePath: "service.go", Language: codegraph.LanguageGo, StartLine: 3, EndLine: 5},
		{ID: "save", Kind: codegraph.NodeKindFunction, Name: "SessionRepositorySave", QualifiedName: "SessionRepository.save", FilePath: "service.go", Language: codegraph.LanguageGo, StartLine: 7, EndLine: 7},
	}
	index.Edges = []codegraph.GraphEdge{
		{SourceNodeID: "login", TargetNodeID: "save", Kind: codegraph.EdgeKindCalls, FilePath: "service.go", Line: 4},
	}
	replaceStoredGraph(t, sourceDir, index)
	resolver := &fakeSourceResolver{
		outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
	}
	service := codegraph.NewContextService(codegraph.SearchServiceOptions{
		Resolver:    resolver,
		StoreOpener: openGraphStore,
	})

	result, err := service.Context("demo@v1", "AuthService.login SessionRepository.save", codegraph.ContextOptions{
		Mode:      codegraph.ContextModeExplore,
		Budget:    "large",
		SyncIndex: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != codegraph.ContextModeExplore || result.Budget.Name != "large" {
		t.Fatalf("mode/budget = %s %#v", result.Mode, result.Budget)
	}
	if len(result.EntryPoints) != 2 {
		t.Fatalf("entry points = %#v, want two concrete symbols", result.EntryPoints)
	}
	if len(result.Relationships) != 1 {
		t.Fatalf("relationships = %#v, want relationship map", result.Relationships)
	}
}
