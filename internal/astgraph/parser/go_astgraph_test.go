package parser

import (
	"reflect"
	"testing"

	"repobridge/internal/astgraph/model"
)

func TestExtractFromSourceMapsGoStructFieldTypeVariants(t *testing.T) {
	source := []byte(`package demo

type Repository struct{}
type Item struct{}
type Service struct {
	Repo *Repository
	Items []Item
	Lookup map[string]Item
}`)

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	serviceID := findNodeID(t, result.Nodes, model.NodeKindStruct, "Service")
	repositoryID := findNodeID(t, result.Nodes, model.NodeKindStruct, "Repository")
	itemID := findNodeID(t, result.Nodes, model.NodeKindStruct, "Item")
	repoField := findNode(t, result.Nodes, model.NodeKindField, "Repo")
	itemsField := findNode(t, result.Nodes, model.NodeKindField, "Items")
	lookupField := findNode(t, result.Nodes, model.NodeKindField, "Lookup")

	if repoField.ReturnType != "Repository" {
		t.Fatalf("Repo returnType = %q, want Repository", repoField.ReturnType)
	}
	if itemsField.ReturnType != "Item" {
		t.Fatalf("Items returnType = %q, want Item", itemsField.ReturnType)
	}
	if lookupField.ReturnType != "Item" {
		t.Fatalf("Lookup returnType = %q, want Item", lookupField.ReturnType)
	}

	assertEdge(t, result.Edges, serviceID, repoField.ID, model.EdgeKindContains)
	assertEdge(t, result.Edges, serviceID, itemsField.ID, model.EdgeKindContains)
	assertEdge(t, result.Edges, serviceID, lookupField.ID, model.EdgeKindContains)
	assertEdge(t, result.Edges, repoField.ID, repositoryID, model.EdgeKindTypeOf)
	assertEdge(t, result.Edges, itemsField.ID, itemID, model.EdgeKindTypeOf)
	assertEdge(t, result.Edges, lookupField.ID, itemID, model.EdgeKindTypeOf)
}

func TestExtractFromSourceMapsGoConstBlocksAndGenericTypeAliases(t *testing.T) {
	source := []byte(`package demo

type Item struct{}
type Cache[K comparable] map[K]Item
const (
	MaxUsers = 10
	DefaultName = "guest"
)`)

	result, err := ExtractFromSource("types.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	cache := findNode(t, result.Nodes, model.NodeKindTypeAlias, "Cache")
	if cache.ReturnType != "map[K]Item" {
		t.Fatalf("Cache returnType = %q, want map[K]Item", cache.ReturnType)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindConstant, "MaxUsers", model.LanguageGo)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindConstant, "DefaultName", model.LanguageGo)
}

func TestExtractFromSourceMapsGoCompactInterfaceMethods(t *testing.T) {
	source := []byte(`package demo

type ID string
type Item struct{}
type Store interface{ Save(ctx context.Context, item *Item) error; Find(id ID) (*Item, error) }`)

	result, err := ExtractFromSource("store.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	storeID := findNodeID(t, result.Nodes, model.NodeKindInterface, "Store")
	save := findNode(t, result.Nodes, model.NodeKindMethod, "Save")
	find := findNode(t, result.Nodes, model.NodeKindMethod, "Find")

	if save.QualifiedName != "Store.Save" || save.ReceiverType != "Store" {
		t.Fatalf("Save node = %#v, want Store.Save receiver Store", save)
	}
	if save.ParameterCount != 2 || !reflect.DeepEqual(save.ParameterTypes, []string{"context.Context", "Item"}) {
		t.Fatalf("Save parameters = %d %#v, want 2 [context.Context Item]", save.ParameterCount, save.ParameterTypes)
	}
	if save.ReturnType != "error" {
		t.Fatalf("Save returnType = %q, want error", save.ReturnType)
	}
	if find.ReturnType != "Item" {
		t.Fatalf("Find returnType = %q, want Item", find.ReturnType)
	}

	assertEdge(t, result.Edges, storeID, save.ID, model.EdgeKindContains)
	assertEdge(t, result.Edges, storeID, find.ID, model.EdgeKindContains)
}

func TestExtractFromSourceFindsGoGenericCallExpressions(t *testing.T) {
	source := []byte(`package demo

type User struct{}

func Load[T any](raw []byte) T { return User{} }

func run(raw []byte) {
	_ = Load[User](raw)
}`)

	result, err := ExtractFromSource("generic.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "run")
	load := findUnresolvedFrom(t, result.Unresolved, "Load", runID)
	if load.ArgumentCount != 1 || !reflect.DeepEqual(load.ArgumentTexts, []string{"raw"}) {
		t.Fatalf("Load arguments = %d %#v, want 1 [raw]", load.ArgumentCount, load.ArgumentTexts)
	}
}

func TestExtractFromSourceMapsGitHubDerivedGoRoutePatterns(t *testing.T) {
	source := []byte(`package demo

func Register(router *Engine, muxRouter *Router) {
	apiV1 := router.Group("/v1")
	authV1 := apiV1.Group("/", AuthMiddleWare())
	authV1.POST("users/add", AddV1User)
	muxRouter.HandleFunc("/products", ProductsHandler).Methods("GET")
}

func AddV1User(c *Context) {}
func ProductsHandler(w ResponseWriter, r *Request) {}
func AuthMiddleWare() HandlerFunc { return nil }`)

	result, err := ExtractFromSource("routes.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	ginRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "POST /v1/users/add")
	ginHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "AddV1User")
	assertEdge(t, result.Edges, ginRouteID, ginHandlerID, model.EdgeKindHandles)

	muxRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /products")
	muxHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "ProductsHandler")
	assertEdge(t, result.Edges, muxRouteID, muxHandlerID, model.EdgeKindHandles)
}

func TestExtractFromSourceMapsGitHubDerivedGoTypeAliasesAndReturnEdges(t *testing.T) {
	source := []byte(`package demo

type ExpectedIptablesRule struct{}
type Entry struct{}
type ExpectedIptablesChain map[string][]ExpectedIptablesRule
type ExpectedIPSet map[string][]*Entry

func BuildChain() ExpectedIptablesChain {
	return ExpectedIptablesChain{}
}`)

	result, err := ExtractFromSource("types.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	chainID := findNodeID(t, result.Nodes, model.NodeKindTypeAlias, "ExpectedIptablesChain")
	buildID := findNodeID(t, result.Nodes, model.NodeKindFunction, "BuildChain")
	ipset := findNode(t, result.Nodes, model.NodeKindTypeAlias, "ExpectedIPSet")
	if ipset.ReturnType != "map[string][]*Entry" {
		t.Fatalf("ExpectedIPSet returnType = %q, want map[string][]*Entry", ipset.ReturnType)
	}
	assertEdge(t, result.Edges, buildID, chainID, model.EdgeKindReturns)
	assertEdge(t, result.Edges, buildID, chainID, model.EdgeKindInstantiates)
}

func TestExtractFromSourceMapsGoStdlibInterfaceSignatures(t *testing.T) {
	source := []byte(`package demo

type Addr interface{ Network() string; String() string }
type Conn interface {
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	LocalAddr() Addr
}`)

	result, err := ExtractFromSource("net.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	connID := findNodeID(t, result.Nodes, model.NodeKindInterface, "Conn")
	read := findNode(t, result.Nodes, model.NodeKindMethod, "Read")
	localAddr := findNode(t, result.Nodes, model.NodeKindMethod, "LocalAddr")

	if read.ParameterCount != 1 || !reflect.DeepEqual(read.ParameterTypes, []string{"byte"}) {
		t.Fatalf("Read parameters = %d %#v, want 1 [byte]", read.ParameterCount, read.ParameterTypes)
	}
	if read.ReturnType != "int" {
		t.Fatalf("Read returnType = %q, want int", read.ReturnType)
	}
	if localAddr.ReturnType != "Addr" {
		t.Fatalf("LocalAddr returnType = %q, want Addr", localAddr.ReturnType)
	}
	assertEdge(t, result.Edges, connID, read.ID, model.EdgeKindContains)
	assertEdge(t, result.Edges, connID, localAddr.ID, model.EdgeKindContains)
}
