package maven

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEffectiveRepositoriesReadsPomParentProfilesSettingsAndMirrors(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "parent")
	childDir := filepath.Join(root, "service")
	settingsPath := filepath.Join(root, "settings.xml")
	writeText(t, filepath.Join(parentDir, "pom.xml"), `<project>
  <repositories>
    <repository>
      <id>parent-releases</id>
      <url>https://repo.example.com/parent</url>
    </repository>
  </repositories>
</project>`)
	writeText(t, filepath.Join(childDir, "pom.xml"), `<project>
  <parent>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
    <version>1.0.0</version>
    <relativePath>../parent/pom.xml</relativePath>
  </parent>
  <repositories>
    <repository>
      <id>service-releases</id>
      <url>https://repo.example.com/service</url>
    </repository>
  </repositories>
  <profiles>
    <profile>
      <id>local-default</id>
      <activation>
        <activeByDefault>true</activeByDefault>
      </activation>
      <repositories>
        <repository>
          <id>default-profile-releases</id>
          <url>https://repo.example.com/default-profile</url>
        </repository>
      </repositories>
    </profile>
  </profiles>
</project>`)
	writeText(t, settingsPath, `<settings>
  <activeProfiles>
    <activeProfile>corp</activeProfile>
  </activeProfiles>
  <profiles>
    <profile>
      <id>corp</id>
      <repositories>
        <repository>
          <id>corp-releases</id>
          <url>https://repo.example.com/corp</url>
        </repository>
      </repositories>
    </profile>
    <profile>
      <id>settings-default</id>
      <activation>
        <activeByDefault>true</activeByDefault>
      </activation>
      <repositories>
        <repository>
          <id>settings-default-releases</id>
          <url>https://repo.example.com/settings-default</url>
        </repository>
      </repositories>
    </profile>
  </profiles>
  <mirrors>
    <mirror>
      <id>corp-service-mirror</id>
      <mirrorOf>service-releases</mirrorOf>
      <url>https://repo.example.com/service-mirror</url>
    </mirror>
  </mirrors>
</settings>`)

	got := EffectiveRepositoriesWithSettings(childDir, settingsPath)
	want := []Repository{
		{ID: "corp-releases", URL: "https://repo.example.com/corp"},
		{ID: "settings-default-releases", URL: "https://repo.example.com/settings-default"},
		{ID: "parent-releases", URL: "https://repo.example.com/parent"},
		{ID: "service-releases", URL: "https://repo.example.com/service-mirror"},
		{ID: "default-profile-releases", URL: "https://repo.example.com/default-profile"},
		{ID: "central", URL: DefaultRepository},
	}
	assertRepositories(t, got, want)
}

func TestEffectiveRepositoriesUsesMavenCentralDefaultWhenNoProjectRepositories(t *testing.T) {
	root := t.TempDir()
	writeText(t, filepath.Join(root, "pom.xml"), `<project></project>`)

	got := EffectiveRepositoriesWithSettings(root, filepath.Join(root, "missing-settings.xml"))
	assertRepositories(t, got, []Repository{{ID: "central", URL: DefaultRepository}})
}

func TestEffectiveRepositoriesDoesNotAppendCentralWhenProjectOverridesCentral(t *testing.T) {
	root := t.TempDir()
	writeText(t, filepath.Join(root, "pom.xml"), `<project>
  <repositories>
    <repository>
      <id>central</id>
      <url>https://repo.example.com/internal-central</url>
    </repository>
  </repositories>
</project>`)

	got := EffectiveRepositoriesWithSettings(root, filepath.Join(root, "missing-settings.xml"))
	assertRepositories(t, got, []Repository{{ID: "central", URL: "https://repo.example.com/internal-central"}})
}

func TestApplyMirrorsSupportsWildcardExternalAndExclusion(t *testing.T) {
	repos := []Repository{
		{ID: "internal", URL: "https://repo.example.com/internal"},
		{ID: "central", URL: DefaultRepository},
		{ID: "local-file", URL: "file:///tmp/m2"},
	}
	mirrors := []mirror{
		{ID: "all-external", MirrorOf: "external:*,!internal", URL: "https://repo.example.com/proxy"},
		{ID: "local", MirrorOf: "local-file", URL: "https://repo.example.com/all"},
	}

	got := applyMirrors(repos, mirrors)
	want := []Repository{
		{ID: "internal", URL: "https://repo.example.com/internal"},
		{ID: "central", URL: "https://repo.example.com/proxy"},
		{ID: "local-file", URL: "https://repo.example.com/all"},
	}
	assertRepositories(t, got, want)
}

func writeText(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertRepositories(t *testing.T, got, want []Repository) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("repositories = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("repositories[%d] = %#v, want %#v; all = %#v", i, got[i], want[i], got)
		}
	}
}
