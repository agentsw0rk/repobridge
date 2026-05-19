---
layout: default
title: RepoBridge Dokumentation
description: Fachliche und funktionale Dokumentation fuer RepoBridge
---

# RepoBridge Dokumentation

RepoBridge ist eine Go-CLI, die Paket- und Repository-Spezifikationen in stabile lokale Quellcode-Pfade aufloest. Das Werkzeug richtet sich an Coding Agents, Entwickler-Tools und Entwicklerinnen und Entwickler, die zu einer Dependency schnell den tatsaechlichen Quellcode lokal verfuegbar machen wollen.

## Was löst RepoBridge?

Viele Entwickler-Workflows brauchen nicht nur Paketnamen, sondern den Quellcode hinter einer Dependency. RepoBridge uebernimmt dafuer die Aufloesung ueber Paket-Registries oder Repository-Hosts, laedt die passenden Quellen herunter und legt sie in einem wiederverwendbaren lokalen Cache ab.

RepoBridge hilft besonders bei diesen Aufgaben:

- Quellcode zu npm-, PyPI-, crates.io-, Maven- und NuGet-Paketen finden.
- Git-Repositories von GitHub, GitLab und Bitbucket in stabile lokale Pfade bringen.
- Coding Agents mit maschinenfreundlichen Pfadangaben versorgen.
- Wiederholte Tool-Laeufe beschleunigen, weil bereits geladene Quellen aus dem Cache kommen.
- Lokale npm-Versionen aus `node_modules`, Lockfiles und `package.json` erkennen.

## Kernfunktionen

### Quellen zu Paketen abrufen

RepoBridge akzeptiert Paket-Spezifikationen und ermittelt daraus die passende Source-Quelle. Bei einem Cache-Miss wird die Quelle geladen; bei einem Cache-Hit wird der bestehende lokale Pfad wiederverwendet.

Beispiele:

```bash
repobridge path react
repobridge path react@19.0.0
repobridge path pypi:requests==2.32.3
repobridge path crates:serde@1.0.217
repobridge path maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0
repobridge path nuget:Newtonsoft.Json@13.0.3
```

Wenn bei npm keine Version angegeben ist, versucht RepoBridge zuerst, die lokal installierte Version zu erkennen. Unterstuetzt werden `node_modules`, `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock` und `package.json`.

### Quellen zu Repositories abrufen

Neben Paketnamen koennen auch Repository-Spezifikationen verwendet werden. Ohne explizite Referenz ermittelt RepoBridge den Default-Branch des Repository-Hosts.

Beispiele:

```bash
repobridge path vercel/next.js
repobridge path github.com/vercel/next.js
repobridge path gitlab.com/group/project
repobridge path bitbucket.org/workspace/repo
repobridge path https://github.com/vercel/next.js/tree/canary
```

### Cache vorbefuellen

Mit `fetch` lassen sich eine oder mehrere Quellen vorab in den Cache laden. Das ist nuetzlich, wenn ein Agenten- oder CI-Workflow spaeter nur noch mit lokalen Pfaden arbeiten soll.

```bash
repobridge fetch react@19.0.0 vercel/next.js
repobridge fetch --quiet pypi:requests==2.32.3
```

### Pfade fuer Automatisierung ausgeben

Das Kommando `path` ist fuer nachgelagerte Tools gedacht. Es gibt pro Eingabe den absoluten Pfad zur lokalen Source-Kopie aus.

```bash
SOURCE_PATH="$(repobridge path react)"
ls "$SOURCE_PATH"
```

Mit `--verbose` zeigt `path` zusaetzliche Fetch-Informationen.

```bash
repobridge path --verbose react
```

### Cache ansehen und bereinigen

RepoBridge speichert geladene Quellen unter `REPOBRIDGE_HOME` oder, wenn die Variable nicht gesetzt ist, unter `~/.repobridge`. Der Cache enthaelt Quellcode-Snapshots und einen `sources.json` Index.

```bash
repobridge list
repobridge list --json
repobridge remove react
repobridge remove github.com/vercel/next.js
repobridge clean
```

`clean` kann gezielt auf Quelltypen oder Registries eingeschraenkt werden:

```bash
repobridge clean --packages
repobridge clean --repos
repobridge clean --npm
repobridge clean --pypi
repobridge clean --crates
repobridge clean --maven
repobridge clean --nuget
```

## Unterstuetzte Eingaben

| Typ | Beispiele | Verhalten |
| --- | --- | --- |
| npm | `react`, `react@19.0.0`, `@scope/pkg@1.2.3` | Standard-Registry ohne Prefix. Ohne Version nutzt RepoBridge eine lokal erkannte Version oder die Registry-Aufloesung. |
| PyPI | `pypi:requests`, `pypi:requests==2.32.3`, `python:requests@2.32.3` | Paket-Metadaten werden aus PyPI gelesen und auf eine Repository-Quelle normalisiert. |
| crates.io | `crates:serde`, `cargo:serde@1.0.217`, `rust:serde@1.0.217` | RepoBridge nutzt crates.io-Metadaten und passende Git-Referenzen. |
| Maven | `maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0`, `java:group:artifact@1.0.0` | RepoBridge bevorzugt `*-sources.jar`; falls nicht vorhanden, wird SCM-Metadaten-Fallback versucht. |
| NuGet | `nuget:Newtonsoft.Json`, `nuget:Newtonsoft.Json@13.0.3`, `dotnet:Serilog@3.1.1` | RepoBridge liest `.nuspec` Repository-Metadaten aus dem Paket und holt die passende Git-Quelle. |
| GitHub | `vercel/next.js`, `github:vercel/next.js`, `github.com/vercel/next.js` | Ohne Ref wird der Default-Branch ueber die Host-API ermittelt. |
| GitLab | `gitlab:group/project`, `gitlab.com/group/subgroup/project` | Projektpfade mit Subgroups werden unterstuetzt. |
| Bitbucket | `bitbucket:workspace/repo`, `bitbucket.org/workspace/repo` | Ohne Ref wird der Default-Branch ueber die Bitbucket-API ermittelt. |
| URL | `https://github.com/vercel/next.js/tree/canary` | Unterstuetzte Host-URLs werden normalisiert; Tree-Refs werden erkannt. |

## CLI-Referenz

| Kommando | Zweck | Wichtige Optionen |
| --- | --- | --- |
| `repobridge path <spec...>` | Laedt Quellen bei Bedarf und gibt absolute lokale Pfade aus. | `--cwd`, `--verbose` |
| `repobridge fetch <spec...>` | Laedt Quellen in den Cache, ohne Pfade als Hauptausgabe zu verwenden. | `--cwd`, `--quiet` |
| `repobridge list` | Listet gecachte Pakete und Repositories. | `--json` |
| `repobridge remove <spec...>` | Entfernt ausgewaehlte Cache-Eintraege. | Alias: `rm` |
| `repobridge clean` | Entfernt Cache-Eintraege nach Typ oder Registry. | `--packages`, `--repos`, `--npm`, `--pypi`, `--crates`, `--maven`, `--nuget` |

## Installation und Voraussetzungen

RepoBridge benoetigt Go 1.22 oder neuer und `git` auf dem `PATH`.

Installation aus dem Repository:

```bash
go install ./cmd/repobridge
```

Lokaler Build:

```bash
go build -o ./bin/repobridge ./cmd/repobridge
./bin/repobridge --version
```

## Konfiguration

| Variable | Bedeutung |
| --- | --- |
| `REPOBRIDGE_HOME` | Cache-Verzeichnis. Standard ist `~/.repobridge`. |
| `GITHUB_TOKEN` | Token fuer GitHub API-Aufrufe und private GitHub-Repositories. |
| `GITLAB_TOKEN` | Token fuer private GitLab-Repositories. |
| `BITBUCKET_TOKEN` | Token fuer private Bitbucket-Repositories. |

Tokens sollten nur ueber die Umgebung gesetzt werden. Sie gehoeren nicht in Quellcode, Logs oder Cache-Inhalte.

## Typische Workflows

### Quellcode einer Dependency analysieren

1. Paket angeben:

   ```bash
   repobridge path react@19.0.0
   ```

2. Den ausgegebenen Pfad in Editor, Agent oder Analyse-Tool verwenden.
3. Bei spaeteren Aufrufen wird derselbe Cache-Eintrag wiederverwendet.

### Quellen fuer einen Agentenlauf vorbereiten

1. Alle benoetigten Quellen vorab laden:

   ```bash
   repobridge fetch react@19.0.0 pypi:requests==2.32.3 vercel/next.js
   ```

2. Im Agentenlauf nur noch `repobridge path <spec>` aufrufen.
3. Der Agent erhaelt stabile lokale Pfade statt wechselnder Remote-URLs.

### Cache vor riskanten Tests isolieren

1. Temporaeres Cache-Verzeichnis setzen:

   ```bash
   export REPOBRIDGE_HOME="$(mktemp -d)"
   ```

2. Befehle ausfuehren:

   ```bash
   repobridge fetch react
   repobridge clean --npm
   ```

3. Das temporaere Verzeichnis nach dem Testlauf entfernen.

## Grenzen und aktueller Stand

RepoBridge dokumentiert und unterstuetzt aktuell die oeffentlichen Registry- und Repository-Quellen, die in der CLI genannt sind. Einige Erweiterungen sind noch nicht enthalten:

- Custom oder private Registry-URLs fuer npm, PyPI, crates.io, Maven und NuGet.
- Checksum- und Signaturpruefung fuer heruntergeladene Archive.
- Maven Snapshot-Metadaten und automatische neueste Maven-Versionen.
- Lokale Versionserkennung fuer Gradle-, Maven- und .NET-Projekte.
- JSON-Ausgabe fuer `path` und `fetch`.
- Konfigurationsdatei fuer Standard-Registries oder Cache-Policies.

## Troubleshooting

### `git` wird nicht gefunden

**Symptom:** Fetching von Repository-Quellen schlaegt fehl.

**Loesung:**

1. Pruefen, ob `git` installiert ist:

   ```bash
   git --version
   ```

2. Sicherstellen, dass `git` im `PATH` liegt.
3. Den RepoBridge-Befehl erneut ausfuehren.

### API-Limits oder private Repositories

**Symptom:** Repository-Aufloesung schlaegt bei GitHub, GitLab oder Bitbucket fehl.

**Loesung:**

1. Passenden Token als Umgebungsvariable setzen:

   ```bash
   export GITHUB_TOKEN="..."
   export GITLAB_TOKEN="..."
   export BITBUCKET_TOKEN="..."
   ```

2. Befehl erneut ausfuehren.
3. Token nicht in Shell-History, Logs oder Projektdateien ablegen.

### Falsche npm-Version wird geladen

**Symptom:** `repobridge path react` verwendet nicht die erwartete Projektversion.

**Loesung:**

1. Mit `--cwd` das Projektverzeichnis angeben:

   ```bash
   repobridge path --cwd /pfad/zum/projekt react
   ```

2. Pruefen, ob `node_modules`, `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock` oder `package.json` die erwartete Version enthalten.
3. Falls eine konkrete Version benoetigt wird, diese explizit angeben:

   ```bash
   repobridge path react@19.0.0
   ```

### Cache soll gezielt geleert werden

**Symptom:** Eine alte oder nicht mehr benoetigte Source-Kopie liegt im Cache.

**Loesung:**

1. Cache-Inhalte anzeigen:

   ```bash
   repobridge list
   ```

2. Einzelne Quelle entfernen:

   ```bash
   repobridge remove react
   ```

3. Alternativ gezielt nach Typ oder Registry bereinigen:

   ```bash
   repobridge clean --repos
   repobridge clean --nuget
   ```

## Weiterfuehrende Informationen

- [README](https://github.com/arkadiuszczarnik/repobridge/blob/main/README.md)
- [Feature-Uebersicht](https://github.com/arkadiuszczarnik/repobridge/blob/main/docs/features/00-feature-set-overview.md)
- [Maven Sources Done](https://github.com/arkadiuszczarnik/repobridge/blob/main/docs/features/maven-sources-done.md)
- [NuGet Sources Done](https://github.com/arkadiuszczarnik/repobridge/blob/main/docs/features/17-nuget-sources-done.md)
