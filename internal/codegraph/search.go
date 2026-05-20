package codegraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"repobridge/internal/codegraph/parser"
	"repobridge/internal/source"
)

type SourceResolver interface {
	EnsureCached(spec string, opts source.Options) (source.Outcome, error)
}

type StoreOpener func(string) (GraphStore, error)

type SearchServiceOptions struct {
	Resolver    SourceResolver
	Indexer     *Indexer
	StoreOpener StoreOpener
	Lifecycle   GraphLifecycle
}

type SearchOptions struct {
	CWD        string
	SyncIndex  bool
	Limit      int
	SourceOpts source.Options
}

type GraphStatus struct {
	SchemaVersion int
	SourcePath    string
	Status        string
	ErrorText     string
	CompletedAt   time.Time
}

type GraphStore interface {
	Close()
	Status() (GraphStatus, error)
	Counts() (GraphCounts, error)
	Files() ([]GraphFile, error)
	Nodes(GraphNodeQuery) ([]GraphNode, error)
	CallsByNode(stableID string) ([]string, error)
	Callgraph(CallgraphQuery) ([]CallgraphEdge, error)
	Replace(IndexResult) error
	Search(SearchQuery) ([]SearchResult, error)
}

type SearchService struct {
	lifecycle GraphLifecycle
}

func NewSearchService(opts SearchServiceOptions) *SearchService {
	lifecycle := opts.Lifecycle
	if lifecycle == nil {
		lifecycle = NewGraphLifecycle(GraphLifecycleOptions{
			Resolver:    opts.Resolver,
			Indexer:     opts.Indexer,
			StoreOpener: opts.StoreOpener,
		})
	}
	return &SearchService{lifecycle: lifecycle}
}

func (s *SearchService) Search(spec, rawQuery string, opts SearchOptions) ([]SearchResult, error) {
	var results []SearchResult
	err := s.lifecycle.UseReady(spec, GraphUseOptions{
		CWD:        opts.CWD,
		SourceOpts: opts.SourceOpts,
		SyncIndex:  opts.SyncIndex,
	}, func(session GraphSession) error {
		query := ParseSearchQuery(rawQuery)
		if opts.Limit > 0 {
			query.Limit = opts.Limit
		}
		found, err := session.Store.Search(query)
		if err != nil {
			return err
		}
		for i := range found {
			if shouldReplaceResultSource(found[i].Source, session.SourcePath) {
				found[i].Source = session.SourceLabel
			}
		}
		results = found
		return nil
	})
	return results, err
}

func currentGraphFiles(sourcePath string, opts parser.Options) ([]GraphFile, error) {
	sourceFiles, err := parser.ScanSourceFiles(sourcePath, opts)
	if err != nil {
		return nil, err
	}
	files := make([]GraphFile, 0, len(sourceFiles))
	for _, sourceFile := range sourceFiles {
		content, err := os.ReadFile(sourceFile.AbsolutePath)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(content)
		files = append(files, GraphFile{
			Path:        sourceFile.RelativePath,
			Language:    sourceFile.Language,
			ContentHash: hex.EncodeToString(hash[:]),
			Size:        sourceFile.Size,
			ModifiedAt:  sourceFile.ModifiedAt,
		})
	}
	return files, nil
}

func shouldReplaceResultSource(resultSource, sourcePath string) bool {
	if resultSource == "" {
		return true
	}
	resultAbs, resultErr := filepath.Abs(resultSource)
	sourceAbs, sourceErr := filepath.Abs(sourcePath)
	return resultErr == nil && sourceErr == nil && resultAbs == sourceAbs
}

func searchSourceLabel(outcome source.Outcome) string {
	if outcome.Name != "" && outcome.Version != "" {
		return outcome.Name + "@" + outcome.Version
	}
	if outcome.Name != "" {
		return outcome.Name
	}
	if outcome.SourceLabel != "" {
		return outcome.SourceLabel
	}
	return outcome.Path
}

type defaultSourceResolver struct{}

func (defaultSourceResolver) EnsureCached(spec string, opts source.Options) (source.Outcome, error) {
	return source.EnsureCached(spec, opts)
}

func defaultStoreOpener(string) (GraphStore, error) {
	return nil, fmt.Errorf("codegraph store opener is required")
}
