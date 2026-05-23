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
	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "fmt", "fmt")
	assertUnresolved(t, result.Unresolved, "helper")
	assertUnresolved(t, result.Unresolved, "Println")
	runID := findNodeID(t, result.Nodes, model.NodeKindMethod, "Run")
	assertUnresolvedFrom(t, result.Unresolved, "helper", runID)
}

func TestExtractFromSourceFindsGoImports(t *testing.T) {
	source := []byte(`package demo

import (
	"fmt"
	h "net/http"
	assert "github.com/stretchr/testify/assert"
)

func run() { fmt.Println("ok"); h.NewRequest("GET", "/", nil); assert.Equal(1, 1) }
`)

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "fmt", "fmt")
	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "h", "net/http")
	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "assert", "github.com/stretchr/testify/assert")
}

func TestExtractFromSourceFindsGoValueAliases(t *testing.T) {
	source := []byte(`package demo

type formPostBinding struct{}
var FormPost = formPostBinding{}

func run() {
	b := FormPost
	_ = b
}
`)

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	assertQualifiedNode(t, result.Nodes, model.NodeKindVariable, "FormPost", "formPostBinding")
	assertQualifiedNode(t, result.Nodes, model.NodeKindVariable, "b", "formPostBinding")
}

func TestExtractFromSourceFindsExpandedGoKinds(t *testing.T) {
	source := []byte(`package demo

type ID string
type User struct {
	Name string
	Profile Profile
}
type Profile struct{}
const MaxUsers = 10
func NewUser() User { return User{} }
func Run() { _ = Profile{} }
`)

	result, err := ExtractFromSource("types.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	assertNodeWithLanguage(t, result.Nodes, model.NodeKindTypeAlias, "ID", model.LanguageGo)
	nameID := findNodeID(t, result.Nodes, model.NodeKindField, "Name")
	profileFieldID := findNodeID(t, result.Nodes, model.NodeKindField, "Profile")
	profileID := findNodeID(t, result.Nodes, model.NodeKindStruct, "Profile")
	newUserID := findNodeID(t, result.Nodes, model.NodeKindFunction, "NewUser")
	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "Run")
	userID := findNodeID(t, result.Nodes, model.NodeKindStruct, "User")
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindConstant, "MaxUsers", model.LanguageGo)
	assertEdge(t, result.Edges, profileFieldID, profileID, model.EdgeKindTypeOf)
	assertEdge(t, result.Edges, newUserID, userID, model.EdgeKindReturns)
	assertEdge(t, result.Edges, newUserID, userID, model.EdgeKindInstantiates)
	assertEdge(t, result.Edges, runID, profileID, model.EdgeKindInstantiates)
	if nameID == "" {
		t.Fatalf("Name field ID is empty")
	}
}

func TestExtractFromSourceAddsGoContainmentGraph(t *testing.T) {
	source := []byte(`package demo

import "fmt"

type Service struct {
	Name string
}

func helper() {}

func (s Service) Run() {
	helper()
	fmt.Println("ok")
}
`)

	result, err := ExtractFromSource("service.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	fileID := findNodeID(t, result.Nodes, model.NodeKindFile, "service.go")
	moduleID := findNodeID(t, result.Nodes, model.NodeKindModule, "demo")
	serviceID := findNodeID(t, result.Nodes, model.NodeKindStruct, "Service")
	importID := findNodeID(t, result.Nodes, model.NodeKindImport, "fmt")
	fieldID := findNodeID(t, result.Nodes, model.NodeKindField, "Name")
	helperID := findNodeID(t, result.Nodes, model.NodeKindFunction, "helper")
	runID := findNodeID(t, result.Nodes, model.NodeKindMethod, "Run")

	assertEdge(t, result.Edges, fileID, moduleID, model.EdgeKindContains)
	assertEdge(t, result.Edges, moduleID, importID, model.EdgeKindContains)
	assertEdge(t, result.Edges, moduleID, serviceID, model.EdgeKindContains)
	assertEdge(t, result.Edges, moduleID, helperID, model.EdgeKindContains)
	assertEdge(t, result.Edges, serviceID, fieldID, model.EdgeKindContains)
	assertEdge(t, result.Edges, serviceID, runID, model.EdgeKindContains)
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

func TestExtractFromSourceFindsRustConstructorsMethodsAndReceivers(t *testing.T) {
	source := []byte(`enum Token {
    Str(&'static str),
    I32(i32),
}

struct Formatter;

impl Formatter {
    fn write_str(&mut self, value: &str) {}
}

fn run(formatter: &mut Formatter) {
    Token::Str("value");
    formatter.write_str("value");
}`)

	result, err := ExtractFromSource("lib.rs", source, model.LanguageRust)
	if err != nil {
		t.Fatal(err)
	}

	str := findNode(t, result.Nodes, model.NodeKindClass, "Str")
	if str.QualifiedName != "Token::Str" || str.ReceiverType != "Token" {
		t.Fatalf("Str variant node = %#v, want Token::Str receiver Token", str)
	}
	if str.ParameterCount != 1 || !reflect.DeepEqual(str.ParameterTypes, []string{"&'static str"}) {
		t.Fatalf("Str parameters = %d %#v, want 1 [&'static str]", str.ParameterCount, str.ParameterTypes)
	}

	writeStr := findNode(t, result.Nodes, model.NodeKindMethod, "write_str")
	if writeStr.QualifiedName != "Formatter::write_str" || writeStr.ReceiverType != "Formatter" {
		t.Fatalf("write_str node = %#v, want Formatter::write_str receiver Formatter", writeStr)
	}
	if writeStr.ParameterCount != 1 || !reflect.DeepEqual(writeStr.ParameterTypes, []string{"&str"}) {
		t.Fatalf("write_str parameters = %d %#v, want 1 [&str]", writeStr.ParameterCount, writeStr.ParameterTypes)
	}

	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "run")
	strRef := findUnresolvedFrom(t, result.Unresolved, "Str", runID)
	if strRef.ReceiverText != "Token" || strRef.ArgumentCount != 1 {
		t.Fatalf("Str call = %#v, want receiver Token and one argument", strRef)
	}
	writeRef := findUnresolvedFrom(t, result.Unresolved, "write_str", runID)
	if writeRef.ReceiverText != "formatter" || writeRef.ArgumentCount != 1 {
		t.Fatalf("write_str call = %#v, want receiver formatter and one argument", writeRef)
	}
}

func TestExtractFromSourceFindsExpandedRustKinds(t *testing.T) {
	source := []byte(`trait Sink {}
struct Profile {}
struct Service {
    profile: Profile,
}
type UserId = String;
const LIMIT: usize = 10;
enum Status { Created, Canceled }
impl Sink for Service {}
fn build() -> Service { Service { profile: Profile {} } }
`)

	result, err := ExtractFromSource("lib.rs", source, model.LanguageRust)
	if err != nil {
		t.Fatal(err)
	}

	serviceID := findNodeID(t, result.Nodes, model.NodeKindStruct, "Service")
	sinkID := findNodeID(t, result.Nodes, model.NodeKindTrait, "Sink")
	fieldID := findNodeID(t, result.Nodes, model.NodeKindField, "profile")
	profileID := findNodeID(t, result.Nodes, model.NodeKindStruct, "Profile")
	buildID := findNodeID(t, result.Nodes, model.NodeKindFunction, "build")
	statusID := findNodeID(t, result.Nodes, model.NodeKindEnum, "Status")
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindTypeAlias, "UserId", model.LanguageRust)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindConstant, "LIMIT", model.LanguageRust)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindEnumMember, "Canceled", model.LanguageRust)
	assertEdge(t, result.Edges, serviceID, sinkID, model.EdgeKindImplements)
	assertEdge(t, result.Edges, fieldID, profileID, model.EdgeKindTypeOf)
	assertEdge(t, result.Edges, buildID, serviceID, model.EdgeKindReturns)
	assertEdge(t, result.Edges, buildID, serviceID, model.EdgeKindInstantiates)
	if statusID == "" {
		t.Fatalf("Status enum ID is empty")
	}
}

func TestExtractFromSourceFindsRustUseImports(t *testing.T) {
	source := []byte(`use serde_test::{assert_de_tokens, assert_ser_tokens, Token};
use crate::de::Error as DeError;

fn run() {
    assert_de_tokens(&Token::Str("value"));
}`)

	result, err := ExtractFromSource("lib.rs", source, model.LanguageRust)
	if err != nil {
		t.Fatal(err)
	}

	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "assert_de_tokens", "serde_test::assert_de_tokens")
	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "assert_ser_tokens", "serde_test::assert_ser_tokens")
	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "Token", "serde_test::Token")
	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "DeError", "crate::de::Error")
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

func TestExtractFromSourceFindsExpandedJavaKinds(t *testing.T) {
	source := []byte(`interface Sink {}
class Base {}
class Service extends Base implements Sink {
  private Profile profile;
  Status status() { return Status.Canceled; }
}
class Profile {}
enum Status { Created, Canceled }
`)

	result, err := ExtractFromSource("Service.java", source, model.LanguageJava)
	if err != nil {
		t.Fatal(err)
	}

	serviceID := findNodeID(t, result.Nodes, model.NodeKindClass, "Service")
	baseID := findNodeID(t, result.Nodes, model.NodeKindClass, "Base")
	sinkID := findNodeID(t, result.Nodes, model.NodeKindInterface, "Sink")
	fieldID := findNodeID(t, result.Nodes, model.NodeKindField, "profile")
	profileID := findNodeID(t, result.Nodes, model.NodeKindClass, "Profile")
	methodID := findNodeID(t, result.Nodes, model.NodeKindMethod, "status")
	statusID := findNodeID(t, result.Nodes, model.NodeKindEnum, "Status")
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindEnumMember, "Canceled", model.LanguageJava)
	assertEdge(t, result.Edges, serviceID, baseID, model.EdgeKindExtends)
	assertEdge(t, result.Edges, serviceID, sinkID, model.EdgeKindImplements)
	assertEdge(t, result.Edges, fieldID, profileID, model.EdgeKindTypeOf)
	assertEdge(t, result.Edges, methodID, statusID, model.EdgeKindReturns)
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

func TestExtractFromSourceNormalizesCSharpCalls(t *testing.T) {
	source := []byte(`class App {
  void Run() {
    serializer.Deserialize<IList<RootObject>>(reader);
    nameof(Run);
    Assert.Equal<string>("a", "b");
    Assert.Throws<JsonException>(() => Run());
    Assert.AreEqual(1, 2);
    StringComparer.OrdinalIgnoreCase.Equals("a", "b");
  }
}`)

	result, err := ExtractFromSource("App.cs", source, model.LanguageCSharp)
	if err != nil {
		t.Fatal(err)
	}
	runID := findNodeID(t, result.Nodes, model.NodeKindMethod, "Run")

	deserialize := findUnresolvedFrom(t, result.Unresolved, "Deserialize", runID)
	if deserialize.ReceiverText != "serializer" {
		t.Fatalf("Deserialize receiver = %q, want serializer", deserialize.ReceiverText)
	}
	assertNoUnresolvedFrom(t, result.Unresolved, "Deserialize<IList<RootObject>>", runID)
	assertNoUnresolvedFrom(t, result.Unresolved, "nameof", runID)

	equal := findUnresolvedFrom(t, result.Unresolved, "Equal", runID)
	if equal.ReceiverText != "Assert" {
		t.Fatalf("Equal receiver = %q, want Assert", equal.ReceiverText)
	}
	throws := findUnresolvedFrom(t, result.Unresolved, "Throws", runID)
	if throws.ReceiverText != "Assert" {
		t.Fatalf("Throws receiver = %q, want Assert", throws.ReceiverText)
	}
	areEqual := findUnresolvedFrom(t, result.Unresolved, "AreEqual", runID)
	if areEqual.ReceiverText != "Assert" {
		t.Fatalf("AreEqual receiver = %q, want Assert", areEqual.ReceiverText)
	}
	equals := findUnresolvedFrom(t, result.Unresolved, "Equals", runID)
	if equals.ReceiverText != "StringComparer.OrdinalIgnoreCase" {
		t.Fatalf("Equals receiver = %q, want StringComparer.OrdinalIgnoreCase", equals.ReceiverText)
	}
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

func TestExtractFromSourceFindsExpandedCSharpKinds(t *testing.T) {
	source := []byte(`interface ISink {}
class Base {}
class Service : Base, ISink {
  private Profile profile;
  public string Name { get; set; }
  public const int Limit = 10;
  public Status Current() { return Status.Canceled; }
}
class Profile {}
enum Status { Created, Canceled }
`)

	result, err := ExtractFromSource("Service.cs", source, model.LanguageCSharp)
	if err != nil {
		t.Fatal(err)
	}

	serviceID := findNodeID(t, result.Nodes, model.NodeKindClass, "Service")
	baseID := findNodeID(t, result.Nodes, model.NodeKindClass, "Base")
	sinkID := findNodeID(t, result.Nodes, model.NodeKindInterface, "ISink")
	fieldID := findNodeID(t, result.Nodes, model.NodeKindField, "profile")
	profileID := findNodeID(t, result.Nodes, model.NodeKindClass, "Profile")
	methodID := findNodeID(t, result.Nodes, model.NodeKindMethod, "Current")
	statusID := findNodeID(t, result.Nodes, model.NodeKindEnum, "Status")
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindProperty, "Name", model.LanguageCSharp)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindConstant, "Limit", model.LanguageCSharp)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindEnumMember, "Canceled", model.LanguageCSharp)
	assertEdge(t, result.Edges, serviceID, baseID, model.EdgeKindExtends)
	assertEdge(t, result.Edges, serviceID, sinkID, model.EdgeKindImplements)
	assertEdge(t, result.Edges, fieldID, profileID, model.EdgeKindTypeOf)
	assertEdge(t, result.Edges, methodID, statusID, model.EdgeKindReturns)
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

func TestExtractFromSourceAttributesKotlinCallsInTrailingLambda(t *testing.T) {
	source := []byte("fun getById(port: Port, id: String) = runCatching { port.save(id) }")

	result, err := ExtractFromSource("Service.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}
	getByIdID := findNodeID(t, result.Nodes, model.NodeKindFunction, "getById")
	assertUnresolvedFrom(t, result.Unresolved, "save", getByIdID)
}

func TestExtractFromSourceSkipsKotlinCallsInsideEscapingLambdas(t *testing.T) {
	source := []byte("fun makeHandler(): () -> Unit = { cleanup() }\nfun cleanup() {}")

	result, err := ExtractFromSource("Handlers.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}
	makeHandlerID := findNodeID(t, result.Nodes, model.NodeKindFunction, "makeHandler")
	assertNoUnresolvedFrom(t, result.Unresolved, "cleanup", makeHandlerID)
}

func TestExtractFromSourceSkipsKotlinCallsInDeferredBuilderLambdas(t *testing.T) {
	source := []byte("fun setup() { launch { doWork() } }")

	result, err := ExtractFromSource("Setup.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}
	setupID := findNodeID(t, result.Nodes, model.NodeKindFunction, "setup")
	assertUnresolvedFrom(t, result.Unresolved, "launch", setupID)
	assertNoUnresolvedFrom(t, result.Unresolved, "doWork", setupID)
}

func TestExtractFromSourceFindsKotlinClassesAndConstructorCalls(t *testing.T) {
	source := []byte(`class NoSuchElementException
annotation class ReplaceWith(val expression: String)
typealias ArrayList<E> = java.util.ArrayList<E>
fun run() {
  NoSuchElementException("missing")
  ReplaceWith("value")
}`)

	result, err := ExtractFromSource("Classes.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}

	assertNodeWithLanguage(t, result.Nodes, model.NodeKindClass, "NoSuchElementException", model.LanguageKotlin)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindClass, "ReplaceWith", model.LanguageKotlin)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindClass, "ArrayList", model.LanguageKotlin)

	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "run")
	exception := findUnresolvedFrom(t, result.Unresolved, "NoSuchElementException", runID)
	if exception.ArgumentCount != 1 {
		t.Fatalf("NoSuchElementException argumentCount = %d, want 1", exception.ArgumentCount)
	}
	replaceWith := findUnresolvedFrom(t, result.Unresolved, "ReplaceWith", runID)
	if replaceWith.ArgumentCount != 1 {
		t.Fatalf("ReplaceWith argumentCount = %d, want 1", replaceWith.ArgumentCount)
	}
}

func TestExtractFromSourceFindsExpandedKotlinKinds(t *testing.T) {
	source := []byte(`interface Sink
open class Base
class Service : Base(), Sink {
  val profile: Profile = Profile()
  fun current(): Status = Status.Canceled
}
class Profile
const val Limit = 10
typealias UserId = String
enum class Status { Created, Canceled }
`)

	result, err := ExtractFromSource("Service.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}

	serviceID := findNodeID(t, result.Nodes, model.NodeKindClass, "Service")
	baseID := findNodeID(t, result.Nodes, model.NodeKindClass, "Base")
	sinkID := findNodeID(t, result.Nodes, model.NodeKindInterface, "Sink")
	propertyID := findNodeID(t, result.Nodes, model.NodeKindProperty, "profile")
	profileID := findNodeID(t, result.Nodes, model.NodeKindClass, "Profile")
	currentID := findNodeID(t, result.Nodes, model.NodeKindFunction, "current")
	statusID := findNodeID(t, result.Nodes, model.NodeKindEnum, "Status")
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindConstant, "Limit", model.LanguageKotlin)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindTypeAlias, "UserId", model.LanguageKotlin)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindEnumMember, "Canceled", model.LanguageKotlin)
	assertEdge(t, result.Edges, serviceID, baseID, model.EdgeKindExtends)
	assertEdge(t, result.Edges, serviceID, sinkID, model.EdgeKindImplements)
	assertEdge(t, result.Edges, propertyID, profileID, model.EdgeKindTypeOf)
	assertEdge(t, result.Edges, currentID, statusID, model.EdgeKindReturns)
}

func TestExtractFromSourceDoesNotDuplicateKotlinInterfacesAsClasses(t *testing.T) {
	source := []byte(`package demo
interface Binding {
  fun bind()
}
`)

	result, err := ExtractFromSource("Binding.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}

	binding := findNode(t, result.Nodes, model.NodeKindInterface, "Binding")
	if binding.QualifiedName != "demo.Binding" {
		t.Fatalf("Binding qualifiedName = %q, want demo.Binding", binding.QualifiedName)
	}
	if binding.EndLine < 4 {
		t.Fatalf("Binding endLine = %d, want full interface range", binding.EndLine)
	}
	for _, node := range result.Nodes {
		if node.Kind == model.NodeKindClass && node.QualifiedName == "demo.Binding" {
			t.Fatalf("unexpected class node for Kotlin interface: %#v", node)
		}
	}
}

func TestExtractFromSourceFindsExpandedTypeScriptKinds(t *testing.T) {
	source := []byte(`interface Sink {}
class Base {}
class Service extends Base implements Sink {
  profile: Profile
  status(): Status { return Status.Canceled }
}
class Profile {}
type UserId = string
enum Status { Created, Canceled }
`)

	result, err := ExtractFromSource("service.ts", source, model.LanguageTypeScript)
	if err != nil {
		t.Fatal(err)
	}

	serviceID := findNodeID(t, result.Nodes, model.NodeKindClass, "Service")
	baseID := findNodeID(t, result.Nodes, model.NodeKindClass, "Base")
	sinkID := findNodeID(t, result.Nodes, model.NodeKindInterface, "Sink")
	propertyID := findNodeID(t, result.Nodes, model.NodeKindProperty, "profile")
	profileID := findNodeID(t, result.Nodes, model.NodeKindClass, "Profile")
	methodID := findNodeID(t, result.Nodes, model.NodeKindMethod, "status")
	statusID := findNodeID(t, result.Nodes, model.NodeKindEnum, "Status")
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindTypeAlias, "UserId", model.LanguageTypeScript)
	assertNodeWithLanguage(t, result.Nodes, model.NodeKindEnumMember, "Canceled", model.LanguageTypeScript)
	assertEdge(t, result.Edges, serviceID, baseID, model.EdgeKindExtends)
	assertEdge(t, result.Edges, serviceID, sinkID, model.EdgeKindImplements)
	assertEdge(t, result.Edges, propertyID, profileID, model.EdgeKindTypeOf)
	assertEdge(t, result.Edges, methodID, statusID, model.EdgeKindReturns)
}

func TestExtractFromSourceFindsKotlinPackageQualifiedNamesAndImports(t *testing.T) {
	source := []byte(`package demo
import kotlin.contracts.*
import kotlin.collections.ArrayList

fun run() { contract { returns() }; ArrayList<String>() }`)

	result, err := ExtractFromSource("Imports.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}

	run := findNode(t, result.Nodes, model.NodeKindFunction, "run")
	if run.QualifiedName != "demo.run" {
		t.Fatalf("run qualifiedName = %q, want demo.run", run.QualifiedName)
	}
	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "*", "kotlin.contracts.*")
	assertQualifiedNode(t, result.Nodes, model.NodeKindImport, "ArrayList", "kotlin.collections.ArrayList")
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
	source := []byte(`fun String.words(limit: Int): List<String> { return split(" ").take(limit) }
fun <T> Array<out T>.isEmpty(): Boolean { return size == 0 }`)

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

	isEmpty := findNode(t, result.Nodes, model.NodeKindFunction, "isEmpty")
	if isEmpty.QualifiedName != "Array<out T>.isEmpty" || isEmpty.ReceiverType != "Array<out T>" {
		t.Fatalf("generic extension node = %#v, want Array<out T>.isEmpty receiver Array<out T>", isEmpty)
	}
}

func TestExtractFromSourceFindsKotlinNavigationExpressionCalls(t *testing.T) {
	source := []byte(`fun run(xs: List<Int>) { xs.isEmpty() }
fun String.dropFirst(): String { return this.substring(1) }`)

	result, err := ExtractFromSource("Navigation.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}

	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "run")
	isEmpty := findUnresolvedFrom(t, result.Unresolved, "isEmpty", runID)
	if isEmpty.ReceiverText != "xs" {
		t.Fatalf("isEmpty receiver = %q, want xs", isEmpty.ReceiverText)
	}
	if isEmpty.ArgumentCount != 0 {
		t.Fatalf("isEmpty argumentCount = %d, want 0", isEmpty.ArgumentCount)
	}

	dropID := findNodeID(t, result.Nodes, model.NodeKindFunction, "dropFirst")
	substring := findUnresolvedFrom(t, result.Unresolved, "substring", dropID)
	if substring.ReceiverText != "this" {
		t.Fatalf("substring receiver = %q, want this", substring.ReceiverText)
	}
	if substring.ArgumentCount != 1 || !reflect.DeepEqual(substring.ArgumentTexts, []string{"1"}) {
		t.Fatalf("substring arguments = %d %#v, want 1 [1]", substring.ArgumentCount, substring.ArgumentTexts)
	}
}

func TestExtractFromSourceCountsKotlinTrailingLambdaAsArgument(t *testing.T) {
	source := []byte(`fun contract(builder: ContractBuilder.() -> Unit) {}
fun run() { contract { returns() } }`)

	result, err := ExtractFromSource("Contracts.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}

	runID := findNodeID(t, result.Nodes, model.NodeKindFunction, "run")
	contract := findUnresolvedFrom(t, result.Unresolved, "contract", runID)
	if contract.ArgumentCount != 1 || !reflect.DeepEqual(contract.ArgumentTexts, []string{"{ returns() }"}) {
		t.Fatalf("contract arguments = %d %#v, want 1 [{ returns() }]", contract.ArgumentCount, contract.ArgumentTexts)
	}
}

func TestExtractFromSourceFiltersKotlinFunctionTypedParameterCalls(t *testing.T) {
	source := []byte(`fun <T> visit(items: Array<T>, predicate: (T) -> Boolean, action: () -> Unit, count: Int) {
  if (predicate(items[0])) action()
  helper(count)
}
fun helper(value: Int) {}`)

	result, err := ExtractFromSource("Callbacks.kt", source, model.LanguageKotlin)
	if err != nil {
		t.Fatal(err)
	}

	visitID := findNodeID(t, result.Nodes, model.NodeKindFunction, "visit")
	assertNoUnresolvedFrom(t, result.Unresolved, "predicate", visitID)
	assertNoUnresolvedFrom(t, result.Unresolved, "action", visitID)
	assertUnresolvedFrom(t, result.Unresolved, "helper", visitID)
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

func TestExtractFromSourceAddsContainmentAcrossLanguages(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		language   model.Language
		source     string
		parentKind model.NodeKind
		parentName string
		childKind  model.NodeKind
		childName  string
	}{
		{
			name:       "java nested class",
			path:       "Outer.java",
			language:   model.LanguageJava,
			source:     "package demo;\nclass Outer {\n  class Inner {}\n  void run() {}\n}\n",
			parentKind: model.NodeKindClass,
			parentName: "Outer",
			childKind:  model.NodeKindClass,
			childName:  "Inner",
		},
		{
			name:       "kotlin property",
			path:       "Service.kt",
			language:   model.LanguageKotlin,
			source:     "package demo\nclass Service {\n  val profile: Profile = Profile()\n}\nclass Profile\n",
			parentKind: model.NodeKindClass,
			parentName: "Service",
			childKind:  model.NodeKindProperty,
			childName:  "profile",
		},
		{
			name:       "csharp nested class",
			path:       "Service.cs",
			language:   model.LanguageCSharp,
			source:     "namespace Demo;\nclass Service {\n  class Nested {}\n  void Run() {}\n}\n",
			parentKind: model.NodeKindClass,
			parentName: "Service",
			childKind:  model.NodeKindClass,
			childName:  "Nested",
		},
		{
			name:       "typescript property",
			path:       "service.ts",
			language:   model.LanguageTypeScript,
			source:     "class Service {\n  profile: Profile\n  run(): void {}\n}\nclass Profile {}\n",
			parentKind: model.NodeKindClass,
			parentName: "Service",
			childKind:  model.NodeKindProperty,
			childName:  "profile",
		},
		{
			name:       "python nested class",
			path:       "service.py",
			language:   model.LanguagePython,
			source:     "class Service:\n    class Nested:\n        pass\n    def run(self):\n        pass\n",
			parentKind: model.NodeKindClass,
			parentName: "Service",
			childKind:  model.NodeKindClass,
			childName:  "Nested",
		},
		{
			name:       "rust field",
			path:       "service.rs",
			language:   model.LanguageRust,
			source:     "struct Service {\n    profile: Profile,\n}\nstruct Profile {}\nimpl Service {\n    fn run(&self) {}\n}\n",
			parentKind: model.NodeKindStruct,
			parentName: "Service",
			childKind:  model.NodeKindField,
			childName:  "profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractFromSource(tt.path, []byte(tt.source), tt.language)
			if err != nil {
				t.Fatal(err)
			}
			fileID := findNodeID(t, result.Nodes, model.NodeKindFile, tt.path)
			parentID := findNodeID(t, result.Nodes, tt.parentKind, tt.parentName)
			childID := findNodeID(t, result.Nodes, tt.childKind, tt.childName)

			assertContainedByFileRoot(t, result.Edges, fileID, parentID)
			assertEdge(t, result.Edges, parentID, childID, model.EdgeKindContains)
		})
	}
}

func TestExtractFromSourceDoesNotContainExternalNodes(t *testing.T) {
	source := []byte("package demo\nfunc run() { println(\"ok\") }\n")

	result, err := ExtractFromSource("main.go", source, model.LanguageGo)
	if err != nil {
		t.Fatal(err)
	}

	for _, edge := range result.Edges {
		if edge.Kind != model.EdgeKindContains {
			continue
		}
		for _, node := range result.Nodes {
			if node.ID == edge.TargetNodeID && node.Kind == model.NodeKindExternal {
				t.Fatalf("external node %s has contains edge %#v", node.QualifiedName, edge)
			}
		}
	}
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

func assertNoNode(t *testing.T, nodes []model.GraphNode, kind model.NodeKind, name string) {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			t.Fatalf("unexpected node %s %s found in %#v", kind, name, nodes)
		}
	}
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

func assertContainedByFileRoot(t *testing.T, edges []model.GraphEdge, fileID, nodeID string) {
	t.Helper()
	for _, edge := range edges {
		if edge.SourceNodeID == fileID && edge.TargetNodeID == nodeID && edge.Kind == model.EdgeKindContains {
			return
		}
	}
	for _, rootEdge := range edges {
		if rootEdge.SourceNodeID != fileID || rootEdge.Kind != model.EdgeKindContains {
			continue
		}
		for _, childEdge := range edges {
			if childEdge.SourceNodeID == rootEdge.TargetNodeID && childEdge.TargetNodeID == nodeID && childEdge.Kind == model.EdgeKindContains {
				return
			}
		}
	}
	t.Fatalf("node %s is not contained by file root %s in %#v", nodeID, fileID, edges)
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
