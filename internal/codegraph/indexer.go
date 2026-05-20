package codegraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"repobridge/internal/codegraph/parser"
)

const SchemaVersion = 3

type IndexOptions struct {
	MaxFileSize int64
}

type Indexer struct {
	opts IndexOptions
}

func NewIndexer(opts IndexOptions) *Indexer {
	return &Indexer{opts: opts}
}

func (i *Indexer) Index(sourcePath string) (IndexResult, error) {
	startedAt := time.Now().UTC()
	result := IndexResult{
		SourcePath:    sourcePath,
		StartedAt:     startedAt,
		SchemaVersion: SchemaVersion,
	}

	files, err := parser.ScanSourceFiles(sourcePath, parser.Options{MaxFileSize: i.opts.MaxFileSize})
	if err != nil {
		result.CompletedAt = time.Now().UTC()
		return result, err
	}

	for _, file := range files {
		content, err := os.ReadFile(file.AbsolutePath)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: read failed: %v", file.RelativePath, err))
			continue
		}

		extracted, err := parser.ExtractFromSource(file.RelativePath, content, file.Language)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: extract failed: %v", file.RelativePath, err))
			continue
		}

		indexedAt := time.Now().UTC()
		hash := sha256.Sum256(content)
		result.Files = append(result.Files, GraphFile{
			Path:        file.RelativePath,
			Language:    file.Language,
			ContentHash: hex.EncodeToString(hash[:]),
			Size:        file.Size,
			ModifiedAt:  file.ModifiedAt,
			IndexedAt:   indexedAt,
			NodeCount:   len(extracted.Nodes),
		})
		result.Nodes = append(result.Nodes, extracted.Nodes...)
		result.Edges = append(result.Edges, extracted.Edges...)
		result.Unresolved = append(result.Unresolved, extracted.Unresolved...)
		result.Warnings = append(result.Warnings, extracted.Warnings...)
	}

	result.CompletedAt = time.Now().UTC()
	return result, nil
}
