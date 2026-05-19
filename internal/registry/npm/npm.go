package npm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"repobridge/internal/httpx"
	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

const DefaultRegistry = "https://registry.npmjs.org"

type npmRepository struct {
	URL       string `json:"url"`
	Directory string `json:"directory"`
}

func (r *npmRepository) UnmarshalJSON(data []byte) error {
	var rawURL string
	if err := json.Unmarshal(data, &rawURL); err == nil {
		r.URL = rawURL
		r.Directory = ""
		return nil
	}
	type repository npmRepository
	var parsed repository
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*r = npmRepository(parsed)
	return nil
}

type npmVersionInfo struct {
	Repository *npmRepository `json:"repository"`
}

type npmResponse struct {
	DistTags   map[string]string         `json:"dist-tags"`
	Versions   map[string]npmVersionInfo `json:"versions"`
	Repository *npmRepository            `json:"repository"`
}

func Resolve(name, version string, client *http.Client, baseURL string) (registry.ResolvedPackage, error) {
	if client == nil {
		client = httpx.NewClient()
	}
	if baseURL == "" {
		baseURL = DefaultRegistry
	}
	resp, err := client.Get(registryURL(baseURL, name))
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return registry.ResolvedPackage{}, repobridge.PackageNotFoundError{Name: name, Registry: "npm"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return registry.ResolvedPackage{}, repobridge.HTTPStatusError{Context: "package info", Status: resp.Status}
	}
	var data npmResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return registry.ResolvedPackage{}, err
	}
	resolvedVersion := version
	if resolvedVersion == "" {
		resolvedVersion = data.DistTags["latest"]
	}
	if resolvedVersion == "" {
		return registry.ResolvedPackage{}, repobridge.VersionNotFoundError{Message: fmt.Sprintf("No latest version found for %q", name)}
	}
	versionInfo, ok := data.Versions[resolvedVersion]
	if !ok {
		keys := make([]string, 0, len(data.Versions))
		for key := range data.Versions {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 5 {
			keys = keys[len(keys)-5:]
		}
		return registry.ResolvedPackage{}, repobridge.VersionNotFoundError{Message: fmt.Sprintf("Version %q not found for %q. Recent versions: %s", resolvedVersion, name, strings.Join(keys, ", "))}
	}
	repoURL, directory := extractRepoURL(data.Repository, versionInfo.Repository)
	if repoURL == "" {
		return registry.ResolvedPackage{}, repobridge.NoRepoURLError{Message: fmt.Sprintf("No repository URL found for %q. This package may not have its source published.", name+"@"+resolvedVersion)}
	}
	return registry.ResolvedPackage{Registry: registry.NPM, Name: name, Version: resolvedVersion, RepoURL: repoURL, RepoDirectory: directory, GitTag: "v" + resolvedVersion}, nil
}

func registryURL(baseURL, name string) string {
	return strings.TrimRight(baseURL, "/") + "/" + url.QueryEscape(name)
}

func extractRepoURL(top, version *npmRepository) (string, string) {
	repo := top
	if version != nil {
		repo = version
	}
	if repo == nil || repo.URL == "" {
		return "", ""
	}
	raw := repo.URL
	raw = strings.TrimPrefix(raw, "git+")
	raw = strings.Replace(raw, "git://", "https://", 1)
	raw = strings.Replace(raw, "git+ssh://git@", "https://", 1)
	raw = strings.Replace(raw, "ssh://git@", "https://", 1)
	raw = strings.TrimSuffix(raw, ".git")
	if suffix, ok := strings.CutPrefix(raw, "github:"); ok {
		raw = "https://github.com/" + suffix
	}
	return registry.NormalizeRepoURL(raw), repo.Directory
}
