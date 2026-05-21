package parser

import (
	"path/filepath"
	"sort"
	"strings"

	"repobridge/internal/astgraph/model"
)

const containmentProvenance = "parser:containment"

func appendContainmentGraph(path string, source []byte, language model.Language, result *ExtractionResult) {
	fileNode := ensureFileNode(path, source, language, result)
	moduleNode, hasModule := ensureModuleNode(path, source, language, result)
	if hasModule {
		appendContainmentEdge(result, fileNode, moduleNode, moduleNode.StartLine, moduleNode.StartColumn, path)
	}

	nodes := append([]model.GraphNode(nil), result.Nodes...)
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].StartLine == nodes[j].StartLine {
			return nodes[i].EndLine > nodes[j].EndLine
		}
		return nodes[i].StartLine < nodes[j].StartLine
	})

	for _, child := range nodes {
		if !needsContainmentParent(child, fileNode.ID, moduleNode.ID) {
			continue
		}
		parent := containmentParent(child, nodes, fileNode, moduleNode, hasModule)
		if parent.ID == "" {
			continue
		}
		appendContainmentEdge(result, parent, child, child.StartLine, child.StartColumn, path)
	}
}

func ensureFileNode(path string, source []byte, language model.Language, result *ExtractionResult) model.GraphNode {
	lineCount := sourceLineCount(source)
	id := stableNodeID(path, model.NodeKindFile, path, 1)
	if node, ok := nodeByStableID(result.Nodes, id); ok {
		return node
	}
	node := model.GraphNode{
		ID:            id,
		Kind:          model.NodeKindFile,
		Name:          path,
		QualifiedName: path,
		FilePath:      path,
		Language:      language,
		StartLine:     1,
		EndLine:       lineCount,
		Signature:     path,
	}
	result.Nodes = append(result.Nodes, node)
	return node
}

func ensureModuleNode(path string, source []byte, language model.Language, result *ExtractionResult) (model.GraphNode, bool) {
	name, line := moduleNameAndLine(path, source, language)
	if name == "" {
		return model.GraphNode{}, false
	}
	id := stableNodeID(path, model.NodeKindModule, name, line)
	if node, ok := nodeByStableID(result.Nodes, id); ok {
		return node, true
	}
	node := model.GraphNode{
		ID:            id,
		Kind:          model.NodeKindModule,
		Name:          name,
		QualifiedName: name,
		FilePath:      path,
		Language:      language,
		StartLine:     line,
		EndLine:       sourceLineCount(source),
		Signature:     moduleSignature(language, name),
	}
	result.Nodes = append(result.Nodes, node)
	return node, true
}

func moduleNameAndLine(path string, source []byte, language model.Language) (string, int) {
	lines := strings.Split(string(source), "\n")
	for index, raw := range lines {
		line := index + 1
		trimmed := strings.TrimSpace(stripLineComment(raw))
		if trimmed == "" {
			continue
		}
		switch language {
		case model.LanguageGo:
			if strings.HasPrefix(trimmed, "package ") {
				return strings.TrimSpace(strings.TrimPrefix(trimmed, "package ")), line
			}
		case model.LanguageJava, model.LanguageKotlin:
			if strings.HasPrefix(trimmed, "package ") {
				name := strings.TrimSpace(strings.TrimPrefix(trimmed, "package "))
				return strings.TrimSuffix(name, ";"), line
			}
		case model.LanguageCSharp:
			if strings.HasPrefix(trimmed, "namespace ") {
				name := strings.TrimSpace(strings.TrimPrefix(trimmed, "namespace "))
				name = strings.TrimSuffix(name, ";")
				name = strings.TrimSuffix(name, "{")
				return strings.TrimSpace(name), line
			}
		}
		break
	}

	if language == model.LanguagePython {
		name := strings.TrimSuffix(filepath.ToSlash(path), filepath.Ext(path))
		name = strings.Trim(strings.ReplaceAll(name, "/", "."), ".")
		if name != "" {
			return name, 1
		}
	}
	return "", 0
}

func moduleSignature(language model.Language, name string) string {
	switch language {
	case model.LanguageGo, model.LanguageJava, model.LanguageKotlin:
		return "package " + name
	case model.LanguageCSharp:
		return "namespace " + name
	default:
		return "module " + name
	}
}

func needsContainmentParent(node model.GraphNode, fileID, moduleID string) bool {
	if node.ID == "" || node.ID == fileID || node.ID == moduleID {
		return false
	}
	return node.Kind != model.NodeKindExternal
}

func containmentParent(child model.GraphNode, nodes []model.GraphNode, fileNode, moduleNode model.GraphNode, hasModule bool) model.GraphNode {
	if parent, ok := receiverContainmentParent(child, nodes); ok {
		return parent
	}
	if parent, ok := rangeContainmentParent(child, nodes); ok {
		return parent
	}
	if hasModule {
		return moduleNode
	}
	return fileNode
}

func receiverContainmentParent(child model.GraphNode, nodes []model.GraphNode) (model.GraphNode, bool) {
	receiver := cleanTypeName(child.ReceiverType)
	if receiver == "" {
		return model.GraphNode{}, false
	}
	for _, candidate := range nodes {
		if candidate.ID == child.ID || candidate.FilePath != child.FilePath || !isContainmentScope(candidate.Kind) {
			continue
		}
		if cleanTypeName(candidate.Name) == receiver || cleanTypeName(candidate.QualifiedName) == receiver {
			return candidate, true
		}
	}
	for _, candidate := range nodes {
		if candidate.ID == child.ID || candidate.FilePath != child.FilePath || !isContainmentScope(candidate.Kind) {
			continue
		}
		if strings.HasSuffix(candidate.QualifiedName, "."+receiver) || strings.HasSuffix(candidate.QualifiedName, "::"+receiver) {
			return candidate, true
		}
	}
	return model.GraphNode{}, false
}

func rangeContainmentParent(child model.GraphNode, nodes []model.GraphNode) (model.GraphNode, bool) {
	var best model.GraphNode
	bestSpan := 0
	for _, candidate := range nodes {
		if candidate.ID == child.ID || candidate.FilePath != child.FilePath || !isContainmentScope(candidate.Kind) {
			continue
		}
		if !containsRange(candidate, child) {
			continue
		}
		span := candidate.EndLine - candidate.StartLine
		if best.ID == "" || span < bestSpan || (span == bestSpan && candidate.StartLine > best.StartLine) {
			best = candidate
			bestSpan = span
		}
	}
	return best, best.ID != ""
}

func containsRange(parent, child model.GraphNode) bool {
	if parent.StartLine <= 0 || parent.EndLine <= 0 || child.StartLine <= 0 {
		return false
	}
	if parent.StartLine > child.StartLine || parent.EndLine < child.EndLine {
		return false
	}
	if parent.StartLine == child.StartLine && parent.EndLine == child.EndLine {
		return false
	}
	return parent.EndLine > parent.StartLine
}

func isContainmentScope(kind model.NodeKind) bool {
	switch kind {
	case model.NodeKindModule, model.NodeKindClass, model.NodeKindStruct, model.NodeKindInterface, model.NodeKindEnum, model.NodeKindTrait, model.NodeKindProtocol, model.NodeKindFunction, model.NodeKindMethod, model.NodeKindHandler:
		return true
	default:
		return false
	}
}

func appendContainmentEdge(result *ExtractionResult, parent, child model.GraphNode, line, column int, path string) {
	if parent.ID == "" || child.ID == "" || parent.ID == child.ID {
		return
	}
	for _, edge := range result.Edges {
		if edge.SourceNodeID == parent.ID && edge.TargetNodeID == child.ID && edge.Kind == model.EdgeKindContains {
			return
		}
	}
	result.Edges = append(result.Edges, model.GraphEdge{
		SourceNodeID: parent.ID,
		TargetNodeID: child.ID,
		Kind:         model.EdgeKindContains,
		FilePath:     path,
		Line:         line,
		Column:       column,
		Provenance:   containmentProvenance,
	})
}

func nodeByStableID(nodes []model.GraphNode, id string) (model.GraphNode, bool) {
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
	}
	return model.GraphNode{}, false
}

func sourceLineCount(source []byte) int {
	if len(source) == 0 {
		return 1
	}
	return strings.Count(string(source), "\n") + 1
}
