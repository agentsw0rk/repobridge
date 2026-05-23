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

func TestClientLatestReleaseRejectsMissingTagName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"assets":[]}`))
	}))
	defer server.Close()

	client := Client{HTTPClient: server.Client(), BaseURL: server.URL}
	if _, err := client.LatestRelease(t.Context()); err == nil {
		t.Fatal("LatestRelease() error = nil, want error")
	}
}

func TestClientLatestReleaseRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := Client{HTTPClient: server.Client(), BaseURL: server.URL}
	if _, err := client.LatestRelease(t.Context()); err == nil {
		t.Fatal("LatestRelease() error = nil, want error")
	}
}

func TestClientDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("archive bytes"))
	}))
	defer server.Close()

	client := Client{HTTPClient: server.Client()}
	got, err := client.Download(t.Context(), server.URL+"/archive")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if string(got) != "archive bytes" {
		t.Fatalf("Download() = %q", got)
	}
}

func TestClientDownloadRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := Client{HTTPClient: server.Client()}
	if _, err := client.Download(t.Context(), server.URL+"/missing"); err == nil {
		t.Fatal("Download() error = nil, want error")
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

func TestSelectPlatformAssetUsesZipForWindows(t *testing.T) {
	release := Release{TagName: "v0.10.5", Assets: []Asset{
		{Name: "checksums.txt", DownloadURL: "https://example.test/checksums.txt"},
		{Name: "repobridge_v0.10.5_windows_amd64.zip", DownloadURL: "https://example.test/windows"},
	}}
	asset, _, err := release.SelectAssets("windows", "amd64")
	if err != nil {
		t.Fatalf("SelectAssets() error = %v", err)
	}
	if asset.Name != "repobridge_v0.10.5_windows_amd64.zip" {
		t.Fatalf("asset = %q", asset.Name)
	}
}

func TestSelectPlatformAssetRequiresChecksums(t *testing.T) {
	release := Release{TagName: "v0.10.5", Assets: []Asset{
		{Name: "repobridge_v0.10.5_linux_amd64.tar.gz", DownloadURL: "https://example.test/linux"},
	}}
	if _, _, err := release.SelectAssets("linux", "amd64"); err == nil {
		t.Fatal("SelectAssets() error = nil, want error")
	}
}

func TestParseChecksums(t *testing.T) {
	content := strings.Join([]string{
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA  repobridge_v0.10.5_linux_amd64.tar.gz",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  repobridge_v0.10.5_windows_amd64.zip",
	}, "\n")
	checksums, err := ParseChecksums([]byte(content))
	if err != nil {
		t.Fatalf("ParseChecksums() error = %v", err)
	}
	if checksums["repobridge_v0.10.5_linux_amd64.tar.gz"] != strings.Repeat("a", 64) {
		t.Fatalf("checksums = %#v", checksums)
	}
	if checksums["repobridge_v0.10.5_windows_amd64.zip"] != strings.Repeat("b", 64) {
		t.Fatalf("checksums = %#v", checksums)
	}
}

func TestParseChecksumsRejectsInvalidHash(t *testing.T) {
	content := "notasha  repobridge_v0.10.5_linux_amd64.tar.gz"
	if _, err := ParseChecksums([]byte(content)); err == nil {
		t.Fatal("ParseChecksums() error = nil, want error")
	}
}

func TestParseChecksumsRejectsInvalidHashCharacters(t *testing.T) {
	content := strings.Repeat("z", 64) + "  repobridge_v0.10.5_linux_amd64.tar.gz"
	if _, err := ParseChecksums([]byte(content)); err == nil {
		t.Fatal("ParseChecksums() error = nil, want error")
	}
}

func TestParseChecksumsRejectsNoChecksums(t *testing.T) {
	if _, err := ParseChecksums([]byte("\n")); err == nil {
		t.Fatal("ParseChecksums() error = nil, want error")
	}
}
