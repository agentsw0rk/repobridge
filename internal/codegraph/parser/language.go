package parser

import (
	"repobridge/internal/codegraph/model"

	tree_sitter_kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func languageFor(kind model.Language) (*tree_sitter.Language, bool) {
	switch kind {
	case model.LanguageGo:
		return tree_sitter.NewLanguage(tree_sitter_go.Language()), true
	case model.LanguageJava:
		return tree_sitter.NewLanguage(tree_sitter_java.Language()), true
	case model.LanguageKotlin:
		return tree_sitter.NewLanguage(tree_sitter_kotlin.Language()), true
	case model.LanguageCSharp:
		return tree_sitter.NewLanguage(tree_sitter_c_sharp.Language()), true
	case model.LanguageJavaScript:
		return tree_sitter.NewLanguage(tree_sitter_javascript.Language()), true
	case model.LanguageTypeScript:
		return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript()), true
	case model.LanguagePython:
		return tree_sitter.NewLanguage(tree_sitter_python.Language()), true
	case model.LanguageRust:
		return tree_sitter.NewLanguage(tree_sitter_rust.Language()), true
	default:
		return nil, false
	}
}
