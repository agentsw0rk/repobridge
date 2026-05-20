package codegraph_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"repobridge/internal/codegraph"
	"repobridge/internal/source"
)

func TestGraphLifecycleUseReadyNoSyncReturnsMissingError(t *testing.T) {
	sourceDir := t.TempDir()
	store := &lifecycleStore{status: codegraph.GraphStatus{Status: "missing"}}
	lifecycle := newTestLifecycle(sourceDir, store, nil)

	err := lifecycle.UseReady("demo@v1", codegraph.GraphUseOptions{SyncIndex: false}, func(codegraph.GraphSession) error {
		t.Fatal("callback called for missing ready graph")
		return nil
	})
	if err == nil {
		t.Fatal("UseReady() error = nil, want missing graph error")
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "missing") || !strings.Contains(message, "sync") {
		t.Fatalf("UseReady() error = %q, want missing sync guidance", err)
	}
	if !store.closed {
		t.Fatal("store closed = false, want true")
	}
}

func TestGraphLifecycleUseReadySyncIndexesMissingGraph(t *testing.T) {
	sourceDir := t.TempDir()
	writeCodegraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)
	store := &lifecycleStore{status: codegraph.GraphStatus{Status: "missing"}}
	lifecycle := newTestLifecycle(sourceDir, store, nil)

	var session codegraph.GraphSession
	err := lifecycle.UseReady("demo@v1", codegraph.GraphUseOptions{SyncIndex: true}, func(got codegraph.GraphSession) error {
		session = got
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.replaced {
		t.Fatal("store replaced = false, want true")
	}
	if session.Status.Status != "complete" || session.SourcePath != sourceDir || session.SourceLabel != "demo@v1" {
		t.Fatalf("session = %#v, want indexed demo session", session)
	}
	if !store.closed {
		t.Fatal("store closed = false, want true")
	}
}

func TestGraphLifecycleUseReadyNoSyncReturnsStaleError(t *testing.T) {
	sourceDir := t.TempDir()
	stored := []codegraph.GraphFile{{Path: "main.go", ContentHash: "old", Size: 1}}
	current := []codegraph.GraphFile{{Path: "main.go", ContentHash: "new", Size: 1}}
	store := &lifecycleStore{
		status: codegraph.GraphStatus{Status: "complete", SchemaVersion: codegraph.SchemaVersion},
		files:  stored,
	}
	lifecycle := newTestLifecycle(sourceDir, store, current)

	err := lifecycle.UseReady("demo@v1", codegraph.GraphUseOptions{SyncIndex: false}, func(codegraph.GraphSession) error {
		t.Fatal("callback called for stale ready graph")
		return nil
	})
	if err == nil {
		t.Fatal("UseReady() error = nil, want stale graph error")
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "stale") || !strings.Contains(message, "sync") {
		t.Fatalf("UseReady() error = %q, want stale sync guidance", err)
	}
	if !store.closed {
		t.Fatal("store closed = false, want true")
	}
}

func TestGraphLifecycleUseInspectableReportsStaleGraph(t *testing.T) {
	sourceDir := t.TempDir()
	stored := []codegraph.GraphFile{{Path: "main.go", ContentHash: "old", Size: 1}}
	current := []codegraph.GraphFile{{Path: "main.go", ContentHash: "new", Size: 1}}
	store := &lifecycleStore{
		status: codegraph.GraphStatus{Status: "complete", SchemaVersion: codegraph.SchemaVersion},
		files:  stored,
	}
	lifecycle := newTestLifecycle(sourceDir, store, current)

	var session codegraph.GraphSession
	err := lifecycle.UseInspectable("demo@v1", codegraph.GraphUseOptions{SyncIndex: false}, func(got codegraph.GraphSession) error {
		session = got
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Status.Status != "stale" {
		t.Fatalf("session status = %#v, want stale", session.Status)
	}
	if !store.closed {
		t.Fatal("store closed = false, want true")
	}
}

func TestGraphLifecycleClosesStoreWhenCallbackFails(t *testing.T) {
	sourceDir := t.TempDir()
	store := &lifecycleStore{
		status: codegraph.GraphStatus{Status: "complete", SchemaVersion: codegraph.SchemaVersion},
	}
	lifecycle := newTestLifecycle(sourceDir, store, nil)
	wantErr := errors.New("callback failed")

	err := lifecycle.UseReady("demo@v1", codegraph.GraphUseOptions{SyncIndex: false}, func(codegraph.GraphSession) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("UseReady() error = %v, want %v", err, wantErr)
	}
	if !store.closed {
		t.Fatal("store closed = false, want true")
	}
}

func newTestLifecycle(sourceDir string, store *lifecycleStore, current []codegraph.GraphFile) *codegraph.GraphLifecycleService {
	return codegraph.NewGraphLifecycle(codegraph.GraphLifecycleOptions{
		Resolver: &fakeSourceResolver{
			outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
		},
		StoreOpener: func(string) (codegraph.GraphStore, error) {
			return store, nil
		},
		CurrentFiles: func(string) ([]codegraph.GraphFile, error) {
			return current, nil
		},
	})
}

type lifecycleStore struct {
	status   codegraph.GraphStatus
	files    []codegraph.GraphFile
	snapshot codegraph.GraphSnapshot
	replaced bool
	closed   bool
}

func (s *lifecycleStore) Close() {
	s.closed = true
}

func (s *lifecycleStore) Status() (codegraph.GraphStatus, error) {
	return s.status, nil
}

func (s *lifecycleStore) Counts() (codegraph.GraphCounts, error) {
	return codegraph.GraphCounts{Files: len(s.files)}, nil
}

func (s *lifecycleStore) Files() ([]codegraph.GraphFile, error) {
	return s.files, nil
}

func (s *lifecycleStore) Snapshot() (codegraph.GraphSnapshot, error) {
	if s.snapshot.SourcePath == "" {
		s.snapshot.SourcePath = s.status.SourcePath
	}
	if len(s.snapshot.Files) == 0 {
		s.snapshot.Files = s.files
	}
	return s.snapshot, nil
}

func (s *lifecycleStore) Nodes(codegraph.GraphNodeQuery) ([]codegraph.GraphNode, error) {
	return nil, nil
}

func (s *lifecycleStore) CallsByNode(string) ([]string, error) {
	return nil, nil
}

func (s *lifecycleStore) Callgraph(codegraph.CallgraphQuery) ([]codegraph.CallgraphEdge, error) {
	return nil, nil
}

func (s *lifecycleStore) Replace(result codegraph.IndexResult) error {
	s.replaced = true
	s.files = result.Files
	s.status = codegraph.GraphStatus{
		SchemaVersion: result.SchemaVersion,
		SourcePath:    result.SourcePath,
		Status:        "complete",
		CompletedAt:   time.Now().UTC(),
	}
	return nil
}

func (s *lifecycleStore) Search(codegraph.SearchQuery) ([]codegraph.SearchResult, error) {
	return nil, nil
}
