#!/usr/bin/env bash

set -euo pipefail

PRODUCT_NAME="${PRODUCT_NAME:-repobridge}"
PRODUCT_DESCRIPTION="${PRODUCT_DESCRIPTION:-Fetch package and repository source code into stable local paths}"
HOMEPAGE_URL="${HOMEPAGE_URL:-https://repobridge.dev}"
LICENSE_ID="${LICENSE_ID:-Apache-2.0}"
RELEASE_BASE_URL="${RELEASE_BASE_URL:-https://github.com/repobridge/repobridge/releases/download}"

die() {
  echo "error: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

release_tag() {
  local version="${1:-${VERSION:-}}"
  [[ -n "${version}" ]] || die "version argument or VERSION env var is required"
  if [[ "${version}" == v* ]]; then
    echo "${version}"
  else
    echo "v${version}"
  fi
}

release_version() {
  local tag
  tag="$(release_tag "$@")"
  echo "${tag#v}"
}

asset_name() {
  local tag="$1"
  local platform="$2"
  local arch="$3"
  local ext="$4"
  echo "${PRODUCT_NAME}_${tag}_${platform}_${arch}.${ext}"
}

asset_url() {
  local tag="$1"
  local asset="$2"
  echo "${RELEASE_BASE_URL}/${tag}/${asset}"
}

checksums_url() {
  local tag="$1"
  echo "${RELEASE_BASE_URL}/${tag}/checksums.txt"
}

download_checksums() {
  local tag="$1"
  local output="$2"
  require_cmd curl
  curl -fsSL "$(checksums_url "${tag}")" -o "${output}"
}

checksum_for_asset() {
  local checksums_file="$1"
  local asset="$2"
  [[ -f "${checksums_file}" ]] || die "checksums file not found: ${checksums_file}"
  awk -v asset="${asset}" '
    $2 == asset || $2 == "./" asset {
      print $1
      found = 1
      exit
    }
    END {
      if (!found) {
        exit 1
      }
    }
  ' "${checksums_file}" || die "checksum not found for ${asset}"
}

prepare_output_dir() {
  local output_dir="$1"
  mkdir -p "${output_dir}"
}

