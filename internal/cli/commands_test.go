package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"repobridge/internal/cache"
	"repobridge/internal/codegraph"
	"repobridge/internal/codegraph/store"
	"repobridge/internal/source"
)

func executeForTest(args ...string) (string, string, error) {
	return executeForTestWithOptions(Options{}, args...)
}

func executeForTestWithOptions(opts Options, args ...string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	opts.Version = "test-version"
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if opts.Indexer == nil {
		opts.Indexer = &fakeIndexer{}
	}
	cmd := NewRootCommand(opts)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

type ensureCall struct {
	spec string
	opts source.Options
}

type fakeApp struct {
	outcomes        map[string]source.Outcome
	calls           []ensureCall
	searchResults   []codegraph.SearchResult
	searchSpec      string
	searchQuery     string
	searchOpts      codegraph.SearchOptions
	statusResult    codegraph.GraphInspectStatus
	statusSpec      string
	statusOpts      codegraph.GraphInspectOptions
	filesResult     codegraph.GraphFilesResult
	filesSpec       string
	filesOpts       codegraph.GraphInspectOptions
	nodeResult      codegraph.GraphNodeLookupResult
	nodeSpec        string
	nodeLookup      string
	nodeOpts        codegraph.GraphInspectOptions
	callgraphResult codegraph.CallgraphResult
	callgraphSpec   string
	callgraphSymbol string
	callgraphOpts   codegraph.CallgraphOptions
	contextResult   codegraph.ContextResult
	contextSpec     string
	contextQuery    string
	contextOpts     codegraph.ContextOptions
}

func (a *fakeApp) EnsureCached(spec string, opts source.Options) (source.Outcome, error) {
	a.calls = append(a.calls, ensureCall{spec: spec, opts: opts})
	return a.outcomes[spec], nil
}

func (a *fakeApp) SearchCode(spec, rawQuery string, opts codegraph.SearchOptions) ([]codegraph.SearchResult, error) {
	a.searchSpec = spec
	a.searchQuery = rawQuery
	a.searchOpts = opts
	return a.searchResults, nil
}

func (a *fakeApp) CodeGraphStatus(spec string, opts codegraph.GraphInspectOptions) (codegraph.GraphInspectStatus, error) {
	a.statusSpec = spec
	a.statusOpts = opts
	return a.statusResult, nil
}

func (a *fakeApp) CodeGraphFiles(spec string, opts codegraph.GraphInspectOptions) (codegraph.GraphFilesResult, error) {
	a.filesSpec = spec
	a.filesOpts = opts
	return a.filesResult, nil
}

func (a *fakeApp) CodeGraphNode(spec, lookup string, opts codegraph.GraphInspectOptions) (codegraph.GraphNodeLookupResult, error) {
	a.nodeSpec = spec
	a.nodeLookup = lookup
	a.nodeOpts = opts
	return a.nodeResult, nil
}

func (a *fakeApp) CodeGraphCallgraph(spec, symbol string, opts codegraph.CallgraphOptions) (codegraph.CallgraphResult, error) {
	a.callgraphSpec = spec
	a.callgraphSymbol = symbol
	a.callgraphOpts = opts
	return a.callgraphResult, nil
}

func (a *fakeApp) CodeGraphContext(spec, query string, opts codegraph.ContextOptions) (codegraph.ContextResult, error) {
	a.contextSpec = spec
	a.contextQuery = query
	a.contextOpts = opts
	return a.contextResult, nil
}

type fakeIndexer struct {
	mu       sync.Mutex
	outcomes []source.Outcome
	waited   bool
}

func (i *fakeIndexer) Schedule(outcome source.Outcome) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.outcomes = append(i.outcomes, outcome)
}

func (i *fakeIndexer) Wait() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.waited = true
}

func (i *fakeIndexer) scheduledOutcomes() []source.Outcome {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]source.Outcome(nil), i.outcomes...)
}

func (i *fakeIndexer) waitedForIndexing() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.waited
}

type blockingWaitIndexer struct {
	fakeIndexer
	waitStarted chan struct{}
	releaseWait chan struct{}
	once        sync.Once
}

func newBlockingWaitIndexer() *blockingWaitIndexer {
	return &blockingWaitIndexer{
		waitStarted: make(chan struct{}),
		releaseWait: make(chan struct{}),
	}
}

func (i *blockingWaitIndexer) Wait() {
	i.fakeIndexer.Wait()
	i.once.Do(func() {
		close(i.waitStarted)
	})
	<-i.releaseWait
}

type commandResult struct {
	stdout string
	stderr string
	err    error
}

func assertCommandDoesNotWaitForIndexer(t *testing.T, opts Options, indexer *blockingWaitIndexer, args ...string) commandResult {
	t.Helper()
	results := make(chan commandResult, 1)
	go func() {
		stdout, stderr, err := executeForTestWithOptions(opts, args...)
		results <- commandResult{stdout: stdout, stderr: stderr, err: err}
	}()

	select {
	case result := <-results:
		return result
	case <-indexer.waitStarted:
		close(indexer.releaseWait)
		t.Fatal("command called Wait on indexer")
	case <-time.After(time.Second):
		t.Fatal("command did not return")
		return commandResult{}
	}
	return commandResult{}
}

func withHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", dir)
	return dir
}

func TestRootVersion(t *testing.T) {
	stdout, _, err := executeForTest("--version")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "test-version") {
		t.Fatalf("stdout = %q, want version", stdout)
	}
}

func TestIndexOutcomePathMarksGraphFailedWhenIndexingFails(t *testing.T) {
	sourceDir := t.TempDir()
	originalIndexSourcePath := indexSourcePath
	indexSourcePath = func(sourcePath string) (codegraph.IndexResult, error) {
		return codegraph.IndexResult{}, errors.New("parse failed")
	}
	defer func() {
		indexSourcePath = originalIndexSourcePath
	}()

	err := indexOutcomePath(sourceDir)
	if err == nil {
		t.Fatal("indexOutcomePath() error = nil, want index error")
	}

	graphDir, graphErr := cache.GraphDirForSource(sourceDir)
	if graphErr != nil {
		t.Fatal(graphErr)
	}
	graph, graphErr := store.Open(graphDir)
	if graphErr != nil {
		t.Fatal(graphErr)
	}
	defer graph.Close()

	status, graphErr := graph.Status()
	if graphErr != nil {
		t.Fatal(graphErr)
	}
	if status.Status != "failed" || status.SourcePath != sourceDir || status.SchemaVersion != codegraph.SchemaVersion {
		t.Fatalf("status = %#v, want failed status for source", status)
	}
	if status.ErrorText != "parse failed" {
		t.Fatalf("error text = %q, want parse failed", status.ErrorText)
	}
}

func TestRemoveAlias(t *testing.T) {
	_, _, err := executeForTest("rm")
	if err == nil {
		t.Fatal("Execute() error = nil, want missing arg error")
	}
	if !strings.Contains(err.Error(), "requires at least 1 arg") {
		t.Fatalf("error = %v, want missing arg error", err)
	}
}

func TestFetchRequiresArgs(t *testing.T) {
	_, _, err := executeForTest("fetch")
	if err == nil {
		t.Fatal("Execute() error = nil, want missing arg error")
	}
}

func TestListEmpty(t *testing.T) {
	withHome(t)

	stdout, _, err := executeForTest("list")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "No sources cached yet." {
		t.Fatalf("stdout = %q, want empty cache message", stdout)
	}
}

func TestCleanEmpty(t *testing.T) {
	withHome(t)

	stdout, _, err := executeForTest("clean")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "Cleaned 0 source(s)" {
		t.Fatalf("stdout = %q, want zero clean summary", stdout)
	}
}

func TestPathEnsuresCachedAndPrintsOnlyAbsolutePath(t *testing.T) {
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"zod@3.22.4": {Path: filepath.Join(t.TempDir(), "zod")},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "path", "--cwd", "/tmp/project", "zod@3.22.4")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout != app.outcomes["zod@3.22.4"].Path+"\n" {
		t.Fatalf("stdout = %q, want only path", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if len(app.calls) != 1 {
		t.Fatalf("calls = %#v, want one call", app.calls)
	}
	if app.calls[0].spec != "zod@3.22.4" || app.calls[0].opts.CWD != "/tmp/project" || app.calls[0].opts.Verbose {
		t.Fatalf("call = %#v, want spec, cwd, non-verbose", app.calls[0])
	}
}

func TestPathSchedulesIndexAfterSuccessfulOutcome(t *testing.T) {
	outcome := source.Outcome{Path: filepath.Join(t.TempDir(), "zod")}
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"zod@3.22.4": outcome,
	}}
	indexer := &fakeIndexer{}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app, Indexer: indexer}, "path", "zod@3.22.4")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout != outcome.Path+"\n" {
		t.Fatalf("stdout = %q, want only path", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	outcomes := indexer.scheduledOutcomes()
	if len(outcomes) != 1 {
		t.Fatalf("scheduled outcomes = %#v, want one", outcomes)
	}
	if outcomes[0] != outcome {
		t.Fatalf("scheduled outcome = %#v, want %#v", outcomes[0], outcome)
	}
}

func TestPathDoesNotWaitForIndexerBeforeReturning(t *testing.T) {
	outcome := source.Outcome{Path: filepath.Join(t.TempDir(), "zod")}
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"zod@3.22.4": outcome,
	}}
	indexer := newBlockingWaitIndexer()

	result := assertCommandDoesNotWaitForIndexer(t, Options{App: app, Indexer: indexer}, indexer, "path", "zod@3.22.4")

	if result.err != nil {
		t.Fatalf("Execute() error = %v", result.err)
	}
	if indexer.waitedForIndexing() {
		t.Fatal("indexer Wait was called")
	}
}

func TestFetchEnsuresCachedAndSummarizesOutcomes(t *testing.T) {
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"zod@3.22.4":     {Name: "zod", Version: "3.22.4", SourceLabel: "npm", Path: "/cache/zod"},
		"left-pad@1.3.0": {Name: "left-pad", Version: "1.3.0", SourceLabel: "npm", Path: "/cache/left-pad", FromCache: true},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "fetch", "--cwd", "/tmp/project", "zod@3.22.4", "left-pad@1.3.0")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"Fetched zod@3.22.4 from npm",
		"Cached left-pad@1.3.0 from npm",
		"Fetched 1 source(s), 1 already cached",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if len(app.calls) != 2 {
		t.Fatalf("calls = %#v, want two calls", app.calls)
	}
	for _, call := range app.calls {
		if call.opts.CWD != "/tmp/project" || !call.opts.Verbose {
			t.Fatalf("call = %#v, want cwd and verbose", call)
		}
	}
}

func TestFetchQuietStillSchedulesIndex(t *testing.T) {
	outcome := source.Outcome{Name: "zod", Version: "3.22.4", SourceLabel: "npm", Path: "/cache/zod"}
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"zod@3.22.4": outcome,
	}}
	indexer := &fakeIndexer{}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app, Indexer: indexer}, "fetch", "--quiet", "zod@3.22.4")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	outcomes := indexer.scheduledOutcomes()
	if len(outcomes) != 1 {
		t.Fatalf("scheduled outcomes = %#v, want one", outcomes)
	}
	if outcomes[0] != outcome {
		t.Fatalf("scheduled outcome = %#v, want %#v", outcomes[0], outcome)
	}
}

func TestFetchDoesNotWaitForIndexerBeforeReturning(t *testing.T) {
	outcome := source.Outcome{Name: "zod", Version: "3.22.4", SourceLabel: "npm", Path: "/cache/zod"}
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"zod@3.22.4": outcome,
	}}
	indexer := newBlockingWaitIndexer()

	result := assertCommandDoesNotWaitForIndexer(t, Options{App: app, Indexer: indexer}, indexer, "fetch", "--quiet", "zod@3.22.4")

	if result.err != nil {
		t.Fatalf("Execute() error = %v", result.err)
	}
	if indexer.waitedForIndexing() {
		t.Fatal("indexer Wait was called")
	}
}

func TestFetchDisplaysMavenLabel(t *testing.T) {
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"maven:org.example:lib@1.0.0": {
			Name:        "org.example:lib",
			Version:     "1.0.0",
			SourceLabel: "Maven",
			Path:        "/cache/lib",
		},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "fetch", "maven:org.example:lib@1.0.0")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Fetched org.example:lib@1.0.0 from Maven") {
		t.Fatalf("stdout = %q, want Maven fetch line", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestFetchDisplaysNuGetLabel(t *testing.T) {
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"nuget:Newtonsoft.Json@13.0.3": {
			Name:        "Newtonsoft.Json",
			Version:     "13.0.3",
			SourceLabel: "NuGet",
			Path:        "/cache/Newtonsoft.Json",
		},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "fetch", "nuget:Newtonsoft.Json@13.0.3")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Fetched Newtonsoft.Json@13.0.3 from NuGet") {
		t.Fatalf("stdout = %q, want NuGet fetch line", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestScanFetchSchedulesIndexForSuccessfulCandidates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"19.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	outcome := source.Outcome{Name: "react", Version: "19.0.0", SourceLabel: "npm", Path: "/cache/react"}
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"react@19.0.0": outcome,
	}}
	indexer := &fakeIndexer{}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app, Indexer: indexer}, "scan", "--cwd", dir, "--fetch", "--no-imports")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "Fetched react@19.0.0 from npm") {
		t.Fatalf("stdout = %q, want fetch output", stdout)
	}
	outcomes := indexer.scheduledOutcomes()
	if len(outcomes) != 1 {
		t.Fatalf("scheduled outcomes = %#v, want one", outcomes)
	}
	if outcomes[0] != outcome {
		t.Fatalf("scheduled outcome = %#v, want %#v", outcomes[0], outcome)
	}
}

func TestScanFetchDoesNotWaitForIndexerBeforeReturning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"19.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	outcome := source.Outcome{Name: "react", Version: "19.0.0", SourceLabel: "npm", Path: "/cache/react"}
	app := &fakeApp{outcomes: map[string]source.Outcome{
		"react@19.0.0": outcome,
	}}
	indexer := newBlockingWaitIndexer()

	result := assertCommandDoesNotWaitForIndexer(t, Options{App: app, Indexer: indexer}, indexer, "scan", "--cwd", dir, "--fetch", "--no-imports")

	if result.err != nil {
		t.Fatalf("Execute() error = %v", result.err)
	}
	if indexer.waitedForIndexing() {
		t.Fatal("indexer Wait was called")
	}
}

func TestScanJSONPrintsProjectCandidates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"^19.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeForTest("scan", "--cwd", dir, "--json", "--no-imports")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var got struct {
		Candidates []struct {
			Spec       string   `json:"spec"`
			Ecosystem  string   `json:"ecosystem"`
			Confidence int      `json:"confidence"`
			Reasons    []string `json:"reasons"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(got.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want one candidate", got.Candidates)
	}
	if got.Candidates[0].Spec != "react@19.0.0" || got.Candidates[0].Ecosystem != "npm" {
		t.Fatalf("candidate = %#v, want react npm", got.Candidates[0])
	}
}

func TestSearchPrintsHumanReadableResults(t *testing.T) {
	app := &fakeApp{searchResults: []codegraph.SearchResult{{
		Source:    "zod@3.22.4",
		Kind:      codegraph.NodeKindFunction,
		Name:      "parse",
		Language:  codegraph.LanguageTypeScript,
		Path:      "src/index.ts",
		StartLine: 12,
		EndLine:   20,
		Calls:     []string{"safeParse"},
	}}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "search", "zod@3.22.4", "kind:function name:parse")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"zod@3.22.4",
		"function parse src/index.ts:12",
		"calls: safeParse",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if app.searchSpec != "zod@3.22.4" || app.searchQuery != "kind:function name:parse" {
		t.Fatalf("search call = %q %q %#v", app.searchSpec, app.searchQuery, app.searchOpts)
	}
	if !app.searchOpts.SyncIndex {
		t.Fatalf("search options = %#v, want SyncIndex true", app.searchOpts)
	}
}

func TestSearchJSONPrintsResults(t *testing.T) {
	app := &fakeApp{searchResults: []codegraph.SearchResult{{
		Source:    "demo",
		Kind:      codegraph.NodeKindFunction,
		Name:      "Run",
		Path:      "main.go",
		StartLine: 1,
	}}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "search", "--json", "demo", "Run")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var got []codegraph.SearchResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(got) != 1 || got[0].Name != "Run" || got[0].Path != "main.go" {
		t.Fatalf("results = %#v, want Run main.go", got)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Fatalf("stdout = %q, want trailing newline", stdout)
	}
}

func TestSearchPassesOptionsAndFilterFlags(t *testing.T) {
	app := &fakeApp{}

	stdout, stderr, err := executeForTestWithOptions(
		Options{App: app},
		"search",
		"--cwd", "/tmp/project",
		"--limit", "7",
		"--kind", "function",
		"--lang", "go",
		"--path", "internal/",
		"--calls", "helper",
		"--no-sync-index",
		"demo",
		"parse",
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "No code graph results found." {
		t.Fatalf("stdout = %q, want no-results message", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if app.searchSpec != "demo" {
		t.Fatalf("search spec = %q, want demo", app.searchSpec)
	}
	for _, want := range []string{"parse", "kind:function", "lang:go", "path:internal/", "calls:helper"} {
		if !strings.Contains(app.searchQuery, want) {
			t.Fatalf("search query = %q, want %q", app.searchQuery, want)
		}
	}
	if app.searchOpts.CWD != "/tmp/project" || app.searchOpts.Limit != 7 || app.searchOpts.SyncIndex {
		t.Fatalf("search options = %#v, want cwd, limit, no sync", app.searchOpts)
	}
}

func TestGraphStatusPrintsHumanReadableSummary(t *testing.T) {
	indexedAt := time.Date(2026, 5, 20, 12, 31, 44, 0, time.UTC)
	app := &fakeApp{statusResult: codegraph.GraphInspectStatus{
		Source:        "demo@v1",
		SourcePath:    "/cache/demo",
		GraphPath:     "/cache/demo/.repobridge-graph",
		Status:        "ready",
		SchemaVersion: codegraph.SchemaVersion,
		IndexedAt:     indexedAt,
		Counts: codegraph.GraphCounts{
			Files:      2,
			Nodes:      5,
			Edges:      3,
			Warnings:   1,
			Unresolved: 4,
		},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "status", "--cwd", "/tmp/project", "demo@v1")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"demo@v1",
		"graph: /cache/demo/.repobridge-graph",
		"status: ready",
		"files: 2",
		"nodes: 5",
		"edges: 3",
		"warnings: 1",
		"indexedAt: 2026-05-20T12:31:44Z",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if app.statusSpec != "demo@v1" || app.statusOpts.CWD != "/tmp/project" || !app.statusOpts.SyncIndex {
		t.Fatalf("status call = %q %#v, want spec, cwd and sync", app.statusSpec, app.statusOpts)
	}
}

func TestGraphStatusJSONPrintsSummary(t *testing.T) {
	app := &fakeApp{statusResult: codegraph.GraphInspectStatus{
		Source: "demo@v1",
		Status: "missing",
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "status", "--json", "--no-sync-index", "demo@v1")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var got codegraph.GraphInspectStatus
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if got.Source != "demo@v1" || got.Status != "missing" {
		t.Fatalf("status = %#v, want demo missing", got)
	}
	if app.statusOpts.SyncIndex {
		t.Fatalf("status options = %#v, want no sync", app.statusOpts)
	}
}

func TestGraphFilesPrintsFilteredFiles(t *testing.T) {
	app := &fakeApp{filesResult: codegraph.GraphFilesResult{
		Source: "demo@v1",
		Files: []codegraph.GraphFile{{
			Path:      "src/main.go",
			Language:  codegraph.LanguageGo,
			NodeCount: 4,
			Size:      128,
		}},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "files", "--path", "src", "--limit", "5", "demo@v1")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"demo@v1",
		"src/main.go go nodes:4 bytes:128",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if app.filesSpec != "demo@v1" || app.filesOpts.PathFilter != "src" || app.filesOpts.Limit != 5 {
		t.Fatalf("files call = %q %#v, want filter and limit", app.filesSpec, app.filesOpts)
	}
}

func TestGraphNodePrintsDetailsAndSourceSnippet(t *testing.T) {
	app := &fakeApp{nodeResult: codegraph.GraphNodeLookupResult{
		Source: "demo@v1",
		Node: &codegraph.GraphNodeDetail{
			ID:            "n1",
			Kind:          codegraph.NodeKindFunction,
			Name:          "Run",
			QualifiedName: "main.Run",
			Language:      codegraph.LanguageGo,
			Path:          "main.go",
			StartLine:     3,
			EndLine:       5,
			Signature:     "func Run()",
			Calls:         []string{"helper"},
			Source: []codegraph.SourceLine{
				{Line: 3, Text: "func Run() {"},
				{Line: 4, Text: "\thelper()"},
			},
		},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "node", "--source-lines", "2", "demo@v1", "main.Run")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"demo@v1",
		"function Run main.go:3",
		"qualified: main.Run",
		"signature: func Run()",
		"calls: helper",
		"3: func Run() {",
		"4: \thelper()",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if app.nodeSpec != "demo@v1" || app.nodeLookup != "main.Run" || app.nodeOpts.SourceLines != 2 {
		t.Fatalf("node call = %q %q %#v, want source-lines", app.nodeSpec, app.nodeLookup, app.nodeOpts)
	}
}

func TestGraphNodePrintsAmbiguousMatches(t *testing.T) {
	app := &fakeApp{nodeResult: codegraph.GraphNodeLookupResult{
		Source: "demo@v1",
		Matches: []codegraph.GraphNodeDetail{
			{ID: "n1", Kind: codegraph.NodeKindFunction, Name: "Run", QualifiedName: "main.Run", Path: "main.go", StartLine: 3},
			{ID: "n2", Kind: codegraph.NodeKindMethod, Name: "Run", QualifiedName: "worker.Run", Path: "worker.go", StartLine: 8},
		},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "node", "demo@v1", "Run")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"Multiple code graph nodes matched Run",
		"n1 function main.Run main.go:3",
		"n2 method worker.Run worker.go:8",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestCallersPrintsHumanReadableEdges(t *testing.T) {
	app := &fakeApp{callgraphResult: codegraph.CallgraphResult{
		Source:    "demo@v1",
		Direction: codegraph.CallgraphDirectionCallers,
		Symbol:    "login",
		Root: &codegraph.GraphNodeDetail{
			ID: "target", Kind: codegraph.NodeKindFunction, Name: "login", QualifiedName: "auth.login", Path: "auth.go", StartLine: 12,
		},
		Edges: []codegraph.CallgraphEdge{{
			Depth: 1,
			From:  codegraph.GraphNodeDetail{ID: "caller", Kind: codegraph.NodeKindMethod, Name: "postLogin", QualifiedName: "AuthController.postLogin", Path: "controller.kt", StartLine: 31},
			To:    codegraph.GraphNodeDetail{ID: "target", Kind: codegraph.NodeKindFunction, Name: "login", QualifiedName: "auth.login", Path: "auth.go", StartLine: 12},
			Kind:  codegraph.EdgeKindCalls,
			Line:  38,
		}},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "callers", "--depth", "2", "--limit", "5", "--kind", "method", "--lang", "kotlin", "--path", "controller", "demo@v1", "login")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"callers of login",
		"method AuthController.postLogin controller.kt:31",
		"calls auth.login at line 38",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if app.callgraphSpec != "demo@v1" || app.callgraphSymbol != "login" {
		t.Fatalf("callgraph call = %q %q, want spec and symbol", app.callgraphSpec, app.callgraphSymbol)
	}
	if app.callgraphOpts.Direction != codegraph.CallgraphDirectionCallers || app.callgraphOpts.Depth != 2 || app.callgraphOpts.Limit != 5 {
		t.Fatalf("callgraph opts = %#v, want callers depth limit", app.callgraphOpts)
	}
	if len(app.callgraphOpts.Kinds) != 1 || app.callgraphOpts.Kinds[0] != codegraph.NodeKindMethod {
		t.Fatalf("kinds = %#v, want method", app.callgraphOpts.Kinds)
	}
	if len(app.callgraphOpts.Languages) != 1 || app.callgraphOpts.Languages[0] != codegraph.LanguageKotlin {
		t.Fatalf("languages = %#v, want kotlin", app.callgraphOpts.Languages)
	}
	if len(app.callgraphOpts.PathFilters) != 1 || app.callgraphOpts.PathFilters[0] != "controller" {
		t.Fatalf("paths = %#v, want controller", app.callgraphOpts.PathFilters)
	}
}

func TestCalleesJSONPrintsEdges(t *testing.T) {
	app := &fakeApp{callgraphResult: codegraph.CallgraphResult{
		Source:    "demo@v1",
		Direction: codegraph.CallgraphDirectionCallees,
		Symbol:    "Run",
		Edges: []codegraph.CallgraphEdge{{
			Depth: 1,
			From:  codegraph.GraphNodeDetail{ID: "run", Name: "Run"},
			To:    codegraph.GraphNodeDetail{ID: "helper", Name: "helper"},
			Kind:  codegraph.EdgeKindCalls,
			Line:  4,
		}},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "callees", "--json", "--include-unresolved", "--no-sync-index", "demo@v1", "Run")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var got codegraph.CallgraphResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if got.Direction != codegraph.CallgraphDirectionCallees || len(got.Edges) != 1 || got.Edges[0].To.Name != "helper" {
		t.Fatalf("result = %#v, want callee helper", got)
	}
	if app.callgraphOpts.SyncIndex || !app.callgraphOpts.IncludeUnresolved {
		t.Fatalf("callgraph opts = %#v, want no sync and unresolved", app.callgraphOpts)
	}
}

func TestImpactPrintsNoEdgesMessage(t *testing.T) {
	app := &fakeApp{callgraphResult: codegraph.CallgraphResult{
		Source:    "demo@v1",
		Direction: codegraph.CallgraphDirectionImpact,
		Symbol:    "Config",
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "impact", "demo@v1", "Config")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "No call graph results found.") {
		t.Fatalf("stdout = %q, want no results message", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if app.callgraphOpts.Direction != codegraph.CallgraphDirectionImpact {
		t.Fatalf("direction = %q, want impact", app.callgraphOpts.Direction)
	}
}

func TestContextPrintsHumanReadableResult(t *testing.T) {
	app := &fakeApp{contextResult: codegraph.ContextResult{
		Source: "demo@v1",
		Mode:   codegraph.ContextModeContext,
		Query:  "auth login flow",
		Budget: codegraph.ContextBudget{Name: "small", SearchLimit: 5, SnippetCount: 3, SourceLines: 8, Depth: 1},
		EntryPoints: []codegraph.GraphNodeDetail{{
			ID: "login", Kind: codegraph.NodeKindFunction, Name: "Login", QualifiedName: "auth.Login", Path: "auth.go", StartLine: 3,
		}},
		Relationships: []codegraph.CallgraphEdge{{
			Depth: 1,
			From:  codegraph.GraphNodeDetail{ID: "login", Name: "Login", QualifiedName: "auth.Login"},
			To:    codegraph.GraphNodeDetail{ID: "session", Name: "createSession", QualifiedName: "auth.createSession"},
			Kind:  codegraph.EdgeKindCalls,
			Line:  4,
		}},
		Snippets: []codegraph.ContextSnippet{{
			Path:      "auth.go",
			StartLine: 3,
			EndLine:   4,
			Lines: []codegraph.SourceLine{
				{Line: 3, Text: "func Login() {"},
				{Line: 4, Text: "\tcreateSession()"},
			},
		}},
		RelatedFiles: []codegraph.GraphFile{{Path: "auth.go", Language: codegraph.LanguageGo, NodeCount: 2}},
		Stats:        codegraph.ContextResultStats{EntryPoints: 1, Relationships: 1, Snippets: 1, RelatedFiles: 1},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "context", "--budget", "small", "--limit", "7", "--depth", "2", "demo@v1", "auth login flow")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"context: auth login flow",
		"source: demo@v1",
		"budget: small",
		"Entry points",
		"function auth.Login auth.go:3",
		"Relationships",
		"auth.Login -> auth.createSession",
		"Snippets",
		"auth.go:3-4",
		"3: func Login() {",
		"Related files",
		"auth.go go nodes:2",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if app.contextSpec != "demo@v1" || app.contextQuery != "auth login flow" {
		t.Fatalf("context call = %q %q, want spec/query", app.contextSpec, app.contextQuery)
	}
	if app.contextOpts.Mode != codegraph.ContextModeContext || app.contextOpts.Budget != "small" || app.contextOpts.Limit != 7 || app.contextOpts.Depth != 2 {
		t.Fatalf("context opts = %#v, want context small limit depth", app.contextOpts)
	}
}

func TestExploreJSONPrintsResult(t *testing.T) {
	app := &fakeApp{contextResult: codegraph.ContextResult{
		Source: "demo@v1",
		Mode:   codegraph.ContextModeExplore,
		Query:  "AuthService.login",
		Budget: codegraph.ContextBudget{Name: "large", SearchLimit: 20, SnippetCount: 8, SourceLines: 28, Depth: 2},
		EntryPoints: []codegraph.GraphNodeDetail{{
			ID: "login", Kind: codegraph.NodeKindMethod, Name: "login", QualifiedName: "AuthService.login", Path: "AuthService.kt", StartLine: 42,
		}},
	}}

	stdout, stderr, err := executeForTestWithOptions(Options{App: app}, "explore", "--json", "--budget", "large", "--no-sync-index", "demo@v1", "AuthService.login")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var got codegraph.ContextResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if got.Mode != codegraph.ContextModeExplore || len(got.EntryPoints) != 1 || got.EntryPoints[0].QualifiedName != "AuthService.login" {
		t.Fatalf("result = %#v, want explore AuthService.login", got)
	}
	if app.contextOpts.SyncIndex || app.contextOpts.Mode != codegraph.ContextModeExplore || app.contextOpts.Budget != "large" {
		t.Fatalf("context opts = %#v, want no-sync explore large", app.contextOpts)
	}
}

func TestListJSONUsesCacheIndex(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{{
		Name: "zod", Version: "3.22.4", Registry: "npm",
		Path: "repos/github.com/colinhacks/zod/3.22.4", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	repos := []cache.RepoEntry{{
		Name: "github.com/owner/repo", Version: "main",
		Path: "repos/github.com/owner/repo/main", FetchedAt: "2026-05-18T12:00:00Z",
	}}
	if err := cache.WriteSources(packages, repos); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("list", "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got cache.SourcesIndex
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(got.Packages) != 1 || got.Packages[0].Name != "zod" {
		t.Fatalf("packages = %#v", got.Packages)
	}
	if len(got.Repos) != 1 || got.Repos[0].Name != "github.com/owner/repo" {
		t.Fatalf("repos = %#v", got.Repos)
	}
}

func TestRemovePackageUsesCacheHelper(t *testing.T) {
	withHome(t)
	if err := cache.WriteSources([]cache.PackageEntry{{
		Name: "zod", Version: "3.22.4", Registry: "npm",
		Path: "repos/github.com/colinhacks/zod/3.22.4", FetchedAt: "2026-05-18T12:00:00Z",
	}}, nil); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("remove", "zod")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Removed zod from npm") {
		t.Fatalf("stdout = %q, want removed package message", stdout)
	}
	info, err := cache.PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Fatalf("PackageInfo() = %#v, want nil", info)
	}
}

func TestRemoveVersionedPackageKeepsOtherVersions(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{
		{Name: "zod", Version: "3.22.4", Registry: "npm", Path: "repos/github.com/colinhacks/zod/3.22.4"},
		{Name: "zod", Version: "4.0.0", Registry: "npm", Path: "repos/github.com/colinhacks/zod/4.0.0"},
	}
	if err := cache.WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("remove", "zod@3.22.4")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Removed zod@3.22.4 from npm") {
		t.Fatalf("stdout = %q, want versioned removed package message", stdout)
	}
	index, err := cache.ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 1 || index.Packages[0].Version != "4.0.0" {
		t.Fatalf("packages = %#v, want only zod 4.0.0", index.Packages)
	}
}

func TestCleanRejectsReposWithRegistryFilter(t *testing.T) {
	withHome(t)

	_, _, err := executeForTest("clean", "--repos", "--npm")
	if err == nil {
		t.Fatal("Execute() error = nil, want conflicting flags error")
	}
	if !strings.Contains(err.Error(), "--repos cannot be combined with registry filters") {
		t.Fatalf("error = %v, want conflicting flags error", err)
	}
}

func TestSubcommandsUseActiveCobraWriters(t *testing.T) {
	withHome(t)
	cmd := NewRootCommand(Options{Version: "test-version"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "No sources cached yet." {
		t.Fatalf("stdout = %q, want subcommand output in command writer", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestInstallAgentPrintConfigDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, err := executeForTest("install-agent", "--target", "codex", "--home", home, "--print-config", "--version", "v9.9.9")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{"target: codex", "file: ", "SKILL.md", "v9.9.9"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "repobridge", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("print-config wrote destination, stat err = %v", err)
	}
}

func TestInstallAgentDryRunReportsActions(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, err := executeForTest("install-agent", "--target", "codex", "--home", home, "--dry-run")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "create") || !strings.Contains(stdout, filepath.Join(".agents", "skills", "repobridge", "SKILL.md")) {
		t.Fatalf("stdout = %q, want dry-run create action", stdout)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "repobridge", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote destination, stat err = %v", err)
	}
}

func TestInstallAgentWritesBundledSkill(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, err := executeForTest("install-agent", "--target", "codex", "--home", home, "--version", "v1.2.3")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "Installed 2 file(s)") {
		t.Fatalf("stdout = %q, want installed summary", stdout)
	}
	content, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "repobridge", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "RepoBridge Project Context") || !strings.Contains(string(content), "v1.2.3") {
		t.Fatalf("installed skill = %q, want skill content and version", string(content))
	}
}

func TestCleanRegistryFilter(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{
		{Name: "zod", Version: "3.22.4", Registry: "npm", Path: "repos/github.com/colinhacks/zod/3.22.4"},
		{Name: "requests", Version: "2.31.0", Registry: "pypi", Path: "repos/github.com/psf/requests/v2.31.0"},
	}
	repos := []cache.RepoEntry{{
		Name: "github.com/owner/repo", Version: "main", Path: "repos/github.com/owner/repo/main",
	}}
	if err := cache.WriteSources(packages, repos); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("clean", "--npm")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "Cleaned 1 source(s)" {
		t.Fatalf("stdout = %q, want one-source clean summary", stdout)
	}
	npmInfo, err := cache.PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if npmInfo != nil {
		t.Fatalf("npm package = %#v, want nil", npmInfo)
	}
	pypiInfo, err := cache.PackageInfo("requests", "pypi")
	if err != nil {
		t.Fatal(err)
	}
	if pypiInfo == nil {
		t.Fatal("pypi package missing after npm clean")
	}
	repoInfo, err := cache.RepoInfo("github.com/owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if repoInfo == nil {
		t.Fatal("repo missing after npm clean")
	}
}

func TestCleanMavenRegistryFilter(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{
		{Name: "zod", Version: "3.22.4", Registry: "npm", Path: "repos/github.com/colinhacks/zod/3.22.4"},
		{Name: "org.example:lib", Version: "1.0.0", Registry: "maven", Path: "repos/maven/org.example/lib/1.0.0"},
	}
	if err := cache.WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("clean", "--maven")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "Cleaned 1 source(s)" {
		t.Fatalf("stdout = %q, want one-source clean summary", stdout)
	}
	mavenInfo, err := cache.PackageInfo("org.example:lib", "maven")
	if err != nil {
		t.Fatal(err)
	}
	if mavenInfo != nil {
		t.Fatalf("maven package = %#v, want nil", mavenInfo)
	}
	npmInfo, err := cache.PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if npmInfo == nil {
		t.Fatal("npm package missing after Maven clean")
	}
}

func TestCleanNuGetRegistryFilter(t *testing.T) {
	withHome(t)
	packages := []cache.PackageEntry{
		{Name: "zod", Version: "3.22.4", Registry: "npm", Path: "repos/github.com/colinhacks/zod/3.22.4"},
		{Name: "Newtonsoft.Json", Version: "13.0.3", Registry: "nuget", Path: "repos/github.com/JamesNK/Newtonsoft.Json/13.0.3"},
	}
	if err := cache.WriteSources(packages, nil); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeForTest("clean", "--nuget")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(stdout) != "Cleaned 1 source(s)" {
		t.Fatalf("stdout = %q, want one-source clean summary", stdout)
	}
	nugetInfo, err := cache.PackageInfo("Newtonsoft.Json", "nuget")
	if err != nil {
		t.Fatal(err)
	}
	if nugetInfo != nil {
		t.Fatalf("nuget package = %#v, want nil", nugetInfo)
	}
	npmInfo, err := cache.PackageInfo("zod", "npm")
	if err != nil {
		t.Fatal(err)
	}
	if npmInfo == nil {
		t.Fatal("npm package missing after NuGet clean")
	}
}
