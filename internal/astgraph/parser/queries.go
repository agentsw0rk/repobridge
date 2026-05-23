package parser

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	"repobridge/internal/astgraph/model"
)

func walkGo(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	appendGoImportNodes(path, source, result)
	appendGoInterfaceNodes(path, source, result)
	appendGoValueAliasNodes(path, source, result)
	appendGoParameterAliasNodes(path, source, result)
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
	appendTypeScriptDecoratorRoutes(path, source, result)
}

func walkPython(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkPythonNode(path, source, node, result, "", false)
}

func walkRust(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	pending := []frameworkRoute{}
	walkRustNode(path, source, node, result, "", &pending, "", "")
}

func walkJava(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkJavaNode(path, source, node, result, "", []string{""}, "")
}

func walkKotlin(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkKotlinNode(path, source, node, result, "", "", "", kotlinPackageName(source, node))
	appendKotlinKtorRoutes(path, source, result)
}

func walkCSharp(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	walkCSharpNode(path, source, node, result, "", "", "", csharpRouteGroupPrefixes(source))
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
	case "class_declaration":
		appendLanguageNode(path, source, node, model.NodeKindClass, language, result)
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
	case "class_definition":
		appendLanguageNode(path, source, node, model.NodeKindClass, model.LanguagePython, result)
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

func walkRustNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string, pendingRoutes *[]frameworkRoute, implType, enumType string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "attribute_item":
		if route, ok := rustAttributeRoute(source, node); ok {
			*pendingRoutes = append(*pendingRoutes, route)
		}
		return
	case "use_declaration":
		appendRustImportNodes(path, source, node, result)
	case "struct_item":
		appendRustStructNode(path, source, node, result)
	case "enum_item":
		enumType = declarationName(source, node)
	case "enum_variant":
		appendRustEnumVariantNode(path, source, node, result, enumType)
	case "impl_item":
		implType = rustImplType(source, node)
	case "function_item":
		if len(*pendingRoutes) > 0 {
			if id := appendRustAttributedHandler(path, source, node, result, *pendingRoutes); id != "" {
				currentNodeID = id
			}
			*pendingRoutes = nil
		} else if id := appendRustFunctionOrMethodNode(path, source, node, result, implType); id != "" {
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
		walkRustNode(path, source, node.NamedChild(i), result, currentNodeID, pendingRoutes, implType, enumType)
	}
}

func walkJavaNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID string, routePrefixes []string, className string) {
	if node == nil {
		return
	}
	if len(routePrefixes) == 0 {
		routePrefixes = []string{""}
	}

	switch node.Kind() {
	case "class_declaration":
		if name := declarationName(source, node); name != "" {
			className = name
		}
		appendScopedLanguageNode(path, source, node, model.NodeKindClass, model.LanguageJava, result, "")
		if prefixes, ok := springClassRoutePrefixes(source, node); ok {
			routePrefixes = combineRoutePatternLists(routePrefixes, prefixes)
		} else if prefixes, ok := springComposedRoutePrefixesFromAnnotations(source, directAnnotations(node)); ok {
			routePrefixes = combineRoutePatternLists(routePrefixes, prefixes)
		}
	case "interface_declaration":
		appendScopedLanguageNode(path, source, node, model.NodeKindInterface, model.LanguageJava, result, "")
	case "enum_declaration":
		appendScopedLanguageNode(path, source, node, model.NodeKindEnum, model.LanguageJava, result, "")
	case "method_declaration":
		if id := appendSpringHandlerOrJavaMethod(path, source, node, result, routePrefixes, className); id != "" {
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
		walkJavaNode(path, source, node.NamedChild(i), result, currentNodeID, routePrefixes, className)
	}
}

func walkKotlinNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID, routePrefix, className, packageName string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "annotated_expression":
		if prefix, ok := springRoutePrefixFromAnnotations(source, directAnnotations(node)); ok {
			routePrefix = combineRoutePatterns(routePrefix, prefix)
		}
	case "import":
		appendKotlinImportNode(path, source, node, result)
	case "class_declaration", "object_declaration":
		if name := declarationName(source, node); name != "" {
			className = name
			appendKotlinClassNode(path, source, node, result, name, packageName)
		}
	case "infix_expression":
		if name := kotlinClassLikeName(source, node); name != "" {
			className = name
		}
	case "type_alias", "typealias":
		if name := declarationName(source, node); name != "" {
			appendKotlinClassNode(path, source, node, result, name, packageName)
		}
	case "function_declaration":
		if id := appendSpringHandlerOrKotlinFunction(path, source, node, result, routePrefix, className, packageName); id != "" {
			currentNodeID = id
		}
	case "anonymous_function", "lambda_literal":
		if !kotlinLambdaInheritsOwner(source, node) {
			currentNodeID = ""
		}
	case "call_expression":
		if currentNodeID != "" {
			appendLanguageCall(path, source, node, model.LanguageKotlin, result, currentNodeID)
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkKotlinNode(path, source, node.NamedChild(i), result, currentNodeID, routePrefix, className, packageName)
	}
}

// kotlinLambdaInheritsOwner reports whether calls inside a lambda should be
// attributed to the enclosing function. A lambda passed as a call argument
// (trailing `foo { ... }` or explicit `foo({ ... })`) executes within the
// caller, so its calls belong to the enclosing owner. A lambda that is stored
// or returned escapes that scope, so its calls keep being dropped.
func kotlinLambdaInheritsOwner(source []byte, node *tree_sitter.Node) bool {
	if kotlinDeferredLambdaArgument(source, node) {
		return false
	}
	parent := node.Parent()
	if parent == nil {
		return false
	}
	switch parent.Kind() {
	case "annotated_lambda", "value_argument":
		return true
	default:
		return false
	}
}

func kotlinDeferredLambdaArgument(source []byte, node *tree_sitter.Node) bool {
	for current := node.Parent(); current != nil; current = current.Parent() {
		if current.Kind() != "call_expression" {
			continue
		}
		details, ok := callReference(source, current)
		if !ok {
			return false
		}
		switch details.name {
		case "launch", "async":
			return true
		default:
			return false
		}
	}
	return false
}

func walkCSharpNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, currentNodeID, routePrefix, className string, routePrefixes map[string]string) {
	if node == nil {
		return
	}

	switch node.Kind() {
	case "class_declaration":
		if name := declarationName(source, node); name != "" {
			className = name
			appendScopedLanguageNode(path, source, node, model.NodeKindClass, model.LanguageCSharp, result, "")
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
		if !appendCSharpMinimalAPIRoute(path, source, node, result, routePrefixes) && currentNodeID != "" {
			appendLanguageCall(path, source, node, model.LanguageCSharp, result, currentNodeID)
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		walkCSharpNode(path, source, node.NamedChild(i), result, currentNodeID, routePrefix, className, routePrefixes)
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
	case "type_conversion_expression":
		if currentNodeID != "" {
			appendGoGenericConversionCall(path, source, node, result, currentNodeID)
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
	metadata := nodeSignatureMetadata(source, node, name)
	if kind == model.NodeKindMethod {
		metadata.receiverType = goReceiverType(source, node)
	}
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	id := stableNodeID(path, kind, name, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:             id,
		Kind:           kind,
		Name:           name,
		QualifiedName:  qualifiedMemberName(metadata.receiverType, name),
		ReceiverType:   metadata.receiverType,
		ParameterCount: metadata.parameterCount,
		ParameterTypes: metadata.parameterTypes,
		ReturnType:     metadata.returnType,
		FilePath:       path,
		Language:       model.LanguageGo,
		StartLine:      startLine,
		EndLine:        int(end.Row) + 1,
		StartColumn:    int(start.Column),
		EndColumn:      int(end.Column),
		Signature:      strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

type goImportSpec struct {
	alias string
	path  string
	line  int
}

func appendGoImportNodes(path string, source []byte, result *ExtractionResult) {
	for _, spec := range goImportSpecs(source) {
		if spec.alias == "" || spec.alias == "_" || spec.alias == "." || spec.path == "" {
			continue
		}
		id := stableNodeID(path, model.NodeKindImport, spec.path, spec.line)
		result.Nodes = append(result.Nodes, model.GraphNode{
			ID:            id,
			Kind:          model.NodeKindImport,
			Name:          spec.alias,
			QualifiedName: spec.path,
			FilePath:      path,
			Language:      model.LanguageGo,
			StartLine:     spec.line,
			EndLine:       spec.line,
			Signature:     fmt.Sprintf("import %s %q", spec.alias, spec.path),
		})
	}
}

type goInterface struct {
	name      string
	line      int
	signature string
	methods   []goInterfaceMethod
}

type goInterfaceMethod struct {
	name           string
	line           int
	signature      string
	parameterTypes []string
	returnType     string
}

func appendGoInterfaceNodes(path string, source []byte, result *ExtractionResult) {
	for _, iface := range goInterfaces(source) {
		if iface.name == "" {
			continue
		}
		interfaceID := stableNodeID(path, model.NodeKindInterface, iface.name, iface.line)
		result.Nodes = append(result.Nodes, model.GraphNode{
			ID:            interfaceID,
			Kind:          model.NodeKindInterface,
			Name:          iface.name,
			QualifiedName: iface.name,
			FilePath:      path,
			Language:      model.LanguageGo,
			StartLine:     iface.line,
			EndLine:       iface.line,
			Signature:     iface.signature,
		})
		for _, method := range iface.methods {
			if method.name == "" {
				continue
			}
			methodID := stableNodeID(path, model.NodeKindMethod, iface.name+"."+method.name, method.line)
			result.Nodes = append(result.Nodes, model.GraphNode{
				ID:             methodID,
				Kind:           model.NodeKindMethod,
				Name:           method.name,
				QualifiedName:  qualifiedMemberName(iface.name, method.name),
				ReceiverType:   iface.name,
				ParameterCount: len(method.parameterTypes),
				ParameterTypes: method.parameterTypes,
				ReturnType:     method.returnType,
				FilePath:       path,
				Language:       model.LanguageGo,
				StartLine:      method.line,
				EndLine:        method.line,
				Signature:      method.signature,
			})
		}
	}
}

type goValueAlias struct {
	name      string
	typ       string
	source    string
	line      int
	signature string
}

func appendGoValueAliasNodes(path string, source []byte, result *ExtractionResult) {
	for _, alias := range goValueAliases(source) {
		if alias.name == "" || alias.typ == "" {
			continue
		}
		id := stableNodeID(path, model.NodeKindVariable, alias.name, alias.line)
		result.Nodes = append(result.Nodes, model.GraphNode{
			ID:            id,
			Kind:          model.NodeKindVariable,
			Name:          alias.name,
			QualifiedName: alias.typ,
			ReceiverType:  alias.typ,
			ReturnType:    alias.source,
			FilePath:      path,
			Language:      model.LanguageGo,
			StartLine:     alias.line,
			EndLine:       alias.line,
			Signature:     alias.signature,
		})
	}
}

func appendGoParameterAliasNodes(path string, source []byte, result *ExtractionResult) {
	for _, alias := range goFunctionParameterAliases(source) {
		if alias.name == "" || alias.typ == "" {
			continue
		}
		id := stableNodeID(path, model.NodeKindVariable, alias.name, alias.line)
		result.Nodes = append(result.Nodes, model.GraphNode{
			ID:            id,
			Kind:          model.NodeKindVariable,
			Name:          alias.name,
			QualifiedName: alias.typ,
			ReceiverType:  alias.typ,
			ReturnType:    alias.source,
			FilePath:      path,
			Language:      model.LanguageGo,
			StartLine:     alias.line,
			EndLine:       alias.line,
			Signature:     alias.signature,
		})
	}
}

func goValueAliases(source []byte) []goValueAlias {
	lines := strings.Split(string(source), "\n")
	packageAliases := map[string]string{}
	var aliases []goValueAlias
	inVarBlock := false
	for index, line := range lines {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(stripLineComment(line))
		if trimmed == "" {
			continue
		}
		if inVarBlock {
			if strings.HasPrefix(trimmed, ")") {
				inVarBlock = false
				continue
			}
			if alias, ok := parseGoVarAlias(trimmed, lineNumber, packageAliases); ok {
				aliases = append(aliases, alias)
				packageAliases[alias.name] = alias.typ
			}
			continue
		}
		if strings.HasPrefix(trimmed, "var (") {
			inVarBlock = true
			continue
		}
		if strings.HasPrefix(trimmed, "var ") {
			if alias, ok := parseGoVarAlias(strings.TrimSpace(strings.TrimPrefix(trimmed, "var ")), lineNumber, packageAliases); ok {
				aliases = append(aliases, alias)
				packageAliases[alias.name] = alias.typ
			}
			continue
		}
		if alias, ok := parseGoShortAlias(trimmed, lineNumber, packageAliases); ok {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}

func parseGoVarAlias(text string, line int, known map[string]string) (goValueAlias, bool) {
	name, rhs, ok := splitGoAssignment(text, "=")
	if !ok {
		return goValueAlias{}, false
	}
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return goValueAlias{}, false
	}
	aliasName := fields[0]
	if strings.Contains(aliasName, ",") {
		return goValueAlias{}, false
	}
	typ := goAliasType(rhs, known)
	if typ == "" && len(fields) > 1 {
		typ = strings.TrimPrefix(strings.TrimSpace(fields[len(fields)-1]), "*")
	}
	if typ == "" {
		typ = goIdentifierAlias(rhs)
	}
	if typ == "" {
		return goValueAlias{}, false
	}
	return goValueAlias{name: aliasName, typ: typ, source: strings.TrimSpace(rhs), line: line, signature: strings.TrimSpace(text)}, true
}

func parseGoShortAlias(text string, line int, known map[string]string) (goValueAlias, bool) {
	name, rhs, ok := splitGoAssignment(text, ":=")
	if !ok {
		return goValueAlias{}, false
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, ",") {
		return goValueAlias{}, false
	}
	typ := goAliasType(rhs, known)
	if typ == "" {
		typ = goCallReturnAlias(rhs)
	}
	if typ == "" {
		typ = goIdentifierAlias(rhs)
	}
	if typ == "" {
		return goValueAlias{}, false
	}
	return goValueAlias{name: name, typ: typ, source: strings.TrimSpace(rhs), line: line, signature: strings.TrimSpace(text)}, true
}

func goInterfaces(source []byte) []goInterface {
	lines := strings.Split(string(source), "\n")
	var interfaces []goInterface
	var current *goInterface
	for index, line := range lines {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(stripLineComment(line))
		if trimmed == "" {
			continue
		}
		if current != nil {
			if before, _, ok := strings.Cut(trimmed, "}"); ok {
				for _, methodText := range splitGoInterfaceMethodTexts(before) {
					if method, ok := parseGoInterfaceMethod(methodText, lineNumber); ok {
						current.methods = append(current.methods, method)
					}
				}
				interfaces = append(interfaces, *current)
				current = nil
				continue
			}
			if method, ok := parseGoInterfaceMethod(trimmed, lineNumber); ok {
				current.methods = append(current.methods, method)
			}
			continue
		}
		name, after, ok := parseGoInterfaceStart(trimmed)
		if !ok {
			continue
		}
		iface := goInterface{name: name, line: lineNumber, signature: trimmed}
		if before, _, closed := strings.Cut(after, "}"); closed {
			for _, methodText := range splitGoInterfaceMethodTexts(before) {
				if method, ok := parseGoInterfaceMethod(methodText, lineNumber); ok {
					iface.methods = append(iface.methods, method)
				}
			}
			interfaces = append(interfaces, iface)
			continue
		}
		for _, methodText := range splitGoInterfaceMethodTexts(after) {
			if method, ok := parseGoInterfaceMethod(methodText, lineNumber); ok {
				iface.methods = append(iface.methods, method)
			}
		}
		current = &iface
	}
	if current != nil {
		interfaces = append(interfaces, *current)
	}
	return interfaces
}

func parseGoInterfaceStart(text string) (string, string, bool) {
	if !strings.HasPrefix(text, "type ") || !strings.Contains(text, " interface") {
		return "", "", false
	}
	afterType := strings.TrimSpace(strings.TrimPrefix(text, "type "))
	fields := strings.Fields(afterType)
	if len(fields) < 2 || !strings.HasPrefix(fields[1], "interface") {
		return "", "", false
	}
	open := strings.Index(text, "{")
	if open < 0 {
		return fields[0], "", true
	}
	return fields[0], strings.TrimSpace(text[open+1:]), true
}

func splitGoInterfaceMethodTexts(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var values []string
	for _, part := range splitTopLevel(text, ';') {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func parseGoInterfaceMethod(text string, line int) (goInterfaceMethod, bool) {
	text = strings.TrimSpace(text)
	if text == "" || strings.Contains(text, " ") && !strings.Contains(text, "(") {
		return goInterfaceMethod{}, false
	}
	open := strings.Index(text, "(")
	if open <= 0 {
		return goInterfaceMethod{}, false
	}
	name := strings.TrimSpace(text[:open])
	if name == "" || strings.ContainsAny(name, " \t") {
		return goInterfaceMethod{}, false
	}
	close := matchingParen(text, open)
	if close < 0 {
		return goInterfaceMethod{}, false
	}
	parameters := text[open+1 : close]
	returnType := strings.TrimSpace(text[close+1:])
	return goInterfaceMethod{
		name:           name,
		line:           line,
		signature:      text,
		parameterTypes: normalizeGoTypeList(parameterTypesFromList(parameters)),
		returnType:     normalizeGoTypeName(returnType),
	}, true
}

func goFunctionParameterAliases(source []byte) []goValueAlias {
	lines := strings.Split(string(source), "\n")
	var aliases []goValueAlias
	for index := 0; index < len(lines); index++ {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(stripLineComment(lines[index]))
		if !strings.HasPrefix(trimmed, "func ") && !strings.HasPrefix(trimmed, "func(") {
			continue
		}
		header := trimmed
		for !strings.Contains(header, "{") && index+1 < len(lines) {
			index++
			header += " " + strings.TrimSpace(stripLineComment(lines[index]))
		}
		if before, _, ok := strings.Cut(header, "{"); ok {
			header = strings.TrimSpace(before)
		}
		_, parameters, ok := goFunctionHeaderNameAndParams(header)
		if !ok {
			continue
		}
		for _, alias := range goParameterAliasesFromList(parameters, lineNumber) {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}

func goFunctionHeaderNameAndParams(header string) (string, string, bool) {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "func") {
		return "", "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(header, "func"))
	if strings.HasPrefix(rest, "(") {
		close := matchingParen(rest, 0)
		if close < 0 || close+1 >= len(rest) {
			return "", "", false
		}
		rest = strings.TrimSpace(rest[close+1:])
	}
	nameEnd := 0
	for nameEnd < len(rest) {
		r := rest[nameEnd]
		if !(r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			break
		}
		nameEnd++
	}
	if nameEnd == 0 {
		return "", "", false
	}
	name := rest[:nameEnd]
	afterName := strings.TrimSpace(rest[nameEnd:])
	if !strings.HasPrefix(afterName, "(") {
		return "", "", false
	}
	close := matchingParen(afterName, 0)
	if close < 0 {
		return "", "", false
	}
	return name, afterName[1:close], true
}

func goParameterAliasesFromList(parameters string, line int) []goValueAlias {
	var aliases []goValueAlias
	for _, part := range splitTopLevel(parameters, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		typ := normalizeGoTypeName(fields[len(fields)-1])
		for _, name := range fields[:len(fields)-1] {
			name = strings.Trim(strings.TrimSpace(name), ",")
			if name == "" || name == "_" || strings.ContainsAny(name, "*[]{}().") {
				continue
			}
			aliases = append(aliases, goValueAlias{
				name:      name,
				typ:       typ,
				source:    typ,
				line:      line,
				signature: fmt.Sprintf("param %s %s", name, typ),
			})
		}
	}
	return aliases
}

func splitGoAssignment(text, operator string) (string, string, bool) {
	left, right, ok := strings.Cut(text, operator)
	if !ok {
		return "", "", false
	}
	right = strings.TrimSpace(right)
	if i := strings.IndexAny(right, ";"); i >= 0 {
		right = strings.TrimSpace(right[:i])
	}
	return strings.TrimSpace(left), right, true
}

func goAliasType(expr string, known map[string]string) string {
	expr = strings.TrimSpace(expr)
	expr = strings.Trim(expr, "()")
	expr = strings.TrimPrefix(expr, "*")
	expr = strings.TrimSpace(expr)
	if before, _, ok := strings.Cut(expr, "{"); ok {
		return normalizeGoTypeName(before)
	}
	if typ, ok := known[expr]; ok {
		return typ
	}
	return ""
}

func goCallReturnAlias(expr string) string {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimPrefix(expr, "&")
	expr = strings.TrimSpace(expr)
	open := strings.Index(expr, "(")
	if open <= 0 {
		return ""
	}
	callee := strings.TrimSpace(expr[:open])
	if callee == "" || strings.ContainsAny(callee, " \t{}[]+-*/%!&|<>=,:") {
		return ""
	}
	return "return:" + callee
}

func goIdentifierAlias(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" || strings.ContainsAny(expr, " .(){}[]+-*/%!&|<>=,:") {
		return ""
	}
	return expr
}

func normalizeGoTypeName(typ string) string {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		return ""
	}
	if before, _, ok := strings.Cut(typ, "`"); ok {
		typ = strings.TrimSpace(before)
	}
	typ = strings.TrimSuffix(typ, ",")
	typ = strings.TrimSpace(typ)
	if strings.HasPrefix(typ, "(") {
		if close := matchingParen(typ, 0); close == len(typ)-1 {
			inner := strings.TrimSpace(typ[1:close])
			parts := splitTopLevel(inner, ',')
			if len(parts) > 0 {
				typ = strings.TrimSpace(parts[0])
			}
		}
	}
	for {
		typ = strings.TrimSpace(typ)
		typ = strings.TrimPrefix(typ, "*")
		typ = strings.TrimPrefix(typ, "&")
		typ = strings.TrimPrefix(typ, "...")
		if strings.HasPrefix(typ, "[]") {
			typ = strings.TrimSpace(strings.TrimPrefix(typ, "[]"))
			continue
		}
		if strings.HasPrefix(typ, "[") {
			close := matchingBracket(typ, 0)
			if close >= 0 && close+1 < len(typ) {
				typ = strings.TrimSpace(typ[close+1:])
				continue
			}
		}
		if strings.HasPrefix(typ, "map[") {
			open := strings.Index(typ, "[")
			close := matchingBracket(typ, open)
			if close >= 0 && close+1 < len(typ) {
				typ = strings.TrimSpace(typ[close+1:])
				continue
			}
		}
		break
	}
	if fields := strings.Fields(typ); len(fields) > 1 {
		return normalizeGoTypeName(fields[len(fields)-1])
	}
	return typ
}

func normalizeGoTypeList(types []string) []string {
	if len(types) == 0 {
		return nil
	}
	out := make([]string, 0, len(types))
	for _, typ := range types {
		normalized := normalizeGoTypeName(typ)
		if normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

func matchingBracket(text string, open int) int {
	if open < 0 || open >= len(text) || text[open] != '[' {
		return -1
	}
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func goImportSpecs(source []byte) []goImportSpec {
	var specs []goImportSpec
	inBlock := false
	for index, line := range strings.Split(string(source), "\n") {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(stripLineComment(line))
		if trimmed == "" {
			continue
		}
		if inBlock {
			if strings.HasPrefix(trimmed, ")") {
				inBlock = false
				continue
			}
			if spec, ok := parseGoImportSpec(trimmed, lineNumber); ok {
				specs = append(specs, spec)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "import (") {
			inBlock = true
			after := strings.TrimSpace(strings.TrimPrefix(trimmed, "import ("))
			if after != "" && after != ")" {
				if before, _, ok := strings.Cut(after, ")"); ok {
					after = strings.TrimSpace(before)
					inBlock = false
				}
				if spec, ok := parseGoImportSpec(after, lineNumber); ok {
					specs = append(specs, spec)
				}
			}
			continue
		}
		if strings.HasPrefix(trimmed, "import ") {
			specText := strings.TrimSpace(strings.TrimPrefix(trimmed, "import "))
			if spec, ok := parseGoImportSpec(specText, lineNumber); ok {
				specs = append(specs, spec)
			}
		}
	}
	return specs
}

func parseGoImportSpec(spec string, line int) (goImportSpec, bool) {
	firstQuote := strings.Index(spec, `"`)
	if firstQuote < 0 {
		return goImportSpec{}, false
	}
	secondQuote := strings.Index(spec[firstQuote+1:], `"`)
	if secondQuote < 0 {
		return goImportSpec{}, false
	}
	secondQuote += firstQuote + 1
	importPath := strings.TrimSpace(spec[firstQuote+1 : secondQuote])
	if importPath == "" {
		return goImportSpec{}, false
	}
	alias := ""
	if prefix := strings.TrimSpace(spec[:firstQuote]); prefix != "" {
		fields := strings.Fields(prefix)
		if len(fields) > 0 {
			alias = fields[len(fields)-1]
		}
	}
	if alias == "" {
		alias = goDefaultImportAlias(importPath)
	}
	return goImportSpec{alias: alias, path: importPath, line: line}, true
}

func goDefaultImportAlias(importPath string) string {
	importPath = strings.Trim(importPath, "/")
	if importPath == "" {
		return ""
	}
	if i := strings.LastIndex(importPath, "/"); i >= 0 {
		return importPath[i+1:]
	}
	return importPath
}

func stripLineComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}

func appendLanguageNode(path string, source []byte, node *tree_sitter.Node, kind model.NodeKind, language model.Language, result *ExtractionResult) string {
	return appendScopedLanguageNode(path, source, node, kind, language, result, "")
}

func appendScopedLanguageNode(path string, source []byte, node *tree_sitter.Node, kind model.NodeKind, language model.Language, result *ExtractionResult, receiverType string) string {
	return appendPackageScopedLanguageNode(path, source, node, kind, language, result, receiverType, "")
}

func appendPackageScopedLanguageNode(path string, source []byte, node *tree_sitter.Node, kind model.NodeKind, language model.Language, result *ExtractionResult, receiverType, packageName string) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	name := nodeText(source, nameNode)
	metadata := nodeSignatureMetadata(source, node, name)
	if receiverType != "" {
		metadata.receiverType = receiverType
	}
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	qualifiedName := packageQualifiedName(packageName, qualifiedMemberName(metadata.receiverType, name))
	id := stableNodeID(path, kind, qualifiedName, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:             id,
		Kind:           kind,
		Name:           name,
		QualifiedName:  qualifiedName,
		ReceiverType:   metadata.receiverType,
		ParameterCount: metadata.parameterCount,
		ParameterTypes: metadata.parameterTypes,
		ReturnType:     metadata.returnType,
		FilePath:       path,
		Language:       language,
		StartLine:      startLine,
		EndLine:        int(end.Row) + 1,
		StartColumn:    int(start.Column),
		EndColumn:      int(end.Column),
		Signature:      strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func appendRustFunctionOrMethodNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, receiverType string) string {
	name := declarationName(source, node)
	if name == "" {
		return ""
	}
	kind := model.NodeKindFunction
	qualifiedName := name
	if receiverType != "" {
		kind = model.NodeKindMethod
		qualifiedName = receiverType + "::" + name
	}
	metadata := rustFunctionMetadata(source, node, name)
	metadata.receiverType = receiverType
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	id := stableNodeID(path, kind, qualifiedName, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:             id,
		Kind:           kind,
		Name:           name,
		QualifiedName:  qualifiedName,
		ReceiverType:   metadata.receiverType,
		ParameterCount: metadata.parameterCount,
		ParameterTypes: metadata.parameterTypes,
		ReturnType:     metadata.returnType,
		FilePath:       path,
		Language:       model.LanguageRust,
		StartLine:      startLine,
		EndLine:        int(end.Row) + 1,
		StartColumn:    int(start.Column),
		EndColumn:      int(end.Column),
		Signature:      strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func appendRustStructNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) string {
	name := declarationName(source, node)
	if name == "" {
		return ""
	}
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	id := stableNodeID(path, model.NodeKindStruct, name, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:             id,
		Kind:           model.NodeKindStruct,
		Name:           name,
		QualifiedName:  name,
		ParameterCount: -1,
		FilePath:       path,
		Language:       model.LanguageRust,
		StartLine:      startLine,
		EndLine:        int(end.Row) + 1,
		StartColumn:    int(start.Column),
		EndColumn:      int(end.Column),
		Signature:      strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func appendRustEnumVariantNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, enumType string) string {
	if enumType == "" {
		return ""
	}
	name := declarationName(source, node)
	if name == "" {
		return ""
	}
	metadata := rustEnumVariantMetadata(source, node, name)
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	qualifiedName := enumType + "::" + name
	id := stableNodeID(path, model.NodeKindClass, qualifiedName, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:             id,
		Kind:           model.NodeKindClass,
		Name:           name,
		QualifiedName:  qualifiedName,
		ReceiverType:   enumType,
		ParameterCount: metadata.parameterCount,
		ParameterTypes: metadata.parameterTypes,
		FilePath:       path,
		Language:       model.LanguageRust,
		StartLine:      startLine,
		EndLine:        int(end.Row) + 1,
		StartColumn:    int(start.Column),
		EndColumn:      int(end.Column),
		Signature:      strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func appendRustImportNodes(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) {
	start := node.StartPosition()
	line := int(start.Row) + 1
	column := int(start.Column)
	for _, importPath := range expandRustUsePaths(rustUseSpec(nodeText(source, node))) {
		name := rustImportName(importPath)
		qualifiedName := rustImportQualifiedName(importPath)
		if name == "" {
			continue
		}
		id := stableNodeID(path, model.NodeKindImport, qualifiedName, line)
		result.Nodes = append(result.Nodes, model.GraphNode{
			ID:            id,
			Kind:          model.NodeKindImport,
			Name:          name,
			QualifiedName: qualifiedName,
			FilePath:      path,
			Language:      model.LanguageRust,
			StartLine:     line,
			EndLine:       line,
			StartColumn:   column,
			EndColumn:     column,
			Signature:     strings.TrimSpace(nodeText(source, node)),
		})
	}
}

func rustUseSpec(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "pub ")
	text = strings.TrimPrefix(text, "use ")
	text = strings.TrimSuffix(text, ";")
	return strings.TrimSpace(text)
}

func expandRustUsePaths(spec string) []string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	open := strings.Index(spec, "{")
	if open < 0 {
		return []string{rustCanonicalImportPath(spec)}
	}
	close := matchingBrace(spec, open)
	if close < 0 {
		return []string{rustCanonicalImportPath(spec)}
	}
	prefix := strings.TrimSuffix(strings.TrimSpace(spec[:open]), "::")
	suffix := strings.TrimSpace(spec[close+1:])
	var paths []string
	for _, part := range splitTopLevel(spec[open+1:close], ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for _, expanded := range expandRustUsePaths(part) {
			if expanded == "self" {
				paths = append(paths, rustCanonicalImportPath(prefix+suffix))
				continue
			}
			paths = append(paths, rustCanonicalImportPath(prefix+"::"+expanded+suffix))
		}
	}
	return paths
}

func matchingBrace(text string, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func rustCanonicalImportPath(importPath string) string {
	importPath = strings.TrimSpace(importPath)
	if before, after, ok := strings.Cut(importPath, " as "); ok {
		return strings.TrimSpace(before) + " as " + strings.TrimSpace(after)
	}
	return importPath
}

func rustImportName(importPath string) string {
	importPath = strings.TrimSpace(importPath)
	if _, alias, ok := strings.Cut(importPath, " as "); ok {
		return strings.TrimSpace(alias)
	}
	if strings.HasSuffix(importPath, "::*") {
		return "*"
	}
	if i := strings.LastIndex(importPath, "::"); i >= 0 {
		return strings.TrimSpace(importPath[i+2:])
	}
	return strings.TrimSpace(importPath)
}

func rustImportQualifiedName(importPath string) string {
	importPath = strings.TrimSpace(importPath)
	if before, _, ok := strings.Cut(importPath, " as "); ok {
		return strings.TrimSpace(before)
	}
	return importPath
}

func appendKotlinClassNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, name, packageName string) string {
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	qualifiedName := packageQualifiedName(packageName, name)
	kind := kotlinTypeNodeKind(source, node)
	id := stableNodeID(path, kind, qualifiedName, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:             id,
		Kind:           kind,
		Name:           name,
		QualifiedName:  qualifiedName,
		ParameterCount: -1,
		FilePath:       path,
		Language:       model.LanguageKotlin,
		StartLine:      startLine,
		EndLine:        int(end.Row) + 1,
		StartColumn:    int(start.Column),
		EndColumn:      int(end.Column),
		Signature:      strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func kotlinTypeNodeKind(source []byte, node *tree_sitter.Node) model.NodeKind {
	if node.Kind() != "class_declaration" {
		return model.NodeKindClass
	}
	fields := strings.Fields(declarationHeader(source, node))
	for _, field := range fields {
		switch field {
		case "interface":
			return model.NodeKindInterface
		case "class":
			return model.NodeKindClass
		}
	}
	return model.NodeKindClass
}

func appendKotlinImportNode(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult) string {
	qualifiedName := kotlinImportPath(source, node)
	if qualifiedName == "" {
		return ""
	}
	name := importSimpleName(qualifiedName)
	start := node.StartPosition()
	end := node.EndPosition()
	startLine := int(start.Row) + 1
	id := stableNodeID(path, model.NodeKindImport, qualifiedName, startLine)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            id,
		Kind:          model.NodeKindImport,
		Name:          name,
		QualifiedName: qualifiedName,
		FilePath:      path,
		Language:      model.LanguageKotlin,
		StartLine:     startLine,
		EndLine:       int(end.Row) + 1,
		StartColumn:   int(start.Column),
		EndColumn:     int(end.Column),
		Signature:     strings.TrimSpace(nodeText(source, node)),
	})
	return id
}

func appendSpringHandlerOrJavaMethod(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, routePrefixes []string, className string) string {
	var routes []springRoute
	for _, routePrefix := range routePrefixes {
		routes = append(routes, springRoutesFromAnnotations(source, directAnnotations(node), routePrefix)...)
	}
	if len(routes) == 0 {
		for _, routePrefix := range routePrefixes {
			routes = append(routes, springComposedRoutesFromAnnotations(source, directAnnotations(node), routePrefix)...)
		}
	}
	if len(routes) == 0 {
		return appendScopedLanguageNode(path, source, node, model.NodeKindMethod, model.LanguageJava, result, className)
	}
	return appendSpringHandler(path, source, node, result, model.LanguageJava, className, routes)
}

func appendSpringHandlerOrKotlinFunction(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, routePrefix, className, packageName string) string {
	routes := springRoutesFromAnnotations(source, directAnnotations(node), routePrefix)
	if len(routes) == 0 {
		return appendPackageScopedLanguageNode(path, source, node, model.NodeKindFunction, model.LanguageKotlin, result, className, packageName)
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
		Language:       language,
		StartLine:      startLine,
		EndLine:        int(end.Row) + 1,
		StartColumn:    int(start.Column),
		EndColumn:      int(end.Column),
		Signature:      strings.TrimSpace(nodeText(source, node)),
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

	details, ok := goCallReference(source, node, functionNode)
	if !ok {
		return
	}

	start := functionNode.StartPosition()
	result.Unresolved = append(result.Unresolved, model.UnresolvedReference{
		FromNodeID:    fromNodeID,
		ReferenceName: details.name,
		ReceiverText:  details.receiverText,
		ArgumentCount: len(details.argumentTexts),
		ArgumentTexts: details.argumentTexts,
		ScopeNodeID:   fromNodeID,
		ReferenceKind: model.EdgeKindCalls,
		FilePath:      path,
		Language:      model.LanguageGo,
		Line:          int(start.Row) + 1,
		Column:        int(start.Column),
	})
}

func appendGoGenericConversionCall(path string, source []byte, node *tree_sitter.Node, result *ExtractionResult, fromNodeID string) {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil || typeNode.Kind() != "generic_type" {
		return
	}
	name, receiver, ok := goGenericReference(source, typeNode)
	if !ok {
		return
	}
	args := []string{}
	if operand := node.ChildByFieldName("operand"); operand != nil {
		if text := strings.TrimSpace(nodeText(source, operand)); text != "" {
			args = append(args, text)
		}
	}
	start := typeNode.StartPosition()
	result.Unresolved = append(result.Unresolved, model.UnresolvedReference{
		FromNodeID:    fromNodeID,
		ReferenceName: name,
		ReceiverText:  receiver,
		ArgumentCount: len(args),
		ArgumentTexts: args,
		ScopeNodeID:   fromNodeID,
		ReferenceKind: model.EdgeKindCalls,
		FilePath:      path,
		Language:      model.LanguageGo,
		Line:          int(start.Row) + 1,
		Column:        int(start.Column),
	})
}

func appendLanguageCall(path string, source []byte, node *tree_sitter.Node, language model.Language, result *ExtractionResult, fromNodeID string) {
	details, ok := callReference(source, node)
	if !ok {
		return
	}
	if language == model.LanguageCSharp {
		details.name = normalizeCSharpCallName(details.name)
		if shouldSkipCSharpCall(details.name) {
			return
		}
	}
	if details.receiverText == "" && isFunctionTypedParameterCall(result, fromNodeID, details.name) {
		return
	}

	start := details.nameNode.StartPosition()
	result.Unresolved = append(result.Unresolved, model.UnresolvedReference{
		FromNodeID:    fromNodeID,
		ReferenceName: details.name,
		ReceiverText:  details.receiverText,
		ArgumentCount: len(details.argumentTexts),
		ArgumentTexts: details.argumentTexts,
		ScopeNodeID:   fromNodeID,
		ReferenceKind: model.EdgeKindCalls,
		FilePath:      path,
		Language:      language,
		Line:          int(start.Row) + 1,
		Column:        int(start.Column),
	})
}

func normalizeCSharpCallName(name string) string {
	name = strings.TrimSpace(name)
	if before, ok := trimTrailingGenericArguments(name); ok {
		return before
	}
	return name
}

func shouldSkipCSharpCall(name string) bool {
	return name == "nameof"
}

func trimTrailingGenericArguments(name string) (string, bool) {
	if !strings.HasSuffix(name, ">") {
		return "", false
	}
	depth := 0
	for i := len(name) - 1; i >= 0; i-- {
		switch name[i] {
		case '>':
			depth++
		case '<':
			depth--
			if depth == 0 {
				prefix := strings.TrimSpace(name[:i])
				return prefix, prefix != ""
			}
		}
	}
	return "", false
}

type callDetails struct {
	nameNode      *tree_sitter.Node
	name          string
	receiverText  string
	argumentTexts []string
}

func callReference(source []byte, node *tree_sitter.Node) (callDetails, bool) {
	for _, field := range []string{"function", "expression", "name"} {
		callee := node.ChildByFieldName(field)
		if callee == nil {
			continue
		}
		nameNode, name, receiver, ok := referenceName(source, callee)
		if ok {
			return callDetails{nameNode: nameNode, name: name, receiverText: receiver, argumentTexts: argumentTexts(source, node)}, true
		}
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		nameNode, name, receiver, ok := referenceName(source, node.NamedChild(i))
		if ok {
			return callDetails{nameNode: nameNode, name: name, receiverText: receiver, argumentTexts: argumentTexts(source, node)}, true
		}
	}
	return callDetails{}, false
}

func referenceName(source []byte, node *tree_sitter.Node) (*tree_sitter.Node, string, string, bool) {
	switch node.Kind() {
	case "identifier", "property_identifier", "field_identifier":
		name := strings.TrimSpace(nodeText(source, node))
		return node, name, "", name != ""
	case "generic_name":
		name := strings.TrimSpace(nodeText(source, node))
		return node, name, "", name != ""
	case "navigation_expression":
		for i := node.NamedChildCount(); i > 0; i-- {
			child := node.NamedChild(i - 1)
			nameNode, name, _, ok := referenceName(source, child)
			if ok {
				return nameNode, name, receiverTextBeforeName(source, node, nameNode), true
			}
		}
		return nil, "", "", false
	case "attribute", "field_expression", "member_access_expression", "member_expression", "scoped_identifier", "selector_expression":
		for _, field := range []string{"name", "field", "attribute", "property"} {
			child := node.ChildByFieldName(field)
			if child == nil {
				continue
			}
			nameNode, name, _, ok := referenceName(source, child)
			if ok {
				return nameNode, name, receiverText(source, node, name), true
			}
		}
		name := strings.TrimSpace(nodeText(source, node))
		if i := strings.LastIndexAny(name, ".:"); i >= 0 {
			name = strings.TrimSpace(name[i+1:])
		}
		if name == "" || strings.ContainsAny(name, " ()[]{}") {
			return nil, "", "", false
		}
		return node, name, receiverText(source, node, name), true
	case "expression":
		if node.NamedChildCount() == 1 {
			return referenceName(source, node.NamedChild(0))
		}
		return nil, "", "", false
	default:
		return nil, "", "", false
	}
}

func goCallReference(source []byte, callNode, functionNode *tree_sitter.Node) (callDetails, bool) {
	nameNode, name, receiver, ok := referenceName(source, functionNode)
	if !ok {
		name, receiver, ok = goGenericReference(source, functionNode)
		if !ok {
			return callDetails{}, false
		}
		return callDetails{nameNode: functionNode, name: name, receiverText: receiver, argumentTexts: argumentTexts(source, callNode)}, true
	}
	return callDetails{nameNode: nameNode, name: name, receiverText: receiver, argumentTexts: argumentTexts(source, callNode)}, true
}

func goGenericReference(source []byte, node *tree_sitter.Node) (string, string, bool) {
	text := strings.TrimSpace(nodeText(source, node))
	bracket := strings.Index(text, "[")
	if bracket <= 0 {
		return "", "", false
	}
	callee := strings.TrimSpace(text[:bracket])
	if callee == "" || strings.ContainsAny(callee, " (){}[]+-*/%!&|<>=,:") {
		return "", "", false
	}
	if dot := strings.LastIndex(callee, "."); dot >= 0 {
		receiver := strings.TrimSpace(callee[:dot])
		name := strings.TrimSpace(callee[dot+1:])
		return name, receiver, name != ""
	}
	return callee, "", true
}

type nodeMetadata struct {
	receiverType   string
	parameterCount int
	parameterTypes []string
	returnType     string
}

func nodeSignatureMetadata(source []byte, node *tree_sitter.Node, name string) nodeMetadata {
	signature := declarationHeader(source, node)
	parameters := parameterListTextAfterName(signature, name)
	parameterTypes := parameterTypesFromList(parameters)
	return nodeMetadata{
		receiverType:   extensionReceiverType(source, node, name, signature),
		parameterCount: len(parameterTypes),
		parameterTypes: parameterTypes,
		returnType:     returnTypeFromHeader(signature, name),
	}
}

func rustFunctionMetadata(source []byte, node *tree_sitter.Node, name string) nodeMetadata {
	signature := declarationHeader(source, node)
	parameters := parameterListTextAfterName(signature, name)
	parameterTypes := rustParameterTypesFromList(parameters)
	return nodeMetadata{
		parameterCount: len(parameterTypes),
		parameterTypes: parameterTypes,
		returnType:     rustReturnTypeFromHeader(signature, name),
	}
}

func rustEnumVariantMetadata(source []byte, node *tree_sitter.Node, name string) nodeMetadata {
	parameters := parameterListTextAfterName(strings.TrimSpace(nodeText(source, node)), name)
	parameterTypes := rustParameterTypesFromList(parameters)
	return nodeMetadata{
		parameterCount: len(parameterTypes),
		parameterTypes: parameterTypes,
	}
}

func rustParameterTypesFromList(parameters string) []string {
	parameters = strings.TrimSpace(parameters)
	if parameters == "" {
		return nil
	}
	parts := splitTopLevel(parameters, ',')
	types := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || rustSelfParameter(part) {
			continue
		}
		if _, typ, ok := splitParameterNameType(part); ok {
			types = append(types, typ)
			continue
		}
		types = append(types, part)
	}
	return types
}

func rustSelfParameter(parameter string) bool {
	parameter = strings.TrimSpace(parameter)
	parameter = strings.TrimPrefix(parameter, "&")
	parameter = strings.TrimSpace(parameter)
	parameter = strings.TrimPrefix(parameter, "'_")
	parameter = strings.TrimSpace(parameter)
	parameter = strings.TrimPrefix(parameter, "mut ")
	parameter = strings.TrimSpace(parameter)
	return parameter == "self" || strings.HasPrefix(parameter, "self:")
}

func rustReturnTypeFromHeader(header, name string) string {
	nameIndex := strings.Index(header, name)
	if nameIndex < 0 {
		return ""
	}
	open := strings.Index(header[nameIndex+len(name):], "(")
	if open < 0 {
		return ""
	}
	open += nameIndex + len(name)
	close := matchingParen(header, open)
	if close < 0 {
		return ""
	}
	after := ""
	if close+1 < len(header) {
		after = strings.TrimSpace(header[close+1:])
	}
	if !strings.HasPrefix(after, "->") {
		return ""
	}
	after = strings.TrimSpace(strings.TrimPrefix(after, "->"))
	if i := strings.IndexAny(after, "{;"); i >= 0 {
		after = after[:i]
	}
	return strings.TrimSpace(after)
}

func declarationHeader(source []byte, node *tree_sitter.Node) string {
	text := strings.TrimSpace(nodeText(source, node))
	for _, marker := range []string{"{", "=>"} {
		if i := strings.Index(text, marker); i >= 0 {
			text = text[:i]
		}
	}
	return strings.TrimSpace(text)
}

func parameterListTextAfterName(header, name string) string {
	nameIndex := strings.Index(header, name)
	if nameIndex < 0 {
		return ""
	}
	open := strings.Index(header[nameIndex+len(name):], "(")
	if open < 0 {
		return ""
	}
	open += nameIndex + len(name)
	close := matchingParen(header, open)
	if close < 0 {
		return ""
	}
	return header[open+1 : close]
}

func matchingParen(text string, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parameterTypesFromList(parameters string) []string {
	parameters = strings.TrimSpace(parameters)
	if parameters == "" {
		return nil
	}
	parts := splitTopLevel(parameters, ',')
	types := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		types = append(types, parameterType(part))
	}
	return types
}

func parameterType(parameter string) string {
	parameter = strings.TrimSpace(parameter)
	if parameter == "" {
		return ""
	}
	if i := strings.Index(parameter, "="); i >= 0 {
		parameter = strings.TrimSpace(parameter[:i])
	}
	if name, typ, ok := splitParameterNameType(parameter); ok {
		_ = name
		return typ
	}
	fields := strings.Fields(parameter)
	if len(fields) == 0 {
		return ""
	}
	if len(fields) == 1 {
		return fields[0]
	}
	return strings.TrimSpace(fields[len(fields)-1])
}

func isFunctionTypedParameterCall(result *ExtractionResult, fromNodeID, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, node := range result.Nodes {
		if node.ID != fromNodeID {
			continue
		}
		_, ok := functionTypedParameterNames(node.Signature, node.Name)[name]
		return ok
	}
	return false
}

func functionTypedParameterNames(signature, functionName string) map[string]struct{} {
	names := map[string]struct{}{}
	parameters := parameterListTextAfterName(signature, functionName)
	if parameters == "" {
		return names
	}
	for _, parameter := range splitTopLevel(parameters, ',') {
		name, typ, ok := splitParameterNameType(parameter)
		if !ok || name == "" || !strings.Contains(typ, "->") {
			continue
		}
		names[name] = struct{}{}
	}
	return names
}

func splitParameterNameType(parameter string) (string, string, bool) {
	parameter = strings.TrimSpace(parameter)
	if parameter == "" {
		return "", "", false
	}
	depth := 0
	for i, r := range parameter {
		switch r {
		case '(', '[', '<':
			depth++
		case ')', ']', '>':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				name := strings.TrimSpace(parameter[:i])
				typ := strings.TrimSpace(parameter[i+1:])
				if name == "" || typ == "" {
					return "", "", false
				}
				return name, typ, true
			}
		}
	}
	return "", "", false
}

func returnTypeFromHeader(header, name string) string {
	nameIndex := strings.Index(header, name)
	if nameIndex < 0 {
		return ""
	}
	open := strings.Index(header[nameIndex+len(name):], "(")
	if open < 0 {
		return ""
	}
	open += nameIndex + len(name)
	close := matchingParen(header, open)
	if close < 0 {
		return ""
	}
	after := ""
	if close+1 < len(header) {
		after = strings.TrimSpace(header[close+1:])
	}
	if strings.HasPrefix(after, ":") {
		after = strings.TrimSpace(strings.TrimPrefix(after, ":"))
		if i := strings.IndexAny(after, " ={"); i >= 0 {
			after = after[:i]
		}
		return strings.TrimSpace(after)
	}
	if after != "" {
		return strings.TrimSpace(after)
	}
	before := strings.TrimSpace(header[:nameIndex])
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return ""
	}
	last := fields[len(fields)-1]
	switch last {
	case "fun", "func", "fn", "def", "public", "private", "protected", "static", "override", "async":
		return ""
	default:
		return last
	}
}

func extensionReceiverTypeFromHeader(header, name string) string {
	nameIndex := strings.Index(header, name)
	if nameIndex < 0 {
		return ""
	}
	before := strings.TrimSpace(header[:nameIndex])
	if !strings.HasSuffix(before, ".") {
		return ""
	}
	before = strings.TrimSuffix(before, ".")
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return ""
	}
	receiver := fields[len(fields)-1]
	for _, keyword := range []string{"fun", "func", "fn", "def"} {
		receiver = strings.TrimPrefix(receiver, keyword+" ")
	}
	return strings.TrimSpace(receiver)
}

func extensionReceiverType(source []byte, node *tree_sitter.Node, name, signature string) string {
	if receiver := extensionReceiverTypeFromAST(source, node); receiver != "" {
		return receiver
	}
	return extensionReceiverTypeFromHeader(signature, name)
}

func extensionReceiverTypeFromAST(source []byte, node *tree_sitter.Node) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	var previous *tree_sitter.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if sameNodeRange(child, nameNode) {
			if previous != nil && isTypeNode(previous) {
				return strings.TrimSpace(nodeText(source, previous))
			}
			return ""
		}
		previous = child
	}
	return ""
}

func sameNodeRange(left, right *tree_sitter.Node) bool {
	return left != nil && right != nil && left.StartByte() == right.StartByte() && left.EndByte() == right.EndByte()
}

func isTypeNode(node *tree_sitter.Node) bool {
	switch node.Kind() {
	case "user_type", "nullable_type", "function_type", "parenthesized_type", "type_identifier", "generic_type":
		return true
	default:
		return false
	}
}

func qualifiedMemberName(receiverType, name string) string {
	receiverType = strings.TrimSpace(receiverType)
	name = strings.TrimSpace(name)
	if receiverType == "" {
		return name
	}
	return receiverType + "." + name
}

func packageQualifiedName(packageName, qualifiedName string) string {
	packageName = strings.TrimSpace(packageName)
	qualifiedName = strings.TrimSpace(qualifiedName)
	if packageName == "" || qualifiedName == "" || strings.HasPrefix(qualifiedName, packageName+".") {
		return qualifiedName
	}
	return packageName + "." + qualifiedName
}

func kotlinPackageName(source []byte, node *tree_sitter.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind() == "package_header" {
		text := strings.TrimSpace(nodeText(source, node))
		text = strings.TrimSpace(strings.TrimPrefix(text, "package"))
		return strings.TrimSpace(text)
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if packageName := kotlinPackageName(source, node.NamedChild(i)); packageName != "" {
			return packageName
		}
	}
	return ""
}

func kotlinImportPath(source []byte, node *tree_sitter.Node) string {
	text := strings.TrimSpace(nodeText(source, node))
	text = strings.TrimSpace(strings.TrimPrefix(text, "import"))
	if before, _, ok := strings.Cut(text, " as "); ok {
		text = strings.TrimSpace(before)
	}
	return strings.TrimSpace(text)
}

func importSimpleName(qualifiedName string) string {
	qualifiedName = strings.TrimSpace(qualifiedName)
	if qualifiedName == "" {
		return ""
	}
	if strings.HasSuffix(qualifiedName, ".*") {
		return "*"
	}
	if i := strings.LastIndex(qualifiedName, "."); i >= 0 {
		return qualifiedName[i+1:]
	}
	return qualifiedName
}

func goReceiverType(source []byte, node *tree_sitter.Node) string {
	receiver := node.ChildByFieldName("receiver")
	if receiver == nil {
		return ""
	}
	text := strings.TrimSpace(nodeText(source, receiver))
	text = strings.TrimPrefix(text, "(")
	text = strings.TrimSuffix(text, ")")
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	typ := fields[len(fields)-1]
	return strings.TrimPrefix(typ, "*")
}

func argumentTexts(source []byte, callNode *tree_sitter.Node) []string {
	args := argumentListNode(callNode)
	var values []string
	if args != nil {
		values = make([]string, 0, args.NamedChildCount())
		for i := uint(0); i < args.NamedChildCount(); i++ {
			child := args.NamedChild(i)
			text := strings.TrimSpace(nodeText(source, child))
			if text == "" {
				continue
			}
			values = append(values, text)
		}
	}
	values = append(values, trailingLambdaTexts(source, callNode)...)
	return values
}

func argumentListNode(node *tree_sitter.Node) *tree_sitter.Node {
	for _, field := range []string{"arguments", "argument"} {
		if child := node.ChildByFieldName(field); child != nil {
			return child
		}
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "argument_list", "arguments", "value_arguments":
			return child
		}
	}
	return nil
}

func trailingLambdaTexts(source []byte, callNode *tree_sitter.Node) []string {
	var values []string
	for i := uint(0); i < callNode.NamedChildCount(); i++ {
		child := callNode.NamedChild(i)
		if child.Kind() != "annotated_lambda" && child.Kind() != "lambda_literal" {
			continue
		}
		text := strings.TrimSpace(nodeText(source, child))
		if text == "" {
			continue
		}
		values = append(values, text)
	}
	return values
}

func receiverText(source []byte, node *tree_sitter.Node, name string) string {
	for _, field := range []string{"object", "receiver", "operand", "argument"} {
		child := node.ChildByFieldName(field)
		if child == nil {
			continue
		}
		text := strings.TrimSpace(nodeText(source, child))
		if text != "" && text != name {
			return text
		}
	}
	text := strings.TrimSpace(nodeText(source, node))
	for _, suffix := range []string{"." + name, "::" + name} {
		if strings.HasSuffix(text, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(text, suffix))
		}
	}
	return ""
}

func receiverTextBeforeName(source []byte, node, nameNode *tree_sitter.Node) string {
	start := int(node.StartByte())
	end := int(nameNode.StartByte())
	if start < 0 || end < start || end > len(source) {
		return ""
	}
	receiver := strings.TrimSpace(string(source[start:end]))
	for _, suffix := range []string{"?.", "::", "."} {
		receiver = strings.TrimSpace(strings.TrimSuffix(receiver, suffix))
	}
	return receiver
}

func splitTopLevel(text string, delimiter rune) []string {
	var parts []string
	start := 0
	depth := 0
	for i, r := range text {
		switch r {
		case '(', '[', '<':
			depth++
		case ')', ']', '>':
			if depth > 0 {
				depth--
			}
		default:
			if r == delimiter && depth == 0 {
				parts = append(parts, text[start:i])
				start = i + len(string(r))
			}
		}
	}
	parts = append(parts, text[start:])
	return parts
}

type springRoute struct {
	Method  string
	Pattern string
	Line    int
	Column  int
}

func springClassRoutePrefix(source []byte, node *tree_sitter.Node) (string, bool) {
	prefixes, ok := springClassRoutePrefixes(source, node)
	if !ok || len(prefixes) == 0 {
		return "", ok
	}
	return prefixes[0], true
}

func springClassRoutePrefixes(source []byte, node *tree_sitter.Node) ([]string, bool) {
	return springRoutePrefixesFromAnnotations(source, directAnnotations(node))
}

func springRoutePrefixFromAnnotations(source []byte, annotations []*tree_sitter.Node) (string, bool) {
	prefixes, ok := springRoutePrefixesFromAnnotations(source, annotations)
	if !ok || len(prefixes) == 0 {
		return "", ok
	}
	return prefixes[0], true
}

func springRoutePrefixesFromAnnotations(source []byte, annotations []*tree_sitter.Node) ([]string, bool) {
	for _, annotation := range annotations {
		name := annotationName(source, annotation)
		if name != "RequestMapping" {
			continue
		}
		patterns := annotationPathValues(source, annotation)
		if len(patterns) == 0 {
			return []string{""}, true
		}
		return patterns, true
	}
	return nil, false
}

func combineRoutePatternLists(prefixes, patterns []string) []string {
	var combined []string
	for _, prefix := range prefixes {
		for _, pattern := range patterns {
			combined = append(combined, combineRoutePatterns(prefix, pattern))
		}
	}
	return uniqueStrings(combined)
}

func springRoutesFromAnnotations(source []byte, annotations []*tree_sitter.Node, routePrefix string) []springRoute {
	var routes []springRoute
	for _, annotation := range annotations {
		methods := springHTTPMethods(source, annotation)
		if len(methods) == 0 {
			continue
		}
		patterns := annotationPathValues(source, annotation)
		if len(patterns) == 0 {
			patterns = []string{""}
		}
		start := annotation.StartPosition()
		for _, method := range methods {
			for _, pattern := range patterns {
				routes = append(routes, springRoute{
					Method:  method,
					Pattern: combineRoutePatterns(routePrefix, pattern),
					Line:    int(start.Row) + 1,
					Column:  int(start.Column),
				})
			}
		}
	}
	return routes
}

func springComposedRoutesFromAnnotations(source []byte, annotations []*tree_sitter.Node, routePrefix string) []springRoute {
	meta := springComposedAnnotationRoutes(source)
	var routes []springRoute
	for _, annotation := range annotations {
		route, ok := meta[annotationName(source, annotation)]
		if !ok {
			continue
		}
		start := annotation.StartPosition()
		route.Pattern = combineRoutePatterns(routePrefix, route.Pattern)
		route.Line = int(start.Row) + 1
		route.Column = int(start.Column)
		routes = append(routes, route)
	}
	return routes
}

func springComposedRoutePrefixesFromAnnotations(source []byte, annotations []*tree_sitter.Node) ([]string, bool) {
	meta := springComposedAnnotationPrefixes(source)
	var prefixes []string
	for _, annotation := range annotations {
		annotationPrefixes, ok := meta[annotationName(source, annotation)]
		if !ok {
			continue
		}
		if appliedPrefixes := annotationPathValues(source, annotation); len(appliedPrefixes) > 0 && isEmptyRoutePrefixList(annotationPrefixes) {
			annotationPrefixes = appliedPrefixes
		}
		prefixes = append(prefixes, annotationPrefixes...)
	}
	return uniqueStrings(prefixes), len(prefixes) > 0
}

func isEmptyRoutePrefixList(prefixes []string) bool {
	return len(prefixes) == 1 && prefixes[0] == ""
}

func springComposedAnnotationPrefixes(source []byte) map[string][]string {
	prefixes := map[string][]string{}
	var pending []string
	for _, raw := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(raw)
		if len(pending) > 0 && strings.Contains(trimmed, "@interface ") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "@interface "))
			if idx := strings.IndexAny(name, " {("); idx >= 0 {
				name = strings.TrimSpace(name[:idx])
			}
			if name != "" {
				prefixes[name] = pending
			}
			pending = nil
			continue
		}
		if strings.HasPrefix(trimmed, "@RequestMapping") {
			values := quotedTexts(trimmed)
			if len(values) == 0 {
				values = []string{""}
			}
			pending = values
			continue
		}
		if strings.HasPrefix(trimmed, "@") {
			pending = nil
		}
	}
	return prefixes
}

func springComposedAnnotationRoutes(source []byte) map[string]springRoute {
	routes := map[string]springRoute{}
	var pending *springRoute
	for index, raw := range strings.Split(string(source), "\n") {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(raw)
		if pending != nil && strings.Contains(trimmed, "@interface ") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "@interface "))
			if idx := strings.IndexAny(name, " {("); idx >= 0 {
				name = strings.TrimSpace(name[:idx])
			}
			if name != "" {
				routes[name] = *pending
			}
			pending = nil
			continue
		}
		if strings.HasPrefix(trimmed, "@") {
			method, ok := springHTTPMethodFromName(strings.TrimPrefix(strings.SplitN(strings.TrimPrefix(trimmed, "@"), "(", 2)[0], "@"))
			if ok {
				pending = &springRoute{Method: method, Pattern: firstQuotedText(trimmed), Line: lineNumber, Column: strings.Index(raw, "@")}
			}
			continue
		}
	}
	return routes
}

func springHTTPMethodFromName(name string) (string, bool) {
	switch name {
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
		return "ANY", true
	default:
		return "", false
	}
}

func springHTTPMethod(source []byte, annotation *tree_sitter.Node) (string, bool) {
	methods := springHTTPMethods(source, annotation)
	if len(methods) == 0 {
		return "", false
	}
	return methods[0], true
}

func springHTTPMethods(source []byte, annotation *tree_sitter.Node) []string {
	name := annotationName(source, annotation)
	switch name {
	case "RequestMapping":
		methods := requestMappingMethods(nodeText(source, annotation))
		if len(methods) == 0 {
			return []string{"ANY"}
		}
		return methods
	default:
		method, ok := springHTTPMethodFromName(name)
		if !ok {
			return nil
		}
		return []string{method}
	}
}

func requestMappingMethod(text string) string {
	methods := requestMappingMethods(text)
	if len(methods) == 0 {
		return ""
	}
	return methods[0]
}

func requestMappingMethods(text string) []string {
	upper := strings.ToUpper(text)
	var methods []string
	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"} {
		if strings.Contains(upper, "REQUESTMETHOD."+method) || strings.Contains(upper, "METHOD="+method) {
			methods = append(methods, method)
		}
	}
	return methods
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

func rustImplType(source []byte, node *tree_sitter.Node) string {
	if node == nil {
		return ""
	}
	if typ := node.ChildByFieldName("type"); typ != nil {
		return rustNormalizeTypeName(nodeText(source, typ))
	}
	var lastType string
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "type_identifier", "generic_type", "scoped_type_identifier":
			lastType = rustNormalizeTypeName(nodeText(source, child))
		}
	}
	return lastType
}

func rustNormalizeTypeName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "&")
	value = strings.TrimSpace(value)
	for strings.HasPrefix(value, "mut ") || strings.HasPrefix(value, "'") {
		if strings.HasPrefix(value, "mut ") {
			value = strings.TrimSpace(strings.TrimPrefix(value, "mut "))
			continue
		}
		fields := strings.Fields(value)
		if len(fields) <= 1 || !strings.HasPrefix(fields[0], "'") {
			break
		}
		value = strings.TrimSpace(strings.Join(fields[1:], " "))
	}
	if i := strings.Index(value, "<"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	return strings.TrimSpace(value)
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
