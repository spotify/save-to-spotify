#!/bin/bash
# sign-and-notarize.sh
#
# Signs and notarizes one or more macOS binaries for distribution.
#
# Required environment variables:
#   CERTIFICATE_BASE64             - Base64-encoded .p12 signing certificate
#   CERTIFICATE_PASSWORD           - Password for the .p12 certificate
#   APPLE_API_KEY_BASE64           - Base64-encoded App Store Connect API key (.p8)
#   APPLE_API_KEY_ID               - App Store Connect API Key ID (e.g. W2TN8NR252)
#   APPLE_API_ISSUER_ID            - App Store Connect API Issuer ID
#
# Usage:
#   ./sign-and-notarize.sh <path-to-binary> [<path-to-binary>...]

set -euo pipefail

# --- Platform check ---
if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Error: This script only runs on macOS (Darwin), detected: $(uname -s)"
  exit 1
fi

# --- Input validation ---
if [[ "$#" -lt 1 ]]; then
  echo "Usage: $0 <path-to-binary> [<path-to-binary>...]"
  exit 1
fi

BINARIES=("$@")
for BINARY in "${BINARIES[@]}"; do
  if [[ ! -f "$BINARY" ]]; then
    echo "Error: binary not found: $BINARY"
    exit 1
  fi
done

for var in CERTIFICATE_BASE64 CERTIFICATE_PASSWORD APPLE_API_KEY_BASE64 APPLE_API_KEY_ID APPLE_API_ISSUER_ID; do
  if [[ -z "${!var:-}" ]]; then
    echo "Error: $var is not set"
    exit 1
  fi
done

# --- Configuration ---
SIGNING_IDENTITY="Developer ID Application: Spotify (2FNC3A47ZF)"
KEYCHAIN_NAME="build-signing.keychain-db"
KEYCHAIN_PATH="${RUNNER_TEMP:-/tmp}/${KEYCHAIN_NAME}"
KEYCHAIN_PASSWORD="$(openssl rand -hex 24)"
P12_PATH="${RUNNER_TEMP:-/tmp}/certificate.p12"
API_KEY_PATH="${RUNNER_TEMP:-/tmp}/AuthKey.p8"

# Store original keychain list for restoration
ORIGINAL_KEYCHAINS=$(security list-keychains -d user | xargs)

# --- Cleanup on exit ---
cleanup() {
  echo "==> Cleaning up"

  if [[ -n "${ORIGINAL_KEYCHAINS:-}" ]]; then
    security list-keychains -d user -s $ORIGINAL_KEYCHAINS 2>/dev/null || true
  fi

  security lock-keychain "$KEYCHAIN_PATH" 2>/dev/null || true
  security delete-keychain "$KEYCHAIN_PATH" 2>/dev/null || true
  rm -f "$P12_PATH" "$API_KEY_PATH"
  for BINARY in "${BINARIES[@]}"; do
    rm -f "${BINARY}.zip"
  done
  echo "==> Cleanup complete"
}
trap cleanup EXIT

# --- Step 1: Set up keychain ---
echo "==> Creating temporary keychain"
security create-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN_PATH"
security set-keychain-settings -lut 21600 "$KEYCHAIN_PATH"
security unlock-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN_PATH"

echo "==> Importing signing certificate"
echo -n "$CERTIFICATE_BASE64" | base64 --decode > "$P12_PATH"
security import "$P12_PATH" -P "$CERTIFICATE_PASSWORD" -A -t cert -f pkcs12 -k "$KEYCHAIN_PATH"
rm -f "$P12_PATH"

security set-key-partition-list -S apple-tool:,apple: -k "$KEYCHAIN_PASSWORD" "$KEYCHAIN_PATH"
security list-keychains -d user -s $ORIGINAL_KEYCHAINS "$KEYCHAIN_PATH"

echo "==> Verifying signing identity"
security find-identity -v -p codesigning "$KEYCHAIN_PATH" | grep "$SIGNING_IDENTITY"

echo "==> Writing API key"
echo "$APPLE_API_KEY_BASE64" | base64 --decode > "$API_KEY_PATH"

for BINARY in "${BINARIES[@]}"; do
  echo "==> Signing binary: $BINARY"
  codesign --force --options runtime \
    --keychain "$KEYCHAIN_PATH" \
    --sign "$SIGNING_IDENTITY" \
    --timestamp \
    "$BINARY"

  echo "==> Verifying signature"
  codesign --verify --strict "$BINARY"
  codesign -dv --verbose=2 "$BINARY"

  echo "==> Submitting for notarization: $BINARY"
  ditto -c -k --keepParent "$BINARY" "${BINARY}.zip"

  xcrun notarytool submit "${BINARY}.zip" \
    --key "$API_KEY_PATH" \
    --key-id "$APPLE_API_KEY_ID" \
    --issuer "$APPLE_API_ISSUER_ID" \
    --wait

  echo "==> Notarization complete: $BINARY"

  echo "==> Final signature check"
  codesign --verify --strict "$BINARY" && echo "Signature: OK"
done

echo "==> Done. Binaries are signed and notarized"
