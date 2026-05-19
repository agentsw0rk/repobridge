package npm

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"repobridge/internal/repobridge"
)

func TestRegistryURL(t *testing.T) {
	if got := registryURL("https://registry.npmjs.org", "@babel/core"); got != "https://registry.npmjs.org/%40babel%2Fcore" {
		t.Fatalf("registryURL() = %q", got)
	}
}

func TestResolvePackageUsesLatestAndVersionRepo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dist-tags":{"latest":"1.2.3"},
			"repository":{"url":"https://github.com/top/repo.git"},
			"versions":{
				"1.2.3":{"repository":{"url":"git+https://github.com/version/repo.git","directory":"packages/pkg"}}
			}
		}`))
	}))
	defer server.Close()

	got, err := Resolve("demo", "", server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.2.3" || got.RepoURL != "https://github.com/version/repo" || got.RepoDirectory != "packages/pkg" || got.GitTag != "v1.2.3" {
		t.Fatalf("Resolve() = %#v", got)
	}
}

func TestResolveRejectsRepositoryURLWithUnsupportedHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dist-tags":{"latest":"1.2.3"},
			"versions":{
				"1.2.3":{"repository":{"url":"https://github.com.evil/o/r.git"}}
			}
		}`))
	}))
	defer server.Close()

	_, err := Resolve("demo", "", server.Client(), server.URL)
	var noRepoErr repobridge.NoRepoURLError
	if !errors.As(err, &noRepoErr) {
		t.Fatalf("Resolve() error = %T %v, want NoRepoURLError", err, err)
	}
}

func TestResolveNormalizesGitHubShorthandRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dist-tags":{"latest":"1.2.3"},
			"versions":{
				"1.2.3":{"repository":{"url":"github:owner/repo"}}
			}
		}`))
	}))
	defer server.Close()

	got, err := Resolve("demo", "", server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoURL != "https://github.com/owner/repo" {
		t.Fatalf("RepoURL = %q", got.RepoURL)
	}
}

func TestResolveUsesTopLevelStringRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dist-tags":{"latest":"1.2.3"},
			"repository":"github:owner/repo",
			"versions":{"1.2.3":{}}
		}`))
	}))
	defer server.Close()

	got, err := Resolve("demo", "", server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoURL != "https://github.com/owner/repo" {
		t.Fatalf("RepoURL = %q", got.RepoURL)
	}
}

func TestResolveUsesVersionLevelStringRepositoryBeforeTopLevel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dist-tags":{"latest":"1.2.3"},
			"repository":{"url":"https://github.com/top/repo.git"},
			"versions":{"1.2.3":{"repository":"https://github.com/version/repo.git"}}
		}`))
	}))
	defer server.Close()

	got, err := Resolve("demo", "", server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoURL != "https://github.com/version/repo" {
		t.Fatalf("RepoURL = %q", got.RepoURL)
	}
}
