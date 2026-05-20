package parser

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	"repobridge/internal/codegraph/model"
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

func referenceArgumentName(source []byte, args []*tree_sitter.Node, index int) string {
	if index < 0 || index >= len(args) {
		return ""
	}
	_, name, ok := referenceName(source, args[index])
	if ok {
		return name
	}
	if args[index].Kind() == "call_expression" || args[index].Kind() == "call" || args[index].Kind() == "invocation_expression" {
		_, name, ok := callReference(source, args[index])
		if ok {
			return name
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
			if _, ref, ok := referenceName(source, node.NamedChild(i)); ok {
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
			} else if _, ref, ok := referenceName(source, value); ok {
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
			_, handler, _ = referenceName(source, value)
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
	for i := uint(0); i < node.NamedChildCount(); i++ {
		decorator := node.NamedChild(i)
		if decorator.Kind() != "decorator" {
			continue
		}
		if route, ok := pythonDecoratorRoute(source, decorator, handlerName); ok {
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

func pythonDecoratorRoute(source []byte, node *tree_sitter.Node, handlerName string) (frameworkRoute, bool) {
	call := namedChildByKind(node, "call")
	if call == nil {
		return frameworkRoute{}, false
	}
	_, methodName, ok := routeMemberCall(source, call)
	if !ok {
		return frameworkRoute{}, false
	}
	args := routeArgumentNodes(call)
	pattern, ok := firstStringArgument(source, args)
	if !ok {
		return frameworkRoute{}, false
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

func appendDjangoRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) bool {
	_, name, ok := callReference(source, node)
	if !ok || (name != "path" && name != "re_path") {
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
		return appendLanguageNode(path, source, node, model.NodeKindMethod, model.LanguageCSharp, result)
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
	handlerID := stableNodeID(path, model.NodeKindHandler, qualifiedName, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            handlerID,
		Kind:          model.NodeKindHandler,
		Name:          name,
		QualifiedName: qualifiedName,
		FilePath:      path,
		Language:      model.LanguageCSharp,
		StartLine:     startLine,
		EndLine:       int(end.Row) + 1,
		StartColumn:   int(start.Column),
		EndColumn:     int(end.Column),
		Signature:     strings.TrimSpace(nodeText(source, node)),
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

func appendCSharpMinimalAPIRoute(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) bool {
	_, methodName, ok := routeMemberCall(source, node)
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
	method := "ANY"
	handler := ""
	if len(args) > 1 {
		method, handler = rustAxumHandler(source, args[1])
	}
	appendFrameworkRoute(path, result, model.LanguageRust, frameworkRoute{
		Framework:   "axum",
		Method:      method,
		Pattern:     pattern,
		HandlerName: handler,
		Line:        line,
		Column:      column,
	})
	return true
}

func rustAxumHandler(source []byte, node *tree_sitter.Node) (string, string) {
	if node == nil {
		return "ANY", ""
	}
	_, methodName, ok := callReference(source, node)
	if !ok {
		return "ANY", referenceArgumentName(source, []*tree_sitter.Node{node}, 0)
	}
	method, ok := httpMethodFromName(methodName)
	if !ok {
		method = "ANY"
	}
	args := routeArgumentNodes(node)
	return method, referenceArgumentName(source, args, 0)
}
