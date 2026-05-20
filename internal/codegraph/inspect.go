package codegraph

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"repobridge/internal/cache"
	"repobridge/internal/codegraph/parser"
	"repobridge/internal/source"
)

type InspectService struct {
	resolver    SourceResolver
	indexer     *Indexer
	storeOpener StoreOpener
}

func NewInspectService(opts SearchServiceOptions) *InspectService {
	search := NewSearchService(opts)
	return &InspectService{
		resolver:    search.resolver,
		indexer:     search.indexer,
		storeOpener: search.storeOpener,
	}
}

func (s *InspectService) Status(spec string, opts GraphInspectOptions) (GraphInspectStatus, error) {
	session, err := s.openGraph(spec, opts)
	if err != nil {
		return GraphInspectStatus{}, err
	}
	defer session.graph.Close()

	status, err := s.ensureInspectableGraph(session.graph, session.outcome.Path, spec, opts.SyncIndex)
	if err != nil {
		return GraphInspectStatus{}, err
	}
	counts, err := session.graph.Counts()
	if err != nil {
		return GraphInspectStatus{}, err
	}
	counts.Warnings = warningCount(status.ErrorText)
	return inspectStatusFromGraph(session, status, counts), nil
}

func (s *InspectService) Files(spec string, opts GraphInspectOptions) (GraphFilesResult, error) {
	session, err := s.openGraph(spec, opts)
	if err != nil {
		return GraphFilesResult{}, err
	}
	defer session.graph.Close()

	if _, err := s.ensureInspectableGraph(session.graph, session.outcome.Path, spec, opts.SyncIndex); err != nil {
		return GraphFilesResult{}, err
	}
	files, err := session.graph.Files()
	if err != nil {
		return GraphFilesResult{}, err
	}
	filtered := make([]GraphFile, 0, len(files))
	for _, file := range files {
		if opts.PathFilter != "" && !strings.Contains(strings.ToLower(file.Path), strings.ToLower(opts.PathFilter)) {
			continue
		}
		filtered = append(filtered, file)
	}
	if opts.Limit > 0 && len(filtered) > opts.Limit {
		filtered = filtered[:opts.Limit]
	}
	return GraphFilesResult{Source: searchSourceLabel(session.outcome), Files: filtered}, nil
}

func (s *InspectService) Node(spec, lookup string, opts GraphInspectOptions) (GraphNodeLookupResult, error) {
	session, err := s.openGraph(spec, opts)
	if err != nil {
		return GraphNodeLookupResult{}, err
	}
	defer session.graph.Close()

	if _, err := s.ensureInspectableGraph(session.graph, session.outcome.Path, spec, opts.SyncIndex); err != nil {
		return GraphNodeLookupResult{}, err
	}
	nodes, err := session.graph.Nodes(GraphNodeQuery{Lookup: lookup, Limit: opts.Limit})
	if err != nil {
		return GraphNodeLookupResult{}, err
	}

	result := GraphNodeLookupResult{Source: searchSourceLabel(session.outcome)}
	details := make([]GraphNodeDetail, 0, len(nodes))
	for _, node := range nodes {
		detail, err := s.nodeDetail(session.graph, session.outcome.Path, node, opts.SourceLines)
		if err != nil {
			return GraphNodeLookupResult{}, err
		}
		details = append(details, detail)
	}
	if len(details) == 1 {
		result.Node = &details[0]
		return result, nil
	}
	result.Matches = details
	return result, nil
}

type inspectSession struct {
	outcome  source.Outcome
	graphDir string
	graph    GraphStore
}

func (s *InspectService) openGraph(spec string, opts GraphInspectOptions) (inspectSession, error) {
	sourceOpts := opts.SourceOpts
	if opts.CWD != "" {
		sourceOpts.CWD = opts.CWD
	}
	outcome, err := s.resolver.EnsureCached(spec, sourceOpts)
	if err != nil {
		return inspectSession{}, err
	}
	graphDir, err := cache.GraphDirForSource(outcome.Path)
	if err != nil {
		return inspectSession{}, err
	}
	graph, err := s.storeOpener(graphDir)
	if err != nil {
		return inspectSession{}, err
	}
	return inspectSession{outcome: outcome, graphDir: graphDir, graph: graph}, nil
}

func (s *InspectService) ensureInspectableGraph(graph GraphStore, sourcePath, spec string, syncIndex bool) (GraphStatus, error) {
	status, err := graph.Status()
	if err != nil {
		return GraphStatus{}, err
	}
	if needsIndex(status) {
		if !syncIndex {
			return status, nil
		}
		result, err := s.indexer.Index(sourcePath)
		if err != nil {
			return GraphStatus{}, err
		}
		if err := graph.Replace(result); err != nil {
			return GraphStatus{}, err
		}
		return graph.Status()
	}

	stale, err := s.graphIsStale(graph, sourcePath)
	if err != nil {
		return GraphStatus{}, err
	}
	if stale {
		if !syncIndex {
			status.Status = "stale"
			return status, nil
		}
		result, err := s.indexer.Index(sourcePath)
		if err != nil {
			return GraphStatus{}, fmt.Errorf("codegraph stale for %s but reindex failed: %w", spec, err)
		}
		if err := graph.Replace(result); err != nil {
			return GraphStatus{}, err
		}
		return graph.Status()
	}
	status.Status = normalizeGraphStatus(status.Status)
	return status, nil
}

func (s *InspectService) graphIsStale(graph GraphStore, sourcePath string) (bool, error) {
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

func inspectStatusFromGraph(session inspectSession, status GraphStatus, counts GraphCounts) GraphInspectStatus {
	return GraphInspectStatus{
		Source:        searchSourceLabel(session.outcome),
		SourcePath:    session.outcome.Path,
		GraphPath:     session.graphDir,
		Status:        normalizeGraphStatus(status.Status),
		SchemaVersion: status.SchemaVersion,
		ErrorText:     status.ErrorText,
		IndexedAt:     status.CompletedAt,
		Counts:        counts,
	}
}

func normalizeGraphStatus(status string) string {
	switch status {
	case "complete":
		return "ready"
	case "missing", "failed", "stale", "indexing":
		return status
	case "":
		return "missing"
	default:
		return status
	}
}

func warningCount(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func (s *InspectService) nodeDetail(graph GraphStore, sourcePath string, node GraphNode, sourceLines int) (GraphNodeDetail, error) {
	calls, err := graph.CallsByNode(node.ID)
	if err != nil {
		return GraphNodeDetail{}, err
	}
	lines, err := readSourceSnippet(sourcePath, node, sourceLines)
	if err != nil {
		return GraphNodeDetail{}, err
	}
	return GraphNodeDetail{
		ID:            node.ID,
		Kind:          node.Kind,
		Name:          node.Name,
		QualifiedName: node.QualifiedName,
		Language:      node.Language,
		Path:          node.FilePath,
		StartLine:     node.StartLine,
		EndLine:       node.EndLine,
		Signature:     node.Signature,
		Calls:         calls,
		Source:        lines,
	}, nil
}

func readSourceSnippet(sourcePath string, node GraphNode, maxLines int) ([]SourceLine, error) {
	if maxLines <= 0 || node.StartLine <= 0 || node.FilePath == "" {
		return nil, nil
	}
	cleanPath := filepath.Clean(node.FilePath)
	if filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("node path escapes source root: %s", node.FilePath)
	}
	content, err := os.ReadFile(filepath.Join(sourcePath, cleanPath))
	if err != nil {
		return nil, err
	}
	allLines := strings.Split(string(content), "\n")
	start := node.StartLine
	if start > len(allLines) {
		return nil, nil
	}
	end := node.EndLine
	if end < start {
		end = start
	}
	if maxEnd := start + maxLines - 1; end > maxEnd {
		end = maxEnd
	}
	if end > len(allLines) {
		end = len(allLines)
	}
	lines := make([]SourceLine, 0, end-start+1)
	for line := start; line <= end; line++ {
		lines = append(lines, SourceLine{Line: line, Text: allLines[line-1]})
	}
	return lines, nil
}

func lookupLooksNumeric(lookup string) (uint64, bool) {
	if lookup == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(lookup, 10, 64)
	return id, err == nil
}
