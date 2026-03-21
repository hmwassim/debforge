#!/usr/bin/env bash
set -euo pipefail

REPO="lutris/lutris"
PKG_NAME="lutris"

for cmd in curl jq dpkg; do
    command -v "$cmd" &>/dev/null || { echo "$cmd is required."; exit 1; }
done

echo "==> Fetching latest Lutris release info..."
RELEASE_JSON=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest")

LATEST_VERSION=$(echo "$RELEASE_JSON" | jq -r '.tag_name | sub("^v"; "")')

if [[ -z "$LATEST_VERSION" || "$LATEST_VERSION" == "null" ]]; then
    echo "ERROR: Failed to determine latest version."
    exit 1
fi

INSTALLED_VERSION=""
if dpkg -s "$PKG_NAME" &>/dev/null; then
    INSTALLED_VERSION=$(dpkg-query -W -f='${Version}' "$PKG_NAME" | cut -d- -f1)
fi

if [[ "$INSTALLED_VERSION" == "$LATEST_VERSION" ]]; then
    echo "Lutris $INSTALLED_VERSION is already the latest version."
    exit 0
fi

echo "Installed: ${INSTALLED_VERSION:-none}  Latest: $LATEST_VERSION"

echo "==> Searching for .deb package..."

DOWNLOAD_URL=$(echo "$RELEASE_JSON" | jq -r '
    .assets[]
    | select(.name | test("\\.deb$"))
    | select(.name | test("all|amd64"))
    | .browser_download_url
    ' | head -n1
)

if [[ -z "$DOWNLOAD_URL" ]]; then
    echo "ERROR: No suitable .deb found in the latest release."
    exit 1
fi

FILENAME=$(basename "$DOWNLOAD_URL")
TMP_FILE="$(mktemp -d)/$FILENAME"

echo "==> Downloading $FILENAME..."
curl -fL -o "$TMP_FILE" "$DOWNLOAD_URL"

echo "==> Installing..."
sudo dpkg -i "$TMP_FILE" || sudo apt-get install -f -y

rm -f "$TMP_FILE"

echo "==> Lutris $LATEST_VERSION installed / updated successfully."
