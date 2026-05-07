#!/usr/bin/env bash
# Signs, verifies, and notarizes one or more macOS binaries in CI
# using a pre-packaged keychain shipped via CI secrets.
#
# Signing env vars (required):
#   MACOS_KEYCHAIN          base64-encoded .keychain-db file
#   MACOS_KEYCHAIN_PWD      password to unlock the keychain
#   MACOS_SIGN_IDENTITY     e.g. "Developer ID Application: Spotify (2FNC3A47ZF)"
#
# Notarization env vars (required):
#   MACOS_NOTARY_KEY        base64-encoded App Store Connect API key (.p8)
#   MACOS_NOTARY_KEY_ID     Key ID
#   MACOS_NOTARY_ISSUER_ID  Issuer ID (UUID)
#
# Usage: sign-and-notarize.sh <binary> [<binary> ...]

set -euo pipefail

# --- Platform check ---
if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Error: This script only runs on macOS (Darwin), detected: $(uname -s)"
  exit 1
fi

if [[ "$#" -eq 0 ]]; then
  echo "Usage: sign-and-notarize.sh <binary> [<binary> ...]"
  exit 2
fi

BINARIES=("$@")

for var in MACOS_KEYCHAIN MACOS_KEYCHAIN_PWD MACOS_SIGN_IDENTITY MACOS_NOTARY_KEY MACOS_NOTARY_KEY_ID MACOS_NOTARY_ISSUER_ID; do
  if [[ -z "${!var:-}" ]]; then
    echo "Error: ${var} is not set"
    exit 1
  fi
done

: "${RUNNER_TEMP:=$(mktemp -d)}"
KEYCHAIN="$RUNNER_TEMP/signing.keychain-db"

# Snapshot the original search list so we can restore it on exit. Without this,
# the temp keychain lingers in the search list if the script crashes.
ORIGINAL_KEYCHAINS=()
while IFS= read -r keychain; do
  ORIGINAL_KEYCHAINS+=("$keychain")
done < <(security list-keychains -d user | sed 's/^[[:space:]]*"//; s/"$//')

cleanup() {
  security list-keychains -d user -s "${ORIGINAL_KEYCHAINS[@]}" 2>/dev/null || true
  security delete-keychain "$KEYCHAIN" 2>/dev/null || true
  rm -rf "$RUNNER_TEMP/notary-submission"
  rm -f "$RUNNER_TEMP/notary-submission.zip"
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
security list-keychains -d user -s "$KEYCHAIN" "${ORIGINAL_KEYCHAINS[@]}"

# Pre-authorize codesign to use the private key without a UI prompt.
# Harmless if already set; required on fresh imports.
security set-key-partition-list \
  -S apple-tool:,apple:,codesign: \
  -s -k "$MACOS_KEYCHAIN_PWD" "$KEYCHAIN" >/dev/null 2>&1 || true

echo "==> Verifying signing identity is available"
security find-identity -v -p codesigning "$KEYCHAIN" \
  | grep -q "$MACOS_SIGN_IDENTITY" \
  || { echo "Signing identity not found in keychain"; exit 1; }

for binary in "${BINARIES[@]}"; do
  if [[ ! -f "$binary" ]]; then
    echo "Binary not found: $binary"
    exit 1
  fi

  echo "==> Signing $binary"
  codesign --force \
    --sign "$MACOS_SIGN_IDENTITY" \
    --options runtime \
    --timestamp \
    --keychain "$KEYCHAIN" \
    "$binary"
done

echo "==> Verifying signature"
codesign --verify --strict --verbose=2 "${BINARIES[@]}"
for binary in "${BINARIES[@]}"; do
  codesign --display --verbose=2 "$binary"
done

echo "==> Notarizing"
KEY="$RUNNER_TEMP/notary.p8"
echo "$MACOS_NOTARY_KEY" | base64 --decode > "$KEY"

# notarytool accepts a single archive, so collect every requested binary into
# one submission bundle while preserving their relative paths.
ARCHIVE_ROOT="$RUNNER_TEMP/notary-submission"
mkdir -p "$ARCHIVE_ROOT"
for binary in "${BINARIES[@]}"; do
  relative_path="$binary"
  if [[ "$relative_path" = /* ]]; then
    relative_path="${relative_path#/}"
  fi

  mkdir -p "$ARCHIVE_ROOT/$(dirname "$relative_path")"
  ditto "$binary" "$ARCHIVE_ROOT/$relative_path"
done

ZIP="$RUNNER_TEMP/notary-submission.zip"
ditto -c -k --keepParent "$ARCHIVE_ROOT" "$ZIP"

xcrun notarytool submit "$ZIP" \
  --key "$KEY" \
  --key-id "$MACOS_NOTARY_KEY_ID" \
  --issuer "$MACOS_NOTARY_ISSUER_ID" \
  --wait

echo "==> Done."
