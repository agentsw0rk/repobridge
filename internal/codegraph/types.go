package codegraph

import (
	"time"

	"repobridge/internal/codegraph/parser"
)

type NodeKind = parser.NodeKind

const (
	NodeKindFile      = parser.NodeKindFile
	NodeKindModule    = parser.NodeKindModule
	NodeKindClass     = parser.NodeKindClass
	NodeKindStruct    = parser.NodeKindStruct
	NodeKindInterface = parser.NodeKindInterface
	NodeKindFunction  = parser.NodeKindFunction
	NodeKindMethod    = parser.NodeKindMethod
	NodeKindImport    = parser.NodeKindImport
)

type EdgeKind = parser.EdgeKind

const (
	EdgeKindContains = parser.EdgeKindContains
	EdgeKindCalls    = parser.EdgeKindCalls
	EdgeKindImports  = parser.EdgeKindImports
)

type Language = parser.Language

const (
	LanguageGo         = parser.LanguageGo
	LanguageJava       = parser.LanguageJava
	LanguageKotlin     = parser.LanguageKotlin
	LanguageCSharp     = parser.LanguageCSharp
	LanguageJavaScript = parser.LanguageJavaScript
	LanguageTypeScript = parser.LanguageTypeScript
	LanguagePython     = parser.LanguagePython
	LanguageRust       = parser.LanguageRust
	LanguageUnknown    = parser.LanguageUnknown
)

type GraphFile = parser.GraphFile

type GraphNode = parser.GraphNode

type GraphEdge = parser.GraphEdge

type UnresolvedReference = parser.UnresolvedReference

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
