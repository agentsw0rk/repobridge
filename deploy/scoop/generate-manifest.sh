#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd -P)"
# shellcheck source=../common/release-utils.sh
source "${REPO_ROOT}/deploy/common/release-utils.sh"

usage() {
  cat <<'USAGE'
Usage: deploy/scoop/generate-manifest.sh <version>

Generates a Scoop manifest for repobridge.

Environment:
  RELEASE_BASE_URL  Base URL for release downloads.
                    Default: https://github.com/repobridge/repobridge/releases/download
  HOMEPAGE_URL      Product homepage shown in the manifest.
                    Default: https://repobridge.dev
  CHECKSUMS_FILE    Optional local checksums.txt.
  OUT               Output file.
                    Default: deploy/scoop/dist/repobridge.json
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

tag="$(release_tag "${1:-}")"
version="$(release_version "${tag}")"
output="${OUT:-${SCRIPT_DIR}/dist/${PRODUCT_NAME}.json}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

checksums_file="${CHECKSUMS_FILE:-${tmp_dir}/checksums.txt}"
if [[ -z "${CHECKSUMS_FILE:-}" ]]; then
  download_checksums "${tag}" "${checksums_file}"
fi

windows_amd64="$(asset_name "${tag}" windows amd64 zip)"

prepare_output_dir "$(dirname "${output}")"

cat >"${output}" <<MANIFEST
{
  "version": "${version}",
  "description": "${PRODUCT_DESCRIPTION}",
  "homepage": "${HOMEPAGE_URL}",
  "license": "${LICENSE_ID}",
  "architecture": {
    "64bit": {
      "url": "$(asset_url "${tag}" "${windows_amd64}")",
      "hash": "$(checksum_for_asset "${checksums_file}" "${windows_amd64}")"
    }
  },
  "bin": "${PRODUCT_NAME}.exe",
  "checkver": {
    "url": "${HOMEPAGE_URL}",
    "regex": "v([0-9.]+)"
  },
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "${RELEASE_BASE_URL}/v\$version/${PRODUCT_NAME}_v\$version_windows_amd64.zip"
      }
    }
  }
}
MANIFEST

echo "wrote ${output}"

