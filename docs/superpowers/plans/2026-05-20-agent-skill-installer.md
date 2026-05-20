# Agent Skill Installer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `repobridge install-agent` for installing the bundled RepoBridge skill into local agent skill directories without any MCP configuration.

**Architecture:** Add a focused `internal/agentinstall` package for target validation, rendering, dry-run planning, idempotent writes, and backups. Add a thin Cobra command in `internal/cli` that parses flags, calls the package, and prints deterministic output.

**Tech Stack:** Go 1.22+, Cobra, Go `testing`, local filesystem with temporary test homes.

---

### Task 1: Agent Installer Core

**Files:**
- Create: `internal/agentinstall/installer.go`
- Test: `internal/agentinstall/installer_test.go`

- [ ] **Step 1: Write failing tests for target expansion, rendering, dry-run, apply, idempotence, and backup**

Create `internal/agentinstall/installer_test.go` with tests that build temporary source templates and temporary homes:

```go
package agentinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanExpandsAllTargets(t *testing.T) {
	sourceDir := writeSourceSkill(t)
	home := t.TempDir()

	result, err := Plan(Options{Target: "all", Version: "v1.2.3", SourceDir: sourceDir, HomeDir: home, DryRun: true})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(result.Files) != 8 {
		t.Fatalf("planned files = %d, want 8", len(result.Files))
	}
	for _, target := range []string{"codex", "claude", "cursor", "opencode"} {
		if !hasTarget(result.Files, target) {
			t.Fatalf("planned files missing target %s: %#v", target, result.Files)
		}
	}
}

func TestPlanRejectsUnknownTarget(t *testing.T) {
	_, err := Plan(Options{Target: "unknown", SourceDir: writeSourceSkill(t), HomeDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "unknown agent target") {
		t.Fatalf("Plan() error = %v, want unknown target", err)
	}
}

func TestRenderAddsVersionMetadata(t *testing.T) {
	sourceDir := writeSourceSkill(t)
	result, err := Plan(Options{Target: "codex", Version: "v1.2.3", SourceDir: sourceDir, HomeDir: t.TempDir(), DryRun: true})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	skill := findPlannedFile(t, result.Files, "SKILL.md")
	if !strings.Contains(string(skill.Content), "Installed Version") || !strings.Contains(string(skill.Content), "`v1.2.3`") {
		t.Fatalf("rendered skill missing version metadata:\n%s", string(skill.Content))
	}
}

func TestRenderOmitsVersionMetadataWhenVersionEmpty(t *testing.T) {
	sourceDir := writeSourceSkill(t)
	result, err := Plan(Options{Target: "codex", SourceDir: sourceDir, HomeDir: t.TempDir(), DryRun: true})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	skill := findPlannedFile(t, result.Files, "SKILL.md")
	if strings.Contains(string(skill.Content), "Installed Version") {
		t.Fatalf("rendered skill unexpectedly includes version metadata:\n%s", string(skill.Content))
	}
}

func TestApplyCreatesFiles(t *testing.T) {
	sourceDir := writeSourceSkill(t)
	home := t.TempDir()

	result, err := Apply(Options{Target: "codex", Version: "v1.2.3", SourceDir: sourceDir, HomeDir: home})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	assertAction(t, result.Files, "create", "SKILL.md")
	assertFileContains(t, filepath.Join(home, ".agents", "skills", "repobridge", "SKILL.md"), "v1.2.3")
	assertFileContains(t, filepath.Join(home, ".agents", "skills", "repobridge", "agents", "openai.yaml"), "RepoBridge")
}

func TestApplyIsIdempotentForIdenticalFiles(t *testing.T) {
	sourceDir := writeSourceSkill(t)
	home := t.TempDir()
	if _, err := Apply(Options{Target: "codex", SourceDir: sourceDir, HomeDir: home}); err != nil {
		t.Fatalf("initial Apply() error = %v", err)
	}

	result, err := Apply(Options{Target: "codex", SourceDir: sourceDir, HomeDir: home})
	if err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}
	assertAction(t, result.Files, "unchanged", "SKILL.md")
}

func TestApplyBacksUpAndReplacesDifferentFile(t *testing.T) {
	sourceDir := writeSourceSkill(t)
	home := t.TempDir()
	dest := filepath.Join(home, ".agents", "skills", "repobridge", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("local edits\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Apply(Options{Target: "codex", SourceDir: sourceDir, HomeDir: home})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	assertAction(t, result.Files, "replace", "SKILL.md")
	assertFileContains(t, dest+".bak", "local edits")
	assertFileContains(t, dest, "RepoBridge Project Context")
}

func TestDryRunDoesNotWrite(t *testing.T) {
	sourceDir := writeSourceSkill(t)
	home := t.TempDir()

	result, err := Plan(Options{Target: "codex", SourceDir: sourceDir, HomeDir: home, DryRun: true})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	assertAction(t, result.Files, "create", "SKILL.md")
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "repobridge", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote destination, stat err = %v", err)
	}
}
```

Add helpers in the same test file:

```go
func writeSourceSkill(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: repobridge\ndescription: test\n---\n\n# RepoBridge Project Context\n\nUse RepoBridge first.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agents", "openai.yaml"), []byte("interface:\n  display_name: RepoBridge\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func hasTarget(files []PlannedFile, target string) bool {
	for _, file := range files {
		if file.Target == target {
			return true
		}
	}
	return false
}

func findPlannedFile(t *testing.T, files []PlannedFile, name string) PlannedFile {
	t.Helper()
	for _, file := range files {
		if filepath.Base(file.Path) == name {
			return file
		}
	}
	t.Fatalf("planned file %s not found in %#v", name, files)
	return PlannedFile{}
}

func assertAction(t *testing.T, files []PlannedFile, action, name string) {
	t.Helper()
	for _, file := range files {
		if filepath.Base(file.Path) == name && file.Action == action {
			return
		}
	}
	t.Fatalf("action %s for %s not found in %#v", action, name, files)
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("%s = %q, want %q", path, string(content), want)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/agentinstall`

Expected: FAIL because `internal/agentinstall` and its types/functions do not exist.

- [ ] **Step 3: Implement minimal installer package**

Create `internal/agentinstall/installer.go`:

```go
package agentinstall

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Options struct {
	Target    string
	Version   string
	SourceDir string
	HomeDir   string
	DryRun    bool
}

type Result struct {
	Files []PlannedFile
}

type PlannedFile struct {
	Target     string
	Path       string
	Action     string
	BackupPath string
	Content    []byte
}

type targetDefinition struct {
	name     string
	relPath  string
}

var targetDefinitions = []targetDefinition{
	{name: "codex", relPath: filepath.Join(".agents", "skills", "repobridge")},
	{name: "claude", relPath: filepath.Join(".claude", "skills", "repobridge")},
	{name: "cursor", relPath: filepath.Join(".cursor", "skills", "repobridge")},
	{name: "opencode", relPath: filepath.Join(".config", "opencode", "skills", "repobridge")},
}

func Plan(opts Options) (Result, error) {
	opts, err := normalizeOptions(opts)
	if err != nil {
		return Result{}, err
	}
	targets, err := expandTargets(opts.Target)
	if err != nil {
		return Result{}, err
	}
	rendered, err := renderFiles(opts.SourceDir, opts.Version)
	if err != nil {
		return Result{}, err
	}
	var result Result
	for _, target := range targets {
		for _, renderedFile := range rendered {
			path := filepath.Join(opts.HomeDir, target.relPath, renderedFile.relPath)
			action, backupPath, err := plannedAction(path, renderedFile.content)
			if err != nil {
				return Result{}, err
			}
			result.Files = append(result.Files, PlannedFile{
				Target:     target.name,
				Path:       path,
				Action:     action,
				BackupPath: backupPath,
				Content:    renderedFile.content,
			})
		}
	}
	return result, nil
}

func Apply(opts Options) (Result, error) {
	result, err := Plan(opts)
	if err != nil {
		return Result{}, err
	}
	if opts.DryRun {
		return result, nil
	}
	for i := range result.Files {
		file := &result.Files[i]
		switch file.Action {
		case "unchanged":
			continue
		case "replace":
			if err := os.MkdirAll(filepath.Dir(file.BackupPath), 0o755); err != nil {
				return Result{}, err
			}
			content, err := os.ReadFile(file.Path)
			if err != nil {
				return Result{}, err
			}
			if err := os.WriteFile(file.BackupPath, content, 0o644); err != nil {
				return Result{}, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(file.Path, file.Content, 0o644); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func PrintConfig(opts Options) (Result, error) {
	opts.DryRun = true
	return Plan(opts)
}

type renderedFile struct {
	relPath string
	content []byte
}

func normalizeOptions(opts Options) (Options, error) {
	opts.Target = strings.TrimSpace(strings.ToLower(opts.Target))
	if opts.Target == "" {
		return Options{}, fmt.Errorf("agent target is required")
	}
	if strings.TrimSpace(opts.SourceDir) == "" {
		opts.SourceDir = filepath.Join("skills", "repobridge")
	}
	if strings.TrimSpace(opts.HomeDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Options{}, err
		}
		opts.HomeDir = home
	}
	return opts, nil
}

func expandTargets(target string) ([]targetDefinition, error) {
	if target == "all" {
		return append([]targetDefinition(nil), targetDefinitions...), nil
	}
	for _, def := range targetDefinitions {
		if def.name == target {
			return []targetDefinition{def}, nil
		}
	}
	return nil, fmt.Errorf("unknown agent target %q", target)
}

func renderFiles(sourceDir, version string) ([]renderedFile, error) {
	skillPath := filepath.Join(sourceDir, "SKILL.md")
	skill, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("read bundled skill %s: %w", skillPath, err)
	}
	files := []renderedFile{{
		relPath: "SKILL.md",
		content: renderSkill(skill, version),
	}}
	openAIPath := filepath.Join(sourceDir, "agents", "openai.yaml")
	if content, err := os.ReadFile(openAIPath); err == nil {
		files = append(files, renderedFile{relPath: filepath.Join("agents", "openai.yaml"), content: content})
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read bundled skill metadata %s: %w", openAIPath, err)
	}
	return files, nil
}

func renderSkill(content []byte, version string) []byte {
	version = strings.TrimSpace(version)
	if version == "" {
		return append([]byte(nil), content...)
	}
	block := []byte(fmt.Sprintf("\n## Installed Version\n\nThis skill was installed by RepoBridge for version `%s`.\n", version))
	if bytes.Contains(content, []byte("\n# ")) {
		parts := bytes.SplitN(content, []byte("\n# "), 2)
		return append(append(append([]byte(nil), parts[0]...), []byte("\n# ")...), append(parts[1], block...)...)
	}
	return append(append([]byte(nil), content...), block...)
}

func plannedAction(path string, content []byte) (string, string, error) {
	current, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "create", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if bytes.Equal(current, content) {
		return "unchanged", "", nil
	}
	backup, err := nextBackupPath(path)
	if err != nil {
		return "", "", err
	}
	return "replace", backup, nil
}

func nextBackupPath(path string) (string, error) {
	candidates := []string{path + ".bak"}
	for i := 1; i < 1000; i++ {
		candidates = append(candidates, fmt.Sprintf("%s.bak.%d", path, i))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not find available backup path for %s", path)
}

func SortFiles(files []PlannedFile) {
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Target != files[j].Target {
			return files[i].Target < files[j].Target
		}
		return files[i].Path < files[j].Path
	})
}
```

- [ ] **Step 4: Run package tests**

Run: `go test ./internal/agentinstall`

Expected: PASS.

### Task 2: CLI Command

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/commands.go`
- Test: `internal/cli/commands_test.go`

- [ ] **Step 1: Write failing CLI tests**

Append tests to `internal/cli/commands_test.go`:

```go
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
```

- [ ] **Step 2: Run CLI tests to verify failure**

Run: `go test ./internal/cli -run TestInstallAgent -count=1`

Expected: FAIL because `install-agent` is not registered.

- [ ] **Step 3: Register command in root**

Modify `internal/cli/root.go` and add before list/remove/clean:

```go
cmd.AddCommand(newInstallAgentCommand(opts))
```

- [ ] **Step 4: Implement Cobra command**

Modify imports in `internal/cli/commands.go` to include:

```go
"repobridge/internal/agentinstall"
```

Add this command function near the other command constructors:

```go
func newInstallAgentCommand(opts Options) *cobra.Command {
	var target string
	var version string
	var dryRun bool
	var printConfig bool
	var home string

	cmd := &cobra.Command{
		Use:   "install-agent",
		Short: "Install RepoBridge skill files for coding agents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			options := agentinstall.Options{
				Target:  target,
				Version: version,
				HomeDir: home,
				DryRun:  dryRun || printConfig,
			}
			if printConfig {
				result, err := agentinstall.PrintConfig(options)
				if err != nil {
					return err
				}
				printAgentInstallConfig(out, result)
				return nil
			}
			result, err := agentinstall.Apply(options)
			if err != nil {
				return err
			}
			printAgentInstallResult(out, result, dryRun)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "agent target: codex, claude, cursor, opencode, or all")
	cmd.Flags().StringVar(&version, "version", "", "pinned RepoBridge version to document in installed skill")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show planned skill installation changes without writing")
	cmd.Flags().BoolVar(&printConfig, "print-config", false, "print rendered skill files without writing")
	cmd.Flags().StringVar(&home, "home", "", "override home directory for installation")
	return cmd
}
```

Add output helpers:

```go
func printAgentInstallResult(out io.Writer, result agentinstall.Result, dryRun bool) {
	agentinstall.SortFiles(result.Files)
	prefix := "Installed"
	if dryRun {
		prefix = "Would install"
	}
	changed := 0
	for _, file := range result.Files {
		if file.Action != "unchanged" {
			changed++
		}
		if file.BackupPath != "" {
			fmt.Fprintf(out, "%s %s %s backup:%s\n", file.Target, file.Action, file.Path, file.BackupPath)
			continue
		}
		fmt.Fprintf(out, "%s %s %s\n", file.Target, file.Action, file.Path)
	}
	fmt.Fprintf(out, "%s %d file(s)\n", prefix, changed)
}

func printAgentInstallConfig(out io.Writer, result agentinstall.Result) {
	agentinstall.SortFiles(result.Files)
	for _, file := range result.Files {
		fmt.Fprintf(out, "---\ntarget: %s\nfile: %s\n", file.Target, file.Path)
		fmt.Fprintln(out, string(file.Content))
	}
}
```

- [ ] **Step 5: Run CLI tests**

Run: `go test ./internal/cli -run TestInstallAgent -count=1`

Expected: PASS.

### Task 3: Documentation and Feature Completion

**Files:**
- Modify: `README.md`
- Modify: `docs/features/00-feature-set-overview.md`
- Create: `docs/features/27-agent-installer-done.md`

- [ ] **Step 1: Add README command documentation**

Add `install-agent` to the command list in `README.md` near other CLI commands:

```markdown
Install the bundled RepoBridge skill for an agent:

```bash
repobridge install-agent --target codex --version v0.3.0
repobridge install-agent --target codex --dry-run
repobridge install-agent --target codex --print-config
```

The installer writes skill files only. It does not install MCP configuration.
```

- [ ] **Step 2: Mark feature 27 done in overview**

Modify `docs/features/00-feature-set-overview.md`:

```markdown
| 27 | Agent-Installer fuer Skill-Konfiguration | Fertig | [27-agent-installer-done.md](27-agent-installer-done.md) | M |
```

- [ ] **Step 3: Create done file**

Create `docs/features/27-agent-installer-done.md`:

```markdown
# Feature 27 Done: Agent Installer

## Zusammenfassung

RepoBridge hat jetzt `install-agent`, um den gebuendelten RepoBridge-Skill fuer Codex, Claude, Cursor und opencode zu installieren. Der Installer unterstuetzt `--dry-run`, `--print-config`, `--target all` und optionale Versionsmetadaten ueber `--version`.

## Abweichungen

Der urspruengliche Feature-Text nennt MCP-Konfiguration. Diese Implementierung installiert bewusst nur Skills und Agent-Instructions. MCP-Konfiguration und `serve --mcp` bleiben ausserhalb des Scopes.

## Offene Punkte

- MCP-Agent-Konfiguration kann spaeter nach Feature 23 separat ergaenzt werden.
- Zielpfade fuer Cursor und opencode sind konservative Skill-Verzeichnisse und koennen angepasst werden, wenn diese Agenten verbindliche Skill-Spezifikationen stabilisieren.
```

- [ ] **Step 4: Run full verification**

Run:

```bash
gofmt -w ./cmd ./internal
go test ./...
go test -tags e2e ./test/e2e-search -run TestGraphCommandsAfterSearchE2E -count=1
```

Expected: all commands pass.
