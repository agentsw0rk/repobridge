package astgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"repobridge/internal/astgraph/parser"
)

const SchemaVersion = 25

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
			if external, externalOK := externalCallTarget(result.Nodes, from, ref); externalOK {
				externalNode := ensureExternalNode(result, external, ref)
				edge := GraphEdge{
					SourceNodeID: from.ID,
					TargetNodeID: externalNode.ID,
					Kind:         EdgeKindCalls,
					FilePath:     ref.FilePath,
					Line:         ref.Line,
					Column:       ref.Column,
					Provenance:   external.provenance,
				}
				key := edgeKey(edge)
				if _, exists := edgeKeys[key]; exists {
					continue
				}
				edgeKeys[key] = struct{}{}
				resolvedEdges = append(resolvedEdges, edge)
				continue
			}
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
	result.Unresolved = filterRemainingUnresolved(result.Nodes, remaining)
}

type externalCall struct {
	name          string
	qualifiedName string
	receiver      string
	importPath    string
	provenance    string
}

func externalCallTarget(nodes []GraphNode, from GraphNode, ref UnresolvedReference) (externalCall, bool) {
	if ref.Language == LanguageRust && ref.ReferenceKind == EdgeKindCalls {
		return rustExternalCallTarget(nodes, from, ref)
	}
	if ref.Language == LanguageCSharp && ref.ReferenceKind == EdgeKindCalls {
		return csharpExternalCallTarget(nodes, from, ref)
	}
	if ref.Language != LanguageGo || ref.ReferenceKind != EdgeKindCalls {
		return externalCall{}, false
	}
	referenceName := strings.TrimSpace(ref.ReferenceName)
	if referenceName == "" {
		return externalCall{}, false
	}
	receiver := rootReceiver(ref.ReceiverText)
	if receiver == "" && isGoPredeclaredCall(referenceName) {
		return externalCall{
			name:          referenceName,
			qualifiedName: "external:builtin." + referenceName,
			receiver:      "builtin",
			importPath:    "builtin",
			provenance:    "go-builtin",
		}, true
	}
	if receiver == "" {
		return externalCall{}, false
	}
	if importPath, ok := goImportPathForReceiver(nodes, ref.FilePath, receiver); ok {
		return externalCall{
			name:          referenceName,
			qualifiedName: "external:" + receiver + "." + referenceName,
			receiver:      receiver,
			importPath:    importPath,
			provenance:    "go-external-import",
		}, true
	}
	if goExternalReceiverFallback(nodes, from, ref) {
		qualifiedReceiver := cleanExternalReceiver(ref.ReceiverText)
		if qualifiedReceiver == "" {
			qualifiedReceiver = receiver
		}
		return externalCall{
			name:          referenceName,
			qualifiedName: "external:" + qualifiedReceiver + "." + referenceName,
			receiver:      qualifiedReceiver,
			importPath:    "unknown",
			provenance:    "go-external-receiver",
		}, true
	}
	return externalCall{}, false
}

func csharpExternalCallTarget(nodes []GraphNode, from GraphNode, ref UnresolvedReference) (externalCall, bool) {
	referenceName := strings.TrimSpace(ref.ReferenceName)
	receiver := cleanExternalReceiver(ref.ReceiverText)
	if referenceName == "" {
		return externalCall{}, false
	}
	if receiver == "" {
		if csharpTestFrameworkCall(ref.FilePath, referenceName) {
			return externalCall{
				name:          referenceName,
				qualifiedName: "external:test-framework." + referenceName,
				receiver:      "test-framework",
				importPath:    "unknown",
				provenance:    "csharp-test-external",
			}, true
		}
		return externalCall{}, false
	}
	if primitiveReceiver := csharpPrimitiveExternalReceiver(nodes, from, ref, receiver); primitiveReceiver != "" {
		return externalCall{
			name:          referenceName,
			qualifiedName: "external:" + primitiveReceiver + "." + referenceName,
			receiver:      primitiveReceiver,
			importPath:    "unknown",
			provenance:    "csharp-primitive-external",
		}, true
	}
	if csharpLinqCall(referenceName) {
		return externalCall{
			name:          referenceName,
			qualifiedName: "external:System.Linq.Enumerable." + referenceName,
			receiver:      "System.Linq.Enumerable",
			importPath:    "unknown",
			provenance:    "csharp-linq-external",
		}, true
	}
	if csharpTestFrameworkCall(ref.FilePath, referenceName) {
		return externalCall{
			name:          referenceName,
			qualifiedName: "external:test-framework." + referenceName,
			receiver:      "test-framework",
			importPath:    "unknown",
			provenance:    "csharp-test-external",
		}, true
	}
	if !csharpExternalReceiver(receiver) {
		return externalCall{}, false
	}
	return externalCall{
		name:          referenceName,
		qualifiedName: "external:" + receiver + "." + referenceName,
		receiver:      receiver,
		importPath:    "unknown",
		provenance:    "csharp-external-receiver",
	}, true
}

func csharpExternalReceiver(receiver string) bool {
	root := rootReceiver(receiver)
	if _, ok := csharpKnownExternalReceivers[root]; ok {
		return true
	}
	if strings.Contains(receiver, ".") && startsWithUpper(root) {
		return true
	}
	return startsWithUpper(receiver)
}

func csharpPrimitiveExternalReceiver(nodes []GraphNode, from GraphNode, ref UnresolvedReference, receiver string) string {
	if csharpStringLiteral(receiver) {
		return "System.String"
	}
	if primitiveReceiver := csharpPrimitiveSystemType(receiver); primitiveReceiver != "" {
		return primitiveReceiver
	}
	receiverType := inferCSharpReceiverType(nodes, from, ref)
	if receiverType == "" {
		return ""
	}
	return csharpPrimitiveSystemType(receiverType)
}

func csharpStringLiteral(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "\"") ||
		strings.HasPrefix(value, "@\"") ||
		strings.HasPrefix(value, "$\"") ||
		strings.HasPrefix(value, "$@\"") ||
		strings.HasPrefix(value, "@$\"")
}

func csharpPrimitiveSystemType(typ string) string {
	switch normalizeCSharpType(typ) {
	case "string":
		return "System.String"
	case "int":
		return "System.Int32"
	case "long":
		return "System.Int64"
	case "short":
		return "System.Int16"
	case "byte":
		return "System.Byte"
	case "bool":
		return "System.Boolean"
	case "char":
		return "System.Char"
	case "double":
		return "System.Double"
	case "float":
		return "System.Single"
	case "decimal":
		return "System.Decimal"
	default:
		return ""
	}
}

func csharpTestFrameworkCall(filePath, referenceName string) bool {
	if !csharpTestFile(filePath) {
		return false
	}
	_, ok := csharpTestFrameworkCalls[referenceName]
	return ok
}

func csharpLinqCall(referenceName string) bool {
	_, ok := csharpLinqCalls[referenceName]
	return ok
}

func csharpTestFile(filePath string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(filePath, "\\", "/"))
	if strings.Contains(normalized, "/test/") ||
		strings.Contains(normalized, "/tests/") ||
		strings.Contains(normalized, ".test/") ||
		strings.Contains(normalized, ".tests/") {
		return true
	}
	base := strings.TrimSuffix(strings.ToLower(path.Base(normalized)), ".cs")
	return strings.HasSuffix(base, "test") || strings.HasSuffix(base, "tests")
}

var csharpKnownExternalReceivers = map[string]struct{}{
	"Activator":      {},
	"Array":          {},
	"Assert":         {},
	"AssertEx":       {},
	"Console":        {},
	"Convert":        {},
	"Directory":      {},
	"Encoding":       {},
	"Enumerable":     {},
	"Environment":    {},
	"File":           {},
	"GC":             {},
	"JsonConvert":    {},
	"JsonSerializer": {},
	"Math":           {},
	"Path":           {},
	"Regex":          {},
	"String":         {},
	"StringComparer": {},
	"Task":           {},
}

var csharpTestFrameworkCalls = map[string]struct{}{
	"AreEqual": {},
	"Equal":    {},
	"IsTrue":   {},
	"OfType":   {},
	"Returns":  {},
	"Single":   {},
	"Throws":   {},
	"True":     {},
}

var csharpLinqCalls = map[string]struct{}{
	"OfType":            {},
	"Select":            {},
	"Single":            {},
	"ToAsyncEnumerable": {},
	"ToList":            {},
}

func startsWithUpper(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(value)
	return first != utf8.RuneError && unicode.IsUpper(first)
}

func rustExternalCallTarget(nodes []GraphNode, from GraphNode, ref UnresolvedReference) (externalCall, bool) {
	referenceName := strings.TrimSpace(ref.ReferenceName)
	if referenceName == "" {
		return externalCall{}, false
	}
	receiver := cleanExternalReceiver(ref.ReceiverText)
	if importPath, ok := rustImportPathForCall(nodes, ref.FilePath, receiver, referenceName); ok {
		return externalCall{
			name:          referenceName,
			qualifiedName: "external:" + importPath,
			receiver:      rustExternalReceiver(importPath),
			importPath:    importPath,
			provenance:    "rust-external-import",
		}, true
	}
	if receiver == "" && isRustPreludeCall(referenceName) {
		return externalCall{
			name:          referenceName,
			qualifiedName: "external:rust.prelude." + referenceName,
			receiver:      "rust.prelude",
			importPath:    "std::prelude",
			provenance:    "rust-prelude",
		}, true
	}
	if receiver == "" || len(callTargetCandidates(nodes, from, ref)) > 0 {
		return externalCall{}, false
	}
	separator := "."
	if rustTypeLikeReceiver(receiver) {
		separator = "::"
	}
	return externalCall{
		name:          referenceName,
		qualifiedName: "external:" + receiver + separator + referenceName,
		receiver:      receiver,
		importPath:    "unknown",
		provenance:    "rust-external-receiver",
	}, true
}

func rustImportPathForCall(nodes []GraphNode, filePath, receiver, referenceName string) (string, bool) {
	receiver = strings.TrimSpace(receiver)
	referenceName = strings.TrimSpace(referenceName)
	if referenceName == "" {
		return "", false
	}
	if receiver != "" {
		if importPath, ok := rustImportPathForName(nodes, filePath, receiver); ok {
			return importPath + "::" + referenceName, true
		}
		return "", false
	}
	return rustImportPathForName(nodes, filePath, referenceName)
}

func rustImportPathForName(nodes []GraphNode, filePath, name string) (string, bool) {
	for _, node := range nodes {
		if node.Kind != NodeKindImport || node.Language != LanguageRust || node.FilePath != filePath {
			continue
		}
		if node.Name == name {
			return node.QualifiedName, true
		}
	}
	for _, node := range nodes {
		if node.Kind != NodeKindImport || node.Language != LanguageRust || node.FilePath != filePath || node.Name != "*" {
			continue
		}
		prefix := strings.TrimSuffix(node.QualifiedName, "::*")
		if prefix != "" {
			return prefix + "::" + name, true
		}
	}
	return "", false
}

func rustExternalReceiver(importPath string) string {
	if before, _, ok := strings.Cut(importPath, "::"); ok {
		return before
	}
	return importPath
}

func rustTypeLikeReceiver(receiver string) bool {
	receiver = strings.TrimSpace(receiver)
	if receiver == "" || strings.ContainsAny(receiver, ".()[]{}") {
		return false
	}
	if strings.Contains(receiver, "::") {
		return true
	}
	first := receiver[0]
	return first >= 'A' && first <= 'Z'
}

func isRustPreludeCall(name string) bool {
	_, ok := rustPreludeCalls[strings.TrimSpace(name)]
	return ok
}

var rustPreludeCalls = map[string]struct{}{
	"Some": {},
	"None": {},
	"Ok":   {},
	"Err":  {},
	"Box":  {},
	"Vec":  {},
	"Rc":   {},
	"Arc":  {},
}

func goExternalReceiverFallback(nodes []GraphNode, from GraphNode, ref UnresolvedReference) bool {
	receiverText := strings.TrimSpace(ref.ReceiverText)
	if receiverText == "" {
		return false
	}
	if strings.Contains(receiverText, ".") {
		return true
	}
	if inferred := inferGoReceiverType(nodes, from, ref, receiverText); inferred != "" && !strings.HasPrefix(inferred, "external:") {
		return false
	}
	return len(callTargetCandidates(nodes, from, ref)) == 0
}

func cleanExternalReceiver(receiver string) string {
	receiver = strings.TrimSpace(receiver)
	receiver = strings.Trim(receiver, "()")
	receiver = strings.TrimSpace(receiver)
	return receiver
}

func ensureExternalNode(result *IndexResult, external externalCall, ref UnresolvedReference) GraphNode {
	id := stableExternalNodeID(ref.FilePath, external.qualifiedName)
	for _, node := range result.Nodes {
		if node.ID == id {
			return node
		}
	}
	node := GraphNode{
		ID:            id,
		Kind:          NodeKindExternal,
		Name:          external.name,
		QualifiedName: external.qualifiedName,
		ReceiverType:  external.receiver,
		ReturnType:    external.importPath,
		FilePath:      ref.FilePath,
		Language:      ref.Language,
		StartLine:     ref.Line,
		EndLine:       ref.Line,
		StartColumn:   ref.Column,
		EndColumn:     ref.Column,
		Signature:     external.provenance + " " + external.qualifiedName,
	}
	result.Nodes = append(result.Nodes, node)
	return node
}

func stableExternalNodeID(path, qualifiedName string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("external:%s:%s", path, qualifiedName)))
	return hex.EncodeToString(hash[:20])
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
	if target, ok := resolveGoFunctionVariableTarget(nodes, from, ref); ok {
		return target, true
	}
	csharpReceiverType := csharpReceiverTypeForCall(nodes, from, ref)
	candidates := callTargetCandidatesWithCSharpReceiver(nodes, from, ref, csharpReceiverType)
	if len(candidates) == 0 {
		return GraphNode{}, false
	}
	bestScore := -1
	var best GraphNode
	ambiguous := false
	for _, candidate := range candidates {
		score := callTargetScore(nodes, from, ref, candidate, csharpReceiverType)
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
	return callTargetCandidatesWithCSharpReceiver(nodes, from, ref, csharpReceiverTypeForCall(nodes, from, ref))
}

func callTargetCandidatesWithCSharpReceiver(nodes []GraphNode, from GraphNode, ref UnresolvedReference, csharpReceiverType string) []GraphNode {
	var candidates []GraphNode
	for _, node := range nodes {
		if !isCallableNode(node.Kind) || node.Language != from.Language {
			continue
		}
		if !callReferenceMatchesNode(ref.ReferenceName, node) {
			continue
		}
		if !callReceiverCanMatchNode(ref, node, csharpReceiverType) {
			continue
		}
		if !callArgumentsMatchNode(ref, node) {
			continue
		}
		candidates = append(candidates, node)
	}
	return candidates
}

func callReceiverCanMatchNode(ref UnresolvedReference, node GraphNode, csharpReceiverType string) bool {
	if ref.Language != LanguageCSharp || strings.TrimSpace(ref.ReceiverText) == "" {
		return true
	}
	if normalizeReceiver(ref.ReceiverText) == "" {
		return true
	}
	if csharpReceiverType != "" && csharpReceiverTypeMatches(csharpReceiverType, node) {
		return true
	}
	return receiverMatchesTarget(ref.ReceiverText, node)
}

func filterRemainingUnresolved(nodes []GraphNode, refs []UnresolvedReference) []UnresolvedReference {
	if len(refs) == 0 {
		return refs
	}
	filtered := make([]UnresolvedReference, 0, len(refs))
	for _, ref := range refs {
		if shouldKeepUnresolved(nodes, ref) {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

func shouldKeepUnresolved(nodes []GraphNode, ref UnresolvedReference) bool {
	if ref.Language == LanguageRust && ref.ReferenceKind == EdgeKindCalls {
		if len(callTargetCandidates(nodes, GraphNode{Language: ref.Language}, ref)) > 0 {
			return true
		}
		if _, ok := externalCallTarget(nodes, GraphNode{Language: ref.Language}, ref); ok {
			return false
		}
		return true
	}
	if ref.Language != LanguageGo || ref.ReferenceKind != EdgeKindCalls {
		return true
	}
	if len(callTargetCandidates(nodes, GraphNode{Language: ref.Language}, ref)) > 0 {
		return true
	}
	if strings.TrimSpace(ref.ReceiverText) == "" && isGoPredeclaredCall(ref.ReferenceName) {
		return false
	}
	if goImportReceiver(nodes, ref) {
		return false
	}
	if _, ok := externalCallTarget(nodes, GraphNode{Language: ref.Language}, ref); ok {
		return false
	}
	return true
}

func goImportReceiver(nodes []GraphNode, ref UnresolvedReference) bool {
	_, ok := goImportPathForReceiver(nodes, ref.FilePath, rootReceiver(ref.ReceiverText))
	return ok
}

func goImportPathForReceiver(nodes []GraphNode, filePath, receiver string) (string, bool) {
	receiver = strings.TrimSpace(receiver)
	if receiver == "" {
		return "", false
	}
	for _, node := range nodes {
		if node.Kind == NodeKindImport && node.Language == LanguageGo && node.FilePath == filePath && node.Name == receiver {
			return node.QualifiedName, true
		}
	}
	return "", false
}

func rootReceiver(receiver string) string {
	receiver = strings.TrimSpace(receiver)
	if receiver == "" {
		return ""
	}
	if before, _, ok := strings.Cut(receiver, "."); ok {
		return strings.TrimSpace(before)
	}
	return receiver
}

func isGoPredeclaredCall(name string) bool {
	_, ok := goPredeclaredCalls[strings.TrimSpace(name)]
	return ok
}

var goPredeclaredCalls = map[string]struct{}{
	"append":     {},
	"bool":       {},
	"byte":       {},
	"cap":        {},
	"clear":      {},
	"close":      {},
	"comparable": {},
	"complex":    {},
	"complex64":  {},
	"complex128": {},
	"copy":       {},
	"delete":     {},
	"error":      {},
	"float32":    {},
	"float64":    {},
	"imag":       {},
	"int":        {},
	"int8":       {},
	"int16":      {},
	"int32":      {},
	"int64":      {},
	"len":        {},
	"make":       {},
	"new":        {},
	"panic":      {},
	"print":      {},
	"println":    {},
	"real":       {},
	"recover":    {},
	"rune":       {},
	"string":     {},
	"uint":       {},
	"uint8":      {},
	"uint16":     {},
	"uint32":     {},
	"uint64":     {},
	"uintptr":    {},
}

func callTargetScore(nodes []GraphNode, from GraphNode, ref UnresolvedReference, target GraphNode, csharpReceiverType string) int {
	score := 0
	score += importedTargetScore(nodes, from, ref, target)
	score += samePackageTargetScore(from, target)
	score += defaultImportedTargetScore(from, ref, target)
	score += goInferredReceiverTargetScore(nodes, from, ref, target)
	score += rustInferredReceiverTargetScore(from, ref, target)
	score += csharpInferredReceiverTargetScore(from, ref, target, csharpReceiverType)
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

func goInferredReceiverTargetScore(nodes []GraphNode, from GraphNode, ref UnresolvedReference, target GraphNode) int {
	if from.Language != LanguageGo || target.Language != LanguageGo || strings.TrimSpace(ref.ReceiverText) == "" {
		return 0
	}
	receiverType := inferGoReceiverType(nodes, from, ref, ref.ReceiverText)
	if receiverType == "" {
		return 0
	}
	if normalizeReceiver(receiverType) == normalizeReceiver(target.ReceiverType) {
		return 180
	}
	return 0
}

func rustInferredReceiverTargetScore(from GraphNode, ref UnresolvedReference, target GraphNode) int {
	if from.Language != LanguageRust || target.Language != LanguageRust || strings.TrimSpace(ref.ReceiverText) == "" {
		return 0
	}
	receiverType := inferRustReceiverType(from, ref)
	if receiverType == "" {
		return 0
	}
	if normalizeRustType(receiverType) == normalizeRustType(target.ReceiverType) {
		return 180
	}
	return 0
}

func csharpInferredReceiverTargetScore(from GraphNode, ref UnresolvedReference, target GraphNode, csharpReceiverType string) int {
	if from.Language != LanguageCSharp || target.Language != LanguageCSharp || strings.TrimSpace(ref.ReceiverText) == "" {
		return 0
	}
	if csharpReceiverType == "" || !csharpReceiverTypeMatches(csharpReceiverType, target) {
		return 0
	}
	return 180
}

func csharpReceiverTypeForCall(nodes []GraphNode, from GraphNode, ref UnresolvedReference) string {
	if from.Language != LanguageCSharp || ref.Language != LanguageCSharp || strings.TrimSpace(ref.ReceiverText) == "" {
		return ""
	}
	return inferCSharpReceiverType(nodes, from, ref)
}

func inferCSharpReceiverType(nodes []GraphNode, from GraphNode, ref UnresolvedReference) string {
	receiver := strings.TrimSpace(ref.ReceiverText)
	if receiver == "" {
		return ""
	}
	if strings.ContainsAny(receiver, ".:()[]{}") {
		return inferCSharpReceiverExpressionType(nodes, from, ref, receiver)
	}
	if receiver == "this" {
		return normalizeCSharpType(from.ReceiverType)
	}
	if receiver == "base" {
		return inferCSharpBaseType(nodes, from)
	}
	for _, parameter := range csharpSignatureParameters(from.Signature, from.Name) {
		name, typ, ok := splitCSharpParameterNameType(parameter)
		if ok && name == receiver {
			return normalizeCSharpType(typ)
		}
	}
	if typ := inferCSharpLocalNewVariableType(from.Signature, receiver); typ != "" {
		return normalizeCSharpType(typ)
	}
	if typ := inferCSharpLocalFactoryVariableType(nodes, from, ref, receiver); typ != "" {
		return normalizeCSharpType(typ)
	}
	if typ := inferCSharpMemberType(nodes, from, receiver); typ != "" {
		return normalizeCSharpType(typ)
	}
	return ""
}

func csharpReceiverTypeMatches(receiverType string, node GraphNode) bool {
	receiverType = normalizeCSharpType(receiverType)
	if receiverType == "" {
		return false
	}
	return receiverType == normalizeCSharpType(node.ReceiverType) ||
		receiverType == normalizeCSharpType(node.Name) ||
		receiverType == normalizeCSharpType(node.QualifiedName)
}

func csharpSignatureParameters(signature, functionName string) []string {
	signature = strings.TrimSpace(signature)
	functionName = strings.TrimSpace(functionName)
	if signature == "" || functionName == "" {
		return nil
	}
	nameIndex := strings.Index(signature, functionName)
	if nameIndex < 0 {
		return nil
	}
	open := strings.Index(signature[nameIndex+len(functionName):], "(")
	if open < 0 {
		return nil
	}
	open += nameIndex + len(functionName)
	close := matchingCloseParen(signature, open)
	if close < 0 {
		return nil
	}
	return splitTopLevelList(signature[open+1:close], ',')
}

func splitCSharpParameterNameType(parameter string) (string, string, bool) {
	parameter = strings.TrimSpace(stripCSharpAttributes(parameter))
	if parameter == "" {
		return "", "", false
	}
	if i := strings.Index(parameter, "="); i >= 0 {
		parameter = strings.TrimSpace(parameter[:i])
	}
	fields := strings.Fields(parameter)
	for len(fields) > 0 && isCSharpParameterModifier(fields[0]) {
		fields = fields[1:]
	}
	if len(fields) < 2 {
		return "", "", false
	}
	name := strings.Trim(fields[len(fields)-1], "?")
	typ := strings.Join(fields[:len(fields)-1], " ")
	if name == "" || typ == "" {
		return "", "", false
	}
	return name, typ, true
}

func stripCSharpAttributes(parameter string) string {
	parameter = strings.TrimSpace(parameter)
	for strings.HasPrefix(parameter, "[") {
		close := strings.Index(parameter, "]")
		if close < 0 {
			return parameter
		}
		parameter = strings.TrimSpace(parameter[close+1:])
	}
	return parameter
}

func isCSharpParameterModifier(field string) bool {
	switch strings.TrimSpace(field) {
	case "this", "ref", "out", "in", "params":
		return true
	default:
		return false
	}
}

var csharpLocalNewRE = regexp.MustCompile(`(?:^|[;\n\r])\s*(?:var|[A-Za-z_][A-Za-z0-9_.<>, ?\[\]]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*new\s+([A-Za-z_][A-Za-z0-9_.<>]*)\s*[\(\{]`)
var csharpLocalCallAssignRE = regexp.MustCompile(`(?:^|[;\n\r])\s*var\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*([^;\n\r]+?\))\s*;`)
var csharpFieldRE = regexp.MustCompile(`(?:^|[;\n\r{}])\s*([A-Za-z_][A-Za-z0-9_.<>, ?\[\]]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:=[^;\n\r{}]*)?;`)
var csharpPropertyRE = regexp.MustCompile(`(?:^|[;\n\r{}])\s*([A-Za-z_][A-Za-z0-9_.<>, ?\[\]]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)

func inferCSharpLocalNewVariableType(signature, receiver string) string {
	for _, match := range csharpLocalNewRE.FindAllStringSubmatch(signature, -1) {
		if len(match) >= 3 && match[1] == receiver {
			return match[2]
		}
	}
	return ""
}

func inferCSharpLocalFactoryVariableType(nodes []GraphNode, from GraphNode, ref UnresolvedReference, receiver string) string {
	for _, match := range csharpLocalCallAssignRE.FindAllStringSubmatch(from.Signature, -1) {
		if len(match) < 3 || match[1] != receiver {
			continue
		}
		if typ := inferCSharpCallExpressionReturnType(nodes, from, ref, match[2]); typ != "" {
			return typ
		}
	}
	return ""
}

func inferCSharpReceiverExpressionType(nodes []GraphNode, from GraphNode, ref UnresolvedReference, receiverExpression string) string {
	receiverExpression = strings.TrimSpace(receiverExpression)
	if typ := inferCSharpCallExpressionReturnType(nodes, from, ref, receiverExpression); typ != "" {
		return typ
	}
	return ""
}

func inferCSharpCallExpressionReturnType(nodes []GraphNode, from GraphNode, ref UnresolvedReference, expression string) string {
	prefix, method, argCount, ok := splitCSharpReceiverCall(expression)
	if !ok {
		return ""
	}
	methodRef := UnresolvedReference{
		FromNodeID:    ref.FromNodeID,
		ReferenceName: method,
		ReceiverText:  prefix,
		ArgumentCount: argCount,
		ScopeNodeID:   ref.ScopeNodeID,
		ReferenceKind: EdgeKindCalls,
		FilePath:      ref.FilePath,
		Language:      ref.Language,
		Line:          ref.Line,
		Column:        ref.Column,
	}
	target, ok := resolveCallTarget(nodes, from, methodRef)
	if !ok || target.ReturnType == "" {
		return ""
	}
	return normalizeCSharpType(target.ReturnType)
}

func splitCSharpReceiverCall(receiver string) (string, string, int, bool) {
	receiver = strings.TrimSpace(stripCSharpOuterParens(receiver))
	if !strings.HasSuffix(receiver, ")") {
		return "", "", 0, false
	}
	open := matchingOpenParen(receiver, len(receiver)-1)
	if open <= 0 {
		return "", "", 0, false
	}
	callee := strings.TrimSpace(receiver[:open])
	dot := strings.LastIndex(callee, ".")
	if dot < 0 {
		return "", callee, countCSharpArguments(receiver[open+1 : len(receiver)-1]), callee != ""
	}
	if dot+1 >= len(callee) {
		return "", "", 0, false
	}
	prefix := strings.TrimSpace(callee[:dot])
	method := strings.TrimSpace(callee[dot+1:])
	if method == "" {
		return "", "", 0, false
	}
	return prefix, method, countCSharpArguments(receiver[open+1 : len(receiver)-1]), true
}

func stripCSharpOuterParens(text string) string {
	return stripGoOuterParens(text)
}

func countCSharpArguments(arguments string) int {
	return len(splitTopLevelList(arguments, ','))
}

func inferCSharpMemberType(nodes []GraphNode, from GraphNode, receiver string) string {
	classNode, ok := csharpClassNode(nodes, from)
	if !ok {
		return ""
	}
	for _, match := range csharpFieldRE.FindAllStringSubmatch(classNode.Signature, -1) {
		if len(match) >= 3 && match[2] == receiver && !csharpMemberTypeLooksLikeStatement(match[1]) {
			return match[1]
		}
	}
	for _, match := range csharpPropertyRE.FindAllStringSubmatch(classNode.Signature, -1) {
		if len(match) >= 3 && match[2] == receiver && !csharpMemberTypeLooksLikeStatement(match[1]) {
			return match[1]
		}
	}
	return ""
}

func inferCSharpBaseType(nodes []GraphNode, from GraphNode) string {
	pattern := regexp.MustCompile(`\bclass\s+` + regexp.QuoteMeta(from.ReceiverType) + `(?:\s*<[^>{}]*>)?\s*:\s*([A-Za-z_][A-Za-z0-9_.]*)`)
	for _, classNode := range csharpClassNodes(nodes, from) {
		match := pattern.FindStringSubmatch(classNode.Signature)
		if len(match) >= 2 {
			return normalizeCSharpType(match[1])
		}
	}
	return ""
}

func csharpClassNode(nodes []GraphNode, from GraphNode) (GraphNode, bool) {
	for _, node := range csharpClassNodes(nodes, from) {
		return node, true
	}
	return GraphNode{}, false
}

func csharpClassNodes(nodes []GraphNode, from GraphNode) []GraphNode {
	var sameFile []GraphNode
	var otherFiles []GraphNode
	for _, node := range nodes {
		if node.Kind != NodeKindClass || node.Language != LanguageCSharp || node.Name != from.ReceiverType {
			continue
		}
		if node.FilePath == from.FilePath {
			sameFile = append(sameFile, node)
		} else {
			otherFiles = append(otherFiles, node)
		}
	}
	return append(sameFile, otherFiles...)
}

func csharpMemberTypeLooksLikeStatement(typ string) bool {
	fields := strings.Fields(strings.TrimSpace(typ))
	if len(fields) == 0 {
		return true
	}
	switch fields[0] {
	case "return", "if", "for", "foreach", "while", "switch", "using", "var", "new":
		return true
	default:
		return false
	}
}

func normalizeCSharpType(typ string) string {
	typ = strings.TrimSpace(typ)
	for strings.HasPrefix(typ, "@") {
		typ = strings.TrimPrefix(typ, "@")
	}
	for strings.HasSuffix(typ, "?") {
		typ = strings.TrimSuffix(typ, "?")
	}
	typ = strings.TrimSpace(typ)
	for strings.HasPrefix(typ, "global::") {
		typ = strings.TrimPrefix(typ, "global::")
	}
	if i := strings.Index(typ, "<"); i >= 0 {
		typ = strings.TrimSpace(typ[:i])
	}
	if i := strings.LastIndex(typ, "."); i >= 0 {
		typ = strings.TrimSpace(typ[i+1:])
	}
	return strings.ToLower(strings.Trim(typ, "[]"))
}

func inferRustReceiverType(from GraphNode, ref UnresolvedReference) string {
	receiver := strings.TrimSpace(ref.ReceiverText)
	if receiver == "" || strings.ContainsAny(receiver, ".:()[]{}") {
		return ""
	}
	for _, parameter := range rustSignatureParameters(from.Signature, from.Name) {
		name, typ, ok := splitRustParameterNameType(parameter)
		if ok && name == receiver {
			return normalizeRustType(typ)
		}
	}
	return ""
}

func rustSignatureParameters(signature, functionName string) []string {
	signature = strings.TrimSpace(signature)
	functionName = strings.TrimSpace(functionName)
	if signature == "" || functionName == "" {
		return nil
	}
	nameIndex := strings.Index(signature, functionName)
	if nameIndex < 0 {
		return nil
	}
	open := strings.Index(signature[nameIndex+len(functionName):], "(")
	if open < 0 {
		return nil
	}
	open += nameIndex + len(functionName)
	close := matchingCloseParen(signature, open)
	if close < 0 {
		return nil
	}
	return splitTopLevelList(signature[open+1:close], ',')
}

func matchingCloseParen(text string, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitRustParameterNameType(parameter string) (string, string, bool) {
	parameter = strings.TrimSpace(parameter)
	if parameter == "" {
		return "", "", false
	}
	depth := 0
	for i, r := range parameter {
		switch r {
		case '(', '[', '<':
			depth++
		case ')', ']', '>':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				name := strings.TrimSpace(parameter[:i])
				typ := strings.TrimSpace(parameter[i+1:])
				name = strings.TrimPrefix(name, "mut ")
				name = strings.TrimSpace(name)
				if name == "" || typ == "" {
					return "", "", false
				}
				return name, typ, true
			}
		}
	}
	return "", "", false
}

func normalizeRustType(typ string) string {
	typ = strings.TrimSpace(typ)
	for strings.HasPrefix(typ, "&") || strings.HasPrefix(typ, "*") {
		typ = strings.TrimPrefix(typ, "&")
		typ = strings.TrimPrefix(typ, "*")
		typ = strings.TrimSpace(typ)
	}
	for {
		fields := strings.Fields(typ)
		if len(fields) == 0 {
			return ""
		}
		if fields[0] == "mut" || strings.HasPrefix(fields[0], "'") {
			typ = strings.TrimSpace(strings.Join(fields[1:], " "))
			continue
		}
		break
	}
	if i := strings.Index(typ, "<"); i >= 0 {
		typ = strings.TrimSpace(typ[:i])
	}
	return strings.Trim(strings.ToLower(typ), "()")
}

func splitTopLevelList(text string, delimiter rune) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var parts []string
	start := 0
	depth := 0
	for i, r := range text {
		switch r {
		case '(', '[', '<':
			depth++
		case ')', ']', '>':
			if depth > 0 {
				depth--
			}
		default:
			if r == delimiter && depth == 0 {
				parts = append(parts, text[start:i])
				start = i + len(string(r))
			}
		}
	}
	parts = append(parts, text[start:])
	return parts
}

func inferGoReceiverType(nodes []GraphNode, from GraphNode, ref UnresolvedReference, receiverText string) string {
	receiverText = strings.TrimSpace(receiverText)
	if receiverText == "" {
		return ""
	}
	if prefix, method, argCount, ok := splitGoReceiverCall(receiverText); ok {
		methodRef := UnresolvedReference{
			FromNodeID:    ref.FromNodeID,
			ReferenceName: method,
			ReceiverText:  prefix,
			ArgumentCount: argCount,
			ScopeNodeID:   ref.ScopeNodeID,
			ReferenceKind: EdgeKindCalls,
			FilePath:      ref.FilePath,
			Language:      ref.Language,
			Line:          ref.Line,
			Column:        ref.Column,
		}
		if target, ok := resolveCallTarget(nodes, from, methodRef); ok {
			return normalizeGoGraphType(target.ReturnType)
		}
		return ""
	}
	receiver := rootReceiver(receiverText)
	if receiver == "" {
		return ""
	}
	for _, node := range nodes {
		if node.Kind != NodeKindVariable || node.Language != LanguageGo || node.Name != receiver || !goAliasNodeVisibleFrom(node, from, ref) {
			continue
		}
		if typ := resolveGoAliasType(nodes, node, ref.FilePath); typ != "" {
			return typ
		}
	}
	return ""
}

func splitGoReceiverCall(receiver string) (string, string, int, bool) {
	receiver = strings.TrimSpace(receiver)
	receiver = stripGoOuterParens(receiver)
	if !strings.HasSuffix(receiver, ")") {
		return "", "", 0, false
	}
	open := matchingOpenParen(receiver, len(receiver)-1)
	if open <= 0 {
		return "", "", 0, false
	}
	callee := strings.TrimSpace(receiver[:open])
	dot := strings.LastIndex(callee, ".")
	if dot <= 0 || dot+1 >= len(callee) {
		return "", "", 0, false
	}
	prefix := strings.TrimSpace(callee[:dot])
	method := strings.TrimSpace(callee[dot+1:])
	if prefix == "" || method == "" {
		return "", "", 0, false
	}
	return prefix, method, countGoArguments(receiver[open+1 : len(receiver)-1]), true
}

func stripGoOuterParens(text string) string {
	text = strings.TrimSpace(text)
	for strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
		close := matchingOpenParen(text, len(text)-1)
		if close != 0 {
			break
		}
		text = strings.TrimSpace(text[1 : len(text)-1])
	}
	return text
}

func matchingOpenParen(text string, close int) int {
	depth := 0
	for i := close; i >= 0; i-- {
		switch text[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func countGoArguments(arguments string) int {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return 0
	}
	count := 1
	depth := 0
	for _, r := range arguments {
		switch r {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				count++
			}
		}
	}
	return count
}

func resolveGoFunctionVariableTarget(nodes []GraphNode, from GraphNode, ref UnresolvedReference) (GraphNode, bool) {
	if from.Language != LanguageGo || ref.Language != LanguageGo || strings.TrimSpace(ref.ReceiverText) != "" {
		return GraphNode{}, false
	}
	var targets []GraphNode
	for _, variable := range nodes {
		if variable.Kind != NodeKindVariable || variable.Language != LanguageGo || variable.Name != ref.ReferenceName || !goAliasNodeVisibleFrom(variable, from, ref) {
			continue
		}
		targetName := resolveGoAliasType(nodes, variable, ref.FilePath)
		if strings.HasPrefix(targetName, "return:") || targetName == "" {
			continue
		}
		for _, node := range nodes {
			if !isCallableNode(node.Kind) || node.Language != LanguageGo || node.Name != targetName {
				continue
			}
			if !callArgumentsMatchNode(ref, node) {
				continue
			}
			targets = append(targets, node)
		}
	}
	if len(targets) != 1 {
		return GraphNode{}, false
	}
	return targets[0], true
}

func goAliasNodeVisibleFrom(node GraphNode, from GraphNode, ref UnresolvedReference) bool {
	if node.FilePath == ref.FilePath {
		if node.StartLine > 0 && ref.Line > 0 && node.StartLine > ref.Line {
			return false
		}
		if from.ID != "" && node.StartLine > 0 && from.StartLine > 0 && from.EndLine > 0 {
			inSameFunction := node.StartLine >= from.StartLine && node.StartLine <= from.EndLine
			isPackageVariable := node.StartLine < from.StartLine && !strings.Contains(node.Signature, ":=")
			return inSameFunction || isPackageVariable
		}
		return true
	}
	return !strings.Contains(node.Signature, ":=") && path.Dir(node.FilePath) == path.Dir(ref.FilePath)
}

func resolveGoAliasType(nodes []GraphNode, alias GraphNode, filePath string) string {
	seen := map[string]struct{}{}
	typ := strings.TrimSpace(alias.ReceiverType)
	for typ != "" {
		key := alias.FilePath + "\x00" + alias.Name + "\x00" + typ
		if _, ok := seen[key]; ok {
			return typ
		}
		seen[key] = struct{}{}
		if strings.HasPrefix(typ, "return:") {
			return resolveGoReturnType(nodes, alias, filePath, strings.TrimPrefix(typ, "return:"))
		}
		next, ok := goPackageAliasType(nodes, filePath, typ)
		if !ok || next == typ {
			return typ
		}
		typ = next
	}
	return typ
}

func resolveGoReturnType(nodes []GraphNode, alias GraphNode, filePath, call string) string {
	call = strings.TrimSpace(call)
	if call == "" {
		return ""
	}
	if prefix, method, argCount, ok := splitGoCallName(call); ok {
		ref := UnresolvedReference{
			FromNodeID:    alias.ID,
			ReferenceName: method,
			ReceiverText:  prefix,
			ArgumentCount: argCount,
			ScopeNodeID:   alias.ID,
			ReferenceKind: EdgeKindCalls,
			FilePath:      filePath,
			Language:      LanguageGo,
			Line:          alias.StartLine,
		}
		if target, ok := resolveCallTarget(nodes, alias, ref); ok {
			return normalizeGoGraphType(target.ReturnType)
		}
		return ""
	}
	ref := UnresolvedReference{
		FromNodeID:    alias.ID,
		ReferenceName: call,
		ArgumentCount: 0,
		ScopeNodeID:   alias.ID,
		ReferenceKind: EdgeKindCalls,
		FilePath:      filePath,
		Language:      LanguageGo,
		Line:          alias.StartLine,
	}
	for _, node := range callTargetCandidates(nodes, alias, ref) {
		if node.ReturnType != "" {
			return normalizeGoGraphType(node.ReturnType)
		}
	}
	return ""
}

func splitGoCallName(call string) (string, string, int, bool) {
	call = strings.TrimSpace(call)
	if strings.HasSuffix(call, ")") {
		open := matchingOpenParen(call, len(call)-1)
		if open > 0 {
			args := countGoArguments(call[open+1 : len(call)-1])
			call = strings.TrimSpace(call[:open])
			dot := strings.LastIndex(call, ".")
			if dot > 0 && dot+1 < len(call) {
				return strings.TrimSpace(call[:dot]), strings.TrimSpace(call[dot+1:]), args, true
			}
		}
	}
	dot := strings.LastIndex(call, ".")
	if dot <= 0 || dot+1 >= len(call) {
		return "", "", 0, false
	}
	return strings.TrimSpace(call[:dot]), strings.TrimSpace(call[dot+1:]), 0, true
}

func normalizeGoGraphType(typ string) string {
	typ = strings.TrimSpace(typ)
	typ = strings.TrimPrefix(typ, "*")
	typ = strings.TrimPrefix(typ, "&")
	typ = strings.TrimSpace(typ)
	return typ
}

func goPackageAliasType(nodes []GraphNode, filePath, name string) (string, bool) {
	for _, node := range nodes {
		if node.Kind != NodeKindVariable || node.Language != LanguageGo || node.Name != name || strings.Contains(node.Signature, ":=") {
			continue
		}
		if path.Dir(node.FilePath) == path.Dir(filePath) {
			return strings.TrimSpace(node.ReceiverType), true
		}
	}
	return "", false
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
	if strings.HasSuffix(importPath, "::*") {
		prefix := strings.TrimSuffix(importPath, "*")
		return target.Name == ref.ReferenceName && strings.HasPrefix(target.QualifiedName, prefix)
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
	if hasVariadicParameter(node) {
		return ref.ArgumentCount >= node.ParameterCount-1
	}
	if isOptionalArityLanguage(node.Language) {
		return ref.ArgumentCount <= node.ParameterCount
	}
	return ref.ArgumentCount == node.ParameterCount
}

func hasVariadicParameter(node GraphNode) bool {
	if len(node.ParameterTypes) == 0 {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(node.ParameterTypes[len(node.ParameterTypes)-1]), "...")
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
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "()")
	value = strings.TrimSpace(value)
	if before, _, ok := strings.Cut(value, "{"); ok {
		value = strings.TrimSpace(before)
	}
	value = strings.TrimPrefix(value, "*")
	value = strings.TrimPrefix(value, "&")
	value = strings.TrimSpace(value)
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
