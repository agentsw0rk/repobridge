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

func TestScanSourceFilesReturnsErrorForMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := ScanSourceFiles(missing, Options{}); err == nil {
		t.Fatal("ScanSourceFiles() error = nil, want error")
	}
}

func TestScanSourceFilesSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.go")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	files, err := ScanSourceFiles(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("files = %#v, want no symlinked files", files)
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
