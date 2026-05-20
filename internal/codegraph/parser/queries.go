package parser

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func walkGo(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkGoNode(path, source, node, result, "")
}

func walkByLanguage(path string, source []byte, node *tree_sitter.Node, language Language, result *ExtractionResult) {
	switch language {
	case LanguageGo:
		walkGo(path, source, node, result)
	case LanguageJavaScript:
		walkJavaScript(path, source, node, result)
	case LanguageTypeScript:
		walkTypeScript(path, source, node, result)
	case LanguagePython:
		walkPython(path, source, node, result)
	case LanguageRust:
		walkRust(path, source, node, result)
	case LanguageJava:
		walkJava(path, source, node, result)
	case LanguageCSharp:
		walkCSharp(path, source, node, result)
	}
}

type extractionConfig struct {
	language       Language
	nodeKinds      map[string]NodeKind
	callKinds      map[string]bool
	anonymousKinds map[string]bool
}

func walkJavaScript(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkConfiguredNode(path, source, node, result, "", extractionConfig{
		language: LanguageJavaScript,
		nodeKinds: map[string]NodeKind{
			"function_declaration": NodeKindFunction,
			"method_definition":    NodeKindMethod,
		},
		callKinds: map[string]bool{
			"call_expression": true,
		},
		anonymousKinds: map[string]bool{
			"arrow_function": true,
			"function":       true,
		},
	})
}

func walkTypeScript(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkConfiguredNode(path, source, node, result, "", extractionConfig{
		language: LanguageTypeScript,
		nodeKinds: map[string]NodeKind{
			"function_declaration": NodeKindFunction,
			"method_definition":    NodeKindMethod,
		},
		callKinds: map[string]bool{
			"call_expression": true,
		},
		anonymousKinds: map[string]bool{
			"arrow_function": true,
			"function":       true,
		},
	})
}

func walkPython(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkConfiguredNode(path, source, node, result, "", extractionConfig{
		language: LanguagePython,
		nodeKinds: map[string]NodeKind{
			"function_definition": NodeKindFunction,
		},
		callKinds: map[string]bool{
			"call": true,
		},
		anonymousKinds: map[string]bool{
			"lambda": true,
		},
	})
}

func walkRust(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkConfiguredNode(path, source, node, result, "", extractionConfig{
		language: LanguageRust,
		nodeKinds: map[string]NodeKind{
			"function_item": NodeKindFunction,
		},
		callKinds: map[string]bool{
			"call_expression": true,
		},
		anonymousKinds: map[string]bool{
			"closure_expression": true,
		},
	})
}

func walkJava(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkConfiguredNode(path, source, node, result, "", extractionConfig{
		language: LanguageJava,
		nodeKinds: map[string]NodeKind{
			"method_declaration": NodeKindMethod,
		},
		callKinds: map[string]bool{
			"method_invocation": true,
		},
		anonymousKinds: map[string]bool{
			"lambda_expression": true,
		},
	})
}

func walkCSharp(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkConfiguredNode(path, source, node, result, "", extractionConfig{
		language: LanguageCSharp,
		nodeKinds: map[string]NodeKind{
			"method_declaration": NodeKindMethod,
		},
		callKinds: map[string]bool{
			"invocation_expression": true,
		},
		anonymousKinds: map[string]bool{
			"anonymous_method_expression": true,
			"lambda_expression":           true,
		},
	})
}

func walkConfiguredNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string, config extractionConfig) {
	if node == nil {
		return
	}

	kind := node.Kind()
	if graphKind, ok := config.nodeKinds[kind]; ok {
		if id := appendLanguageNode(path, source, node, graphKind, config.language, result); id != "" {
			currentNodeID = id
		}
	} else if config.anonymousKinds[kind] {
		currentNodeID = ""
	} else if config.callKinds[kind] && currentNodeID != "" {
		appendLanguageCall(path, source, node, config.language, result, currentNodeID)
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkConfiguredNode(path, source, node.NamedChild(i), result, currentNodeID, config)
	}
}

func walkGoNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "function_declaration":
		if id := appendGoNode(path, source, node, NodeKindFunction, result); id != "" {
			currentNodeID = id
		}
	case "method_declaration":
		if id := appendGoNode(path, source, node, NodeKindMethod, result); id != "" {
			currentNodeID = id
		}
	case "func_literal":
		currentNodeID = ""
	case "call_expression":
		if currentNodeID != "" {
			appendGoCall(path, source, node, result, currentNodeID)
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkGoNode(path, source, node.NamedChild(i), result, currentNodeID)
	}
}

func appendGoNode(path string, source []byte, node *tree_sitter.Node, kind NodeKind, result *ExtractionResult) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	name := nodeText(source, nameNode)
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	id := stableNodeID(path, kind, name, startLine)
	result.Nodes = append(result.Nodes, GraphNode{
		ID:            id,
		Kind:          kind,
		Name:          name,
		QualifiedName: name,
		FilePath:      path,
		Language:      LanguageGo,
		StartLine:     startLine,
		EndLine:       int(end.Row) + 1,
		StartColumn:   int(start.Column),
		EndColumn:     int(end.Column),
		Signature:     strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func appendLanguageNode(path string, source []byte, node *tree_sitter.Node, kind NodeKind, language Language, result *ExtractionResult) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	name := nodeText(source, nameNode)
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	id := stableNodeID(path, kind, name, startLine)
	result.Nodes = append(result.Nodes, GraphNode{
		ID:            id,
		Kind:          kind,
		Name:          name,
		QualifiedName: name,
		FilePath:      path,
		Language:      language,
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

	name, ok := goCallReferenceName(source, functionNode)
	if !ok {
		return
	}

	start := functionNode.StartPosition()
	result.Unresolved = append(result.Unresolved, UnresolvedReference{
		FromNodeID:    fromNodeID,
		ReferenceName: name,
		ReferenceKind: EdgeKindCalls,
		FilePath:      path,
		Language:      LanguageGo,
		Line:          int(start.Row) + 1,
		Column:        int(start.Column),
	})
}

func appendLanguageCall(path string, source []byte, node *tree_sitter.Node, language Language, result *ExtractionResult, fromNodeID string) {
	nameNode, name, ok := callReference(source, node)
	if !ok {
		return
	}

	start := nameNode.StartPosition()
	result.Unresolved = append(result.Unresolved, UnresolvedReference{
		FromNodeID:    fromNodeID,
		ReferenceName: name,
		ReferenceKind: EdgeKindCalls,
		FilePath:      path,
		Language:      language,
		Line:          int(start.Row) + 1,
		Column:        int(start.Column),
	})
}

func callReference(source []byte, node *tree_sitter.Node) (*tree_sitter.Node, string, bool) {
	for _, field := range []string{"function", "expression", "name"} {
		callee := node.ChildByFieldName(field)
		if callee == nil {
			continue
		}
		nameNode, name, ok := referenceName(source, callee)
		if ok {
			return nameNode, name, true
		}
	}
	return nil, "", false
}

func referenceName(source []byte, node *tree_sitter.Node) (*tree_sitter.Node, string, bool) {
	switch node.Kind() {
	case "identifier", "property_identifier", "field_identifier":
		name := strings.TrimSpace(nodeText(source, node))
		return node, name, name != ""
	case "attribute", "field_expression", "member_access_expression", "member_expression", "scoped_identifier", "selector_expression":
		for _, field := range []string{"name", "field", "attribute", "property"} {
			child := node.ChildByFieldName(field)
			if child == nil {
				continue
			}
			nameNode, name, ok := referenceName(source, child)
			if ok {
				return nameNode, name, true
			}
		}
		name := strings.TrimSpace(nodeText(source, node))
		if i := strings.LastIndexAny(name, ".:"); i >= 0 {
			name = strings.TrimSpace(name[i+1:])
		}
		if name == "" || strings.ContainsAny(name, " ()[]{}") {
			return nil, "", false
		}
		return node, name, true
	default:
		return nil, "", false
	}
}

func goCallReferenceName(source []byte, node *tree_sitter.Node) (string, bool) {
	switch node.Kind() {
	case "identifier":
		name := strings.TrimSpace(nodeText(source, node))
		return name, name != ""
	case "selector_expression":
		name := strings.TrimSpace(nodeText(source, node))
		if lastDot := strings.LastIndex(name, "."); lastDot >= 0 {
			name = name[lastDot+1:]
		}
		return name, name != ""
	default:
		return "", false
	}
}

func nodeText(source []byte, node *tree_sitter.Node) string {
	start := int(node.StartByte())
	end := int(node.EndByte())
	if start < 0 || end < start || end > len(source) {
		return ""
	}
	return string(source[start:end])
}
