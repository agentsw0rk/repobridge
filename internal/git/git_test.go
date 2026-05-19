package git

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAuthenticatedCloneURL(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "gh")
	t.Setenv("GITLAB_TOKEN", "gl")
	t.Setenv("BITBUCKET_TOKEN", "bb")

	tests := map[string]string{
		"https://github.com/o/r":    "https://github.com/o/r",
		"https://gitlab.com/o/r":    "https://gitlab.com/o/r",
		"https://bitbucket.org/o/r": "https://bitbucket.org/o/r",
	}
	for input, want := range tests {
		if got := AuthenticatedCloneURL(input); got != want {
			t.Fatalf("AuthenticatedCloneURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRemoveGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveGitDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git still exists or unexpected err: %v", err)
	}
}

func TestCloneAtRefPreservesPreExistingTargetOnFailure(t *testing.T) {
	target := t.TempDir()
	marker := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := CloneAtRef("not-a-repo", target, "missing")
	if result.Success {
		t.Fatalf("CloneAtRef unexpectedly succeeded: %#v", result)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal(err)
	}
}

func TestCloneAtRefDoesNotPassTokenInCloneURLOrError(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	calls := captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
		if len(env) == 0 {
			t.Fatalf("expected auth config env")
		}
		return []byte("remote output containing secret-token"), errForTest("auth failed")
	})
	target := filepath.Join(t.TempDir(), "clone")

	result := CloneAtRef("https://github.com/o/r", target, "main")
	if result.Error == nil {
		t.Fatalf("CloneAtRef unexpectedly succeeded: %#v", result)
	}
	for _, call := range calls() {
		if strings.Contains(strings.Join(call, " "), "secret-token") {
			t.Fatalf("token leaked in git arguments: %#v", call)
		}
	}
	if strings.Contains(result.Error.Error(), "secret-token") {
		t.Fatalf("token leaked in error: %v", result.Error)
	}
}

func TestCloneAtCommitFetchesExactCommit(t *testing.T) {
	repoURL := "https://github.com/o/r"
	target := filepath.Join(t.TempDir(), "clone")
	commit := "0123456789abcdef0123456789abcdef01234567"
	calls := captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
		return []byte("ok"), nil
	})

	result := CloneAtCommit(repoURL, target, commit)
	if !result.Success {
		t.Fatalf("CloneAtCommit failed: %#v", result)
	}

	want := [][]string{
		{"clone", "--no-checkout", "--depth", "1", repoURL, target},
		{"-C", target, "fetch", "--depth", "1", "origin", commit},
		{"-C", target, "checkout", "--detach", commit},
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("git calls = %#v, want %#v", got, want)
	}
}

func TestCloneAtRefDoesNotFallbackOnNonMissingRefFailure(t *testing.T) {
	calls := captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
		return []byte("Authentication failed"), errForTest("auth failed")
	})

	result := CloneAtRef("https://github.com/o/r", filepath.Join(t.TempDir(), "clone"), "main")
	if result.Success {
		t.Fatalf("CloneAtRef unexpectedly succeeded: %#v", result)
	}
	if got := len(calls()); got != 1 {
		t.Fatalf("CloneAtRef made %d clone attempts, want 1", got)
	}
}

func TestRunGitCloneRedactsRunnerErrorToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
		return []byte("plain output"), errForTest("runner leaked secret-token")
	})

	err := runGitClone("https://github.com/o/r", filepath.Join(t.TempDir(), "clone"))
	if err == nil {
		t.Fatal("runGitClone unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("token leaked in error: %v", err)
	}
}

func TestCloneAtTagFallbackOnlyAfterMissingRefFailures(t *testing.T) {
	attempt := 0
	calls := captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
		attempt++
		switch attempt {
		case 1, 2:
			return []byte("fatal: couldn't find remote ref"), errForTest("exit status 128")
		default:
			return []byte("cloned"), nil
		}
	})

	result := CloneAtTag("https://github.com/o/r", filepath.Join(t.TempDir(), "clone"), "1.2.3")
	if !result.Success {
		t.Fatalf("CloneAtTag failed: %#v", result)
	}
	if result.Warning == "" || !strings.Contains(result.Warning, "cloned default branch") {
		t.Fatalf("CloneAtTag warning = %q, want default branch warning", result.Warning)
	}
	gotCalls := calls()
	if len(gotCalls) != 3 {
		t.Fatalf("CloneAtTag made %d clone attempts, want 3", len(gotCalls))
	}
	assertArgsContain(t, gotCalls[0], "--branch", "v1.2.3")
	assertArgsContain(t, gotCalls[1], "--branch", "1.2.3")
	if strings.Contains(strings.Join(gotCalls[2], " "), "--branch") {
		t.Fatalf("fallback clone included branch args: %#v", gotCalls[2])
	}
}

func TestCloneAtTagDoesNotFallbackOnNonMissingRefFailure(t *testing.T) {
	calls := captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
		return []byte("Authentication failed"), errForTest("auth failed")
	})

	result := CloneAtTag("https://github.com/o/r", filepath.Join(t.TempDir(), "clone"), "1.2.3")
	if result.Success {
		t.Fatalf("CloneAtTag unexpectedly succeeded: %#v", result)
	}
	if got := len(calls()); got != 1 {
		t.Fatalf("CloneAtTag made %d clone attempts, want 1", got)
	}
}

func TestCloneAtTagStrictDoesNotCloneDefaultBranch(t *testing.T) {
	calls := captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
		return []byte("fatal: couldn't find remote ref"), errors.New("exit status 128")
	})

	result := CloneAtTagStrict("https://github.com/o/r", filepath.Join(t.TempDir(), "clone"), "1.2.3")
	if result.Success || result.Error == nil {
		t.Fatalf("CloneAtTagStrict result = %#v, want missing tag error", result)
	}
	gotCalls := calls()
	if len(gotCalls) != 2 {
		t.Fatalf("CloneAtTagStrict made %d clone attempts, want 2", len(gotCalls))
	}
	assertArgsContain(t, gotCalls[0], "--branch", "v1.2.3")
	assertArgsContain(t, gotCalls[1], "--branch", "1.2.3")
}

func TestCloneAtRefMissingRefFallsBackWithWarning(t *testing.T) {
	attempt := 0
	calls := captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
		attempt++
		if attempt == 1 {
			return []byte("fatal: remote branch feature not found in upstream origin"), errForTest("exit status 128")
		}
		return []byte("cloned"), nil
	})

	result := CloneAtRef("https://github.com/o/r", filepath.Join(t.TempDir(), "clone"), "feature")
	if !result.Success {
		t.Fatalf("CloneAtRef failed: %#v", result)
	}
	if result.Warning == "" || !strings.Contains(result.Warning, "cloned default branch") {
		t.Fatalf("CloneAtRef warning = %q, want default branch warning", result.Warning)
	}
	gotCalls := calls()
	if len(gotCalls) != 2 {
		t.Fatalf("CloneAtRef made %d clone attempts, want 2", len(gotCalls))
	}
	assertArgsContain(t, gotCalls[0], "--branch", "feature")
	if strings.Contains(strings.Join(gotCalls[1], " "), "--branch") {
		t.Fatalf("fallback clone included branch args: %#v", gotCalls[1])
	}
}

func TestCloneAtRefRejectsCommitSHA(t *testing.T) {
	calls := captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
		t.Fatal("git runner should not be called for commit SHA refs")
		return nil, nil
	})

	result := CloneAtRef("https://github.com/o/r", filepath.Join(t.TempDir(), "clone"), "0123456789abcdef0123456789abcdef01234567")
	if result.Success || result.Error == nil {
		t.Fatalf("CloneAtRef result = %#v, want commit SHA rejection", result)
	}
	if got := len(calls()); got != 0 {
		t.Fatalf("CloneAtRef made %d clone attempts, want 0", got)
	}
}

func TestGitLabAndBitbucketAuthUsesEnvHeadersAndRedactsErrors(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		user  string
		token string
		url   string
	}{
		{name: "gitlab", env: "GITLAB_TOKEN", user: "oauth2", token: "gl-secret", url: "https://gitlab.com/o/r"},
		{name: "bitbucket", env: "BITBUCKET_TOKEN", user: "x-token-auth", token: "bb-secret", url: "https://bitbucket.org/o/r"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.env, tt.token)
			var capturedEnv []string
			calls := captureGitCalls(t, func(env []string, args []string) ([]byte, error) {
				capturedEnv = append([]string(nil), env...)
				return []byte("remote output leaked " + tt.token), errForTest("runner leaked " + tt.token)
			})

			result := CloneAtRef(tt.url, filepath.Join(t.TempDir(), "clone"), "main")
			if result.Error == nil {
				t.Fatalf("CloneAtRef unexpectedly succeeded: %#v", result)
			}
			if strings.Contains(result.Error.Error(), tt.token) {
				t.Fatalf("token leaked in error: %v", result.Error)
			}
			for _, call := range calls() {
				if strings.Contains(strings.Join(call, " "), tt.token) {
					t.Fatalf("token leaked in git arguments: %#v", call)
				}
			}
			expectedHeader := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(tt.user+":"+tt.token))
			if !envContains(capturedEnv, "GIT_CONFIG_VALUE_0="+expectedHeader) {
				t.Fatalf("auth header env missing; env=%#v", capturedEnv)
			}
		})
	}
}

type errForTest string

func (e errForTest) Error() string { return string(e) }

func captureGitCalls(t *testing.T, fn func([]string, []string) ([]byte, error)) func() [][]string {
	t.Helper()
	previous := gitRunner
	var calls [][]string
	gitRunner = func(env []string, args ...string) ([]byte, error) {
		copied := append([]string(nil), args...)
		calls = append(calls, copied)
		return fn(env, args)
	}
	t.Cleanup(func() {
		gitRunner = previous
	})
	return func() [][]string {
		return calls
	}
}

func assertArgsContain(t *testing.T, args []string, want ...string) {
	t.Helper()
	haystack := strings.Join(args, "\x00")
	for _, part := range want {
		if !strings.Contains(haystack, part) {
			t.Fatalf("args %#v do not contain %q", args, part)
		}
	}
}

func envContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
