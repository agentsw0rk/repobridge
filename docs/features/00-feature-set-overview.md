# Feature Set: RepoBridge

RepoBridge ist eine Go-CLI, die Paket- und Repository-Spezifikationen in stabile lokale Quellcode-Pfade auflöst. Die Übersicht beschreibt das aktuelle Feature-Set und ordnet mögliche Folgearbeiten so, dass jede Einheit eigenständig implementierbar bleibt.

## Produktziel

Coding Agents und Entwickler-Tools sollen mit einem einfachen Kommando den passenden Quellcode zu einer Dependency oder einem Repository erhalten. RepoBridge übernimmt dafür Registry-Auflösung, Git- oder Archiv-Fetching, Cache-Verwaltung und maschinenlesbare Ausgabe.

## Aktueller Umfang

| # | Feature | Status | Implementierung | Aufwand |
|---|---------|--------|-----------------|---------|
| 1 | CLI-Grundgerüst mit Version, Fehlerausgabe und Cobra-Kommandos | Fertig | `cmd/repobridge`, `internal/cli` | S |
| 2 | Paket-Spezifikation parsen und Registry erkennen | Fertig | `internal/registry` | S |
| 3 | npm-Pakete auf Repository, Version und Monorepo-Unterverzeichnis auflösen | Fertig | `internal/registry/npm` | M |
| 4 | PyPI-Pakete auf Repository und Version auflösen | Fertig | `internal/registry/pypi` | M |
| 5 | crates.io-Pakete auf Repository und Version auflösen | Fertig | `internal/registry/crates` | M |
| 6 | Maven-Koordinaten auf Source-JAR oder SCM-Fallback auflösen | Fertig | `internal/registry/maven`, `internal/source` | L |
| 7 | GitHub-, GitLab- und Bitbucket-Repositories inklusive Default-Branch auflösen | Fertig | `internal/registry/repo` | M |
| 8 | Quellen per Git klonen, Tag/Ref auswählen und `.git` aus dem Cache entfernen | Fertig | `internal/source`, `internal/git` | M |
| 9 | Maven Source-JARs begrenzt und pfadsicher herunterladen und extrahieren | Fertig | `internal/source/archive_fetcher.go` | M |
| 10 | Stabiler lokaler Cache mit `sources.json` Index und `REPOBRIDGE_HOME` | Fertig | `internal/cache` | M |
| 11 | `path` Kommando: Cache-Miss füllen und absolute Pfade ausgeben | Fertig | `internal/cli`, `internal/source` | S |
| 12 | `fetch` Kommando: mehrere Quellen vorab in den Cache laden | Fertig | `internal/cli` | S |
| 13 | `list --json` und menschenlesbare Cache-Übersicht | Fertig | `internal/cli`, `internal/cache` | S |
| 14 | `remove` und `clean` für gezielte Cache-Bereinigung | Fertig | `internal/cli`, `internal/cache` | M |
| 15 | npm-Version aus `node_modules`, `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock` und `package.json` erkennen | Fertig | `internal/lockfile` | M |
| 16 | Token-Unterstützung für private oder rate-limitierte Repository-APIs | Fertig | `GITHUB_TOKEN`, `GITLAB_TOKEN`, `BITBUCKET_TOKEN` | S |
| 17 | NuGet-Pakete über `.nuspec` Repository-Metadaten auf Git-Sources auflösen | Fertig | [17-nuget-sources.md](17-nuget-sources.md) | L |
| 18 | Projekt-Dependencies aus Manifesten, Lockfiles und Imports als RepoBridge-Specs scannen | Fertig | `internal/projectscan`, `repobridge scan` | M |

## Unterstützte Eingaben

| Typ | Beispiele | Verhalten |
|-----|-----------|-----------|
| npm | `react`, `react@19.0.0`, `@scope/pkg@1.2.3` | Ohne Prefix Standard-Registry; ohne Version wird `latest` oder lokal erkannte Version genutzt. |
| PyPI | `pypi:requests`, `pypi:requests==2.32.3`, `python:requests@2.32.3` | Resolver nutzt PyPI-Metadaten und normalisiert Repository-URLs. |
| crates.io | `crates:serde`, `cargo:serde@1.0.217` | Resolver nutzt crates.io-Metadaten und Git-Tags. |
| Maven | `maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0` | Source-JAR hat Vorrang; bei 404 wird SCM aus dem POM versucht. |
| NuGet | `nuget:Newtonsoft.Json`, `nuget:Newtonsoft.Json@13.0.3`, `dotnet:Serilog@3.1.1` | `.nupkg` wird nur für `.nuspec`-Metadaten gelesen; Source kommt aus Git-Repository-Metadaten. |
| GitHub | `vercel/next.js`, `github:vercel/next.js`, `github.com/vercel/next.js` | Ohne Ref wird der Default-Branch über die Host-API ermittelt. |
| GitLab | `gitlab:group/project`, `gitlab.com/group/subgroup/project` | Projektpfade mit Subgroups werden unterstützt. |
| Bitbucket | `bitbucket:workspace/repo`, `bitbucket.org/workspace/repo` | Default-Branch wird über die Bitbucket-API ermittelt. |
| URL | `https://github.com/vercel/next.js/tree/canary` | Unterstützte Host-URLs werden normalisiert; Tree/Blob-Refs werden erkannt. |

## CLI-Kommandos

| Kommando | Zweck | Wichtige Flags |
|----------|-------|----------------|
| `repobridge path <spec...>` | Gibt absolute Quellcode-Pfade aus und lädt bei Cache-Miss nach. | `--cwd`, `--verbose` |
| `repobridge fetch <spec...>` | Lädt Quellen in den Cache, ohne Pfade als Hauptausgabe zu verwenden. | `--cwd`, `--quiet` |
| `repobridge scan` | Erkennt Dependency-Specs in einem Projekt und kann sie optional direkt cachen. | `--cwd`, `--json`, `--fetch`, `--limit`, `--no-imports` |
| `repobridge list` | Listet gecachte Packages und Repositories. | `--json` |
| `repobridge remove <spec...>` | Entfernt konkrete Cache-Einträge. | Alias: `rm` |
| `repobridge clean` | Entfernt Cache-Einträge nach Typ oder Registry. | `--packages`, `--repos`, `--npm`, `--pypi`, `--crates`, `--maven`, `--nuget` |

## Projektarchitektur

| Bereich | Paket | Aufgabe |
|---------|-------|---------|
| CLI | `internal/cli` | Cobra-Kommandos, Ausgabeformat, Flags und Fehlergrenzen. |
| Source-Orchestrierung | `internal/source` | Entscheidet zwischen Package- und Repo-Fetching, Cache-Reuse und Index-Updates. |
| Cache | `internal/cache` | Cache-Home, relative Pfadsicherheit, `sources.json`, Remove/Clean-Operationen. |
| Registry-Erkennung | `internal/registry` | Input-Klassifikation, Prefixe, Package-Spec-Parser, Repository-URL-Normalisierung. |
| Registry-Resolver | `internal/registry/{npm,pypi,crates,maven,nuget}` | Registry-spezifische Metadatenabfragen und Source-Informationen. |
| Repository-Resolver | `internal/registry/repo` | GitHub/GitLab/Bitbucket-Spezifikationen und Default-Branch-Abfragen. |
| Git | `internal/git` | Klonen nach Tag oder Ref und Entfernen des eingebetteten Git-Verzeichnisses. |
| Lockfiles | `internal/lockfile` | Lokale npm-Versionserkennung aus installierten Paketen und Lockfiles. |
| Projekt-Scan | `internal/projectscan` | Manifest-, Lockfile- und Import-Erkennung für `repobridge scan`. |
| HTTP | `internal/httpx` | Gemeinsamer HTTP-Client mit Timeout. |

## Entwicklungsreihenfolge für Folgefeatures

| # | Feature | Abhängig von | Aufwand |
|---|---------|--------------|---------|
| 1 | Custom Registry URLs für npm, PyPI, crates.io und Maven | Resolver-Konfiguration | M |
| 2 | Checksum- und Signaturprüfung für heruntergeladene Archive | Archiv-Fetching | M |
| 3 | Snapshot- und Metadata-Auflösung für Maven-Versionen | Maven-Resolver | L |
| 4 | Gradle- und Maven-Projektdateien zur lokalen Versionserkennung auswerten | Lockfile-Erkennung | M |
| 5 | Cache-Statuskommando mit Größen, Alter und beschädigten Einträgen | Cache-Index | M |
| 6 | Optionales JSON-Ausgabeformat für `path` und `fetch` | CLI-Ausgabe | S |
| 7 | Konfigurationsdatei für Standard-Registry, Cache-Policy und Token-Hinweise | CLI + Cache | M |

## Qualität und Tests

| Thema | Erwartung |
|-------|-----------|
| Parser und Resolver | Table-driven Tests für akzeptierte, abgelehnte und normalisierte Eingaben. |
| Cache-Operationen | Tests mit temporärem `REPOBRIDGE_HOME`; keine echten Home-Verzeichnisse verändern. |
| CLI-Verhalten | Kommandotests mit injiziertem App-Interface und deterministischer Ausgabe. |
| Fetching | HTTP- und Git-Grenzen mocken; keine Netzwerkabhängigkeit in Unit-Tests. |
| Sicherheitsgrenzen | Tests für Pfad-Traversal, Archivlimits, URL-Normalisierung und fehlerhafte Cache-Indizes. |

## Weiterführende Dokumentation

| Thema | Datei |
|-------|-------|
| Projekt-README und Nutzerbeispiele | [../../README.md](../../README.md) |
| Maven-Source-Support Abschlussnotiz | [maven-sources-done.md](maven-sources-done.md) |
| Maven-Source-Implementierungsplan | [../superpowers/plans/2026-05-18-maven-sources.md](../superpowers/plans/2026-05-18-maven-sources.md) |
| Maven-Source-Designspec | [../superpowers/specs/2026-05-18-maven-sources-design.md](../superpowers/specs/2026-05-18-maven-sources-design.md) |
| NuGet-Source-Feature | [17-nuget-sources.md](17-nuget-sources.md) |
| NuGet-Source-Support Abschlussnotiz | [17-nuget-sources-done.md](17-nuget-sources-done.md) |
| NuGet-Source-Designspec | [../superpowers/specs/2026-05-19-nuget-sources-design.md](../superpowers/specs/2026-05-19-nuget-sources-design.md) |

## Tech-Stack

| Kategorie | Technologie |
|-----------|-------------|
| Sprache | Go 1.22+ |
| CLI Framework | Cobra |
| Tests | Go `testing` Package |
| Quellcode-Fetching | `git` auf dem `PATH`, HTTP Downloads |
| Persistenz | Lokales Dateisystem unter `REPOBRIDGE_HOME` oder `~/.repobridge` |
| Unterstützte Paketquellen | npm, PyPI, crates.io, Maven Central, NuGet |
| Unterstützte Repository-Hosts | GitHub, GitLab, Bitbucket |
