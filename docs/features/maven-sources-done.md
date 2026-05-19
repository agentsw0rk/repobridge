# Maven Sources Done

## Summary

RepoBridge now supports Maven Central package inputs such as:

```bash
repobridge path maven:org.jetbrains.kotlin:kotlin-stdlib@2.1.0
```

The implementation adds Maven registry detection, coordinate parsing, Maven Central source/POM URL construction, source JAR download and extraction, Git fallback from POM SCM metadata after source JAR 404, Maven cache indexing, `clean --maven`, and README documentation.

## Deviations

- POM SCM lookup was deferred until after a source archive 404. The original plan described resolving POM SCM metadata during Maven resolution, but final review showed that would violate the source-JAR-first behavior by letting POM failures block valid source JAR downloads.
- Maven coordinate validation was tightened beyond the original plan. `groupId`, `artifactId`, and `version` reject path separators and `..` to keep cache paths stable.
- Source archive extraction gained additional hardening beyond the original plan: compressed-size, uncompressed-size, and entry-count limits.
- Git fallback normalizes a leading `v` from Maven `GitTag` before calling the existing `CloneAtTag` helper, because that helper already probes both `v<version>` and `<version>`.

## Open Questions And Technical Debt

- Custom/private Maven repositories are not supported yet.
- Snapshot metadata resolution is not supported yet.
- Automatic latest-version resolution is not supported yet.
- Checksums and signatures are not verified yet.
- Gradle/Maven lockfile detection is not implemented yet.
