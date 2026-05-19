package maven

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"repobridge/internal/httpx"
	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

const DefaultRepository = "https://repo1.maven.org/maven2"

type Coordinates struct {
	GroupID    string
	ArtifactID string
	Version    string
}

type pomProject struct {
	SCM pomSCM `xml:"scm"`
}

type pomSCM struct {
	Connection          string `xml:"connection"`
	DeveloperConnection string `xml:"developerConnection"`
	URL                 string `xml:"url"`
}

func Resolve(name, version string, _ *http.Client, baseURL string) (registry.ResolvedPackage, error) {
	if baseURL == "" {
		baseURL = DefaultRepository
	}
	baseURL = strings.TrimRight(baseURL, "/")

	coords, err := parseCoordinates(name, version)
	if err != nil {
		return registry.ResolvedPackage{}, err
	}

	return registry.ResolvedPackage{
		Registry:          registry.Maven,
		Name:              coords.GroupID + ":" + coords.ArtifactID,
		Version:           coords.Version,
		GitTag:            "v" + coords.Version,
		SourceArchiveURL:  baseURL + "/" + artifactPath(coords, "sources", "jar"),
		SourceMetadataURL: baseURL + "/" + artifactPath(coords, "", "pom"),
	}, nil
}

func parseCoordinates(name, version string) (Coordinates, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if name == "" {
		return Coordinates{}, fmt.Errorf("Maven package name must not be empty")
	}
	if version == "" {
		return Coordinates{}, fmt.Errorf("Maven version must not be empty")
	}

	parts := strings.Split(name, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return Coordinates{}, fmt.Errorf("invalid Maven coordinates %q, want groupId:artifactId", name)
	}
	groupID := strings.TrimSpace(parts[0])
	artifactID := strings.TrimSpace(parts[1])
	if err := validateCoordinateComponent("groupId", groupID); err != nil {
		return Coordinates{}, err
	}
	if err := validateCoordinateComponent("artifactId", artifactID); err != nil {
		return Coordinates{}, err
	}
	if err := validateCoordinateComponent("version", version); err != nil {
		return Coordinates{}, err
	}

	return Coordinates{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
	}, nil
}

func validateCoordinateComponent(label, value string) error {
	if strings.Contains(value, "/") || strings.Contains(value, `\`) || strings.Contains(value, "..") {
		return fmt.Errorf("Maven %s must not contain path separators or '..': %q", label, value)
	}
	return nil
}

func artifactPath(coords Coordinates, classifier, extension string) string {
	groupPath := strings.ReplaceAll(coords.GroupID, ".", "/")
	filename := coords.ArtifactID + "-" + coords.Version
	if classifier != "" {
		filename += "-" + classifier
	}
	filename += "." + extension
	return groupPath + "/" + coords.ArtifactID + "/" + coords.Version + "/" + filename
}

func ResolveSCMURL(client *http.Client, pomURL, name, version string) (string, error) {
	if client == nil {
		client = httpx.NewClient()
	}
	coords, err := parseCoordinates(name, version)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(pomURL) == "" {
		return "", fmt.Errorf("Maven POM URL must not be empty")
	}

	resp, err := client.Get(pomURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", repobridge.VersionNotFoundError{
			Message: fmt.Sprintf("Maven artifact %q version %q not found", coords.GroupID+":"+coords.ArtifactID, coords.Version),
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", repobridge.HTTPStatusError{Context: "Maven POM", Status: resp.Status}
	}

	var project pomProject
	if err := xml.NewDecoder(resp.Body).Decode(&project); err != nil {
		return "", err
	}

	for _, candidate := range []string{project.SCM.Connection, project.SCM.DeveloperConnection, project.SCM.URL} {
		if normalized := normalizeSCMURL(candidate); normalized != "" {
			return normalized, nil
		}
	}
	return "", nil
}

func normalizeSCMURL(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "scm:git:")
	value = strings.TrimPrefix(value, "git+")
	value = strings.TrimSuffix(value, ".git")

	switch {
	case strings.HasPrefix(value, "git://"):
		value = "https://" + strings.TrimPrefix(value, "git://")
	case strings.HasPrefix(value, "git@"):
		converted := scpLikeGitURL(value)
		if converted == "" {
			return ""
		}
		value = converted
	case strings.HasPrefix(value, "ssh://git@"):
		value = "https://" + strings.TrimPrefix(value, "ssh://git@")
	}

	normalized := registry.NormalizeRepoURL(value)
	if strings.HasPrefix(normalized, "https://github.com/") || strings.HasPrefix(normalized, "http://github.com/") {
		return strings.ToLower(normalized)
	}
	return normalized
}

func scpLikeGitURL(value string) string {
	hostAndPath := strings.TrimPrefix(value, "git@")
	host, path, ok := strings.Cut(hostAndPath, ":")
	if !ok || host == "" || path == "" {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: host, Path: "/" + path}).String()
}
