#!/usr/bin/env bash
set -euo pipefail

repo="agentsw0rk/repobridge"
install_dir="${REPOBRIDGE_INSTALL_DIR:-${HOME}/.local/bin}"
version="${1:-${REPOBRIDGE_VERSION:-}}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "repobridge installer: missing required command: $1" >&2
    exit 1
  fi
}

need curl
need uname

curl_args=(-fsSL)
if [[ -n "${GITHUB_TOKEN:-}" ]]; then
  curl_args+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
fi

if [[ -z "$version" ]]; then
  latest_json="$(curl "${curl_args[@]}" "https://api.github.com/repos/${repo}/releases/latest")"
  version="$(printf '%s\n' "$latest_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
fi

if [[ -z "$version" ]]; then
  echo "repobridge installer: could not determine latest release version" >&2
  exit 1
fi

case "$(uname -s)" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  MINGW*|MSYS*|CYGWIN*) os="windows" ;;
  *)
    echo "repobridge installer: unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "repobridge installer: unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

if [[ "$os" == "windows" && "$arch" != "amd64" ]]; then
  echo "repobridge installer: Windows release assets are currently available for amd64 only" >&2
  exit 1
fi

if [[ "$os" == "windows" ]]; then
  ext="zip"
  need unzip
else
  ext="tar.gz"
  need tar
fi

asset="repobridge_${version}_${os}_${arch}.${ext}"
url="https://github.com/${repo}/releases/download/${version}/${asset}"
tmp_dir="$(mktemp -d)"
archive="${tmp_dir}/${asset}"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

echo "Downloading ${asset}"
curl "${curl_args[@]}" -o "$archive" "$url"

if [[ "$ext" == "zip" ]]; then
  unzip -q "$archive" -d "$tmp_dir"
else
  tar -xzf "$archive" -C "$tmp_dir"
fi

package_dir="$(find "$tmp_dir" -mindepth 1 -maxdepth 1 -type d -name "repobridge_*" | head -n 1)"
if [[ -z "$package_dir" ]]; then
  echo "repobridge installer: release archive did not contain a repobridge package directory" >&2
  exit 1
fi

mkdir -p "$install_dir"
cp -R "$package_dir"/. "$install_dir"/

if [[ -f "$install_dir/repobridge" ]]; then
  chmod +x "$install_dir/repobridge"
  binary="$install_dir/repobridge"
elif [[ -f "$install_dir/repobridge.exe" ]]; then
  binary="$install_dir/repobridge.exe"
else
  echo "repobridge installer: binary was not found after install" >&2
  exit 1
fi

echo "Installed repobridge ${version} to ${binary}"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *)
    echo "Add ${install_dir} to PATH to run repobridge from any directory." >&2
    ;;
esac

"$binary" --version
