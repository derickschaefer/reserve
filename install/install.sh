#!/bin/sh

# Copyright (c) 2026 Derick Schaefer
# Licensed under the MIT License. See LICENSE file for details.

set -eu

BASE_URL="${RESERVE_DOWNLOAD_BASE_URL:-https://download.reservecli.dev}"
VERSION="${1:-latest}"

fail() {
  printf '%s\n' "reserve install: $*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

detect_os() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    darwin) printf '%s\n' "darwin" ;;
    linux) printf '%s\n' "linux" ;;
    *) fail "unsupported operating system: $os" ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) printf '%s\n' "amd64" ;;
    aarch64|arm64) printf '%s\n' "arm64" ;;
    *) fail "unsupported architecture: $arch" ;;
  esac
}

choose_install_dir() {
  if [ -d "/usr/local/bin" ] && [ -w "/usr/local/bin" ]; then
    printf '%s\n' "/usr/local/bin"
    return
  fi

  if [ ! -d "${HOME}/.local/bin" ]; then
    mkdir -p "${HOME}/.local/bin"
  fi
  printf '%s\n' "${HOME}/.local/bin"
}

download() {
  url="$1"
  out="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
    return
  fi

  if command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
    return
  fi

  fail "neither curl nor wget is available for downloading artifacts"
}

need_cmd uname
need_cmd tar
need_cmd chmod

OS="$(detect_os)"
ARCH="$(detect_arch)"
INSTALL_DIR="$(choose_install_dir)"
ARCHIVE="reserve_${OS}_${ARCH}.tar.gz"
URL="${BASE_URL}/releases/${VERSION}/${ARCHIVE}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

ARCHIVE_PATH="${TMPDIR}/${ARCHIVE}"
printf '%s\n' "reserve install: downloading ${URL}"
download "$URL" "$ARCHIVE_PATH"

printf '%s\n' "reserve install: extracting ${ARCHIVE}"
tar -xzf "$ARCHIVE_PATH" -C "$TMPDIR"

if [ ! -f "${TMPDIR}/reserve" ]; then
  fail "archive did not contain expected binary: reserve"
fi

chmod +x "${TMPDIR}/reserve"
TARGET="${INSTALL_DIR}/reserve"

if command -v install >/dev/null 2>&1; then
  install "${TMPDIR}/reserve" "$TARGET"
else
  cp "${TMPDIR}/reserve" "$TARGET"
  chmod +x "$TARGET"
fi

printf '\n%s\n' "reserve install: installed to ${TARGET}"
if ! printf '%s' ":$PATH:" | grep -q ":${INSTALL_DIR}:"; then
  printf '%s\n' "reserve install: add ${INSTALL_DIR} to your PATH if it is not already present"
fi
printf '%s\n' "reserve install: run 'reserve version' to verify the installation"
