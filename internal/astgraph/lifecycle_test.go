package astgraph_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"repobridge/internal/astgraph"
	"repobridge/internal/source"
)

func TestGraphLifecycleUseReadyNoSyncReturnsMissingError(t *testing.T) {
	sourceDir := t.TempDir()
	store := &lifecycleStore{status: astgraph.GraphStatus{Status: "missing"}}
	lifecycle := newTestLifecycle(sourceDir, store, nil)

	err := lifecycle.UseReady("demo@v1", astgraph.GraphUseOptions{SyncIndex: false}, func(astgraph.GraphSession) error {
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
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)
	store := &lifecycleStore{status: astgraph.GraphStatus{Status: "missing"}}
	lifecycle := newTestLifecycle(sourceDir, store, nil)

	var session astgraph.GraphSession
	err := lifecycle.UseReady("demo@v1", astgraph.GraphUseOptions{SyncIndex: true}, func(got astgraph.GraphSession) error {
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

func TestGraphLifecycleUsesOutcomeGraphPath(t *testing.T) {
	sourceDir := t.TempDir()
	writeASTGraphFixture(t, sourceDir, "main.go", `package main
func main() {}
`)
	graphDir := t.TempDir()
	store := &lifecycleStore{status: astgraph.GraphStatus{Status: "missing"}}
	var openedDir string
	lifecycle := astgraph.NewGraphLifecycle(astgraph.GraphLifecycleOptions{
		Resolver: &fakeSourceResolver{
			outcome: source.Outcome{Path: sourceDir, Name: "project:.", SourceLabel: "project:.", GraphPath: graphDir},
		},
		StoreOpener: func(dir string) (astgraph.GraphStore, error) {
			openedDir = dir
			return store, nil
		},
	})

	err := lifecycle.UseReady("project:.", astgraph.GraphUseOptions{SyncIndex: true}, func(astgraph.GraphSession) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if openedDir != graphDir {
		t.Fatalf("opened graph dir = %q, want outcome graph dir %q", openedDir, graphDir)
	}
}

func TestGraphLifecycleCarriesOutcomeSourceKind(t *testing.T) {
	sourceDir := t.TempDir()
	store := &lifecycleStore{status: astgraph.GraphStatus{Status: "complete", SchemaVersion: astgraph.SchemaVersion}}
	lifecycle := astgraph.NewGraphLifecycle(astgraph.GraphLifecycleOptions{
		Resolver: &fakeSourceResolver{
			outcome: source.Outcome{Path: sourceDir, Name: "project:.", SourceKind: "project"},
		},
		StoreOpener: func(string) (astgraph.GraphStore, error) {
			return store, nil
		},
		CurrentFiles: func(string) ([]astgraph.GraphFile, error) {
			return nil, nil
		},
	})

	var session astgraph.GraphSession
	err := lifecycle.UseInspectable("project:.", astgraph.GraphUseOptions{SyncIndex: false}, func(got astgraph.GraphSession) error {
		session = got
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.SourceKind != "project" {
		t.Fatalf("SourceKind = %q, want project", session.SourceKind)
	}
}

func TestGraphLifecycleUseReadyNoSyncReturnsStaleError(t *testing.T) {
	sourceDir := t.TempDir()
	stored := []astgraph.GraphFile{{Path: "main.go", ContentHash: "old", Size: 1}}
	current := []astgraph.GraphFile{{Path: "main.go", ContentHash: "new", Size: 1}}
	store := &lifecycleStore{
		status: astgraph.GraphStatus{Status: "complete", SchemaVersion: astgraph.SchemaVersion},
		files:  stored,
	}
	lifecycle := newTestLifecycle(sourceDir, store, current)

	err := lifecycle.UseReady("demo@v1", astgraph.GraphUseOptions{SyncIndex: false}, func(astgraph.GraphSession) error {
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
	stored := []astgraph.GraphFile{{Path: "main.go", ContentHash: "old", Size: 1}}
	current := []astgraph.GraphFile{{Path: "main.go", ContentHash: "new", Size: 1}}
	store := &lifecycleStore{
		status: astgraph.GraphStatus{Status: "complete", SchemaVersion: astgraph.SchemaVersion},
		files:  stored,
	}
	lifecycle := newTestLifecycle(sourceDir, store, current)

	var session astgraph.GraphSession
	err := lifecycle.UseInspectable("demo@v1", astgraph.GraphUseOptions{SyncIndex: false}, func(got astgraph.GraphSession) error {
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
		status: astgraph.GraphStatus{Status: "complete", SchemaVersion: astgraph.SchemaVersion},
	}
	lifecycle := newTestLifecycle(sourceDir, store, nil)
	wantErr := errors.New("callback failed")

	err := lifecycle.UseReady("demo@v1", astgraph.GraphUseOptions{SyncIndex: false}, func(astgraph.GraphSession) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("UseReady() error = %v, want %v", err, wantErr)
	}
	if !store.closed {
		t.Fatal("store closed = false, want true")
	}
}

func newTestLifecycle(sourceDir string, store *lifecycleStore, current []astgraph.GraphFile) *astgraph.GraphLifecycleService {
	return astgraph.NewGraphLifecycle(astgraph.GraphLifecycleOptions{
		Resolver: &fakeSourceResolver{
			outcome: source.Outcome{Path: sourceDir, Name: "demo", Version: "v1"},
		},
		StoreOpener: func(string) (astgraph.GraphStore, error) {
			return store, nil
		},
		CurrentFiles: func(string) ([]astgraph.GraphFile, error) {
			return current, nil
		},
	})
}

type lifecycleStore struct {
	status   astgraph.GraphStatus
	files    []astgraph.GraphFile
	snapshot astgraph.GraphSnapshot
	replaced bool
	closed   bool
}

func (s *lifecycleStore) Close() {
	s.closed = true
}

func (s *lifecycleStore) Status() (astgraph.GraphStatus, error) {
	return s.status, nil
}

func (s *lifecycleStore) Counts() (astgraph.GraphCounts, error) {
	return astgraph.GraphCounts{Files: len(s.files)}, nil
}

func (s *lifecycleStore) Files() ([]astgraph.GraphFile, error) {
	return s.files, nil
}

func (s *lifecycleStore) Snapshot() (astgraph.GraphSnapshot, error) {
	if s.snapshot.SourcePath == "" {
		s.snapshot.SourcePath = s.status.SourcePath
	}
	if len(s.snapshot.Files) == 0 {
		s.snapshot.Files = s.files
	}
	return s.snapshot, nil
}

func (s *lifecycleStore) Nodes(astgraph.GraphNodeQuery) ([]astgraph.GraphNode, error) {
	return nil, nil
}

func (s *lifecycleStore) CallsByNode(string) ([]string, error) {
	return nil, nil
}

func (s *lifecycleStore) Callgraph(astgraph.CallgraphQuery) ([]astgraph.CallgraphEdge, error) {
	return nil, nil
}

func (s *lifecycleStore) Replace(result astgraph.IndexResult) error {
	s.replaced = true
	s.files = result.Files
	s.status = astgraph.GraphStatus{
		SchemaVersion: result.SchemaVersion,
		SourcePath:    result.SourcePath,
		Status:        "complete",
		CompletedAt:   time.Now().UTC(),
	}
	return nil
}

func (s *lifecycleStore) Search(astgraph.SearchQuery) ([]astgraph.SearchResult, error) {
	return nil, nil
}
