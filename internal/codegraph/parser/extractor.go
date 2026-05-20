package parser

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	"repobridge/internal/codegraph"
)

type ExtractionResult struct {
	Nodes      []codegraph.GraphNode
	Edges      []codegraph.GraphEdge
	Unresolved []codegraph.UnresolvedReference
	Warnings   []string
}

func ExtractFromSource(path string, source []byte, language codegraph.Language) (ExtractionResult, error) {
	tsLanguage, ok := languageFor(language)
	if !ok {
		return ExtractionResult{
			Warnings: []string{fmt.Sprintf("unsupported language: %s", language)},
		}, nil
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(tsLanguage); err != nil {
		return ExtractionResult{}, err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return ExtractionResult{}, fmt.Errorf("parse failed: %s", path)
	}
	defer tree.Close()

	result := ExtractionResult{}
	walkByLanguage(path, source, tree.RootNode(), language, &result)
	return result, nil
}

func stableNodeID(path string, kind codegraph.NodeKind, name string, line int) string {
	hash := sha1.Sum([]byte(fmt.Sprintf("%s:%s:%s:%d", path, kind, name, line)))
	return hex.EncodeToString(hash[:])
}
