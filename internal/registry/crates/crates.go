package crates

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

const DefaultAPI = "https://crates.io/api/v1"

type crateResponse struct {
	Crate crateInfo `json:"crate"`
}

type crateInfo struct {
	MaxVersion string  `json:"max_version"`
	Repository string  `json:"repository"`
	Homepage   *string `json:"homepage"`
}

func Resolve(name, version string, client *http.Client, baseURL string) (registry.ResolvedPackage, error) {
	if client == nil {
		client = httpx.NewClient()
	}
	if baseURL == "" {
		baseURL = DefaultAPI
	}
	baseURL = strings.TrimRight(baseURL, "/")
	escapedName := url.PathEscape(name)
	resp, err := client.Get(baseURL + "/crates/" + escapedName)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return registry.ResolvedPackage{}, repobridge.PackageNotFoundError{Name: name, Registry: "crates.io"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return registry.ResolvedPackage{}, repobridge.HTTPStatusError{Context: "crate info", Status: resp.Status}
	}
	var data crateResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return registry.ResolvedPackage{}, err
	}
	resolvedVersion := version
	if resolvedVersion == "" {
		resolvedVersion = data.Crate.MaxVersion
	} else if err := verifyVersion(client, baseURL, name, version); err != nil {
		return registry.ResolvedPackage{}, err
	}
	repoURL := normalizeRepoURL(data.Crate.Repository)
	if repoURL == "" && data.Crate.Homepage != nil {
		repoURL = normalizeRepoURL(*data.Crate.Homepage)
	}
	if repoURL == "" {
		return registry.ResolvedPackage{}, repobridge.NoRepoURLError{Message: fmt.Sprintf("No repository URL found for %q. This crate may not have its source published.", name+"@"+resolvedVersion)}
	}
	return registry.ResolvedPackage{Registry: registry.Crates, Name: name, Version: resolvedVersion, RepoURL: repoURL, GitTag: "v" + resolvedVersion}, nil
}

func verifyVersion(client *http.Client, baseURL, name, version string) error {
	resp, err := client.Get(baseURL + "/crates/" + url.PathEscape(name) + "/" + url.PathEscape(version))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return repobridge.VersionNotFoundError{Message: fmt.Sprintf("Version %q not found for crate %q", version, name)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return repobridge.HTTPStatusError{Context: "crate version info", Status: resp.Status}
	}
	return nil
}

func normalizeRepoURL(value string) string {
	return registry.NormalizeRepoURL(value)
}
