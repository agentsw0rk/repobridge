package parser

import (
	"regexp"
	"strings"

	"repobridge/internal/astgraph/model"
)

type expandedClassHeader struct {
	name       string
	extends    []string
	implements []string
	line       int
}

func appendExpandedGraphKinds(path string, source []byte, language model.Language, result *ExtractionResult) {
	switch language {
	case model.LanguageGo:
		appendGoExpandedKinds(path, source, result)
	case model.LanguageJava:
		appendJavaExpandedKinds(path, source, result)
	case model.LanguageKotlin:
		appendKotlinExpandedKinds(path, source, result)
	case model.LanguageCSharp:
		appendCSharpExpandedKinds(path, source, result)
	case model.LanguageTypeScript:
		appendTypeScriptExpandedKinds(path, source, result)
	case model.LanguageRust:
		appendRustExpandedKinds(path, source, result)
	}
	appendFieldTypeEdges(path, result)
	appendReturnTypeEdges(path, result)
	appendLocalInstantiationEdges(path, source, language, result)
}

func appendGoExpandedKinds(path string, source []byte, result *ExtractionResult) {
	lines := strings.Split(string(source), "\n")
	typeAliasRE := regexp.MustCompile(`^type\s+([A-Za-z_]\w*)\s+(.+)$`)
	structStartRE := regexp.MustCompile(`^type\s+([A-Za-z_]\w*)\s+struct\s*\{`)
	constRE := regexp.MustCompile(`^const\s+([A-Za-z_]\w*)\b`)
	fieldRE := regexp.MustCompile(`^([A-Za-z_]\w*)\s+([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)?)\b`)
	var currentStruct string
	for index, raw := range lines {
		line := index + 1
		trimmed := strings.TrimSpace(stripLineComment(raw))
		if trimmed == "" {
			continue
		}
		if currentStruct != "" {
			if strings.HasPrefix(trimmed, "}") {
				currentStruct = ""
				continue
			}
			if match := fieldRE.FindStringSubmatch(trimmed); match != nil {
				fieldID := appendExpandedNode(path, result, model.NodeKindField, match[1], currentStruct+"."+match[1], currentStruct, match[2], model.LanguageGo, line, strings.TrimSpace(raw))
				appendTypeOfEdge(path, result, fieldID, match[2], line)
			}
			continue
		}
		if match := structStartRE.FindStringSubmatch(trimmed); match != nil {
			currentStruct = match[1]
			appendExpandedNode(path, result, model.NodeKindStruct, currentStruct, currentStruct, "", "", model.LanguageGo, line, strings.TrimSpace(raw))
			if strings.Contains(trimmed, "struct{}") {
				currentStruct = ""
			}
			continue
		}
		if match := constRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindConstant, match[1], match[1], "", "", model.LanguageGo, line, strings.TrimSpace(raw))
			continue
		}
		if match := typeAliasRE.FindStringSubmatch(trimmed); match != nil && !strings.Contains(match[2], "struct") && !strings.Contains(match[2], "interface") {
			appendExpandedNode(path, result, model.NodeKindTypeAlias, match[1], match[1], "", strings.TrimSpace(match[2]), model.LanguageGo, line, strings.TrimSpace(raw))
		}
	}
}

func appendJavaExpandedKinds(path string, source []byte, result *ExtractionResult) {
	lines := strings.Split(string(source), "\n")
	headers := make([]expandedClassHeader, 0)
	classRE := regexp.MustCompile(`\bclass\s+([A-Za-z_]\w*)(?:\s+extends\s+([A-Za-z_]\w*))?(?:\s+implements\s+([A-Za-z_][\w\s,]*))?`)
	interfaceRE := regexp.MustCompile(`\binterface\s+([A-Za-z_]\w*)`)
	enumRE := regexp.MustCompile(`\benum\s+([A-Za-z_]\w*)\s*\{([^}]*)\}`)
	fieldRE := regexp.MustCompile(`^(?:public|private|protected|static|final|\s)*\s*([A-Z][A-Za-z_]\w*)\s+([a-zA-Z_]\w*)\s*;`)
	currentClass := ""
	for index, raw := range lines {
		line := index + 1
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if match := interfaceRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindInterface, match[1], match[1], "", "", model.LanguageJava, line, trimmed)
		}
		if match := classRE.FindStringSubmatch(trimmed); match != nil {
			currentClass = match[1]
			appendExpandedNode(path, result, model.NodeKindClass, currentClass, currentClass, "", "", model.LanguageJava, line, trimmed)
			headers = append(headers, expandedClassHeader{name: currentClass, extends: splitTypeList(match[2]), implements: splitTypeList(match[3]), line: line})
		}
		if match := enumRE.FindStringSubmatch(trimmed); match != nil {
			appendEnumNodes(path, result, match[1], match[2], model.LanguageJava, line, trimmed)
		}
		if currentClass != "" {
			if strings.HasPrefix(trimmed, "}") {
				currentClass = ""
				continue
			}
			if match := fieldRE.FindStringSubmatch(trimmed); match != nil && !strings.Contains(trimmed, "(") {
				fieldID := appendExpandedNode(path, result, model.NodeKindField, match[2], currentClass+"."+match[2], currentClass, match[1], model.LanguageJava, line, trimmed)
				appendTypeOfEdge(path, result, fieldID, match[1], line)
			}
		}
	}
	appendClassHeaderEdges(path, result, headers)
}

func appendKotlinExpandedKinds(path string, source []byte, result *ExtractionResult) {
	lines := strings.Split(string(source), "\n")
	headers := make([]expandedClassHeader, 0)
	interfaceRE := regexp.MustCompile(`^interface\s+([A-Za-z_]\w*)`)
	classRE := regexp.MustCompile(`^(?:open\s+)?(?:enum\s+)?class\s+([A-Za-z_]\w*)(?:\s*:\s*([^{]+))?`)
	enumRE := regexp.MustCompile(`^enum\s+class\s+([A-Za-z_]\w*)\s*\{([^}]*)\}`)
	propertyRE := regexp.MustCompile(`^(?:const\s+)?(?:val|var)\s+([A-Za-z_]\w*)\s*:\s*([A-Za-z_]\w*)`)
	constRE := regexp.MustCompile(`^const\s+val\s+([A-Za-z_]\w*)\b`)
	typeAliasRE := regexp.MustCompile(`^typealias\s+([A-Za-z_]\w*)\s*=`)
	currentClass := ""
	for index, raw := range lines {
		line := index + 1
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if match := interfaceRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindInterface, match[1], match[1], "", "", model.LanguageKotlin, line, trimmed)
		}
		if match := enumRE.FindStringSubmatch(trimmed); match != nil {
			appendEnumNodes(path, result, match[1], match[2], model.LanguageKotlin, line, trimmed)
		}
		if match := classRE.FindStringSubmatch(trimmed); match != nil {
			currentClass = match[1]
			if !strings.HasPrefix(trimmed, "enum ") {
				appendExpandedNode(path, result, model.NodeKindClass, currentClass, currentClass, "", "", model.LanguageKotlin, line, trimmed)
			}
			extends, implements := splitKotlinBaseTypes(match[2])
			headers = append(headers, expandedClassHeader{name: currentClass, extends: extends, implements: implements, line: line})
		}
		if match := typeAliasRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindTypeAlias, match[1], match[1], "", "", model.LanguageKotlin, line, trimmed)
		}
		if match := propertyRE.FindStringSubmatch(trimmed); match != nil {
			kind := model.NodeKindProperty
			if strings.HasPrefix(trimmed, "const ") {
				kind = model.NodeKindConstant
			}
			qualified := match[1]
			receiver := ""
			if currentClass != "" && kind == model.NodeKindProperty {
				qualified = currentClass + "." + match[1]
				receiver = currentClass
			}
			nodeID := appendExpandedNode(path, result, kind, match[1], qualified, receiver, match[2], model.LanguageKotlin, line, trimmed)
			appendTypeOfEdge(path, result, nodeID, match[2], line)
		} else if match := constRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindConstant, match[1], match[1], "", "", model.LanguageKotlin, line, trimmed)
		}
		if currentClass != "" && strings.HasPrefix(trimmed, "}") {
			currentClass = ""
		}
	}
	appendClassHeaderEdges(path, result, headers)
}

func appendCSharpExpandedKinds(path string, source []byte, result *ExtractionResult) {
	lines := strings.Split(string(source), "\n")
	headers := make([]expandedClassHeader, 0)
	interfaceRE := regexp.MustCompile(`\binterface\s+([A-Za-z_]\w*)`)
	classRE := regexp.MustCompile(`\bclass\s+([A-Za-z_]\w*)(?:\s*:\s*([A-Za-z_][\w\s,]*))?`)
	enumRE := regexp.MustCompile(`\benum\s+([A-Za-z_]\w*)\s*\{([^}]*)\}`)
	propertyRE := regexp.MustCompile(`^(?:public|private|protected|internal|\s)*\s*([A-Za-z_]\w*)\s+([A-Z][A-Za-z_]\w*)\s*\{`)
	constRE := regexp.MustCompile(`\bconst\s+[A-Za-z_]\w*\s+([A-Za-z_]\w*)\b`)
	fieldRE := regexp.MustCompile(`^(?:public|private|protected|internal|static|readonly|\s)*\s*([A-Z][A-Za-z_]\w*)\s+([a-z][A-Za-z_]\w*)\s*;`)
	currentClass := ""
	for index, raw := range lines {
		line := index + 1
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if match := interfaceRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindInterface, match[1], match[1], "", "", model.LanguageCSharp, line, trimmed)
		}
		if match := classRE.FindStringSubmatch(trimmed); match != nil {
			currentClass = match[1]
			appendExpandedNode(path, result, model.NodeKindClass, currentClass, currentClass, "", "", model.LanguageCSharp, line, trimmed)
			extends, implements := splitCSharpBaseTypes(match[2], result)
			headers = append(headers, expandedClassHeader{name: currentClass, extends: extends, implements: implements, line: line})
		}
		if match := enumRE.FindStringSubmatch(trimmed); match != nil {
			appendEnumNodes(path, result, match[1], match[2], model.LanguageCSharp, line, trimmed)
		}
		if currentClass != "" {
			if match := propertyRE.FindStringSubmatch(trimmed); match != nil && !strings.Contains(trimmed, "(") {
				nodeID := appendExpandedNode(path, result, model.NodeKindProperty, match[2], currentClass+"."+match[2], currentClass, match[1], model.LanguageCSharp, line, trimmed)
				appendTypeOfEdge(path, result, nodeID, match[1], line)
			}
			if match := constRE.FindStringSubmatch(trimmed); match != nil {
				appendExpandedNode(path, result, model.NodeKindConstant, match[1], currentClass+"."+match[1], currentClass, "", model.LanguageCSharp, line, trimmed)
			}
			if match := fieldRE.FindStringSubmatch(trimmed); match != nil && !strings.Contains(trimmed, "(") {
				fieldID := appendExpandedNode(path, result, model.NodeKindField, match[2], currentClass+"."+match[2], currentClass, match[1], model.LanguageCSharp, line, trimmed)
				appendTypeOfEdge(path, result, fieldID, match[1], line)
			}
		}
		if currentClass != "" && strings.HasPrefix(trimmed, "}") {
			currentClass = ""
		}
	}
	appendClassHeaderEdges(path, result, headers)
}

func appendTypeScriptExpandedKinds(path string, source []byte, result *ExtractionResult) {
	lines := strings.Split(string(source), "\n")
	headers := make([]expandedClassHeader, 0)
	interfaceRE := regexp.MustCompile(`^interface\s+([A-Za-z_]\w*)`)
	classRE := regexp.MustCompile(`^class\s+([A-Za-z_]\w*)(?:\s+extends\s+([A-Za-z_]\w*))?(?:\s+implements\s+([A-Za-z_][\w\s,]*))?`)
	propertyRE := regexp.MustCompile(`^([A-Za-z_]\w*)\s*:\s*([A-Za-z_]\w*)\b`)
	typeAliasRE := regexp.MustCompile(`^type\s+([A-Za-z_]\w*)\s*=`)
	enumRE := regexp.MustCompile(`^enum\s+([A-Za-z_]\w*)\s*\{([^}]*)\}`)
	currentClass := ""
	for index, raw := range lines {
		line := index + 1
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if match := interfaceRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindInterface, match[1], match[1], "", "", model.LanguageTypeScript, line, trimmed)
		}
		if match := classRE.FindStringSubmatch(trimmed); match != nil {
			currentClass = match[1]
			appendExpandedNode(path, result, model.NodeKindClass, currentClass, currentClass, "", "", model.LanguageTypeScript, line, trimmed)
			headers = append(headers, expandedClassHeader{name: currentClass, extends: splitTypeList(match[2]), implements: splitTypeList(match[3]), line: line})
		}
		if match := typeAliasRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindTypeAlias, match[1], match[1], "", "", model.LanguageTypeScript, line, trimmed)
		}
		if match := enumRE.FindStringSubmatch(trimmed); match != nil {
			appendEnumNodes(path, result, match[1], match[2], model.LanguageTypeScript, line, trimmed)
		}
		if currentClass != "" {
			if match := propertyRE.FindStringSubmatch(trimmed); match != nil {
				nodeID := appendExpandedNode(path, result, model.NodeKindProperty, match[1], currentClass+"."+match[1], currentClass, match[2], model.LanguageTypeScript, line, trimmed)
				appendTypeOfEdge(path, result, nodeID, match[2], line)
			}
			if strings.HasPrefix(trimmed, "}") {
				currentClass = ""
			}
		}
	}
	appendClassHeaderEdges(path, result, headers)
}

func appendRustExpandedKinds(path string, source []byte, result *ExtractionResult) {
	lines := strings.Split(string(source), "\n")
	traitRE := regexp.MustCompile(`^trait\s+([A-Za-z_]\w*)`)
	structRE := regexp.MustCompile(`^struct\s+([A-Za-z_]\w*)`)
	fieldRE := regexp.MustCompile(`^([A-Za-z_]\w*)\s*:\s*([A-Za-z_]\w*)`)
	typeAliasRE := regexp.MustCompile(`^type\s+([A-Za-z_]\w*)\s*=`)
	constRE := regexp.MustCompile(`^const\s+([A-Za-z_]\w*)\s*:`)
	enumRE := regexp.MustCompile(`^enum\s+([A-Za-z_]\w*)\s*\{([^}]*)\}`)
	implRE := regexp.MustCompile(`^impl\s+([A-Za-z_]\w*)\s+for\s+([A-Za-z_]\w*)`)
	currentStruct := ""
	for index, raw := range lines {
		line := index + 1
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if currentStruct != "" {
			if strings.HasPrefix(trimmed, "}") {
				currentStruct = ""
				continue
			}
			if match := fieldRE.FindStringSubmatch(strings.TrimSuffix(trimmed, ",")); match != nil {
				fieldID := appendExpandedNode(path, result, model.NodeKindField, match[1], currentStruct+"."+match[1], currentStruct, match[2], model.LanguageRust, line, trimmed)
				appendTypeOfEdge(path, result, fieldID, match[2], line)
			}
			continue
		}
		if match := traitRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindTrait, match[1], match[1], "", "", model.LanguageRust, line, trimmed)
		}
		if match := structRE.FindStringSubmatch(trimmed); match != nil {
			currentStruct = match[1]
			appendExpandedNode(path, result, model.NodeKindStruct, match[1], match[1], "", "", model.LanguageRust, line, trimmed)
			if strings.Contains(trimmed, "{}") {
				currentStruct = ""
			}
		}
		if match := typeAliasRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindTypeAlias, match[1], match[1], "", "", model.LanguageRust, line, trimmed)
		}
		if match := constRE.FindStringSubmatch(trimmed); match != nil {
			appendExpandedNode(path, result, model.NodeKindConstant, match[1], match[1], "", "", model.LanguageRust, line, trimmed)
		}
		if match := enumRE.FindStringSubmatch(trimmed); match != nil {
			appendEnumNodes(path, result, match[1], match[2], model.LanguageRust, line, trimmed)
		}
		if match := implRE.FindStringSubmatch(trimmed); match != nil {
			sourceID := findLocalTypeID(result.Nodes, match[2])
			targetID := findLocalTypeID(result.Nodes, match[1])
			appendExpandedEdge(path, result, sourceID, targetID, model.EdgeKindImplements, line, "rust-impl")
		}
	}
}

func appendExpandedNode(path string, result *ExtractionResult, kind model.NodeKind, name, qualified, receiver, returnType string, language model.Language, line int, signature string) string {
	if name == "" {
		return ""
	}
	if qualified == "" {
		qualified = name
	}
	for _, node := range result.Nodes {
		if node.Kind == kind && node.Name == name && node.FilePath == path && node.StartLine == line {
			return node.ID
		}
	}
	id := stableNodeID(path, kind, qualified, line)
	result.Nodes = append(result.Nodes, model.GraphNode{
		ID:            id,
		Kind:          kind,
		Name:          name,
		QualifiedName: qualified,
		ReceiverType:  receiver,
		ReturnType:    strings.TrimSpace(returnType),
		FilePath:      path,
		Language:      language,
		StartLine:     line,
		EndLine:       line,
		Signature:     signature,
	})
	return id
}

func appendEnumNodes(path string, result *ExtractionResult, enumName, body string, language model.Language, line int, signature string) {
	enumID := appendExpandedNode(path, result, model.NodeKindEnum, enumName, enumName, "", "", language, line, signature)
	_ = enumID
	for _, member := range splitTypeList(body) {
		member = strings.TrimSpace(strings.Split(member, "=")[0])
		if member == "" {
			continue
		}
		appendExpandedNode(path, result, model.NodeKindEnumMember, member, enumName+"."+member, enumName, "", language, line, member)
	}
}

func appendClassHeaderEdges(path string, result *ExtractionResult, headers []expandedClassHeader) {
	for _, header := range headers {
		sourceID := findLocalTypeID(result.Nodes, header.name)
		for _, target := range header.extends {
			appendExpandedEdge(path, result, sourceID, findLocalTypeID(result.Nodes, target), model.EdgeKindExtends, header.line, "class-header")
		}
		for _, target := range header.implements {
			appendExpandedEdge(path, result, sourceID, findLocalTypeID(result.Nodes, target), model.EdgeKindImplements, header.line, "class-header")
		}
	}
}

func appendReturnTypeEdges(path string, result *ExtractionResult) {
	for _, node := range result.Nodes {
		if node.ReturnType == "" || (node.Kind != model.NodeKindFunction && node.Kind != model.NodeKindMethod && node.Kind != model.NodeKindHandler) {
			continue
		}
		appendExpandedEdge(path, result, node.ID, findLocalTypeID(result.Nodes, node.ReturnType), model.EdgeKindReturns, node.StartLine, "return-type")
	}
}

func appendFieldTypeEdges(path string, result *ExtractionResult) {
	for _, node := range result.Nodes {
		if node.ReturnType == "" || (node.Kind != model.NodeKindField && node.Kind != model.NodeKindProperty) {
			continue
		}
		appendTypeOfEdge(path, result, node.ID, node.ReturnType, node.StartLine)
	}
}

func appendTypeOfEdge(path string, result *ExtractionResult, sourceID, typeName string, line int) {
	appendExpandedEdge(path, result, sourceID, findLocalTypeID(result.Nodes, typeName), model.EdgeKindTypeOf, line, "type-annotation")
}

func appendLocalInstantiationEdges(path string, source []byte, language model.Language, result *ExtractionResult) {
	if language != model.LanguageGo && language != model.LanguageRust {
		return
	}
	lines := strings.Split(string(source), "\n")
	typeNames := localTypeNames(result.Nodes)
	for index, raw := range lines {
		line := index + 1
		ownerID := ownerCallableAtLine(result.Nodes, line)
		if ownerID == "" {
			continue
		}
		for name, targetID := range typeNames {
			if strings.Contains(raw, name+"{}") || strings.Contains(raw, name+" {") || strings.Contains(raw, name+"(") {
				appendExpandedEdge(path, result, ownerID, targetID, model.EdgeKindInstantiates, line, "constructor-syntax")
			}
		}
	}
}

func appendExpandedEdge(path string, result *ExtractionResult, sourceID, targetID string, kind model.EdgeKind, line int, provenance string) {
	if sourceID == "" || targetID == "" || sourceID == targetID {
		return
	}
	for _, edge := range result.Edges {
		if edge.SourceNodeID == sourceID && edge.TargetNodeID == targetID && edge.Kind == kind && edge.Line == line {
			return
		}
	}
	result.Edges = append(result.Edges, model.GraphEdge{
		SourceNodeID: sourceID,
		TargetNodeID: targetID,
		Kind:         kind,
		FilePath:     path,
		Line:         line,
		Provenance:   provenance,
	})
}

func findLocalTypeID(nodes []model.GraphNode, typeName string) string {
	typeName = cleanTypeName(typeName)
	if typeName == "" {
		return ""
	}
	for _, node := range nodes {
		if !isLocalTypeKind(node.Kind) || node.Kind == model.NodeKindClass {
			continue
		}
		if node.Name == typeName || node.QualifiedName == typeName || strings.HasSuffix(node.QualifiedName, "."+typeName) || strings.HasSuffix(node.QualifiedName, "::"+typeName) {
			return node.ID
		}
	}
	for _, node := range nodes {
		if !isLocalTypeKind(node.Kind) {
			continue
		}
		if node.Name == typeName || node.QualifiedName == typeName || strings.HasSuffix(node.QualifiedName, "."+typeName) || strings.HasSuffix(node.QualifiedName, "::"+typeName) {
			return node.ID
		}
	}
	return ""
}

func localTypeNames(nodes []model.GraphNode) map[string]string {
	names := make(map[string]string)
	for _, node := range nodes {
		if isLocalTypeKind(node.Kind) {
			names[node.Name] = node.ID
		}
	}
	return names
}

func isLocalTypeKind(kind model.NodeKind) bool {
	switch kind {
	case model.NodeKindClass, model.NodeKindStruct, model.NodeKindInterface, model.NodeKindEnum, model.NodeKindTrait, model.NodeKindProtocol, model.NodeKindTypeAlias:
		return true
	default:
		return false
	}
}

func ownerCallableAtLine(nodes []model.GraphNode, line int) string {
	for _, node := range nodes {
		if (node.Kind == model.NodeKindFunction || node.Kind == model.NodeKindMethod || node.Kind == model.NodeKindHandler) && node.StartLine <= line && node.EndLine >= line {
			return node.ID
		}
	}
	return ""
}

func cleanTypeName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "*")
	value = strings.TrimPrefix(value, "&")
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "?")
	value = strings.TrimSuffix(value, "!")
	if idx := strings.IndexAny(value, "<(["); idx >= 0 {
		value = value[:idx]
	}
	if strings.Contains(value, ".") {
		parts := strings.Split(value, ".")
		value = parts[len(parts)-1]
	}
	if strings.Contains(value, "::") {
		parts := strings.Split(value, "::")
		value = parts[len(parts)-1]
	}
	return strings.TrimSpace(value)
}

func splitTypeList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := cleanTypeName(strings.TrimSpace(part))
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func splitKotlinBaseTypes(value string) (extends, implements []string) {
	types := splitTypeList(strings.ReplaceAll(value, "()", ""))
	if len(types) == 0 {
		return nil, nil
	}
	extends = append(extends, types[0])
	if len(types) > 1 {
		implements = append(implements, types[1:]...)
	}
	return extends, implements
}

func splitCSharpBaseTypes(value string, result *ExtractionResult) (extends, implements []string) {
	for _, typ := range splitTypeList(value) {
		targetID := findLocalTypeID(result.Nodes, typ)
		target := nodeKindByID(result.Nodes, targetID)
		if target == model.NodeKindInterface {
			implements = append(implements, typ)
			continue
		}
		if len(extends) == 0 {
			extends = append(extends, typ)
		} else {
			implements = append(implements, typ)
		}
	}
	return extends, implements
}

func nodeKindByID(nodes []model.GraphNode, id string) model.NodeKind {
	for _, node := range nodes {
		if node.ID == id {
			return node.Kind
		}
	}
	return ""
}
