#!/bin/sh
set -e

REPO="zenk-t-suzuki/AgentsBuilder"
BIN="agentsbuilder"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Fetch latest release tag from GitHub API
echo "Fetching latest release..."
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' \
  | sed 's/.*"tag_name": *"\(.*\)".*/\1/')

if [ -z "$TAG" ]; then
  echo "Failed to fetch latest release tag."
  exit 1
fi

URL="https://github.com/${REPO}/releases/download/${TAG}/${BIN}-linux-${ARCH}"

echo "Downloading ${BIN} ${TAG} (linux/${ARCH})..."
curl -fsSL "$URL" -o "/tmp/${BIN}"
chmod +x "/tmp/${BIN}"

echo "Installing to ${INSTALL_DIR}/${BIN} ..."
if [ -w "$INSTALL_DIR" ]; then
  mv "/tmp/${BIN}" "${INSTALL_DIR}/${BIN}"
else
  sudo mv "/tmp/${BIN}" "${INSTALL_DIR}/${BIN}"
fi

echo "Done! Run: ${BIN}"
