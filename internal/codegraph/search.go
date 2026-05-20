package codegraph

import (
	"fmt"
	"path/filepath"
	"time"

	"repobridge/internal/cache"
	"repobridge/internal/source"
)

type SourceResolver interface {
	EnsureCached(spec string, opts source.Options) (source.Outcome, error)
}

type SearchServiceOptions struct {
	Resolver SourceResolver
	Indexer  *Indexer
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
	Replace(IndexResult) error
	Search(SearchQuery) ([]SearchResult, error)
}

type SearchService struct {
	resolver SourceResolver
	indexer  *Indexer
}

var graphStoreOpener func(string) (GraphStore, error)

func RegisterGraphStoreOpener(opener func(string) (GraphStore, error)) {
	graphStoreOpener = opener
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

	return &SearchService{
		resolver: resolver,
		indexer:  indexer,
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

	if graphStoreOpener == nil {
		return nil, fmt.Errorf("codegraph store is unavailable")
	}
	graph, err := graphStoreOpener(graphDir)
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
