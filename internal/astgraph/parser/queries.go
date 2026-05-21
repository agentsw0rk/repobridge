package parser

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	"repobridge/internal/astgraph/model"
)

func walkGo(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkGoNode(path, source, node, result, "", map[string]string{})
}

func walkByLanguage(path string, source []byte, node *tree_sitter.Node, language model.Language, result *ExtractionResult) {
	switch language {
	case model.LanguageGo:
		walkGo(path, source, node, result)
	case model.LanguageJavaScript:
		walkJavaScript(path, source, node, result)
	case model.LanguageTypeScript:
		walkTypeScript(path, source, node, result)
	case model.LanguagePython:
		walkPython(path, source, node, result)
	case model.LanguageRust:
		walkRust(path, source, node, result)
	case model.LanguageJava:
		walkJava(path, source, node, result)
	case model.LanguageKotlin:
		walkKotlin(path, source, node, result)
	case model.LanguageCSharp:
		walkCSharp(path, source, node, result)
	}
}

type extractionConfig struct {
	language       model.Language
	nodeKinds      map[string]model.NodeKind
	callKinds      map[string]bool
	anonymousKinds map[string]bool
}

func walkJavaScript(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkJavaScriptRouteAwareNode(path, source, node, result, "", model.LanguageJavaScript)
}

func walkTypeScript(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkJavaScriptRouteAwareNode(path, source, node, result, "", model.LanguageTypeScript)
}

func walkPython(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkPythonNode(path, source, node, result, "", false)
}

func walkRust(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	pending := []frameworkRoute{}
	walkRustNode(path, source, node, result, "", &pending)
}

func walkJava(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkJavaNode(path, source, node, result, "", "", "")
}

func walkKotlin(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkKotlinNode(path, source, node, result, "", "", "")
}

func walkCSharp(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkCSharpNode(path, source, node, result, "", "", "")
}

func walkConfiguredNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string, config extractionConfig) {
	if node == nil {
		return
	}

	kind := node.Kind()
	if graphKind, ok := config.nodeKinds[kind]; ok {
		if id := appendLanguageNode(path, source, node, graphKind, config.language, result); id != "" {
			currentNodeID = id
		}
	} else if config.anonymousKinds[kind] {
		currentNodeID = ""
	} else if config.callKinds[kind] && currentNodeID != "" {
		appendLanguageCall(path, source, node, config.language, result, currentNodeID)
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkConfiguredNode(path, source, node.NamedChild(i), result, currentNodeID, config)
	}
}

func walkJavaScriptRouteAwareNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string, language model.Language) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "function_declaration":
		if id := appendLanguageNode(path, source, node, model.NodeKindFunction, language, result); id != "" {
			currentNodeID = id
		}
	case "method_definition":
		if id := appendLanguageNode(path, source, node, model.NodeKindMethod, language, result); id != "" {
			currentNodeID = id
		}
	case "arrow_function", "function":
		currentNodeID = ""
	case "call_expression":
		if !appendJavaScriptRoute(path, source, node, result, language) && currentNodeID != "" {
			appendLanguageCall(path, source, node, language, result, currentNodeID)
		}
	case "jsx_self_closing_element", "jsx_opening_element":
		appendReactRouterJSXRoute(path, source, node, result, language)
	case "object":
		appendReactRouterObjectRoute(path, source, node, result, language)
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkJavaScriptRouteAwareNode(path, source, node.NamedChild(i), result, currentNodeID, language)
	}
}

func walkPythonNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string, suppressDefinition bool) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "decorated_definition":
		if id, handled := appendPythonDecoratedRoute(path, source, node, result); handled {
			currentNodeID = id
			suppressDefinition = true
		}
	case "function_definition":
		if !suppressDefinition {
			if id := appendLanguageNode(path, source, node, model.NodeKindFunction, model.LanguagePython, result); id != "" {
				currentNodeID = id
			}
		}
	case "lambda":
		currentNodeID = ""
	case "call":
		if !appendDjangoRoute(path, source, node, result) && currentNodeID != "" {
			appendLanguageCall(path, source, node, model.LanguagePython, result, currentNodeID)
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		childSuppress := suppressDefinition && child.Kind() == "function_definition"
		walkPythonNode(path, source, child, result, currentNodeID, childSuppress)
	}
}

func walkRustNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string, pendingRoutes *[]frameworkRoute) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "attribute_item":
		if route, ok := rustAttributeRoute(source, node); ok {
			*pendingRoutes = append(*pendingRoutes, route)
		}
		return
	case "function_item":
		if len(*pendingRoutes) > 0 {
			if id := appendRustAttributedHandler(path, source, node, result, *pendingRoutes); id != "" {
				currentNodeID = id
			}
			*pendingRoutes = nil
		} else if id := appendLanguageNode(path, source, node, model.NodeKindFunction, model.LanguageRust, result); id != "" {
			currentNodeID = id
		}
	case "closure_expression":
		currentNodeID = ""
	case "call_expression":
		if !appendRustRouterRoute(path, source, node, result) && currentNodeID != "" {
			appendLanguageCall(path, source, node, model.LanguageRust, result, currentNodeID)
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkRustNode(path, source, node.NamedChild(i), result, currentNodeID, pendingRoutes)
	}
}

func walkJavaNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID, routePrefix, className string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "class_declaration":
		if name := declarationName(source, node); name != "" {
			className = name
		}
		if prefix, ok := springClassRoutePrefix(source, node); ok {
			routePrefix = combineRoutePatterns(routePrefix, prefix)
		}
	case "method_declaration":
		if id := appendSpringHandlerOrJavaMethod(path, source, node, result, routePrefix, className); id != "" {
			currentNodeID = id
		}
	case "lambda_expression":
		currentNodeID = ""
	case "method_invocation":
		if currentNodeID != "" {
			appendLanguageCall(path, source, node, model.LanguageJava, result, currentNodeID)
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkJavaNode(path, source, node.NamedChild(i), result, currentNodeID, routePrefix, className)
	}
}

func walkKotlinNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID, routePrefix, className string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "annotated_expression":
		if prefix, ok := springRoutePrefixFromAnnotations(source, directAnnotations(node)); ok {
			routePrefix = combineRoutePatterns(routePrefix, prefix)
		}
	case "class_declaration", "object_declaration":
		if name := declarationName(source, node); name != "" {
			className = name
		}
	case "infix_expression":
		if name := kotlinClassLikeName(source, node); name != "" {
			className = name
		}
	case "function_declaration":
		if id := appendSpringHandlerOrKotlinFunction(path, source, node, result, routePrefix, className); id != "" {
			currentNodeID = id
		}
	case "anonymous_function", "lambda_literal":
		currentNodeID = ""
	case "call_expression":
		if currentNodeID != "" {
			appendLanguageCall(path, source, node, model.LanguageKotlin, result, currentNodeID)
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkKotlinNode(path, source, node.NamedChild(i), result, currentNodeID, routePrefix, className)
	}
}

func walkCSharpNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID, routePrefix, className string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "class_declaration":
		if name := declarationName(source, node); name != "" {
			className = name
		}
		if prefix, ok := csharpRoutePrefix(source, node, className); ok {
			routePrefix = combineRoutePatterns(routePrefix, prefix)
		}
	case "method_declaration":
		if id := appendCSharpHandlerOrMethod(path, source, node, result, routePrefix, className); id != "" {
			currentNodeID = id
		}
	case "anonymous_method_expression", "lambda_expression":
		currentNodeID = ""
	case "invocation_expression":
		if !appendCSharpMinimalAPIRoute(path, source, node, result) && currentNodeID != "" {
			appendLanguageCall(path, source, node, model.LanguageCSharp, result, currentNodeID)
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkCSharpNode(path, source, node.NamedChild(i), result, currentNodeID, routePrefix, className)
	}
}

func walkGoNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string, routePrefixes map[string]string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "function_declaration":
		if id := appendGoNode(path, source, node, model.NodeKindFunction, result); id != "" {
			currentNodeID = id
		}
	case "method_declaration":
		if id := appendGoNode(path, source, node, model.NodeKindMethod, result); id != "" {
			currentNodeID = id
		}
	case "func_literal":
		currentNodeID = ""
	case "short_var_declaration":
		recordGoRoutePrefix(source, node, routePrefixes)
	case "call_expression":
		if !appendGoRoute(path, source, node, result, routePrefixes) && currentNodeID != "" {
			appendGoCall(path, source, node, result, currentNodeID)
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkGoNode(path, source, node.NamedChild(i), result, currentNodeID, routePrefixes)
	}
}

func appendGoNode(path string, source []byte, node *tree_sitter.Node, kind model.NodeKind, result *ExtractionResult) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	name := nodeText(source, nameNode)
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	id := stableNodeID(path, kind, name, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            id,
		Kind:          kind,
		Name:          name,
		QualifiedName: name,
		FilePath:      path,
		Language:      model.LanguageGo,
		StartLine:     startLine,
		EndLine:       int(end.Row) + 1,
		StartColumn:   int(start.Column),
		EndColumn:     int(end.Column),
		Signature:     strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func appendLanguageNode(path string, source []byte, node *tree_sitter.Node, kind model.NodeKind, language model.Language, result *ExtractionResult) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	name := nodeText(source, nameNode)
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	id := stableNodeID(path, kind, name, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            id,
		Kind:          kind,
		Name:          name,
		QualifiedName: name,
		FilePath:      path,
		Language:      language,
		StartLine:     startLine,
		EndLine:       int(end.Row) + 1,
		StartColumn:   int(start.Column),
		EndColumn:     int(end.Column),
		Signature:     strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func appendSpringHandlerOrJavaMethod(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, routePrefix, className string) string {
	routes := springRoutesFromAnnotations(source, directAnnotations(node), routePrefix)
	if len(routes) == 0 {
		return appendLanguageNode(path, source, node, model.NodeKindMethod, model.LanguageJava, result)
	}
	return appendSpringHandler(path, source, node, result, model.LanguageJava, className, routes)
}

func appendSpringHandlerOrKotlinFunction(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, routePrefix, className string) string {
	routes := springRoutesFromAnnotations(source, directAnnotations(node), routePrefix)
	if len(routes) == 0 {
		return appendLanguageNode(path, source, node, model.NodeKindFunction, model.LanguageKotlin, result)
	}
	return appendSpringHandler(path, source, node, result, model.LanguageKotlin, className, routes)
}

func appendSpringHandler(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, language model.Language, className string, routes []springRoute) string {
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
		Language:      language,
		StartLine:     startLine,
		EndLine:       int(end.Row) + 1,
		StartColumn:   int(start.Column),
		EndColumn:     int(end.Column),
		Signature:     strings.TrimSpace(nodeText(source, node)),
	})

	for _, route := range routes {
		appendSpringRoute(path, result, language, route, handlerID, qualifiedName)
	}
	return handlerID
}

func appendSpringRoute(path string, result *ExtractionResult, language model.Language, route springRoute, handlerID, handlerName string) {
	name := strings.TrimSpace(route.Method + " " + route.Pattern)
	id := stableNodeID(path, model.NodeKindRoute, name, route.Line)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            id,
		Kind:          model.NodeKindRoute,
		Name:          name,
		QualifiedName: "spring " + name,
		FilePath:      path,
		Language:      language,
		StartLine:     route.Line,
		EndLine:       route.Line,
		StartColumn:   route.Column,
		EndColumn:     route.Column,
		Signature:     fmt.Sprintf("spring route %s -> %s", name, handlerName),
	})
	result.Edges = append(result.Edges, model.GraphEdge{
		SourceNodeID: id,
		TargetNodeID: handlerID,
		Kind:         model.EdgeKindHandles,
		FilePath:     path,
		Line:         route.Line,
		Column:       route.Column,
		Provenance:   "spring",
	})
}

func appendGoCall(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, fromNodeID string) {
	functionNode := node.ChildByFieldName("function")
	if functionNode == nil {
		return
	}

	name, ok := goCallReferenceName(source, functionNode)
	if !ok {
		return
	}

	start := functionNode.StartPosition()
	result.Unresolved = append(result.Unresolved, model.UnresolvedReference{
		FromNodeID:    fromNodeID,
		ReferenceName: name,
		ReferenceKind: model.EdgeKindCalls,
		FilePath:      path,
		Language:      model.LanguageGo,
		Line:          int(start.Row) + 1,
		Column:        int(start.Column),
	})
}

func appendLanguageCall(path string, source []byte, node *tree_sitter.Node, language model.Language, result *ExtractionResult, fromNodeID string) {
	nameNode, name, ok := callReference(source, node)
	if !ok {
		return
	}

	start := nameNode.StartPosition()
	result.Unresolved = append(result.Unresolved, model.UnresolvedReference{
		FromNodeID:    fromNodeID,
		ReferenceName: name,
		ReferenceKind: model.EdgeKindCalls,
		FilePath:      path,
		Language:      language,
		Line:          int(start.Row) + 1,
		Column:        int(start.Column),
	})
}

func callReference(source []byte, node *tree_sitter.Node) (*tree_sitter.Node, string, bool) {
	for _, field := range []string{"function", "expression", "name"} {
		callee := node.ChildByFieldName(field)
		if callee == nil {
			continue
		}
		nameNode, name, ok := referenceName(source, callee)
		if ok {
			return nameNode, name, true
		}
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		nameNode, name, ok := referenceName(source, node.NamedChild(i))
		if ok {
			return nameNode, name, true
		}
	}
	return nil, "", false
}

func referenceName(source []byte, node *tree_sitter.Node) (*tree_sitter.Node, string, bool) {
	switch node.Kind() {
	case "identifier", "property_identifier", "field_identifier":
		name := strings.TrimSpace(nodeText(source, node))
		return node, name, name != ""
	case "attribute", "field_expression", "member_access_expression", "member_expression", "scoped_identifier", "selector_expression":
		for _, field := range []string{"name", "field", "attribute", "property"} {
			child := node.ChildByFieldName(field)
			if child == nil {
				continue
			}
			nameNode, name, ok := referenceName(source, child)
			if ok {
				return nameNode, name, true
			}
		}
		name := strings.TrimSpace(nodeText(source, node))
		if i := strings.LastIndexAny(name, ".:"); i >= 0 {
			name = strings.TrimSpace(name[i+1:])
		}
		if name == "" || strings.ContainsAny(name, " ()[]{}") {
			return nil, "", false
		}
		return node, name, true
	case "expression":
		if node.NamedChildCount() == 1 {
			return referenceName(source, node.NamedChild(0))
		}
		return nil, "", false
	default:
		return nil, "", false
	}
}

func goCallReferenceName(source []byte, node *tree_sitter.Node) (string, bool) {
	switch node.Kind() {
	case "identifier":
		name := strings.TrimSpace(nodeText(source, node))
		return name, name != ""
	case "selector_expression":
		name := strings.TrimSpace(nodeText(source, node))
		if lastDot := strings.LastIndex(name, "."); lastDot >= 0 {
			name = name[lastDot+1:]
		}
		return name, name != ""
	default:
		return "", false
	}
}

type springRoute struct {
	Method  string
	Pattern string
	Line    int
	Column  int
}

func springClassRoutePrefix(source []byte, node *tree_sitter.Node) (string, bool) {
	return springRoutePrefixFromAnnotations(source, directAnnotations(node))
}

func springRoutePrefixFromAnnotations(source []byte, annotations []*tree_sitter.Node) (string, bool) {
	for _, annotation := range annotations {
		name := annotationName(source, annotation)
		if name != "RequestMapping" {
			continue
		}
		patterns := annotationPathValues(source, annotation)
		if len(patterns) == 0 {
			return "", true
		}
		return patterns[0], true
	}
	return "", false
}

func springRoutesFromAnnotations(source []byte, annotations []*tree_sitter.Node, routePrefix string) []springRoute {
	var routes []springRoute
	for _, annotation := range annotations {
		method, ok := springHTTPMethod(source, annotation)
		if !ok {
			continue
		}
		patterns := annotationPathValues(source, annotation)
		if len(patterns) == 0 {
			patterns = []string{""}
		}
		start := annotation.StartPosition()
		for _, pattern := range patterns {
			routes = append(routes, springRoute{
				Method:  method,
				Pattern: combineRoutePatterns(routePrefix, pattern),
				Line:    int(start.Row) + 1,
				Column:  int(start.Column),
			})
		}
	}
	return routes
}

func springHTTPMethod(source []byte, annotation *tree_sitter.Node) (string, bool) {
	switch annotationName(source, annotation) {
	case "GetMapping":
		return "GET", true
	case "PostMapping":
		return "POST", true
	case "PutMapping":
		return "PUT", true
	case "DeleteMapping":
		return "DELETE", true
	case "PatchMapping":
		return "PATCH", true
	case "RequestMapping":
		method := requestMappingMethod(nodeText(source, annotation))
		if method == "" {
			method = "ANY"
		}
		return method, true
	default:
		return "", false
	}
}

func requestMappingMethod(text string) string {
	upper := strings.ToUpper(text)
	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"} {
		if strings.Contains(upper, "REQUESTMETHOD."+method) || strings.Contains(upper, "METHOD="+method) {
			return method
		}
	}
	return ""
}

func annotationPathValues(source []byte, annotation *tree_sitter.Node) []string {
	values := annotationNamedPathValues(source, annotation)
	if len(values) > 0 {
		return values
	}
	return annotationPositionalPathValues(source, annotation)
}

func annotationNamedPathValues(source []byte, node *tree_sitter.Node) []string {
	var values []string
	var walk func(*tree_sitter.Node)
	walk = func(current *tree_sitter.Node) {
		if current == nil {
			return
		}
		if current.Kind() == "element_value_pair" || current.Kind() == "value_argument" {
			text := nodeText(source, current)
			if strings.Contains(text, "=") {
				key := strings.TrimSpace(strings.SplitN(text, "=", 2)[0])
				key = strings.Trim(key, " \t\r\n")
				if key == "path" || key == "value" {
					values = append(values, stringValues(source, current)...)
				}
			}
		}
		for i := uint(0); i < current.NamedChildCount(); i++ {
			walk(current.NamedChild(i))
		}
	}
	walk(node)
	return uniqueStrings(values)
}

func annotationPositionalPathValues(source []byte, node *tree_sitter.Node) []string {
	var values []string
	var walk func(*tree_sitter.Node)
	walk = func(current *tree_sitter.Node) {
		if current == nil {
			return
		}
		switch current.Kind() {
		case "annotation_argument_list", "value_arguments":
			for i := uint(0); i < current.NamedChildCount(); i++ {
				child := current.NamedChild(i)
				text := nodeText(source, child)
				if strings.Contains(text, "=") {
					continue
				}
				values = append(values, stringValues(source, child)...)
			}
		default:
			for i := uint(0); i < current.NamedChildCount(); i++ {
				walk(current.NamedChild(i))
			}
		}
	}
	walk(node)
	return uniqueStrings(values)
}

func stringValues(source []byte, node *tree_sitter.Node) []string {
	var values []string
	var walk func(*tree_sitter.Node)
	walk = func(current *tree_sitter.Node) {
		if current == nil {
			return
		}
		switch current.Kind() {
		case "string_fragment", "string_content", "string_literal_content":
			value := strings.TrimSpace(nodeText(source, current))
			if value != "" {
				values = append(values, value)
			}
			return
		case "interpreted_string_literal", "raw_string_literal":
			value := strings.Trim(nodeText(source, current), "\"`")
			if value != "" {
				values = append(values, value)
			}
			return
		case "string", "string_literal":
			if current.NamedChildCount() == 0 {
				value := strings.Trim(nodeText(source, current), "\"'`")
				if value != "" {
					values = append(values, value)
				}
				return
			}
		}
		for i := uint(0); i < current.NamedChildCount(); i++ {
			walk(current.NamedChild(i))
		}
	}
	walk(node)
	return uniqueStrings(values)
}

func directAnnotations(node *tree_sitter.Node) []*tree_sitter.Node {
	var annotations []*tree_sitter.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "annotation", "marker_annotation":
			annotations = append(annotations, child)
		case "modifiers":
			for j := uint(0); j < child.NamedChildCount(); j++ {
				modifier := child.NamedChild(j)
				if modifier.Kind() == "annotation" || modifier.Kind() == "marker_annotation" {
					annotations = append(annotations, modifier)
				}
			}
		}
	}
	return annotations
}

func annotationName(source []byte, annotation *tree_sitter.Node) string {
	for i := uint(0); i < annotation.NamedChildCount(); i++ {
		if name := firstIdentifier(source, annotation.NamedChild(i)); name != "" {
			return name
		}
	}
	return ""
}

func firstIdentifier(source []byte, node *tree_sitter.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind() == "identifier" {
		return strings.TrimSpace(nodeText(source, node))
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if name := firstIdentifier(source, node.NamedChild(i)); name != "" {
			return name
		}
	}
	return ""
}

func declarationName(source []byte, node *tree_sitter.Node) string {
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		return strings.TrimSpace(nodeText(source, nameNode))
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "identifier" {
			name := strings.TrimSpace(nodeText(source, child))
			if name != "" && name != "class" && name != "fun" {
				return name
			}
		}
	}
	return ""
}

func kotlinClassLikeName(source []byte, node *tree_sitter.Node) string {
	seenClassKeyword := false
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() != "identifier" {
			continue
		}
		text := strings.TrimSpace(nodeText(source, child))
		if text == "class" || text == "object" {
			seenClassKeyword = true
			continue
		}
		if seenClassKeyword && text != "" {
			return text
		}
	}
	return ""
}

func combineRoutePatterns(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		left = "/"
	}
	if right == "" {
		right = "/"
	}
	combined := strings.TrimRight(left, "/") + "/" + strings.TrimLeft(right, "/")
	if combined == "" || combined == "/" {
		return "/"
	}
	return combined
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func nodeText(source []byte, node *tree_sitter.Node) string {
	start := int(node.StartByte())
	end := int(node.EndByte())
	if start < 0 || end < start || end > len(source) {
		return ""
	}
	return string(source[start:end])
}
