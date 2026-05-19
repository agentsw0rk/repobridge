#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd -P)"
# shellcheck source=../common/release-utils.sh
source "${REPO_ROOT}/deploy/common/release-utils.sh"

usage() {
  cat <<'USAGE'
Usage: deploy/brew/generate-formula.sh <version>

Generates a Homebrew formula for repobridge.

Environment:
  RELEASE_BASE_URL  Base URL for release downloads.
                    Default: https://github.com/repobridge/repobridge/releases/download
  HOMEPAGE_URL      Product homepage shown in the formula.
                    Default: https://repobridge.dev
  CHECKSUMS_FILE    Optional local checksums.txt.
  OUT               Output file.
                    Default: deploy/brew/dist/repobridge.rb
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

tag="$(release_tag "${1:-}")"
version="$(release_version "${tag}")"
output="${OUT:-${SCRIPT_DIR}/dist/${PRODUCT_NAME}.rb}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

checksums_file="${CHECKSUMS_FILE:-${tmp_dir}/checksums.txt}"
if [[ -z "${CHECKSUMS_FILE:-}" ]]; then
  download_checksums "${tag}" "${checksums_file}"
fi

darwin_amd64="$(asset_name "${tag}" darwin amd64 tar.gz)"
darwin_arm64="$(asset_name "${tag}" darwin arm64 tar.gz)"
linux_amd64="$(asset_name "${tag}" linux amd64 tar.gz)"
linux_arm64="$(asset_name "${tag}" linux arm64 tar.gz)"

prepare_output_dir "$(dirname "${output}")"

cat >"${output}" <<FORMULA
class Repobridge < Formula
  desc "${PRODUCT_DESCRIPTION}"
  homepage "${HOMEPAGE_URL}"
  version "${version}"
  license "${LICENSE_ID}"

  on_macos do
    if Hardware::CPU.arm?
      url "$(asset_url "${tag}" "${darwin_arm64}")"
      sha256 "$(checksum_for_asset "${checksums_file}" "${darwin_arm64}")"
    else
      url "$(asset_url "${tag}" "${darwin_amd64}")"
      sha256 "$(checksum_for_asset "${checksums_file}" "${darwin_amd64}")"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "$(asset_url "${tag}" "${linux_arm64}")"
      sha256 "$(checksum_for_asset "${checksums_file}" "${linux_arm64}")"
    else
      url "$(asset_url "${tag}" "${linux_amd64}")"
      sha256 "$(checksum_for_asset "${checksums_file}" "${linux_amd64}")"
    end
  end

  def install
    bin.install "${PRODUCT_NAME}"
  end

  test do
    system "#{bin}/${PRODUCT_NAME}", "--version"
  end
end
FORMULA

echo "wrote ${output}"

