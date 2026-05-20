package parser

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"repobridge/internal/codegraph"
)

func walkGo(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkGoNode(path, source, node, result, "")
}

func walkGoNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "function_declaration":
		if id := appendGoNode(path, source, node, codegraph.NodeKindFunction, result); id != "" {
			currentNodeID = id
		}
	case "method_declaration":
		if id := appendGoNode(path, source, node, codegraph.NodeKindMethod, result); id != "" {
			currentNodeID = id
		}
	case "call_expression":
		appendGoCall(path, source, node, result, currentNodeID)
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkGoNode(path, source, node.NamedChild(i), result, currentNodeID)
	}
}

func appendGoNode(path string, source []byte, node *tree_sitter.Node, kind codegraph.NodeKind, result *ExtractionResult) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	name := nodeText(source, nameNode)
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	id := stableNodeID(path, kind, name, startLine)
	result.Nodes = append(result.Nodes, codegraph.GraphNode{
		ID:            id,
		Kind:          kind,
		Name:          name,
		QualifiedName: name,
		FilePath:      path,
		Language:      codegraph.LanguageGo,
		StartLine:     startLine,
		EndLine:       int(end.Row) + 1,
		StartColumn:   int(start.Column),
		EndColumn:     int(end.Column),
		Signature:     strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func appendGoCall(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, fromNodeID string) {
	functionNode := node.ChildByFieldName("function")
	if functionNode == nil {
		return
	}
	name := nodeText(source, functionNode)
	if lastDot := strings.LastIndex(name, "."); lastDot >= 0 {
		name = name[lastDot+1:]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}

	start := functionNode.StartPosition()
	result.Unresolved = append(result.Unresolved, codegraph.UnresolvedReference{
		FromNodeID:    fromNodeID,
		ReferenceName: name,
		ReferenceKind: codegraph.EdgeKindCalls,
		FilePath:      path,
		Language:      codegraph.LanguageGo,
		Line:          int(start.Row) + 1,
		Column:        int(start.Column),
	})
}

func nodeText(source []byte, node *tree_sitter.Node) string {
	start := int(node.StartByte())
	end := int(node.EndByte())
	if start < 0 || end < start || end > len(source) {
		return ""
	}
	return string(source[start:end])
}
