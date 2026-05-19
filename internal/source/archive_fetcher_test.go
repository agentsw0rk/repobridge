package source

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"repobridge/internal/registry"
)

func TestFetchPackageArchiveDownloadsMavenSourceJar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	body := zipBytes(t, map[string]string{
		"src/main/java/App.java": "class App {}",
		"META-INF/MANIFEST.MF":   "Manifest-Version: 1.0\n",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	pkg := registry.ResolvedPackage{
		Registry:         registry.Maven,
		Name:             "org.example:demo",
		Version:          "1.2.3",
		SourceArchiveURL: server.URL + "/demo-1.2.3-sources.jar",
	}
	result := FetchPackageArchive(pkg, server.Client())

	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if !result.Success {
		t.Fatal("Success = false, want true")
	}
	if result.Package != pkg.Name || result.Version != pkg.Version || result.Registry != registry.Maven {
		t.Fatalf("result = %#v", result)
	}
	if got, want := result.Path, "repos/maven/org.example/demo/1.2.3"; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
	extracted := filepath.Join(home, filepath.FromSlash(result.Path), "src/main/java/App.java")
	content, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "class App {}" {
		t.Fatalf("extracted content = %q", content)
	}
}

func TestFetchPackageArchiveReturnsSourceArchiveNotFoundErrorFor404(t *testing.T) {
	t.Setenv("REPOBRIDGE_HOME", t.TempDir())

	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	pkg := registry.ResolvedPackage{
		Registry:         registry.Maven,
		Name:             "org.example:demo",
		Version:          "1.2.3",
		SourceArchiveURL: server.URL + "/missing.jar",
	}
	result := FetchPackageArchive(pkg, server.Client())

	var notFound *sourceArchiveNotFoundError
	if !errors.As(result.Error, &notFound) {
		t.Fatalf("error = %T %[1]v, want sourceArchiveNotFoundError", result.Error)
	}
}

func TestFetchPackageArchiveRejectsOversizedCompressedDownload(t *testing.T) {
	t.Setenv("REPOBRIDGE_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(maxSourceArchiveBytes+1, 10))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	pkg := registry.ResolvedPackage{
		Registry:         registry.Maven,
		Name:             "org.example:demo",
		Version:          "1.2.3",
		SourceArchiveURL: server.URL + "/demo-1.2.3-sources.jar",
	}
	result := FetchPackageArchive(pkg, server.Client())

	if result.Error == nil || !strings.Contains(result.Error.Error(), "source archive exceeds maximum size") {
		t.Fatalf("error = %v, want compressed archive size limit error", result.Error)
	}
}

func TestFetchPackageArchiveRejectsOversizedUncompressedArchiveMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	body := zipWithAdvertisedUncompressedSize(t, "Big.java", uint32(maxExtractedArchiveBytes+1))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	pkg := registry.ResolvedPackage{
		Registry:         registry.Maven,
		Name:             "org.example:demo",
		Version:          "1.2.3",
		SourceArchiveURL: server.URL + "/demo-1.2.3-sources.jar",
	}
	result := FetchPackageArchive(pkg, server.Client())

	if result.Error == nil || !strings.Contains(result.Error.Error(), "extracted source archive exceeds maximum size") {
		t.Fatalf("error = %v, want extracted archive size limit error", result.Error)
	}
	target := filepath.Join(home, "repos/maven/org.example/demo/1.2.3")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists or unexpected error after failed extraction: %v", err)
	}
}

func TestFetchPackageArchiveRejectsOversizedUncompressedArchiveCopy(t *testing.T) {
	var dst bytes.Buffer
	remaining := uint64(4)

	err := copyArchiveEntry(strings.NewReader("12345"), &dst, &remaining, 4)

	if err == nil || !strings.Contains(err.Error(), "extracted source archive exceeds maximum size") {
		t.Fatalf("error = %v, want extracted archive size limit error", err)
	}
}

func TestFetchPackageArchiveRejectsTooManyArchiveEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	body := zipWithEmptyEntries(t, maxSourceArchiveEntries+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	pkg := registry.ResolvedPackage{
		Registry:         registry.Maven,
		Name:             "org.example:demo",
		Version:          "1.2.3",
		SourceArchiveURL: server.URL + "/demo-1.2.3-sources.jar",
	}
	result := FetchPackageArchive(pkg, server.Client())

	if result.Error == nil || !strings.Contains(result.Error.Error(), "source archive contains too many entries") {
		t.Fatalf("error = %v, want archive entry count limit error", result.Error)
	}
	target := filepath.Join(home, "repos/maven/org.example/demo/1.2.3")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists or unexpected error after failed extraction: %v", err)
	}
}

func TestFetchPackageArchiveRejectsUnsafeZipEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	body := zipBytes(t, map[string]string{
		"../escape.txt": "nope",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	pkg := registry.ResolvedPackage{
		Registry:         registry.Maven,
		Name:             "org.example:demo",
		Version:          "1.2.3",
		SourceArchiveURL: server.URL + "/demo-1.2.3-sources.jar",
	}
	result := FetchPackageArchive(pkg, server.Client())

	if result.Error == nil {
		t.Fatal("error = nil, want unsafe zip error")
	}
	target := filepath.Join(home, "repos/maven/org.example/demo/1.2.3")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists or unexpected error after failed extraction: %v", err)
	}
}

func TestFetchPackageArchiveRejectsNestedTraversalZipEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	body := zipBytes(t, map[string]string{
		"dir/../file.txt": "nope",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	pkg := registry.ResolvedPackage{
		Registry:         registry.Maven,
		Name:             "org.example:demo",
		Version:          "1.2.3",
		SourceArchiveURL: server.URL + "/demo-1.2.3-sources.jar",
	}
	result := FetchPackageArchive(pkg, server.Client())

	if result.Error == nil {
		t.Fatal("error = nil, want unsafe zip error")
	}
	target := filepath.Join(home, "repos/maven/org.example/demo/1.2.3")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists or unexpected error after failed extraction: %v", err)
	}
}

func TestFetchPackageArchiveReusesExistingTargetWithoutHTTPCall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REPOBRIDGE_HOME", home)

	target := filepath.Join(home, "repos/maven/org.example/demo/1.2.3")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "Existing.java"), []byte("class Existing {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatal("HTTP server should not be called for reusable target")
	}))
	t.Cleanup(server.Close)

	pkg := registry.ResolvedPackage{
		Registry:         registry.Maven,
		Name:             "org.example:demo",
		Version:          "1.2.3",
		SourceArchiveURL: server.URL + "/demo-1.2.3-sources.jar",
	}
	result := FetchPackageArchive(pkg, server.Client())

	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if !result.Success {
		t.Fatal("Success = false, want true")
	}
	if called {
		t.Fatal("HTTP server was called")
	}
	if got, want := result.Path, "repos/maven/org.example/demo/1.2.3"; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipWithEmptyEntries(t *testing.T, count int) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for i := 0; i < count; i++ {
		if _, err := writer.Create("empty-" + strconv.Itoa(i) + ".txt"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipWithAdvertisedUncompressedSize(t *testing.T, name string, uncompressedSize uint32) []byte {
	t.Helper()

	var buf bytes.Buffer
	writeUint32 := func(value uint32) {
		t.Helper()
		if err := binary.Write(&buf, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	writeUint16 := func(value uint16) {
		t.Helper()
		if err := binary.Write(&buf, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}

	nameBytes := []byte(name)
	localOffset := buf.Len()
	writeUint32(0x04034b50)
	writeUint16(20)
	writeUint16(0)
	writeUint16(0)
	writeUint16(0)
	writeUint16(0)
	writeUint32(0)
	writeUint32(0)
	writeUint32(uncompressedSize)
	writeUint16(uint16(len(nameBytes)))
	writeUint16(0)
	if _, err := buf.Write(nameBytes); err != nil {
		t.Fatal(err)
	}

	centralOffset := buf.Len()
	writeUint32(0x02014b50)
	writeUint16(20)
	writeUint16(20)
	writeUint16(0)
	writeUint16(0)
	writeUint16(0)
	writeUint16(0)
	writeUint32(0)
	writeUint32(0)
	writeUint32(uncompressedSize)
	writeUint16(uint16(len(nameBytes)))
	writeUint16(0)
	writeUint16(0)
	writeUint16(0)
	writeUint16(0)
	writeUint32(0)
	writeUint32(uint32(localOffset))
	if _, err := buf.Write(nameBytes); err != nil {
		t.Fatal(err)
	}
	centralSize := buf.Len() - centralOffset

	writeUint32(0x06054b50)
	writeUint16(0)
	writeUint16(0)
	writeUint16(1)
	writeUint16(1)
	writeUint32(uint32(centralSize))
	writeUint32(uint32(centralOffset))
	writeUint16(0)

	return buf.Bytes()
}
