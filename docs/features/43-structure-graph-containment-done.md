# Feature 43 Done: Structure Graph Containment

## Zusammenfassung

RepoBridge erzeugt beim AST-Indexing jetzt einen persistierten Containment-Graphen:

- Pro indexierter Source-Datei wird ein stabiler `file`-Node erzeugt.
- Statisch erkennbare Packages bzw. Namespaces werden als `module`-Nodes erzeugt.
- `contains`-Kanten verbinden Datei, Modul, Typen, Methoden, Felder, Properties, Imports und Nested Types.
- Receiver-basierte Member wie Methoden, Felder, Properties und Enum-Member werden ihrem lokalen Typ zugeordnet.
- Nested Types werden ueber Tree-sitter-Ranges ihrem einschliessenden Typ zugeordnet.
- `external`-Nodes bekommen keine `contains`-Kanten.
- `SchemaVersion` wurde auf `27` gebumpt, damit bestehende Graph-Stores neu indexiert werden.

## Abweichungen vom Feature-Dokument

- Die im Feature-Dokument skizzierte `ContainmentBuilder`-API wurde nicht als Scope-Stack in jeden Parser eingebaut. Stattdessen gibt es eine zentrale Parser-Nachverarbeitung in `internal/astgraph/parser/containment.go`. Das passt besser zur bestehenden Architektur, weil viele Nodes bereits aus einer Mischung aus Tree-sitter-Walkern und Expanded-Kind-Passes entstehen.
- ObjectBox-Entitaeten wurden nicht geaendert und daher nicht neu generiert. `contains` existierte bereits als `EdgeKind` und wird ueber `EdgeEntity.Kind` stabil als String gespeichert.
- Zusatz-CLI-Flags wie `node --children`, `node --parents` oder `files --tree` bleiben Folgefeatures. Bestehende Edge-Filter koennen `contains` bereits als normalen Edge-Kind lesen.

## Offene Fragen und technische Schulden

- Rust enum variants bleiben aktuell als `class`-Nodes modelliert, wie im bestehenden Resolver erwartet. Eine spaetere Praezisierung auf `enum_member` waere ein separates Kompatibilitaetsthema.
- Einige Expanded-Kind-Heuristiken sind weiterhin regex-basiert. Die Containment-Nachverarbeitung gleicht das aus, aber langfristig waere eine vollstaendig AST-basierte Member-Erkennung robuster.
- Module werden fuer Go, Java, Kotlin, C# und Python erzeugt. JavaScript/TypeScript bleiben file-rooted, solange kein statisches Modulkonzept eindeutig aus dem Source ableitbar ist.

## Verifikation

- `go test ./internal/astgraph/parser -run 'TestExtractFromSource.*Contain'`
- `go test ./internal/astgraph -run 'TestIndexerBuildsContainmentGraphAndKeepsCallEdges|TestSchemaVersionBumpedForContainmentGraph'`
- `go test ./internal/astgraph/...`
- `go test ./...`

