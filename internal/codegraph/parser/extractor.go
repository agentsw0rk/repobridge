package parser

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"time"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type NodeKind string

const (
	NodeKindFile      NodeKind = "file"
	NodeKindModule    NodeKind = "module"
	NodeKindClass     NodeKind = "class"
	NodeKindStruct    NodeKind = "struct"
	NodeKindInterface NodeKind = "interface"
	NodeKindFunction  NodeKind = "function"
	NodeKindMethod    NodeKind = "method"
	NodeKindImport    NodeKind = "import"
)

type EdgeKind string

const (
	EdgeKindContains EdgeKind = "contains"
	EdgeKindCalls    EdgeKind = "calls"
	EdgeKindImports  EdgeKind = "imports"
)

type Language string

const (
	LanguageGo         Language = "go"
	LanguageJava       Language = "java"
	LanguageKotlin     Language = "kotlin"
	LanguageCSharp     Language = "csharp"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
	LanguagePython     Language = "python"
	LanguageRust       Language = "rust"
	LanguageUnknown    Language = "unknown"
)

type GraphFile struct {
	Path        string
	Language    Language
	ContentHash string
	Size        int64
	ModifiedAt  time.Time
	IndexedAt   time.Time
	NodeCount   int
}

type GraphNode struct {
	ID            string
	Kind          NodeKind
	Name          string
	QualifiedName string
	FilePath      string
	Language      Language
	StartLine     int
	EndLine       int
	StartColumn   int
	EndColumn     int
	Signature     string
}

type GraphEdge struct {
	SourceNodeID string
	TargetNodeID string
	Kind         EdgeKind
	FilePath     string
	Line         int
	Column       int
	Provenance   string
}

type UnresolvedReference struct {
	FromNodeID    string
	ReferenceName string
	ReferenceKind EdgeKind
	FilePath      string
	Language      Language
	Line          int
	Column        int
}

type ExtractionResult struct {
	Nodes      []GraphNode
	Edges      []GraphEdge
	Unresolved []UnresolvedReference
	Warnings   []string
}

func ExtractFromSource(path string, source []byte, language Language) (ExtractionResult, error) {
	tsLanguage, ok := languageFor(language)
	if !ok {
		return ExtractionResult{
			Warnings: []string{fmt.Sprintf("unsupported language: %s", language)},
		}, nil
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(tsLanguage); err != nil {
		return ExtractionResult{}, err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return ExtractionResult{}, fmt.Errorf("parse failed: %s", path)
	}
	defer tree.Close()

	result := ExtractionResult{}
	walkByLanguage(path, source, tree.RootNode(), language, &result)
	return result, nil
}

func stableNodeID(path string, kind NodeKind, name string, line int) string {
	hash := sha1.Sum([]byte(fmt.Sprintf("%s:%s:%s:%d", path, kind, name, line)))
	return hex.EncodeToString(hash[:])
}
