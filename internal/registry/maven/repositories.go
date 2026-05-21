package maven

import (
	"encoding/xml"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Repository struct {
	ID  string
	URL string
}

type pomModel struct {
	Parent       pomParent       `xml:"parent"`
	Repositories []repositoryXML `xml:"repositories>repository"`
	Profiles     []profileXML    `xml:"profiles>profile"`
}

type pomParent struct {
	RelativePath *string `xml:"relativePath"`
}

type repositoryXML struct {
	ID  string `xml:"id"`
	URL string `xml:"url"`
}

type profileXML struct {
	ID           string          `xml:"id"`
	Activation   activationXML   `xml:"activation"`
	Repositories []repositoryXML `xml:"repositories>repository"`
}

type activationXML struct {
	ActiveByDefault string `xml:"activeByDefault"`
}

type settingsXML struct {
	ActiveProfiles []string     `xml:"activeProfiles>activeProfile"`
	Profiles       []profileXML `xml:"profiles>profile"`
	Mirrors        []mirror     `xml:"mirrors>mirror"`
}

type mirror struct {
	ID       string `xml:"id"`
	MirrorOf string `xml:"mirrorOf"`
	URL      string `xml:"url"`
}

func EffectiveRepositories(cwd string) []Repository {
	return EffectiveRepositoriesWithSettings(cwd, defaultSettingsPath())
}

func EffectiveRepositoriesWithSettings(cwd, settingsPath string) []Repository {
	var repos []Repository
	seen := map[string]bool{}
	settings := readSettings(settingsPath)
	for _, profile := range settings.Profiles {
		if stringInList(strings.TrimSpace(profile.ID), settings.ActiveProfiles) || isActiveByDefault(profile) {
			addRepositories(&repos, seen, profile.Repositories)
		}
	}
	for _, model := range readPomHierarchy(cwd) {
		addRepositories(&repos, seen, model.Repositories)
		for _, profile := range model.Profiles {
			if isActiveByDefault(profile) {
				addRepositories(&repos, seen, profile.Repositories)
			}
		}
	}
	if !seen["central"] {
		repos = append(repos, Repository{ID: "central", URL: DefaultRepository})
	}
	return applyMirrors(repos, settings.Mirrors)
}

func defaultSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".m2", "settings.xml")
}

func readSettings(path string) settingsXML {
	if strings.TrimSpace(path) == "" {
		return settingsXML{}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return settingsXML{}
	}
	var settings settingsXML
	if err := xml.Unmarshal(content, &settings); err != nil {
		return settingsXML{}
	}
	return settings
}

func readPomHierarchy(cwd string) []pomModel {
	pomPath := nearestPom(cwd)
	if pomPath == "" {
		return nil
	}
	return readPomHierarchyFrom(pomPath, map[string]bool{})
}

func nearestPom(cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		candidate := filepath.Join(abs, "pom.xml")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
}

func readPomHierarchyFrom(path string, visited map[string]bool) []pomModel {
	abs, err := filepath.Abs(path)
	if err != nil || visited[abs] {
		return nil
	}
	visited[abs] = true
	content, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	var model pomModel
	if err := xml.Unmarshal(content, &model); err != nil {
		return nil
	}
	parentPath := localParentPom(abs, model.Parent)
	if parentPath == "" {
		return []pomModel{model}
	}
	hierarchy := readPomHierarchyFrom(parentPath, visited)
	return append(hierarchy, model)
}

func localParentPom(pomPath string, parent pomParent) string {
	if parent.RelativePath != nil && strings.TrimSpace(*parent.RelativePath) == "" {
		return ""
	}
	relativePath := "../pom.xml"
	if parent.RelativePath != nil {
		relativePath = strings.TrimSpace(*parent.RelativePath)
	}
	if relativePath == "" {
		return ""
	}
	candidate := filepath.Clean(filepath.Join(filepath.Dir(pomPath), filepath.FromSlash(relativePath)))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	return ""
}

func addRepositories(repos *[]Repository, seen map[string]bool, values []repositoryXML) {
	for _, value := range values {
		id := strings.TrimSpace(value.ID)
		repoURL := strings.TrimRight(strings.TrimSpace(value.URL), "/")
		if repoURL == "" {
			continue
		}
		if id == "" {
			id = repoURL
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		*repos = append(*repos, Repository{ID: id, URL: repoURL})
	}
}

func applyMirrors(repos []Repository, mirrors []mirror) []Repository {
	if len(mirrors) == 0 {
		return repos
	}
	result := make([]Repository, 0, len(repos))
	for _, repo := range repos {
		effective := repo
		for _, candidate := range mirrors {
			if strings.TrimSpace(candidate.URL) == "" {
				continue
			}
			if mirrorMatches(candidate.MirrorOf, repo) {
				effective.URL = strings.TrimRight(strings.TrimSpace(candidate.URL), "/")
				break
			}
		}
		result = append(result, effective)
	}
	return result
}

func mirrorMatches(pattern string, repo Repository) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	parts := strings.Split(pattern, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "!") && strings.TrimPrefix(part, "!") == repo.ID {
			return false
		}
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case repo.ID, "*":
			return true
		case "external:*":
			if isExternalRepository(repo.URL) {
				return true
			}
		}
	}
	return false
}

func isExternalRepository(repoURL string) bool {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return false
	}
	if parsed.Scheme == "file" || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host != "localhost" && host != "127.0.0.1" && host != "::1"
}

func stringInList(value string, values []string) bool {
	for _, candidate := range values {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func isActiveByDefault(profile profileXML) bool {
	return strings.EqualFold(strings.TrimSpace(profile.Activation.ActiveByDefault), "true")
}
