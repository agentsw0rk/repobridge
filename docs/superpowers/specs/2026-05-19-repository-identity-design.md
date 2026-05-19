# Repository Identity Design

## Kontext

`repobridge` rekonstruiert Repository-Identitaet aktuell an mehreren Stellen:

- `internal/registry/registry.go` entscheidet, ob ein Input als Package oder Repository gilt.
- `internal/registry/repo/repo.go` parsed Repository-Shorthands, URLs, Hosts und Refs.
- `internal/registry/repo_url.go` normalisiert Repository-URLs fuer Package-Metadaten.
- `internal/source/source.go` baut Display-Namen, Clone-URLs und Cache-Keys fuer Repo-Fetches.
- `internal/source/git_fetcher.go` leitet Repository-Namen erneut aus Package-Repo-URLs ab.
- `internal/cache/cache.go` und `internal/cache/remove.go` kodieren Cache-Pfade und Remove-Regeln.
- `internal/cli/commands.go` parsed beim Entfernen erneut, um einen Cache-Selector zu bauen.

Diese Verteilung macht den Code schwerer zu testen und birgt Integrationsrisiko: `fetch`, `path` und `remove` koennen fuer denselben Input unterschiedliche Display-Namen, Clone-URLs, Cache-Keys oder Remove-Selectoren ableiten. Besonders riskant sind GitLab-Projekte mit verschachtelten Pfaden wie `gitlab.com/group/subgroup/project`.

## Ziel

Repository-Identitaet wird ein tiefes Modul mit kleiner, stabiler Grenze. Das Modul soll:

- Repo-Inputs offline parsen und normalisieren.
- GitHub, GitLab und Bitbucket einheitlich abbilden.
- GitLab-Nested-Paths als normales Modell behandeln.
- explizite Refs aus `@ref`, `#ref`, `tree` und GitLab `/-/tree` erkennen.
- Display-Namen, Clone-URLs, Cache-Keys und relative Cache-Pfade aus derselben Identitaet ableiten.
- Default-Branch-Aufloesung nur ueber einen externen Port ausfuehren.
- `remove` offline halten, damit ein unversionierter Remove weiterhin alle gecachten Refs eines Repos meint.

Nicht Ziel:

- allgemeines Provider-Plugin-System.
- SSH-Clone-URL-Unterstuetzung.
- Cache-Migration fuer bereits existierende Pfade.
- Aenderung der Package-Registry-Aufloesung ausser der Nutzung der gemeinsamen Repo-URL-Normalisierung.

## Vorgeschlagene Architektur

Neues Modul: `internal/repoidentity`.

Das Modul trennt reine Identitaetslogik von externer Provider-Metadatenauflösung.

```go
package repoidentity

import "context"

type Host string

const (
	GitHub    Host = "github.com"
	GitLab    Host = "gitlab.com"
	Bitbucket Host = "bitbucket.org"
)

type Repository struct {
	Host        Host
	ProjectPath string
}

type ParsedRepository struct {
	Repository
	ExplicitRef string
}

type ResolvedRepository struct {
	Repository
	GitRef string
	RepoURL string
}

type ProviderMetadata struct {
	DefaultBranch        string
	CanonicalProjectPath string
	CloneURL             string
}

type MetadataProvider interface {
	Host() Host
	LookupRepository(ctx context.Context, repo Repository) (ProviderMetadata, error)
}

type Parser struct {
	DefaultHost Host
	Hosts       map[Host]struct{}
}

func DefaultParser() Parser
func (p Parser) Parse(input string) (ParsedRepository, bool)
func (p Parser) IsRepoLike(input string) bool
func (p Parser) NormalizeURL(input string) (string, bool)
func (p Parser) ForRemove(input string) (ParsedRepository, error)

type Resolver struct {
	Parser      Parser
	Providers   map[Host]MetadataProvider
	FallbackRef string
}

func NewResolver(parser Parser, providers ...MetadataProvider) Resolver
func (r Resolver) Resolve(ctx context.Context, input string) (ResolvedRepository, error)
func (r Resolver) ResolveParsed(ctx context.Context, parsed ParsedRepository) (ResolvedRepository, error)
func (r Resolver) ForCache(ctx context.Context, input string) (ResolvedRepository, error)
```

Value-Methoden leiten die Formen ab, die heute manuell rekonstruiert werden:

```go
func (r Repository) DisplayName() string
func (r Repository) RepoURL() string
func (r Repository) CacheRootRelativePath() string
func (r ResolvedRepository) CacheKey() cache.RepoKey
func (r ResolvedRepository) CacheRelativePath() string
```

`Repository.ProjectPath` ersetzt das bisherige `Owner + Repo`-Denken. Fuer GitHub und Bitbucket ist das typischerweise `owner/repo`; fuer GitLab darf es `group/subgroup/project` sein.

## Datenfluss

### Fetch und Path

`source.ensureRepoCached` nutzt `Resolver.ForCache`.

1. Input wird offline geparsed.
2. Falls ein expliziter Ref vorhanden ist, entsteht daraus direkt ein `ResolvedRepository`.
3. Falls kein Ref vorhanden ist, fragt `Resolver` den `MetadataProvider` des Hosts nach dem Default Branch.
4. `source` nutzt `ResolvedRepository.CacheKey()` fuer Lookup/Record.
5. `source` nutzt `ResolvedRepository.DisplayName()`, `RepoURL` und `GitRef` fuer `FetchRepo`.

### Remove

`cli.removeSource` nutzt `Parser.ForRemove`.

1. Input wird offline geparsed.
2. `DisplayName()` wird als Cache-Name verwendet.
3. `ExplicitRef` wird als Version gesetzt.
4. Leerer Ref bleibt leer und bedeutet weiterhin: alle gecachten Refs fuer dieses Repository entfernen.

### Registry-URL-Normalisierung

`registry.NormalizeRepoURL` kann intern auf `repoidentity.DefaultParser().NormalizeURL` delegieren. Package-Registry-Resolver muessen dann keine eigenen GitHub/GitLab/Bitbucket-Pfadregeln pflegen.

## Dependency Strategy

**In-process**

Parsing, Repo-like-Erkennung, URL-Normalisierung, Display-Namen, Clone-URLs, Cache-Keys und relative Cache-Pfade sind reine, deterministische Logik. Diese Logik wird ueber Boundary-Tests am Parser und Resolver getestet.

**True external**

GitHub, GitLab und Bitbucket sind Drittanbieter. Default-Branch-Aufloesung laeuft nur ueber `MetadataProvider`. Produktion nutzt HTTP-Adapter; Tests nutzen `MemoryProvider` oder `httptest` fuer adapter-spezifische Request-/Statuscode-Tests.

## Fehlerbehandlung

- ungueltige Repo-Inputs geben einen Invalid-Repo-Fehler zurueck.
- Package-Inputs wie `@scope/pkg` duerfen nicht als Repo akzeptiert werden.
- URLs mit unsupported Hosts werden abgelehnt.
- URLs mit encoded slashes im Pfad werden abgelehnt.
- Provider-Fehler bleiben auf Provider-Ebene typisiert, z. B. Repo-not-found, Auth-Hinweis oder Rate-Limit.
- Wenn Provider-Metadaten keinen Default Branch liefern, verwendet der Resolver den bestehenden Fallback `main`.

## Teststrategie

Neue Boundary-Tests in `internal/repoidentity`:

- `owner/repo@main` wird zu `github.com/owner/repo` mit Ref `main`.
- `github:owner/repo#main` wird zu kanonischer GitHub-Identitaet.
- `https://github.com/owner/repo/tree/main` extrahiert Ref `main`.
- `gitlab.com/group/subgroup/project@v1` behaelt `group/subgroup/project` als ProjectPath.
- `https://gitlab.com/group/subgroup/project/-/tree/main` extrahiert Ref `main`.
- `@scope/pkg` wird nicht als Repository akzeptiert.
- `ForCache` loest fehlenden Ref ueber `MemoryProvider`.
- `ForRemove` loest keinen Provider-Call aus und behaelt leeren Ref als "alle Refs".
- `NormalizeURL` normalisiert GitHub/GitLab/Bitbucket-URLs und lehnt Non-Repo-Pfade ab.

Bestehende Tests koennen danach reduziert oder auf die neue Boundary umgestellt werden:

- `registry.DetectInputType`
- `repo.ParseSpec`
- `registry.NormalizeRepoURL`
- `source.repoDisplayName`
- manuelle `cache.RepoKey`- und `cache.RemoveSelector`-Konstruktion in CLI-/Source-Tests

Adapter-Tests bleiben separat fuer:

- GitHub/GitLab/Bitbucket HTTP-Pfade.
- Token-Header.
- 404/Auth/Rate-Limit-Mapping.
- JSON-Decoding.

## Migrationsplan

1. `internal/repoidentity` mit Parser, Resolver und Tests einfuehren.
2. `source.ensureRepoCached` auf `Resolver.ForCache` umstellen.
3. `cli.removeSource` auf `Parser.ForRemove` umstellen.
4. `registry.NormalizeRepoURL` auf `Parser.NormalizeURL` delegieren.
5. `source.repoDisplayName` entfernen oder auf `repoidentity` reduzieren.
6. Tests umstellen und ueberfluessige Helper-Tests entfernen, sobald Boundary-Tests dieselben Verhalten abdecken.

## Offene Entscheidungen

- Die erste Version speichert Refs weiterhin in der bisherigen Form als Cache-Version. Path-safe Ref-Key-Encoding fuer Refs mit Slash wird nicht in diesen Refactor aufgenommen, weil es Cache-Kompatibilitaet und Migration beruehrt.
- `MetadataProvider` soll vorerst nur Default-Branch-Metadaten liefern. Erweiterungen fuer SSH oder Provider-spezifische Clone-URLs bleiben ausserhalb des aktuellen Scopes.
