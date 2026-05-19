package registry

import (
	"net/url"
	"strings"
)

var supportedRepoHosts = map[string]bool{
	"github.com":    true,
	"gitlab.com":    true,
	"bitbucket.org": true,
}

func NormalizeRepoURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(parsed.Host)
	if !supportedRepoHosts[host] {
		return ""
	}
	escapedPath := parsed.EscapedPath()
	if strings.Contains(strings.ToLower(escapedPath), "%2f") {
		return ""
	}
	normalizedPath, ok := repoPath(host, escapedPath)
	if !ok {
		return ""
	}
	return parsed.Scheme + "://" + host + normalizedPath
}

func repoPath(host, path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	if host == "gitlab.com" {
		return gitLabRepoPath(parts)
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	if repo == "" {
		return "", false
	}
	if len(parts) == 2 {
		return "/" + parts[0] + "/" + repo, true
	}
	if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") && parts[3] != "" {
		return "/" + parts[0] + "/" + repo, true
	}
	return "", false
}

func gitLabRepoPath(parts []string) (string, bool) {
	for i := range parts {
		if parts[i] == "" {
			return "", false
		}
		if parts[i] == "-" {
			if i < 2 || i+2 >= len(parts) || (parts[i+1] != "tree" && parts[i+1] != "blob") || parts[i+2] == "" {
				return "", false
			}
			return "/" + strings.TrimSuffix(strings.Join(parts[:i], "/"), ".git"), true
		}
	}
	path := "/" + strings.TrimSuffix(strings.Join(parts, "/"), ".git")
	return path, path != "/"
}
