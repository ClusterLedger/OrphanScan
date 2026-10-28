#!/usr/bin/env sh
# Offline installer regression tests. Every installation uses a temporary path.
set -eu
repo_dir="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
fixture_dir="$(mktemp -d)"
trap 'rm -rf "$fixture_dir"' EXIT
mkdir -p "$fixture_dir/bin" "$fixture_dir/payload" "$fixture_dir/install"
printf '#!/bin/sh\necho "OrphanScan 0.1.0"\n' > "$fixture_dir/payload/orphanscan"
for platform in Linux_x86_64 Linux_arm64 Darwin_x86_64 Darwin_arm64; do
  tar -czf "$fixture_dir/orphanscan_${platform}.tar.gz" -C "$fixture_dir/payload" orphanscan
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$fixture_dir" && sha256sum "orphanscan_${platform}.tar.gz") >> "$fixture_dir/checksums.txt"
  else
    (cd "$fixture_dir" && shasum -a 256 "orphanscan_${platform}.tar.gz") >> "$fixture_dir/checksums.txt"
  fi
done
mkdir -p "$fixture_dir/no-binary" "$fixture_dir/empty-binary"
printf 'readme only' > "$fixture_dir/no-binary/README"
: > "$fixture_dir/empty-binary/orphanscan"
for kind in no-binary empty-binary; do
  entry="orphanscan"
  if [ "$kind" = no-binary ]; then entry="README"; fi
  tar -czf "$fixture_dir/${kind}.tar.gz" -C "$fixture_dir/$kind" "$entry"
  if command -v sha256sum >/dev/null 2>&1; then
    checksum="$(sha256sum "$fixture_dir/${kind}.tar.gz" | awk '{print $1}')"
  else
    checksum="$(shasum -a 256 "$fixture_dir/${kind}.tar.gz" | awk '{print $1}')"
  fi
  printf '%s  orphanscan_Linux_x86_64.tar.gz\n' "$checksum" > "$fixture_dir/${kind}-checksums.txt"
done
cat > "$fixture_dir/bin/uname" <<'MOCK'
#!/bin/sh
case "$1" in -s) echo "${MOCK_OS:-Linux}" ;; -m) echo "${MOCK_ARCH:-x86_64}" ;; esac
MOCK
cat > "$fixture_dir/bin/curl" <<'MOCK'
#!/bin/sh
set -eu
out=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    http*) url="$1"; shift ;;
    *) shift ;;
  esac
done
case "$url" in
  */releases/latest)
    [ "${MOCK_FAILURE:-}" != api ] || exit 22
    printf '{"tag_name":"v0.1.0"}\n'
    ;;
  */checksums.txt)
    [ "${MOCK_FAILURE:-}" != checksums ] || exit 22
    case "${MOCK_FAILURE:-}" in
      mismatch) awk '{$1="0000000000000000000000000000000000000000000000000000000000000000";print}' "$FIXTURE_DIR/checksums.txt" > "$out" ;;
      no-binary|empty-binary) cp "$FIXTURE_DIR/${MOCK_FAILURE}-checksums.txt" "$out" ;;
      missing) printf 'no matching archive\n' > "$out" ;;
      *) cp "$FIXTURE_DIR/checksums.txt" "$out" ;;
    esac
    ;;
  *.tar.gz)
    [ "${MOCK_FAILURE:-}" != archive ] || exit 22
    if [ "${MOCK_FAILURE:-}" = no-binary ] || [ "${MOCK_FAILURE:-}" = empty-binary ]; then
      cp "$FIXTURE_DIR/${MOCK_FAILURE}.tar.gz" "$out"
    elif [ "${MOCK_FAILURE:-}" = corrupt ]; then
      printf 'corrupt download' > "$out"
    else
      cp "$FIXTURE_DIR/${url##*/}" "$out"
    fi
    ;;
  *) exit 22 ;;
esac
MOCK
chmod +x "$fixture_dir/bin/curl" "$fixture_dir/bin/uname"
export FIXTURE_DIR="$fixture_dir"
export PATH="$fixture_dir/bin:$PATH"
export INSTALL_DIR="$fixture_dir/install"

for spec in Linux:x86_64 Linux:aarch64 Darwin:x86_64 Darwin:arm64; do
  export MOCK_OS="${spec%:*}" MOCK_ARCH="${spec#*:}"
  sh "$repo_dir/install.sh" > "$fixture_dir/output" 2>&1
  [ "$("$INSTALL_DIR/orphanscan" --version)" = "OrphanScan 0.1.0" ] || exit 1
done
export MOCK_OS=Linux MOCK_ARCH=x86_64
# A pinned version must work even when the latest-release API is unavailable.
MOCK_FAILURE=api VERSION=v0.1.0 sh "$repo_dir/install.sh" > "$fixture_dir/output" 2>&1
for failure in api archive checksums mismatch missing corrupt no-binary empty-binary; do
  printf 'previous installation\n' > "$INSTALL_DIR/orphanscan"
  if MOCK_FAILURE="$failure" VERSION='' sh "$repo_dir/install.sh" > "$fixture_dir/output" 2>&1; then
    echo "expected installer failure: $failure" >&2; exit 1
  fi
  [ "$(cat "$INSTALL_DIR/orphanscan")" = 'previous installation' ] || exit 1
done
if MOCK_OS=FreeBSD sh "$repo_dir/install.sh" > "$fixture_dir/output" 2>&1; then exit 1; fi
if VERSION='v0.1.0/../../escape' sh "$repo_dir/install.sh" > "$fixture_dir/output" 2>&1; then exit 1; fi
if INSTALL_DIR=relative VERSION=v0.1.0 sh "$repo_dir/install.sh" > "$fixture_dir/output" 2>&1; then exit 1; fi
if INSTALL_DIR=/dev/null/orphanscan VERSION=v0.1.0 sh "$repo_dir/install.sh" > "$fixture_dir/output" 2>&1; then exit 1; fi
for kind in directory symlink; do
  blocked="$fixture_dir/blocked-$kind"
  mkdir -p "$blocked" "$fixture_dir/existing-$kind"
  printf 'preserved contents\n' > "$fixture_dir/existing-$kind/marker"
  if [ "$kind" = directory ]; then
    mv "$fixture_dir/existing-$kind" "$blocked/orphanscan"
  else
    ln -s "$fixture_dir/existing-$kind" "$blocked/orphanscan"
  fi
  if INSTALL_DIR="$blocked" VERSION=v0.1.0 sh "$repo_dir/install.sh" > "$fixture_dir/output" 2>&1; then
    echo "expected rejection of $kind destination" >&2; exit 1
  fi
  grep -q 'installation destination is a directory' "$fixture_dir/output"
  [ "$(cat "$blocked/orphanscan/marker")" = 'preserved contents' ] || exit 1
  for directory in "$blocked" "$blocked/orphanscan"; do
    [ "$(find "$directory/." ! -name . -prune -print | wc -l | tr -d ' ')" -eq 1 ] || exit 1
  done
done
echo 'Installer tests passed (offline fixtures, all Linux/macOS architecture mappings).'
