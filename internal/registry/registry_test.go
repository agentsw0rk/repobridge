package registry

import "testing"

func TestDetectRegistry(t *testing.T) {
	tests := []struct {
		spec  string
		reg   Registry
		clean string
	}{
		{"zod", NPM, "zod"},
		{"npm:react", NPM, "react"},
		{"pypi:requests", PyPI, "requests"},
		{"pip:requests", PyPI, "requests"},
		{"python:requests", PyPI, "requests"},
		{"crates:serde", Crates, "serde"},
		{"cargo:serde", Crates, "serde"},
		{"rust:serde", Crates, "serde"},
		{"maven:org.jetbrains.kotlin:kotlin-stdlib", Maven, "org.jetbrains.kotlin:kotlin-stdlib"},
		{"java:com.fasterxml.jackson.core:jackson-databind", Maven, "com.fasterxml.jackson.core:jackson-databind"},
		{"kotlin:org.jetbrains.kotlin:kotlin-stdlib", Maven, "org.jetbrains.kotlin:kotlin-stdlib"},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			got := DetectRegistry(tt.spec)
			if got.Registry != tt.reg || got.CleanSpec != tt.clean {
				t.Fatalf("DetectRegistry() = %#v, want %s %q", got, tt.reg, tt.clean)
			}
		})
	}
}

func TestParsePackageSpec(t *testing.T) {
	tests := []struct {
		spec    string
		reg     Registry
		name    string
		version string
	}{
		{"zod@3.22.4", NPM, "zod", "3.22.4"},
		{"@babel/core@7.0.0", NPM, "@babel/core", "7.0.0"},
		{"pypi:requests==2.31.0", PyPI, "requests", "2.31.0"},
		{"crates:serde@1.0.200", Crates, "serde", "1.0.200"},
		{"maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0", Maven, "org.jetbrains.kotlin:kotlin-stdlib", "2.1.0"},
		{"java:com.fasterxml.jackson.core:jackson-databind@2.17.2", Maven, "com.fasterxml.jackson.core:jackson-databind", "2.17.2"},
		{"kotlin:org.jetbrains.kotlin:kotlin-stdlib@2.1.0", Maven, "org.jetbrains.kotlin:kotlin-stdlib", "2.1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			got := ParsePackageSpec(tt.spec)
			if got.Registry != tt.reg || got.Name != tt.name || got.Version != tt.version {
				t.Fatalf("ParsePackageSpec() = %#v", got)
			}
		})
	}
}

func TestDetectInputType(t *testing.T) {
	tests := map[string]InputType{
		"zod":                                            PackageInput,
		"npm:owner/repo":                                 PackageInput,
		"owner/repo":                                     RepoInput,
		"github:owner/repo":                              RepoInput,
		"GitHub:owner/repo":                              RepoInput,
		"github.com/owner/repo":                          RepoInput,
		"gitlab.com/team/project":                        RepoInput,
		"gitlab.com/group/subgroup/repo":                 RepoInput,
		"bitbucket.org/team/project":                     RepoInput,
		"https://github.com/owner/repo":                  RepoInput,
		"https://GitHub.com/owner/repo":                  RepoInput,
		"https://example.com/owner/repo":                 RepoInput,
		"@scope/pkg":                                     PackageInput,
		"maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0": PackageInput,
		"java:com.fasterxml.jackson.core:jackson-databind@2.17.2": PackageInput,
		"kotlin:org.jetbrains.kotlin:kotlin-stdlib@2.1.0":         PackageInput,
	}
	for spec, want := range tests {
		if got := DetectInputType(spec); got != want {
			t.Fatalf("DetectInputType(%q) = %s, want %s", spec, got, want)
		}
	}
}

func TestNormalizeRepoURLRejectsNonRepoExtraPath(t *testing.T) {
	if got := NormalizeRepoURL("https://github.com/owner/repo/issues"); got != "" {
		t.Fatalf("NormalizeRepoURL() = %q, want empty", got)
	}
}

func TestNormalizeRepoURLRejectsEncodedSlashInPath(t *testing.T) {
	tests := []string{
		"https://github.com/owner/repo%2Fissues",
		"https://github.com/owner%2Frepo/issues",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if got := NormalizeRepoURL(input); got != "" {
				t.Fatalf("NormalizeRepoURL() = %q, want empty", got)
			}
		})
	}
}

func TestNormalizeRepoURLSupportsGitLabNestedProjectPaths(t *testing.T) {
	tests := map[string]string{
		"https://gitlab.com/group/subgroup/project":                  "https://gitlab.com/group/subgroup/project",
		"https://gitlab.com/group/subgroup/project/-/tree/main":      "https://gitlab.com/group/subgroup/project",
		"https://gitlab.com/group/subgroup/project/-/blob/main/x.go": "https://gitlab.com/group/subgroup/project",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := NormalizeRepoURL(input); got != want {
				t.Fatalf("NormalizeRepoURL() = %q, want %q", got, want)
			}
		})
	}
}
