package astgraph

import (
	"sort"
	"strconv"
	"strings"
)

type ContextService struct {
	lifecycle GraphLifecycle
}

func NewContextService(opts SearchServiceOptions) *ContextService {
	search := NewSearchService(opts)
	return &ContextService{lifecycle: search.lifecycle}
}

func (s *ContextService) Context(spec, rawQuery string, opts ContextOptions) (ContextResult, error) {
	if opts.Mode == "" {
		opts.Mode = ContextModeContext
	}
	budget := ApplyContextBudgetOverrides(opts)
	var result ContextResult
	err := s.lifecycle.UseReady(spec, GraphUseOptions{
		CWD:        opts.CWD,
		SourceOpts: opts.SourceOpts,
		SyncIndex:  opts.SyncIndex,
	}, func(session GraphSession) error {
		parsed := ParseContextQuery(rawQuery)
		parsed.Search.Limit = budget.SearchLimit
		results, err := session.Store.Search(parsed.Search)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			results, err = s.searchContextTerms(session.Store, parsed.Terms, budget)
			if err != nil {
				return err
			}
		}
		entryPoints, err := s.entryPoints(session.Store, parsed, results, budget)
		if err != nil {
			return err
		}
		relationships, err := s.relationships(session.Store, entryPoints, budget)
		if err != nil {
			return err
		}
		snippets, err := s.snippets(session.SourcePath, entryPoints, budget)
		if err != nil {
			return err
		}
		files, err := session.Store.Files()
		if err != nil {
			return err
		}
		relatedFiles := relatedGraphFiles(files, entryPoints, budget.SearchLimit)

		result = ContextResult{
			Source:        session.SourceLabel,
			Mode:          opts.Mode,
			Query:         rawQuery,
			Budget:        budget,
			EntryPoints:   entryPoints,
			Relationships: relationships,
			Snippets:      snippets,
			RelatedFiles:  relatedFiles,
			Warnings:      splitWarnings(session.Status.ErrorText),
		}
		result.Stats = ContextResultStats{
			Terms:         len(parsed.Terms),
			EntryPoints:   len(result.EntryPoints),
			Relationships: len(result.Relationships),
			Snippets:      len(result.Snippets),
			RelatedFiles:  len(result.RelatedFiles),
		}
		return nil
	})
	return result, err
}

func (s *ContextService) searchContextTerms(graph GraphStore, terms []string, budget ContextBudget) ([]SearchResult, error) {
	results := make([]SearchResult, 0)
	seen := make(map[string]struct{})
	for _, term := range terms {
		query := ParseSearchQuery(term)
		query.Limit = budget.SearchLimit
		termResults, err := graph.Search(query)
		if err != nil {
			return nil, err
		}
		for _, result := range termResults {
			key := result.ID + "|" + result.Path + "|" + result.Name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			results = append(results, result)
			if len(results) >= budget.SearchLimit {
				return results, nil
			}
		}
	}
	return results, nil
}

func (s *ContextService) entryPoints(graph GraphStore, parsed ContextParsedQuery, searchResults []SearchResult, budget ContextBudget) ([]GraphNodeDetail, error) {
	seen := make(map[string]struct{})
	entryPoints := make([]GraphNodeDetail, 0, len(searchResults))
	for _, result := range searchResults {
		detail := GraphNodeDetail{
			ID:             result.ID,
			Kind:           result.Kind,
			Name:           result.Name,
			QualifiedName:  result.QualifiedName,
			ReceiverType:   result.ReceiverType,
			ParameterCount: result.ParameterCount,
			ParameterTypes: result.ParameterTypes,
			ReturnType:     result.ReturnType,
			Language:       result.Language,
			Path:           result.Path,
			StartLine:      result.StartLine,
			EndLine:        result.EndLine,
			Calls:          result.Calls,
		}
		key := contextEntryKey(detail)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		entryPoints = append(entryPoints, detail)
	}
	for _, symbol := range parsed.Symbols {
		nodes, err := graph.Nodes(GraphNodeQuery{Lookup: symbol, Limit: budget.SearchLimit})
		if err != nil {
			return nil, err
		}
		for _, node := range nodes {
			calls, err := graph.CallsByNode(node.ID)
			if err != nil {
				return nil, err
			}
			detail := graphNodeDetailFromGraphNode(node)
			detail.Calls = calls
			key := contextEntryKey(detail)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			entryPoints = append(entryPoints, detail)
		}
	}
	sort.SliceStable(entryPoints, func(i, j int) bool {
		return contextEntryKey(entryPoints[i]) < contextEntryKey(entryPoints[j])
	})
	if len(entryPoints) > budget.SearchLimit {
		entryPoints = entryPoints[:budget.SearchLimit]
	}
	return entryPoints, nil
}

func (s *ContextService) relationships(graph GraphStore, entryPoints []GraphNodeDetail, budget ContextBudget) ([]CallgraphEdge, error) {
	edges := make([]CallgraphEdge, 0)
	seen := make(map[string]int)
	for _, entry := range entryPoints {
		if entry.ID == "" {
			continue
		}
		for _, direction := range []CallgraphDirection{CallgraphDirectionCallees, CallgraphDirectionCallers} {
			result, err := graph.Callgraph(CallgraphQuery{
				RootNodeID:        entry.ID,
				Direction:         direction,
				Depth:             budget.Depth,
				Limit:             budget.SearchLimit,
				IncludeUnresolved: true,
			})
			if err != nil {
				return nil, err
			}
			for _, edge := range result {
				key := contextRelationshipKey(edge)
				existing, ok := seen[key]
				if ok {
					if edges[existing].Unresolved && !edge.Unresolved {
						edges[existing] = edge
					}
					continue
				}
				seen[key] = len(edges)
				edges = append(edges, edge)
			}
		}
	}
	if len(edges) > budget.SearchLimit {
		edges = edges[:budget.SearchLimit]
	}
	return edges, nil
}

func (s *ContextService) snippets(sourcePath string, entryPoints []GraphNodeDetail, budget ContextBudget) ([]ContextSnippet, error) {
	snippets := make([]ContextSnippet, 0, budget.SnippetCount)
	seen := make(map[string]struct{})
	for _, entry := range entryPoints {
		if len(snippets) >= budget.SnippetCount {
			break
		}
		if entry.Path == "" || entry.StartLine <= 0 {
			continue
		}
		key := entry.Path + ":" + intString(entry.StartLine)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		lines, err := readSourceSnippet(sourcePath, GraphNode{FilePath: entry.Path, StartLine: entry.StartLine, EndLine: entry.EndLine}, budget.SourceLines)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			continue
		}
		snippets = append(snippets, ContextSnippet{
			Path:      entry.Path,
			StartLine: lines[0].Line,
			EndLine:   lines[len(lines)-1].Line,
			Lines:     lines,
		})
	}
	return snippets, nil
}

func graphNodeDetailFromGraphNode(node GraphNode) GraphNodeDetail {
	return GraphNodeDetail{
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
	}
}

func relatedGraphFiles(files []GraphFile, entries []GraphNodeDetail, limit int) []GraphFile {
	if limit <= 0 {
		limit = len(files)
	}
	wanted := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Path != "" {
			wanted[entry.Path] = struct{}{}
		}
	}
	related := make([]GraphFile, 0, len(wanted))
	for _, file := range files {
		if _, ok := wanted[file.Path]; ok {
			related = append(related, file)
		}
	}
	sort.SliceStable(related, func(i, j int) bool { return related[i].Path < related[j].Path })
	if len(related) > limit {
		related = related[:limit]
	}
	return related
}

func splitWarnings(text string) []string {
	var warnings []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			warnings = append(warnings, line)
		}
	}
	return warnings
}

func contextEntryKey(entry GraphNodeDetail) string {
	return entry.Path + "|" + intString(entry.StartLine) + "|" + entry.QualifiedName + "|" + entry.Name
}

func contextRelationshipKey(edge CallgraphEdge) string {
	reference := edge.ReferenceName
	if reference == "" {
		reference = edge.To.ID
	}
	if reference == "" {
		reference = edge.To.QualifiedName
	}
	if reference == "" {
		reference = edge.To.Name
	}
	return edge.From.ID + "|" + string(edge.Kind) + "|" + edge.Path + "|" + intString(edge.Line) + "|" + reference
}

func intString(value int) string {
	return strconv.Itoa(value)
}
