package codegraph

import (
	"fmt"

	"repobridge/internal/cache"
	"repobridge/internal/codegraph/parser"
)

type CallgraphService struct {
	resolver    SourceResolver
	indexer     *Indexer
	storeOpener StoreOpener
}

func NewCallgraphService(opts SearchServiceOptions) *CallgraphService {
	search := NewSearchService(opts)
	return &CallgraphService{
		resolver:    search.resolver,
		indexer:     search.indexer,
		storeOpener: search.storeOpener,
	}
}

func (s *CallgraphService) Callgraph(spec, symbol string, opts CallgraphOptions) (CallgraphResult, error) {
	if opts.Direction == "" {
		opts.Direction = CallgraphDirectionCallees
	}
	if opts.Depth <= 0 {
		opts.Depth = 1
	}
	if opts.Depth > 5 {
		opts.Depth = 5
	}

	sourceOpts := opts.SourceOpts
	if opts.CWD != "" {
		sourceOpts.CWD = opts.CWD
	}
	outcome, err := s.resolver.EnsureCached(spec, sourceOpts)
	if err != nil {
		return CallgraphResult{}, err
	}
	graphDir, err := cache.GraphDirForSource(outcome.Path)
	if err != nil {
		return CallgraphResult{}, err
	}
	graph, err := s.storeOpener(graphDir)
	if err != nil {
		return CallgraphResult{}, err
	}
	defer graph.Close()

	if err := s.ensureGraph(graph, outcome.Path, spec, opts.SyncIndex); err != nil {
		return CallgraphResult{}, err
	}

	result := CallgraphResult{
		Source:    searchSourceLabel(outcome),
		Symbol:    symbol,
		Direction: opts.Direction,
	}
	nodes, err := graph.Nodes(GraphNodeQuery{Lookup: symbol, Limit: opts.Limit})
	if err != nil {
		return CallgraphResult{}, err
	}
	details := graphNodeDetails(nodes)
	if len(details) != 1 {
		result.Matches = details
		return result, nil
	}
	root := details[0]
	result.Root = &root

	edges, err := graph.Callgraph(CallgraphQuery{
		RootNodeID:        root.ID,
		Direction:         opts.Direction,
		Depth:             opts.Depth,
		Limit:             opts.Limit,
		Kinds:             opts.Kinds,
		Languages:         opts.Languages,
		PathFilters:       opts.PathFilters,
		IncludeUnresolved: opts.IncludeUnresolved,
	})
	if err != nil {
		return CallgraphResult{}, err
	}
	result.Edges = edges
	return result, nil
}

func (s *CallgraphService) ensureGraph(graph GraphStore, sourcePath, spec string, syncIndex bool) error {
	status, err := graph.Status()
	if err != nil {
		return err
	}
	if needsIndex(status) {
		if !syncIndex {
			return fmt.Errorf("codegraph missing or incomplete for %s; enable sync index to build it", spec)
		}
		result, err := s.indexer.Index(sourcePath)
		if err != nil {
			return err
		}
		return graph.Replace(result)
	}
	stale, err := s.graphIsStale(graph, sourcePath)
	if err != nil {
		return err
	}
	if stale {
		if !syncIndex {
			return fmt.Errorf("codegraph stale for %s; enable sync index to rebuild it", spec)
		}
		result, err := s.indexer.Index(sourcePath)
		if err != nil {
			return err
		}
		return graph.Replace(result)
	}
	return nil
}

func (s *CallgraphService) graphIsStale(graph GraphStore, sourcePath string) (bool, error) {
	stored, err := graph.Files()
	if err != nil {
		return false, err
	}
	current, err := currentGraphFiles(sourcePath, parserOptionsForIndexer(s.indexer))
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

func graphNodeDetails(nodes []GraphNode) []GraphNodeDetail {
	details := make([]GraphNodeDetail, 0, len(nodes))
	for _, node := range nodes {
		details = append(details, GraphNodeDetail{
			ID:            node.ID,
			Kind:          node.Kind,
			Name:          node.Name,
			QualifiedName: node.QualifiedName,
			Language:      node.Language,
			Path:          node.FilePath,
			StartLine:     node.StartLine,
			EndLine:       node.EndLine,
			Signature:     node.Signature,
		})
	}
	return details
}
