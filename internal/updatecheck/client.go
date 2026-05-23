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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/repos/agentsw0rk/repobridge/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient().Do(req)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download returned HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
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
	checksums := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid checksum line %q", line)
		}
		hash := strings.ToLower(fields[0])
		if !isSHA256(hash) {
			return nil, fmt.Errorf("invalid sha256 checksum for %s", fields[1])
		}
		checksums[fields[1]] = hash
	}
	if len(checksums) == 0 {
		return nil, fmt.Errorf("no checksums found")
	}
	return checksums, nil
}

func isSHA256(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, r := range hash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
