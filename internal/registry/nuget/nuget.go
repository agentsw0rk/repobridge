package nuget

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"repobridge/internal/httpx"
	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

const (
	DefaultServiceIndexURL = "https://api.nuget.org/v3/index.json"
	maxNupkgBytes          = 256 * 1024 * 1024
	maxNuspecBytes         = 2 * 1024 * 1024
)

type serviceIndex struct {
	Resources []serviceResource `json:"resources"`
}

type serviceResource struct {
	ID   string          `json:"@id"`
	Type json.RawMessage `json:"@type"`
}

type versionsIndex struct {
	Versions []string `json:"versions"`
}

type nuspecDocument struct {
	Metadata nuspecMetadata `xml:"metadata"`
}

type nuspecMetadata struct {
	ID         string           `xml:"id"`
	Version    string           `xml:"version"`
	Repository nuspecRepository `xml:"repository"`
}

type nuspecRepository struct {
	Type   string `xml:"type,attr"`
	URL    string `xml:"url,attr"`
	Commit string `xml:"commit,attr"`
	Branch string `xml:"branch,attr"`
}

func Resolve(name, version string, client *http.Client, serviceIndexURL string) (registry.ResolvedPackage, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if name == "" {
		return registry.ResolvedPackage{}, fmt.Errorf("NuGet package name must not be empty")
	}
	if hasUnsafePathComponent(name) || hasUnsafePathComponent(version) {
		return registry.ResolvedPackage{}, fmt.Errorf("NuGet package name and version must not contain path separators or '..'")
	}
	if client == nil {
		client = httpx.NewClient()
	}
	if serviceIndexURL == "" {
		serviceIndexURL = DefaultServiceIndexURL
	}

	flatBaseURL, err := loadFlatContainerBaseURL(client, serviceIndexURL)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}

	versions, err := loadVersions(client, flatBaseURL, name)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	resolvedVersion, err := resolveVersion(name, version, versions)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}

	nupkgURL, err := flatContainerURL(flatBaseURL, name, resolvedVersion, strings.ToLower(name)+"."+strings.ToLower(resolvedVersion)+".nupkg")
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	nupkgPath, err := downloadNupkg(client, nupkgURL)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	defer os.Remove(nupkgPath)

	metadata, err := readNuspecMetadata(nupkgPath)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}
	repoURL := normalizeRepositoryURL(metadata.Repository)
	if repoURL == "" {
		return registry.ResolvedPackage{}, repobridge.NoRepoURLError{
			Message: fmt.Sprintf("No supported git repository URL found for %q. This package may not have its source published.", name+"@"+resolvedVersion),
		}
	}

	resolved := registry.ResolvedPackage{
		Registry: registry.NuGet,
		Name:     chooseCanonical(metadata.ID, name),
		Version:  chooseCanonical(metadata.Version, resolvedVersion),
		RepoURL:  repoURL,
	}
	if strings.TrimSpace(metadata.Repository.Commit) != "" {
		resolved.GitRef = strings.TrimSpace(metadata.Repository.Commit)
	} else {
		resolved.GitTag = "v" + resolved.Version
	}
	return resolved, nil
}

func loadFlatContainerBaseURL(client *http.Client, serviceIndexURL string) (string, error) {
	resp, err := client.Get(serviceIndexURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", repobridge.HTTPStatusError{Context: "NuGet service index", Status: resp.Status}
	}

	var index serviceIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return "", err
	}
	for _, resource := range index.Resources {
		if resource.ID != "" && resource.hasType("PackageBaseAddress/3.0.0") {
			return resource.ID, nil
		}
	}
	return "", fmt.Errorf("NuGet service index missing PackageBaseAddress/3.0.0 resource")
}

func (r serviceResource) hasType(want string) bool {
	var single string
	if err := json.Unmarshal(r.Type, &single); err == nil {
		return strings.EqualFold(single, want)
	}
	var many []string
	if err := json.Unmarshal(r.Type, &many); err == nil {
		for _, item := range many {
			if strings.EqualFold(item, want) {
				return true
			}
		}
	}
	return false
}

func loadVersions(client *http.Client, flatBaseURL, name string) ([]string, error) {
	versionsURL, err := flatContainerURL(flatBaseURL, name, "index.json")
	if err != nil {
		return nil, err
	}
	resp, err := client.Get(versionsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, repobridge.PackageNotFoundError{Name: name, Registry: "NuGet"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, repobridge.HTTPStatusError{Context: "NuGet package versions", Status: resp.Status}
	}

	var index versionsIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, err
	}
	return index.Versions, nil
}

func resolveVersion(name, requested string, versions []string) (string, error) {
	if requested != "" {
		for _, version := range versions {
			if strings.EqualFold(version, requested) {
				return version, nil
			}
		}
		return "", repobridge.VersionNotFoundError{Message: fmt.Sprintf("Version %q not found for NuGet package %q", requested, name)}
	}

	latest := ""
	for _, version := range versions {
		if strings.Contains(version, "-") {
			continue
		}
		if latest == "" || compareVersions(version, latest) > 0 {
			latest = version
		}
	}
	if latest == "" {
		return "", repobridge.VersionNotFoundError{Message: fmt.Sprintf("No stable version found for NuGet package %q", name)}
	}
	return latest, nil
}

func flatContainerURL(flatBaseURL, name string, parts ...string) (string, error) {
	segments := []string{strings.ToLower(name)}
	for _, part := range parts {
		segments = append(segments, strings.ToLower(part))
	}
	u, err := url.Parse(flatBaseURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid NuGet flat-container URL %q", flatBaseURL)
	}
	all := append([]string{strings.TrimRight(u.Path, "/")}, segments...)
	u.Path = path.Join(all...)
	return u.String(), nil
}

func downloadNupkg(client *http.Client, nupkgURL string) (string, error) {
	resp, err := client.Get(nupkgURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", repobridge.VersionNotFoundError{Message: fmt.Sprintf("NuGet package content not found at %s", nupkgURL)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", repobridge.HTTPStatusError{Context: "NuGet package content", Status: resp.Status}
	}
	if resp.ContentLength > maxNupkgBytes {
		return "", fmt.Errorf("NuGet package exceeds maximum size of %d bytes", maxNupkgBytes)
	}

	tmp, err := os.CreateTemp("", "repobridge-*.nupkg")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		tmp.Close()
		if !keep {
			os.Remove(tmp.Name())
		}
	}()

	written, err := io.Copy(tmp, io.LimitReader(resp.Body, maxNupkgBytes+1))
	if err != nil {
		return "", err
	}
	if written > maxNupkgBytes {
		return "", fmt.Errorf("NuGet package exceeds maximum size of %d bytes", maxNupkgBytes)
	}
	keep = true
	return tmp.Name(), nil
}

func readNuspecMetadata(nupkgPath string) (nuspecMetadata, error) {
	zr, err := zip.OpenReader(nupkgPath)
	if err != nil {
		return nuspecMetadata{}, err
	}
	defer zr.Close()

	for _, file := range zr.File {
		isNuspec := strings.HasSuffix(strings.ToLower(file.Name), ".nuspec")
		if unsafeZipPath(file.Name) {
			if isNuspec {
				return nuspecMetadata{}, fmt.Errorf("unsafe .nuspec path %q in NuGet package", file.Name)
			}
			return nuspecMetadata{}, fmt.Errorf("unsafe ZIP path %q in NuGet package", file.Name)
		}
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			if isNuspec {
				return nuspecMetadata{}, fmt.Errorf("unsafe .nuspec symlink %q in NuGet package", file.Name)
			}
			return nuspecMetadata{}, fmt.Errorf("unsafe ZIP symlink %q in NuGet package", file.Name)
		}
		if !isNuspec {
			continue
		}
		if file.UncompressedSize64 > maxNuspecBytes {
			return nuspecMetadata{}, fmt.Errorf("NuGet .nuspec exceeds maximum size of %d bytes", maxNuspecBytes)
		}
		rc, err := file.Open()
		if err != nil {
			return nuspecMetadata{}, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxNuspecBytes+1))
		closeErr := rc.Close()
		if err != nil {
			return nuspecMetadata{}, err
		}
		if closeErr != nil {
			return nuspecMetadata{}, closeErr
		}
		if len(data) > maxNuspecBytes {
			return nuspecMetadata{}, fmt.Errorf("NuGet .nuspec exceeds maximum size of %d bytes", maxNuspecBytes)
		}

		var pkg nuspecDocument
		if err := xml.NewDecoder(bytes.NewReader(data)).Decode(&pkg); err != nil {
			return nuspecMetadata{}, err
		}
		return pkg.Metadata, nil
	}
	return nuspecMetadata{}, fmt.Errorf("NuGet package is missing .nuspec metadata")
}

func normalizeRepositoryURL(repo nuspecRepository) string {
	if !strings.EqualFold(strings.TrimSpace(repo.Type), "git") {
		return ""
	}
	value := strings.TrimSpace(repo.URL)
	value = strings.TrimPrefix(value, "git+")
	value = strings.TrimSuffix(value, ".git")
	normalized := registry.NormalizeRepoURL(value)
	if strings.HasPrefix(normalized, "https://github.com/") || strings.HasPrefix(normalized, "http://github.com/") {
		return strings.ToLower(normalized)
	}
	return normalized
}

func compareVersions(a, b string) int {
	ap := versionParts(a)
	bp := versionParts(b)
	max := len(ap)
	if len(bp) > max {
		max = len(bp)
	}
	for i := 0; i < max; i++ {
		var av, bv int
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return strings.Compare(a, b)
}

func versionParts(version string) []int {
	segments := strings.Split(version, ".")
	parts := make([]int, 0, len(segments))
	for _, segment := range segments {
		digits := segment
		for i, r := range segment {
			if r < '0' || r > '9' {
				digits = segment[:i]
				break
			}
		}
		value, err := strconv.Atoi(digits)
		if err != nil {
			value = 0
		}
		parts = append(parts, value)
	}
	return parts
}

func chooseCanonical(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func hasUnsafePathComponent(value string) bool {
	return strings.Contains(value, "/") || strings.Contains(value, `\`) || strings.Contains(value, "..")
}

func unsafeZipPath(name string) bool {
	cleaned := path.Clean(name)
	return path.IsAbs(name) || cleaned == "." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../")
}
