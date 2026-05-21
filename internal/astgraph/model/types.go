package model

import "time"

type NodeKind string

const (
	NodeKindFile           NodeKind = "file"
	NodeKindModule         NodeKind = "module"
	NodeKindClass          NodeKind = "class"
	NodeKindStruct         NodeKind = "struct"
	NodeKindInterface      NodeKind = "interface"
	NodeKindFunction       NodeKind = "function"
	NodeKindMethod         NodeKind = "method"
	NodeKindImport         NodeKind = "import"
	NodeKindExternal       NodeKind = "external"
	NodeKindVariable       NodeKind = "variable"
	NodeKindField          NodeKind = "field"
	NodeKindProperty       NodeKind = "property"
	NodeKindConstant       NodeKind = "constant"
	NodeKindEnum           NodeKind = "enum"
	NodeKindEnumMember     NodeKind = "enum_member"
	NodeKindTrait          NodeKind = "trait"
	NodeKindProtocol       NodeKind = "protocol"
	NodeKindTypeAlias      NodeKind = "type_alias"
	NodeKindRoute          NodeKind = "route"
	NodeKindHandler        NodeKind = "handler"
	NodeKindComponentRoute NodeKind = "component_route"
)

type EdgeKind string

const (
	EdgeKindContains     EdgeKind = "contains"
	EdgeKindCalls        EdgeKind = "calls"
	EdgeKindImports      EdgeKind = "imports"
	EdgeKindHandles      EdgeKind = "handles"
	EdgeKindRoutesTo     EdgeKind = "routes_to"
	EdgeKindMiddleware   EdgeKind = "middleware"
	EdgeKindExtends      EdgeKind = "extends"
	EdgeKindImplements   EdgeKind = "implements"
	EdgeKindReferences   EdgeKind = "references"
	EdgeKindTypeOf       EdgeKind = "type_of"
	EdgeKindReturns      EdgeKind = "returns"
	EdgeKindInstantiates EdgeKind = "instantiates"
	EdgeKindOverrides    EdgeKind = "overrides"
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
	ID             string
	Kind           NodeKind
	Name           string
	QualifiedName  string
	ReceiverType   string
	ParameterCount int
	ParameterTypes []string
	ReturnType     string
	FilePath       string
	Language       Language
	StartLine      int
	EndLine        int
	StartColumn    int
	EndColumn      int
	Signature      string
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
	ReceiverText  string
	ArgumentCount int
	ArgumentTexts []string
	ScopeNodeID   string
	ReferenceKind EdgeKind
	FilePath      string
	Language      Language
	Line          int
	Column        int
}
