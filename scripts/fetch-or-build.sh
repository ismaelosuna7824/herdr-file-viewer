#!/usr/bin/env sh
# Install-time build step for the Herdr plugin.
#
# Downloads the prebuilt file-viewer binary for this platform from the GitHub
# release named in VERSION — so users do NOT need Go installed. If the download
# can't happen (offline, unsupported platform) it falls back to `go build` when
# Go is available, and otherwise fails with a clear message.
#
# Run from the plugin root as: sh scripts/fetch-or-build.sh

REPO="ismaelosuna7824/herdr-file-viewer"
DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$DIR/bin/file-viewer"
VERSION="$(cat "$DIR/VERSION" 2>/dev/null)"
mkdir -p "$DIR/bin"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  darwin | linux) : ;;
  *) os="" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *) arch="" ;;
esac

download() {
  [ -n "$os" ] && [ -n "$arch" ] && [ -n "$VERSION" ] || return 1
  url="https://github.com/$REPO/releases/download/$VERSION/file-viewer-$os-$arch"
  echo "Downloading prebuilt binary: $url"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$OUT" || return 1
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$OUT" "$url" || return 1
  else
    return 1
  fi
  chmod +x "$OUT"
}

build_from_source() {
  command -v go >/dev/null 2>&1 || return 1
  echo "Building from source with Go…"
  ( cd "$DIR" && go build -ldflags "-X main.version=$VERSION" -o bin/file-viewer ./cmd/file-viewer )
}

if download; then
  echo "Installed prebuilt file-viewer ($os-$arch, $VERSION)."
elif build_from_source; then
  echo "Built file-viewer from source."
else
  echo "ERROR: no prebuilt binary for this platform and Go is not installed." >&2
  echo "Install Go (https://go.dev/dl) or file an issue for your platform." >&2
  exit 1
fi
