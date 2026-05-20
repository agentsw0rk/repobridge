//go:build e2e

package e2escan

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
)

type project struct {
	Name        string   `json:"name"`
	Ecosystem   string   `json:"ecosystem"`
	Repo        string   `json:"repo"`
	Commit      string   `json:"commit"`
	SparsePaths []string `json:"sparse_paths"`
	Expected    []string `json:"expected"`
}

type scanResult struct {
	Candidates []struct {
		Spec string `json:"spec"`
	} `json:"candidates"`
}

func TestProjectScanE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e scan test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required for e2e scan test: %v", err)
	}

	root := repoRoot(t)
	workspace := os.Getenv("REPOBRIDGE_E2E_DIR")
	if workspace == "" {
		workspace = filepath.Join(root, "e2e")
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	projects := loadProjects(t, root)
	binary := buildRepoBridge(t, root, workspace)
	resultsDir := filepath.Join(workspace, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, project := range projects {
		project := project
		t.Run(project.Name, func(t *testing.T) {
			repoDir := checkoutProject(t, workspace, project)
			output := runScan(t, binary, repoDir)
			resultPath := filepath.Join(resultsDir, project.Name+".json")
			if err := os.WriteFile(resultPath, output, 0o644); err != nil {
				t.Fatal(err)
			}
			assertExpectedSpecs(t, project, output)
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func loadProjects(t *testing.T, root string) []project {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "test", "e2e-scan", "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	var projects []project
	if err := json.Unmarshal(content, &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) == 0 {
		t.Fatal("e2e project manifest is empty")
	}
	for _, project := range projects {
		if project.Name == "" || project.Repo == "" || project.Commit == "" || len(project.SparsePaths) == 0 || len(project.Expected) == 0 {
			t.Fatalf("project manifest entry is incomplete: %#v", project)
		}
	}
	return projects
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
	runCommand(t, root, 2*time.Minute, "go", "build", "-o", binary, "./cmd/repobridge")
	return binary
}

func checkoutProject(t *testing.T, workspace string, project project) string {
	t.Helper()
	repoDir := filepath.Join(workspace, "repos", project.Name)
	if os.Getenv("REPOBRIDGE_E2E_REFRESH") == "1" {
		if err := os.RemoveAll(repoDir); err != nil {
			t.Fatal(err)
		}
	}
	if currentCommit(repoDir) == project.Commit {
		return repoDir
	}
	if err := os.RemoveAll(repoDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	runCommand(t, repoDir, time.Minute, "git", "init")
	runCommand(t, repoDir, time.Minute, "git", "remote", "add", "origin", project.Repo)
	runCommand(t, repoDir, time.Minute, "git", "config", "core.sparseCheckout", "true")
	writeSparseCheckout(t, repoDir, project.SparsePaths)
	runCommand(t, repoDir, 3*time.Minute, "git", "fetch", "--depth", "1", "origin", project.Commit)
	runCommand(t, repoDir, time.Minute, "git", "checkout", "--detach", "FETCH_HEAD")
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
	var patterns []string
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

func runScan(t *testing.T, binary, repoDir string) []byte {
	t.Helper()
	return runCommand(t, filepath.Dir(binary), time.Minute, binary, "scan", "--cwd", repoDir, "--json", "--no-imports")
}

func assertExpectedSpecs(t *testing.T, project project, output []byte) {
	t.Helper()
	var result scanResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("could not parse scan JSON for %s: %v\n%s", project.Name, err, string(output))
	}
	found := map[string]bool{}
	for _, candidate := range result.Candidates {
		found[candidate.Spec] = true
	}
	for _, expected := range project.Expected {
		if !found[expected] {
			t.Fatalf("%s missing expected spec %q\nfound: %s", project.Name, expected, formatSpecs(found))
		}
	}
}

func formatSpecs(specs map[string]bool) string {
	values := make([]string, 0, len(specs))
	for spec := range specs {
		values = append(values, spec)
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}

func runCommand(t *testing.T, dir string, timeout time.Duration, name string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
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
