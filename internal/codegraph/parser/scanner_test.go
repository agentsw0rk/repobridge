package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSourceFilesIncludesSupportedLanguagesAndSkipsGeneratedDirs(t *testing.T) {
	root := t.TempDir()
	writeParserFixture(t, root, "main.go", "package main\nfunc main() {}\n")
	writeParserFixture(t, root, "src/app.ts", "export function run() {}\n")
	writeParserFixture(t, root, "node_modules/pkg/index.js", "function hidden() {}\n")
	writeParserFixture(t, root, ".repobridge-graph/data", "ignored")
	writeParserFixture(t, root, "README.md", "# ignored")

	files, err := ScanSourceFiles(root, Options{MaxFileSize: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %#v, want two supported source files", files)
	}
	if files[0].RelativePath != "main.go" || files[0].Language != "go" {
		t.Fatalf("first file = %#v", files[0])
	}
	if files[1].RelativePath != "src/app.ts" || files[1].Language != "typescript" {
		t.Fatalf("second file = %#v", files[1])
	}
}

func writeParserFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
