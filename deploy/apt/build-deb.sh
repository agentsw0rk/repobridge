#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd -P)"
# shellcheck source=../common/release-utils.sh
source "${REPO_ROOT}/deploy/common/release-utils.sh"

usage() {
  cat <<'USAGE'
Usage: deploy/apt/build-deb.sh <version> [amd64|arm64]

Builds a Debian package for repobridge from a Linux release tarball.

Environment:
  RELEASE_BASE_URL  Base URL for release downloads.
                    Default: https://github.com/repobridge/repobridge/releases/download
  HOMEPAGE_URL      Product homepage written into package metadata.
                    Default: https://repobridge.dev
  MAINTAINER        Debian maintainer field.
                    Default: repobridge maintainers <maintainers@repobridge.dev>
  CHECKSUMS_FILE    Optional local checksums.txt.
  SOURCE_TARBALL    Optional local Linux tarball to package.
  OUT_DIR           Output directory.
                    Default: deploy/apt/dist
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

require_cmd dpkg-deb
require_cmd tar
require_cmd curl
require_cmd sha256sum

tag="$(release_tag "${1:-}")"
version="$(release_version "${tag}")"
arch="${2:-${ARCH:-amd64}}"
case "${arch}" in
  amd64|arm64) ;;
  *) die "unsupported Debian architecture: ${arch}" ;;
esac

maintainer="${MAINTAINER:-repobridge maintainers <maintainers@repobridge.dev>}"
output_dir="${OUT_DIR:-${SCRIPT_DIR}/dist}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

asset="$(asset_name "${tag}" linux "${arch}" tar.gz)"
source_tarball="${SOURCE_TARBALL:-${tmp_dir}/${asset}}"
checksums_file="${CHECKSUMS_FILE:-${tmp_dir}/checksums.txt}"

if [[ -z "${SOURCE_TARBALL:-}" ]]; then
  curl -fsSL "$(asset_url "${tag}" "${asset}")" -o "${source_tarball}"
fi
if [[ -z "${CHECKSUMS_FILE:-}" ]]; then
  download_checksums "${tag}" "${checksums_file}"
fi

expected_sha="$(checksum_for_asset "${checksums_file}" "${asset}")"
actual_sha="$(sha256sum "${source_tarball}" | awk '{print $1}')"
[[ "${actual_sha}" == "${expected_sha}" ]] || die "checksum mismatch for ${asset}"

extract_dir="${tmp_dir}/extract"
pkg_dir="${tmp_dir}/${PRODUCT_NAME}_${version}_${arch}"
mkdir -p "${extract_dir}" "${pkg_dir}/DEBIAN" "${pkg_dir}/usr/bin" "${pkg_dir}/usr/share/doc/${PRODUCT_NAME}"
tar -xzf "${source_tarball}" -C "${extract_dir}"

binary_path="$(find "${extract_dir}" -type f -name "${PRODUCT_NAME}" -perm -111 | head -n 1)"
[[ -n "${binary_path}" ]] || die "could not find executable ${PRODUCT_NAME} in ${asset}"
install -m 0755 "${binary_path}" "${pkg_dir}/usr/bin/${PRODUCT_NAME}"

installed_size="$(du -sk "${pkg_dir}/usr" | awk '{print $1}')"
cat >"${pkg_dir}/DEBIAN/control" <<CONTROL
Package: ${PRODUCT_NAME}
Version: ${version}
Section: devel
Priority: optional
Architecture: ${arch}
Maintainer: ${maintainer}
Installed-Size: ${installed_size}
Homepage: ${HOMEPAGE_URL}
Description: ${PRODUCT_DESCRIPTION}
 repobridge turns package or repository specs into local source trees.
CONTROL

cat >"${pkg_dir}/usr/share/doc/${PRODUCT_NAME}/copyright" <<COPYRIGHT
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: ${PRODUCT_NAME}
Source: ${HOMEPAGE_URL}
License: ${LICENSE_ID}
COPYRIGHT

prepare_output_dir "${output_dir}"
dpkg-deb --build --root-owner-group "${pkg_dir}" "${output_dir}/${PRODUCT_NAME}_${version}_${arch}.deb"
echo "wrote ${output_dir}/${PRODUCT_NAME}_${version}_${arch}.deb"
