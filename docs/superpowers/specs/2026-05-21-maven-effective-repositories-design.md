# Maven Effective Repositories Design

## Problem

RepoBridge currently resolves every Maven artifact against a single Maven Central base URL. That fails for projects that use internal Maven repositories, repository managers, or `settings.xml` mirrors. It also makes repository order impossible to honor because the resolved package carries only one source archive URL and one POM URL.

## Scope

This feature adds project-aware Maven repository discovery for `repobridge path` and `repobridge fetch` when a Maven package is resolved with a `--cwd` or current working directory that contains Maven configuration.

The first implementation supports:

- `pom.xml` repositories.
- Parent POMs reachable through local `parent.relativePath`, with the default `../pom.xml`.
- Active-by-default POM profiles.
- `settings.xml` profiles activated through `<activeProfiles>`.
- `settings.xml` mirrors with exact repository ids, `*`, `external:*`, and `!id` exclusions.
- Maven Central only as Maven's default inherited repository or when explicitly configured.

The implementation does not support remote parent POM resolution, Maven encrypted credentials, server authentication, proxy settings, repository policies, or full Maven model interpolation. Those are documented follow-up items rather than hidden behavior.

## Architecture

Add a focused Maven repository configuration unit in `internal/registry/maven`. It parses Maven XML into a stable ordered list of repositories, applies mirrors, removes duplicate repository ids while preserving first effective position, and returns Maven Central as the default if no local configuration disables or replaces it.

Extend `registry.ResolvedPackage` so Maven packages can carry an ordered list of repository-specific source/POM URL candidates. Keep the existing `SourceArchiveURL` and `SourceMetadataURL` fields populated for compatibility, but make the Maven fetcher iterate the ordered URL candidates before falling back to SCM metadata.

Thread `cwd` from `source.Acquirer.Ensure` into the default resolver. Custom test resolvers remain supported through the existing interface; only the built-in resolver needs project context.

## Data Flow

1. `source.ensurePackageCached` parses the package spec and passes `cwd` to the default resolver.
2. The Maven resolver parses coordinates, builds the effective repository list from `cwd`, and creates ordered source/POM URL candidates.
3. `GitFetcher.FetchPackage` tries each source JAR URL in order.
4. If all source JAR candidates return 404, it tries POM SCM metadata in the same repository order.
5. If a source JAR download returns a non-404 HTTP error, resolution stops because the selected earlier repository is present but failing.

## Testing

Unit tests cover Maven repository parsing, parent POM order, settings profiles, mirror application, Maven Central defaults, and source/POM fetch order. Existing archive safety tests remain unchanged.

## Documentation

Update README Maven notes and add a feature done file that records the implemented scope and limitations. The broader package-manager registry audit remains tracked through Feature 29 and future follow-ups.
