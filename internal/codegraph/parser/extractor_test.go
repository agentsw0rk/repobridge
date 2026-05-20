package parser

import (
	"testing"

	"repobridge/internal/codegraph"
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

	result, err := ExtractFromSource("service.go", source, codegraph.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	assertNode(t, result.Nodes, codegraph.NodeKindFunction, "helper")
	assertNode(t, result.Nodes, codegraph.NodeKindMethod, "Run")
	assertUnresolved(t, result.Unresolved, "helper")
	assertUnresolved(t, result.Unresolved, "Println")
	runID := findNodeID(t, result.Nodes, codegraph.NodeKindMethod, "Run")
	assertUnresolvedFrom(t, result.Unresolved, "helper", runID)
}

func assertNode(t *testing.T, nodes []codegraph.GraphNode, kind codegraph.NodeKind, name string) {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return
		}
	}
	t.Fatalf("node %s %s not found in %#v", kind, name, nodes)
}

func findNodeID(t *testing.T, nodes []codegraph.GraphNode, kind codegraph.NodeKind, name string) string {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return node.ID
		}
	}
	t.Fatalf("node %s %s not found in %#v", kind, name, nodes)
	return ""
}

func assertUnresolved(t *testing.T, refs []codegraph.UnresolvedReference, name string) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name {
			return
		}
	}
	t.Fatalf("reference %s not found in %#v", name, refs)
}

func assertUnresolvedFrom(t *testing.T, refs []codegraph.UnresolvedReference, name string, fromID string) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name && ref.FromNodeID == fromID {
			return
		}
	}
	t.Fatalf("reference %s from %s not found in %#v", name, fromID, refs)
}
