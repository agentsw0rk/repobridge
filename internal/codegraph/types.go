package codegraph

import (
	"time"

	"repobridge/internal/codegraph/model"
)

type NodeKind = model.NodeKind

const (
	NodeKindFile      = model.NodeKindFile
	NodeKindModule    = model.NodeKindModule
	NodeKindClass     = model.NodeKindClass
	NodeKindStruct    = model.NodeKindStruct
	NodeKindInterface = model.NodeKindInterface
	NodeKindFunction  = model.NodeKindFunction
	NodeKindMethod    = model.NodeKindMethod
	NodeKindImport    = model.NodeKindImport
)

type EdgeKind = model.EdgeKind

const (
	EdgeKindContains = model.EdgeKindContains
	EdgeKindCalls    = model.EdgeKindCalls
	EdgeKindImports  = model.EdgeKindImports
)

type Language = model.Language

const (
	LanguageGo         = model.LanguageGo
	LanguageJava       = model.LanguageJava
	LanguageKotlin     = model.LanguageKotlin
	LanguageCSharp     = model.LanguageCSharp
	LanguageJavaScript = model.LanguageJavaScript
	LanguageTypeScript = model.LanguageTypeScript
	LanguagePython     = model.LanguagePython
	LanguageRust       = model.LanguageRust
	LanguageUnknown    = model.LanguageUnknown
)

type GraphFile = model.GraphFile

type GraphNode = model.GraphNode

type GraphEdge = model.GraphEdge

type UnresolvedReference = model.UnresolvedReference

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
