//go:build e2e

package e2esearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"repobridge/internal/cache"
	"repobridge/internal/codegraph"
)

const (
	kotlinComposeDisplayName = "github.com/meltwater/kotlin-compose"
	kotlinComposeCommit      = "953e574e6fb15018ba3be02afdc6cbfe068601d8"
	kotlinComposeRepo        = "https://github.com/meltwater/kotlin-compose.git"
	kotlinComposeSpec        = kotlinComposeDisplayName + "@" + kotlinComposeCommit
)

func TestSearchKotlinRepoE2E(t *testing.T) {
	env := setupSearchE2E(t)

	t.Logf("using repobridge binary: %s", env.binary)
	t.Logf("using REPOBRIDGE_HOME: %s", env.home)
	t.Logf("searching pinned Kotlin repo: %s", kotlinComposeSpec)

	results := runSearchJSON(t, env.binary, env.home, kotlinComposeSpec, `lang:kotlin kind:function name:up calls:exec path:DockerCompose.kt`)
	assertKotlinUpResult(t, results)

	secondResults := runSearchJSON(t, env.binary, env.home, kotlinComposeSpec, `lang:kotlin kind:function name:up path:DockerCompose.kt`)
	assertKotlinUpResult(t, secondResults)
}

func TestGraphCommandsAfterSearchE2E(t *testing.T) {
	env := setupSearchE2E(t)

	searchResults := runSearchJSON(t, env.binary, env.home, kotlinComposeSpec, `lang:kotlin kind:function name:up calls:exec path:DockerCompose.kt`)
	upResult := findKotlinUpResult(t, searchResults)

	var status codegraph.GraphInspectStatus
	runRepoBridgeJSON(t, env, &status, "status", "--json", "--no-sync-index", kotlinComposeSpec)
	if status.Source != kotlinComposeSpec || status.Status != "ready" {
		t.Fatalf("status = %#v, want ready status for %s", status, kotlinComposeSpec)
	}
	if status.SourcePath != env.sourceDir {
		t.Fatalf("status source path = %q, want %q", status.SourcePath, env.sourceDir)
	}
	if status.Counts.Files == 0 || status.Counts.Nodes == 0 {
		t.Fatalf("status counts = %#v, want indexed files and nodes", status.Counts)
	}
	t.Logf("verified status: %s files=%d nodes=%d edges=%d", status.Status, status.Counts.Files, status.Counts.Nodes, status.Counts.Edges)

	var files codegraph.GraphFilesResult
	runRepoBridgeJSON(t, env, &files, "files", "--json", "--no-sync-index", "--path", "DockerCompose.kt", "--limit", "5", kotlinComposeSpec)
	assertKotlinDockerComposeFile(t, files)

	var node codegraph.GraphNodeLookupResult
	runRepoBridgeJSON(t, env, &node, "node", "--json", "--no-sync-index", "--source-lines", "6", kotlinComposeSpec, upResult.ID)
	if node.Node == nil {
		t.Fatalf("node lookup returned no node: %#v", node)
	}
	if node.Node.ID != upResult.ID || node.Node.Name != "up" || node.Node.Kind != codegraph.NodeKindFunction {
		t.Fatalf("node = %#v, want Kotlin up function with id %s", node.Node, upResult.ID)
	}
	if !sourceLinesContain(node.Node.Source, "fun up") {
		t.Fatalf("node source = %#v, want fun up source line", node.Node.Source)
	}
	t.Logf("verified node: %s %s %s:%d", node.Node.Kind, node.Node.Name, node.Node.Path, node.Node.StartLine)

	var callers codegraph.CallgraphResult
	runRepoBridgeJSON(t, env, &callers, "callers", "--json", "--no-sync-index", "--include-unresolved", "--depth", "1", "--limit", "10", kotlinComposeSpec, "ps")
	if callers.Direction != codegraph.CallgraphDirectionCallers || callers.Root == nil || callers.Root.Name != "ps" {
		t.Fatalf("callers result = %#v, want callers of ps", callers)
	}
	if !callgraphHasCaller(callers.Edges, "up", "ps") {
		t.Fatalf("callers edges = %#v, want up calling ps", callers.Edges)
	}
	t.Logf("verified callers: %s has %d caller edge(s)", callers.Symbol, len(callers.Edges))

	var contextResult codegraph.ContextResult
	runRepoBridgeJSON(t, env, &contextResult, "context", "--json", "--no-sync-index", "--budget", "small", "--limit", "3", kotlinComposeSpec, "up exec ps")
	assertContextLikeResult(t, contextResult, codegraph.ContextModeContext, "up exec ps")

	var exploreResult codegraph.ContextResult
	runRepoBridgeJSON(t, env, &exploreResult, "explore", "--json", "--no-sync-index", "--budget", "small", "--limit", "3", kotlinComposeSpec, "docker compose up")
	assertContextLikeResult(t, exploreResult, codegraph.ContextModeExplore, "docker compose up")
}

func TestSearchOutputBudgetReportE2E(t *testing.T) {
	env := setupSearchE2E(t)
	tasks := []searchBudgetTask{
		{
			Name:        "Find Kotlin up function",
			SearchQuery: `lang:kotlin kind:function name:up path:DockerCompose.kt`,
			RGPattern:   `fun up`,
		},
		{
			Name:        "Find functions calling exec",
			SearchQuery: `lang:kotlin kind:function calls:exec path:DockerCompose.kt`,
			RGPattern:   `exec\(`,
		},
		{
			Name:        "Find functions calling ps",
			SearchQuery: `lang:kotlin kind:function calls:ps path:DockerCompose.kt`,
			RGPattern:   `ps\(`,
		},
		{
			Name:        "Find docker compose resolver",
			SearchQuery: `lang:kotlin kind:function name:findDockerCompose path:DockerCompose.kt`,
			RGPattern:   `findDockerCompose`,
		},
	}

	report := newSearchBudgetReport(env)
	for _, task := range tasks {
		searchOutput := runSearchRaw(t, env.binary, env.home, kotlinComposeSpec, task.SearchQuery)
		var results []codegraph.SearchResult
		if err := json.Unmarshal(searchOutput, &results); err != nil {
			t.Fatalf("could not parse search JSON for %s: %v\n%s", task.Name, err, string(searchOutput))
		}
		if len(results) == 0 {
			t.Fatalf("search task %q returned no results", task.Name)
		}

		rgOutput := runRG(t, env.sourceDir, task.RGPattern)
		row := searchBudgetRow{
			Task:         task,
			SearchMetric: measureOutput(searchOutput),
			RGMetric:     measureOutput(rgOutput),
			ResultCount:  len(results),
		}
		if row.SearchMetric.EstimatedTokens >= row.RGMetric.EstimatedTokens {
			t.Fatalf("%s did not reduce estimated tokens: search=%d rg=%d", task.Name, row.SearchMetric.EstimatedTokens, row.RGMetric.EstimatedTokens)
		}
		report.Rows = append(report.Rows, row)
	}

	reportPath := filepath.Join(env.workspace, "results", "search-output-budget.md")
	writeSearchBudgetReport(t, reportPath, report)
	t.Logf("search output budget report written to: %s", reportPath)
}

type searchE2EEnv struct {
	root      string
	workspace string
	binary    string
	home      string
	sourceDir string
}

func setupSearchE2E(t *testing.T) searchE2EEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping e2e search test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required for e2e search test: %v", err)
	}
	if _, err := exec.LookPath("rg"); err != nil {
		t.Fatalf("rg is required for e2e search budget test: %v", err)
	}

	root := repoRoot(t)
	workspace := os.Getenv("REPOBRIDGE_E2E_SEARCH_DIR")
	if workspace == "" {
		workspace = filepath.Join(root, "e2e", "search")
	}
	if os.Getenv("REPOBRIDGE_E2E_REFRESH") == "1" {
		if err := os.RemoveAll(workspace); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(workspace, "results"), 0o755); err != nil {
		t.Fatal(err)
	}

	binary := buildRepoBridge(t, root, workspace)
	home := filepath.Join(workspace, "home")
	t.Setenv("REPOBRIDGE_HOME", home)
	sourceDir := checkoutCachedRepo(t, home)
	return searchE2EEnv{root: root, workspace: workspace, binary: binary, home: home, sourceDir: sourceDir}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func buildRepoBridge(t *testing.T, root, workspace string) string {
	t.Helper()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(binDir, "repobridge")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	runCommand(t, root, nil, 2*time.Minute, "go", "build", "-o", binary, "./cmd/repobridge")
	return binary
}

func checkoutCachedRepo(t *testing.T, home string) string {
	t.Helper()
	relativePath := cache.RepoRelativePath(kotlinComposeDisplayName, kotlinComposeCommit)
	repoDir := filepath.Join(home, filepath.FromSlash(relativePath))
	if currentCommit(repoDir) != kotlinComposeCommit {
		if err := os.RemoveAll(repoDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			t.Fatal(err)
		}
		runCommand(t, repoDir, nil, time.Minute, "git", "init")
		runCommand(t, repoDir, nil, time.Minute, "git", "remote", "add", "origin", kotlinComposeRepo)
		runCommand(t, repoDir, nil, time.Minute, "git", "config", "core.sparseCheckout", "true")
		writeSparseCheckout(t, repoDir, []string{
			"src/main/kotlin/com/meltwater/docker/compose/DockerCompose.kt",
		})
		runCommand(t, repoDir, nil, 3*time.Minute, "git", "fetch", "--depth", "1", "origin", kotlinComposeCommit)
		runCommand(t, repoDir, nil, time.Minute, "git", "checkout", "--detach", "FETCH_HEAD")
	}
	if err := cache.WriteSources(nil, []cache.RepoEntry{{
		Name:    kotlinComposeDisplayName,
		Version: kotlinComposeCommit,
		Path:    relativePath,
	}}); err != nil {
		t.Fatal(err)
	}
	return repoDir
}

func currentCommit(repoDir string) string {
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		return ""
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func writeSparseCheckout(t *testing.T, repoDir string, paths []string) {
	t.Helper()
	patterns := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(filepath.ToSlash(path))
		if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
			t.Fatalf("invalid sparse checkout path: %q", path)
		}
		patterns = append(patterns, "/"+path)
	}
	sort.Strings(patterns)
	infoDir := filepath.Join(repoDir, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(infoDir, "sparse-checkout"), []byte(strings.Join(patterns, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runSearchJSON(t *testing.T, binary, home, spec, query string) []codegraph.SearchResult {
	t.Helper()
	output := runSearchRaw(t, binary, home, spec, query)
	var results []codegraph.SearchResult
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("could not parse search JSON: %v\n%s", err, string(output))
	}
	t.Logf("search query %q returned %d result(s)", query, len(results))
	return results
}

func runSearchRaw(t *testing.T, binary, home, spec, query string) []byte {
	t.Helper()
	return runCommand(t, filepath.Dir(binary), []string{"REPOBRIDGE_HOME=" + home}, 4*time.Minute, binary, "search", "--json", "--limit", "20", spec, query)
}

func runRepoBridgeJSON(t *testing.T, env searchE2EEnv, target any, args ...string) {
	t.Helper()
	t.Logf("running repobridge %s", strings.Join(args, " "))
	output := runCommand(t, filepath.Dir(env.binary), []string{"REPOBRIDGE_HOME=" + env.home}, 4*time.Minute, env.binary, args...)
	t.Logf("repobridge %s returned %d bytes", args[0], len(output))
	if err := json.Unmarshal(output, target); err != nil {
		t.Fatalf("could not parse JSON for repobridge %s: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}

func runRG(t *testing.T, sourceDir, pattern string) []byte {
	t.Helper()
	return runCommand(t, sourceDir, nil, time.Minute, "rg", "-n", "-C", "3", "--no-heading", "--color", "never", pattern, sourceDir)
}

func assertKotlinUpResult(t *testing.T, results []codegraph.SearchResult) {
	t.Helper()
	_ = findKotlinUpResult(t, results)
}

func findKotlinUpResult(t *testing.T, results []codegraph.SearchResult) codegraph.SearchResult {
	t.Helper()
	for _, result := range results {
		if result.Language != codegraph.LanguageKotlin {
			continue
		}
		if result.Kind != codegraph.NodeKindFunction || result.Name != "up" {
			continue
		}
		if !strings.HasSuffix(filepath.ToSlash(result.Path), "DockerCompose.kt") {
			t.Fatalf("result path = %q, want DockerCompose.kt", result.Path)
		}
		if !contains(result.Calls, "exec") {
			t.Fatalf("result calls = %#v, want exec", result.Calls)
		}
		t.Logf("verified Kotlin search result: %s %s %s:%d calls=%s", result.Kind, result.Name, result.Path, result.StartLine, strings.Join(result.Calls, ", "))
		return result
	}
	t.Fatalf("Kotlin function up with exec call not found in %#v", results)
	return codegraph.SearchResult{}
}

func assertKotlinDockerComposeFile(t *testing.T, result codegraph.GraphFilesResult) {
	t.Helper()
	for _, file := range result.Files {
		if file.Language != codegraph.LanguageKotlin {
			continue
		}
		if !strings.HasSuffix(filepath.ToSlash(file.Path), "DockerCompose.kt") {
			continue
		}
		if file.NodeCount == 0 || file.Size == 0 {
			t.Fatalf("file = %#v, want indexed nodes and size", file)
		}
		t.Logf("verified file: %s language=%s nodes=%d bytes=%d", file.Path, file.Language, file.NodeCount, file.Size)
		return
	}
	t.Fatalf("DockerCompose.kt Kotlin file not found in %#v", result.Files)
}

func sourceLinesContain(lines []codegraph.SourceLine, want string) bool {
	for _, line := range lines {
		if strings.Contains(line.Text, want) {
			return true
		}
	}
	return false
}

func callgraphHasCaller(edges []codegraph.CallgraphEdge, callerName, referenceName string) bool {
	for _, edge := range edges {
		if edge.From.Name == callerName && edge.ReferenceName == referenceName {
			return true
		}
	}
	return false
}

func assertContextLikeResult(t *testing.T, result codegraph.ContextResult, mode codegraph.ContextMode, query string) {
	t.Helper()
	if result.Source != kotlinComposeSpec || result.Mode != mode || result.Query != query {
		t.Fatalf("context result = %#v, want %s result for %q", result, mode, query)
	}
	if result.Budget.Name != "small" {
		t.Fatalf("budget = %#v, want small", result.Budget)
	}
	if result.Stats.EntryPoints == 0 || result.Stats.Snippets == 0 || len(result.RelatedFiles) == 0 {
		t.Fatalf("context stats = %#v relatedFiles=%#v, want entry points, snippets, and related files", result.Stats, result.RelatedFiles)
	}
	if !contextHasDockerComposeSnippet(result.Snippets) {
		t.Fatalf("snippets = %#v, want DockerCompose.kt snippet", result.Snippets)
	}
	t.Logf("verified %s: entryPoints=%d relationships=%d snippets=%d relatedFiles=%d",
		mode,
		result.Stats.EntryPoints,
		result.Stats.Relationships,
		result.Stats.Snippets,
		result.Stats.RelatedFiles,
	)
}

func contextHasDockerComposeSnippet(snippets []codegraph.ContextSnippet) bool {
	for _, snippet := range snippets {
		if strings.HasSuffix(filepath.ToSlash(snippet.Path), "DockerCompose.kt") && len(snippet.Lines) > 0 {
			return true
		}
	}
	return false
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type searchBudgetTask struct {
	Name        string
	SearchQuery string
	RGPattern   string
}

type outputMetric struct {
	Bytes           int
	Lines           int
	EstimatedTokens int
}

type searchBudgetRow struct {
	Task         searchBudgetTask
	SearchMetric outputMetric
	RGMetric     outputMetric
	ResultCount  int
}

type searchBudgetReport struct {
	GeneratedAt time.Time
	Spec        string
	SourceDir   string
	Rows        []searchBudgetRow
}

func newSearchBudgetReport(env searchE2EEnv) searchBudgetReport {
	return searchBudgetReport{
		GeneratedAt: time.Now().UTC(),
		Spec:        kotlinComposeSpec,
		SourceDir:   env.sourceDir,
	}
}

func measureOutput(output []byte) outputMetric {
	return outputMetric{
		Bytes:           len(output),
		Lines:           bytes.Count(output, []byte("\n")),
		EstimatedTokens: estimateTokens(output),
	}
}

func estimateTokens(output []byte) int {
	if len(output) == 0 {
		return 0
	}
	return (len(output) + 3) / 4
}

func writeSearchBudgetReport(t *testing.T, path string, report searchBudgetReport) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var markdown strings.Builder
	markdown.WriteString("# RepoBridge Search Output Budget\n\n")
	markdown.WriteString(fmt.Sprintf("- Generated: `%s`\n", report.GeneratedAt.Format(time.RFC3339)))
	markdown.WriteString(fmt.Sprintf("- Spec: `%s`\n", report.Spec))
	markdown.WriteString(fmt.Sprintf("- Source: `%s`\n", report.SourceDir))
	markdown.WriteString("- Token estimate: UTF-8 bytes divided by 4. Use model API usage or a model tokenizer for exact accounting.\n\n")
	markdown.WriteString("| Task | Results | RepoBridge bytes | RepoBridge est. tokens | rg bytes | rg est. tokens | Estimated reduction |\n")
	markdown.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, row := range report.Rows {
		reduction := 0.0
		if row.RGMetric.EstimatedTokens > 0 {
			reduction = 100 * (1 - float64(row.SearchMetric.EstimatedTokens)/float64(row.RGMetric.EstimatedTokens))
		}
		markdown.WriteString(fmt.Sprintf(
			"| %s | %d | %d | %d | %d | %d | %.1f%% |\n",
			row.Task.Name,
			row.ResultCount,
			row.SearchMetric.Bytes,
			row.SearchMetric.EstimatedTokens,
			row.RGMetric.Bytes,
			row.RGMetric.EstimatedTokens,
			reduction,
		))
	}
	markdown.WriteString("\n## Search Tasks\n\n")
	for _, row := range report.Rows {
		markdown.WriteString(fmt.Sprintf("### %s\n\n", row.Task.Name))
		markdown.WriteString(fmt.Sprintf("- RepoBridge query: `%s`\n", row.Task.SearchQuery))
		markdown.WriteString(fmt.Sprintf("- rg command: `rg -n -C 3 --no-heading --color never %q <source>`\n\n", row.Task.RGPattern))
	}
	if err := os.WriteFile(path, []byte(markdown.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCommand(t *testing.T, dir string, env []string, timeout time.Duration, name string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("command timed out in %s: %s %s\nstderr:\n%s", dir, name, strings.Join(args, " "), stderr.String())
	}
	if err != nil {
		t.Fatalf("command failed in %s: %s %s\nerror: %v\nstdout:\n%s\nstderr:\n%s", dir, name, strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	if stderr.Len() > 0 {
		fmt.Fprintf(os.Stderr, "%s %s stderr:\n%s", name, strings.Join(args, " "), stderr.String())
	}
	return stdout.Bytes()
}
