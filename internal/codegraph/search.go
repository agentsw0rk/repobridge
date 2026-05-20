package codegraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"repobridge/internal/cache"
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
	Files() ([]GraphFile, error)
	Replace(IndexResult) error
	Search(SearchQuery) ([]SearchResult, error)
}

type SearchService struct {
	resolver    SourceResolver
	indexer     *Indexer
	storeOpener StoreOpener
}

func NewSearchService(opts SearchServiceOptions) *SearchService {
	resolver := opts.Resolver
	if resolver == nil {
		resolver = defaultSourceResolver{}
	}

	indexer := opts.Indexer
	if indexer == nil {
		indexer = NewIndexer(IndexOptions{})
	}

	storeOpener := opts.StoreOpener
	if storeOpener == nil {
		storeOpener = defaultStoreOpener
	}

	return &SearchService{
		resolver:    resolver,
		indexer:     indexer,
		storeOpener: storeOpener,
	}
}

func (s *SearchService) Search(spec, rawQuery string, opts SearchOptions) ([]SearchResult, error) {
	sourceOpts := opts.SourceOpts
	if opts.CWD != "" {
		sourceOpts.CWD = opts.CWD
	}

	outcome, err := s.resolver.EnsureCached(spec, sourceOpts)
	if err != nil {
		return nil, err
	}

	graphDir, err := cache.GraphDirForSource(outcome.Path)
	if err != nil {
		return nil, err
	}

	graph, err := s.storeOpener(graphDir)
	if err != nil {
		return nil, err
	}
	defer graph.Close()

	status, err := graph.Status()
	if err != nil {
		return nil, err
	}
	if needsIndex(status) {
		if !opts.SyncIndex {
			return nil, fmt.Errorf("codegraph missing or incomplete for %s; enable sync index to build it", spec)
		}
		result, err := s.indexer.Index(outcome.Path)
		if err != nil {
			return nil, err
		}
		if err := graph.Replace(result); err != nil {
			return nil, err
		}
	} else {
		stale, err := s.graphIsStale(graph, outcome.Path)
		if err != nil {
			return nil, err
		}
		if stale {
			if !opts.SyncIndex {
				return nil, fmt.Errorf("codegraph stale for %s; enable sync index to rebuild it", spec)
			}
			result, err := s.indexer.Index(outcome.Path)
			if err != nil {
				return nil, err
			}
			if err := graph.Replace(result); err != nil {
				return nil, err
			}
		}
	}

	query := ParseSearchQuery(rawQuery)
	if opts.Limit > 0 {
		query.Limit = opts.Limit
	}
	results, err := graph.Search(query)
	if err != nil {
		return nil, err
	}

	sourceLabel := searchSourceLabel(outcome)
	for i := range results {
		if shouldReplaceResultSource(results[i].Source, outcome.Path) {
			results[i].Source = sourceLabel
		}
	}
	return results, nil
}

func needsIndex(status GraphStatus) bool {
	return status.Status != "complete" || status.SchemaVersion != SchemaVersion
}

func (s *SearchService) graphIsStale(graph GraphStore, sourcePath string) (bool, error) {
	stored, err := graph.Files()
	if err != nil {
		return false, err
	}
	current, err := currentGraphFiles(sourcePath, parser.Options{MaxFileSize: s.indexer.opts.MaxFileSize})
	if err != nil {
		return false, err
	}
	if len(stored) != len(current) {
		return true, nil
	}
	currentByPath := make(map[string]GraphFile, len(current))
	for _, file := range current {
		currentByPath[file.Path] = file
	}
	for _, storedFile := range stored {
		currentFile, ok := currentByPath[storedFile.Path]
		if !ok {
			return true, nil
		}
		if storedFile.ContentHash != currentFile.ContentHash || storedFile.Size != currentFile.Size {
			return true, nil
		}
	}
	return false, nil
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
