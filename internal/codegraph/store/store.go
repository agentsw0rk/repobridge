package store

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/objectbox/objectbox-go/objectbox"

	"repobridge/internal/codegraph"
)

type Store struct {
	ob *objectbox.ObjectBox
}

type Status = codegraph.GraphStatus

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	ob, err := openObjectBox(dir)
	if err != nil {
		if _, statErr := os.Stat(dir); statErr != nil {
			return nil, err
		}
		corruptDir := fmt.Sprintf("%s.corrupt.%d", dir, time.Now().UnixNano())
		if renameErr := os.Rename(dir, corruptDir); renameErr != nil {
			return nil, fmt.Errorf("open codegraph store failed: %v; failed to move corrupt store to %s: %w", err, corruptDir, renameErr)
		}
		if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
			return nil, fmt.Errorf("open codegraph store failed: %v; moved corrupt store to %s but failed to recreate graph dir: %w", err, corruptDir, mkdirErr)
		}
		retry, retryErr := openObjectBox(dir)
		if retryErr != nil {
			return nil, fmt.Errorf("open codegraph store failed: %v; moved corrupt store to %s but retry failed: %w", err, corruptDir, retryErr)
		}
		ob = retry
	}

	return &Store{ob: ob}, nil
}

func openObjectBox(dir string) (*objectbox.ObjectBox, error) {
	return objectbox.NewBuilder().Directory(dir).Model(ObjectBoxModel()).Build()
}

func (s *Store) Close() {
	if s == nil || s.ob == nil {
		return
	}
	s.ob.Close()
}

func (s *Store) Status() (Status, error) {
	var status Status
	err := s.ob.RunInReadTx(func() error {
		var err error
		status, err = s.statusInTx()
		return err
	})
	return status, err
}

func (s *Store) statusInTx() (Status, error) {
	metadata, err := BoxForMetadataEntity(s.ob).GetAll()
	if err != nil {
		return Status{}, err
	}
	if len(metadata) == 0 {
		return Status{Status: "missing"}, nil
	}

	sort.Slice(metadata, func(i, j int) bool {
		if metadata[i].CompletedAt.Equal(metadata[j].CompletedAt) {
			return metadata[i].Id < metadata[j].Id
		}
		return metadata[i].CompletedAt.After(metadata[j].CompletedAt)
	})

	current := metadata[0]
	return Status{
		SchemaVersion: current.SchemaVersion,
		SourcePath:    current.SourcePath,
		Status:        current.Status,
		ErrorText:     current.ErrorText,
		CompletedAt:   current.CompletedAt,
	}, nil
}

func (s *Store) Replace(result codegraph.IndexResult) error {
	return s.ob.RunInWriteTx(func() error {
		unresolvedBox := BoxForUnresolvedReferenceEntity(s.ob)
		edgeBox := BoxForEdgeEntity(s.ob)
		nodeBox := BoxForNodeEntity(s.ob)
		fileBox := BoxForFileEntity(s.ob)
		metadataBox := BoxForMetadataEntity(s.ob)

		if err := unresolvedBox.RemoveAll(); err != nil {
			return err
		}
		if err := edgeBox.RemoveAll(); err != nil {
			return err
		}
		if err := nodeBox.RemoveAll(); err != nil {
			return err
		}
		if err := fileBox.RemoveAll(); err != nil {
			return err
		}
		if err := metadataBox.RemoveAll(); err != nil {
			return err
		}

		for _, entity := range fileEntities(result.Files) {
			if _, err := fileBox.Put(entity); err != nil {
				return err
			}
		}
		for _, entity := range nodeEntities(result.Nodes) {
			if _, err := nodeBox.Put(entity); err != nil {
				return err
			}
		}
		for _, entity := range edgeEntities(result.Edges) {
			if _, err := edgeBox.Put(entity); err != nil {
				return err
			}
		}
		for _, entity := range unresolvedReferenceEntities(result.Unresolved) {
			if _, err := unresolvedBox.Put(entity); err != nil {
				return err
			}
		}

		_, err := metadataBox.Put(&MetadataEntity{
			SchemaVersion: result.SchemaVersion,
			SourcePath:    result.SourcePath,
			Status:        "complete",
			ErrorText:     strings.Join(result.Warnings, "\n"),
			StartedAt:     result.StartedAt,
			CompletedAt:   result.CompletedAt,
		})
		return err
	})
}

func (s *Store) MarkFailed(sourcePath string, startedAt time.Time, indexErr error) error {
	errorText := ""
	if indexErr != nil {
		errorText = indexErr.Error()
	}
	completedAt := time.Now().UTC()
	return s.ob.RunInWriteTx(func() error {
		metadataBox := BoxForMetadataEntity(s.ob)
		if err := metadataBox.RemoveAll(); err != nil {
			return err
		}
		_, err := metadataBox.Put(&MetadataEntity{
			SchemaVersion: codegraph.SchemaVersion,
			SourcePath:    sourcePath,
			Status:        "failed",
			ErrorText:     errorText,
			StartedAt:     startedAt,
			CompletedAt:   completedAt,
		})
		return err
	})
}

func (s *Store) Files() ([]codegraph.GraphFile, error) {
	var files []codegraph.GraphFile
	err := s.ob.RunInReadTx(func() error {
		entities, err := BoxForFileEntity(s.ob).GetAll()
		if err != nil {
			return err
		}
		files = make([]codegraph.GraphFile, 0, len(entities))
		for _, entity := range entities {
			files = append(files, graphFileFromEntity(entity))
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		return nil
	})
	return files, err
}

func (s *Store) Counts() (codegraph.GraphCounts, error) {
	var counts codegraph.GraphCounts
	err := s.ob.RunInReadTx(func() error {
		files, err := BoxForFileEntity(s.ob).GetAll()
		if err != nil {
			return err
		}
		nodes, err := BoxForNodeEntity(s.ob).GetAll()
		if err != nil {
			return err
		}
		edges, err := BoxForEdgeEntity(s.ob).GetAll()
		if err != nil {
			return err
		}
		unresolved, err := BoxForUnresolvedReferenceEntity(s.ob).GetAll()
		if err != nil {
			return err
		}
		counts.Files = len(files)
		counts.Nodes = len(nodes)
		counts.Edges = len(edges)
		counts.Unresolved = len(unresolved)
		return nil
	})
	return counts, err
}

func (s *Store) Nodes(query codegraph.GraphNodeQuery) ([]codegraph.GraphNode, error) {
	var nodes []codegraph.GraphNode
	err := s.ob.RunInReadTx(func() error {
		entities, err := BoxForNodeEntity(s.ob).GetAll()
		if err != nil {
			return err
		}
		lookup := strings.TrimSpace(query.Lookup)
		numericID, hasNumericID := parseObjectBoxID(lookup)
		var exact []codegraph.GraphNode
		var fuzzy []codegraph.GraphNode
		for _, entity := range entities {
			node := graphNodeFromEntity(entity)
			if lookup == "" {
				nodes = append(nodes, node)
				continue
			}
			if nodeMatchesExactLookup(entity, lookup, numericID, hasNumericID) {
				exact = append(exact, node)
				continue
			}
			if nodeMatchesFuzzyLookup(entity, lookup) {
				fuzzy = append(fuzzy, node)
			}
		}
		if lookup != "" {
			if len(exact) > 0 {
				nodes = exact
			} else {
				nodes = fuzzy
			}
		}
		sort.SliceStable(nodes, func(i, j int) bool {
			if nodes[i].FilePath != nodes[j].FilePath {
				return nodes[i].FilePath < nodes[j].FilePath
			}
			if nodes[i].StartLine != nodes[j].StartLine {
				return nodes[i].StartLine < nodes[j].StartLine
			}
			if nodes[i].QualifiedName != nodes[j].QualifiedName {
				return nodes[i].QualifiedName < nodes[j].QualifiedName
			}
			if nodes[i].Name != nodes[j].Name {
				return nodes[i].Name < nodes[j].Name
			}
			return nodes[i].ID < nodes[j].ID
		})
		if query.Limit > 0 && len(nodes) > query.Limit {
			nodes = nodes[:query.Limit]
		}
		return nil
	})
	return nodes, err
}

func (s *Store) Search(query codegraph.SearchQuery) ([]codegraph.SearchResult, error) {
	var results []codegraph.SearchResult
	err := s.ob.RunInReadTx(func() error {
		nodes, err := BoxForNodeEntity(s.ob).GetAll()
		if err != nil {
			return err
		}

		status, err := s.statusInTx()
		if err != nil {
			return err
		}

		type scoredResult struct {
			result   codegraph.SearchResult
			stableID string
		}

		scored := make([]scoredResult, 0, len(nodes))
		for _, node := range nodes {
			score, ok := scoreNode(node, query)
			if !ok {
				continue
			}

			calls, err := s.callsByNodeInTx(node.StableID)
			if err != nil {
				return err
			}
			if !matchesCallFilters(calls, query.Calls) {
				continue
			}

			scored = append(scored, scoredResult{
				result: codegraph.SearchResult{
					Source:        status.SourcePath,
					Kind:          codegraph.NodeKind(node.Kind),
					Name:          node.Name,
					QualifiedName: node.QualifiedName,
					Language:      codegraph.Language(node.Language),
					Path:          node.FilePath,
					StartLine:     node.StartLine,
					EndLine:       node.EndLine,
					Score:         score,
					Calls:         calls,
				},
				stableID: node.StableID,
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

		results = make([]codegraph.SearchResult, 0, len(scored))
		for _, result := range scored {
			results = append(results, result.result)
		}
		if query.Limit > 0 && len(results) > query.Limit {
			results = results[:query.Limit]
		}
		return nil
	})
	return results, err
}

func (s *Store) CallsByNode(stableID string) ([]string, error) {
	var calls []string
	err := s.ob.RunInReadTx(func() error {
		var err error
		calls, err = s.callsByNodeInTx(stableID)
		return err
	})
	return calls, err
}

func (s *Store) callsByNodeInTx(stableID string) ([]string, error) {
	unresolved, err := BoxForUnresolvedReferenceEntity(s.ob).GetAll()
	if err != nil {
		return nil, err
	}

	names := make(map[string]struct{})
	for _, ref := range unresolved {
		if ref.FromStableID != stableID || ref.ReferenceKind != string(codegraph.EdgeKindCalls) || ref.ReferenceName == "" {
			continue
		}
		names[ref.ReferenceName] = struct{}{}
	}

	edges, err := BoxForEdgeEntity(s.ob).GetAll()
	if err != nil {
		return nil, err
	}
	nodes, err := BoxForNodeEntity(s.ob).GetAll()
	if err != nil {
		return nil, err
	}
	nodeNames := make(map[string]string, len(nodes))
	for _, node := range nodes {
		nodeNames[node.StableID] = node.Name
	}
	for _, edge := range edges {
		if edge.SourceStableID != stableID || edge.Kind != string(codegraph.EdgeKindCalls) {
			continue
		}
		if name := nodeNames[edge.TargetStableID]; name != "" {
			names[name] = struct{}{}
		}
	}

	calls := make([]string, 0, len(names))
	for name := range names {
		calls = append(calls, name)
	}
	sort.Strings(calls)
	return calls, nil
}

func fileEntities(files []codegraph.GraphFile) []*FileEntity {
	entities := make([]*FileEntity, 0, len(files))
	for _, file := range files {
		entities = append(entities, &FileEntity{
			Path:        file.Path,
			Language:    string(file.Language),
			ContentHash: file.ContentHash,
			Size:        file.Size,
			ModifiedAt:  file.ModifiedAt,
			IndexedAt:   file.IndexedAt,
			NodeCount:   file.NodeCount,
		})
	}
	return entities
}

func graphFileFromEntity(file *FileEntity) codegraph.GraphFile {
	return codegraph.GraphFile{
		Path:        file.Path,
		Language:    codegraph.Language(file.Language),
		ContentHash: file.ContentHash,
		Size:        file.Size,
		ModifiedAt:  file.ModifiedAt,
		IndexedAt:   file.IndexedAt,
		NodeCount:   file.NodeCount,
	}
}

func graphNodeFromEntity(node *NodeEntity) codegraph.GraphNode {
	return codegraph.GraphNode{
		ID:            node.StableID,
		Kind:          codegraph.NodeKind(node.Kind),
		Name:          node.Name,
		QualifiedName: node.QualifiedName,
		FilePath:      node.FilePath,
		Language:      codegraph.Language(node.Language),
		StartLine:     node.StartLine,
		EndLine:       node.EndLine,
		StartColumn:   node.StartColumn,
		EndColumn:     node.EndColumn,
		Signature:     node.Signature,
	}
}

func nodeMatchesExactLookup(node *NodeEntity, lookup string, numericID uint64, hasNumericID bool) bool {
	if hasNumericID && node.Id == numericID {
		return true
	}
	return node.StableID == lookup || node.QualifiedName == lookup || node.Name == lookup
}

func nodeMatchesFuzzyLookup(node *NodeEntity, lookup string) bool {
	lookup = strings.ToLower(lookup)
	return strings.Contains(strings.ToLower(node.QualifiedName), lookup) || strings.Contains(strings.ToLower(node.Name), lookup)
}

func parseObjectBoxID(lookup string) (uint64, bool) {
	id, err := strconv.ParseUint(lookup, 10, 64)
	return id, err == nil
}

func nodeEntities(nodes []codegraph.GraphNode) []*NodeEntity {
	entities := make([]*NodeEntity, 0, len(nodes))
	for _, node := range nodes {
		entities = append(entities, &NodeEntity{
			StableID:      node.ID,
			Kind:          string(node.Kind),
			Name:          node.Name,
			QualifiedName: node.QualifiedName,
			FilePath:      node.FilePath,
			Language:      string(node.Language),
			StartLine:     node.StartLine,
			EndLine:       node.EndLine,
			StartColumn:   node.StartColumn,
			EndColumn:     node.EndColumn,
			Signature:     node.Signature,
		})
	}
	return entities
}

func edgeEntities(edges []codegraph.GraphEdge) []*EdgeEntity {
	entities := make([]*EdgeEntity, 0, len(edges))
	for _, edge := range edges {
		entities = append(entities, &EdgeEntity{
			SourceStableID: edge.SourceNodeID,
			TargetStableID: edge.TargetNodeID,
			Kind:           string(edge.Kind),
			FilePath:       edge.FilePath,
			Line:           edge.Line,
			Column:         edge.Column,
			Provenance:     edge.Provenance,
		})
	}
	return entities
}

func unresolvedReferenceEntities(refs []codegraph.UnresolvedReference) []*UnresolvedReferenceEntity {
	entities := make([]*UnresolvedReferenceEntity, 0, len(refs))
	for _, ref := range refs {
		entities = append(entities, &UnresolvedReferenceEntity{
			FromStableID:  ref.FromNodeID,
			ReferenceName: ref.ReferenceName,
			ReferenceKind: string(ref.ReferenceKind),
			FilePath:      ref.FilePath,
			Language:      string(ref.Language),
			Line:          ref.Line,
			Column:        ref.Column,
		})
	}
	return entities
}

func scoreNode(node *NodeEntity, query codegraph.SearchQuery) (float64, bool) {
	if !matchesAny(string(codegraph.NodeKind(node.Kind)), nodeKindStrings(query.Kinds), true) {
		return 0, false
	}
	if !matchesAny(string(codegraph.Language(node.Language)), languageStrings(query.Languages), true) {
		return 0, false
	}
	if !matchesAny(node.FilePath, query.PathFilters, false) {
		return 0, false
	}
	if !matchesNameFilters(node, query.NameFilters) {
		return 0, false
	}
	if !matchesText(node, query.Text) {
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
		if strings.Contains(strings.ToLower(node.FilePath), strings.ToLower(filter)) {
			score += 5
		}
	}
	return score, true
}

func matchesAny(value string, filters []string, exact bool) bool {
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

func matchesNameFilters(node *NodeEntity, filters []string) bool {
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

func matchesText(node *NodeEntity, text string) bool {
	if text == "" {
		return true
	}
	text = strings.ToLower(text)
	return strings.Contains(strings.ToLower(node.Name), text) ||
		strings.Contains(strings.ToLower(node.QualifiedName), text) ||
		strings.Contains(strings.ToLower(node.FilePath), text) ||
		strings.Contains(strings.ToLower(node.Signature), text)
}

func matchesCallFilters(calls []string, filters []string) bool {
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

func nodeKindStrings(kinds []codegraph.NodeKind) []string {
	values := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		values = append(values, string(kind))
	}
	return values
}

func languageStrings(languages []codegraph.Language) []string {
	values := make([]string, 0, len(languages))
	for _, language := range languages {
		values = append(values, string(language))
	}
	return values
}
