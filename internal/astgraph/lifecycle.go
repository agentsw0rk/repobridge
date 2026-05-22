package astgraph

import (
	"fmt"

	"repobridge/internal/astgraph/parser"
	"repobridge/internal/cache"
	"repobridge/internal/source"
)

type GraphLifecycle interface {
	UseReady(spec string, opts GraphUseOptions, use func(GraphSession) error) error
	UseInspectable(spec string, opts GraphUseOptions, use func(GraphSession) error) error
}

type GraphUseOptions struct {
	CWD        string
	SourceOpts source.Options
	SyncIndex  bool
}

type GraphSession struct {
	Spec        string
	Outcome     source.Outcome
	SourcePath  string
	SourceLabel string
	SourceKind  string
	GraphDir    string
	Status      GraphStatus
	Store       GraphStore
}

type GraphLifecycleOptions struct {
	Resolver     SourceResolver
	Indexer      *Indexer
	StoreOpener  StoreOpener
	CurrentFiles func(sourcePath string) ([]GraphFile, error)
}

type GraphLifecycleService struct {
	resolver     SourceResolver
	indexer      *Indexer
	storeOpener  StoreOpener
	currentFiles func(sourcePath string) ([]GraphFile, error)
}

func NewGraphLifecycle(opts GraphLifecycleOptions) *GraphLifecycleService {
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
	currentFiles := opts.CurrentFiles
	if currentFiles == nil {
		currentFiles = func(sourcePath string) ([]GraphFile, error) {
			return currentGraphFiles(sourcePath, parserOptionsForIndexer(indexer))
		}
	}
	return &GraphLifecycleService{
		resolver:     resolver,
		indexer:      indexer,
		storeOpener:  storeOpener,
		currentFiles: currentFiles,
	}
}

func needsIndex(status GraphStatus) bool {
	return status.Status != "complete" || status.SchemaVersion != SchemaVersion
}

func (s *GraphLifecycleService) UseReady(spec string, opts GraphUseOptions, use func(GraphSession) error) error {
	return s.useGraph(spec, opts, true, use)
}

func (s *GraphLifecycleService) UseInspectable(spec string, opts GraphUseOptions, use func(GraphSession) error) error {
	return s.useGraph(spec, opts, false, use)
}

func (s *GraphLifecycleService) useGraph(spec string, opts GraphUseOptions, requireReady bool, use func(GraphSession) error) error {
	session, err := s.openSession(spec, opts)
	if err != nil {
		return err
	}
	defer session.Store.Close()

	status, err := s.ensureGraph(session.Store, session.SourcePath, spec, opts.SyncIndex, requireReady)
	if err != nil {
		return err
	}
	session.Status = status
	return use(session)
}

func (s *GraphLifecycleService) openSession(spec string, opts GraphUseOptions) (GraphSession, error) {
	sourceOpts := opts.SourceOpts
	if opts.CWD != "" {
		sourceOpts.CWD = opts.CWD
	}
	outcome, err := s.resolver.EnsureCached(spec, sourceOpts)
	if err != nil {
		return GraphSession{}, err
	}
	graphDir := outcome.GraphPath
	if graphDir == "" {
		var err error
		graphDir, err = cache.GraphDirForSource(outcome.Path)
		if err != nil {
			return GraphSession{}, err
		}
	}
	graph, err := s.storeOpener(graphDir)
	if err != nil {
		return GraphSession{}, err
	}
	return GraphSession{
		Spec:        spec,
		Outcome:     outcome,
		SourcePath:  outcome.Path,
		SourceLabel: searchSourceLabel(outcome),
		SourceKind:  outcome.SourceKind,
		GraphDir:    graphDir,
		Store:       graph,
	}, nil
}

func (s *GraphLifecycleService) ensureGraph(graph GraphStore, sourcePath, spec string, syncIndex, requireReady bool) (GraphStatus, error) {
	status, err := graph.Status()
	if err != nil {
		return GraphStatus{}, err
	}
	if needsIndex(status) {
		if !syncIndex {
			if requireReady {
				return GraphStatus{}, fmt.Errorf("astgraph missing or incomplete for %s; enable sync index to build it", spec)
			}
			return status, nil
		}
		return s.reindex(graph, sourcePath)
	}

	stale, err := s.graphIsStale(graph, sourcePath)
	if err != nil {
		return GraphStatus{}, err
	}
	if stale {
		if !syncIndex {
			if requireReady {
				return GraphStatus{}, fmt.Errorf("astgraph stale for %s; enable sync index to rebuild it", spec)
			}
			status.Status = "stale"
			return status, nil
		}
		return s.reindex(graph, sourcePath)
	}
	status.Status = normalizeGraphStatus(status.Status)
	return status, nil
}

func (s *GraphLifecycleService) reindex(graph GraphStore, sourcePath string) (GraphStatus, error) {
	result, err := s.indexer.Index(sourcePath)
	if err != nil {
		return GraphStatus{}, err
	}
	if err := graph.Replace(result); err != nil {
		return GraphStatus{}, err
	}
	return graph.Status()
}

func (s *GraphLifecycleService) graphIsStale(graph GraphStore, sourcePath string) (bool, error) {
	stored, err := graph.Files()
	if err != nil {
		return false, err
	}
	current, err := s.currentFiles(sourcePath)
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

func parserOptionsForIndexer(indexer *Indexer) parser.Options {
	if indexer == nil {
		return parser.Options{}
	}
	return parser.Options{MaxFileSize: indexer.opts.MaxFileSize}
}
