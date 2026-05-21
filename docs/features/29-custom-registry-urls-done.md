# Feature 29 Maven Repository Resolution Done

## Summary

RepoBridge now resolves Maven artifacts against the effective local Maven repository configuration instead of assuming a single Maven Central URL.

Implemented behavior:

- Repositories are read from `pom.xml`.
- Local parent POMs are followed through `parent.relativePath`, including Maven's default `../pom.xml`.
- Active-by-default POM profiles contribute repositories.
- `settings.xml` active profiles contribute repositories.
- `settings.xml` mirrors are applied for exact repository ids, `*`, `external:*`, and `!id` exclusions.
- Maven source JARs are tried in repository order.
- POM SCM fallback is tried in the same repository order after source JAR lookup misses.
- Maven Central is included through Maven's default repository behavior only when no effective repository with id `central` is present.

## Deviations

Feature 29 originally described configurable registry bases for npm, PyPI, crates.io, Maven, and NuGet. This implementation covers the Maven-specific project-configuration problem from the issue. It does not add CLI flags or config-file registry overrides for the other package managers.

## Open Questions And Technical Debt

- Remote parent POMs are not resolved.
- Maven credentials, encrypted passwords, servers, proxies, and repository policies are not interpreted.
- Property interpolation is intentionally limited; this is not a full Maven model builder.
- Snapshot metadata resolution remains tracked separately in `docs/features/31-maven-snapshot-metadata.md`.
- Custom registry assumptions for npm, PyPI, crates.io, NuGet, Cargo, and Gradle should be tracked as follow-up work instead of being folded into this Maven-specific change.
