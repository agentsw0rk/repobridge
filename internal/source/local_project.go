package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"repobridge/internal/cache"
)

const projectSpecPrefix = "project:"

func isProjectSpec(spec string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(spec)), projectSpecPrefix)
}

func resolveProjectSource(spec, cwd string) (Outcome, error) {
	raw := strings.TrimSpace(spec)
	target := strings.TrimSpace(raw[len(projectSpecPrefix):])
	if target == "" {
		target = "."
	}
	root := strings.TrimSpace(cwd)
	if root == "" {
		root = "."
	}
	rootAbs, err := canonicalDir(root)
	if err != nil {
		return Outcome{}, fmt.Errorf("resolve project cwd: %w", err)
	}

	var targetAbs string
	if filepath.IsAbs(filepath.FromSlash(target)) {
		targetAbs, err = canonicalDir(filepath.FromSlash(target))
	} else {
		targetAbs, err = canonicalDir(filepath.Join(rootAbs, filepath.FromSlash(target)))
		if err == nil {
			rel, relErr := filepath.Rel(rootAbs, targetAbs)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
				return Outcome{}, fmt.Errorf("project path escapes cwd: %s", target)
			}
		}
	}
	if err != nil {
		return Outcome{}, fmt.Errorf("resolve project source: %w", err)
	}
	graphPath, err := cache.ProjectGraphDirForSource(targetAbs)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		Path:        targetAbs,
		Name:        canonicalProjectSpec(rootAbs, targetAbs),
		SourceLabel: "project",
		SourceKind:  "project",
		GraphPath:   graphPath,
		FromCache:   true,
	}, nil
}

func canonicalDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

func canonicalProjectSpec(rootAbs, targetAbs string) string {
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || rel == "." {
		return "project:."
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "project:" + filepath.ToSlash(targetAbs)
	}
	return "project:./" + filepath.ToSlash(rel)
}
