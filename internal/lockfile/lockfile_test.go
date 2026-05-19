package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripVersionPrefix(t *testing.T) {
	tests := map[string]string{
		"^1.2.3":  "1.2.3",
		"~1.2.3":  "1.2.3",
		">=1.2.3": "1.2.3",
		"1.2.3":   "1.2.3",
	}
	for input, want := range tests {
		if got := stripVersionPrefix(input); got != want {
			t.Fatalf("stripVersionPrefix(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPackageJSONVersion(t *testing.T) {
	content := `{"dependencies":{"zod":"^3.22.4"},"devDependencies":{"vitest":"~1.0.0"},"peerDependencies":{"react":">=18.2.0"}}`
	if got := parsePackageJSONVersion(content, "zod"); got != "3.22.4" {
		t.Fatalf("zod = %q", got)
	}
	if got := parsePackageJSONVersion(content, "vitest"); got != "1.0.0" {
		t.Fatalf("vitest = %q", got)
	}
	if got := parsePackageJSONVersion(content, "react"); got != "18.2.0" {
		t.Fatalf("react = %q", got)
	}
}

func TestPackageJSONSkipsProtocols(t *testing.T) {
	content := `{"dependencies":{"a":"workspace:*","b":"link:../b","c":"file:./c.tgz","d":"github:owner/repo","e":"git+https://example.com/e.git"}}`
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if got := parsePackageJSONVersion(content, name); got != "" {
			t.Fatalf("%s = %q, want empty", name, got)
		}
	}
}

func TestPackageJSONSkipsComplexRanges(t *testing.T) {
	content := `{
		"dependencies": {
			"a": ">=18 <20",
			"b": "^18 || ^19",
			"c": "*",
			"d": "latest",
			"e": "next",
			"f": "",
			"g": "18.x",
			"h": "18 - 19"
		},
		"peerDependencies": {
			"i": "<=18.2.0",
			"j": ">18.0.0"
		}
	}`
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		if got := parsePackageJSONVersion(content, name); got != "" {
			t.Fatalf("%s = %q, want empty", name, got)
		}
	}
}

func TestPackageJSONIgnoresScalarTopLevelFields(t *testing.T) {
	content := `{"name":"app","version":"1.0.0","scripts":{"test":"go test ./..."},"dependencies":{"zod":"^3.22.4"}}`
	if got := parsePackageJSONVersion(content, "zod"); got != "3.22.4" {
		t.Fatalf("parsePackageJSONVersion = %q, want zod version", got)
	}
}

func TestPackageLockV7(t *testing.T) {
	content := `{"packages":{"node_modules/zod":{"version":"3.22.4"}}}`
	if got := parsePackageLock(content, "zod"); got != "3.22.4" {
		t.Fatalf("parsePackageLock = %q", got)
	}
}

func TestPackageLockSkipsProtocolVersions(t *testing.T) {
	v7 := `{"packages":{"node_modules/zod":{"version":"file:../zod"}}}`
	if got := parsePackageLock(v7, "zod"); got != "" {
		t.Fatalf("parsePackageLock v7 = %q, want empty", got)
	}

	v6 := `{"dependencies":{"zod":{"version":"link:../zod"}}}`
	if got := parsePackageLock(v6, "zod"); got != "" {
		t.Fatalf("parsePackageLock v6 = %q, want empty", got)
	}
}

func TestPackageLockV7NestedDependency(t *testing.T) {
	content := `{"packages":{"node_modules/a":{"version":"1.0.0"},"node_modules/a/node_modules/zod":{"version":"3.22.4"}}}`
	if got := parsePackageLock(content, "zod"); got != "3.22.4" {
		t.Fatalf("parsePackageLock = %q, want nested zod version", got)
	}
}

func TestPackageLockV6NestedDependency(t *testing.T) {
	content := `{"dependencies":{"a":{"version":"1.0.0","dependencies":{"zod":{"version":"3.22.4"}}}}}`
	if got := parsePackageLock(content, "zod"); got != "3.22.4" {
		t.Fatalf("parsePackageLock = %q, want nested zod version", got)
	}
}

func TestPackageLockNestedSkipsProtocolVersion(t *testing.T) {
	v7 := `{"packages":{"node_modules/a":{"version":"1.0.0"},"node_modules/a/node_modules/zod":{"version":"file:../zod"}}}`
	if got := parsePackageLock(v7, "zod"); got != "" {
		t.Fatalf("parsePackageLock v7 = %q, want empty", got)
	}

	v6 := `{"dependencies":{"a":{"version":"1.0.0","dependencies":{"zod":{"version":"file:../zod"}}}}}`
	if got := parsePackageLock(v6, "zod"); got != "" {
		t.Fatalf("parsePackageLock v6 = %q, want empty", got)
	}
}

func TestDetectInstalledVersionPriority(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "zod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "zod", "package.json"), []byte(`{"version":"9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"packages":{"node_modules/zod":{"version":"1.0.0"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := DetectInstalledVersion("zod", dir)
	if got != "9.9.9" {
		t.Fatalf("DetectInstalledVersion = %q, want node_modules version", got)
	}
}

func TestDetectInstalledVersionFallsThroughPackageLockProtocol(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"packages":{"node_modules/zod":{"version":"file:../zod"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"zod":"^3.22.4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := DetectInstalledVersion("zod", dir)
	if got != "3.22.4" {
		t.Fatalf("DetectInstalledVersion = %q, want package.json fallback version", got)
	}
}

func TestPNPMFixture(t *testing.T) {
	content := readFixture(t, "pnpm-v9-workspace.yaml")
	if got := parsePNPMLock(content, "react"); got == "" {
		t.Fatal("react version empty")
	}
}

func TestYarnV1Fixture(t *testing.T) {
	content := readFixture(t, "yarn-v1.lock")
	if got := parseYarnLock(content, "@babel/core"); got == "" {
		t.Fatal("@babel/core version empty")
	}
}

func TestYarnBerryFixture(t *testing.T) {
	content := readFixture(t, "yarn-berry.lock")
	if got := parseYarnLock(content, "@types/react"); got == "" {
		t.Fatal("@types/react version empty")
	}
}

func TestSplitPackageSpec(t *testing.T) {
	tests := []struct {
		spec     string
		wantName string
		wantRest string
		wantOK   bool
	}{
		{spec: "zod@3.22.0", wantName: "zod", wantRest: "3.22.0", wantOK: true},
		{spec: "@scope/pkg@1.2.3", wantName: "@scope/pkg", wantRest: "1.2.3", wantOK: true},
		{spec: "zod@npm:^3.22.0", wantName: "zod", wantRest: "npm:^3.22.0", wantOK: true},
		{spec: "zod"},
		{spec: "@scope/pkg"},
	}
	for _, tt := range tests {
		gotName, gotRest, gotOK := splitPackageSpec(tt.spec)
		if gotName != tt.wantName || gotRest != tt.wantRest || gotOK != tt.wantOK {
			t.Fatalf("splitPackageSpec(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.spec, gotName, gotRest, gotOK, tt.wantName, tt.wantRest, tt.wantOK)
		}
	}
}

func TestStripPeerSuffix(t *testing.T) {
	tests := map[string]string{
		"18.0.0":                    "18.0.0",
		"18.0.0(react@18.0.0)":      "18.0.0",
		"15.5.15(a@1)(b@2(c@3))":    "15.5.15",
		"3.22.0  ":                  "3.22.0",
		"3.22.0(peer@1.0.0)   ":     "3.22.0",
		"1.0.0-alpha.1(peer@1.0.0)": "1.0.0-alpha.1",
	}
	for input, want := range tests {
		if got := stripPeerSuffix(input); got != want {
			t.Fatalf("stripPeerSuffix(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestStripPeerSuffixPNPMV5Underscore(t *testing.T) {
	tests := map[string]string{
		"18.2.0_react@18.2.0":  "18.2.0",
		"18.2.0(react@18.2.0)": "18.2.0",
		"1.0.0_custom-build.1": "1.0.0_custom-build.1",
		"1.0.0_build@metadata": "1.0.0",
		"1.0.0-alpha_peer@2.0": "1.0.0-alpha",
		"1.0.0-alpha_20240518": "1.0.0-alpha_20240518",
	}
	for input, want := range tests {
		if got := stripPeerSuffix(input); got != want {
			t.Fatalf("stripPeerSuffix(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCleanValue(t *testing.T) {
	tests := map[string]string{
		`  "1.2.3"  `:             "1.2.3",
		`  1.2.3 # comment`:       "1.2.3",
		`  '1.2.3' # comment`:     "1.2.3",
		`github:foo/bar#branch`:   "github:foo/bar#branch",
		`"github:foo/bar#branch"`: "github:foo/bar#branch",
	}
	for input, want := range tests {
		if got := cleanValue(input); got != want {
			t.Fatalf("cleanValue(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestStripInlineComment(t *testing.T) {
	tests := map[string]string{
		"1.2.3":                   "1.2.3",
		"1.2.3 # comment":         "1.2.3",
		"1.2.3  # trailing":       "1.2.3",
		"github:foo/bar#branch":   "github:foo/bar#branch",
		"https://x.test/a#branch": "https://x.test/a#branch",
	}
	for input, want := range tests {
		if got := stripInlineComment(input); got != want {
			t.Fatalf("stripInlineComment(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIsRegistryVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "1.2.3", want: true},
		{version: "1.0.0-beta.1", want: true},
		{version: "1.0.0-rc.1+build.5114f85", want: true},
		{version: ""},
		{version: "0.0.0-use.local"},
		{version: "link:../pkg"},
		{version: "file:./tarball.tgz"},
		{version: "workspace:*"},
		{version: "workspace:^1.0.0"},
		{version: "portal:../pkg"},
		{version: "github:owner/repo"},
		{version: "git+https://example.com/repo.git"},
		{version: "http://example.com/pkg.tgz"},
		{version: "https://example.com/pkg.tgz"},
		{version: "npm:other-pkg@^1"},
	}
	for _, tt := range tests {
		if got := isRegistryVersion(tt.version); got != tt.want {
			t.Fatalf("isRegistryVersion(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

func TestPackageLockV6(t *testing.T) {
	content := `{"dependencies":{"zod":{"version":"3.22.4"}}}`
	if got := parsePackageLock(content, "zod"); got != "3.22.4" {
		t.Fatalf("parsePackageLock = %q", got)
	}
}

func TestPackageJSONAbsentAndInvalid(t *testing.T) {
	content := `{"dependencies":{"zod":"^3.22.0"}}`
	if got := parsePackageJSONVersion(content, "not-there"); got != "" {
		t.Fatalf("parsePackageJSONVersion absent = %q, want empty", got)
	}
	if got := parsePackageJSONVersion("not json", "zod"); got != "" {
		t.Fatalf("parsePackageJSONVersion invalid = %q, want empty", got)
	}
}

func TestDetectInstalledVersionFallsBackToPackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"zod":"^3.22.4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DetectInstalledVersion("zod", dir); got != "3.22.4" {
		t.Fatalf("DetectInstalledVersion = %q, want package.json version", got)
	}
}

func TestDetectInstalledVersionScopedNodeModules(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "@scope", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "@scope", "pkg", "package.json"), []byte(`{"version":"1.2.3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DetectInstalledVersion("@scope/pkg", dir); got != "1.2.3" {
		t.Fatalf("DetectInstalledVersion = %q, want scoped node_modules version", got)
	}
}

func TestPNPMTopLevelForms(t *testing.T) {
	v5 := `lockfileVersion: '5.4'

specifiers:
  zod: ^3.22.0

dependencies:
  zod: 3.22.0

packages:
  /zod/3.22.0:
    resolution: {}
`
	if got := parsePNPMLock(v5, "zod"); got != "3.22.0" {
		t.Fatalf("parsePNPMLock v5 = %q", got)
	}

	v6 := `lockfileVersion: '6.0'

dependencies:
  zod:
    specifier: ^3.22.0
    version: 3.22.0

packages:
  /zod@3.22.0:
    resolution: {}
`
	if got := parsePNPMLock(v6, "zod"); got != "3.22.0" {
		t.Fatalf("parsePNPMLock v6 = %q", got)
	}
}

func TestPNPMDirectImporterDependency(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      react-dom:
        specifier: ^18.0.0
        version: 18.2.0(react@18.0.0)
`
	if got := parsePNPMLock(content, "react-dom"); got != "18.2.0" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMIgnoresPeerSuffixFalseMatch(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      react-dom:
        specifier: ^18.0.0
        version: 18.2.0(react@17.0.0)
`
	if got := parsePNPMLock(content, "react"); got != "" {
		t.Fatalf("parsePNPMLock = %q, want empty", got)
	}
}

func TestPNPMMultipleImportersFirstWins(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      zod:
        specifier: ^3.22.0
        version: 3.22.0
  apps/docs:
    dependencies:
      zod:
        specifier: ^3.23.0
        version: 3.23.0
`
	if got := parsePNPMLock(content, "zod"); got != "3.22.0" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMDevDependenciesInImporter(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  .:
    devDependencies:
      typescript:
        specifier: ^5.0.0
        version: 5.4.5
`
	if got := parsePNPMLock(content, "typescript"); got != "5.4.5" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMOptionalDependenciesInImporter(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  .:
    optionalDependencies:
      fsevents:
        specifier: ^2.3.0
        version: 2.3.3
`
	if got := parsePNPMLock(content, "fsevents"); got != "2.3.3" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMTransitiveViaSnapshots(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      next:
        specifier: ^14
        version: 14.0.0(react@18.2.0)

packages:
  foo@1.0.0:
    resolution: {}
  next@14.0.0:
    resolution: {}
  react@18.2.0:
    resolution: {}

snapshots:
  foo@1.0.0: {}
  next@14.0.0(react@18.2.0):
    dependencies:
      foo: 1.0.0
      react: 18.2.0
  react@18.2.0: {}
`
	if got := parsePNPMLock(content, "foo"); got != "1.0.0" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMV5TransitiveSlashPackageKeys(t *testing.T) {
	content := `lockfileVersion: '5.4'

dependencies:
  react: 18.2.0

packages:
  /react/18.2.0:
    dependencies:
      loose-envify: 1.4.0
  /loose-envify/1.4.0:
    resolution: {}
`
	if got := parsePNPMLock(content, "loose-envify"); got != "1.4.0" {
		t.Fatalf("parsePNPMLock = %q, want transitive loose-envify version", got)
	}
}

func TestPNPMV5PeerSuffixDirectDependency(t *testing.T) {
	content := `lockfileVersion: '5.4'

dependencies:
  react-dom: 18.2.0_react@18.2.0

packages:
  /react-dom/18.2.0_react@18.2.0:
    resolution: {}
`
	if got := parsePNPMLock(content, "react-dom"); got != "18.2.0" {
		t.Fatalf("parsePNPMLock = %q, want peer-stripped react-dom version", got)
	}
}

func TestPNPMV5PeerSuffixPackageKeyFallback(t *testing.T) {
	content := `lockfileVersion: '5.4'

packages:
  /react-dom/18.2.0_react@18.2.0:
    resolution: {}
`
	if got := parsePNPMLock(content, "react-dom"); got != "18.2.0" {
		t.Fatalf("parsePNPMLock = %q, want peer-stripped react-dom version", got)
	}
}

func TestPNPMTransitiveFallsBackToPackagesWhenUnreachable(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      zod:
        specifier: ^3
        version: 3.22.0

packages:
  unused@9.9.9:
    resolution: {}
  zod@3.22.0:
    resolution: {}

snapshots:
  zod@3.22.0: {}
`
	if got := parsePNPMLock(content, "unused"); got != "9.9.9" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMTransitiveHandlesCycles(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      a:
        specifier: ^1
        version: 1.0.0

snapshots:
  a@1.0.0:
    dependencies:
      b: 1.0.0
  b@1.0.0:
    dependencies:
      a: 1.0.0
      target: 2.0.0
  target@2.0.0: {}
`
	if got := parsePNPMLock(content, "target"); got != "2.0.0" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMNoContentCases(t *testing.T) {
	if got := parsePNPMLock("", "zod"); got != "" {
		t.Fatalf("empty file = %q, want empty", got)
	}
	if got := parsePNPMLock("# just a comment\n# another one\n", "zod"); got != "" {
		t.Fatalf("comment-only file = %q, want empty", got)
	}

	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      react:
        specifier: ^18.0.0
        version: 18.0.0
`
	if got := parsePNPMLock(content, "zod"); got != "" {
		t.Fatalf("absent package = %q, want empty", got)
	}
}

func TestPNPMScopedDirectAndFallback(t *testing.T) {
	direct := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      '@scope/pkg':
        specifier: ^1.0.0
        version: 1.2.3
`
	if got := parsePNPMLock(direct, "@scope/pkg"); got != "1.2.3" {
		t.Fatalf("scoped direct = %q", got)
	}

	fallback := `lockfileVersion: '9.0'

packages:
  '@scope/pkg@1.2.3':
    resolution: {}
`
	if got := parsePNPMLock(fallback, "@scope/pkg"); got != "1.2.3" {
		t.Fatalf("scoped fallback = %q", got)
	}
}

func TestPNPMPackagesAndSnapshotsFallback(t *testing.T) {
	packages := `lockfileVersion: '9.0'

packages:
  zod@3.22.0:
    resolution: {}
`
	if got := parsePNPMLock(packages, "zod"); got != "3.22.0" {
		t.Fatalf("packages fallback = %q", got)
	}

	snapshots := `lockfileVersion: '9.0'

snapshots:
  zod@3.22.0: {}
`
	if got := parsePNPMLock(snapshots, "zod"); got != "3.22.0" {
		t.Fatalf("snapshots fallback = %q", got)
	}
}

func TestPNPMSkipsLinkVersionInTopLevelDeps(t *testing.T) {
	content := `lockfileVersion: '6.0'

dependencies:
  my-lib: link:../my-lib
`
	if got := parsePNPMLock(content, "my-lib"); got != "" {
		t.Fatalf("parsePNPMLock = %q, want empty", got)
	}
}

func TestPNPMSlashAtPackageKeyFallback(t *testing.T) {
	content := `lockfileVersion: '9.0'

packages:
  /zod@3.22.0:
    resolution: {}
`
	if got := parsePNPMLock(content, "zod"); got != "3.22.0" {
		t.Fatalf("parsePNPMLock = %q, want slash-at zod version", got)
	}
}

func TestPNPMScopedSlashAtPackageKeyFallback(t *testing.T) {
	content := `lockfileVersion: '9.0'

packages:
  /@scope/pkg@1.2.3:
    resolution: {}
`
	if got := parsePNPMLock(content, "@scope/pkg"); got != "1.2.3" {
		t.Fatalf("parsePNPMLock = %q, want scoped slash-at version", got)
	}
}

func TestPNPMSlashAtPackageKeyTransitive(t *testing.T) {
	content := `lockfileVersion: '9.0'

dependencies:
  a: 1.0.0

packages:
  /a@1.0.0:
    dependencies:
      zod: 3.22.0
  /zod@3.22.0:
    resolution: {}
`
	if got := parsePNPMLock(content, "zod"); got != "3.22.0" {
		t.Fatalf("parsePNPMLock = %q, want transitive slash-at zod version", got)
	}
}

func TestPNPMSkipsLinkAndFileProtocols(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  apps/web:
    dependencies:
      linked:
        specifier: workspace:^
        version: link:../../packages/linked
      tarball:
        specifier: file:./pkg.tgz
        version: file:pkg.tgz
  apps/docs:
    dependencies:
      linked:
        specifier: ^1.2.3
        version: 1.2.3
`
	if got := parsePNPMLock(content, "tarball"); got != "" {
		t.Fatalf("tarball = %q, want empty", got)
	}
	if got := parsePNPMLock(content, "linked"); got != "1.2.3" {
		t.Fatalf("linked = %q, want later registry version", got)
	}
}

func TestPNPMIndentRelativeParses4SpaceIndent(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
    .:
        dependencies:
            zod:
                specifier: ^3.22.0
                version: 3.22.0
`
	if got := parsePNPMLock(content, "zod"); got != "3.22.0" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMInlineCommentStripped(t *testing.T) {
	content := `lockfileVersion: '9.0'

dependencies:
  zod: 3.22.0 # pinned
`
	if got := parsePNPMLock(content, "zod"); got != "3.22.0" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMCRLFLineEndings(t *testing.T) {
	content := "lockfileVersion: '9.0'\r\n\r\nimporters:\r\n  .:\r\n    dependencies:\r\n      zod:\r\n        specifier: ^3.22.0\r\n        version: 3.22.0\r\n"
	if got := parsePNPMLock(content, "zod"); got != "3.22.0" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMTransitivePicksReachableVersion(t *testing.T) {
	content := `lockfileVersion: '9.0'

importers:
  .:
    dependencies:
      next:
        specifier: ^14
        version: 14.0.0(react@18.2.0)

snapshots:
  next@14.0.0(react@18.2.0):
    dependencies:
      react: 18.2.0
  react@17.0.0: {}
  react@18.2.0: {}
`
	if got := parsePNPMLock(content, "react"); got != "18.2.0" {
		t.Fatalf("parsePNPMLock = %q", got)
	}
}

func TestPNPMFixtureTransitiveViaBFS(t *testing.T) {
	content := readFixture(t, "pnpm-v9-workspace.yaml")
	if got := parsePNPMLock(content, "js-tokens"); got != "4.0.0" {
		t.Fatalf("js-tokens = %q", got)
	}
	if got := parsePNPMLock(content, "scheduler"); got != "0.23.0" {
		t.Fatalf("scheduler = %q", got)
	}
}

func TestPNPMFixtureExactVersions(t *testing.T) {
	content := readFixture(t, "pnpm-v9-workspace.yaml")
	tests := map[string]string{
		"next":         "14.0.0",
		"@types/react": "18.2.45",
		"react":        "18.2.0",
		"typescript":   "5.3.3",
	}
	for name, want := range tests {
		if got := parsePNPMLock(content, name); got != want {
			t.Fatalf("parsePNPMLock(%q) = %q, want %q", name, got, want)
		}
	}
	if got := parsePNPMLock(content, "definitely-not-here"); got != "" {
		t.Fatalf("parsePNPMLock absent = %q, want empty", got)
	}
}

func TestYarnScopedCases(t *testing.T) {
	v1 := "# yarn lockfile v1\n\n\n\"@scope/pkg@^1.0.0\":\n  version \"1.0.0\"\n"
	if got := parseYarnLock(v1, "@scope/pkg"); got != "1.0.0" {
		t.Fatalf("yarn v1 scoped = %q", got)
	}

	berry := "__metadata:\n  version: 6\n\n\"@scope/pkg@npm:^1.0.0\":\n  version: 1.2.3\n"
	if got := parseYarnLock(berry, "@scope/pkg"); got != "1.2.3" {
		t.Fatalf("yarn berry scoped = %q", got)
	}
}

func TestYarnV1SingleSpecifier(t *testing.T) {
	content := "# yarn lockfile v1\n\n\n\"zod@^3.22.0\":\n  version \"3.22.0\"\n  resolved \"https://registry.yarnpkg.com/zod/-/zod-3.22.0.tgz\"\n"
	if got := parseYarnLock(content, "zod"); got != "3.22.0" {
		t.Fatalf("parseYarnLock = %q", got)
	}
}

func TestYarnNoContentCases(t *testing.T) {
	if got := parseYarnLock("", "zod"); got != "" {
		t.Fatalf("empty file = %q, want empty", got)
	}

	content := "# yarn lockfile v1\n\n\n\"foo@^1.0.0\":\n  version \"1.0.0\"\n"
	if got := parseYarnLock(content, "zod"); got != "" {
		t.Fatalf("absent package = %q, want empty", got)
	}
}

func TestYarnSkipsMetadataBlock(t *testing.T) {
	content := "__metadata:\n  version: 6\n"
	if got := parseYarnLock(content, "__metadata"); got != "" {
		t.Fatalf("parseYarnLock = %q, want empty", got)
	}
}

func TestYarnV1MultiSpecifier(t *testing.T) {
	content := "# yarn lockfile v1\n\n\n\"foo@^1.0.0\":\n  version \"1.0.0\"\n\n\"bar@^1.0.0\", \"bar@~1.2.0\":\n  version \"1.2.3\"\n"
	if got := parseYarnLock(content, "bar"); got != "1.2.3" {
		t.Fatalf("parseYarnLock = %q", got)
	}
}

func TestYarnBerryNPMProtocol(t *testing.T) {
	content := "# This file is generated by running \"yarn install\".\n\n__metadata:\n  version: 6\n  cacheKey: 8\n\n\"zod@npm:^3.22.0\":\n  version: 3.22.0\n  resolution: \"zod@npm:3.22.0\"\n"
	if got := parseYarnLock(content, "zod"); got != "3.22.0" {
		t.Fatalf("parseYarnLock = %q", got)
	}
}

func TestYarnBerryCommaSeparatedSpecifiers(t *testing.T) {
	content := "__metadata:\n  version: 6\n\n\"foo@npm:^1.0.0, foo@workspace:*\":\n  version: 1.2.3\n  resolution: \"foo@npm:1.2.3\"\n"
	if got := parseYarnLock(content, "foo"); got != "1.2.3" {
		t.Fatalf("parseYarnLock = %q", got)
	}
}

func TestYarnBerrySkipsWorkspaceRootSentinel(t *testing.T) {
	content := "__metadata:\n  version: 6\n\n\"myproject@workspace:.\":\n  version: 0.0.0-use.local\n  resolution: \"myproject@workspace:.\"\n"
	if got := parseYarnLock(content, "myproject"); got != "" {
		t.Fatalf("parseYarnLock = %q, want empty", got)
	}
}

func TestYarnInlineComments(t *testing.T) {
	v1 := "# yarn lockfile v1\n\n\n\"zod@^3.22.0\":\n  version \"3.22.0\" # pinned\n"
	if got := parseYarnLock(v1, "zod"); got != "3.22.0" {
		t.Fatalf("yarn v1 inline comment = %q", got)
	}

	berry := "__metadata:\n  version: 6\n\n\"zod@npm:^3.22.0\":\n  version: 3.22.0 # pinned\n"
	if got := parseYarnLock(berry, "zod"); got != "3.22.0" {
		t.Fatalf("yarn berry inline comment = %q", got)
	}
}

func TestYarnWorkspaceBlockDoesNotBlockLaterRealBlock(t *testing.T) {
	content := "__metadata:\n  version: 6\n\n\"foo@workspace:packages/foo\":\n  version: 0.0.0-use.local\n  resolution: \"foo@workspace:packages/foo\"\n\n\"foo@npm:^1.0.0\":\n  version: 1.2.3\n  resolution: \"foo@npm:1.2.3\"\n"
	if got := parseYarnLock(content, "foo"); got != "1.2.3" {
		t.Fatalf("parseYarnLock = %q", got)
	}
}

func TestYarnCRLFLineEndings(t *testing.T) {
	content := "# yarn lockfile v1\r\n\r\n\r\n\"zod@^3.22.0\":\r\n  version \"3.22.0\"\r\n"
	if got := parseYarnLock(content, "zod"); got != "3.22.0" {
		t.Fatalf("parseYarnLock = %q", got)
	}
}

func TestYarnSkipsProtocolVersions(t *testing.T) {
	content := "# yarn lockfile v1\n\n\n\"my-lib@file:../my-lib\":\n  version \"file:../my-lib\"\n"
	if got := parseYarnLock(content, "my-lib"); got != "" {
		t.Fatalf("parseYarnLock = %q, want empty", got)
	}
}

func TestYarnFixturesExactVersions(t *testing.T) {
	yarnV1 := readFixture(t, "yarn-v1.lock")
	if got := parseYarnLock(yarnV1, "@babel/core"); got != "7.23.0" {
		t.Fatalf("@babel/core = %q", got)
	}
	if got := parseYarnLock(yarnV1, "lodash"); got != "4.17.21" {
		t.Fatalf("lodash = %q", got)
	}
	for name, want := range map[string]string{
		"react":      "18.2.0",
		"typescript": "5.3.3",
		"zod":        "3.22.4",
	} {
		if got := parseYarnLock(yarnV1, name); got != want {
			t.Fatalf("yarn v1 %s = %q, want %q", name, got, want)
		}
	}
	if got := parseYarnLock(yarnV1, "not-installed-anywhere"); got != "" {
		t.Fatalf("yarn v1 absent = %q, want empty", got)
	}

	yarnBerry := readFixture(t, "yarn-berry.lock")
	if got := parseYarnLock(yarnBerry, "@types/react"); got != "18.2.45" {
		t.Fatalf("@types/react = %q", got)
	}
	if got := parseYarnLock(yarnBerry, "typescript"); got != "5.3.3" {
		t.Fatalf("typescript = %q", got)
	}
	for name, want := range map[string]string{
		"lodash": "4.17.21",
		"react":  "18.2.0",
	} {
		if got := parseYarnLock(yarnBerry, name); got != want {
			t.Fatalf("yarn berry %s = %q, want %q", name, got, want)
		}
	}
	if got := parseYarnLock(yarnBerry, "not-installed-anywhere"); got != "" {
		t.Fatalf("yarn berry absent = %q, want empty", got)
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
