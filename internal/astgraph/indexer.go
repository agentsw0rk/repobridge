package astgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"repobridge/internal/astgraph/parser"
)

const SchemaVersion = 14

type IndexOptions struct {
	MaxFileSize int64
}

type Indexer struct {
	opts IndexOptions
}

func NewIndexer(opts IndexOptions) *Indexer {
	return &Indexer{opts: opts}
}

func (i *Indexer) Index(sourcePath string) (IndexResult, error) {
	startedAt := time.Now().UTC()
	result := IndexResult{
		SourcePath:    sourcePath,
		StartedAt:     startedAt,
		SchemaVersion: SchemaVersion,
	}

	files, err := parser.ScanSourceFiles(sourcePath, parser.Options{MaxFileSize: i.opts.MaxFileSize})
	if err != nil {
		result.CompletedAt = time.Now().UTC()
		return result, err
	}

	for _, file := range files {
		content, err := os.ReadFile(file.AbsolutePath)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: read failed: %v", file.RelativePath, err))
			continue
		}

		extracted, err := parser.ExtractFromSource(file.RelativePath, content, file.Language)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: extract failed: %v", file.RelativePath, err))
			continue
		}

		indexedAt := time.Now().UTC()
		hash := sha256.Sum256(content)
		result.Files = append(result.Files, GraphFile{
			Path:        file.RelativePath,
			Language:    file.Language,
			ContentHash: hex.EncodeToString(hash[:]),
			Size:        file.Size,
			ModifiedAt:  file.ModifiedAt,
			IndexedAt:   indexedAt,
			NodeCount:   len(extracted.Nodes),
		})
		result.Nodes = append(result.Nodes, extracted.Nodes...)
		result.Edges = append(result.Edges, extracted.Edges...)
		result.Unresolved = append(result.Unresolved, extracted.Unresolved...)
		result.Warnings = append(result.Warnings, extracted.Warnings...)
	}

	resolveCallEdges(&result)
	result.CompletedAt = time.Now().UTC()
	return result, nil
}

func resolveCallEdges(result *IndexResult) {
	if len(result.Nodes) == 0 || len(result.Unresolved) == 0 {
		return
	}
	resolvedEdges := make([]GraphEdge, 0, len(result.Unresolved))
	remaining := make([]UnresolvedReference, 0, len(result.Unresolved))
	edgeKeys := existingEdgeKeys(result.Edges)

	for _, ref := range result.Unresolved {
		if ref.ReferenceKind != EdgeKindCalls || strings.TrimSpace(ref.ReferenceName) == "" {
			remaining = append(remaining, ref)
			continue
		}
		from, ok := nodeByID(result.Nodes, ref.FromNodeID)
		if !ok {
			remaining = append(remaining, ref)
			continue
		}
		target, ok := resolveCallTarget(result.Nodes, from, ref)
		if !ok {
			remaining = append(remaining, ref)
			continue
		}
		edge := GraphEdge{
			SourceNodeID: from.ID,
			TargetNodeID: target.ID,
			Kind:         EdgeKindCalls,
			FilePath:     ref.FilePath,
			Line:         ref.Line,
			Column:       ref.Column,
			Provenance:   "call-resolver",
		}
		key := edgeKey(edge)
		if _, exists := edgeKeys[key]; exists {
			continue
		}
		edgeKeys[key] = struct{}{}
		resolvedEdges = append(resolvedEdges, edge)
	}

	result.Edges = append(result.Edges, resolvedEdges...)
	result.Unresolved = remaining
}

func nodeByID(nodes []GraphNode, id string) (GraphNode, bool) {
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
	}
	return GraphNode{}, false
}

func resolveCallTarget(nodes []GraphNode, from GraphNode, ref UnresolvedReference) (GraphNode, bool) {
	candidates := callTargetCandidates(nodes, from, ref)
	if len(candidates) == 0 {
		return GraphNode{}, false
	}
	bestScore := -1
	var best GraphNode
	ambiguous := false
	for _, candidate := range candidates {
		score := callTargetScore(nodes, from, ref, candidate)
		if score > bestScore {
			bestScore = score
			best = candidate
			ambiguous = false
			continue
		}
		if score == bestScore {
			ambiguous = true
		}
	}
	if ambiguous {
		return GraphNode{}, false
	}
	return best, true
}

func callTargetCandidates(nodes []GraphNode, from GraphNode, ref UnresolvedReference) []GraphNode {
	var candidates []GraphNode
	for _, node := range nodes {
		if !isCallableNode(node.Kind) || node.Language != from.Language {
			continue
		}
		if !callReferenceMatchesNode(ref.ReferenceName, node) {
			continue
		}
		if !callArgumentsMatchNode(ref, node) {
			continue
		}
		candidates = append(candidates, node)
	}
	return candidates
}

func callTargetScore(nodes []GraphNode, from GraphNode, ref UnresolvedReference, target GraphNode) int {
	score := 0
	score += importedTargetScore(nodes, from, ref, target)
	score += samePackageTargetScore(from, target)
	score += defaultImportedTargetScore(from, ref, target)
	if target.FilePath == from.FilePath {
		score += 100
	}
	if from.ReceiverType != "" && target.ReceiverType == from.ReceiverType {
		score += 80
	}
	if receiverMatchesTarget(ref.ReceiverText, target) {
		score += 60
	}
	if target.ParameterCount == ref.ArgumentCount {
		score += 20
	}
	if ref.ScopeNodeID != "" && ref.ScopeNodeID == from.ID {
		score += 5
	}
	return score
}

func defaultImportedTargetScore(from GraphNode, ref UnresolvedReference, target GraphNode) int {
	if from.Language != LanguageKotlin || target.Language != LanguageKotlin || strings.TrimSpace(ref.ReceiverText) != "" {
		return 0
	}
	if target.Name != strings.TrimSpace(ref.ReferenceName) {
		return 0
	}
	for _, packageName := range kotlinDefaultImportPackages {
		if target.QualifiedName == packageName+"."+target.Name {
			return 120
		}
	}
	return 0
}

var kotlinDefaultImportPackages = []string{
	"kotlin",
	"kotlin.annotation",
	"kotlin.collections",
	"kotlin.comparisons",
	"kotlin.io",
	"kotlin.ranges",
	"kotlin.sequences",
	"kotlin.text",
	"kotlin.jvm",
	"kotlin.jvm.functions",
}

func samePackageTargetScore(from GraphNode, target GraphNode) int {
	if from.Language != target.Language {
		return 0
	}
	fromPackage := packageNameOf(from)
	if fromPackage == "" || fromPackage != packageNameOf(target) {
		return 0
	}
	return 150
}

func packageNameOf(node GraphNode) string {
	qualifiedName := strings.TrimSpace(node.QualifiedName)
	name := strings.TrimSpace(node.Name)
	if qualifiedName == "" || name == "" {
		return ""
	}
	if receiverType := strings.TrimSpace(node.ReceiverType); receiverType != "" {
		suffix := "." + receiverType + "." + name
		if strings.HasSuffix(qualifiedName, suffix) {
			return strings.TrimSuffix(qualifiedName, suffix)
		}
	}
	suffix := "." + name
	if strings.HasSuffix(qualifiedName, suffix) {
		return strings.TrimSuffix(qualifiedName, suffix)
	}
	return ""
}

func importedTargetScore(nodes []GraphNode, from GraphNode, ref UnresolvedReference, target GraphNode) int {
	for _, node := range nodes {
		if node.Kind != NodeKindImport || node.FilePath != from.FilePath || node.Language != from.Language {
			continue
		}
		if importMatchesTarget(node, ref, target) {
			return 200
		}
	}
	return 0
}

func importMatchesTarget(importNode GraphNode, ref UnresolvedReference, target GraphNode) bool {
	importPath := strings.TrimSpace(importNode.QualifiedName)
	if importPath == "" {
		return false
	}
	if strings.HasSuffix(importPath, ".*") {
		prefix := strings.TrimSuffix(importPath, "*")
		return target.Name == ref.ReferenceName && strings.HasPrefix(target.QualifiedName, prefix)
	}
	return importNode.Name == ref.ReferenceName && target.QualifiedName == importPath
}

func isCallableNode(kind NodeKind) bool {
	switch kind {
	case NodeKindFunction, NodeKindMethod, NodeKindHandler, NodeKindClass:
		return true
	default:
		return false
	}
}

func callReferenceMatchesNode(reference string, node GraphNode) bool {
	reference = strings.TrimSpace(reference)
	return reference == node.Name ||
		reference == node.QualifiedName ||
		strings.HasSuffix(reference, "."+node.Name) ||
		strings.HasSuffix(reference, "::"+node.Name)
}

func callArgumentsMatchNode(ref UnresolvedReference, node GraphNode) bool {
	if node.Kind == NodeKindClass {
		return true
	}
	if isOptionalArityLanguage(node.Language) {
		return ref.ArgumentCount <= node.ParameterCount
	}
	return ref.ArgumentCount == node.ParameterCount
}

func isOptionalArityLanguage(language Language) bool {
	switch language {
	case LanguageJavaScript, LanguageTypeScript:
		return true
	default:
		return false
	}
}

func receiverMatchesTarget(receiver string, node GraphNode) bool {
	receiver = normalizeReceiver(receiver)
	if receiver == "" {
		return false
	}
	return receiver == normalizeReceiver(node.ReceiverType) ||
		receiver == normalizeReceiver(node.QualifiedName) ||
		receiver == normalizeReceiver(node.Name) ||
		strings.HasSuffix(normalizeReceiver(node.QualifiedName), "."+receiver)
}

func normalizeReceiver(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "*")
	value = strings.TrimPrefix(value, "&")
	if value == "this" || value == "self" {
		return ""
	}
	return strings.ToLower(value)
}

func existingEdgeKeys(edges []GraphEdge) map[string]struct{} {
	keys := make(map[string]struct{}, len(edges))
	for _, edge := range edges {
		keys[edgeKey(edge)] = struct{}{}
	}
	return keys
}

func edgeKey(edge GraphEdge) string {
	return strings.Join([]string{
		edge.SourceNodeID,
		edge.TargetNodeID,
		string(edge.Kind),
		edge.FilePath,
		fmt.Sprint(edge.Line),
		fmt.Sprint(edge.Column),
	}, "\x00")
}
