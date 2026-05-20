package codegraph

import (
	"time"

	"repobridge/internal/codegraph/model"
	"repobridge/internal/source"
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
	ID            string   `json:"id,omitempty"`
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

type GraphCounts struct {
	Files      int `json:"files"`
	Nodes      int `json:"nodes"`
	Edges      int `json:"edges"`
	Unresolved int `json:"unresolved"`
	Warnings   int `json:"warnings"`
}

type GraphInspectOptions struct {
	CWD         string
	SyncIndex   bool
	Limit       int
	PathFilter  string
	SourceLines int
	SourceOpts  source.Options
}

type GraphInspectStatus struct {
	Source        string      `json:"source"`
	SourcePath    string      `json:"sourcePath"`
	GraphPath     string      `json:"graphPath"`
	Status        string      `json:"status"`
	SchemaVersion int         `json:"schemaVersion"`
	ErrorText     string      `json:"errorText,omitempty"`
	IndexedAt     time.Time   `json:"indexedAt,omitempty"`
	Counts        GraphCounts `json:"counts"`
}

type GraphFilesResult struct {
	Source string      `json:"source"`
	Files  []GraphFile `json:"files"`
}

type SourceLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

type GraphNodeDetail struct {
	ID            string       `json:"id"`
	Kind          NodeKind     `json:"kind"`
	Name          string       `json:"name"`
	QualifiedName string       `json:"qualifiedName"`
	Language      Language     `json:"language"`
	Path          string       `json:"path"`
	StartLine     int          `json:"startLine"`
	EndLine       int          `json:"endLine"`
	Signature     string       `json:"signature,omitempty"`
	Calls         []string     `json:"calls,omitempty"`
	Source        []SourceLine `json:"source,omitempty"`
}

type GraphNodeLookupResult struct {
	Source  string            `json:"source"`
	Node    *GraphNodeDetail  `json:"node,omitempty"`
	Matches []GraphNodeDetail `json:"matches,omitempty"`
}

type GraphNodeQuery struct {
	Lookup string
	Limit  int
}

type CallgraphDirection string

const (
	CallgraphDirectionCallers CallgraphDirection = "callers"
	CallgraphDirectionCallees CallgraphDirection = "callees"
	CallgraphDirectionImpact  CallgraphDirection = "impact"
)

type CallgraphOptions struct {
	CWD               string
	SyncIndex         bool
	Limit             int
	Depth             int
	Kinds             []NodeKind
	Languages         []Language
	PathFilters       []string
	IncludeUnresolved bool
	Direction         CallgraphDirection
	SourceOpts        source.Options
}

type CallgraphResult struct {
	Source    string             `json:"source"`
	Symbol    string             `json:"symbol"`
	Direction CallgraphDirection `json:"direction"`
	Root      *GraphNodeDetail   `json:"root,omitempty"`
	Matches   []GraphNodeDetail  `json:"matches,omitempty"`
	Edges     []CallgraphEdge    `json:"edges"`
}

type CallgraphEdge struct {
	Depth         int             `json:"depth"`
	From          GraphNodeDetail `json:"from"`
	To            GraphNodeDetail `json:"to"`
	Kind          EdgeKind        `json:"kind"`
	Path          string          `json:"path,omitempty"`
	Line          int             `json:"line,omitempty"`
	Column        int             `json:"column,omitempty"`
	ReferenceName string          `json:"referenceName,omitempty"`
	Unresolved    bool            `json:"unresolved,omitempty"`
}

type CallgraphQuery struct {
	RootNodeID        string
	Direction         CallgraphDirection
	Depth             int
	Limit             int
	Kinds             []NodeKind
	Languages         []Language
	PathFilters       []string
	IncludeUnresolved bool
}

type ContextMode string

const (
	ContextModeContext ContextMode = "context"
	ContextModeExplore ContextMode = "explore"
)

type ContextOptions struct {
	CWD        string
	SyncIndex  bool
	Limit      int
	Depth      int
	Budget     string
	Mode       ContextMode
	SourceOpts source.Options
}

type ContextBudget struct {
	Name         string `json:"name"`
	SearchLimit  int    `json:"searchLimit"`
	SnippetCount int    `json:"snippetCount"`
	SourceLines  int    `json:"sourceLines"`
	Depth        int    `json:"depth"`
}

type ContextParsedQuery struct {
	Raw     string      `json:"raw"`
	Terms   []string    `json:"terms"`
	Symbols []string    `json:"symbols"`
	Search  SearchQuery `json:"search"`
}

type ContextResult struct {
	Source        string             `json:"source"`
	Mode          ContextMode        `json:"mode"`
	Query         string             `json:"query"`
	Budget        ContextBudget      `json:"budget"`
	EntryPoints   []GraphNodeDetail  `json:"entryPoints"`
	Relationships []CallgraphEdge    `json:"relationships"`
	Snippets      []ContextSnippet   `json:"snippets"`
	RelatedFiles  []GraphFile        `json:"relatedFiles"`
	Warnings      []string           `json:"warnings,omitempty"`
	Stats         ContextResultStats `json:"stats"`
}

type ContextSnippet struct {
	Path      string       `json:"path"`
	StartLine int          `json:"startLine"`
	EndLine   int          `json:"endLine"`
	Lines     []SourceLine `json:"lines"`
}

type ContextResultStats struct {
	Terms         int `json:"terms"`
	EntryPoints   int `json:"entryPoints"`
	Relationships int `json:"relationships"`
	Snippets      int `json:"snippets"`
	RelatedFiles  int `json:"relatedFiles"`
}
