# Feature 19: AST Codegraph Search

## Problem

RepoBridge kann Quellcode fuer Dependencies und Repositories lokal cachen, aber Coding Agents muessen danach weiterhin Dateien mit Textsuche, Glob-Scans und manuellen Reads erkunden. Das kostet Zeit und Kontext, obwohl der Quellcode bereits stabil im lokalen Cache liegt.

Nach dem Fetching soll RepoBridge aus gecachten Quellen einen lokalen Codegraphen erzeugen. Der Graph soll Symbolstrukturen und Funktionsaufrufe abfragbar machen, damit Nutzer und Agents relevante Einstiegspunkte schneller finden.

## Ziel

RepoBridge indexiert erfolgreich gecachte Quellen automatisch im Hintergrund und bietet einen separaten Suchbefehl:

```bash
repobridge path react@19.0.0
repobridge fetch pypi:requests==2.32.3
repobridge scan --cwd . --fetch --limit 10

repobridge search react@19.0.0 "kind:function name:render"
repobridge search pypi:requests==2.32.3 "calls:send lang:python"
repobridge search github.com/vercel/next.js "path:packages kind:method"
```

Das Indexing laeuft nach `path`, `fetch` und `scan --fetch` asynchron, damit die bestehenden Kommandos ihre primaere Aufgabe nicht blockieren. Wenn `search` keinen fertigen oder aktuellen Index findet, baut `search` den Index fuer die angefragte Source synchron und sucht danach.

## Nicht-Ziele

- Keine vollstaendige Portierung von CodeGraph.
- Kein MCP-Server oder Agent-Installer in dieser Iteration.
- Kein Watcher fuer laufende Source-Aenderungen.
- Keine semantisch perfekte Cross-File-Aufloesung aller Sprachfeatures.
- Keine Netzwerk- oder Remote-Datenbank; alle Graphdaten bleiben lokal.
- Keine Aenderung der bisherigen `path`-, `fetch`- oder `scan`-Hauptausgabe durch den Hintergrund-Indexer.

## Architektur

### Automatisches Indexing

Nach jedem erfolgreichen `EnsureCached`-Outcome stoesst die CLI einen nicht-blockierenden Indexjob an. Betroffen sind:

- `repobridge path <spec...>`
- `repobridge fetch <spec...>`
- `repobridge scan --fetch`

Der Job bekommt Source-Pfad, kanonischen Namen, Version und Source-Label aus dem Outcome. Fehler im Hintergrundindex werden nicht als Kommando-Fehler behandelt. Bei nicht-quiet/non-json Ausgaben kann RepoBridge eine kurze Warnung auf `stderr` schreiben, wenn der Job nicht gestartet werden kann. Laufzeitfehler des Jobs werden in der Graph-Metadatenbank oder in einer kleinen Statusdatei neben dem Graphen abgelegt.

### Search-Fallback

`repobridge search <spec> <query>` stellt sicher, dass die angefragte Source im Cache liegt. Wenn kein Graph existiert, der Graph unvollstaendig ist oder die gespeicherten File-Hashes nicht zur Source passen, baut `search` den Index synchron und fuehrt danach die Suche aus.

Dadurch bleiben Fetch-Workflows schnell, aber die erste Suche liefert keine leeren Ergebnisse nur wegen eines noch laufenden Hintergrundjobs.

### Graph-Speicher

Die Graphdaten liegen neben jeder gecachten Source:

```text
<cached-source>/.repobridge-graph/
```

Die Datenbank nutzt ObjectBox Go. Die Entities leben in einem neuen Paket, zum Beispiel `internal/codegraph/store`, und werden mit `go generate ./...` generiert.

ObjectBox-Entities:

| Entity | Zweck |
|---|---|
| `GraphMetadata` | Schema-Version, Source-Pfad, Source-Hash/Indexstatus, Start-/Endzeit, Fehler |
| `GraphFile` | Relativer Pfad, Sprache, Content-Hash, Groesse, mtime, IndexedAt |
| `GraphNode` | Symbol- oder Strukturknoten mit Kind, Name, QualifiedName, Datei, Sprache, Position, Signatur |
| `GraphEdge` | Beziehung zwischen Nodes mit Kind, Source/Target IDs, Position und Provenienz |
| `UnresolvedReference` | Aufrufe/Referenzen, die nicht eindeutig auf einen Zielknoten aufgeloest werden konnten |

Wichtige Indexe:

- `GraphNode.Name`
- `GraphNode.Kind`
- `GraphNode.Language`
- `GraphNode.FilePath`
- `GraphEdge.Kind`
- `GraphEdge.SourceNodeID`
- `GraphEdge.TargetNodeID`
- `UnresolvedReference.ReferenceName`

### Parser und Extraktion

Ein neues Paket `internal/codegraph/parser` kapselt Tree-sitter:

- Spracherkennung ueber Dateiendungen.
- Parser-Registry fuer Go, Java, Kotlin, C#, JavaScript/TypeScript, Python und Rust.
- Datei-Scan mit Ausschluessen fuer `.git`, `node_modules`, `vendor`, `dist`, `build`, `target`, `bin`, `obj`, `.repobridge-graph` und weitere generierte Verzeichnisse.
- Maximalgroesse pro Datei, um sehr grosse oder minifizierte Dateien zu ueberspringen.
- Sauberes Schliessen von Tree-sitter `Parser`, `Tree`, `Query` und `QueryCursor` Objekten.

Erste Node-Kinds:

- `file`
- `module`
- `class`
- `struct`
- `interface`
- `function`
- `method`
- `import`

Erste Edge-Kinds:

- `contains`
- `calls`
- `imports`

Funktions- und Methodenaufrufe werden ueber sprachspezifische Tree-sitter-Queries extrahiert. Wenn ein Ziel im aktuellen Graph eindeutig gefunden wird, entsteht eine `calls` Edge. Sonst speichert RepoBridge einen `UnresolvedReference` mit Namen, Datei und Position.

### Search Command

Neuer Befehl:

```bash
repobridge search <spec> <query>
```

Flags:

| Flag | Zweck |
|---|---|
| `--cwd` | Arbeitsverzeichnis fuer Versions-/Spec-Aufloesung wie bei `path` |
| `--json` | Maschinenlesbare Ausgabe |
| `--limit` | Maximale Trefferzahl, Default 20 |
| `--kind` | Node-Kind Filter, wiederholbar |
| `--lang` | Sprachfilter, wiederholbar |
| `--path` | Pfad-Substring Filter, wiederholbar |
| `--calls` | Suche nach Nodes, die einen bestimmten Aufruf enthalten |
| `--no-sync-index` | Kein synchroner Index-Fallback; Fehler, wenn der Graph fehlt oder veraltet ist |

Die Query unterstuetzt zusaetzlich einfache Token:

- `kind:function`
- `lang:go`
- `path:internal/cache`
- `name:EnsureCached`
- `calls:Put`

Unbekannte `key:value` Tokens werden als normaler Suchtext behandelt, damit Suchen nach Text wie `TODO:` nicht fehlschlagen.

## Ausgabe

Menschenlesbare Ausgabe:

```text
react@19.0.0
  function render packages/react-dom/src/client/ReactDOMRoot.js:124
    calls: updateContainer
```

JSON-Ausgabe:

```json
[
  {
    "source": "react@19.0.0",
    "kind": "function",
    "name": "render",
    "qualifiedName": "render",
    "language": "javascript",
    "path": "packages/react-dom/src/client/ReactDOMRoot.js",
    "startLine": 124,
    "endLine": 148,
    "score": 12.5,
    "calls": ["updateContainer"]
  }
]
```

## Fehlerverhalten

| Fall | Verhalten |
|---|---|
| Source kann nicht aufgeloest oder gecacht werden | `search` gibt den bestehenden Resolver-/Fetch-Fehler zurueck |
| Hintergrundindex schlaegt fehl | Urspruengliches Kommando bleibt erfolgreich; Fehler wird als Indexstatus gespeichert |
| `search` findet fehlenden/veralteten Graph | Synchronous Reindex, danach Suche |
| `--no-sync-index` und Graph fehlt | Klarer Fehler mit Hinweis auf erneuten `path`/`fetch` oder Suche ohne Flag |
| Parser fuer Sprache fehlt | Datei wird uebersprungen und als Warnung im Indexstatus erfasst |
| Datei ist zu gross oder nicht lesbar | Datei wird uebersprungen und als Warnung erfasst |
| ObjectBox-Datenbank ist korrupt | Graph-Verzeichnis wird gesichert oder neu aufgebaut; Source-Cache bleibt unberuehrt |

## Betroffene Dateien

| Datei/Paket | Änderung |
|---|---|
| `go.mod`, `go.sum` | Tree-sitter-, Grammar- und ObjectBox-Abhaengigkeiten |
| `internal/codegraph` | Neuer Indexing- und Search-Service |
| `internal/codegraph/parser` | Tree-sitter Parser-Registry und sprachspezifische Queries |
| `internal/codegraph/store` | ObjectBox Entities, generiertes Model und Query-Schicht |
| `internal/cli/root.go` | `search` Command registrieren |
| `internal/cli/commands.go` | Hintergrundindexing nach erfolgreichen `path`, `fetch`, `scan --fetch`; neuer Search-Command |
| `internal/source/source.go` | Falls noetig Outcome-Metadaten fuer Indexjob ergaenzen |
| `internal/cache` | Hilfsfunktion fuer Graph-Verzeichnisse unterhalb eines Source-Pfads |
| `README.md` | Search- und Indexing-Dokumentation |
| `docs/features/00-feature-set-overview.md` | Feature-Set aktualisieren |

## Abhängigkeiten

- Bestehendes Fetching und Cache-Indexing fuer npm, PyPI, crates.io, Maven, NuGet und Repositories.
- Bestehende `scan --fetch` Orchestrierung.
- `github.com/tree-sitter/go-tree-sitter`.
- Tree-sitter Grammar-Packages fuer die Zielsprachen, soweit als Go-Bindings verfuegbar.
- `github.com/objectbox/objectbox-go`.
- ObjectBox Codegeneration ueber `go generate ./...`.

## Akzeptanzkriterien

- `repobridge path <spec>` gibt weiterhin nur den Source-Pfad aus und startet danach einen Hintergrund-Indexjob.
- `repobridge fetch <spec>` und `repobridge scan --fetch` starten ebenfalls Hintergrund-Indexjobs fuer erfolgreich gecachte Quellen.
- Hintergrundindex-Fehler machen das urspruengliche Kommando nicht fehlerhaft.
- Pro gecachter Source entsteht ein Graph-Verzeichnis neben dem Source-Verzeichnis.
- `repobridge search <spec> <query>` baut einen fehlenden oder veralteten Graph synchron auf.
- `repobridge search <spec> "kind:function name:<x>"` findet passende Funktionen/Methoden nach Name.
- `repobridge search <spec> "calls:<x>"` findet Funktionen/Methoden, die einen Aufruf mit diesem Namen enthalten.
- `--json` liefert deterministische, maschinenlesbare Treffer.
- Nicht unterstuetzte oder fehlerhafte Dateien werden uebersprungen und im Indexstatus dokumentiert.
- Tests decken Parser-Extraktion, ObjectBox Store, Search-Query-Parsing, CLI-Ausgabe und asynchrones Indexing ohne echte Netzwerkabhaengigkeit ab.
- `go generate ./...`, `gofmt -w ./cmd ./internal`, `go test ./...` und `go vet ./...` laufen erfolgreich.
