package astgraph

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type InspectService struct {
	lifecycle GraphLifecycle
}

func NewInspectService(opts SearchServiceOptions) *InspectService {
	search := NewSearchService(opts)
	return &InspectService{lifecycle: search.lifecycle}
}

func (s *InspectService) Status(spec string, opts GraphInspectOptions) (GraphInspectStatus, error) {
	var result GraphInspectStatus
	err := s.lifecycle.UseInspectable(spec, inspectGraphUseOptions(opts), func(session GraphSession) error {
		counts, err := session.Store.Counts()
		if err != nil {
			return err
		}
		counts.Warnings = warningCount(session.Status.ErrorText)
		result = inspectStatusFromSession(session, counts)
		return nil
	})
	return result, err
}

func (s *InspectService) Files(spec string, opts GraphInspectOptions) (GraphFilesResult, error) {
	var result GraphFilesResult
	err := s.lifecycle.UseInspectable(spec, inspectGraphUseOptions(opts), func(session GraphSession) error {
		files, err := session.Store.Files()
		if err != nil {
			return err
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
		result = GraphFilesResult{Source: session.SourceLabel, Files: filtered}
		return nil
	})
	return result, err
}

func (s *InspectService) Node(spec, lookup string, opts GraphInspectOptions) (GraphNodeLookupResult, error) {
	var result GraphNodeLookupResult
	err := s.lifecycle.UseInspectable(spec, inspectGraphUseOptions(opts), func(session GraphSession) error {
		nodes, err := session.Store.Nodes(GraphNodeQuery{Lookup: lookup, Limit: opts.Limit})
		if err != nil {
			return err
		}
		result = GraphNodeLookupResult{Source: session.SourceLabel}
		details := make([]GraphNodeDetail, 0, len(nodes))
		for _, node := range nodes {
			detail, err := s.nodeDetail(session.Store, session.SourcePath, node, opts.SourceLines)
			if err != nil {
				return err
			}
			details = append(details, detail)
		}
		if len(details) == 1 {
			result.Node = &details[0]
			return nil
		}
		result.Matches = details
		return nil
	})
	return result, err
}

func inspectStatusFromSession(session GraphSession, counts GraphCounts) GraphInspectStatus {
	return GraphInspectStatus{
		Source:        session.SourceLabel,
		SourcePath:    session.SourcePath,
		GraphPath:     session.GraphDir,
		Status:        normalizeGraphStatus(session.Status.Status),
		SchemaVersion: session.Status.SchemaVersion,
		ErrorText:     session.Status.ErrorText,
		IndexedAt:     session.Status.CompletedAt,
		Counts:        counts,
	}
}

func inspectGraphUseOptions(opts GraphInspectOptions) GraphUseOptions {
	return GraphUseOptions{
		CWD:        opts.CWD,
		SourceOpts: opts.SourceOpts,
		SyncIndex:  opts.SyncIndex,
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
		Calls:          calls,
		Source:         lines,
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
