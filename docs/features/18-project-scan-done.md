# Project Scan Done

## Summary

RepoBridge now supports scanning a project for dependency source specs:

```bash
repobridge scan --cwd .
repobridge scan --cwd . --json
repobridge scan --cwd . --fetch --limit 10
repobridge scan --cwd . --no-imports
```

The implementation adds a `projectscan` package and a `scan` CLI command. It detects direct dependency candidates from common project files and optional import hints, ranks them by confidence, emits human-readable or JSON output, and can fetch detected sources into the existing RepoBridge cache.

The `repobridge-project-context` skill now uses `repobridge scan` directly instead of a Python prototype script.

## Deviations

- The first implementation keeps detection conservative. It covers the core project files used in tests and avoids guessing unresolved JVM coordinates from imports without a version.
- `--fetch` continues after individual fetch failures and reports them on stderr.

## Open Questions And Technical Debt

- Add deeper support for pnpm/yarn lockfiles, `pyproject.toml`, `Pipfile`, Gradle, and richer Python/Rust/.NET import mapping.
- Consider a future `--ecosystem` filter once more ecosystems have deep detection.
- Consider adding fetched path details to JSON output when `--fetch` is used.
