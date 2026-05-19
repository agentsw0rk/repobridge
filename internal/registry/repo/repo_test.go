package repo

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"repobridge/internal/repobridge"
)

func TestParseSpec(t *testing.T) {
	tests := []struct {
		spec  string
		host  string
		owner string
		repo  string
		ref   string
	}{
		{"owner/project", "github.com", "owner", "project", ""},
		{"owner/project@dev", "github.com", "owner", "project", "dev"},
		{"github:owner/project#main", "github.com", "owner", "project", "main"},
		{"GitHub:owner/project", "github.com", "owner", "project", ""},
		{"gitlab:team/project", "gitlab.com", "team", "project", ""},
		{"gitlab:group/subgroup/project", "gitlab.com", "group", "subgroup/project", ""},
		{"gitlab:group/subgroup/project@dev", "gitlab.com", "group", "subgroup/project", "dev"},
		{"bitbucket:team/project@release", "bitbucket.org", "team", "project", "release"},
		{"github.com/owner/project", "github.com", "owner", "project", ""},
		{"gitlab.com/team/project", "gitlab.com", "team", "project", ""},
		{"bitbucket.org/team/project", "bitbucket.org", "team", "project", ""},
		{"https://github.com/owner/project.git", "github.com", "owner", "project", ""},
		{"https://GitHub.com/owner/project", "github.com", "owner", "project", ""},
		{"https://github.com/owner/project/tree/trunk/src", "github.com", "owner", "project", "trunk"},
		{"https://gitlab.com/group/subgroup/project", "gitlab.com", "group", "subgroup/project", ""},
		{"https://gitlab.com/group/subgroup/project/-/tree/trunk/src", "gitlab.com", "group", "subgroup/project", "trunk"},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			got, ok := ParseSpec(tt.spec)
			if !ok {
				t.Fatalf("ParseSpec(%q) failed", tt.spec)
			}
			if got.Host != tt.host || got.Owner != tt.owner || got.Repo != tt.repo || got.Ref != tt.ref {
				t.Fatalf("ParseSpec() = %#v", got)
			}
		})
	}
}

func TestParseSpecRejectsUnsupportedURLHost(t *testing.T) {
	if _, ok := ParseSpec("https://example.com/owner/project"); ok {
		t.Fatal("ParseSpec accepted unsupported URL host")
	}
}

func TestParseSpecRejectsScopedPackage(t *testing.T) {
	if _, ok := ParseSpec("@scope/pkg"); ok {
		t.Fatal("ParseSpec accepted scoped package")
	}
}

func TestParseSpecRejectsUnsupportedURLPaths(t *testing.T) {
	tests := []string{
		"https://example.com/owner/repo/docs",
		"https://github.com/owner/repo/issues/1",
	}
	for _, spec := range tests {
		t.Run(spec, func(t *testing.T) {
			if _, ok := ParseSpec(spec); ok {
				t.Fatalf("ParseSpec(%q) accepted unsupported URL path", spec)
			}
		})
	}
}

func TestParseURLSpecWithHashRef(t *testing.T) {
	got, ok := ParseSpec("https://github.com/owner/repo#dev")
	if !ok {
		t.Fatal("ParseSpec failed")
	}
	if got.Ref != "dev" {
		t.Fatalf("Ref = %q, want dev", got.Ref)
	}
}

func TestParseURLSpecWithAtRef(t *testing.T) {
	got, ok := ParseSpec("https://github.com/owner/repo@dev")
	if !ok {
		t.Fatal("ParseSpec failed")
	}
	if got.Repo != "repo" || got.Ref != "dev" {
		t.Fatalf("ParseSpec() = %#v", got)
	}
}

func TestParseURLSpecWithDotGitAtRef(t *testing.T) {
	got, ok := ParseSpec("https://github.com/owner/repo.git@dev")
	if !ok {
		t.Fatal("ParseSpec failed")
	}
	if got.Repo != "repo" || got.Ref != "dev" {
		t.Fatalf("ParseSpec() = %#v", got)
	}
}

func TestResolveFallbackHost(t *testing.T) {
	got, err := Resolve(Spec{Host: "example.com", Owner: "o", Repo: "r", Ref: "dev"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.GitRef != "dev" || got.RepoURL != "https://example.com/o/r" || got.DisplayName != "example.com/o/r" {
		t.Fatalf("Resolve() = %#v", got)
	}
}

func TestResolveGitHubDefaultBranch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/project" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"default_branch":"trunk"}`))
	}))
	defer server.Close()

	got, err := resolveGitHub(Spec{Host: "github.com", Owner: "owner", Repo: "project"}, server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.GitRef != "trunk" {
		t.Fatalf("GitRef = %q, want trunk", got.GitRef)
	}
}

func TestResolveGitHubForbiddenWithoutRateLimitMentionsTokenOrAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "10")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by personal access token"}`))
	}))
	defer server.Close()

	_, err := resolveGitHub(Spec{Host: "github.com", Owner: "owner", Repo: "project"}, server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	var rateLimitErr repobridge.RateLimitExceededError
	if errors.As(err, &rateLimitErr) {
		t.Fatalf("error = %T, want non-rate-limit auth/access error", err)
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "token") && !strings.Contains(message, "access") {
		t.Fatalf("error = %q, want token/access hint", err)
	}
}

func TestResolveGitHubForbiddenRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer server.Close()

	_, err := resolveGitHub(Spec{Host: "github.com", Owner: "owner", Repo: "project"}, server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	var rateLimitErr repobridge.RateLimitExceededError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("error = %T, want RateLimitExceededError", err)
	}
}

func TestResolveBitbucketUsesBearerToken(t *testing.T) {
	t.Setenv("BITBUCKET_TOKEN", "secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q, want Bearer secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"mainbranch":{"name":"trunk"}}`))
	}))
	defer server.Close()

	got, err := resolveBitbucket(Spec{Host: "bitbucket.org", Owner: "owner", Repo: "project"}, server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.GitRef != "trunk" {
		t.Fatalf("GitRef = %q, want trunk", got.GitRef)
	}
}

func TestResolveBitbucketForbiddenMentionsToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	_, err := resolveBitbucket(Spec{Host: "bitbucket.org", Owner: "owner", Repo: "project"}, server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "BITBUCKET_TOKEN") {
		t.Fatalf("error = %q, want BITBUCKET_TOKEN hint", err)
	}
}

func TestResolveBitbucketForbiddenWithTokenMentionsToken(t *testing.T) {
	t.Setenv("BITBUCKET_TOKEN", "secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	_, err := resolveBitbucket(Spec{Host: "bitbucket.org", Owner: "owner", Repo: "project"}, server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "BITBUCKET_TOKEN") {
		t.Fatalf("error = %q, want BITBUCKET_TOKEN hint", err)
	}
}
