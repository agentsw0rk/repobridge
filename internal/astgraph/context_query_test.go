package astgraph

import (
	"reflect"
	"testing"
)

func TestParseContextQueryExtractsFiltersTermsAndSymbols(t *testing.T) {
	got := ParseContextQuery(`"auth login" AuthService.login session_store kind:function lang:kotlin path:src/auth calls:createSession`)

	if got.Search.Kinds[0] != NodeKindFunction || got.Search.Languages[0] != LanguageKotlin {
		t.Fatalf("search filters = %#v, want function/kotlin", got.Search)
	}
	if got.Search.PathFilters[0] != "src/auth" || got.Search.Calls[0] != "createSession" {
		t.Fatalf("path/calls = %#v, want src/auth createSession", got.Search)
	}
	wantTerms := []string{"auth login", "AuthService.login", "session_store"}
	if !reflect.DeepEqual(got.Terms, wantTerms) {
		t.Fatalf("terms = %#v, want %#v", got.Terms, wantTerms)
	}
	wantSymbols := []string{"auth login", "AuthService.login", "session_store"}
	if !reflect.DeepEqual(got.Symbols, wantSymbols) {
		t.Fatalf("symbols = %#v, want %#v", got.Symbols, wantSymbols)
	}
	if got.Search.Text != "auth login AuthService.login session_store" {
		t.Fatalf("search text = %q", got.Search.Text)
	}
}

func TestParseContextQueryFiltersStopwordsAndKeepsConcreteTerms(t *testing.T) {
	got := ParseContextQuery(`how does the login flow call SessionRepository save`)

	wantTerms := []string{"login", "flow", "call", "SessionRepository", "save"}
	if !reflect.DeepEqual(got.Terms, wantTerms) {
		t.Fatalf("terms = %#v, want %#v", got.Terms, wantTerms)
	}
	wantSymbols := []string{"SessionRepository"}
	if !reflect.DeepEqual(got.Symbols, wantSymbols) {
		t.Fatalf("symbols = %#v, want %#v", got.Symbols, wantSymbols)
	}
}

func TestContextBudgetProfilesAndOverrides(t *testing.T) {
	small := ContextBudgetProfile("small")
	if small.SearchLimit != 5 || small.SnippetCount != 3 || small.SourceLines != 8 || small.Depth != 1 {
		t.Fatalf("small = %#v", small)
	}
	large := ContextBudgetProfile("large")
	if large.SearchLimit != 20 || large.SnippetCount != 8 || large.SourceLines != 28 || large.Depth != 2 {
		t.Fatalf("large = %#v", large)
	}

	applied := ApplyContextBudgetOverrides(ContextOptions{Budget: "small", Limit: 12, Depth: 3})
	if applied.SearchLimit != 12 || applied.Depth != 3 {
		t.Fatalf("applied = %#v, want limit/depth overrides", applied)
	}
}
