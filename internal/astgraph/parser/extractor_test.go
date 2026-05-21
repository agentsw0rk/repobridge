package parser

import (
	"reflect"
	"strings"
	"testing"

	"repobridge/internal/astgraph/model"
)

func TestExtractFromSourceFindsGoFunctionsMethodsAndCalls(t *testing.T) {
	source := []byte(`package demo

import "fmt"

func helper() {}

type Service struct{}

func (s Service) Run() {
	helper()
	fmt.Println("ok")
}
`)

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	assertNode(t, result.Nodes, model.NodeKindFunction, "helper")
	assertNode(t, result.Nodes, model.NodeKindMethod, "Run")
	assertUnresolved(t, result.Unresolved, "helper")
	assertUnresolved(t, result.Unresolved, "Println")
	runID := findNodeID(t, result.Nodes, model.NodeKindMethod, "Run")
	assertUnresolvedFrom(t, result.Unresolved, "helper", runID)
}

func TestExtractFromSourceSkipsGoCallsWithoutOwner(t *testing.T) {
	source := []byte("package demo\nvar x = helper()\nfunc helper() {}\n")

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	assertNoUnresolved(t, result.Unresolved, "helper")
}

func TestExtractFromSourceSkipsGoCallsInsideFunctionLiterals(t *testing.T) {
	source := []byte("package demo\nfunc outer() func() { return func() { helper() } }\nfunc helper() {}\n")

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	outerID := findNodeID(t, result.Nodes, model.NodeKindFunction, "outer")
	assertNoUnresolvedFrom(t, result.Unresolved, "helper", outerID)
	assertNoUnresolved(t, result.Unresolved, "helper")
}

func TestExtractFromSourceSkipsGoComplexCallTargets(t *testing.T) {
	source := []byte("package demo\nfunc outer() { func() {}() }\n")

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	assertNoUnresolvedContaining(t, result.Unresolved, "func")
}

func TestExtractFromSourceFindsJavaScriptFunctionAndCall(t *testing.T) {
	source := []byte(`function render() { updateContainer(); }`)

	result, err := ExtractFromSource("app.js", source, model.LanguageJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindFunction, "render", model.LanguageJavaScript)
	renderID := findNodeID(t, result.Nodes, model.NodeKindFunction, "render")
	assertUnresolvedWithLanguage(t, result.Unresolved, "updateContainer", model.LanguageJavaScript)
	assertUnresolvedFrom(t, result.Unresolved, "updateContainer", renderID)
}

func TestExtractFromSourceFindsExpressAndReactRouterRoutes(t *testing.T) {
	source := []byte(`function loginHandler(req, res) {}
const Settings = () => null
app.post('/api/login', loginHandler)
router.use('/admin', requireAdmin)
const routes = [{ path: '/settings', Component: Settings }]
const home = <Route path="/home" element={<Home />} />`)

	result, err := ExtractFromSource("routes.js", source, model.LanguageJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "POST /api/login")
	handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "loginHandler")
	assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	middlewareID := findNodeID(t, result.Nodes, model.NodeKindRoute, "USE /admin")
	middlewareHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "requireAdmin")
	assertEdge(t, result.Edges, middlewareID, middlewareHandlerID, model.EdgeKindMiddleware)
	settingsID := findNodeID(t, result.Nodes, model.NodeKindComponentRoute, "/settings")
	settingsTargetID := findNodeID(t, result.Nodes, model.NodeKindHandler, "Settings")
	assertEdge(t, result.Edges, settingsID, settingsTargetID, model.EdgeKindRoutesTo)
	homeID := findNodeID(t, result.Nodes, model.NodeKindComponentRoute, "/home")
	homeTargetID := findNodeID(t, result.Nodes, model.NodeKindHandler, "Home")
	assertEdge(t, result.Edges, homeID, homeTargetID, model.EdgeKindRoutesTo)
}

func TestExtractFromSourceFindsTypeScriptFunctionAndCall(t *testing.T) {
	source := []byte(`function render(): void { updateContainer(); }`)

	result, err := ExtractFromSource("app.ts", source, model.LanguageTypeScript)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindFunction, "render", model.LanguageTypeScript)
	renderID := findNodeID(t, result.Nodes, model.NodeKindFunction, "render")
	assertUnresolvedWithLanguage(t, result.Unresolved, "updateContainer", model.LanguageTypeScript)
	assertUnresolvedFrom(t, result.Unresolved, "updateContainer", renderID)
}

func TestExtractFromSourceFindsTypeScriptExpressRoute(t *testing.T) {
	source := []byte(`function getUser(req: Request, res: Response): void {}
router.get('/users/:id', getUser)`)

	result, err := ExtractFromSource("routes.ts", source, model.LanguageTypeScript)
	if err != nil {
		t.Fatal(err)
	}
	routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users/:id")
	handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "getUser")
	assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
}

func TestExtractFromSourceFindsPythonFunctionAndCall(t *testing.T) {
	source := []byte("def send():\n    request()\n")

	result, err := ExtractFromSource("client.py", source, model.LanguagePython)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindFunction, "send", model.LanguagePython)
	sendID := findNodeID(t, result.Nodes, model.NodeKindFunction, "send")
	assertUnresolvedWithLanguage(t, result.Unresolved, "request", model.LanguagePython)
	assertUnresolvedFrom(t, result.Unresolved, "request", sendID)
}

func TestExtractFromSourceFindsPythonFrameworkRoutes(t *testing.T) {
	source := []byte(`@app.post("/login")
def login_view():
    audit()

@blueprint.route("/health", methods=["GET"])
def health():
    pass

urlpatterns = [
    path("users/", user_view),
    re_path(r"^legacy/$", legacy_view),
]
`)

	result, err := ExtractFromSource("views.py", source, model.LanguagePython)
	if err != nil {
		t.Fatal(err)
	}
	loginRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "POST /login")
	loginHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "login_view")
	assertEdge(t, result.Edges, loginRouteID, loginHandlerID, model.EdgeKindHandles)
	healthRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /health")
	healthHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "health")
	assertEdge(t, result.Edges, healthRouteID, healthHandlerID, model.EdgeKindHandles)
	usersRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "ANY /users/")
	usersHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "user_view")
	assertEdge(t, result.Edges, usersRouteID, usersHandlerID, model.EdgeKindHandles)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindRoute, "ANY ^legacy/$", model.LanguagePython)
	assertUnresolvedFrom(t, result.Unresolved, "audit", loginHandlerID)
}

func TestExtractFromSourceFindsRustFunctionAndCall(t *testing.T) {
	source := []byte(`fn run() { helper(); }`)

	result, err := ExtractFromSource("main.rs", source, model.LanguageRust)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindFunction, "run", model.LanguageRust)
	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "run")
	assertUnresolvedWithLanguage(t, result.Unresolved, "helper", model.LanguageRust)
	assertUnresolvedFrom(t, result.Unresolved, "helper", runID)
}

func TestExtractFromSourceFindsRustFrameworkRoutes(t *testing.T) {
	source := []byte(`#[get("/health")]
async fn health_check() {}

fn app() {
    Router::new().route("/users", get(list_users)).nest("/api", api_router);
}`)

	result, err := ExtractFromSource("routes.rs", source, model.LanguageRust)
	if err != nil {
		t.Fatal(err)
	}
	healthRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /health")
	healthHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "health_check")
	assertEdge(t, result.Edges, healthRouteID, healthHandlerID, model.EdgeKindHandles)
	usersRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users")
	usersHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "list_users")
	assertEdge(t, result.Edges, usersRouteID, usersHandlerID, model.EdgeKindHandles)
	nestRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "ANY /api")
	nestTargetID := findNodeID(t, result.Nodes, model.NodeKindHandler, "api_router")
	assertEdge(t, result.Edges, nestRouteID, nestTargetID, model.EdgeKindRoutesTo)
}

func TestExtractFromSourceFindsJavaMethodAndCall(t *testing.T) {
	source := []byte(`class App { void run() { helper(); } }`)

	result, err := ExtractFromSource("App.java", source, model.LanguageJava)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindMethod, "run", model.LanguageJava)
	runID := findNodeID(t, result.Nodes, model.NodeKindMethod, "run")
	assertUnresolvedWithLanguage(t, result.Unresolved, "helper", model.LanguageJava)
	assertUnresolvedFrom(t, result.Unresolved, "helper", runID)
}

func TestExtractFromSourceFindsSpringJavaRouteAndHandler(t *testing.T) {
	source := []byte(`@RestController
@RequestMapping("/api")
class AuthController {
  @PostMapping(path="/login")
  public void login() { audit(); }
}`)

	result, err := ExtractFromSource("AuthController.java", source, model.LanguageJava)
	if err != nil {
		t.Fatal(err)
	}
	routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "POST /api/login")
	handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "login")
	assertQualifiedNode(t, result.Nodes, model.NodeKindHandler, "login", "AuthController.login")
	assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	assertUnresolvedFrom(t, result.Unresolved, "audit", handlerID)
}

func TestExtractFromSourceFindsSpringJavaRequestMappingMethod(t *testing.T) {
	source := []byte(`@RestController
class AuthController {
  @RequestMapping(value="/login", method=RequestMethod.POST)
  public void login() {}
}`)

	result, err := ExtractFromSource("AuthController.java", source, model.LanguageJava)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindRoute, "POST /login", model.LanguageJava)
}

func TestExtractFromSourceFindsCSharpMethodAndCall(t *testing.T) {
	source := []byte(`class App { void Run() { Helper(); } }`)

	result, err := ExtractFromSource("App.cs", source, model.LanguageCSharp)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindMethod, "Run", model.LanguageCSharp)
	runID := findNodeID(t, result.Nodes, model.NodeKindMethod, "Run")
	assertUnresolvedWithLanguage(t, result.Unresolved, "Helper", model.LanguageCSharp)
	assertUnresolvedFrom(t, result.Unresolved, "Helper", runID)
}

func TestExtractFromSourceFindsAspNetRoutes(t *testing.T) {
	source := []byte(`[Route("api/[controller]")]
public class UsersController {
  [HttpGet("{id}")]
  public IActionResult Get(int id) { return Ok(); }
}
var app = builder.Build();
app.MapPost("/login", Login);`)

	result, err := ExtractFromSource("UsersController.cs", source, model.LanguageCSharp)
	if err != nil {
		t.Fatal(err)
	}
	controllerRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /api/users/{id}")
	controllerHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "Get")
	assertEdge(t, result.Edges, controllerRouteID, controllerHandlerID, model.EdgeKindHandles)
	minimalRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "POST /login")
	minimalHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "Login")
	assertEdge(t, result.Edges, minimalRouteID, minimalHandlerID, model.EdgeKindHandles)
}

func TestExtractFromSourceFindsKotlinFunctionAndCall(t *testing.T) {
	result, err := ExtractFromSource("App.kt", []byte(`fun run() { helper() }`), model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindFunction, "run", model.LanguageKotlin)
	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "run")
	assertUnresolvedWithLanguage(t, result.Unresolved, "helper", model.LanguageKotlin)
	assertUnresolvedFrom(t, result.Unresolved, "helper", runID)
}

func TestExtractFromSourceAddsStructuredKotlinCallAndSignatureMetadata(t *testing.T) {
	source := []byte(`class Box {
  fun pick(value: String): Int { return value.length }
  fun run() { pick("x") }
}`)

	result, err := ExtractFromSource("Box.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}

	pick := findNode(t, result.Nodes, model.NodeKindFunction, "pick")
	if pick.QualifiedName != "Box.pick" {
		t.Fatalf("pick qualifiedName = %q, want Box.pick", pick.QualifiedName)
	}
	if pick.ReceiverType != "Box" {
		t.Fatalf("pick receiverType = %q, want Box", pick.ReceiverType)
	}
	if pick.ParameterCount != 1 || !reflect.DeepEqual(pick.ParameterTypes, []string{"String"}) {
		t.Fatalf("pick parameters = %d %#v, want 1 [String]", pick.ParameterCount, pick.ParameterTypes)
	}
	if pick.ReturnType != "Int" {
		t.Fatalf("pick returnType = %q, want Int", pick.ReturnType)
	}

	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "run")
	ref := findUnresolvedFrom(t, result.Unresolved, "pick", runID)
	if ref.ScopeNodeID != runID {
		t.Fatalf("scopeNodeID = %q, want %q", ref.ScopeNodeID, runID)
	}
	if ref.ArgumentCount != 1 || !reflect.DeepEqual(ref.ArgumentTexts, []string{`"x"`}) {
		t.Fatalf("arguments = %d %#v, want 1 [\"x\"]", ref.ArgumentCount, ref.ArgumentTexts)
	}
}

func TestExtractFromSourceRecordsKotlinExtensionReceiver(t *testing.T) {
	source := []byte(`fun String.words(limit: Int): List<String> { return split(" ").take(limit) }`)

	result, err := ExtractFromSource("Extensions.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}

	words := findNode(t, result.Nodes, model.NodeKindFunction, "words")
	if words.QualifiedName != "String.words" || words.ReceiverType != "String" {
		t.Fatalf("extension node = %#v, want String.words receiver String", words)
	}
	if words.ParameterCount != 1 || !reflect.DeepEqual(words.ParameterTypes, []string{"Int"}) {
		t.Fatalf("extension parameters = %d %#v, want 1 [Int]", words.ParameterCount, words.ParameterTypes)
	}
	if words.ReturnType != "List<String>" {
		t.Fatalf("extension returnType = %q, want List<String>", words.ReturnType)
	}
}

func TestExtractFromSourceFindsSpringKotlinRouteAndHandler(t *testing.T) {
	source := []byte(`@RestController
@RequestMapping("/api")
class AuthController {
  @GetMapping("/users/{id}")
  fun user() { audit() }
}`)

	result, err := ExtractFromSource("AuthController.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}
	routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /api/users/{id}")
	handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "user")
	assertQualifiedNode(t, result.Nodes, model.NodeKindHandler, "user", "AuthController.user")
	assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	assertUnresolvedFrom(t, result.Unresolved, "audit", handlerID)
}

func TestExtractFromSourceWarnsForUnsupportedLanguage(t *testing.T) {
	result, err := ExtractFromSource("file.unknown", []byte("content"), model.LanguageUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "unsupported language: unknown" {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}
	if len(result.Nodes) != 0 {
		t.Fatalf("expected no nodes, got %#v", result.Nodes)
	}
	if len(result.Unresolved) != 0 {
		t.Fatalf("expected no unresolved references, got %#v", result.Unresolved)
	}
}

func assertNodeWithLanguage(t *testing.T, nodes []model.GraphNode, kind model.NodeKind, name string, language model.Language) {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name && node.Language == language {
			return
		}
	}
	t.Fatalf("node %s %s with language %s not found in %#v", kind, name, language, nodes)
}

func assertNode(t *testing.T, nodes []model.GraphNode, kind model.NodeKind, name string) {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return
		}
	}
	t.Fatalf("node %s %s not found in %#v", kind, name, nodes)
}

func assertQualifiedNode(t *testing.T, nodes []model.GraphNode, kind model.NodeKind, name, qualified string) {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name && node.QualifiedName == qualified {
			return
		}
	}
	t.Fatalf("node %s %s qualified %s not found in %#v", kind, name, qualified, nodes)
}

func findNodeID(t *testing.T, nodes []model.GraphNode, kind model.NodeKind, name string) string {
	t.Helper()
	return findNode(t, nodes, kind, name).ID
}

func findNode(t *testing.T, nodes []model.GraphNode, kind model.NodeKind, name string) model.GraphNode {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return node
		}
	}
	t.Fatalf("node %s %s not found in %#v", kind, name, nodes)
	return model.GraphNode{}
}

func assertEdge(t *testing.T, edges []model.GraphEdge, fromID, toID string, kind model.EdgeKind) {
	t.Helper()
	for _, edge := range edges {
		if edge.SourceNodeID == fromID && edge.TargetNodeID == toID && edge.Kind == kind {
			return
		}
	}
	t.Fatalf("edge %s -> %s %s not found in %#v", fromID, toID, kind, edges)
}

func assertUnresolved(t *testing.T, refs []model.UnresolvedReference, name string) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name {
			return
		}
	}
	t.Fatalf("reference %s not found in %#v", name, refs)
}

func assertUnresolvedWithLanguage(t *testing.T, refs []model.UnresolvedReference, name string, language model.Language) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name && ref.Language == language {
			return
		}
	}
	t.Fatalf("reference %s with language %s not found in %#v", name, language, refs)
}

func assertUnresolvedFrom(t *testing.T, refs []model.UnresolvedReference, name string, fromID string) {
	t.Helper()
	_ = findUnresolvedFrom(t, refs, name, fromID)
}

func findUnresolvedFrom(t *testing.T, refs []model.UnresolvedReference, name string, fromID string) model.UnresolvedReference {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name && ref.FromNodeID == fromID {
			return ref
		}
	}
	t.Fatalf("reference %s from %s not found in %#v", name, fromID, refs)
	return model.UnresolvedReference{}
}

func assertNoUnresolvedFrom(t *testing.T, refs []model.UnresolvedReference, name string, fromID string) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name && ref.FromNodeID == fromID {
			t.Fatalf("unexpected reference %s from %s found in %#v", name, fromID, refs)
		}
	}
}

func assertNoUnresolved(t *testing.T, refs []model.UnresolvedReference, name string) {
	t.Helper()
	for _, ref := range refs {
		if ref.ReferenceName == name {
			t.Fatalf("unexpected reference %s found in %#v", name, refs)
		}
	}
}

func assertNoUnresolvedContaining(t *testing.T, refs []model.UnresolvedReference, text string) {
	t.Helper()
	for _, ref := range refs {
		if strings.Contains(ref.ReferenceName, text) {
			t.Fatalf("unexpected reference containing %q found in %#v", text, refs)
		}
	}
}

func assertWarning(t *testing.T, warnings []string, want string) {
	t.Helper()
	for _, warning := range warnings {
		if warning == want {
			return
		}
	}
	t.Fatalf("warning %q not found in %#v", want, warnings)
}
