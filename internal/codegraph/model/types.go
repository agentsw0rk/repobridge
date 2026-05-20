package model

import "time"

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
