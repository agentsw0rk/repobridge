# Schema Version Rule

If a change only takes effect after the AST graph is rebuilt, bump `SchemaVersion`
in `internal/astgraph/indexer.go` — i.e. any extraction/indexing change (node/edge
kinds, walker logic, call attribution, resolution) whose output differs from a
cached graph.

**Why:** Graphs auto-rebuild only when `needsIndex` (`internal/astgraph/lifecycle.go`)
sees a stale `SchemaVersion` or changed file hashes. Code-only changes leave hashes
untouched, so old indexes serve stale results; `project:.` graphs
(`~/.repobridge/projects/<id>/.repobridge-graph`) have no `clean`/`remove` command,
making the bump their only invalidation.

**How:** Bump the constant and update the guard test `TestSchemaVersionBumped...`
in `internal/astgraph/indexer_test.go` to assert the new value, named for the change.
