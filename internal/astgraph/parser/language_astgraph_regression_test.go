package parser

import (
	"testing"

	"repobridge/internal/astgraph/model"
)

func TestExtractFromSourceMapsCrossLanguageRouteRegressionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		language    model.Language
		source      string
		route       string
		handler     string
		edge        model.EdgeKind
		handlerKind model.NodeKind
	}{
		{
			name:        "javascript express",
			path:        "routes.js",
			language:    model.LanguageJavaScript,
			source:      "function login(req, res) {}\napp.post('/login', login)",
			route:       "POST /login",
			handler:     "login",
			edge:        model.EdgeKindHandles,
			handlerKind: model.NodeKindHandler,
		},
		{
			name:        "typescript express",
			path:        "routes.ts",
			language:    model.LanguageTypeScript,
			source:      "function getUser(req: Request, res: Response): void {}\nrouter.get('/users/:id', getUser)",
			route:       "GET /users/:id",
			handler:     "getUser",
			edge:        model.EdgeKindHandles,
			handlerKind: model.NodeKindHandler,
		},
		{
			name:        "python decorator",
			path:        "views.py",
			language:    model.LanguagePython,
			source:      "@app.post('/login')\ndef login_view():\n    pass\n",
			route:       "POST /login",
			handler:     "login_view",
			edge:        model.EdgeKindHandles,
			handlerKind: model.NodeKindHandler,
		},
		{
			name:        "rust attribute",
			path:        "routes.rs",
			language:    model.LanguageRust,
			source:      "#[get(\"/health\")]\nasync fn health_check() {}\n",
			route:       "GET /health",
			handler:     "health_check",
			edge:        model.EdgeKindHandles,
			handlerKind: model.NodeKindHandler,
		},
		{
			name:     "java spring",
			path:     "AuthController.java",
			language: model.LanguageJava,
			source: `@RestController
@RequestMapping("/api")
class AuthController {
  @PostMapping("/login")
  public void login() {}
}`,
			route:       "POST /api/login",
			handler:     "login",
			edge:        model.EdgeKindHandles,
			handlerKind: model.NodeKindHandler,
		},
		{
			name:     "kotlin spring",
			path:     "AuthController.kt",
			language: model.LanguageKotlin,
			source: `@RestController
@RequestMapping("/api")
class AuthController {
  @GetMapping("/users/{id}")
  fun user() {}
}`,
			route:       "GET /api/users/{id}",
			handler:     "user",
			edge:        model.EdgeKindHandles,
			handlerKind: model.NodeKindHandler,
		},
		{
			name:     "csharp aspnet",
			path:     "UsersController.cs",
			language: model.LanguageCSharp,
			source: `[Route("api/[controller]")]
public class UsersController {
  [HttpGet("{id}")]
  public IActionResult Get(int id) { return Ok(); }
}`,
			route:       "GET /api/users/{id}",
			handler:     "Get",
			edge:        model.EdgeKindHandles,
			handlerKind: model.NodeKindHandler,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractFromSource(tt.path, []byte(tt.source), tt.language)
			if err != nil {
				t.Fatal(err)
			}

			routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, tt.route)
			handlerID := findNodeID(t, result.Nodes, tt.handlerKind, tt.handler)
			assertEdge(t, result.Edges, routeID, handlerID, tt.edge)
		})
	}
}

func TestExtractFromSourceMapsCrossLanguageTypeRelationshipRegressionMatrix(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		language   model.Language
		source     string
		sourceKind model.NodeKind
		sourceName string
		targetKind model.NodeKind
		targetName string
		edge       model.EdgeKind
	}{
		{
			name:       "typescript property type",
			path:       "service.ts",
			language:   model.LanguageTypeScript,
			source:     "class Profile {}\nclass Service {\n  profile: Profile\n}\n",
			sourceKind: model.NodeKindProperty,
			sourceName: "profile",
			targetKind: model.NodeKindClass,
			targetName: "Profile",
			edge:       model.EdgeKindTypeOf,
		},
		{
			name:       "rust trait implementation",
			path:       "lib.rs",
			language:   model.LanguageRust,
			source:     "trait Sink {}\nstruct Service {}\nimpl Sink for Service {}\n",
			sourceKind: model.NodeKindStruct,
			sourceName: "Service",
			targetKind: model.NodeKindTrait,
			targetName: "Sink",
			edge:       model.EdgeKindImplements,
		},
		{
			name:       "java class extends",
			path:       "Service.java",
			language:   model.LanguageJava,
			source:     "class Base {}\nclass Service extends Base {}\n",
			sourceKind: model.NodeKindClass,
			sourceName: "Service",
			targetKind: model.NodeKindClass,
			targetName: "Base",
			edge:       model.EdgeKindExtends,
		},
		{
			name:       "kotlin property type",
			path:       "Service.kt",
			language:   model.LanguageKotlin,
			source:     "class Profile\nclass Service {\n  val profile: Profile = Profile()\n}\n",
			sourceKind: model.NodeKindProperty,
			sourceName: "profile",
			targetKind: model.NodeKindClass,
			targetName: "Profile",
			edge:       model.EdgeKindTypeOf,
		},
		{
			name:       "csharp interface implementation",
			path:       "Service.cs",
			language:   model.LanguageCSharp,
			source:     "interface ISink {}\nclass Service : ISink {}\n",
			sourceKind: model.NodeKindClass,
			sourceName: "Service",
			targetKind: model.NodeKindInterface,
			targetName: "ISink",
			edge:       model.EdgeKindImplements,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractFromSource(tt.path, []byte(tt.source), tt.language)
			if err != nil {
				t.Fatal(err)
			}

			sourceID := findNodeID(t, result.Nodes, tt.sourceKind, tt.sourceName)
			targetID := findNodeID(t, result.Nodes, tt.targetKind, tt.targetName)
			assertEdge(t, result.Edges, sourceID, targetID, tt.edge)
		})
	}
}

func TestExtractFromSourceFixesDocumentedOpenLanguageGaps(t *testing.T) {
	t.Run("javascript exported express router alias", func(t *testing.T) {
		source := []byte(`import { Router } from "express"
const api = Router()
function listUsers(req, res) {}
api.get("/users", listUsers)
export default api`)

		result, err := ExtractFromSource("users.js", source, model.LanguageJavaScript)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "listUsers")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})

	t.Run("typescript nestjs decorator controller", func(t *testing.T) {
		source := []byte(`@Controller('/users')
class UsersController {
  @Get(':id')
  getUser() {}
}`)

		result, err := ExtractFromSource("users.controller.ts", source, model.LanguageTypeScript)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users/:id")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "getUser")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})

	t.Run("typescript nestjs constants and path arrays", func(t *testing.T) {
		source := []byte(`const BASE = '/users'

@Controller(BASE)
class UsersController {
  @Get([':id', 'me'])
  getUser() {}
}`)

		result, err := ExtractFromSource("users.controller.ts", source, model.LanguageTypeScript)
		if err != nil {
			t.Fatal(err)
		}

		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "getUser")
		idRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users/:id")
		assertEdge(t, result.Edges, idRouteID, handlerID, model.EdgeKindHandles)
		meRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users/me")
		assertEdge(t, result.Edges, meRouteID, handlerID, model.EdgeKindHandles)
	})

	t.Run("typescript nestjs guards interceptors and multiline route decorator", func(t *testing.T) {
		source := []byte(`@Controller('users')
@UseGuards(AuthGuard)
class UsersController {
  @UseGuards(AuthGuard)
  @Get(
    ':id',
  )
  @UseInterceptors(ClassSerializerInterceptor)
  async getUser(@Param('id') id: string) {}
}`)

		result, err := ExtractFromSource("users.controller.ts", source, model.LanguageTypeScript)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users/:id")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "getUser")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})

	t.Run("typescript nestjs controller object path and method arrays", func(t *testing.T) {
		source := []byte(`@Controller({ version: '1', path: 'users' })
class UsersController {
  @Get([':id', 'me'])
  getUser() {}
}`)

		result, err := ExtractFromSource("users.controller.ts", source, model.LanguageTypeScript)
		if err != nil {
			t.Fatal(err)
		}

		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "getUser")
		idRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users/:id")
		assertEdge(t, result.Edges, idRouteID, handlerID, model.EdgeKindHandles)
		meRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users/me")
		assertEdge(t, result.Edges, meRouteID, handlerID, model.EdgeKindHandles)
		assertNoNode(t, result.Nodes, model.NodeKindRoute, "GET /1/:id")
		assertNoNode(t, result.Nodes, model.NodeKindRoute, "GET /1/me")
	})

	t.Run("typescript nestjs controller object version without path", func(t *testing.T) {
		source := []byte(`@Controller({ version: '1' })
class HealthController {
  @Get('health')
  health() {}
}`)

		result, err := ExtractFromSource("health.controller.ts", source, model.LanguageTypeScript)
		if err != nil {
			t.Fatal(err)
		}

		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "health")
		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /health")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
		assertNoNode(t, result.Nodes, model.NodeKindRoute, "GET /1/health")
	})

	t.Run("typescript nestjs method object path array", func(t *testing.T) {
		source := []byte(`@Controller('users')
class UsersController {
  @Get({ path: [':id', 'me'] })
  getUser() {}
}`)

		result, err := ExtractFromSource("users.controller.ts", source, model.LanguageTypeScript)
		if err != nil {
			t.Fatal(err)
		}

		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "getUser")
		idRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users/:id")
		assertEdge(t, result.Edges, idRouteID, handlerID, model.EdgeKindHandles)
		meRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users/me")
		assertEdge(t, result.Edges, meRouteID, handlerID, model.EdgeKindHandles)
	})

	t.Run("python flask class based view", func(t *testing.T) {
		source := []byte(`class UserView:
    pass

app.add_url_rule('/users', view_func=UserView.as_view('users'))`)

		result, err := ExtractFromSource("views.py", source, model.LanguagePython)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "ANY /users")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "UserView")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})

	t.Run("python fastapi apirouter prefix", func(t *testing.T) {
		source := []byte(`from fastapi import APIRouter

router = APIRouter(prefix="/api")

@router.get("/users")
def list_users():
    pass`)

		result, err := ExtractFromSource("users.py", source, model.LanguagePython)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /api/users")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "list_users")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})

	t.Run("rust nested axum router", func(t *testing.T) {
		source := []byte(`fn app() {
    Router::new().nest("/api", Router::new().route("/users", get(list_users)));
}`)

		result, err := ExtractFromSource("routes.rs", source, model.LanguageRust)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /api/users")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "list_users")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})

	t.Run("rust axum method router chain", func(t *testing.T) {
		source := []byte(`fn app() {
    Router::new().route("/users", get(list_users).post(create_user));
}`)

		result, err := ExtractFromSource("routes.rs", source, model.LanguageRust)
		if err != nil {
			t.Fatal(err)
		}

		getRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users")
		listHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "list_users")
		assertEdge(t, result.Edges, getRouteID, listHandlerID, model.EdgeKindHandles)

		postRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "POST /users")
		createHandlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "create_user")
		assertEdge(t, result.Edges, postRouteID, createHandlerID, model.EdgeKindHandles)
	})

	t.Run("java spring composed annotation", func(t *testing.T) {
		source := []byte(`@GetMapping("/health")
@interface HealthEndpoint {}

class HealthController {
  @HealthEndpoint
  public void health() {}
}`)

		result, err := ExtractFromSource("HealthController.java", source, model.LanguageJava)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /health")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "health")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})

	t.Run("java spring request mapping method array", func(t *testing.T) {
		source := []byte(`class UsersController {
  @RequestMapping(value="/users", method={RequestMethod.GET, RequestMethod.POST})
  public void users() {}
}`)

		result, err := ExtractFromSource("UsersController.java", source, model.LanguageJava)
		if err != nil {
			t.Fatal(err)
		}

		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "users")
		getRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /users")
		assertEdge(t, result.Edges, getRouteID, handlerID, model.EdgeKindHandles)
		postRouteID := findNodeID(t, result.Nodes, model.NodeKindRoute, "POST /users")
		assertEdge(t, result.Edges, postRouteID, handlerID, model.EdgeKindHandles)
	})

	t.Run("kotlin deferred coroutine builder does not attribute call", func(t *testing.T) {
		source := []byte("fun setup() { launch { doWork() } }")

		result, err := ExtractFromSource("Setup.kt", source, model.LanguageKotlin)
		if err != nil {
			t.Fatal(err)
		}

		setupID := findNodeID(t, result.Nodes, model.NodeKindFunction, "setup")
		assertNoUnresolvedFrom(t, result.Unresolved, "doWork", setupID)
	})

	t.Run("kotlin ktor nested route dsl", func(t *testing.T) {
		source := []byte(`fun Application.module() {
    routing {
        route("/api") {
            get("/users") {
                listUsers()
            }
        }
    }
}

fun listUsers() {}
`)

		result, err := ExtractFromSource("Routes.kt", source, model.LanguageKotlin)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /api/users")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "listUsers")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})

	t.Run("csharp minimal api route group", func(t *testing.T) {
		source := []byte(`var api = app.MapGroup("/api");
api.MapGet("/users", ListUsers);`)

		result, err := ExtractFromSource("Program.cs", source, model.LanguageCSharp)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /api/users")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "ListUsers")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})

	t.Run("csharp nested minimal api route groups", func(t *testing.T) {
		source := []byte(`var api = app.MapGroup("/api");
var v1 = api.MapGroup("/v1");
v1.MapGet("/users", ListUsers);`)

		result, err := ExtractFromSource("Program.cs", source, model.LanguageCSharp)
		if err != nil {
			t.Fatal(err)
		}

		routeID := findNodeID(t, result.Nodes, model.NodeKindRoute, "GET /api/v1/users")
		handlerID := findNodeID(t, result.Nodes, model.NodeKindHandler, "ListUsers")
		assertEdge(t, result.Edges, routeID, handlerID, model.EdgeKindHandles)
	})
}
