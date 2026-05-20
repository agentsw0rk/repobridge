package codegraph

import "testing"

func TestParseSearchQueryExtractsFiltersAndText(t *testing.T) {
	got := ParseSearchQuery(`kind:function lang:go path:internal/cache name:Ensure calls:Put free text`)

	if got.Text != "free text" {
		t.Fatalf("Text = %q, want free text", got.Text)
	}
	if got.Kinds[0] != NodeKindFunction {
		t.Fatalf("Kinds = %#v, want function", got.Kinds)
	}
	if got.Languages[0] != LanguageGo {
		t.Fatalf("Languages = %#v, want go", got.Languages)
	}
	if got.PathFilters[0] != "internal/cache" {
		t.Fatalf("PathFilters = %#v", got.PathFilters)
	}
	if got.NameFilters[0] != "Ensure" {
		t.Fatalf("NameFilters = %#v", got.NameFilters)
	}
	if got.Calls[0] != "Put" {
		t.Fatalf("Calls = %#v", got.Calls)
	}
}

func TestParseSearchQueryTreatsUnknownPrefixesAsText(t *testing.T) {
	got := ParseSearchQuery(`NOTE: kind:not-a-kind name:Fetch`)
	if got.Text != "NOTE: kind:not-a-kind" {
		t.Fatalf("Text = %q", got.Text)
	}
	if got.NameFilters[0] != "Fetch" {
		t.Fatalf("NameFilters = %#v", got.NameFilters)
	}
}
