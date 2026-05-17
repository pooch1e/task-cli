#!/usr/bin/env bash
# install.sh — install the latest task-cli binary
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/pooch1e/task-cli/main/install.sh | bash
#
# Environment overrides:
#   INSTALL_DIR   — destination directory (default: ~/.local/bin)
#   VERSION       — specific tag to install (default: latest release)

set -euo pipefail

REPO="pooch1e/task-cli"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"

# ── detect OS and architecture ────────────────────────────────────────────────

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "${OS}" in
  darwin) ;;
  linux)  ;;
  *)      echo "error: unsupported OS '${OS}'" >&2; exit 1 ;;
esac

case "${ARCH}" in
  x86_64)       ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)            echo "error: unsupported architecture '${ARCH}'" >&2; exit 1 ;;
esac

BINARY_NAME="task-${OS}-${ARCH}"

# ── resolve version ───────────────────────────────────────────────────────────

if [[ -z "${VERSION:-}" ]]; then
  echo "Fetching latest release..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | head -1 \
    | sed 's/.*"tag_name": "\(.*\)".*/\1/')
fi

if [[ -z "${VERSION}" ]]; then
  echo "error: could not determine latest release version" >&2
  exit 1
fi

echo "Installing task-cli ${VERSION} for ${OS}/${ARCH}..."

# ── download ──────────────────────────────────────────────────────────────────

BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

curl -fsSL "${BASE_URL}/${BINARY_NAME}"   -o "${TMP_DIR}/task"
curl -fsSL "${BASE_URL}/checksums.txt"    -o "${TMP_DIR}/checksums.txt"

# ── verify checksum ───────────────────────────────────────────────────────────

cd "${TMP_DIR}"

EXPECTED=$(grep "${BINARY_NAME}" checksums.txt | awk '{print $1}')
if [[ -z "${EXPECTED}" ]]; then
  echo "error: no checksum found for '${BINARY_NAME}'" >&2
  exit 1
fi

# sha256sum on Linux, shasum on macOS
if command -v sha256sum &>/dev/null; then
  ACTUAL=$(sha256sum task | awk '{print $1}')
else
  ACTUAL=$(shasum -a 256 task | awk '{print $1}')
fi

if [[ "${EXPECTED}" != "${ACTUAL}" ]]; then
  echo "error: checksum mismatch" >&2
  echo "  expected: ${EXPECTED}" >&2
  echo "  actual:   ${ACTUAL}" >&2
  exit 1
fi

echo "Checksum OK"

# ── install ───────────────────────────────────────────────────────────────────

chmod +x task
mkdir -p "${INSTALL_DIR}"
mv task "${INSTALL_DIR}/task"

echo "Installed task-cli ${VERSION} → ${INSTALL_DIR}/task"

# Suggest adding to PATH if not already there
if ! echo "${PATH}" | grep -q "${INSTALL_DIR}"; then
  echo ""
  echo "Note: ${INSTALL_DIR} is not in your PATH."
  echo "Add this to your shell profile:"
  echo "  export PATH=\"\${HOME}/.local/bin:\${PATH}\""
fi
