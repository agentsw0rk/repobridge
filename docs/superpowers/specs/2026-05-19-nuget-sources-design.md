# NuGet Sources Design

## Context

RepoBridge is a Go CLI that resolves package and repository specs into stable local source paths. It currently supports npm, PyPI, crates.io, Maven, and direct Git repository inputs. NuGet packages are missing, which limits RepoBridge for .NET projects.

NuGet differs from Maven for RepoBridge's purpose. Maven has a common `*-sources.jar` convention. NuGet `.nupkg` files commonly contain binaries and metadata, and `.snupkg` files are symbol packages rather than reliable source packages. Therefore NuGet support should use package metadata to find the real Git repository instead of caching extracted package binaries as source.

## Decisions

- Add NuGet as a package registry with `nuget:` and `dotnet:` prefixes.
- Support `name@version` syntax, for example `nuget:Newtonsoft.Json@13.0.3`.
- If no version is provided, select the latest stable NuGet version.
- Allow prerelease versions only when explicitly requested.
- Download `.nupkg` only temporarily to read the `.nuspec`.
- Use `.nuspec` `<repository type="git" url="..." commit="..." branch="..." />` metadata as the source of truth.
- Clone a commit when present.
- Without a commit, try version tags via the existing `v<version>` then `<version>` Git behavior.
- Do not fall back to the repository default branch.
- Do not cache extracted `.nupkg` contents as source.

## NuGet API Notes

Context7 research against official NuGet docs found these relevant API patterns:

- NuGet v3 starts at the service index, normally `https://api.nuget.org/v3/index.json`.
- Clients discover resources from the service index instead of hardcoding every endpoint.
- `PackageBaseAddress/3.0.0` provides flat-container package content such as `.nupkg` and `.nuspec`.
- `RegistrationsBaseUrl` provides package metadata and version information.
- Flat-container package URLs use lowercased package IDs and versions.
- `.nuspec` metadata can include a `repository` element with source control `type`, `url`, `branch`, and `commit`.

## User-Facing Behavior

Examples:

```bash
repobridge path nuget:Newtonsoft.Json@13.0.3
repobridge path dotnet:Newtonsoft.Json@13.0.3
repobridge path nuget:Newtonsoft.Json
repobridge fetch nuget:Serilog@3.1.1
repobridge clean --nuget
```

Expected behavior:

- `path` prints the cached source path after resolving and fetching the Git repository.
- `fetch` reports `Fetched <id>@<version> from NuGet` or `Cached ... from NuGet`.
- `list` displays NuGet package entries with the `NuGet` label.
- `remove nuget:<id>@<version>` removes the matching NuGet cache entry.
- `clean --nuget` removes only NuGet package entries.

## Architecture

### Registry Layer

Extend `internal/registry`:

- Add `NuGet Registry = "nuget"`.
- Add `NuGet` to `Registry.Label()`.
- Add prefixes `nuget:` and `dotnet:`.
- Add support in `SupportedRegistry`.
- Reuse the generic `name@version` parser shape for NuGet package specs.

### NuGet Resolver

Create `internal/registry/nuget`.

Primary responsibilities:

1. Load the service index.
2. Discover `PackageBaseAddress/3.0.0` and a SemVer2-compatible `RegistrationsBaseUrl`.
3. Resolve the version:
   - Explicit version: verify it exists.
   - Missing version: choose the newest stable version.
4. Download the `.nupkg` to a temporary file.
5. Open the ZIP safely enough to locate exactly the `.nuspec`.
6. Parse `.nuspec` XML metadata.
7. Normalize the repository URL with `registry.NormalizeRepoURL`.
8. Return `registry.ResolvedPackage`.

The resolver should preserve the canonical package ID from NuGet metadata when available. Package ID matching should be case-insensitive for API paths and cache lookups should store the canonical ID returned by NuGet.

### Source Layer

Extend `internal/source.defaultResolvePackage` to call `nuget.Resolve`.

NuGet packages should flow through `FetchPackageWithGit`, not `FetchPackageArchive`. This keeps NuGet source behavior consistent with npm, PyPI, and crates.io once the repository is known.

### Ref Handling

`registry.ResolvedPackage` already has `RepoURL`, `GitTag`, and other fields. To represent commit-first behavior without abusing `GitTag`, implementation can either:

- Add a `GitRef` field to `ResolvedPackage`, then teach `FetchPackageWithGit` to prefer it over `GitTag`.
- Or use `GitTag` for the commit and accept the existing tag helper behavior.

The cleaner design is adding `GitRef` because a commit is not a tag. `FetchPackageWithGit` should use:

1. `GitRef` when non-empty.
2. Version tag behavior when `GitTag` is set.
3. Current version fallback only for registries that already rely on it.

For NuGet, missing commit means `GitTag` should be `v<version>` so existing tag probing tries `v<version>` and `<version>`.

### Archive Handling

NuGet `.nupkg` is a ZIP archive. It should be downloaded with bounded size like Maven source archives. The resolver only needs to read the `.nuspec`, so it should not extract the full archive into the cache.

Security rules:

- Reject path traversal.
- Reject symlinks.
- Enforce compressed size and entry count limits.
- Enforce a small read limit for the `.nuspec` content.
- Clean up temporary files.

## Error Handling

| Case | Error |
|------|-------|
| Package ID not found | `repobridge.PackageNotFoundError{Name: id, Registry: "NuGet"}` |
| Explicit version not found | `repobridge.VersionNotFoundError` |
| No stable version for implicit version | `repobridge.VersionNotFoundError` |
| Service index lacks required resources | Resolver error naming the missing NuGet resource |
| `.nupkg` download fails | HTTP/download error |
| `.nuspec` missing | Resolver error naming the package/version |
| Repository metadata missing or not `git` | `repobridge.NoRepoURLError` |
| Repository URL unsupported by RepoBridge | `repobridge.NoRepoURLError` |
| Commit/tag clone fails | Existing Git fetch error |

## Tests

Add table-driven and `httptest` coverage:

- `DetectRegistry` recognizes `nuget:` and `dotnet:`.
- `ParsePackageSpec` parses NuGet package IDs and versions.
- `SupportedRegistry` accepts NuGet.
- NuGet service index resource discovery handles required resources.
- Latest stable version ignores prereleases.
- Explicit prerelease version resolves.
- Missing package and missing version return typed errors.
- `.nupkg` with `.nuspec` repository commit resolves to `GitRef`.
- `.nupkg` with repository URL but no commit resolves to version tag behavior.
- Missing `.nuspec` returns a clear resolver error.
- Unsupported repository host returns `NoRepoURLError`.
- `EnsureCached` returns existing NuGet cache entries without fetching.
- `clean --nuget` filters NuGet entries only.
- Fetch output displays `NuGet`.

## Documentation Updates

- Update `README.md` supported registries, examples, and command flag notes.
- Update `docs/features/00-feature-set-overview.md`.
- Add a completion note after implementation at `docs/features/17-nuget-sources-done.md`.

## Open Technical Debt

- Private/custom NuGet feeds are out of scope for the first iteration.
- Local .NET lockfile/project version detection is out of scope.
- `.snupkg` symbol package inspection is out of scope.
- NuGet package signatures/checksums are out of scope.
