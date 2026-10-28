#!/usr/bin/env sh
# Install a checksum-verified release. Optional VERSION / first argument pins it.
set -eu

REPO="ClusterLedger/OrphanScan"
BINARY="orphanscan"
BASE_URL="https://github.com/${REPO}/releases"

die() { echo "install.sh: $*" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"; }
for cmd in curl tar uname mktemp awk sed grep tr; do need_cmd "$cmd"; done
if command -v sha256sum >/dev/null 2>&1; then
  checksum_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  checksum_cmd="shasum"
else
  die "SHA-256 verification requires sha256sum or shasum"
fi

[ "$#" -le 1 ] || die "usage: install.sh [vVERSION]"
tag="${1:-${VERSION:-}}"
if [ -n "$tag" ]; then
  printf '%s\n' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$' || die "invalid release version"
fi
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
  linux) goos="Linux" ;;
  darwin) goos="Darwin" ;;
  *) die "unsupported OS: $os (supported: linux, darwin)" ;;
esac
case "$arch" in
  x86_64|amd64) goarch="x86_64" ;;
  aarch64|arm64) goarch="arm64" ;;
  *) die "unsupported architecture: $arch (supported: amd64, arm64)" ;;
esac
asset="${BINARY}_${goos}_${goarch}.tar.gz"

if [ -z "$tag" ]; then
  echo "Fetching latest release for ${REPO}..."
  release="$(curl -fsSL --connect-timeout 10 --max-time 120 --retry 2 "https://api.github.com/repos/${REPO}/releases/latest")" || die "could not fetch the latest release; check connectivity and whether a release is published"
  tag="$(printf '%s\n' "$release" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  printf '%s\n' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$' || die "could not determine a release version"
fi

tmpdir="$(mktemp -d)"
install_tmp=""
cleanup() {
  rm -rf "$tmpdir"
  if [ -n "$install_tmp" ]; then rm -f "$install_tmp"; fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
url="${BASE_URL}/download/${tag}"
echo "Downloading ${asset} (${tag})..."
curl -fsSL --connect-timeout 10 --max-time 120 --retry 2 -o "${tmpdir}/${asset}" "${url}/${asset}" || die "archive download failed"
curl -fsSL --connect-timeout 10 --max-time 120 --retry 2 -o "${tmpdir}/checksums.txt" "${url}/checksums.txt" || die "checksum download failed"
expected="$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1 }' "${tmpdir}/checksums.txt")"
[ "${#expected}" -eq 64 ] || die "release checksum is missing or malformed"
case "$expected" in *[!0-9a-fA-F]*) die "release checksum is malformed" ;; esac
if [ "$checksum_cmd" = "sha256sum" ]; then
  actual="$(sha256sum "${tmpdir}/${asset}" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "${tmpdir}/${asset}" | awk '{print $1}')"
fi
[ "$actual" = "$expected" ] || die "SHA-256 checksum mismatch; installation aborted"

# Extract only the binary to stdout so archive paths cannot write outside tmpdir.
tar -xOzf "${tmpdir}/${asset}" "$BINARY" > "${tmpdir}/${BINARY}" || die "archive did not contain ${BINARY}"
[ -s "${tmpdir}/${BINARY}" ] || die "archive contained an empty binary"
install_dir="${INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    install_dir="/usr/local/bin"
  else
    install_dir="${HOME}/.local/bin"
  fi
fi
case "$install_dir" in /*) ;; *) die "INSTALL_DIR must be an absolute path" ;; esac
mkdir -p "$install_dir" || die "could not create installation directory"
[ ! -d "${install_dir}/${BINARY}" ] || die "installation destination is a directory: ${install_dir}/${BINARY}"
install_tmp="$(mktemp "${install_dir}/.${BINARY}.XXXXXX")" || die "installation directory is not writable"
cp "${tmpdir}/${BINARY}" "$install_tmp"
chmod 755 "$install_tmp"
mv -f "$install_tmp" "${install_dir}/${BINARY}"
install_tmp=""
echo "Installed ${BINARY} ${tag} to ${install_dir}/${BINARY}"
case ":$PATH:" in
  *":${install_dir}:"*) ;;
  *) echo "Add to PATH: export PATH=\"${install_dir}:\$PATH\"" ;;
esac
echo "Run: ${BINARY} --help"
