package astgraph

import "strings"

func ParseSearchQuery(raw string) SearchQuery {
	query := SearchQuery{Limit: 20}
	var text []string

	for _, token := range strings.Fields(raw) {
		prefix, value, ok := strings.Cut(token, ":")
		if !ok {
			text = append(text, token)
			continue
		}

		value = stripQuotes(value)
		switch prefix {
		case "kind":
			kind, valid := parseNodeKind(value)
			if !valid {
				text = append(text, token)
				continue
			}
			query.Kinds = append(query.Kinds, kind)
		case "lang", "language":
			language, valid := parseLanguage(value)
			if !valid {
				text = append(text, token)
				continue
			}
			query.Languages = append(query.Languages, language)
		case "path":
			query.PathFilters = append(query.PathFilters, value)
		case "name":
			query.NameFilters = append(query.NameFilters, value)
		case "calls":
			query.Calls = append(query.Calls, value)
		default:
			text = append(text, token)
		}
	}

	query.Text = strings.Join(text, " ")
	return query
}

func parseNodeKind(value string) (NodeKind, bool) {
	switch kind := NodeKind(value); kind {
	case NodeKindFile, NodeKindModule, NodeKindClass, NodeKindStruct, NodeKindInterface, NodeKindFunction, NodeKindMethod, NodeKindImport, NodeKindRoute, NodeKindHandler, NodeKindComponentRoute:
		return kind, true
	default:
		return "", false
	}
}

func parseLanguage(value string) (Language, bool) {
	switch language := Language(value); language {
	case LanguageGo, LanguageJava, LanguageKotlin, LanguageCSharp, LanguageJavaScript, LanguageTypeScript, LanguagePython, LanguageRust, LanguageUnknown:
		return language, true
	default:
		return "", false
	}
}

func stripQuotes(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1]
	}
	return value
}
