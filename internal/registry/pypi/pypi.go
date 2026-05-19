package pypi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"repobridge/internal/httpx"
	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

const DefaultAPI = "https://pypi.org/pypi"

type response struct {
	Info info `json:"info"`
}

type info struct {
	Version     string            `json:"version"`
	HomePage    string            `json:"home_page"`
	ProjectURLs map[string]string `json:"project_urls"`
}

func Resolve(name, version string, client *http.Client, baseURL string) (registry.ResolvedPackage, error) {
	if client == nil {
		client = httpx.NewClient()
	}
	if baseURL == "" {
		baseURL = DefaultAPI
	}
	target := strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(name)
	if version != "" {
		target += "/" + url.PathEscape(version)
	}
	target += "/json"
	resp, err := client.Get(target)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return registry.ResolvedPackage{}, repobridge.PackageNotFoundError{Name: name, Registry: "PyPI"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return registry.ResolvedPackage{}, repobridge.HTTPStatusError{Context: "package info", Status: resp.Status}
	}
	var data response
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return registry.ResolvedPackage{}, err
	}
	repoURL := extractRepoURL(data.Info)
	if repoURL == "" {
		return registry.ResolvedPackage{}, repobridge.NoRepoURLError{Message: fmt.Sprintf("No repository URL found for %q. This package may not have its source published.", name+"@"+data.Info.Version)}
	}
	return registry.ResolvedPackage{Registry: registry.PyPI, Name: name, Version: data.Info.Version, RepoURL: repoURL, GitTag: "v" + data.Info.Version}, nil
}

func extractRepoURL(info info) string {
	keys := []string{"Source", "Source Code", "Repository", "GitHub", "Code", "Homepage"}
	for _, key := range keys {
		if value := info.ProjectURLs[key]; isGitRepoURL(value) {
			return normalizeRepoURL(value)
		}
	}
	if isGitRepoURL(info.HomePage) {
		return normalizeRepoURL(info.HomePage)
	}
	for _, value := range info.ProjectURLs {
		if isGitRepoURL(value) {
			return normalizeRepoURL(value)
		}
	}
	return ""
}

func isGitRepoURL(value string) bool {
	return registry.NormalizeRepoURL(value) != ""
}

func normalizeRepoURL(value string) string {
	return registry.NormalizeRepoURL(value)
}
