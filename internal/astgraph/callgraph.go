package astgraph

type CallgraphService struct {
	lifecycle GraphLifecycle
}

func NewCallgraphService(opts SearchServiceOptions) *CallgraphService {
	search := NewSearchService(opts)
	return &CallgraphService{lifecycle: search.lifecycle}
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

	result := CallgraphResult{
		Symbol:    symbol,
		Direction: opts.Direction,
	}
	err := s.lifecycle.UseReady(spec, GraphUseOptions{
		CWD:        opts.CWD,
		SourceOpts: opts.SourceOpts,
		SyncIndex:  opts.SyncIndex,
	}, func(session GraphSession) error {
		result.Source = session.SourceLabel
		nodes, err := session.Store.Nodes(GraphNodeQuery{Lookup: symbol, Limit: opts.Limit})
		if err != nil {
			return err
		}
		details := graphNodeDetails(nodes)
		if len(details) != 1 {
			result.Matches = details
			return nil
		}
		root := details[0]
		result.Root = &root

		edges, err := session.Store.Callgraph(CallgraphQuery{
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
			return err
		}
		result.Edges = edges
		return nil
	})
	return result, err
}

func graphNodeDetails(nodes []GraphNode) []GraphNodeDetail {
	details := make([]GraphNodeDetail, 0, len(nodes))
	for _, node := range nodes {
		details = append(details, GraphNodeDetail{
			ID:             node.ID,
			Kind:           node.Kind,
			Name:           node.Name,
			QualifiedName:  node.QualifiedName,
			ReceiverType:   node.ReceiverType,
			ParameterCount: node.ParameterCount,
			ParameterTypes: node.ParameterTypes,
			ReturnType:     node.ReturnType,
			Language:       node.Language,
			Path:           node.FilePath,
			StartLine:      node.StartLine,
			EndLine:        node.EndLine,
			Signature:      node.Signature,
		})
	}
	return details
}
