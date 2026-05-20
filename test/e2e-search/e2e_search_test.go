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
	if testing.Short() {
		t.Skip("skipping e2e search test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required for e2e search test: %v", err)
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
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	binary := buildRepoBridge(t, root, workspace)
	home := filepath.Join(workspace, "home")
	t.Setenv("REPOBRIDGE_HOME", home)
	checkoutCachedRepo(t, home)
	t.Logf("using repobridge binary: %s", binary)
	t.Logf("using REPOBRIDGE_HOME: %s", home)
	t.Logf("searching pinned Kotlin repo: %s", kotlinComposeSpec)

	results := runSearchJSON(t, binary, home, kotlinComposeSpec, `lang:kotlin kind:function name:up calls:exec path:DockerCompose.kt`)
	assertKotlinUpResult(t, results)

	secondResults := runSearchJSON(t, binary, home, kotlinComposeSpec, `lang:kotlin kind:function name:up path:DockerCompose.kt`)
	assertKotlinUpResult(t, secondResults)
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

func checkoutCachedRepo(t *testing.T, home string) {
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
	output := runCommand(t, filepath.Dir(binary), []string{"REPOBRIDGE_HOME=" + home}, 4*time.Minute, binary, "search", "--json", "--limit", "20", spec, query)
	var results []codegraph.SearchResult
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("could not parse search JSON: %v\n%s", err, string(output))
	}
	t.Logf("search query %q returned %d result(s)", query, len(results))
	return results
}

func assertKotlinUpResult(t *testing.T, results []codegraph.SearchResult) {
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
		return
	}
	t.Fatalf("Kotlin function up with exec call not found in %#v", results)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
