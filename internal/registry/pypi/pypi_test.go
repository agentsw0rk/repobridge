package pypi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"repobridge/internal/repobridge"
)

func TestResolveUsesProjectURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"info":{
				"version":"2.31.0",
				"home_page":"https://example.com",
				"project_urls":{"Source":"https://github.com/psf/requests.git"}
			}
		}`))
	}))
	defer server.Close()
	got, err := Resolve("requests", "", server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "2.31.0" || got.RepoURL != "https://github.com/psf/requests" {
		t.Fatalf("Resolve() = %#v", got)
	}
}

func TestResolveIgnoresProjectURLWithUnsupportedHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"info":{
				"version":"2.31.0",
				"home_page":"https://example.com",
				"project_urls":{"Source":"https://github.com.evil/o/r.git"}
			}
		}`))
	}))
	defer server.Close()

	_, err := Resolve("requests", "", server.Client(), server.URL)
	var noRepoErr repobridge.NoRepoURLError
	if !errors.As(err, &noRepoErr) {
		t.Fatalf("Resolve() error = %T %v, want NoRepoURLError", err, err)
	}
}

func TestResolveIgnoresProjectURLWithNonRepoExtraPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"info":{
				"version":"2.31.0",
				"home_page":"https://example.com",
				"project_urls":{"Bug Tracker":"https://github.com/owner/repo/issues"}
			}
		}`))
	}))
	defer server.Close()

	_, err := Resolve("requests", "", server.Client(), server.URL)
	var noRepoErr repobridge.NoRepoURLError
	if !errors.As(err, &noRepoErr) {
		t.Fatalf("Resolve() error = %T %v, want NoRepoURLError", err, err)
	}
}

func TestResolveEscapesNameAndVersionPathSegments(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"info":{
				"version":"1.0/2",
				"home_page":"",
				"project_urls":{"Source":"https://github.com/owner/repo.git"}
			}
		}`))
	}))
	defer server.Close()

	if _, err := Resolve("my/pkg", "1.0/2", server.Client(), server.URL); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/my%2Fpkg/1.0%2F2/json" {
		t.Fatalf("request path = %q", gotPath)
	}
}
