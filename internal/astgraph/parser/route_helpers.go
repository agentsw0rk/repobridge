package parser

import (
	"fmt"
	"regexp"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	"repobridge/internal/astgraph/model"
)

type frameworkRoute struct {
	Framework   string
	Method      string
	Pattern     string
	Kind        model.NodeKind
	EdgeKind    model.EdgeKind
	HandlerName string
	Line        int
	Column      int
}

func appendFrameworkRoute(path string, result *ExtractionResult, language model.Language, route frameworkRoute) string {
	pattern := normalizeRoutePattern(route.Pattern)
	name := pattern
	if route.Method != "" {
		name = strings.TrimSpace(strings.ToUpper(route.Method) + " " + pattern)
	}
	kind := route.Kind
	if kind == "" {
		kind = model.NodeKindRoute
	}
	edgeKind := route.EdgeKind
	if edgeKind == "" {
		edgeKind = model.EdgeKindHandles
	}
	id := stableNodeID(path, kind, route.Framework+" "+name, route.Line)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            id,
		Kind:          kind,
		Name:          name,
		QualifiedName: strings.TrimSpace(route.Framework + " " + name),
		FilePath:      path,
		Language:      language,
		StartLine:     route.Line,
		EndLine:       route.Line,
		StartColumn:   route.Column,
		EndColumn:     route.Column,
		Signature:     frameworkRouteSignature(route, name),
	})

	handler := strings.TrimSpace(route.HandlerName)
	if handler == "" {
		return id
	}
	handlerID := stableNodeID(path, model.NodeKindHandler, handler, route.Line)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            handlerID,
		Kind:          model.NodeKindHandler,
		Name:          handler,
		QualifiedName: handler,
		FilePath:      path,
		Language:      language,
		StartLine:     route.Line,
		EndLine:       route.Line,
		StartColumn:   route.Column,
		EndColumn:     route.Column,
		Signature:     fmt.Sprintf("%s handler %s", route.Framework, handler),
	})
	result.Edges = append(result.Edges, model.GraphEdge{
		SourceNodeID: id,
		TargetNodeID: handlerID,
		Kind:         edgeKind,
		FilePath:     path,
		Line:         route.Line,
		Column:       route.Column,
		Provenance:   route.Framework,
	})
	return id
}

func frameworkRouteSignature(route frameworkRoute, name string) string {
	switch route.Kind {
	case model.NodeKindComponentRoute:
		if route.HandlerName != "" {
			return fmt.Sprintf("%s component route %s -> %s", route.Framework, name, route.HandlerName)
		}
		return fmt.Sprintf("%s component route %s", route.Framework, name)
	default:
		if route.HandlerName != "" {
			return fmt.Sprintf("%s route %s -> %s", route.Framework, name, route.HandlerName)
		}
		return fmt.Sprintf("%s route %s", route.Framework, name)
	}
}

func normalizeRoutePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.Trim(pattern, "\"'`")
	if pattern == "" {
		return "/"
	}
	if strings.HasPrefix(pattern, "^") {
		return pattern
	}
	if !strings.HasPrefix(pattern, "/") {
		return "/" + pattern
	}
	return pattern
}

func routeLineColumn(node *tree_sitter.Node) (int, int) {
	start := node.StartPosition()
	return int(start.Row) + 1, int(start.Column)
}

func namedChildByKind(node *tree_sitter.Node, kind string) *tree_sitter.Node {
	if node == nil {
		return nil
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == kind {
			return child
		}
	}
	return nil
}

func routeArgumentNodes(node *tree_sitter.Node) []*tree_sitter.Node {
	for _, kind := range []string{"arguments", "argument_list"} {
		args := namedChildByKind(node, kind)
		if args == nil {
			continue
		}
		values := make([]*tree_sitter.Node, 0, args.NamedChildCount())
		for i := uint(0); i < args.NamedChildCount(); i++ {
			child := args.NamedChild(i)
			if child.Kind() == "argument" && child.NamedChildCount() == 1 {
				child = child.NamedChild(0)
			}
			values = append(values, child)
		}
		return values
	}
	return nil
}

func firstStringValue(source []byte, node *tree_sitter.Node) (string, bool) {
	values := stringValues(source, node)
	if len(values) == 0 {
		return "", false
	}
	return values[0], true
}

func firstStringArgument(source []byte, args []*tree_sitter.Node) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	return firstStringValue(source, args[0])
}

func firstQuotedText(text string) string {
	values := quotedTexts(text)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func quotedTexts(text string) []string {
	var values []string
	for _, quote := range []string{"\"", "'", "`"} {
		rest := text
		for {
			start := strings.Index(rest, quote)
			if start < 0 {
				break
			}
			after := rest[start+len(quote):]
			end := strings.Index(after, quote)
			if end < 0 {
				break
			}
			values = append(values, after[:end])
			rest = after[end+len(quote):]
		}
	}
	return values
}

func referenceArgumentName(source []byte, args []*tree_sitter.Node, index int) string {
	if index < 0 || index >= len(args) {
		return ""
	}
	_, name, _, ok := referenceName(source, args[index])
	if ok {
		return name
	}
	if args[index].Kind() == "call_expression" || args[index].Kind() == "call" || args[index].Kind() == "invocation_expression" {
		call, ok := callReference(source, args[index])
		if ok {
			return call.name
		}
	}
	return jsxElementName(source, args[index])
}

func routeMemberCall(source []byte, node *tree_sitter.Node) (receiver, method string, ok bool) {
	function := node.ChildByFieldName("function")
	if function == nil && node.NamedChildCount() > 0 {
		function = node.NamedChild(0)
	}
	if function == nil {
		return "", "", false
	}
	for _, field := range []string{"property", "field", "name"} {
		child := function.ChildByFieldName(field)
		if child != nil {
			method = strings.TrimSpace(nodeText(source, child))
			break
		}
	}
	if method == "" && function.NamedChildCount() > 0 {
		last := function.NamedChild(function.NamedChildCount() - 1)
		if last.Kind() == "property_identifier" || last.Kind() == "field_identifier" || last.Kind() == "identifier" {
			method = strings.TrimSpace(nodeText(source, last))
		}
	}
	if function.NamedChildCount() > 0 {
		first := function.NamedChild(0)
		if first.Kind() == "identifier" || first.Kind() == "package_identifier" {
			receiver = strings.TrimSpace(nodeText(source, first))
		}
	}
	return receiver, method, method != ""
}

func httpMethodFromName(name string) (string, bool) {
	switch strings.ToLower(name) {
	case "get":
		return "GET", true
	case "post":
		return "POST", true
	case "put":
		return "PUT", true
	case "delete":
		return "DELETE", true
	case "patch":
		return "PATCH", true
	case "head":
		return "HEAD", true
	case "options":
		return "OPTIONS", true
	default:
		return "", false
	}
}

func jsxElementName(source []byte, node *tree_sitter.Node) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "jsx_self_closing_element", "jsx_opening_element":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child.Kind() == "identifier" {
				return strings.TrimSpace(nodeText(source, child))
			}
		}
	case "jsx_expression":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			if name := jsxElementName(source, node.NamedChild(i)); name != "" {
				return name
			}
			if _, ref, _, ok := referenceName(source, node.NamedChild(i)); ok {
				return ref
			}
		}
	}
	return ""
}

func appendJavaScriptRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, language model.Language) bool {
	_, methodName, ok := routeMemberCall(source, node)
	if !ok {
		return false
	}
	args := routeArgumentNodes(node)
	if len(args) == 0 {
		return false
	}
	line, column := routeLineColumn(node)
	if strings.EqualFold(methodName, "use") {
		pattern := "/"
		handlerIndex := 0
		if value, ok := firstStringArgument(source, args); ok {
			pattern = value
			handlerIndex = 1
		}
		handler := referenceArgumentName(source, args, handlerIndex)
		appendFrameworkRoute(path, result, language, frameworkRoute{
			Framework:   "express",
			Method:      "USE",
			Pattern:     pattern,
			EdgeKind:    model.EdgeKindMiddleware,
			HandlerName: handler,
			Line:        line,
			Column:      column,
		})
		return true
	}
	method, ok := httpMethodFromName(methodName)
	if !ok {
		return false
	}
	pattern, ok := firstStringArgument(source, args)
	if !ok {
		return false
	}
	appendFrameworkRoute(path, result, language, frameworkRoute{
		Framework:   "express",
		Method:      method,
		Pattern:     pattern,
		HandlerName: referenceArgumentName(source, args, 1),
		Line:        line,
		Column:      column,
	})
	return true
}

func appendTypeScriptDecoratorRoutes(path string, source []byte, result *ExtractionResult) {
	lines := strings.Split(string(source), "\n")
	constants := typeScriptStringConstants(lines)
	classPrefix := ""
	var pendingRoutes []springRoute
	for index := 0; index < len(lines); index++ {
		raw := lines[index]
		lineNumber := index + 1
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "@Controller") {
			decoratorText, endIndex := typeScriptDecoratorText(lines, index)
			classPrefix = typescriptDecoratorPatterns(decoratorText, constants)[0]
			index = endIndex
			continue
		}
		if strings.HasPrefix(trimmed, "@") {
			decoratorText, endIndex := typeScriptDecoratorText(lines, index)
			if method, patterns, ok := typescriptHTTPDecorator(decoratorText, constants); ok {
				pendingRoutes = pendingRoutes[:0]
				for _, pattern := range patterns {
					pendingRoutes = append(pendingRoutes, springRoute{
						Method:  method,
						Pattern: combineRoutePatterns(normalizeRoutePattern(classPrefix), normalizeRoutePattern(pattern)),
						Line:    lineNumber,
						Column:  strings.Index(raw, "@"),
					})
				}
			}
			index = endIndex
			continue
		}
		if len(pendingRoutes) == 0 || !strings.Contains(trimmed, "(") {
			continue
		}
		name := strings.TrimSpace(strings.SplitN(trimmed, "(", 2)[0])
		fields := strings.Fields(name)
		if len(fields) > 0 {
			name = fields[len(fields)-1]
		}
		if name == "" || strings.ContainsAny(name, "=@") {
			continue
		}
		appendSpringHandlerLineRoutes(path, result, model.LanguageTypeScript, name, pendingRoutes)
		pendingRoutes = nil
	}
}

func typeScriptDecoratorText(lines []string, start int) (string, int) {
	text := strings.TrimSpace(lines[start])
	balance := strings.Count(text, "(") - strings.Count(text, ")")
	if balance <= 0 {
		return text, start
	}
	parts := []string{text}
	for index := start + 1; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		parts = append(parts, trimmed)
		balance += strings.Count(trimmed, "(") - strings.Count(trimmed, ")")
		if balance <= 0 {
			return strings.Join(parts, " "), index
		}
	}
	return text, start
}

func typeScriptStringConstants(lines []string) map[string]string {
	constants := map[string]string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "const ") || !strings.Contains(trimmed, "=") {
			continue
		}
		left, right, _ := strings.Cut(strings.TrimPrefix(trimmed, "const "), "=")
		name := strings.TrimSpace(left)
		if i := strings.IndexAny(name, ": "); i >= 0 {
			name = strings.TrimSpace(name[:i])
		}
		value := firstQuotedText(right)
		if name != "" && value != "" {
			constants[name] = value
		}
	}
	return constants
}

func typescriptHTTPDecorator(text string, constants map[string]string) (string, []string, bool) {
	open := strings.Index(text, "(")
	name := strings.TrimPrefix(strings.TrimSpace(text), "@")
	if open >= 0 {
		name = strings.TrimSpace(strings.TrimPrefix(text[:open], "@"))
	}
	method, ok := httpMethodFromName(name)
	if !ok {
		return "", nil, false
	}
	return method, typescriptDecoratorPatterns(text, constants), true
}

func typescriptDecoratorPatterns(text string, constants map[string]string) []string {
	open := strings.Index(text, "(")
	close := strings.LastIndex(text, ")")
	if open < 0 || close <= open {
		return []string{""}
	}
	argument := strings.TrimSpace(text[open+1 : close])
	if values := typescriptObjectDecoratorPathValues(argument, constants); len(values) > 0 {
		return values
	}
	if value := constants[argument]; value != "" {
		return []string{value}
	}
	if values := quotedTexts(argument); len(values) > 0 {
		return values
	}
	return []string{""}
}

var typeScriptObjectPathPattern = regexp.MustCompile(`(?s)(?:^|[,{\s])(path|value)\s*:\s*(\[[^\]]*\]|"[^"]*"|'[^']*'|` + "`[^`]*`" + `|[A-Za-z_$][A-Za-z0-9_$]*)`)

func typescriptObjectDecoratorPathValues(argument string, constants map[string]string) []string {
	argument = strings.TrimSpace(argument)
	if !strings.HasPrefix(argument, "{") || !strings.HasSuffix(argument, "}") {
		return nil
	}
	var values []string
	for _, match := range typeScriptObjectPathPattern.FindAllStringSubmatch(argument, -1) {
		if len(match) < 3 {
			continue
		}
		value := strings.TrimSpace(match[2])
		if constant := constants[value]; constant != "" {
			values = append(values, constant)
			continue
		}
		values = append(values, quotedTexts(value)...)
	}
	if len(values) == 0 {
		return []string{""}
	}
	return uniqueStrings(values)
}

func appendSpringHandlerLine(path string, result *ExtractionResult, language model.Language, handlerName string, route springRoute) {
	appendSpringHandlerLineRoutes(path, result, language, handlerName, []springRoute{route})
}

func appendSpringHandlerLineRoutes(path string, result *ExtractionResult, language model.Language, handlerName string, routes []springRoute) {
	if len(routes) == 0 {
		return
	}
	route := routes[0]
	handlerID := stableNodeID(path, model.NodeKindHandler, handlerName, route.Line)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            handlerID,
		Kind:          model.NodeKindHandler,
		Name:          handlerName,
		QualifiedName: handlerName,
		FilePath:      path,
		Language:      language,
		StartLine:     route.Line,
		EndLine:       route.Line,
		StartColumn:   route.Column,
		EndColumn:     route.Column,
		Signature:     fmt.Sprintf("decorated handler %s", handlerName),
	})
	for _, route := range routes {
		appendSpringRoute(path, result, language, route, handlerID, handlerName)
	}
}

func appendReactRouterJSXRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, language model.Language) bool {
	if jsxElementName(source, node) != "Route" {
		return false
	}
	var pattern, handler string
	for i := uint(0); i < node.NamedChildCount(); i++ {
		attr := node.NamedChild(i)
		if attr.Kind() != "jsx_attribute" || attr.NamedChildCount() < 2 {
			continue
		}
		name := strings.TrimSpace(nodeText(source, attr.NamedChild(0)))
		value := attr.NamedChild(1)
		switch name {
		case "path":
			pattern, _ = firstStringValue(source, value)
		case "Component", "component", "element":
			if ref := jsxElementName(source, value); ref != "" {
				handler = ref
			} else if _, ref, _, ok := referenceName(source, value); ok {
				handler = ref
			}
		}
	}
	if pattern == "" {
		return false
	}
	line, column := routeLineColumn(node)
	appendFrameworkRoute(path, result, language, frameworkRoute{
		Framework:   "react-router",
		Pattern:     pattern,
		Kind:        model.NodeKindComponentRoute,
		EdgeKind:    model.EdgeKindRoutesTo,
		HandlerName: handler,
		Line:        line,
		Column:      column,
	})
	return true
}

func appendReactRouterObjectRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, language model.Language) bool {
	var pattern, handler string
	for i := uint(0); i < node.NamedChildCount(); i++ {
		pair := node.NamedChild(i)
		if pair.Kind() != "pair" || pair.NamedChildCount() < 2 {
			continue
		}
		key := strings.TrimSpace(nodeText(source, pair.NamedChild(0)))
		value := pair.NamedChild(1)
		switch key {
		case "path":
			pattern, _ = firstStringValue(source, value)
		case "Component", "component":
			_, handler, _, _ = referenceName(source, value)
		case "element":
			handler = jsxElementName(source, value)
		}
	}
	if pattern == "" || handler == "" {
		return false
	}
	line, column := routeLineColumn(node)
	appendFrameworkRoute(path, result, language, frameworkRoute{
		Framework:   "react-router",
		Pattern:     pattern,
		Kind:        model.NodeKindComponentRoute,
		EdgeKind:    model.EdgeKindRoutesTo,
		HandlerName: handler,
		Line:        line,
		Column:      column,
	})
	return true
}

func appendPythonDecoratedRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) (string, bool) {
	function := namedChildByKind(node, "function_definition")
	if function == nil {
		return "", false
	}
	handlerName := declarationName(source, function)
	if handlerName == "" {
		return "", false
	}
	var routes []frameworkRoute
	prefixes := pythonAPIRouterPrefixes(source)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		decorator := node.NamedChild(i)
		if decorator.Kind() != "decorator" {
			continue
		}
		if route, ok := pythonDecoratorRoute(source, decorator, handlerName, prefixes); ok {
			routes = append(routes, route)
		}
	}
	if len(routes) == 0 {
		return "", false
	}
	line, column := routeLineColumn(function)
	handlerID := stableNodeID(path, model.NodeKindHandler, handlerName, line)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            handlerID,
		Kind:          model.NodeKindHandler,
		Name:          handlerName,
		QualifiedName: handlerName,
		FilePath:      path,
		Language:      model.LanguagePython,
		StartLine:     line,
		EndLine:       int(function.EndPosition().Row) + 1,
		StartColumn:   column,
		EndColumn:     int(function.EndPosition().Column),
		Signature:     strings.TrimSpace(nodeText(source, function)),
	})
	for _, route := range routes {
		route.HandlerName = ""
		routeID := appendFrameworkRoute(path, result, model.LanguagePython, route)
		result.Edges = append(result.Edges, model.GraphEdge{
			SourceNodeID: routeID,
			TargetNodeID: handlerID,
			Kind:         model.EdgeKindHandles,
			FilePath:     path,
			Line:         route.Line,
			Column:       route.Column,
			Provenance:   route.Framework,
		})
	}
	return handlerID, true
}

func pythonDecoratorRoute(source []byte, node *tree_sitter.Node, handlerName string, prefixes map[string]string) (frameworkRoute, bool) {
	call := namedChildByKind(node, "call")
	if call == nil {
		return frameworkRoute{}, false
	}
	receiver, methodName, ok := routeMemberCall(source, call)
	if !ok {
		return frameworkRoute{}, false
	}
	args := routeArgumentNodes(call)
	pattern, ok := firstStringArgument(source, args)
	if !ok {
		return frameworkRoute{}, false
	}
	if prefix := prefixes[receiver]; prefix != "" {
		pattern = combineRoutePatterns(prefix, pattern)
	}
	line, column := routeLineColumn(node)
	if method, ok := httpMethodFromName(methodName); ok {
		return frameworkRoute{Framework: pythonFrameworkName(methodName), Method: method, Pattern: pattern, HandlerName: handlerName, Line: line, Column: column}, true
	}
	if strings.EqualFold(methodName, "route") || strings.EqualFold(methodName, "api_route") {
		methods := httpMethodsInText(nodeText(source, call))
		if len(methods) == 0 {
			methods = []string{"ANY"}
		}
		return frameworkRoute{Framework: pythonFrameworkName(methodName), Method: methods[0], Pattern: pattern, HandlerName: handlerName, Line: line, Column: column}, true
	}
	return frameworkRoute{}, false
}

var pythonAPIRouterPrefixPattern = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*APIRouter\s*\([^)]*prefix\s*=\s*(?:"([^"]+)"|'([^']+)')`)

func pythonAPIRouterPrefixes(source []byte) map[string]string {
	prefixes := make(map[string]string)
	for _, match := range pythonAPIRouterPrefixPattern.FindAllStringSubmatch(string(source), -1) {
		if len(match) < 4 {
			continue
		}
		prefix := match[2]
		if prefix == "" {
			prefix = match[3]
		}
		if prefix != "" {
			prefixes[match[1]] = prefix
		}
	}
	return prefixes
}

func appendDjangoRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) bool {
	call, ok := callReference(source, node)
	if !ok {
		return false
	}
	if call.name == "add_url_rule" {
		return appendPythonAddURLRule(path, source, node, result)
	}
	if call.name != "path" && call.name != "re_path" {
		return false
	}
	args := routeArgumentNodes(node)
	pattern, ok := firstStringArgument(source, args)
	if !ok {
		return false
	}
	edgeKind := model.EdgeKindHandles
	handler := referenceArgumentName(source, args, 1)
	if handler == "include" || strings.Contains(nodeText(source, node), "include(") {
		edgeKind = model.EdgeKindRoutesTo
	}
	line, column := routeLineColumn(node)
	appendFrameworkRoute(path, result, model.LanguagePython, frameworkRoute{
		Framework:   "django",
		Method:      "ANY",
		Pattern:     pattern,
		EdgeKind:    edgeKind,
		HandlerName: handler,
		Line:        line,
		Column:      column,
	})
	return true
}

func appendPythonAddURLRule(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) bool {
	args := routeArgumentNodes(node)
	pattern, ok := firstStringArgument(source, args)
	if !ok {
		return false
	}
	handler := pythonAddURLRuleHandler(source, node)
	if handler == "" {
		return false
	}
	line, column := routeLineColumn(node)
	appendFrameworkRoute(path, result, model.LanguagePython, frameworkRoute{
		Framework:   "flask",
		Method:      "ANY",
		Pattern:     pattern,
		HandlerName: handler,
		Line:        line,
		Column:      column,
	})
	return true
}

func pythonAddURLRuleHandler(source []byte, node *tree_sitter.Node) string {
	text := nodeText(source, node)
	if idx := strings.Index(text, "view_func="); idx >= 0 {
		value := strings.TrimSpace(text[idx+len("view_func="):])
		if end := strings.IndexAny(value, ",)"); end >= 0 {
			value = strings.TrimSpace(value[:end])
		}
		if dot := strings.Index(value, ".as_view"); dot > 0 {
			return strings.TrimSpace(value[:dot])
		}
		if i := strings.LastIndexAny(value, "."); i >= 0 {
			value = value[i+1:]
		}
		return strings.Trim(value, " \t\r\n")
	}
	return ""
}

var (
	kotlinKtorRoutePattern   = regexp.MustCompile(`\broute\s*\(`)
	kotlinKtorHandlerPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

type kotlinKtorPrefixScope struct {
	prefix     string
	closeDepth int
}

type kotlinKtorRouteScope struct {
	route      frameworkRoute
	closeDepth int
	added      bool
}

func appendKotlinKtorRoutes(path string, source []byte, result *ExtractionResult) {
	lines := strings.Split(string(source), "\n")
	var prefixes []kotlinKtorPrefixScope
	var routes []kotlinKtorRouteScope
	depth := 0
	for index, raw := range lines {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(raw)
		popKotlinKtorScopes(&prefixes, &routes, depth)

		if pattern, ok := kotlinKtorRoutePatternFromLine(trimmed); ok {
			afterDepth := depth + strings.Count(raw, "{") - strings.Count(raw, "}")
			prefixes = append(prefixes, kotlinKtorPrefixScope{
				prefix:     combineRoutePatterns(kotlinKtorPrefix(prefixes), pattern),
				closeDepth: afterDepth,
			})
		}
		if method, pattern, ok := kotlinKtorHTTPRouteFromLine(trimmed); ok {
			afterDepth := depth + strings.Count(raw, "{") - strings.Count(raw, "}")
			routes = append(routes, kotlinKtorRouteScope{
				route: frameworkRoute{
					Framework: "ktor",
					Method:    method,
					Pattern:   combineRoutePatterns(kotlinKtorPrefix(prefixes), pattern),
					Line:      lineNumber,
					Column:    strings.Index(raw, strings.ToLower(method)),
				},
				closeDepth: afterDepth,
			})
		}
		if handler := kotlinKtorHandlerFromLine(trimmed); handler != "" && len(routes) > 0 {
			current := &routes[len(routes)-1]
			if !current.added {
				current.route.HandlerName = handler
				appendFrameworkRoute(path, result, model.LanguageKotlin, current.route)
				current.added = true
			}
		}

		depth += strings.Count(raw, "{") - strings.Count(raw, "}")
		popKotlinKtorScopes(&prefixes, &routes, depth)
	}
}

func popKotlinKtorScopes(prefixes *[]kotlinKtorPrefixScope, routes *[]kotlinKtorRouteScope, depth int) {
	for len(*prefixes) > 0 && depth < (*prefixes)[len(*prefixes)-1].closeDepth {
		*prefixes = (*prefixes)[:len(*prefixes)-1]
	}
	for len(*routes) > 0 && depth < (*routes)[len(*routes)-1].closeDepth {
		*routes = (*routes)[:len(*routes)-1]
	}
}

func kotlinKtorPrefix(prefixes []kotlinKtorPrefixScope) string {
	if len(prefixes) == 0 {
		return ""
	}
	return prefixes[len(prefixes)-1].prefix
}

func kotlinKtorRoutePatternFromLine(line string) (string, bool) {
	if !kotlinKtorRoutePattern.MatchString(line) {
		return "", false
	}
	pattern := firstQuotedText(line)
	return pattern, pattern != ""
}

func kotlinKtorHTTPRouteFromLine(line string) (string, string, bool) {
	open := strings.Index(line, "(")
	if open < 0 {
		return "", "", false
	}
	name := strings.TrimSpace(line[:open])
	fields := strings.Fields(name)
	if len(fields) > 0 {
		name = fields[len(fields)-1]
	}
	method, ok := httpMethodFromName(name)
	if !ok {
		return "", "", false
	}
	pattern := firstQuotedText(line)
	return method, pattern, pattern != ""
}

func kotlinKtorHandlerFromLine(line string) string {
	if strings.HasPrefix(line, "fun ") || strings.HasPrefix(line, "class ") || strings.HasPrefix(line, "object ") {
		return ""
	}
	for _, match := range kotlinKtorHandlerPattern.FindAllStringSubmatch(line, -1) {
		if len(match) < 2 {
			continue
		}
		name := match[1]
		switch name {
		case "routing", "route", "get", "post", "put", "delete", "patch", "head", "options":
			continue
		default:
			return name
		}
	}
	return ""
}

func pythonFrameworkName(methodName string) string {
	if strings.EqualFold(methodName, "route") {
		return "flask"
	}
	return "fastapi"
}

func httpMethodsInText(text string) []string {
	upper := strings.ToUpper(text)
	var methods []string
	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"} {
		if strings.Contains(upper, method) {
			methods = append(methods, method)
		}
	}
	return uniqueStrings(methods)
}

func recordGoRoutePrefix(source []byte, node *tree_sitter.Node, prefixes map[string]string) {
	if node.NamedChildCount() < 2 {
		return
	}
	left := node.NamedChild(0)
	right := node.NamedChild(1)
	if left.Kind() != "expression_list" || right.Kind() != "expression_list" || left.NamedChildCount() == 0 || right.NamedChildCount() == 0 {
		return
	}
	varName := strings.TrimSpace(nodeText(source, left.NamedChild(0)))
	call := right.NamedChild(0)
	if call.Kind() != "call_expression" {
		return
	}
	receiver, method, ok := routeMemberCall(source, call)
	if !ok || method != "Group" {
		return
	}
	args := routeArgumentNodes(call)
	pattern, ok := firstStringArgument(source, args)
	if !ok {
		return
	}
	prefix := prefixes[receiver]
	prefixes[varName] = combineRoutePatterns(prefix, pattern)
}

func appendGoRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, prefixes map[string]string) bool {
	if appendGoMuxMethodsRoute(path, source, node, result, prefixes) {
		return true
	}
	receiver, methodName, ok := routeMemberCall(source, node)
	if !ok {
		return false
	}
	args := routeArgumentNodes(node)
	line, column := routeLineColumn(node)
	if strings.EqualFold(methodName, "Use") {
		pattern := prefixes[receiver]
		handlerIndex := 0
		if value, ok := firstStringArgument(source, args); ok {
			pattern = combineRoutePatterns(pattern, value)
			handlerIndex = 1
		}
		appendFrameworkRoute(path, result, model.LanguageGo, frameworkRoute{
			Framework:   goFrameworkName(methodName),
			Method:      "USE",
			Pattern:     pattern,
			EdgeKind:    model.EdgeKindMiddleware,
			HandlerName: referenceArgumentName(source, args, handlerIndex),
			Line:        line,
			Column:      column,
		})
		return true
	}
	if strings.EqualFold(methodName, "HandleFunc") {
		pattern, ok := firstStringArgument(source, args)
		if !ok {
			return false
		}
		appendFrameworkRoute(path, result, model.LanguageGo, frameworkRoute{
			Framework:   "gorilla/mux",
			Method:      "ANY",
			Pattern:     combineRoutePatterns(prefixes[receiver], pattern),
			HandlerName: referenceArgumentName(source, args, 1),
			Line:        line,
			Column:      column,
		})
		return true
	}
	method, ok := httpMethodFromName(methodName)
	if !ok {
		return false
	}
	pattern, ok := firstStringArgument(source, args)
	if !ok {
		return false
	}
	appendFrameworkRoute(path, result, model.LanguageGo, frameworkRoute{
		Framework:   goFrameworkName(methodName),
		Method:      method,
		Pattern:     combineRoutePatterns(prefixes[receiver], pattern),
		HandlerName: referenceArgumentName(source, args, 1),
		Line:        line,
		Column:      column,
	})
	return true
}

func appendGoMuxMethodsRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, prefixes map[string]string) bool {
	_, methodName, ok := routeMemberCall(source, node)
	if !ok || methodName != "Methods" {
		return false
	}
	function := node.ChildByFieldName("function")
	if function == nil || function.NamedChildCount() == 0 {
		return false
	}
	inner := function.NamedChild(0)
	if inner.Kind() != "call_expression" {
		return false
	}
	receiver, innerMethod, ok := routeMemberCall(source, inner)
	if !ok || !strings.EqualFold(innerMethod, "HandleFunc") {
		return false
	}
	innerArgs := routeArgumentNodes(inner)
	pattern, ok := firstStringArgument(source, innerArgs)
	if !ok {
		return false
	}
	methodArgs := routeArgumentNodes(node)
	method := "ANY"
	if value, ok := firstStringArgument(source, methodArgs); ok {
		method = strings.ToUpper(value)
	}
	line, column := routeLineColumn(node)
	appendFrameworkRoute(path, result, model.LanguageGo, frameworkRoute{
		Framework:   "gorilla/mux",
		Method:      method,
		Pattern:     combineRoutePatterns(prefixes[receiver], pattern),
		HandlerName: referenceArgumentName(source, innerArgs, 1),
		Line:        line,
		Column:      column,
	})
	return true
}

func goFrameworkName(methodName string) string {
	if methodName == strings.ToUpper(methodName) {
		return "gin"
	}
	return "chi"
}

func csharpRoutePrefix(source []byte, node *tree_sitter.Node, className string) (string, bool) {
	for _, attr := range csharpAttributes(node) {
		if csharpAttributeName(source, attr) != "Route" {
			continue
		}
		values := stringValues(source, attr)
		if len(values) == 0 {
			return "", true
		}
		return csharpExpandRouteTokens(values[0], className), true
	}
	return "", false
}

func appendCSharpHandlerOrMethod(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, routePrefix, className string) string {
	routes := csharpAttributeRoutes(source, node, routePrefix, className)
	if len(routes) == 0 {
		return appendScopedLanguageNode(path, source, node, model.NodeKindMethod, model.LanguageCSharp, result, className)
	}
	name := declarationName(source, node)
	if name == "" {
		return ""
	}
	qualifiedName := name
	if className != "" {
		qualifiedName = className + "." + name
	}
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	metadata := nodeSignatureMetadata(source, node, name)
	metadata.receiverType = className
	handlerID := stableNodeID(path, model.NodeKindHandler, qualifiedName, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:             handlerID,
		Kind:           model.NodeKindHandler,
		Name:           name,
		QualifiedName:  qualifiedName,
		ReceiverType:   metadata.receiverType,
		ParameterCount: metadata.parameterCount,
		ParameterTypes: metadata.parameterTypes,
		ReturnType:     metadata.returnType,
		FilePath:       path,
		Language:       model.LanguageCSharp,
		StartLine:      startLine,
		EndLine:        int(end.Row) + 1,
		StartColumn:    int(start.Column),
		EndColumn:      int(end.Column),
		Signature:      strings.TrimSpace(nodeText(source, node)),
	})
	for _, route := range routes {
		route.HandlerName = ""
		routeID := appendFrameworkRoute(path, result, model.LanguageCSharp, route)
		result.Edges = append(result.Edges, model.GraphEdge{
			SourceNodeID: routeID,
			TargetNodeID: handlerID,
			Kind:         model.EdgeKindHandles,
			FilePath:     path,
			Line:         route.Line,
			Column:       route.Column,
			Provenance:   route.Framework,
		})
	}
	return handlerID
}

func csharpAttributeRoutes(source []byte, node *tree_sitter.Node, routePrefix, className string) []frameworkRoute {
	var routes []frameworkRoute
	for _, attr := range csharpAttributes(node) {
		name := csharpAttributeName(source, attr)
		method, ok := csharpHTTPAttributeMethod(name)
		if !ok {
			continue
		}
		values := stringValues(source, attr)
		pattern := ""
		if len(values) > 0 {
			pattern = csharpExpandRouteTokens(values[0], className)
		}
		line, column := routeLineColumn(attr)
		routes = append(routes, frameworkRoute{
			Framework: "aspnet",
			Method:    method,
			Pattern:   combineRoutePatterns(routePrefix, pattern),
			Line:      line,
			Column:    column,
		})
	}
	return routes
}

func appendCSharpMinimalAPIRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, prefixes map[string]string) bool {
	receiver, methodName, ok := routeMemberCall(source, node)
	if !ok || !strings.HasPrefix(methodName, "Map") {
		return false
	}
	method, ok := httpMethodFromName(strings.TrimPrefix(methodName, "Map"))
	if !ok {
		return false
	}
	args := routeArgumentNodes(node)
	pattern, ok := firstStringArgument(source, args)
	if !ok {
		return false
	}
	pattern = combineRoutePatterns(prefixes[receiver], pattern)
	line, column := routeLineColumn(node)
	appendFrameworkRoute(path, result, model.LanguageCSharp, frameworkRoute{
		Framework:   "aspnet",
		Method:      method,
		Pattern:     pattern,
		HandlerName: referenceArgumentName(source, args, 1),
		Line:        line,
		Column:      column,
	})
	return true
}

func csharpRouteGroupPrefixes(source []byte) map[string]string {
	prefixes := map[string]string{}
	for _, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, ".MapGroup(") || !strings.Contains(trimmed, "=") {
			continue
		}
		left, right, _ := strings.Cut(trimmed, "=")
		leftFields := strings.Fields(strings.TrimSpace(left))
		if len(leftFields) == 0 {
			continue
		}
		name := leftFields[len(leftFields)-1]
		receiver := csharpMapGroupReceiver(trimmed)
		pattern := firstQuotedText(right)
		if name != "" && pattern != "" {
			prefixes[name] = combineRoutePatterns(prefixes[receiver], pattern)
		}
	}
	return prefixes
}

func csharpMapGroupReceiver(line string) string {
	before, _, ok := strings.Cut(line, ".MapGroup(")
	if !ok {
		return ""
	}
	before = strings.TrimSpace(before)
	if i := strings.LastIndex(before, "="); i >= 0 {
		before = strings.TrimSpace(before[i+1:])
	}
	if i := strings.LastIndexAny(before, " \t"); i >= 0 {
		before = strings.TrimSpace(before[i+1:])
	}
	return before
}

func csharpAttributes(node *tree_sitter.Node) []*tree_sitter.Node {
	var attrs []*tree_sitter.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() != "attribute_list" {
			continue
		}
		for j := uint(0); j < child.NamedChildCount(); j++ {
			if child.NamedChild(j).Kind() == "attribute" {
				attrs = append(attrs, child.NamedChild(j))
			}
		}
	}
	return attrs
}

func csharpAttributeName(source []byte, attr *tree_sitter.Node) string {
	return firstIdentifier(source, attr)
}

func csharpHTTPAttributeMethod(name string) (string, bool) {
	name = strings.TrimSuffix(name, "Attribute")
	switch name {
	case "HttpGet":
		return "GET", true
	case "HttpPost":
		return "POST", true
	case "HttpPut":
		return "PUT", true
	case "HttpDelete":
		return "DELETE", true
	case "HttpPatch":
		return "PATCH", true
	case "HttpHead":
		return "HEAD", true
	case "HttpOptions":
		return "OPTIONS", true
	default:
		return "", false
	}
}

func csharpExpandRouteTokens(pattern, className string) string {
	controller := strings.TrimSuffix(className, "Controller")
	controller = strings.ToLower(controller)
	pattern = strings.ReplaceAll(pattern, "[controller]", controller)
	pattern = strings.ReplaceAll(pattern, "[Controller]", controller)
	return pattern
}

func rustAttributeRoute(source []byte, node *tree_sitter.Node) (frameworkRoute, bool) {
	name := firstIdentifier(source, node)
	method, ok := httpMethodFromName(name)
	if !ok {
		return frameworkRoute{}, false
	}
	pattern, ok := firstStringValue(source, node)
	if !ok {
		return frameworkRoute{}, false
	}
	line, column := routeLineColumn(node)
	return frameworkRoute{
		Framework: "rust-web",
		Method:    method,
		Pattern:   pattern,
		Line:      line,
		Column:    column,
	}, true
}

func appendRustAttributedHandler(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, routes []frameworkRoute) string {
	name := declarationName(source, node)
	if name == "" {
		return ""
	}
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	handlerID := stableNodeID(path, model.NodeKindHandler, name, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            handlerID,
		Kind:          model.NodeKindHandler,
		Name:          name,
		QualifiedName: name,
		FilePath:      path,
		Language:      model.LanguageRust,
		StartLine:     startLine,
		EndLine:       int(end.Row) + 1,
		StartColumn:   int(start.Column),
		EndColumn:     int(end.Column),
		Signature:     strings.TrimSpace(nodeText(source, node)),
	})
	for _, route := range routes {
		route.HandlerName = ""
		routeID := appendFrameworkRoute(path, result, model.LanguageRust, route)
		result.Edges = append(result.Edges, model.GraphEdge{
			SourceNodeID: routeID,
			TargetNodeID: handlerID,
			Kind:         model.EdgeKindHandles,
			FilePath:     path,
			Line:         route.Line,
			Column:       route.Column,
			Provenance:   route.Framework,
		})
	}
	return handlerID
}

func appendRustRouterRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) bool {
	_, methodName, ok := routeMemberCall(source, node)
	if !ok || (methodName != "route" && methodName != "nest") {
		return false
	}
	args := routeArgumentNodes(node)
	pattern, ok := firstStringArgument(source, args)
	if !ok {
		return false
	}
	line, column := routeLineColumn(node)
	if methodName == "nest" {
		if len(args) > 1 && appendRustNestedRouterRoutes(path, source, args[1], result, pattern) {
			return true
		}
		appendFrameworkRoute(path, result, model.LanguageRust, frameworkRoute{
			Framework:   "axum",
			Method:      "ANY",
			Pattern:     pattern,
			EdgeKind:    model.EdgeKindRoutesTo,
			HandlerName: referenceArgumentName(source, args, 1),
			Line:        line,
			Column:      column,
		})
		return true
	}
	routes := []frameworkRoute{{Framework: "axum", Method: "ANY", Pattern: pattern, Line: line, Column: column}}
	if len(args) > 1 {
		routes = rustAxumHandlerRoutes(source, args[1], pattern, line, column)
	}
	for _, route := range routes {
		appendFrameworkRoute(path, result, model.LanguageRust, route)
	}
	return true
}

func appendRustNestedRouterRoutes(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, prefix string) bool {
	added := false
	var walk func(*tree_sitter.Node)
	walk = func(current *tree_sitter.Node) {
		if current == nil {
			return
		}
		_, methodName, ok := routeMemberCall(source, current)
		if ok && methodName == "route" {
			args := routeArgumentNodes(current)
			pattern, hasPattern := firstStringArgument(source, args)
			if hasPattern {
				line, column := routeLineColumn(current)
				fullPattern := combineRoutePatterns(prefix, pattern)
				routes := []frameworkRoute{{
					Framework: "axum",
					Method:    "ANY",
					Pattern:   fullPattern,
					Line:      line,
					Column:    column,
				}}
				if len(args) > 1 {
					routes = rustAxumHandlerRoutes(source, args[1], fullPattern, line, column)
				}
				for _, route := range routes {
					appendFrameworkRoute(path, result, model.LanguageRust, route)
				}
				added = true
			}
		}
		for i := uint(0); i < current.NamedChildCount(); i++ {
			walk(current.NamedChild(i))
		}
	}
	walk(node)
	return added
}

func rustAxumHandlerRoutes(source []byte, node *tree_sitter.Node, pattern string, line, column int) []frameworkRoute {
	handlers := rustAxumHandlers(source, node)
	routes := make([]frameworkRoute, 0, len(handlers))
	for _, handler := range handlers {
		routes = append(routes, frameworkRoute{
			Framework:   "axum",
			Method:      handler.method,
			Pattern:     pattern,
			HandlerName: handler.handler,
			Line:        line,
			Column:      column,
		})
	}
	if len(routes) == 0 {
		method, handler := rustAxumHandler(source, node)
		routes = append(routes, frameworkRoute{
			Framework:   "axum",
			Method:      method,
			Pattern:     pattern,
			HandlerName: handler,
			Line:        line,
			Column:      column,
		})
	}
	return routes
}

type rustAxumMethodHandler struct {
	method  string
	handler string
}

var rustAxumMethodCallPattern = regexp.MustCompile(`\b(get|post|put|delete|patch|head|options)\s*\(\s*([A-Za-z_][A-Za-z0-9_:]*)`)

func rustAxumHandlers(source []byte, node *tree_sitter.Node) []rustAxumMethodHandler {
	text := nodeText(source, node)
	var handlers []rustAxumMethodHandler
	for _, match := range rustAxumMethodCallPattern.FindAllStringSubmatch(text, -1) {
		if len(match) < 3 {
			continue
		}
		method, ok := httpMethodFromName(match[1])
		if !ok {
			continue
		}
		handlers = append(handlers, rustAxumMethodHandler{method: method, handler: match[2]})
	}
	return handlers
}

func rustAxumHandler(source []byte, node *tree_sitter.Node) (string, string) {
	if node == nil {
		return "ANY", ""
	}
	call, ok := callReference(source, node)
	if !ok {
		return "ANY", referenceArgumentName(source, []*tree_sitter.Node{node}, 0)
	}
	method, ok := httpMethodFromName(call.name)
	if !ok {
		method = "ANY"
	}
	args := routeArgumentNodes(node)
	return method, referenceArgumentName(source, args, 0)
}
