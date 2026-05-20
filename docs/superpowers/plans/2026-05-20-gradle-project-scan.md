# Gradle Project Scan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `repobridge scan` detect Maven dependencies declared directly in `build.gradle` and `build.gradle.kts`.

**Architecture:** Extend the existing `internal/projectscan` manifest scanners with a static Gradle scanner. The scanner reads build files as text, extracts only clear static dependency declarations, and emits the same Maven specs that `scanPom` already emits.

**Tech Stack:** Go 1.22+, standard library regex/string parsing, Go `testing`.

---

### Task 1: Gradle Scanner Tests

**Files:**
- Modify: `internal/projectscan/scan_test.go`

- [ ] Add tests for `build.gradle.kts` and `build.gradle` direct dependencies.
- [ ] Include negative assertions for dynamic versions and local/project dependencies.
- [ ] Run `go test ./internal/projectscan -run TestScanProjectDetectsGradleDependencies -count=1` and verify the tests fail before production code is changed.

### Task 2: Gradle Scanner Implementation

**Files:**
- Modify: `internal/projectscan/scan.go`

- [ ] Add `state.scanGradle()` to `Scan`.
- [ ] Implement static parsing for supported Gradle configurations.
- [ ] Convert valid `group:artifact:version` coordinates and map notation to `maven:<group>:<artifact>@<version>`.
- [ ] Run `go test ./internal/projectscan -count=1` and verify it passes.

### Task 3: Verification and Docs

**Files:**
- Create: `docs/features/04-gradle-project-scan-done.md`

- [ ] Run `gofmt -w ./internal/projectscan`.
- [ ] Run `go test ./... -count=1`.
- [ ] Run `go vet ./...`.
- [ ] Write the feature done note.
- [ ] Commit the implementation.

