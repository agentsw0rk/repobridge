package git

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type CloneResult struct {
	Success bool
	Warning string
	Error   error
}

type gitCommandRunner func(env []string, args ...string) ([]byte, error)

var gitRunner gitCommandRunner = func(env []string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.CombinedOutput()
}

var commitSHARE = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
var fullCommitSHARE = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func AuthenticatedCloneURL(cloneURL string) string {
	return cloneURL
}

func CloneAtTag(repoURL, target, version string) CloneResult {
	if err := ensureTargetAvailable(target); err != nil {
		return CloneResult{Error: err}
	}
	for _, tag := range []string{"v" + version, version} {
		if err := runGitClone(repoURL, target, "--depth", "1", "--branch", tag); err == nil {
			return CloneResult{Success: true}
		} else if isMissingRefError(err) {
			removeFailedClone(target)
		} else {
			return CloneResult{Error: err}
		}
	}

	if err := runGitClone(repoURL, target, "--depth", "1"); err != nil {
		return CloneResult{Error: err}
	}
	return CloneResult{
		Success: true,
		Warning: fmt.Sprintf("Could not find tag v%s or %s; cloned default branch instead.", version, version),
	}
}

func CloneAtCommit(repoURL, target, commit string) CloneResult {
	if err := ensureTargetAvailable(target); err != nil {
		return CloneResult{Error: err}
	}
	if !fullCommitSHARE.MatchString(commit) {
		return CloneResult{Error: fmt.Errorf("invalid commit SHA: %q", commit)}
	}
	if err := runGitClone(repoURL, target, "--no-checkout", "--depth", "1"); err != nil {
		return CloneResult{Error: err}
	}
	if err := runGitInTarget(repoURL, target, "fetch", "--depth", "1", "origin", commit); err != nil {
		removeFailedClone(target)
		return CloneResult{Error: err}
	}
	if err := runGitInTarget(repoURL, target, "checkout", "--detach", commit); err != nil {
		removeFailedClone(target)
		return CloneResult{Error: err}
	}
	return CloneResult{Success: true}
}

func CloneAtTagStrict(repoURL, target, version string) CloneResult {
	if err := ensureTargetAvailable(target); err != nil {
		return CloneResult{Error: err}
	}
	for _, tag := range []string{"v" + version, version} {
		if err := runGitClone(repoURL, target, "--depth", "1", "--branch", tag); err == nil {
			return CloneResult{Success: true}
		} else if isMissingRefError(err) {
			removeFailedClone(target)
		} else {
			return CloneResult{Error: err}
		}
	}
	return CloneResult{Error: fmt.Errorf("could not find tag v%s or %s", version, version)}
}

func CloneAtRef(repoURL, target, ref string) CloneResult {
	if err := ensureTargetAvailable(target); err != nil {
		return CloneResult{Error: err}
	}
	if commitSHARE.MatchString(ref) {
		return CloneResult{Error: fmt.Errorf("clone ref %q looks like a commit SHA; CloneAtRef supports branch or tag refs", ref)}
	}
	if err := runGitClone(repoURL, target, "--depth", "1", "--branch", ref); err == nil {
		return CloneResult{Success: true}
	} else if isMissingRefError(err) {
		removeFailedClone(target)
	} else {
		return CloneResult{Error: err}
	}

	if err := runGitClone(repoURL, target, "--depth", "1"); err != nil {
		return CloneResult{Error: err}
	}
	return CloneResult{
		Success: true,
		Warning: fmt.Sprintf("Could not find ref %s; cloned default branch instead.", ref),
	}
}

func RemoveGitDir(dir string) error {
	return os.RemoveAll(filepath.Join(dir, ".git"))
}

func runGitClone(repoURL, target string, args ...string) error {
	env := authConfigEnv(repoURL)
	cloneArgs := append([]string{"clone"}, args...)
	cloneArgs = append(cloneArgs, repoURL, target)
	output, err := gitRunner(env, cloneArgs...)
	if err != nil {
		return fmt.Errorf("git clone failed: %s\n%s", redactSecrets(err.Error()), redactSecrets(string(output)))
	}
	return nil
}

func runGitInTarget(repoURL, target string, args ...string) error {
	env := authConfigEnv(repoURL)
	gitArgs := append([]string{"-C", target}, args...)
	output, err := gitRunner(env, gitArgs...)
	if err != nil {
		return fmt.Errorf("git command failed: %s\n%s", redactSecrets(err.Error()), redactSecrets(string(output)))
	}
	return nil
}

func removeFailedClone(target string) {
	_ = os.RemoveAll(target)
}

func ensureTargetAvailable(target string) error {
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("clone target already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func authConfigEnv(repoURL string) []string {
	configs := []struct {
		prefix string
		env    string
		user   string
	}{
		{prefix: "https://github.com/", env: "GITHUB_TOKEN", user: "x-access-token"},
		{prefix: "https://gitlab.com/", env: "GITLAB_TOKEN", user: "oauth2"},
		{prefix: "https://bitbucket.org/", env: "BITBUCKET_TOKEN", user: "x-token-auth"},
	}

	for _, config := range configs {
		token := os.Getenv(config.env)
		if token == "" || !strings.HasPrefix(repoURL, config.prefix) {
			continue
		}
		header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(config.user+":"+token))
		return []string{
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http." + config.prefix + ".extraHeader",
			"GIT_CONFIG_VALUE_0=" + header,
		}
	}
	return nil
}

func redactSecrets(message string) string {
	for _, env := range []string{"GITHUB_TOKEN", "GITLAB_TOKEN", "BITBUCKET_TOKEN"} {
		token := os.Getenv(env)
		if token != "" {
			message = strings.ReplaceAll(message, token, "[REDACTED]")
		}
	}
	return message
}

func isMissingRefError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "remote branch") && strings.Contains(message, "not found") ||
		strings.Contains(message, "couldn't find remote ref") ||
		strings.Contains(message, "could not find remote ref") ||
		strings.Contains(message, "couldn't find any revision to build") ||
		strings.Contains(message, "pathspec") && strings.Contains(message, "did not match")
}
