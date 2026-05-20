package parser

import (
	"strings"
	"testing"

	"repobridge/internal/codegraph/model"
)

func TestExtractFromSourceFindsGoFunctionsMethodsAndCalls(t *testing.T) {
	source := []byte(`package demo

import "fmt"

func helper() {}

type Service struct{}

func (s Service) Run() {
	helper()
	fmt.Println("ok")
}
`)

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	assertNode(t, result.Nodes, model.NodeKindFunction, "helper")
	assertNode(t, result.Nodes, model.NodeKindMethod, "Run")
	assertUnresolved(t, result.Unresolved, "helper")
	assertUnresolved(t, result.Unresolved, "Println")
	runID := findNodeID(t, result.Nodes, model.NodeKindMethod, "Run")
	assertUnresolvedFrom(t, result.Unresolved, "helper", runID)
}

func TestExtractFromSourceSkipsGoCallsWithoutOwner(t *testing.T) {
	source := []byte("package demo\nvar x = helper()\nfunc helper() {}\n")

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	assertNoUnresolved(t, result.Unresolved, "helper")
}

func TestExtractFromSourceSkipsGoCallsInsideFunctionLiterals(t *testing.T) {
	source := []byte("package demo\nfunc outer() func() { return func() { helper() } }\nfunc helper() {}\n")

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	outerID := findNodeID(t, result.Nodes, model.NodeKindFunction, "outer")
	assertNoUnresolvedFrom(t, result.Unresolved, "helper", outerID)
	assertNoUnresolved(t, result.Unresolved, "helper")
}

func TestExtractFromSourceSkipsGoComplexCallTargets(t *testing.T) {
	source := []byte("package demo\nfunc outer() { func() {}() }\n")

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	assertNoUnresolvedContaining(t, result.Unresolved, "func")
}

func TestExtractFromSourceFindsJavaScriptFunctionAndCall(t *testing.T) {
	source := []byte(`function render() { updateContainer(); }`)

	result, err := ExtractFromSource("app.js", source, model.LanguageJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindFunction, "render", model.LanguageJavaScript)
	renderID := findNodeID(t, result.Nodes, model.NodeKindFunction, "render")
	assertUnresolvedWithLanguage(t, result.Unresolved, "updateContainer", model.LanguageJavaScript)
	assertUnresolvedFrom(t, result.Unresolved, "updateContainer", renderID)
}

func TestExtractFromSourceFindsTypeScriptFunctionAndCall(t *testing.T) {
	source := []byte(`function render(): void { updateContainer(); }`)

	result, err := ExtractFromSource("app.ts", source, model.LanguageTypeScript)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindFunction, "render", model.LanguageTypeScript)
	renderID := findNodeID(t, result.Nodes, model.NodeKindFunction, "render")
	assertUnresolvedWithLanguage(t, result.Unresolved, "updateContainer", model.LanguageTypeScript)
	assertUnresolvedFrom(t, result.Unresolved, "updateContainer", renderID)
}

func TestExtractFromSourceFindsPythonFunctionAndCall(t *testing.T) {
	source := []byte("def send():\n    request()\n")

	result, err := ExtractFromSource("client.py", source, model.LanguagePython)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindFunction, "send", model.LanguagePython)
	sendID := findNodeID(t, result.Nodes, model.NodeKindFunction, "send")
	assertUnresolvedWithLanguage(t, result.Unresolved, "request", model.LanguagePython)
	assertUnresolvedFrom(t, result.Unresolved, "request", sendID)
}

func TestExtractFromSourceFindsRustFunctionAndCall(t *testing.T) {
	source := []byte(`fn run() { helper(); }`)

	result, err := ExtractFromSource("main.rs", source, model.LanguageRust)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindFunction, "run", model.LanguageRust)
	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "run")
	assertUnresolvedWithLanguage(t, result.Unresolved, "helper", model.LanguageRust)
	assertUnresolvedFrom(t, result.Unresolved, "helper", runID)
}

func TestExtractFromSourceFindsJavaMethodAndCall(t *testing.T) {
	source := []byte(`class App { void run() { helper(); } }`)

	result, err := ExtractFromSource("App.java", source, model.LanguageJava)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindMethod, "run", model.LanguageJava)
	runID := findNodeID(t, result.Nodes, model.NodeKindMethod, "run")
	assertUnresolvedWithLanguage(t, result.Unresolved, "helper", model.LanguageJava)
	assertUnresolvedFrom(t, result.Unresolved, "helper", runID)
}

func TestExtractFromSourceFindsCSharpMethodAndCall(t *testing.T) {
	source := []byte(`class App { void Run() { Helper(); } }`)

	result, err := ExtractFromSource("App.cs", source, model.LanguageCSharp)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindMethod, "Run", model.LanguageCSharp)
	runID := findNodeID(t, result.Nodes, model.NodeKindMethod, "Run")
	assertUnresolvedWithLanguage(t, result.Unresolved, "Helper", model.LanguageCSharp)
	assertUnresolvedFrom(t, result.Unresolved, "Helper", runID)
}

func TestExtractFromSourceHandlesKotlinSupportStatus(t *testing.T) {
	result, err := ExtractFromSource("App.kt", []byte(`fun run() { helper() }`), model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}
	assertWarning(t, result.Warnings, "unsupported language: kotlin")
	if len(result.Nodes) != 0 {
		t.Fatalf("expected no nodes, got %#v", result.Nodes)
	}
	if len(result.Unresolved) != 0 {
		t.Fatalf("expected no unresolved references, got %#v", result.Unresolved)
	}
}

func TestExtractFromSourceWarnsForUnsupportedLanguage(t *testing.T) {
	result, err := ExtractFromSource("file.unknown", []byte("content"), model.LanguageUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "unsupported language: unknown" {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}
	if len(result.Nodes) != 0 {
		t.Fatalf("expected no nodes, got %#v", result.Nodes)
	}
	if len(result.Unresolved) != 0 {
		t.Fatalf("expected no unresolved references, got %#v", result.Unresolved)
	}
}

func assertNodeWithLanguage(t *testing.T, nodes []model.GraphNode, kind model.NodeKind, name string, language model.Language) {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name && node.Language == language {
			return
		}
	}
	t.Fatalf("node %s %s with language %s not found in %#v", kind, name, language, nodes)
}

func assertNode(t *testing.T, nodes []model.GraphNode, kind model.NodeKind, name string) {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return
		}
	}
	t.Fatalf("node %s %s not found in %#v", kind, name, nodes)
}

func findNodeID(t *testing.T, nodes []model.GraphNode, kind model.NodeKind, name string) string {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return node.ID
		}
	}
	t.Fatalf("node %s %s not found in %#v", kind, name, nodes)
	return ""
}

func assertUnresolved(t *testing.T, refs []model.UnresolvedReference, name string) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name {
			return
		}
	}
	t.Fatalf("reference %s not found in %#v", name, refs)
}

func assertUnresolvedWithLanguage(t *testing.T, refs []model.UnresolvedReference, name string, language model.Language) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name && ref.Language == language {
			return
		}
	}
	t.Fatalf("reference %s with language %s not found in %#v", name, language, refs)
}

func assertUnresolvedFrom(t *testing.T, refs []model.UnresolvedReference, name string, fromID string) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name && ref.FromNodeID == fromID {
			return
		}
	}
	t.Fatalf("reference %s from %s not found in %#v", name, fromID, refs)
}

func assertNoUnresolvedFrom(t *testing.T, refs []model.UnresolvedReference, name string, fromID string) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name && ref.FromNodeID == fromID {
			t.Fatalf("unexpected reference %s from %s found in %#v", name, fromID, refs)
		}
	}
}

func assertNoUnresolved(t *testing.T, refs []model.UnresolvedReference, name string) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name {
			t.Fatalf("unexpected reference %s found in %#v", name, refs)
		}
	}
}

func assertNoUnresolvedContaining(t *testing.T, refs []model.UnresolvedReference, text string) {
	t.Helper()
	for _, ref := range refs {
		if strings.Contains(ref.ReferenceName, text) {
			t.Fatalf("unexpected reference containing %q found in %#v", text, refs)
		}
	}
}

func assertWarning(t *testing.T, warnings []string, want string) {
	t.Helper()
	for _, warning := range warnings {
		if warning == want {
			return
		}
	}
	t.Fatalf("warning %q not found in %#v", want, warnings)
}
