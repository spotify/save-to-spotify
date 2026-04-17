#!/bin/bash
# Signs, verifies, and optionally notarizes a macOS binary in CI
#
# Signing env vars (required):
#   MACOS_KEYCHAIN          base64-encoded .keychain-db file
#   MACOS_KEYCHAIN_PWD      password to unlock the keychain
#   MACOS_SIGN_IDENTITY     e.g. "Developer ID Application: You (Your ID)"
#
# Notarization env vars (required if --notarize):
#   MACOS_NOTARY_KEY        base64-encoded App Store Connect API key (.p8)
#   MACOS_NOTARY_KEY_ID     Key ID
#   MACOS_NOTARY_ISSUER_ID  Issuer ID (UUID)
#
# Usage: sign-and-notarize.sh <binary>

set -euo pipefail

# --- Platform check ---
if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Error: This script only runs on macOS (Darwin), detected: $(uname -s)"
  exit 1
fi

BINARY="${1:?binary path required}"

: "${RUNNER_TEMP:=$(mktemp -d)}"
KEYCHAIN="$RUNNER_TEMP/signing.keychain-db"

# Snapshot the original search list so we can restore it on exit. Without this,
# the temp keychain lingers in the search list if the script crashes.
ORIGINAL_KEYCHAINS=$(security list-keychains -d user | tr -d '"' | xargs)

cleanup() {
  security list-keychains -d user -s $ORIGINAL_KEYCHAINS 2>/dev/null || true
  security delete-keychain "$KEYCHAIN" 2>/dev/null || true
  rm -f "$RUNNER_TEMP/notary.p8"
}
trap cleanup EXIT
echo "==> Decoding keychain"
echo "$MACOS_KEYCHAIN" | base64 --decode > "$KEYCHAIN"

echo "==> Unlocking keychain"
security unlock-keychain -p "$MACOS_KEYCHAIN_PWD" "$KEYCHAIN"
# Prevent auto-lock during the job (default is a few minutes of idle)
security set-keychain-settings -lut 21600 "$KEYCHAIN"

echo "==> Adding keychain to search list"
security list-keychains -d user -s "$KEYCHAIN" $ORIGINAL_KEYCHAINS

security set-key-partition-list \
  -S apple-tool:,apple:,codesign: \
  -s -k "$MACOS_KEYCHAIN_PWD" "$KEYCHAIN" >/dev/null 2>&1 || true

echo "==> Verifying signing identity is available"
security find-identity -v -p codesigning "$KEYCHAIN" \
  | grep -q "$MACOS_SIGN_IDENTITY" \
  || { echo "Signing identity not found in keychain"; exit 1; }

echo "==> Signing $BINARY"
codesign --force \
  --sign "$MACOS_SIGN_IDENTITY" \
  --options runtime \
  --timestamp \
  --keychain "$KEYCHAIN" \
  "$BINARY"

echo "==> Verifying signature"
codesign --verify --strict --verbose=2 "$BINARY"
codesign --display --verbose=2 "$BINARY"

echo "==> Notarizing"
KEY="$RUNNER_TEMP/notary.p8"
echo "$MACOS_NOTARY_KEY" | base64 --decode > "$KEY"

# notarytool needs a zip/dmg/pkg container, not a bare Mach-O
ZIP="$RUNNER_TEMP/$(basename "$BINARY").zip"
ditto -c -k --keepParent "$BINARY" "$ZIP"

xcrun notarytool submit "$ZIP" \
  --key "$KEY" \
  --key-id "$MACOS_NOTARY_KEY_ID" \
  --issuer "$MACOS_NOTARY_ISSUER_ID" \
  --wait

echo "==> Done."
