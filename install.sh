#!/usr/bin/env sh
# Install the latest OrphanScan release binary for the current OS/arch.
# Usage: curl -sSL https://raw.githubusercontent.com/ClusterLedger/OrphanScan/main/install.sh | sh
set -eu

REPO="ClusterLedger/OrphanScan"
BINARY="orphanscan"
BASE_URL="https://github.com/${REPO}/releases"

die() {
  echo "install.sh: $*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

need_cmd curl
need_cmd tar
need_cmd uname
need_cmd mktemp

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  linux)  goos="Linux" ;;
  darwin) goos="Darwin" ;;
  *)      die "unsupported OS: $os (supported: linux, darwin)" ;;
esac

case "$arch" in
  x86_64|amd64) goarch="x86_64" ;;
  aarch64|arm64) goarch="arm64" ;;
  *) die "unsupported architecture: $arch (supported: amd64, arm64)" ;;
esac

asset="${BINARY}_${goos}_${goarch}.tar.gz"

echo "Fetching latest release for ${REPO}..."
tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
[ -n "$tag" ] || die "could not determine latest release tag (is the repo public and does it have a release?)"

url="${BASE_URL}/download/${tag}/${asset}"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "Downloading ${asset} (${tag})..."
curl -fsSL -o "${tmpdir}/${asset}" "$url" || die "download failed: $url"

tar -xzf "${tmpdir}/${asset}" -C "$tmpdir"
[ -f "${tmpdir}/${BINARY}" ] || die "archive did not contain ${BINARY}"
chmod +x "${tmpdir}/${BINARY}"

install_dir=""
if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
  install_dir="/usr/local/bin"
elif mkdir -p "${HOME}/.local/bin" 2>/dev/null; then
  install_dir="${HOME}/.local/bin"
else
  die "could not find a writable install directory"
fi

mv "${tmpdir}/${BINARY}" "${install_dir}/${BINARY}"
echo "Installed ${BINARY} ${tag} to ${install_dir}/${BINARY}"

case ":$PATH:" in
  *":${install_dir}:"*) ;;
  *)
    echo "Note: ${install_dir} is not on your PATH. Add it with:"
    echo "  export PATH=\"${install_dir}:\$PATH\""
    ;;
esac

echo "Run: ${BINARY} --help"
