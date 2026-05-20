package parser

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func languageFor(kind Language) (*tree_sitter.Language, bool) {
	switch kind {
	case LanguageGo:
		return tree_sitter.NewLanguage(tree_sitter_go.Language()), true
	case LanguageJava:
		return tree_sitter.NewLanguage(tree_sitter_java.Language()), true
	case LanguageCSharp:
		return tree_sitter.NewLanguage(tree_sitter_c_sharp.Language()), true
	case LanguageJavaScript:
		return tree_sitter.NewLanguage(tree_sitter_javascript.Language()), true
	case LanguageTypeScript:
		return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript()), true
	case LanguagePython:
		return tree_sitter.NewLanguage(tree_sitter_python.Language()), true
	case LanguageRust:
		return tree_sitter.NewLanguage(tree_sitter_rust.Language()), true
	default:
		return nil, false
	}
}
