package parser

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"repobridge/internal/codegraph"
)

func languageFor(kind codegraph.Language) (*tree_sitter.Language, bool) {
	switch kind {
	case codegraph.LanguageGo:
		return tree_sitter.NewLanguage(tree_sitter_go.Language()), true
	default:
		return nil, false
	}
}
