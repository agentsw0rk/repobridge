# RepoBridge Self-Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safe, non-blocking update checks to every normal RepoBridge command, plus an explicit `repobridge self-update` installer.

**Architecture:** Put release lookup, version comparison, state caching, checksum verification, archive extraction, and binary replacement in a focused `internal/updatecheck` package. Keep CLI concerns in `internal/cli`: command wiring, skip rules, interactivity, prompts, and output. Opportunistic checks fail open; explicit `self-update` fails closed.

**Tech Stack:** Go standard library, Cobra, existing `internal/cache.Home`, GitHub Releases HTTP API, release archives already produced by `.github/workflows/generator-generic-ossf-slsa3-publish.yml`.

---

## File Structure

- Create `internal/updatecheck/version.go`: release-version parsing and comparison.
- Create `internal/updatecheck/version_test.go`: table-driven version behavior.
- Create `internal/updatecheck/client.go`: GitHub latest-release client, asset model, asset selection, checksum parsing.
- Create `internal/updatecheck/client_test.go`: fake HTTP server tests for latest release, asset selection, checksum verification.
- Create `internal/updatecheck/state.go`: best-effort update check state under `REPOBRIDGE_HOME`.
- Create `internal/updatecheck/state_test.go`: state read/write and stale/fresh interval tests.
- Create `internal/updatecheck/install.go`: archive download, checksum verification, extraction, executable replacement.
- Create `internal/updatecheck/install_test.go`: local archive/checksum/install tests.
- Create `internal/updatecheck/service.go`: high-level `Check`, `Install`, and opportunistic fail-open API.
- Create `internal/updatecheck/service_test.go`: check-only/current/outdated/error behavior.
- Modify `internal/cli/root.go`: extend `Options`, attach persistent pre-run update hook, skip update checks for version/internal/self-update cases.
- Modify `internal/cli/commands.go`: add `self-update` command.
- Modify `internal/cli/commands_test.go`: CLI command behavior and hook tests.
- Modify `README.md`: document `self-update`, non-interactive behavior, and `REPOBRIDGE_NO_UPDATE_CHECK=1`.

---

### Task 1: Version Comparison

**Files:**
- Create: `internal/updatecheck/version.go`
- Test: `internal/updatecheck/version_test.go`

- [ ] **Step 1: Write failing version tests**

Create `internal/updatecheck/version_test.go`:

```go
package updatecheck

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "equal with v prefix", a: "v0.10.4", b: "0.10.4", want: 0},
		{name: "newer patch", a: "v0.10.5", b: "v0.10.4", want: 1},
		{name: "older minor", a: "v0.9.9", b: "v0.10.0", want: -1},
		{name: "newer minor", a: "v0.11.0", b: "v0.10.9", want: 1},
		{name: "newer major", a: "v1.0.0", b: "v0.99.99", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareVersions(tt.a, tt.b)
			if err != nil {
				t.Fatalf("CompareVersions() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareVersionsRejectsMalformedVersions(t *testing.T) {
	for _, version := range []string{"", "dev", "main", "v1", "v1.2", "v1.2.x", "v1.2.3.4"} {
		t.Run(version, func(t *testing.T) {
			if _, err := CompareVersions(version, "v1.2.3"); err == nil {
				t.Fatalf("CompareVersions(%q, v1.2.3) error = nil, want error", version)
			}
		})
	}
}

func TestIsReleaseVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "v0.10.4", want: true},
		{version: "0.10.4", want: true},
		{version: "dev", want: false},
		{version: "", want: false},
	}
	for _, tt := range tests {
		if got := IsReleaseVersion(tt.version); got != tt.want {
			t.Fatalf("IsReleaseVersion(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/updatecheck -run 'TestCompareVersions|TestIsReleaseVersion'`

Expected: FAIL because package or functions do not exist.

- [ ] **Step 3: Implement minimal version comparison**

Create `internal/updatecheck/version.go`:

```go
package updatecheck

import (
	"fmt"
	"strconv"
	"strings"
)

type semanticVersion struct {
	major int
	minor int
	patch int
}

func IsReleaseVersion(version string) bool {
	_, err := parseVersion(version)
	return err == nil
}

func CompareVersions(a, b string) (int, error) {
	av, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	bv, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	switch {
	case av.major != bv.major:
		return compareInt(av.major, bv.major), nil
	case av.minor != bv.minor:
		return compareInt(av.minor, bv.minor), nil
	default:
		return compareInt(av.patch, bv.patch), nil
	}
}

func parseVersion(version string) (semanticVersion, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("invalid release version %q", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid release version %q", version)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid release version %q", version)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid release version %q", version)
	}
	return semanticVersion{major: major, minor: minor, patch: patch}, nil
}

func compareInt(a, b int) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/updatecheck -run 'TestCompareVersions|TestIsReleaseVersion'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/updatecheck/version.go internal/updatecheck/version_test.go
git commit -m "Add release version comparison"
```

---

### Task 2: Release Client, Asset Selection, And Checksums

**Files:**
- Create: `internal/updatecheck/client.go`
- Test: `internal/updatecheck/client_test.go`

- [ ] **Step 1: Write failing release client tests**

Create `internal/updatecheck/client_test.go`:

```go
package updatecheck

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientLatestRelease(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/agentsw0rk/repobridge/releases/latest" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body := `{
			"tag_name":"v0.10.5",
			"html_url":"https://github.com/agentsw0rk/repobridge/releases/tag/v0.10.5",
			"assets":[
				{"name":"checksums.txt","browser_download_url":"__SERVER__/checksums.txt"},
				{"name":"repobridge_v0.10.5_linux_amd64.tar.gz","browser_download_url":"__SERVER__/linux.tar.gz"}
			]
		}`
		body = strings.ReplaceAll(body, "__SERVER__", serverURL)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	serverURL = server.URL

	client := Client{HTTPClient: server.Client(), BaseURL: server.URL}
	release, err := client.LatestRelease(t.Context())
	if err != nil {
		t.Fatalf("LatestRelease() error = %v", err)
	}
	if release.TagName != "v0.10.5" {
		t.Fatalf("TagName = %q", release.TagName)
	}
	if len(release.Assets) != 2 {
		t.Fatalf("assets = %d, want 2", len(release.Assets))
	}
}

func TestSelectPlatformAsset(t *testing.T) {
	release := Release{TagName: "v0.10.5", Assets: []Asset{
		{Name: "checksums.txt", DownloadURL: "https://example.test/checksums.txt"},
		{Name: "repobridge_v0.10.5_linux_amd64.tar.gz", DownloadURL: "https://example.test/linux"},
		{Name: "repobridge_v0.10.5_darwin_arm64.tar.gz", DownloadURL: "https://example.test/darwin"},
		{Name: "repobridge_v0.10.5_windows_amd64.zip", DownloadURL: "https://example.test/windows"},
	}}
	asset, checksums, err := release.SelectAssets("linux", "amd64")
	if err != nil {
		t.Fatalf("SelectAssets() error = %v", err)
	}
	if asset.Name != "repobridge_v0.10.5_linux_amd64.tar.gz" {
		t.Fatalf("asset = %q", asset.Name)
	}
	if checksums.Name != "checksums.txt" {
		t.Fatalf("checksums = %q", checksums.Name)
	}
}

func TestParseChecksums(t *testing.T) {
	content := strings.Join([]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  repobridge_v0.10.5_linux_amd64.tar.gz",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  repobridge_v0.10.5_windows_amd64.zip",
	}, "\n")
	checksums, err := ParseChecksums([]byte(content))
	if err != nil {
		t.Fatalf("ParseChecksums() error = %v", err)
	}
	if checksums["repobridge_v0.10.5_windows_amd64.zip"] != strings.Repeat("b", 64) {
		t.Fatalf("checksums = %#v", checksums)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/updatecheck -run 'TestClientLatestRelease|TestSelectPlatformAsset|TestParseChecksums'`

Expected: FAIL because `Client`, `Release`, and checksum parsing are undefined.

- [ ] **Step 3: Implement release client**

Create `internal/updatecheck/client.go`:

```go
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultGitHubAPIBaseURL = "https://api.github.com"

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

func (c Client) LatestRelease(ctx context.Context) (Release, error) {
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGitHubAPIBaseURL
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/repos/agentsw0rk/repobridge/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Release{}, fmt.Errorf("GitHub latest release returned HTTP %d", resp.StatusCode)
	}
	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return Release{}, err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return Release{}, fmt.Errorf("GitHub latest release missing tag_name")
	}
	return release, nil
}

func (c Client) Download(ctx context.Context, url string) ([]byte, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download returned HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func (r Release) SelectAssets(goos, goarch string) (Asset, Asset, error) {
	archiveName := fmt.Sprintf("repobridge_%s_%s_%s", r.TagName, goos, goarch)
	if goos == "windows" {
		archiveName += ".zip"
	} else {
		archiveName += ".tar.gz"
	}
	var archive Asset
	var checksums Asset
	for _, asset := range r.Assets {
		switch asset.Name {
		case archiveName:
			archive = asset
		case "checksums.txt":
			checksums = asset
		}
	}
	if archive.Name == "" {
		return Asset{}, Asset{}, fmt.Errorf("release %s has no asset for %s/%s", r.TagName, goos, goarch)
	}
	if checksums.Name == "" {
		return Asset{}, Asset{}, fmt.Errorf("release %s has no checksums.txt", r.TagName)
	}
	return archive, checksums, nil
}

func ParseChecksums(content []byte) (map[string]string, error) {
	result := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if len(fields[0]) != 64 {
			return nil, fmt.Errorf("invalid sha256 checksum for %s", fields[1])
		}
		result[fields[1]] = strings.ToLower(fields[0])
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no checksums found")
	}
	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/updatecheck -run 'TestClientLatestRelease|TestSelectPlatformAsset|TestParseChecksums'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/updatecheck/client.go internal/updatecheck/client_test.go
git commit -m "Add GitHub release update client"
```

---

### Task 3: Update Check State Cache

**Files:**
- Create: `internal/updatecheck/state.go`
- Test: `internal/updatecheck/state_test.go`

- [ ] **Step 1: Write failing state tests**

Create `internal/updatecheck/state_test.go`:

```go
package updatecheck

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStateStoreRoundTrips(t *testing.T) {
	store := StateStore{HomeDir: t.TempDir()}
	now := time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC)
	state := State{
		CheckedAt:  now,
		Latest:     "v0.10.5",
		ReleaseURL: "https://github.com/agentsw0rk/repobridge/releases/tag/v0.10.5",
	}
	if err := store.Write(state); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := store.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !got.CheckedAt.Equal(now) || got.Latest != state.Latest || got.ReleaseURL != state.ReleaseURL {
		t.Fatalf("state = %#v, want %#v", got, state)
	}
	if filepath.Base(store.path()) != "update-check.json" {
		t.Fatalf("state path = %s", store.path())
	}
}

func TestStateStoreFreshness(t *testing.T) {
	now := time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC)
	fresh := State{CheckedAt: now.Add(-time.Hour)}
	stale := State{CheckedAt: now.Add(-25 * time.Hour)}
	if !fresh.IsFresh(now, 24*time.Hour) {
		t.Fatal("fresh state reported stale")
	}
	if stale.IsFresh(now, 24*time.Hour) {
		t.Fatal("stale state reported fresh")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/updatecheck -run TestStateStore`

Expected: FAIL because `StateStore` is undefined.

- [ ] **Step 3: Implement state cache**

Create `internal/updatecheck/state.go`:

```go
package updatecheck

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"repobridge/internal/cache"
)

type State struct {
	CheckedAt  time.Time `json:"checkedAt"`
	Latest     string    `json:"latest,omitempty"`
	ReleaseURL string    `json:"releaseUrl,omitempty"`
}

type StateStore struct {
	HomeDir string
}

func DefaultStateStore() (StateStore, error) {
	home, err := cache.Home()
	if err != nil {
		return StateStore{}, err
	}
	return StateStore{HomeDir: home}, nil
}

func (s State) IsFresh(now time.Time, interval time.Duration) bool {
	if s.CheckedAt.IsZero() {
		return false
	}
	return now.Sub(s.CheckedAt) < interval
}

func (s StateStore) Read() (State, error) {
	content, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(content, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s StateStore) Write(state State) error {
	if err := os.MkdirAll(filepath.Dir(s.path()), 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(), append(content, '\n'), 0o644)
}

func (s StateStore) path() string {
	return filepath.Join(s.HomeDir, "update-check.json")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/updatecheck -run TestStateStore`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/updatecheck/state.go internal/updatecheck/state_test.go
git commit -m "Add update check state cache"
```

---

### Task 4: Archive Verification And Installation

**Files:**
- Create: `internal/updatecheck/install.go`
- Test: `internal/updatecheck/install_test.go`

- [ ] **Step 1: Write failing install tests**

Create `internal/updatecheck/install_test.go`:

```go
package updatecheck

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
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

func TestExtractTarGzRelease(t *testing.T) {
	archive := tarGzFixture(t, map[string]string{
		"repobridge_v1_linux_amd64/repobridge":    "binary",
		"repobridge_v1_linux_amd64/libobjectbox.so": "native",
	})
	dir := t.TempDir()
	extracted, err := ExtractArchive("repobridge_v1_linux_amd64.tar.gz", archive, dir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(extracted, "repobridge")); err != nil {
		t.Fatalf("repobridge missing: %v", err)
	}
}

func TestExtractZipRelease(t *testing.T) {
	archive := zipFixture(t, map[string]string{
		"repobridge_v1_windows_amd64/repobridge.exe": "binary",
		"repobridge_v1_windows_amd64/objectbox.dll": "native",
	})
	dir := t.TempDir()
	extracted, err := ExtractArchive("repobridge_v1_windows_amd64.zip", archive, dir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(extracted, "repobridge.exe")); err != nil {
		t.Fatalf("repobridge.exe missing: %v", err)
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
	if err := os.WriteFile(filepath.Join(extracted, "repobridge"), []byte("new"), 0o755); err != nil {
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
	if got, _ := os.ReadFile(filepath.Join(dir, "libobjectbox.so")); string(got) != "native" {
		t.Fatalf("native lib = %q, want native", got)
	}
}
```

Add helper functions in the same test file:

```go
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
```

Include `strings` in the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/updatecheck -run 'TestVerifyArchiveChecksum|TestExtract|TestInstallExtractedRelease'`

Expected: FAIL because install functions are undefined.

- [ ] **Step 3: Implement archive and install functions**

Create `internal/updatecheck/install.go`:

```go
package updatecheck

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func VerifyArchiveChecksum(name string, content []byte, checksums map[string]string) error {
	want := strings.ToLower(strings.TrimSpace(checksums[name]))
	if want == "" {
		return fmt.Errorf("checksum for %s not found", name)
	}
	sum := sha256.Sum256(content)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}

func ExtractArchive(name string, content []byte, targetDir string) (string, error) {
	if strings.HasSuffix(name, ".zip") {
		return extractZip(content, targetDir)
	}
	if strings.HasSuffix(name, ".tar.gz") {
		return extractTarGz(content, targetDir)
	}
	return "", fmt.Errorf("unsupported release archive %s", name)
}

func InstallExtractedRelease(extractedDir, currentExecutable, goos string) error {
	binaryName := "repobridge"
	if goos == "windows" {
		binaryName = "repobridge.exe"
	}
	sourceBinary := filepath.Join(extractedDir, binaryName)
	if _, err := os.Stat(sourceBinary); err != nil {
		return fmt.Errorf("release binary not found: %w", err)
	}
	if goos != "windows" {
		if err := os.Chmod(sourceBinary, 0o755); err != nil {
			return err
		}
	}
	destDir := filepath.Dir(currentExecutable)
	for _, pattern := range []string{"libobjectbox.*", "objectbox.dll"} {
		matches, err := filepath.Glob(filepath.Join(extractedDir, pattern))
		if err != nil {
			return err
		}
		for _, match := range matches {
			if err := copyFile(match, filepath.Join(destDir, filepath.Base(match)), 0o644); err != nil {
				return err
			}
		}
	}
	return os.Rename(sourceBinary, currentExecutable)
}

func extractTarGz(content []byte, targetDir string) (string, error) {
	gr, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	return extractTarEntries(tr, targetDir)
}

func extractTarEntries(tr *tar.Reader, targetDir string) (string, error) {
	var root string
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		target, top, err := safeExtractPath(targetDir, header.Name)
		if err != nil {
			return "", err
		}
		if root == "" {
			root = filepath.Join(targetDir, top)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(out, tr)
		closeErr := out.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	if root == "" {
		return "", fmt.Errorf("release archive is empty")
	}
	return root, nil
}

func extractZip(content []byte, targetDir string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", err
	}
	var root string
	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}
		target, top, err := safeExtractPath(targetDir, file.Name)
		if err != nil {
			return "", err
		}
		if root == "" {
			root = filepath.Join(targetDir, top)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		in, err := file.Open()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			_ = in.Close()
			return "", err
		}
		_, copyErr := io.Copy(out, in)
		closeInErr := in.Close()
		closeOutErr := out.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeInErr != nil {
			return "", closeInErr
		}
		if closeOutErr != nil {
			return "", closeOutErr
		}
	}
	if root == "" {
		return "", fmt.Errorf("release archive is empty")
	}
	return root, nil
}

func safeExtractPath(targetDir, archivePath string) (string, string, error) {
	clean := filepath.Clean(filepath.FromSlash(archivePath))
	if clean == "." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || filepath.IsAbs(clean) {
		return "", "", fmt.Errorf("unsafe archive path %s", archivePath)
	}
	parts := strings.Split(clean, string(os.PathSeparator))
	if len(parts) == 0 || parts[0] == "" {
		return "", "", fmt.Errorf("unsafe archive path %s", archivePath)
	}
	return filepath.Join(targetDir, clean), parts[0], nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, content, mode)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/updatecheck -run 'TestVerifyArchiveChecksum|TestExtract|TestInstallExtractedRelease'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/updatecheck/install.go internal/updatecheck/install_test.go
git commit -m "Add release archive installation"
```

---

### Task 5: High-Level Update Service

**Files:**
- Create: `internal/updatecheck/service.go`
- Test: `internal/updatecheck/service_test.go`

- [ ] **Step 1: Write failing service tests**

Create `internal/updatecheck/service_test.go`:

```go
package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServiceCheckReportsAvailableUpdate(t *testing.T) {
	server := releaseServer(t, "v0.10.5")
	service := Service{
		CurrentVersion: "v0.10.4",
		Client:         Client{HTTPClient: server.Client(), BaseURL: server.URL},
		Now:            func() time.Time { return time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC) },
	}
	result, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !result.Available || result.LatestVersion != "v0.10.5" {
		t.Fatalf("result = %#v, want available v0.10.5", result)
	}
}

func TestServiceCheckReportsCurrent(t *testing.T) {
	server := releaseServer(t, "v0.10.4")
	service := Service{CurrentVersion: "v0.10.4", Client: Client{HTTPClient: server.Client(), BaseURL: server.URL}}
	result, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Available {
		t.Fatalf("result = %#v, want no update", result)
	}
}

func TestServiceOpportunisticCheckUsesFreshState(t *testing.T) {
	store := StateStore{HomeDir: t.TempDir()}
	now := time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC)
	if err := store.Write(State{CheckedAt: now.Add(-time.Hour), Latest: "v0.10.5"}); err != nil {
		t.Fatal(err)
	}
	service := Service{
		CurrentVersion: "v0.10.4",
		StateStore:     store,
		Now:            func() time.Time { return now },
		CheckInterval:  24 * time.Hour,
	}
	result, err := service.OpportunisticCheck(context.Background())
	if err != nil {
		t.Fatalf("OpportunisticCheck() error = %v", err)
	}
	if !result.Available || result.LatestVersion != "v0.10.5" {
		t.Fatalf("result = %#v, want cached update", result)
	}
}

func releaseServer(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://example.test/` + tag + `","assets":[]}`))
	}))
	return server
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/updatecheck -run 'TestService'`

Expected: FAIL because `Service` is undefined.

- [ ] **Step 3: Implement service**

Create `internal/updatecheck/service.go`:

```go
package updatecheck

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"
)

const DefaultCheckInterval = 24 * time.Hour

type Service struct {
	CurrentVersion string
	Client         Client
	StateStore     StateStore
	CheckInterval  time.Duration
	Now            func() time.Time
	ExecutablePath func() (string, error)
	GOOS           string
	GOARCH         string
}

type CheckResult struct {
	Available      bool
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	Release        Release
}

func (s Service) Check(ctx context.Context) (CheckResult, error) {
	if !IsReleaseVersion(s.CurrentVersion) {
		return CheckResult{CurrentVersion: s.CurrentVersion}, nil
	}
	release, err := s.Client.LatestRelease(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	cmp, err := CompareVersions(release.TagName, s.CurrentVersion)
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{
		Available:      cmp > 0,
		CurrentVersion: s.CurrentVersion,
		LatestVersion:  release.TagName,
		ReleaseURL:     release.HTMLURL,
		Release:        release,
	}, nil
}

func (s Service) OpportunisticCheck(ctx context.Context) (CheckResult, error) {
	store := s.StateStore
	if store.HomeDir == "" {
		defaultStore, err := DefaultStateStore()
		if err != nil {
			return CheckResult{}, nil
		}
		store = defaultStore
	}
	now := s.now()
	interval := s.CheckInterval
	if interval <= 0 {
		interval = DefaultCheckInterval
	}
	if state, err := store.Read(); err == nil && state.IsFresh(now, interval) {
		cmp, cmpErr := CompareVersions(state.Latest, s.CurrentVersion)
		if cmpErr == nil && cmp > 0 {
			return CheckResult{
				Available:      true,
				CurrentVersion: s.CurrentVersion,
				LatestVersion:  state.Latest,
				ReleaseURL:     state.ReleaseURL,
			}, nil
		}
		return CheckResult{CurrentVersion: s.CurrentVersion, LatestVersion: state.Latest}, nil
	}
	result, err := s.Check(ctx)
	state := State{CheckedAt: now, Latest: result.LatestVersion, ReleaseURL: result.ReleaseURL}
	if err != nil {
		state.CheckedAt = now
		_ = store.Write(state)
		return CheckResult{}, nil
	}
	_ = store.Write(state)
	return result, nil
}

func (s Service) Install(ctx context.Context, release Release) error {
	goos := s.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := s.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	archiveAsset, checksumAsset, err := release.SelectAssets(goos, goarch)
	if err != nil {
		return err
	}
	checksumContent, err := s.Client.Download(ctx, checksumAsset.DownloadURL)
	if err != nil {
		return err
	}
	checksums, err := ParseChecksums(checksumContent)
	if err != nil {
		return err
	}
	archiveContent, err := s.Client.Download(ctx, archiveAsset.DownloadURL)
	if err != nil {
		return err
	}
	if err := VerifyArchiveChecksum(archiveAsset.Name, archiveContent, checksums); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "repobridge-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	extracted, err := ExtractArchive(archiveAsset.Name, archiveContent, tmp)
	if err != nil {
		return err
	}
	executablePath := os.Executable
	if s.ExecutablePath != nil {
		executablePath = s.ExecutablePath
	}
	current, err := executablePath()
	if err != nil {
		return err
	}
	if current == "" {
		return fmt.Errorf("could not locate current executable")
	}
	return InstallExtractedRelease(extracted, current, goos)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}
```

- [ ] **Step 4: Run service tests**

Run: `go test ./internal/updatecheck -run 'TestService'`

Expected: PASS.

- [ ] **Step 5: Run all updatecheck tests**

Run: `go test ./internal/updatecheck`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/updatecheck/service.go internal/updatecheck/service_test.go
git commit -m "Add update check service"
```

---

### Task 6: CLI Self-Update Command

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/commands.go`
- Test: `internal/cli/commands_test.go`

- [ ] **Step 1: Write failing CLI tests for `self-update --check-only`**

Append to `internal/cli/commands_test.go`:

```go
func TestSelfUpdateCheckOnlyReportsAvailableUpdate(t *testing.T) {
	hook := &fakeUpdateChecker{result: updatecheck.CheckResult{
		Available:      true,
		CurrentVersion: "v0.10.4",
		LatestVersion:  "v0.10.5",
		ReleaseURL:     "https://example.test/v0.10.5",
	}}
	stdout, stderr, err := executeForTestWithOptions(Options{
		Version:       "v0.10.4",
		UpdateChecker: hook,
	}, "self-update", "--check-only")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "RepoBridge v0.10.5 is available") {
		t.Fatalf("stdout = %q, want update available", stdout)
	}
}

func TestSelfUpdateCheckOnlyReportsCurrentVersion(t *testing.T) {
	hook := &fakeUpdateChecker{result: updatecheck.CheckResult{
		Available:      false,
		CurrentVersion: "v0.10.4",
		LatestVersion:  "v0.10.4",
	}}
	stdout, _, err := executeForTestWithOptions(Options{
		Version:       "v0.10.4",
		UpdateChecker: hook,
	}, "self-update", "--check-only")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "RepoBridge v0.10.4 is current") {
		t.Fatalf("stdout = %q, want current version", stdout)
	}
}
```

Add imports to `internal/cli/commands_test.go`:

```go
import "context"
import "repobridge/internal/updatecheck"
```

Add fake type near other fakes:

```go
type fakeUpdateChecker struct {
	result       updatecheck.CheckResult
	checkErr     error
	installErr   error
	checkCalls    int
	installCalls  int
	opportunistic int
}

func (f *fakeUpdateChecker) Check(ctx context.Context) (updatecheck.CheckResult, error) {
	f.checkCalls++
	return f.result, f.checkErr
}

func (f *fakeUpdateChecker) OpportunisticCheck(ctx context.Context) (updatecheck.CheckResult, error) {
	f.opportunistic++
	return f.result, f.checkErr
}

func (f *fakeUpdateChecker) Install(ctx context.Context, release updatecheck.Release) error {
	f.installCalls++
	return f.installErr
}
```

Modify `executeForTestWithOptions` so tests can pass an explicit release version:

```go
func executeForTestWithOptions(opts Options, args ...string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if opts.Version == "" {
		opts.Version = "test-version"
	}
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if opts.Indexer == nil {
		opts.Indexer = &fakeIndexer{}
	}
	cmd := NewRootCommand(opts)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli -run 'TestSelfUpdateCheckOnly'`

Expected: FAIL because `Options.UpdateChecker` and `self-update` do not exist.

- [ ] **Step 3: Extend CLI options and interfaces**

Modify `internal/cli/root.go` imports to add:

```go
import (
	"context"
	"repobridge/internal/updatecheck"
)
```

Add to `Options`:

```go
UpdateChecker UpdateChecker
```

Add interface below `IndexScheduler`:

```go
type UpdateChecker interface {
	Check(context.Context) (updatecheck.CheckResult, error)
	OpportunisticCheck(context.Context) (updatecheck.CheckResult, error)
	Install(context.Context, updatecheck.Release) error
}
```

Add helper:

```go
func updateCheckerForOptions(opts Options) UpdateChecker {
	if opts.UpdateChecker != nil {
		return opts.UpdateChecker
	}
	return updatecheck.Service{CurrentVersion: opts.Version}
}
```

- [ ] **Step 4: Add command implementation**

Modify `NewRootCommand` in `internal/cli/root.go`:

```go
cmd.AddCommand(newSelfUpdateCommand(opts))
```

Add this function to `internal/cli/commands.go` near `newInstallAgentCommand`:

```go
func newSelfUpdateCommand(opts Options) *cobra.Command {
	var checkOnly bool
	var yes bool
	var force bool
	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Check for and install the latest RepoBridge release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			checker := updateCheckerForOptions(opts)
			result, err := checker.Check(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if checkOnly {
				if result.Available {
					fmt.Fprintf(out, "RepoBridge %s is available (current %s)\n%s\n", result.LatestVersion, result.CurrentVersion, result.ReleaseURL)
				} else {
					current := result.CurrentVersion
					if current == "" {
						current = opts.Version
					}
					fmt.Fprintf(out, "RepoBridge %s is current\n", current)
				}
				return nil
			}
			if !result.Available && !force {
				fmt.Fprintf(out, "RepoBridge %s is current\n", result.CurrentVersion)
				return nil
			}
			if !yes {
				return fmt.Errorf("self-update requires --yes for non-interactive installation")
			}
			if err := checker.Install(cmd.Context(), result.Release); err != nil {
				return err
			}
			fmt.Fprintf(out, "Updated RepoBridge to %s\n", result.LatestVersion)
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check-only", false, "check for updates without installing")
	cmd.Flags().BoolVar(&yes, "yes", false, "install without prompting")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall the latest release even if current")
	return cmd
}
```

- [ ] **Step 5: Run command tests**

Run: `go test ./internal/cli -run 'TestSelfUpdateCheckOnly'`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/root.go internal/cli/commands.go internal/cli/commands_test.go
git commit -m "Add self-update command"
```

---

### Task 7: Opportunistic Update Hook For All Normal Commands

**Files:**
- Modify: `internal/cli/root.go`
- Test: `internal/cli/commands_test.go`

- [ ] **Step 1: Write failing hook tests**

Append to `internal/cli/commands_test.go`:

```go
func TestUpdateCheckRunsForNormalCommand(t *testing.T) {
	withHome(t)
	hook := &fakeUpdateChecker{}
	_, _, err := executeForTestWithOptions(Options{
		Version:       "v0.10.4",
		UpdateChecker: hook,
	}, "list")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if hook.opportunistic != 1 {
		t.Fatalf("opportunistic checks = %d, want 1", hook.opportunistic)
	}
}

func TestUpdateCheckSkipsVersionCommand(t *testing.T) {
	hook := &fakeUpdateChecker{}
	stdout, _, err := executeForTestWithOptions(Options{
		Version:       "v0.10.4",
		UpdateChecker: hook,
	}, "--version")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "v0.10.4") {
		t.Fatalf("stdout = %q, want version", stdout)
	}
	if hook.opportunistic != 0 {
		t.Fatalf("opportunistic checks = %d, want 0", hook.opportunistic)
	}
}

func TestUpdateCheckSkipsSelfUpdateCommand(t *testing.T) {
	hook := &fakeUpdateChecker{}
	_, _, err := executeForTestWithOptions(Options{
		Version:       "v0.10.4",
		UpdateChecker: hook,
	}, "self-update", "--check-only")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if hook.opportunistic != 0 {
		t.Fatalf("opportunistic checks = %d, want 0", hook.opportunistic)
	}
}

func TestUpdateCheckSkipsWhenDisabledByEnv(t *testing.T) {
	t.Setenv("REPOBRIDGE_NO_UPDATE_CHECK", "1")
	hook := &fakeUpdateChecker{}
	_, _, err := executeForTestWithOptions(Options{
		Version:       "v0.10.4",
		UpdateChecker: hook,
	}, "list")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if hook.opportunistic != 0 {
		t.Fatalf("opportunistic checks = %d, want 0", hook.opportunistic)
	}
}

func TestUpdateCheckHintGoesToStderrForNonInteractiveCalls(t *testing.T) {
	withHome(t)
	hook := &fakeUpdateChecker{result: updatecheck.CheckResult{
		Available:      true,
		CurrentVersion: "v0.10.4",
		LatestVersion:  "v0.10.5",
	}}
	_, stderr, err := executeForTestWithOptions(Options{
		Version:       "v0.10.4",
		UpdateChecker: hook,
	}, "list")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stderr, "RepoBridge v0.10.5 is available; run `repobridge self-update`") {
		t.Fatalf("stderr = %q, want update hint", stderr)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli -run 'TestUpdateCheck'`

Expected: FAIL because no persistent update hook exists.

- [ ] **Step 3: Implement skip rules and hook**

Modify `internal/cli/root.go` imports to include:

```go
import "context"
```

Add to `NewRootCommand` before adding child commands:

```go
cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
	return maybeRunUpdateCheck(cmd, opts)
}
```

Add helper functions in `internal/cli/root.go`:

```go
func maybeRunUpdateCheck(cmd *cobra.Command, opts Options) error {
	if shouldSkipUpdateCheck(cmd, opts.Version) {
		return nil
	}
	result, err := updateCheckerForOptions(opts).OpportunisticCheck(context.Background())
	if err != nil {
		return nil
	}
	if result.Available {
		fmt.Fprintf(cmd.ErrOrStderr(), "RepoBridge %s is available; run `repobridge self-update`\n", result.LatestVersion)
	}
	return nil
}

func shouldSkipUpdateCheck(cmd *cobra.Command, version string) bool {
	if os.Getenv("REPOBRIDGE_NO_UPDATE_CHECK") == "1" {
		return true
	}
	if !updatecheck.IsReleaseVersion(version) {
		return true
	}
	if cmd.CalledAs() == "self-update" || cmd.CommandPath() == "repobridge self-update" {
		return true
	}
	if strings.HasPrefix(cmd.Name(), "__") {
		return true
	}
	return false
}
```

`repobridge --version` is handled by Cobra before command `RunE`, so `PersistentPreRunE` is not invoked for that path. The `TestUpdateCheckSkipsVersionCommand` test proves the skip behavior.

- [ ] **Step 4: Run hook tests**

Run: `go test ./internal/cli -run 'TestUpdateCheck'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/root.go internal/cli/commands_test.go
git commit -m "Run safe update checks for normal commands"
```

---

### Task 8: Interactive Prompt Support

**Files:**
- Modify: `internal/cli/root.go`
- Test: `internal/cli/commands_test.go`

- [ ] **Step 1: Write failing prompt tests**

Extend `Options` in the test expectation to support injection:

```go
func TestInteractiveUpdatePromptInstallsWhenAccepted(t *testing.T) {
	hook := &fakeUpdateChecker{result: updatecheck.CheckResult{
		Available:      true,
		CurrentVersion: "v0.10.4",
		LatestVersion:  "v0.10.5",
		Release:        updatecheck.Release{TagName: "v0.10.5"},
	}}
	stdout, stderr, err := executeForTestWithOptions(Options{
		Version:       "v0.10.4",
		UpdateChecker: hook,
		Interactive:   func() bool { return true },
		Prompt:        func(message string) bool { return true },
	}, "list")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if hook.installCalls != 1 {
		t.Fatalf("install calls = %d, want 1", hook.installCalls)
	}
	if stdout == "" && stderr == "" {
		t.Fatal("expected normal command output or update output")
	}
}

func TestInteractiveUpdatePromptDeclineContinuesCommand(t *testing.T) {
	hook := &fakeUpdateChecker{result: updatecheck.CheckResult{
		Available:      true,
		CurrentVersion: "v0.10.4",
		LatestVersion:  "v0.10.5",
	}}
	_, _, err := executeForTestWithOptions(Options{
		Version:       "v0.10.4",
		UpdateChecker: hook,
		Interactive:   func() bool { return true },
		Prompt:        func(message string) bool { return false },
	}, "list")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if hook.installCalls != 0 {
		t.Fatalf("install calls = %d, want 0", hook.installCalls)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli -run 'TestInteractiveUpdatePrompt'`

Expected: FAIL because `Options.Interactive` and `Options.Prompt` do not exist.

- [ ] **Step 3: Add injected interactivity to CLI options**

Modify `Options` in `internal/cli/root.go`:

```go
Interactive func() bool
Prompt      func(message string) bool
```

Add helpers:

```go
func isInteractive(opts Options) bool {
	if opts.Interactive != nil {
		return opts.Interactive()
	}
	stdin, stdinOK := os.Stdin.Stat()
	stderr, stderrOK := os.Stderr.Stat()
	return stdinOK == nil && stderrOK == nil &&
		(stdin.Mode()&os.ModeCharDevice) != 0 &&
		(stderr.Mode()&os.ModeCharDevice) != 0
}

func promptYesNo(opts Options, message string) bool {
	if opts.Prompt != nil {
		return opts.Prompt(message)
	}
	fmt.Fprint(os.Stderr, message+" ")
	var answer string
	if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes" || answer == "j" || answer == "ja"
}
```

Modify `maybeRunUpdateCheck`:

```go
if result.Available {
	if isInteractive(opts) {
		message := fmt.Sprintf("RepoBridge %s is available. Update from %s now? [y/N]", result.LatestVersion, result.CurrentVersion)
		if promptYesNo(opts, message) {
			if err := updateCheckerForOptions(opts).Install(context.Background(), result.Release); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "RepoBridge update failed: %v\n", err)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "Updated RepoBridge to %s\n", result.LatestVersion)
			}
		}
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "RepoBridge %s is available; run `repobridge self-update`\n", result.LatestVersion)
}
```

- [ ] **Step 4: Run prompt tests**

Run: `go test ./internal/cli -run 'TestInteractiveUpdatePrompt|TestUpdateCheckHint'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/root.go internal/cli/commands_test.go
git commit -m "Prompt interactive users for updates"
```

---

### Task 9: README Documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add self-update documentation**

Add near the usage section that contains `--version`:

```markdown
### Updates

RepoBridge checks for newer GitHub releases before normal commands, at most once every 24 hours. Interactive terminal users are asked before an update is installed. Non-interactive callers such as scripts, CI jobs, LLM tools, and coding agents are never prompted; they may see a short stderr hint instead.

Disable automatic checks:

```bash
REPOBRIDGE_NO_UPDATE_CHECK=1 repobridge search project:. "kind:function"
```

Check or install explicitly:

```bash
repobridge self-update --check-only
repobridge self-update --yes
```
```

Add `self-update` to the command table:

```markdown
| `repobridge self-update` | Checks for and installs the latest RepoBridge release. |
```

Add flags to the flags table:

```markdown
| `self-update` | `--check-only`, `--yes`, `--force`. |
```

- [ ] **Step 2: Run README grep check**

Run: `rg -n "self-update|REPOBRIDGE_NO_UPDATE_CHECK|--check-only" README.md`

Expected: Output includes the command table, update section, and flags table.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Document self-update behavior"
```

---

### Task 10: Full Verification

**Files:**
- No new files unless previous tasks reveal compile issues.

- [ ] **Step 1: Format Go files**

Run:

```bash
gofmt -w ./cmd ./internal
```

Expected: Command exits 0.

- [ ] **Step 2: Run updatecheck package tests**

Run:

```bash
go test ./internal/updatecheck
```

Expected: PASS.

- [ ] **Step 3: Run CLI tests**

Run:

```bash
go test ./internal/cli
```

Expected: PASS.

- [ ] **Step 4: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Run vet**

Run:

```bash
go vet ./...
```

Expected: exit 0.

- [ ] **Step 6: Build the CLI**

Run:

```bash
go build -o ./bin/repobridge ./cmd/repobridge
```

Expected: exit 0 and `./bin/repobridge` exists.

- [ ] **Step 7: Manual smoke test with update checks disabled**

Run:

```bash
REPOBRIDGE_NO_UPDATE_CHECK=1 ./bin/repobridge --version
```

Expected: Prints the version and no update-check prompt.

- [ ] **Step 8: Commit verification fixes after checking the diff**

Run:

```bash
git status --short
```

If the output lists modified files from formatting or compile fixes, commit them:

```bash
git add .
git commit -m "Polish self-update implementation"
```

If `git status --short` prints nothing, do not create an empty commit.

---

## Self-Review Notes

- Spec coverage: tasks cover normal-command checks, non-interactive behavior, explicit `self-update`, checksums, archive extraction, state cache, env opt-out, and documentation.
- No real GitHub calls are used in tests; all release responses use `httptest`.
- The implementation deliberately avoids new dependencies by using `os.FileInfo.Mode()&os.ModeCharDevice` for TTY detection.
- The plan keeps update logic out of Cobra command code except for prompts and command wiring.
