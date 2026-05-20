# Search E2E

This opt-in test runs the real `repobridge search` CLI against a pinned public
Kotlin repository. It verifies repository fetching, cache wiring, ObjectBox
graph storage, Tree-sitter Kotlin parsing, and JSON search output together.

Run it explicitly:

```bash
go test -tags=e2e ./test/e2e-search -count=1 -v
```

Useful environment variables:

- `REPOBRIDGE_E2E_SEARCH_DIR`: override the workspace directory, defaults to `./e2e/search`.
- `REPOBRIDGE_E2E_REFRESH=1`: delete and re-fetch the local E2E cache.

The test intentionally is not part of `go test ./...` because it requires
network access and GitHub availability.
