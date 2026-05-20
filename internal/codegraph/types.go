package codegraph

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

type IndexResult struct {
	SourcePath    string
	Files         []GraphFile
	Nodes         []GraphNode
	Edges         []GraphEdge
	Unresolved    []UnresolvedReference
	Warnings      []string
	StartedAt     time.Time
	CompletedAt   time.Time
	SchemaVersion int
}

type SearchQuery struct {
	Text        string
	Kinds       []NodeKind
	Languages   []Language
	PathFilters []string
	NameFilters []string
	Calls       []string
	Limit       int
}

type SearchResult struct {
	Source        string   `json:"source"`
	Kind          NodeKind `json:"kind"`
	Name          string   `json:"name"`
	QualifiedName string   `json:"qualifiedName"`
	Language      Language `json:"language"`
	Path          string   `json:"path"`
	StartLine     int      `json:"startLine"`
	EndLine       int      `json:"endLine"`
	Score         float64  `json:"score"`
	Calls         []string `json:"calls"`
}
