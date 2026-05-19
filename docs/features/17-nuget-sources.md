# Feature 17: NuGet Sources

## Problem

RepoBridge kann aktuell npm, PyPI, crates.io, Maven und direkte Repository-Spezifikationen auf lokale Quellcode-Pfade auflösen. .NET-Projekte verwenden jedoch NuGet-Pakete, und Coding Agents brauchen auch für diese Dependencies den passenden Quellcode statt nur kompilierten Paketinhalt.

NuGet-Pakete enthalten in `.nupkg`-Archiven häufig DLLs, Metadaten und Assets, aber nicht zuverlässig den eigentlichen Quellcode. Deshalb soll RepoBridge NuGet nicht als Archiv-Source behandeln, sondern die Paketmetadaten nutzen, um ein echtes Git-Repository reproduzierbar zu klonen.

## Ziel

RepoBridge unterstützt NuGet-Paket-Spezifikationen:

```bash
repobridge path nuget:Newtonsoft.Json@13.0.3
repobridge path dotnet:Newtonsoft.Json@13.0.3
repobridge path nuget:Newtonsoft.Json
```

Ohne Version wird die neueste stabile Version gewählt. Prerelease-Versionen werden nur akzeptiert, wenn sie explizit angegeben werden.

## Nicht-Ziele

- Keine Nutzung entpackter `.nupkg`-Inhalte als Source-Fallback.
- Keine automatische Auswahl des Default-Branches, wenn weder Commit noch Versionstag nutzbar ist.
- Keine Unterstützung privater NuGet-Feeds in der ersten Iteration.
- Keine Auswertung lokaler `.csproj`, `packages.lock.json` oder `packages.config` Dateien in der ersten Iteration.

## Architektur

### Registry-Erkennung

`internal/registry` erhält eine neue Registry `NuGet` mit Label `NuGet`. Die Prefixe `nuget:` und `dotnet:` werden als Package-Input erkannt. Die Package-Syntax folgt dem bestehenden `name@version` Muster.

### NuGet Resolver

Ein neues Paket `internal/registry/nuget` kapselt die NuGet-v3-Auflösung:

1. Service Index laden.
2. `PackageBaseAddress/3.0.0` und `RegistrationsBaseUrl` finden.
3. Package-Version ermitteln:
   - Explizite Version prüfen.
   - Ohne Version neueste stabile Version aus der Versionsliste wählen.
4. `.nupkg` temporär über den Flat-Container laden.
5. `.nuspec` aus dem Paketarchiv lesen.
6. `<repository type="git" url="..." commit="..." branch="..." />` auswerten.
7. Repository-URL über `registry.NormalizeRepoURL` normalisieren.

Der Resolver gibt ein `registry.ResolvedPackage` mit `Registry: registry.NuGet`, kanonischem Paketnamen, Version, `RepoURL` und Ref-Informationen zurück.

### Ref-Auswahl

Die Ref-Auswahl ist reproduzierbar:

1. Wenn die `.nuspec` ein `repository`-`commit` enthält, wird dieser Commit als Git-Ref verwendet.
2. Ohne Commit wird `GitTag` auf `v<version>` gesetzt. Die bestehende Git-Logik probiert dadurch `v<version>` und `<version>`.
3. Wenn kein Commit und kein passender Versionstag klonbar ist, schlägt der Fetch fehl.

RepoBridge klont in diesem Feature keinen Default-Branch für NuGet-Pakete, weil dieser nicht zuverlässig zur Paketversion passt.

### Source Fetching

`internal/source.defaultResolvePackage` ruft für NuGet den neuen Resolver auf. Danach nutzt RepoBridge den bestehenden Git-Fetch-Pfad. Die `.nupkg` wird nur im Resolver temporär verarbeitet und nicht als Cache-Source gespeichert.

### CLI und Cache

`clean` erhält einen Registry-Filter:

```bash
repobridge clean --nuget
```

`list`, `fetch` und `remove` verwenden das neue Label `NuGet` über `registry.Registry.Label()`.

## Fehlerverhalten

| Fall | Verhalten |
|------|-----------|
| Package existiert nicht | `PackageNotFoundError` mit Registry `NuGet` |
| Explizite Version existiert nicht | `VersionNotFoundError` |
| Keine stabile Version ohne explizite Version | `VersionNotFoundError` |
| `.nupkg` fehlt oder Download schlägt fehl | HTTP- oder Download-Fehler |
| `.nuspec` fehlt im Paket | NuGet-spezifischer Resolver-Fehler |
| Kein nutzbares Git-Repository in `.nuspec` | `NoRepoURLError` |
| Repository-URL ist kein unterstützter Host | `NoRepoURLError` |
| Paketarchiv enthält unsichere Pfade | Archiv-Sicherheitsfehler |

## Betroffene Dateien

| Datei/Paket | Änderung |
|-------------|----------|
| `internal/registry/registry.go` | Registry-Konstante, Prefixe, Parser-Unterstützung |
| `internal/registry/nuget` | Neuer NuGet-v3-Resolver |
| `internal/source/source.go` | Resolver in `defaultResolvePackage` einbinden |
| `internal/cli/commands.go` | `clean --nuget` und Registry-Filter erweitern |
| `internal/source/archive_fetcher.go` oder NuGet-spezifische Helfer | Sichere temporäre `.nupkg`/`.nuspec` Verarbeitung |
| `README.md` | Nutzerbeispiele und unterstützte Eingaben ergänzen |
| `docs/features/00-feature-set-overview.md` | Feature-Set aktualisieren |

## Akzeptanzkriterien

- `repobridge path nuget:Newtonsoft.Json@13.0.3` löst NuGet-Metadaten auf und versucht den passenden Git-Ref zu klonen.
- `repobridge path dotnet:<id>@<version>` funktioniert identisch zu `nuget:<id>@<version>`.
- `nuget:<id>` ohne Version wählt die neueste stabile Version und ignoriert Prereleases.
- Eine explizite Prerelease-Version wird nicht abgelehnt.
- Ein `.nuspec`-Repository-Commit hat Vorrang vor Versionstags.
- Ohne Commit werden Versionstags probiert; kein Default-Branch-Fallback.
- Fehlt ein nutzbares Git-Repository, liefert RepoBridge einen klaren Fehler statt Paketbinärdateien zu cachen.
- `repobridge clean --nuget` entfernt nur NuGet-Cache-Einträge.
- `go test ./...` läuft erfolgreich.

## Abhängigkeiten

- Bestehende Registry-Erkennung und Package-Spec-Parser.
- Bestehender Git-Fetch-Pfad mit Tag/Ref-Unterstützung.
- Bestehende Cache-Index-Struktur für Package-Einträge.
- Bestehende sichere Archivlogik aus dem Maven-Source-JAR-Feature als Orientierung.

