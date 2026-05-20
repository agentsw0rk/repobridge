# Project Scan E2E

This opt-in test clones pinned public GitHub projects into the ignored `e2e/`
directory, runs the local `repobridge scan` CLI, and verifies minimum expected
dependency specs for each ecosystem.

Run it explicitly:

```bash
go test -tags=e2e ./test/e2e-scan -count=1
```

Useful environment variables:

- `REPOBRIDGE_E2E_DIR`: override the workspace directory, defaults to `./e2e`.
- `REPOBRIDGE_E2E_REFRESH=1`: delete and re-fetch project checkouts.

The test intentionally is not part of `go test ./...` because it requires
network access and GitHub availability.

