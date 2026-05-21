package astgraph

import (
	"sort"
	"strings"
)

type GraphSnapshot struct {
	SourcePath string
	Files      []GraphFile
	Nodes      []GraphNode
	Edges      []GraphEdge
	Unresolved []UnresolvedReference
}

type QueryOptions struct {
	SourceLabel string
	MaxDepth    int
}

type GraphQueries interface {
	Search(SearchQuery) ([]SearchResult, error)
}

func NewGraphQueries(snapshot GraphSnapshot, opts QueryOptions) GraphQueries {
	return &snapshotGraphQueries{
		snapshot: snapshot,
		opts:     opts,
	}
}

type snapshotGraphQueries struct {
	snapshot GraphSnapshot
	opts     QueryOptions
}

func (q *snapshotGraphQueries) Search(query SearchQuery) ([]SearchResult, error) {
	type scoredResult struct {
		result   SearchResult
		stableID string
	}

	scored := make([]scoredResult, 0, len(q.snapshot.Nodes))
	for _, node := range q.snapshot.Nodes {
		score, ok := scoreGraphNode(node, query)
		if !ok {
			continue
		}
		calls := q.callsByNode(node.ID)
		if !callsMatchFilters(calls, query.Calls) {
			continue
		}
		scored = append(scored, scoredResult{
			result: SearchResult{
				Source:         q.sourceLabel(),
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
				Score:          score,
				Calls:          calls,
			},
			stableID: node.ID,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		left := scored[i].result
		right := scored[j].result
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.StartLine != right.StartLine {
			return left.StartLine < right.StartLine
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.QualifiedName != right.QualifiedName {
			return left.QualifiedName < right.QualifiedName
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Language != right.Language {
			return left.Language < right.Language
		}
		if left.EndLine != right.EndLine {
			return left.EndLine < right.EndLine
		}
		return scored[i].stableID < scored[j].stableID
	})

	results := make([]SearchResult, 0, len(scored))
	for _, result := range scored {
		results = append(results, result.result)
	}
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}
	return results, nil
}

func (q *snapshotGraphQueries) sourceLabel() string {
	if q.opts.SourceLabel != "" {
		return q.opts.SourceLabel
	}
	return q.snapshot.SourcePath
}

func (q *snapshotGraphQueries) callsByNode(stableID string) []string {
	nodeNames := make(map[string]string, len(q.snapshot.Nodes))
	for _, node := range q.snapshot.Nodes {
		nodeNames[node.ID] = node.Name
	}

	names := make(map[string]struct{})
	for _, edge := range q.snapshot.Edges {
		if edge.SourceNodeID != stableID || edge.Kind != EdgeKindCalls {
			continue
		}
		if name := nodeNames[edge.TargetNodeID]; name != "" {
			names[name] = struct{}{}
		}
	}
	for _, ref := range q.snapshot.Unresolved {
		if ref.FromNodeID != stableID || ref.ReferenceKind != EdgeKindCalls || ref.ReferenceName == "" {
			continue
		}
		names[ref.ReferenceName] = struct{}{}
	}

	calls := make([]string, 0, len(names))
	for name := range names {
		calls = append(calls, name)
	}
	sort.Strings(calls)
	return calls
}

func scoreGraphNode(node GraphNode, query SearchQuery) (float64, bool) {
	if !graphValueMatchesAny(string(node.Kind), graphNodeKindStrings(query.Kinds), true) {
		return 0, false
	}
	if !graphValueMatchesAny(string(node.Language), graphLanguageStrings(query.Languages), true) {
		return 0, false
	}
	if !graphNodeMatchesPathFilters(node, query.PathFilters) {
		return 0, false
	}
	if !graphNodeMatchesNameFilters(node, query.NameFilters) {
		return 0, false
	}
	if !graphNodeMatchesText(node, query.Text) {
		return 0, false
	}

	score := 1.0
	for _, filter := range query.NameFilters {
		filter = strings.ToLower(filter)
		name := strings.ToLower(node.Name)
		qualified := strings.ToLower(node.QualifiedName)
		switch {
		case name == filter:
			score += 100
		case strings.Contains(name, filter):
			score += 75
		case qualified == filter:
			score += 60
		case strings.Contains(qualified, filter):
			score += 45
		}
	}
	if query.Text != "" {
		text := strings.ToLower(query.Text)
		if strings.EqualFold(node.Name, query.Text) {
			score += 50
		} else if strings.Contains(strings.ToLower(node.Name), text) || strings.Contains(strings.ToLower(node.QualifiedName), text) {
			score += 35
		} else if strings.Contains(strings.ToLower(node.Signature), text) {
			score += 20
		} else if strings.Contains(strings.ToLower(node.FilePath), text) {
			score += 10
		}
	}
	for _, filter := range query.PathFilters {
		if graphRouteOrFileMatchesPathFilter(node, filter) {
			score += 5
		}
	}
	return score, true
}

func graphNodeMatchesPathFilters(node GraphNode, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if graphRouteOrFileMatchesPathFilter(node, filter) {
			return true
		}
	}
	return false
}

func graphRouteOrFileMatchesPathFilter(node GraphNode, filter string) bool {
	filter = strings.ToLower(filter)
	if strings.Contains(strings.ToLower(node.FilePath), filter) {
		return true
	}
	if node.Kind != NodeKindRoute {
		return false
	}
	return strings.Contains(strings.ToLower(node.Name), filter) ||
		strings.Contains(strings.ToLower(node.QualifiedName), filter) ||
		strings.Contains(strings.ToLower(node.Signature), filter)
}

func graphNodeMatchesNameFilters(node GraphNode, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		filter = strings.ToLower(filter)
		if strings.Contains(strings.ToLower(node.Name), filter) || strings.Contains(strings.ToLower(node.QualifiedName), filter) {
			return true
		}
	}
	return false
}

func graphNodeMatchesText(node GraphNode, text string) bool {
	if text == "" {
		return true
	}
	text = strings.ToLower(text)
	return strings.Contains(strings.ToLower(node.Name), text) ||
		strings.Contains(strings.ToLower(node.QualifiedName), text) ||
		strings.Contains(strings.ToLower(node.FilePath), text) ||
		strings.Contains(strings.ToLower(node.Signature), text)
}

func callsMatchFilters(calls []string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		filter = strings.ToLower(filter)
		for _, call := range calls {
			if strings.Contains(strings.ToLower(call), filter) {
				return true
			}
		}
	}
	return false
}

func graphValueMatchesAny(value string, filters []string, exact bool) bool {
	if len(filters) == 0 {
		return true
	}
	value = strings.ToLower(value)
	for _, filter := range filters {
		filter = strings.ToLower(filter)
		if exact && value == filter {
			return true
		}
		if !exact && strings.Contains(value, filter) {
			return true
		}
	}
	return false
}

func graphNodeKindStrings(kinds []NodeKind) []string {
	values := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		values = append(values, string(kind))
	}
	return values
}

func graphLanguageStrings(languages []Language) []string {
	values := make([]string, 0, len(languages))
	for _, language := range languages {
		values = append(values, string(language))
	}
	return values
}
