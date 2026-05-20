# AST Codegraph Search Design

## Problem

RepoBridge bringt Dependency- und Repository-Quellen bereits in stabile lokale Pfade. Danach muessen Coding Agents aber weiterhin mit Textsuche, Glob-Scans und Datei-Reads erkunden, welche Symbole, Dateien und Funktionsaufrufe relevant sind. Der lokale Cache bietet damit noch keinen semantischen Suchvorteil.

## Goal

Nach erfolgreichen `path`, `fetch` und `scan --fetch` Befehlen soll RepoBridge im Hintergrund einen lokalen AST-basierten Graphen fuer die gecachte Source erzeugen. Ein neuer `search` Befehl liest diesen Graphen und findet Symbole sowie Funktionsaufrufe. Wenn der Graph bei der Suche fehlt oder veraltet ist, baut `search` ihn synchron auf und sucht danach.

## Scope

In Scope:

- Async Auto-Indexing nach erfolgreichen Cache-Outcomes.
- ObjectBox-Datenbank pro gecachter Source unter `<source>/.repobridge-graph/`.
- Tree-sitter AST Parsing fuer Go, Java/Kotlin, C#, JavaScript/TypeScript, Python und Rust, soweit stabile Go-Grammar-Bindings verfuegbar sind.
- Graph-Entities fuer Dateien, Nodes, Edges, unresolved References und Metadaten.
- Search nach Symbolnamen, Node-Kinds, Sprache, Pfad und Aufrufen.
- JSON- und menschenlesbare CLI-Ausgabe.

Out of scope:

- CodeGraph vollstaendig nachbauen.
- Watcher, MCP-Server, Agent-Installer oder Live-Sync.
- Perfekte Cross-File- und Cross-Language-Semantik.
- Remote-Speicher oder externe Services.

## Architecture

Neue Pakete:

- `internal/codegraph`: oeffentliche Service-Schicht fuer Indexing und Search.
- `internal/codegraph/parser`: Datei-Scan, Spracherkennung, Tree-sitter Parser-Registry und sprachspezifische Extraction.
- `internal/codegraph/store`: ObjectBox Entities, generiertes Model und Query-Schicht.

Die CLI bleibt Orchestrator. Nach einem erfolgreichen `EnsureCached` ruft sie einen kleinen Scheduler auf, der einen Hintergrundjob startet. Der Scheduler blockiert die Hauptausgabe nicht und begrenzt parallele Jobs innerhalb des Prozesses, damit `fetch` mit vielen Specs nicht beliebig viele Parser startet.

`search` ist synchron: Source sicherstellen, Graphstatus pruefen, bei Bedarf indexieren, Query ausfuehren, Ergebnisse ausgeben.

## Data Model

ObjectBox Entities:

- `GraphMetadata`: Schema-Version, Source-Pfad, Status, Fehlertext, Zeiten.
- `GraphFile`: relativer Pfad, Sprache, Content-Hash, Groesse, mtime, IndexedAt, NodeCount.
- `GraphNode`: Kind, Name, QualifiedName, Datei, Sprache, Start-/Endposition, Signatur, Flags.
- `GraphEdge`: SourceNodeID, TargetNodeID, Kind, Datei, Position, Provenienz.
- `UnresolvedReference`: FromNodeID, ReferenceName, ReferenceKind, Datei, Sprache, Position.

Der Store bietet keine frei streuenden ObjectBox-Aufrufe in CLI oder Parser. Er stellt fokussierte Methoden bereit:

- `OpenGraph(sourcePath string)`
- `ReplaceGraph(result IndexResult)`
- `Status()`
- `Search(query SearchQuery)`
- `FindCalls(name string, filters SearchFilters)`

## Parsing

Der Parser arbeitet dateibasiert:

1. Source-Verzeichnis scannen.
2. Generierte und schwere Verzeichnisse ausschliessen.
3. Sprache ueber Extension erkennen.
4. Datei-Hash und mtime berechnen.
5. Tree-sitter Parser fuer die Sprache laden.
6. Nodes und lokale Relationships extrahieren.
7. Aufrufe gegen bekannte Nodes aufloesen oder als unresolved speichern.

Tree-sitter Ressourcen werden explizit geschlossen. Query-Patterns bleiben sprachspezifisch, damit neue Sprachen spaeter isoliert ergaenzt werden koennen.

## Search

Command:

```bash
repobridge search <spec> <query>
```

Basisfilter:

- `kind:<node-kind>`
- `lang:<language>`
- `path:<substring>`
- `name:<substring>`
- `calls:<name>`

Flags:

- `--cwd`
- `--json`
- `--limit`
- `--kind`
- `--lang`
- `--path`
- `--calls`
- `--no-sync-index`

Query-Parsing ist bewusst klein. Unbekannte `key:value` Tokens werden Suchtext, nicht Fehler. Dadurch bleiben normale Textsuchen robust.

## Error Handling

Async Indexing darf bestehende Befehle nicht fehlschlagen lassen. Startfehler koennen als Warnung auf `stderr` erscheinen, Laufzeitfehler werden im Graphstatus gespeichert.

Search-Fehler sind echte CLI-Fehler, wenn Source-Aufloesung, synchrones Indexing oder ObjectBox-Oeffnen fehlschlaegt. Bei korruptem Graphspeicher darf RepoBridge den Graph neu aufbauen, aber niemals die gecachte Source oder `sources.json` beschaedigen.

## Testing

Tests sollen ohne echte Netzwerkzugriffe laufen:

- Parser-Fixtures pro Sprache fuer Funktionen, Methoden, Imports und Calls.
- Store-Tests mit temporaeren Verzeichnissen.
- Search-Query-Parser Tests.
- CLI-Tests mit Fake-App und Fake-Indexer, die pruefen, dass `path`, `fetch` und `scan --fetch` Indexjobs starten, ohne Ausgabe und Exit-Code zu veraendern.
- Search-CLI-Tests fuer synchronen Index-Fallback und JSON-Ausgabe.

## External References

- Tree-sitter Go bindings: `github.com/tree-sitter/go-tree-sitter`
- ObjectBox Go: `github.com/objectbox/objectbox-go`
- CodeGraph reference concepts: `https://github.com/colbymchenry/codegraph`
