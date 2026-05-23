package updatecheck

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyArchiveChecksum(t *testing.T) {
	content := []byte("archive bytes")
	sum := sha256.Sum256(content)

	if err := VerifyArchiveChecksum("repobridge.tar.gz", content, map[string]string{
		"repobridge.tar.gz": hex.EncodeToString(sum[:]),
	}); err != nil {
		t.Fatalf("VerifyArchiveChecksum() error = %v", err)
	}

	if err := VerifyArchiveChecksum("repobridge.tar.gz", content, map[string]string{
		"repobridge.tar.gz": strings.Repeat("0", 64),
	}); err == nil {
		t.Fatal("VerifyArchiveChecksum() error = nil, want mismatch")
	}
}

func TestVerifyArchiveChecksumRequiresNamedChecksum(t *testing.T) {
	if err := VerifyArchiveChecksum("repobridge.tar.gz", []byte("archive bytes"), map[string]string{
		"other.tar.gz": strings.Repeat("0", 64),
	}); err == nil {
		t.Fatal("VerifyArchiveChecksum() error = nil, want missing checksum")
	}
}

func TestExtractTarGzRelease(t *testing.T) {
	archive := tarGzFixture(t, map[string]string{
		"repobridge_v1_linux_amd64/repobridge":      "binary",
		"repobridge_v1_linux_amd64/libobjectbox.so": "native",
	})
	dir := t.TempDir()

	extracted, err := ExtractArchive("repobridge_v1_linux_amd64.tar.gz", archive, dir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	if extracted != filepath.Join(dir, "repobridge_v1_linux_amd64") {
		t.Fatalf("extracted = %q, want top-level release directory", extracted)
	}
	if _, err := os.Stat(filepath.Join(extracted, "repobridge")); err != nil {
		t.Fatalf("repobridge missing: %v", err)
	}
}

func TestExtractZipRelease(t *testing.T) {
	archive := zipFixture(t, map[string]string{
		"repobridge_v1_windows_amd64/repobridge.exe": "binary",
		"repobridge_v1_windows_amd64/objectbox.dll":  "native",
	})
	dir := t.TempDir()

	extracted, err := ExtractArchive("repobridge_v1_windows_amd64.zip", archive, dir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	if extracted != filepath.Join(dir, "repobridge_v1_windows_amd64") {
		t.Fatalf("extracted = %q, want top-level release directory", extracted)
	}
	if _, err := os.Stat(filepath.Join(extracted, "repobridge.exe")); err != nil {
		t.Fatalf("repobridge.exe missing: %v", err)
	}
}

func TestExtractArchiveRejectsUnsupportedFormat(t *testing.T) {
	if _, err := ExtractArchive("repobridge_v1_linux_amd64.tar.xz", []byte("archive"), t.TempDir()); err == nil {
		t.Fatal("ExtractArchive() error = nil, want unsupported format")
	}
}

func TestExtractTarGzRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := tarGzFixture(t, map[string]string{
		"../escape": "unsafe",
	})

	if _, err := ExtractArchive("repobridge_v1_linux_amd64.tar.gz", archive, dir); err == nil {
		t.Fatal("ExtractArchive() error = nil, want unsafe path error")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); !os.IsNotExist(err) {
		t.Fatalf("escape file stat error = %v, want not exist", err)
	}
}

func TestExtractZipRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := zipFixture(t, map[string]string{
		"../escape": "unsafe",
	})

	if _, err := ExtractArchive("repobridge_v1_windows_amd64.zip", archive, dir); err == nil {
		t.Fatal("ExtractArchive() error = nil, want unsafe path error")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); !os.IsNotExist(err) {
		t.Fatalf("escape file stat error = %v, want not exist", err)
	}
}

func TestInstallExtractedReleaseReplacesExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows replacement is covered by zip extraction and manual integration")
	}
	dir := t.TempDir()
	current := filepath.Join(dir, "repobridge")
	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	extracted := filepath.Join(dir, "release")
	if err := os.MkdirAll(extracted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extracted, "repobridge"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extracted, "libobjectbox.so"), []byte("native"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InstallExtractedRelease(extracted, current, "linux"); err != nil {
		t.Fatalf("InstallExtractedRelease() error = %v", err)
	}
	if got, _ := os.ReadFile(current); string(got) != "new" {
		t.Fatalf("binary = %q, want new", got)
	}
	if info, err := os.Stat(current); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("binary mode = %v, want executable bit", info.Mode().Perm())
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "libobjectbox.so")); string(got) != "native" {
		t.Fatalf("native lib = %q, want native", got)
	}
}

func TestInstallExtractedReleaseReplacesWindowsExecutable(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "repobridge.exe")
	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	extracted := filepath.Join(dir, "release")
	if err := os.MkdirAll(extracted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extracted, "repobridge.exe"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extracted, "objectbox.dll"), []byte("native"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InstallExtractedRelease(extracted, current, "windows"); err != nil {
		t.Fatalf("InstallExtractedRelease() error = %v", err)
	}
	if got, _ := os.ReadFile(current); string(got) != "new" {
		t.Fatalf("binary = %q, want new", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "objectbox.dll")); string(got) != "native" {
		t.Fatalf("native lib = %q, want native", got)
	}
}

func TestReplaceFileWindowsReplacesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new.exe")
	dst := filepath.Join(dir, "repobridge.exe")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := replaceFile(src, dst, "windows"); err != nil {
		t.Fatalf("replaceFile() error = %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "new" {
		t.Fatalf("dst = %q, want new", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src stat error = %v, want not exist", err)
	}
}

func TestReplaceFileLinuxReplacesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new")
	dst := filepath.Join(dir, "repobridge")
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceFile(src, dst, "linux"); err != nil {
		t.Fatalf("replaceFile() error = %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "new" {
		t.Fatalf("dst = %q, want new", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src stat error = %v, want not exist", err)
	}
}

func TestReplaceFileWindowsKeepsDestinationWhenSourceMissing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "missing.exe")
	dst := filepath.Join(dir, "repobridge.exe")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := replaceFile(src, dst, "windows"); err == nil {
		t.Fatal("replaceFile() error = nil, want missing source error")
	}
	if got, _ := os.ReadFile(dst); string(got) != "old" {
		t.Fatalf("dst = %q, want old destination preserved", got)
	}
}

func TestInstallExtractedReleaseDoesNotOverwriteNativeLibsWhenBinaryReplaceFails(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "repobridge")
	if err := os.WriteFile(current, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	existingNative := filepath.Join(dir, "libobjectbox.so")
	if err := os.WriteFile(existingNative, []byte("old native"), 0o644); err != nil {
		t.Fatal(err)
	}

	extracted := filepath.Join(dir, "release")
	if err := os.MkdirAll(extracted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extracted, "repobridge"), []byte("new binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extracted, "libobjectbox.so"), []byte("new native"), 0o644); err != nil {
		t.Fatal(err)
	}

	originalReplace := replaceExtractedBinary
	replaceExtractedBinary = func(src, dst, goos string) error {
		return errors.New("replace failed")
	}
	t.Cleanup(func() {
		replaceExtractedBinary = originalReplace
	})

	if err := InstallExtractedRelease(extracted, current, "linux"); err == nil {
		t.Fatal("InstallExtractedRelease() error = nil, want binary replacement failure")
	}
	if got, _ := os.ReadFile(existingNative); string(got) != "old native" {
		t.Fatalf("native lib = %q, want old native preserved", got)
	}
}

func TestWriteReaderToFileUpdatesExistingFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file permissions do not preserve POSIX modes")
	}
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeReaderToFile(path, strings.NewReader("new"), 0o644); err != nil {
		t.Fatalf("writeReaderToFile() error = %v", err)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode = %v, want 0644", got)
	}
}

func TestCopyFileUpdatesExistingFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file permissions do not preserve POSIX modes")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst, 0o644); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}
	if info, err := os.Stat(dst); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode = %v, want 0644", got)
	}
}

func TestInstallExtractedReleaseRequiresBinary(t *testing.T) {
	extracted := t.TempDir()
	current := filepath.Join(t.TempDir(), "repobridge")

	if err := InstallExtractedRelease(extracted, current, "linux"); err == nil {
		t.Fatal("InstallExtractedRelease() error = nil, want missing binary")
	}
}

func tarGzFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		data := []byte(content)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
