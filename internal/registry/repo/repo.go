package repo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"repobridge/internal/httpx"
	"repobridge/internal/repobridge"
)

const defaultHost = "github.com"

var supportedHosts = map[string]bool{
	"github.com":    true,
	"gitlab.com":    true,
	"bitbucket.org": true,
}

var ownerRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)
var repoRE = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type Spec struct {
	Host  string
	Owner string
	Repo  string
	Ref   string
}

type Resolved struct {
	GitRef      string
	RepoURL     string
	DisplayName string
}

func ParseSpec(spec string) (Spec, bool) {
	input := strings.TrimSpace(spec)
	if input == "" || strings.HasPrefix(input, "@") {
		return Spec{}, false
	}

	remaining := input
	lowerRemaining := strings.ToLower(remaining)
	host := defaultHost
	ref := ""

	switch {
	case strings.HasPrefix(lowerRemaining, "github:"):
		remaining = remaining[len("github:"):]
		host = "github.com"
	case strings.HasPrefix(lowerRemaining, "gitlab:"):
		remaining = remaining[len("gitlab:"):]
		host = "gitlab.com"
	case strings.HasPrefix(lowerRemaining, "bitbucket:"):
		remaining = remaining[len("bitbucket:"):]
		host = "bitbucket.org"
	case strings.HasPrefix(lowerRemaining, "http://") || strings.HasPrefix(lowerRemaining, "https://"):
		return parseURLSpec(remaining)
	default:
		for h := range supportedHosts {
			prefix := h + "/"
			if strings.HasPrefix(strings.ToLower(remaining), prefix) {
				host = h
				remaining = remaining[len(prefix):]
				break
			}
		}
	}

	if at := strings.Index(remaining, "@"); at > 0 {
		ref = remaining[at+1:]
		remaining = remaining[:at]
	} else if hash := strings.Index(remaining, "#"); hash > 0 {
		ref = remaining[hash+1:]
		remaining = remaining[:hash]
	}

	parts := strings.Split(remaining, "/")
	if host == "gitlab.com" && len(parts) > 2 {
		spec, ok := parseGitLabURLSpec(parts, "")
		if !ok {
			return Spec{}, false
		}
		if ref != "" {
			spec.Ref = ref
		}
		return spec, true
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Spec{}, false
	}
	repoName := strings.TrimSuffix(parts[1], ".git")
	if !ownerRE.MatchString(parts[0]) || !repoRE.MatchString(repoName) {
		return Spec{}, false
	}
	return Spec{Host: host, Owner: parts[0], Repo: repoName, Ref: ref}, true
}

func parseURLSpec(raw string) (Spec, bool) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return Spec{}, false
	}
	host := strings.ToLower(parsed.Host)
	if !supportedHosts[host] {
		return Spec{}, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return Spec{}, false
	}
	if host == "gitlab.com" {
		return parseGitLabURLSpec(parts, parsed.Fragment)
	}
	repoSegment, ref := splitRepoAndRef(parts[1])
	repoName := strings.TrimSuffix(repoSegment, ".git")
	if !ownerRE.MatchString(parts[0]) || !repoRE.MatchString(repoName) {
		return Spec{}, false
	}
	if parsed.Fragment != "" {
		ref = parsed.Fragment
	}
	if len(parts) > 2 {
		if len(parts) < 4 || (parts[2] != "tree" && parts[2] != "blob") || parts[3] == "" {
			return Spec{}, false
		}
		ref = parts[3]
	}
	return Spec{Host: host, Owner: parts[0], Repo: repoName, Ref: ref}, true
}

func parseGitLabURLSpec(parts []string, fragment string) (Spec, bool) {
	ref := ""
	projectParts := parts
	for i, part := range parts {
		if part != "-" {
			continue
		}
		if i < 2 || i+2 >= len(parts) || (parts[i+1] != "tree" && parts[i+1] != "blob") || parts[i+2] == "" {
			return Spec{}, false
		}
		projectParts = parts[:i]
		ref = parts[i+2]
		break
	}
	last := len(projectParts) - 1
	repoSegment, atRef := splitRepoAndRef(projectParts[last])
	if atRef != "" {
		ref = atRef
	}
	projectParts[last] = strings.TrimSuffix(repoSegment, ".git")
	if fragment != "" {
		ref = fragment
	}
	if len(projectParts) < 2 || !ownerRE.MatchString(projectParts[0]) {
		return Spec{}, false
	}
	for _, part := range projectParts[1:] {
		if !repoRE.MatchString(part) {
			return Spec{}, false
		}
	}
	return Spec{Host: "gitlab.com", Owner: projectParts[0], Repo: strings.Join(projectParts[1:], "/"), Ref: ref}, true
}

func splitRepoAndRef(segment string) (string, string) {
	if at := strings.LastIndex(segment, "@"); at > 0 {
		return segment[:at], segment[at+1:]
	}
	return segment, ""
}

func IsRepoSpec(spec string) bool {
	_, ok := ParseSpec(spec)
	return ok
}

func Resolve(spec Spec, client *http.Client) (Resolved, error) {
	if client == nil {
		client = httpx.NewClient()
	}
	switch spec.Host {
	case "github.com":
		return resolveGitHub(spec, client, "https://api.github.com")
	case "gitlab.com":
		return resolveGitLab(spec, client, "https://gitlab.com/api/v4")
	case "bitbucket.org":
		return resolveBitbucket(spec, client, "https://api.bitbucket.org/2.0")
	default:
		return Resolved{
			GitRef:      firstNonEmpty(spec.Ref, "main"),
			RepoURL:     fmt.Sprintf("https://%s/%s/%s", spec.Host, spec.Owner, spec.Repo),
			DisplayName: fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo),
		}, nil
	}
}

func resolveGitHub(spec Spec, client *http.Client, baseURL string) (Resolved, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/repos/%s/%s", baseURL, spec.Owner, spec.Repo), nil)
	if err != nil {
		return Resolved{}, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Resolved{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		hint := " If this is a private repo, set GITHUB_TOKEN."
		if os.Getenv("GITHUB_TOKEN") != "" {
			hint = " Your token may lack access to this repository."
		}
		return Resolved{}, repobridge.RepoNotFoundError{Message: fmt.Sprintf("Repository %q not found on GitHub.%s", spec.Owner+"/"+spec.Repo, hint)}
	}
	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(string(body)), "rate limit") {
			return Resolved{}, repobridge.RateLimitExceededError{}
		}
		hint := " If this is a private repo, set GITHUB_TOKEN."
		if os.Getenv("GITHUB_TOKEN") != "" {
			hint = " Your token may lack access to this repository."
		}
		return Resolved{}, repobridge.RepoNotFoundError{Message: fmt.Sprintf("Repository %q is not accessible on GitHub.%s", spec.Owner+"/"+spec.Repo, hint)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Resolved{}, repobridge.HTTPStatusError{Context: "repository info", Status: resp.Status}
	}
	var data struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return Resolved{}, err
	}
	return Resolved{
		GitRef:      firstNonEmpty(spec.Ref, data.DefaultBranch, "main"),
		RepoURL:     fmt.Sprintf("https://github.com/%s/%s", spec.Owner, spec.Repo),
		DisplayName: fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo),
	}, nil
}

func resolveGitLab(spec Spec, client *http.Client, baseURL string) (Resolved, error) {
	projectPath := url.QueryEscape(spec.Owner + "/" + spec.Repo)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/projects/%s", baseURL, projectPath), nil)
	if err != nil {
		return Resolved{}, err
	}
	if token := os.Getenv("GITLAB_TOKEN"); token != "" {
		req.Header.Set("PRIVATE-TOKEN", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Resolved{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		hint := " If this is a private repo, set GITLAB_TOKEN."
		if os.Getenv("GITLAB_TOKEN") != "" {
			hint = " Your token may lack access to this repository."
		}
		return Resolved{}, repobridge.RepoNotFoundError{Message: fmt.Sprintf("Repository %q not found on GitLab.%s", spec.Owner+"/"+spec.Repo, hint)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Resolved{}, repobridge.HTTPStatusError{Context: "repository info", Status: resp.Status}
	}
	var data struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return Resolved{}, err
	}
	return Resolved{
		GitRef:      firstNonEmpty(spec.Ref, data.DefaultBranch, "main"),
		RepoURL:     fmt.Sprintf("https://gitlab.com/%s/%s", spec.Owner, spec.Repo),
		DisplayName: fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo),
	}, nil
}

func resolveBitbucket(spec Spec, client *http.Client, baseURL string) (Resolved, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/repositories/%s/%s", baseURL, spec.Owner, spec.Repo), nil)
	if err != nil {
		return Resolved{}, err
	}
	if token := os.Getenv("BITBUCKET_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Resolved{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		hint := " If this is a private repo, set BITBUCKET_TOKEN."
		if os.Getenv("BITBUCKET_TOKEN") != "" {
			hint = " Your BITBUCKET_TOKEN may lack access to this repository."
		}
		return Resolved{}, repobridge.RepoNotFoundError{Message: fmt.Sprintf("Repository %q not found on Bitbucket.%s", spec.Owner+"/"+spec.Repo, hint)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Resolved{}, repobridge.HTTPStatusError{Context: "repository info", Status: resp.Status}
	}
	var data struct {
		MainBranch *struct {
			Name string `json:"name"`
		} `json:"mainbranch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return Resolved{}, err
	}
	defaultBranch := "main"
	if data.MainBranch != nil && data.MainBranch.Name != "" {
		defaultBranch = data.MainBranch.Name
	}
	return Resolved{
		GitRef:      firstNonEmpty(spec.Ref, defaultBranch),
		RepoURL:     fmt.Sprintf("https://bitbucket.org/%s/%s", spec.Owner, spec.Repo),
		DisplayName: fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
