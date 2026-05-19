# repobridge deployment scripts

This directory contains release packaging helpers for package-manager targets.
Generated package metadata uses the product name `repobridge` and neutral
product metadata. Personal account names are intentionally not embedded in the
generated Homebrew formula, Scoop manifest, or Debian package metadata.

## Configuration

All scripts accept a release version as their first argument. `0.1.1` and
`v0.1.1` are both accepted.

Common environment variables:

| Variable | Default                                                      | Description |
| --- |--------------------------------------------------------------| --- |
| `RELEASE_BASE_URL` | `https://github.com/repobridge/repobridge/releases/download` | Base URL that contains release assets. Override this for the actual release host. |
| `HOMEPAGE_URL` | `https://repobridge.agentswork.tech`                            | Public product homepage written into package metadata. |
| `CHECKSUMS_FILE` | downloads `checksums.txt`                                    | Optional local checksum file. |
| `PRODUCT_NAME` | `repobridge`                                                 | Package and binary name. |

## Homebrew

Generate a formula:

```bash
RELEASE_BASE_URL="https://example.com/repobridge/releases/download" \
  deploy/brew/generate-formula.sh v0.1.1
```

Default output:

```text
deploy/brew/dist/repobridge.rb
```

Platform shortcut:

```bash
deploy/platform/macos-release.sh v0.1.1
```

## Scoop

Generate a Scoop manifest:

```bash
RELEASE_BASE_URL="https://example.com/repobridge/releases/download" \
  deploy/scoop/generate-manifest.sh v0.1.1
```

Default output:

```text
deploy/scoop/dist/repobridge.json
```

Platform shortcut:

```bash
deploy/platform/windows-release.sh v0.1.1
```

## Apt / Debian

Build a Debian package:

```bash
RELEASE_BASE_URL="https://example.com/repobridge/releases/download" \
  deploy/apt/build-deb.sh v0.1.1 amd64
```

Supported architectures:

```text
amd64
arm64
```

Default output:

```text
deploy/apt/dist/repobridge_0.1.1_amd64.deb
```

Platform shortcut:

```bash
deploy/platform/linux-release.sh v0.1.1 amd64
```

