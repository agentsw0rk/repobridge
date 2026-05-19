package maven

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

func TestArtifactPaths(t *testing.T) {
	coords := Coordinates{GroupID: "org.jetbrains.kotlin", ArtifactID: "kotlin-stdlib", Version: "2.1.0"}
	if got, want := artifactPath(coords, "sources", "jar"), "org/jetbrains/kotlin/kotlin-stdlib/2.1.0/kotlin-stdlib-2.1.0-sources.jar"; got != want {
		t.Fatalf("source path = %q, want %q", got, want)
	}
	if got, want := artifactPath(coords, "", "pom"), "org/jetbrains/kotlin/kotlin-stdlib/2.1.0/kotlin-stdlib-2.1.0.pom"; got != want {
		t.Fatalf("pom path = %q, want %q", got, want)
	}
}

func TestParseCoordinates(t *testing.T) {
	got, err := parseCoordinates("org.jetbrains.kotlin:kotlin-stdlib", "2.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.GroupID != "org.jetbrains.kotlin" || got.ArtifactID != "kotlin-stdlib" || got.Version != "2.1.0" {
		t.Fatalf("coordinates = %#v", got)
	}
}

func TestParseCoordinatesRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"", "1.0.0"},
		{"org.example", "1.0.0"},
		{"org.example:lib:extra", "1.0.0"},
		{"org.example:lib", ""},
		{"org.example:", "1.0.0"},
		{":lib", "1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"@"+tt.version, func(t *testing.T) {
			if _, err := parseCoordinates(tt.name, tt.version); err == nil {
				t.Fatal("error = nil, want invalid coordinates error")
			}
		})
	}
}

func TestNormalizeSCMURL(t *testing.T) {
	tests := map[string]string{
		"scm:git:https://github.com/owner/repo.git": "https://github.com/owner/repo",
		"git+https://github.com/owner/repo.git":     "https://github.com/owner/repo",
		"https://github.com/owner/repo":             "https://github.com/owner/repo",
		"git@github.com:owner/repo.git":             "https://github.com/owner/repo",
		"ssh://git@github.com/owner/repo.git":       "https://github.com/owner/repo",
		"git://github.com/owner/repo.git":           "https://github.com/owner/repo",
		"https://example.com/owner/repo":            "",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := normalizeSCMURL(input); got != want {
				t.Fatalf("normalizeSCMURL() = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveBuildsSourceAndMetadataURLsWithoutFetchingPOM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("Resolve made unexpected HTTP request to %s", r.URL.Path)
	}))
	defer server.Close()

	got, err := Resolve("org.jetbrains.kotlin:kotlin-stdlib", "2.1.0", server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.Registry != registry.Maven || got.Name != "org.jetbrains.kotlin:kotlin-stdlib" || got.Version != "2.1.0" {
		t.Fatalf("resolved = %#v", got)
	}
	if got.SourceArchiveURL != server.URL+"/org/jetbrains/kotlin/kotlin-stdlib/2.1.0/kotlin-stdlib-2.1.0-sources.jar" {
		t.Fatalf("SourceArchiveURL = %q", got.SourceArchiveURL)
	}
	if got.SourceMetadataURL != server.URL+"/org/jetbrains/kotlin/kotlin-stdlib/2.1.0/kotlin-stdlib-2.1.0.pom" {
		t.Fatalf("SourceMetadataURL = %q", got.SourceMetadataURL)
	}
	if got.RepoURL != "" {
		t.Fatalf("RepoURL = %q, want empty", got.RepoURL)
	}
	if got.GitTag != "v2.1.0" {
		t.Fatalf("GitTag = %q", got.GitTag)
	}
}

func TestResolveSCMURLPicksSCMConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/org/example/lib/1.0.0/lib-1.0.0.pom" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<project>
  <scm>
    <connection>scm:git:https://github.com/Owner/repo.git</connection>
    <developerConnection>scm:git:https://github.com/other/dev.git</developerConnection>
    <url>https://github.com/other/url</url>
  </scm>
</project>`))
	}))
	defer server.Close()

	got, err := ResolveSCMURL(server.Client(), server.URL+"/org/example/lib/1.0.0/lib-1.0.0.pom", "org.example:lib", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/owner/repo" {
		t.Fatalf("ResolveSCMURL() = %q, want normalized connection URL", got)
	}
}

func TestResolveSCMURLAllowsMissingSCM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<project></project>`))
	}))
	defer server.Close()

	got, err := ResolveSCMURL(server.Client(), server.URL+"/org/example/lib/1.0.0/lib-1.0.0.pom", "org.example:lib", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("ResolveSCMURL() = %q, want empty", got)
	}
}

func TestResolveSCMURLReturnsVersionNotFoundForMissingPOM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := ResolveSCMURL(server.Client(), server.URL+"/org/example/lib/1.0.0/lib-1.0.0.pom", "org.example:lib", "1.0.0")
	var versionErr repobridge.VersionNotFoundError
	if !errors.As(err, &versionErr) {
		t.Fatalf("error = %T %[1]v, want VersionNotFoundError", err)
	}
}

func TestResolveSCMURLReturnsHTTPStatusErrorForPOMFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := ResolveSCMURL(server.Client(), server.URL+"/org/example/lib/1.0.0/lib-1.0.0.pom", "org.example:lib", "1.0.0")
	var statusErr repobridge.HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %T %[1]v, want HTTPStatusError", err)
	}
	if statusErr.Context != "Maven POM" {
		t.Fatalf("Context = %q, want Maven POM", statusErr.Context)
	}
}

func TestParseCoordinatesRejectsPathComponents(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"org/example:lib", "1.0.0"},
		{`org\example:lib`, "1.0.0"},
		{"org..example:lib", "1.0.0"},
		{"org.example:li/b", "1.0.0"},
		{`org.example:li\b`, "1.0.0"},
		{"org.example:..", "1.0.0"},
		{"org.example:lib", "1/0/0"},
		{"org.example:lib", `1\0\0`},
		{"org.example:lib", "1.0..0"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"@"+tt.version, func(t *testing.T) {
			if _, err := parseCoordinates(tt.name, tt.version); err == nil {
				t.Fatal("error = nil, want invalid path component error")
			}
		})
	}
}
