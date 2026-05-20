package codegraph

import (
	"regexp"
	"strings"
)

var (
	quotedContextTerm = regexp.MustCompile(`"([^"]+)"`)
	contextWord       = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.-]*`)
)

func ParseContextQuery(raw string) ContextParsedQuery {
	parsed := ContextParsedQuery{Raw: raw}
	searchParts := make([]string, 0)
	seenTerms := make(map[string]struct{})
	seenSymbols := make(map[string]struct{})

	remaining := quotedContextTerm.ReplaceAllStringFunc(raw, func(match string) string {
		value := strings.Trim(match, `"`)
		addContextTerm(&parsed.Terms, seenTerms, value)
		addContextSymbol(&parsed.Symbols, seenSymbols, value)
		searchParts = append(searchParts, value)
		return " "
	})

	for _, token := range strings.Fields(remaining) {
		prefix, value, ok := strings.Cut(token, ":")
		if ok && isKnownContextFilter(prefix, value) {
			searchParts = append(searchParts, token)
			continue
		}
		for _, word := range contextWord.FindAllString(token, -1) {
			if isContextStopword(word) {
				continue
			}
			addContextTerm(&parsed.Terms, seenTerms, word)
			if looksLikeContextSymbol(word) {
				addContextSymbol(&parsed.Symbols, seenSymbols, word)
			}
			searchParts = append(searchParts, word)
		}
	}

	parsed.Search = ParseSearchQuery(strings.Join(searchParts, " "))
	return parsed
}

func isKnownContextFilter(prefix, value string) bool {
	switch prefix {
	case "kind":
		_, ok := parseNodeKind(value)
		return ok
	case "lang", "language":
		_, ok := parseLanguage(value)
		return ok
	case "path", "name", "calls":
		return strings.TrimSpace(value) != ""
	default:
		return false
	}
}

func addContextTerm(values *[]string, seen map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	key := strings.ToLower(value)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*values = append(*values, value)
}

func addContextSymbol(values *[]string, seen map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	key := strings.ToLower(value)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*values = append(*values, value)
}

func looksLikeContextSymbol(value string) bool {
	return strings.Contains(value, ".") ||
		strings.Contains(value, "_") ||
		hasUppercase(value)
}

func hasUppercase(value string) bool {
	for _, r := range value {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func isContextStopword(value string) bool {
	switch strings.ToLower(value) {
	case "a", "an", "and", "are", "as", "by", "der", "die", "das", "does", "for", "from", "how", "in", "is", "it", "of", "on", "or", "the", "to", "und", "was", "what", "where", "with":
		return true
	default:
		return false
	}
}

func ContextBudgetProfile(name string) ContextBudget {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "large":
		return ContextBudget{Name: "large", SearchLimit: 20, SnippetCount: 8, SourceLines: 28, Depth: 2}
	case "small", "":
		return ContextBudget{Name: "small", SearchLimit: 5, SnippetCount: 3, SourceLines: 8, Depth: 1}
	default:
		return ContextBudget{Name: "medium", SearchLimit: 10, SnippetCount: 5, SourceLines: 16, Depth: 1}
	}
}

func ApplyContextBudgetOverrides(opts ContextOptions) ContextBudget {
	budget := ContextBudgetProfile(opts.Budget)
	if opts.Limit > 0 {
		budget.SearchLimit = opts.Limit
	}
	if opts.Depth > 0 {
		budget.Depth = opts.Depth
	}
	return budget
}
