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
	name    string
	relPath string
}

type renderedFile struct {
	relPath string
	content []byte
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
		for _, file := range rendered {
			path := filepath.Join(opts.HomeDir, target.relPath, file.relPath)
			action, backupPath, err := plannedAction(path, file.content)
			if err != nil {
				return Result{}, err
			}
			result.Files = append(result.Files, PlannedFile{
				Target:     target.name,
				Path:       path,
				Action:     action,
				BackupPath: backupPath,
				Content:    append([]byte(nil), file.content...),
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
			current, err := os.ReadFile(file.Path)
			if err != nil {
				return Result{}, err
			}
			if err := os.MkdirAll(filepath.Dir(file.BackupPath), 0o755); err != nil {
				return Result{}, err
			}
			if err := os.WriteFile(file.BackupPath, current, 0o644); err != nil {
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

func SortFiles(files []PlannedFile) {
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Target != files[j].Target {
			return files[i].Target < files[j].Target
		}
		return files[i].Path < files[j].Path
	})
}

func normalizeOptions(opts Options) (Options, error) {
	opts.Target = strings.TrimSpace(strings.ToLower(opts.Target))
	if opts.Target == "" {
		return Options{}, fmt.Errorf("agent target is required")
	}
	if strings.TrimSpace(opts.SourceDir) == "" {
		sourceDir, err := defaultSourceDir()
		if err != nil {
			return Options{}, err
		}
		opts.SourceDir = sourceDir
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

func defaultSourceDir() (string, error) {
	candidates := []string{filepath.Join("skills", "repobridge")}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "skills", "repobridge"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDir, "skills", "repobridge"),
			filepath.Join(executableDir, "..", "skills", "repobridge"),
		)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "SKILL.md")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("bundled repobridge skill not found")
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

	metadataPath := filepath.Join(sourceDir, "agents", "openai.yaml")
	if metadata, err := os.ReadFile(metadataPath); err == nil {
		files = append(files, renderedFile{
			relPath: filepath.Join("agents", "openai.yaml"),
			content: append([]byte(nil), metadata...),
		})
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read bundled skill metadata %s: %w", metadataPath, err)
	}
	return files, nil
}

func renderSkill(content []byte, version string) []byte {
	version = strings.TrimSpace(version)
	if version == "" {
		return append([]byte(nil), content...)
	}
	block := []byte(fmt.Sprintf("\n## Installed Version\n\nThis skill was installed by RepoBridge for version `%s`.\n", version))
	if i := bytes.Index(content, []byte("\n# ")); i >= 0 {
		nextSection := bytes.Index(content[i+1:], []byte("\n## "))
		if nextSection >= 0 {
			insert := i + 1 + nextSection
			rendered := make([]byte, 0, len(content)+len(block))
			rendered = append(rendered, content[:insert]...)
			rendered = append(rendered, block...)
			rendered = append(rendered, content[insert:]...)
			return rendered
		}
	}
	rendered := append([]byte(nil), content...)
	return append(rendered, block...)
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
	backupPath, err := nextBackupPath(path)
	if err != nil {
		return "", "", err
	}
	return "replace", backupPath, nil
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
