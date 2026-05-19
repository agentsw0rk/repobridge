# NuGet Sources Done

## Summary

RepoBridge now supports NuGet package inputs such as:

```bash
repobridge path nuget:Newtonsoft.Json@13.0.3
repobridge path dotnet:Serilog@3.1.1
```

The implementation adds NuGet registry detection, NuGet v3 service-index resolution, latest-stable version selection, temporary `.nupkg` download for `.nuspec` repository metadata, Git commit-first source fetching, strict version-tag fallback without default-branch cloning, NuGet cache indexing, `clean --nuget`, and README documentation.

## Deviations

- NuGet package binaries are not cached as source. The `.nupkg` is used only to read `.nuspec` metadata.
- Prerelease versions are selected only when explicitly requested.
- Default-branch fallback is disabled for NuGet to keep package source resolution tied to the selected package version.

## Open Questions And Technical Debt

- Custom/private NuGet feeds are not supported yet.
- Local .NET project and lockfile version detection is not implemented yet.
- `.snupkg` symbol package inspection is not implemented yet.
- NuGet package signatures and checksums are not verified yet.
