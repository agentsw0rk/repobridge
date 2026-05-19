package nuget

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"repobridge/internal/registry"
	"repobridge/internal/repobridge"
)

func TestResolveUsesRepositoryCommitAsGitRef(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"1.2.3"},
		Packages: map[string][]byte{
			"1.2.3": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata>
  <id>Example.Package</id>
  <version>1.2.3</version>
  <repository type="git" url="git+https://github.com/Owner/Repo.git" commit="0123456789abcdef0123456789abcdef01234567" branch="main" />
</metadata></package>`),
		},
	})
	defer server.Close()

	got, err := Resolve("Example.Package", "1.2.3", server.Client(), server.ServiceIndexURL)
	if err != nil {
		t.Fatal(err)
	}
	if got.Registry != registry.NuGet || got.Name != "Example.Package" || got.Version != "1.2.3" {
		t.Fatalf("resolved package identity = %#v", got)
	}
	if got.RepoURL != "https://github.com/owner/repo" {
		t.Fatalf("RepoURL = %q, want normalized GitHub URL", got.RepoURL)
	}
	if got.GitRef != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("GitRef = %q, want nuspec repository commit", got.GitRef)
	}
	if got.GitTag != "" {
		t.Fatalf("GitTag = %q, want empty when commit exists", got.GitTag)
	}
}

func TestResolveLatestStableIgnoresPrereleaseAndUnlistedRegistrationVersions(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"1.0.0", "2.0.0-beta.1", "1.5.0", "3.0.0"},
		RegistrationVersions: []nugetRegistrationVersion{
			{Version: "1.0.0"},
			{Version: "2.0.0-beta.1"},
			{Version: "1.5.0"},
			{Version: "3.0.0", Listed: boolPtr(false)},
		},
		Packages: map[string][]byte{
			"1.5.0": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata>
  <id>Example.Package</id>
  <version>1.5.0</version>
  <repository type="git" url="https://github.com/owner/repo.git" />
</metadata></package>`),
		},
	})
	defer server.Close()

	got, err := Resolve("Example.Package", "", server.Client(), server.ServiceIndexURL)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.5.0" {
		t.Fatalf("Version = %q, want latest stable", got.Version)
	}
	if got.GitTag != "v1.5.0" {
		t.Fatalf("GitTag = %q, want version tag", got.GitTag)
	}
	if got.GitRef != "" {
		t.Fatalf("GitRef = %q, want empty without commit", got.GitRef)
	}
}

func TestResolveWithoutStableListedVersionReturnsVersionNotFound(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"1.0.0", "2.0.0-beta.1"},
		RegistrationVersions: []nugetRegistrationVersion{
			{Version: "1.0.0", Listed: boolPtr(false)},
			{Version: "2.0.0-beta.1"},
		},
		Packages: map[string][]byte{
			"1.0.0": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata><id>Example.Package</id><version>1.0.0</version></metadata></package>`),
		},
	})
	defer server.Close()

	_, err := Resolve("Example.Package", "", server.Client(), server.ServiceIndexURL)
	var versionErr repobridge.VersionNotFoundError
	if !errors.As(err, &versionErr) {
		t.Fatalf("error = %T %[1]v, want VersionNotFoundError", err)
	}
}

func TestResolveExplicitPrereleaseVersion(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"1.0.0", "2.0.0-beta.1"},
		Packages: map[string][]byte{
			"2.0.0-beta.1": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata>
  <id>Example.Package</id>
  <version>2.0.0-beta.1</version>
  <repository type="git" url="https://github.com/owner/repo.git" />
</metadata></package>`),
		},
	})
	defer server.Close()

	got, err := Resolve("Example.Package", "2.0.0-BETA.1", server.Client(), server.ServiceIndexURL)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "2.0.0-beta.1" {
		t.Fatalf("Version = %q, want canonical prerelease version from versions index", got.Version)
	}
	if got.GitTag != "v2.0.0-beta.1" {
		t.Fatalf("GitTag = %q, want prerelease version tag", got.GitTag)
	}
}

func TestResolveIgnoresNestedFakeNuspecWhenRootNuspecExists(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"1.0.0"},
		Packages: map[string][]byte{
			"1.0.0": orderedZipPackage(t, []zipEntry{
				{
					Name: "content/fake.nuspec",
					Content: []byte(`<package><metadata>
  <id>Other.Package</id>
  <version>9.9.9</version>
  <repository type="git" url="https://github.com/other/repo.git" />
</metadata></package>`),
				},
				{
					Name: "Example.Package.nuspec",
					Content: []byte(`<package><metadata>
  <id>Example.Package</id>
  <version>1.0.0</version>
  <repository type="git" url="https://github.com/owner/repo.git" />
</metadata></package>`),
				},
			}),
		},
	})
	defer server.Close()

	got, err := Resolve("Example.Package", "1.0.0", server.Client(), server.ServiceIndexURL)
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoURL != "https://github.com/owner/repo" {
		t.Fatalf("RepoURL = %q, want root nuspec repository URL", got.RepoURL)
	}
}

func TestResolveRejectsMismatchedNuspecIdentity(t *testing.T) {
	tests := []struct {
		name       string
		nuspecID   string
		nuspecVers string
		want       string
	}{
		{
			name:       "id",
			nuspecID:   "Other.Package",
			nuspecVers: "1.0.0",
			want:       `NuGet .nuspec id "Other.Package" does not match requested package "Example.Package"`,
		},
		{
			name:       "version",
			nuspecID:   "Example.Package",
			nuspecVers: "9.9.9",
			want:       `NuGet .nuspec version "9.9.9" does not match resolved version "1.0.0"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newNuGetTestServer(t, nugetTestPackage{
				ID:       "Example.Package",
				Versions: []string{"1.0.0"},
				Packages: map[string][]byte{
					"1.0.0": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata>
  <id>`+tt.nuspecID+`</id>
  <version>`+tt.nuspecVers+`</version>
  <repository type="git" url="https://github.com/owner/repo.git" />
</metadata></package>`),
				},
			})
			defer server.Close()

			_, err := Resolve("Example.Package", "1.0.0", server.Client(), server.ServiceIndexURL)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %T %[1]v, want %q", err, tt.want)
			}
		})
	}
}

func TestResolveMissingPackageReturnsPackageNotFound(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Other.Package",
		Versions: []string{"1.0.0"},
		Packages: map[string][]byte{
			"1.0.0": nuspecPackage(t, "Other.Package.nuspec", `<package><metadata><id>Other.Package</id><version>1.0.0</version></metadata></package>`),
		},
	})
	defer server.Close()

	_, err := Resolve("Missing.Package", "", server.Client(), server.ServiceIndexURL)
	var notFound repobridge.PackageNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("error = %T %[1]v, want PackageNotFoundError", err)
	}
	if notFound.Name != "Missing.Package" || notFound.Registry != "NuGet" {
		t.Fatalf("PackageNotFoundError = %#v", notFound)
	}
}

func TestResolveMissingRepositoryReturnsNoRepoURL(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"1.0.0"},
		Packages: map[string][]byte{
			"1.0.0": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata>
  <id>Example.Package</id>
  <version>1.0.0</version>
</metadata></package>`),
		},
	})
	defer server.Close()

	_, err := Resolve("Example.Package", "1.0.0", server.Client(), server.ServiceIndexURL)
	var noRepo repobridge.NoRepoURLError
	if !errors.As(err, &noRepo) {
		t.Fatalf("error = %T %[1]v, want NoRepoURLError", err)
	}
}

func TestResolveMissingNuspecReturnsClearError(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"1.0.0"},
		Packages: map[string][]byte{
			"1.0.0": zipPackage(t, map[string][]byte{
				"lib/net8.0/Example.Package.dll": []byte("binary"),
			}),
		},
	})
	defer server.Close()

	_, err := Resolve("Example.Package", "1.0.0", server.Client(), server.ServiceIndexURL)
	if err == nil || !strings.Contains(err.Error(), "NuGet package is missing .nuspec") {
		t.Fatalf("error = %T %[1]v, want clear missing nuspec error", err)
	}
}

func TestResolveUnsupportedRepositoryHostReturnsNoRepoURL(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"1.0.0"},
		Packages: map[string][]byte{
			"1.0.0": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata>
  <id>Example.Package</id>
  <version>1.0.0</version>
  <repository type="git" url="https://example.com/owner/repo.git" />
</metadata></package>`),
		},
	})
	defer server.Close()

	_, err := Resolve("Example.Package", "1.0.0", server.Client(), server.ServiceIndexURL)
	var noRepo repobridge.NoRepoURLError
	if !errors.As(err, &noRepo) {
		t.Fatalf("error = %T %[1]v, want NoRepoURLError", err)
	}
}

func TestResolveExplicitMissingVersionReturnsVersionNotFound(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"1.0.0"},
		Packages: map[string][]byte{
			"1.0.0": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata><id>Example.Package</id><version>1.0.0</version></metadata></package>`),
		},
	})
	defer server.Close()

	_, err := Resolve("Example.Package", "2.0.0", server.Client(), server.ServiceIndexURL)
	var versionErr repobridge.VersionNotFoundError
	if !errors.As(err, &versionErr) {
		t.Fatalf("error = %T %[1]v, want VersionNotFoundError", err)
	}
}

func TestResolveWithoutStableVersionReturnsVersionNotFound(t *testing.T) {
	server := newNuGetTestServer(t, nugetTestPackage{
		ID:       "Example.Package",
		Versions: []string{"2.0.0-beta.1"},
		Packages: map[string][]byte{
			"2.0.0-beta.1": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata><id>Example.Package</id><version>2.0.0-beta.1</version></metadata></package>`),
		},
	})
	defer server.Close()

	_, err := Resolve("Example.Package", "", server.Client(), server.ServiceIndexURL)
	var versionErr repobridge.VersionNotFoundError
	if !errors.As(err, &versionErr) {
		t.Fatalf("error = %T %[1]v, want VersionNotFoundError", err)
	}
}

func TestResolveRejectsUnsafeNuspecPath(t *testing.T) {
	for _, name := range []string{
		"../Example.Package.nuspec",
		`..\Example.Package.nuspec`,
		`dir\..\Example.Package.nuspec`,
		"C:/Example.Package.nuspec",
	} {
		t.Run(name, func(t *testing.T) {
			server := newNuGetTestServer(t, nugetTestPackage{
				ID:       "Example.Package",
				Versions: []string{"1.0.0"},
				Packages: map[string][]byte{
					"1.0.0": zipPackage(t, map[string][]byte{
						name: []byte(`<package><metadata>
  <id>Example.Package</id>
  <version>1.0.0</version>
  <repository type="git" url="https://github.com/owner/repo.git" />
</metadata></package>`),
					}),
				},
			})
			defer server.Close()

			_, err := Resolve("Example.Package", "1.0.0", server.Client(), server.ServiceIndexURL)
			if err == nil || !strings.Contains(err.Error(), "unsafe .nuspec path") {
				t.Fatalf("error = %T %[1]v, want unsafe nuspec path error", err)
			}
		})
	}
}

func TestResolveNormalizesCommonGitRepositoryURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "git protocol", url: "git://github.com/owner/repo.git"},
		{name: "ssh url", url: "ssh://git@github.com/owner/repo.git"},
		{name: "scp like", url: "git@github.com:owner/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newNuGetTestServer(t, nugetTestPackage{
				ID:       "Example.Package",
				Versions: []string{"1.0.0"},
				Packages: map[string][]byte{
					"1.0.0": nuspecPackage(t, "Example.Package.nuspec", `<package><metadata>
  <id>Example.Package</id>
  <version>1.0.0</version>
  <repository type="git" url="`+tt.url+`" />
</metadata></package>`),
				},
			})
			defer server.Close()

			got, err := Resolve("Example.Package", "1.0.0", server.Client(), server.ServiceIndexURL)
			if err != nil {
				t.Fatal(err)
			}
			if got.RepoURL != "https://github.com/owner/repo" {
				t.Fatalf("RepoURL = %q, want normalized HTTPS GitHub URL", got.RepoURL)
			}
		})
	}
}

type nugetTestPackage struct {
	ID                   string
	Versions             []string
	RegistrationVersions []nugetRegistrationVersion
	Packages             map[string][]byte
}

type nugetRegistrationVersion struct {
	Version string
	Listed  *bool
}

type nugetTestServer struct {
	*httptest.Server
	ServiceIndexURL string
}

func newNuGetTestServer(t *testing.T, pkg nugetTestPackage) nugetTestServer {
	t.Helper()
	lowerID := strings.ToLower(pkg.ID)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flatBase := "http://" + r.Host + "/flat/"
		switch r.URL.Path {
		case "/v3/index.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resources":[{"@id":"` + flatBase + `","@type":"PackageBaseAddress/3.0.0"},{"@id":"http://` + r.Host + `/registrations/","@type":"RegistrationsBaseUrl/3.6.0"}]}`))
		case "/flat/" + lowerID + "/index.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"versions":[` + quotedVersions(pkg.Versions) + `]}`))
		case "/registrations/" + lowerID + "/index.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(registrationIndexJSON(pkg)))
		default:
			prefix := "/flat/" + lowerID + "/"
			suffix := "/" + lowerID + "."
			if strings.HasPrefix(r.URL.Path, prefix) && strings.Contains(r.URL.Path, suffix) && strings.HasSuffix(r.URL.Path, ".nupkg") {
				version := strings.TrimPrefix(r.URL.Path, prefix)
				version = strings.TrimSuffix(version, ".nupkg")
				if before, after, ok := strings.Cut(version, "/"+lowerID+"."); ok {
					if before != after {
						t.Fatalf("package path version mismatch: %q", r.URL.Path)
					}
					if data := pkg.Packages[before]; data != nil {
						w.Header().Set("Content-Type", "application/octet-stream")
						_, _ = w.Write(data)
						return
					}
				}
			}
			http.NotFound(w, r)
		}
	}))
	return nugetTestServer{Server: server, ServiceIndexURL: server.URL + "/v3/index.json"}
}

func registrationIndexJSON(pkg nugetTestPackage) string {
	versions := pkg.RegistrationVersions
	if len(versions) == 0 {
		versions = make([]nugetRegistrationVersion, 0, len(pkg.Versions))
		for _, version := range pkg.Versions {
			versions = append(versions, nugetRegistrationVersion{Version: version})
		}
	}

	items := make([]string, 0, len(versions))
	for _, version := range versions {
		listed := ""
		if version.Listed != nil {
			listed = `,"listed":` + strconv.FormatBool(*version.Listed)
		}
		items = append(items, `{"catalogEntry":{"id":"`+pkg.ID+`","version":"`+version.Version+`"`+listed+`}}`)
	}
	return `{"items":[{"items":[` + strings.Join(items, ",") + `]}]}`
}

func quotedVersions(versions []string) string {
	quoted := make([]string, 0, len(versions))
	for _, version := range versions {
		quoted = append(quoted, `"`+version+`"`)
	}
	return strings.Join(quoted, ",")
}

func boolPtr(value bool) *bool {
	return &value
}

func nuspecPackage(t *testing.T, name, content string) []byte {
	t.Helper()
	return zipPackage(t, map[string][]byte{name: []byte(content)})
}

type zipEntry struct {
	Name    string
	Content []byte
}

func orderedZipPackage(t *testing.T, files []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, file := range files {
		f, err := zw.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(file.Content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipPackage(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
