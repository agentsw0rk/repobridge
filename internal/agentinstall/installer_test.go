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
