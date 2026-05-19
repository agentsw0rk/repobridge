package crates

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"repobridge/internal/repobridge"
)

func TestResolveLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crate":{"max_version":"1.0.200","repository":"https://github.com/serde-rs/serde.git","homepage":null}}`))
	}))
	defer server.Close()
	got, err := Resolve("serde", "", server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.0.200" || got.RepoURL != "https://github.com/serde-rs/serde" {
		t.Fatalf("Resolve() = %#v", got)
	}
}

func TestResolveRejectsRepositoryURLWithUnsupportedHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crate":{"max_version":"1.0.200","repository":"https://github.com.evil/o/r.git","homepage":null}}`))
	}))
	defer server.Close()

	_, err := Resolve("serde", "", server.Client(), server.URL)
	var noRepoErr repobridge.NoRepoURLError
	if !errors.As(err, &noRepoErr) {
		t.Fatalf("Resolve() error = %T %v, want NoRepoURLError", err, err)
	}
}

func TestResolveRejectsHomepageWithNonRepoExtraPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crate":{"max_version":"1.0.200","repository":"","homepage":"https://github.com/owner/repo/issues"}}`))
	}))
	defer server.Close()

	_, err := Resolve("serde", "", server.Client(), server.URL)
	var noRepoErr repobridge.NoRepoURLError
	if !errors.As(err, &noRepoErr) {
		t.Fatalf("Resolve() error = %T %v, want NoRepoURLError", err, err)
	}
}

func TestResolveEscapesNameAndVersionPathSegments(t *testing.T) {
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crate":{"max_version":"1.0.200","repository":"https://github.com/owner/repo.git","homepage":null}}`))
	}))
	defer server.Close()

	if _, err := Resolve("bad/name", "1.0/2", server.Client(), server.URL); err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"/crates/bad%2Fname", "/crates/bad%2Fname/1.0%2F2"}
	if len(gotPaths) != len(wantPaths) {
		t.Fatalf("request paths = %#v", gotPaths)
	}
	for i, want := range wantPaths {
		if gotPaths[i] != want {
			t.Fatalf("request paths = %#v", gotPaths)
		}
	}
}
