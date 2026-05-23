package updatecheck

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServiceCheckReportsAvailableUpdate(t *testing.T) {
	server := releaseServer(t, "v0.10.5")
	service := Service{
		CurrentVersion: "v0.10.4",
		Client:         Client{HTTPClient: server.Client(), BaseURL: server.URL},
		Now:            func() time.Time { return time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC) },
	}

	result, err := service.Check(t.Context())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !result.Available || result.LatestVersion != "v0.10.5" {
		t.Fatalf("result = %#v, want available v0.10.5", result)
	}
	if result.CurrentVersion != "v0.10.4" || result.ReleaseURL != "https://example.test/v0.10.5" {
		t.Fatalf("result = %#v, want current version and release URL", result)
	}
}

func TestServiceCheckReportsCurrent(t *testing.T) {
	server := releaseServer(t, "v0.10.4")
	service := Service{CurrentVersion: "v0.10.4", Client: Client{HTTPClient: server.Client(), BaseURL: server.URL}}

	result, err := service.Check(t.Context())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.Available {
		t.Fatalf("result = %#v, want no update", result)
	}
}

func TestServiceCheckSkipsNonReleaseCurrentVersion(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("unexpected release request for %s", r.URL.Path)
	}))
	defer server.Close()
	service := Service{CurrentVersion: "dev", Client: Client{HTTPClient: server.Client(), BaseURL: server.URL}}

	result, err := service.Check(t.Context())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if called {
		t.Fatal("Check() called release server for non-release version")
	}
	if result.Available || result.CurrentVersion != "dev" {
		t.Fatalf("result = %#v, want no update for dev version", result)
	}
}

func TestServiceOpportunisticCheckUsesFreshState(t *testing.T) {
	store := StateStore{HomeDir: t.TempDir()}
	now := time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC)
	if err := store.Write(State{
		CheckedAt:  now.Add(-time.Hour),
		Latest:     "v0.10.5",
		ReleaseURL: "https://example.test/v0.10.5",
	}); err != nil {
		t.Fatal(err)
	}
	service := Service{
		CurrentVersion: "v0.10.4",
		StateStore:     store,
		Now:            func() time.Time { return now },
		CheckInterval:  24 * time.Hour,
	}

	result, err := service.OpportunisticCheck(t.Context())
	if err != nil {
		t.Fatalf("OpportunisticCheck() error = %v", err)
	}
	if !result.Available || result.LatestVersion != "v0.10.5" {
		t.Fatalf("result = %#v, want cached update", result)
	}
	if result.ReleaseURL != "https://example.test/v0.10.5" {
		t.Fatalf("ReleaseURL = %q", result.ReleaseURL)
	}
}

func releaseServer(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://example.test/` + tag + `","assets":[]}`))
	}))
	t.Cleanup(server.Close)
	return server
}
