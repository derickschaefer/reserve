#!/usr/bin/env bash

# Copyright (c) 2026 Derick Schaefer
# Licensed under the MIT License. See LICENSE file for details.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${VERSION:-}}"
DIST_DIR="${DIST_DIR:-${ROOT_DIR}/dist}"
BINARY="${BINARY:-reserve}"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
MANIFEST_SOURCE="${MANIFEST_SOURCE:-${ROOT_DIR}/release-manifest.json}"
export GOCACHE="${GOCACHE:-${ROOT_DIR}/.gocache}"
export GOMODCACHE="${GOMODCACHE:-${ROOT_DIR}/.gomodcache}"
# Avoid macOS-specific metadata in release archives so Linux tar/unzip runs cleanly.
export COPYFILE_DISABLE=1

if [[ -z "${VERSION}" ]]; then
  echo "usage: scripts/build-dist.sh <version>" >&2
  exit 1
fi

if [[ ! -f "${MANIFEST_SOURCE}" ]]; then
  echo "missing release manifest source: ${MANIFEST_SOURCE}" >&2
  exit 1
fi

if ! grep -q "\"latest_version\": \"${VERSION}\"" "${MANIFEST_SOURCE}"; then
  echo "release manifest latest_version does not match build version ${VERSION}" >&2
  exit 1
fi

BUILD_DIR="${DIST_DIR}/build"
VERSION_DIR="${DIST_DIR}/releases/${VERSION}"
LATEST_DIR="${DIST_DIR}/releases/latest"

rm -rf "${BUILD_DIR}" "${VERSION_DIR}" "${LATEST_DIR}"
mkdir -p "${BUILD_DIR}" "${VERSION_DIR}" "${LATEST_DIR}"

targets=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

for target in "${targets[@]}"; do
  read -r GOOS GOARCH <<<"${target}"
  ext=""
  if [[ "${GOOS}" == "windows" ]]; then
    ext=".exe"
  fi

  binary_path="${BUILD_DIR}/${BINARY}${ext}"
  archive_base="${BINARY}_${GOOS}_${GOARCH}"
  ldflags="-s -w -X github.com/derickschaefer/reserve/cmd.Version=${VERSION} -X github.com/derickschaefer/reserve/cmd.BuildTime=${BUILD_TIME}"

  echo "building ${archive_base}"
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build -trimpath -ldflags="${ldflags}" -o "${binary_path}" "${ROOT_DIR}"

  if [[ "${GOOS}" == "windows" ]]; then
    archive_name="${archive_base}.zip"
    (
      cd "${BUILD_DIR}"
      zip -q "${VERSION_DIR}/${archive_name}" "$(basename "${binary_path}")"
    )
  else
    archive_name="${archive_base}.tar.gz"
    (
      cd "${BUILD_DIR}"
      tar -czf "${VERSION_DIR}/${archive_name}" "$(basename "${binary_path}")"
    )
  fi

  cp "${VERSION_DIR}/${archive_name}" "${LATEST_DIR}/${archive_name}"
  rm -f "${binary_path}"
done

cp "${ROOT_DIR}/install/install.sh" "${DIST_DIR}/install.sh"
cp "${ROOT_DIR}/install/install.ps1" "${DIST_DIR}/install.ps1"
cp "${MANIFEST_SOURCE}" "${DIST_DIR}/release.json"
cp "${MANIFEST_SOURCE}" "${VERSION_DIR}/release.json"
cp "${MANIFEST_SOURCE}" "${LATEST_DIR}/release.json"

(
  cd "${VERSION_DIR}"
  sha256sum *.tar.gz *.zip > SHA256SUMS
)
cp "${VERSION_DIR}/SHA256SUMS" "${LATEST_DIR}/SHA256SUMS"

echo "wrote Cloudflare distribution layout under ${DIST_DIR}"
