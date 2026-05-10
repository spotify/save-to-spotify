#!/usr/bin/env bash
#
# Install the save-to-spotify CLI from GitHub Releases.
#
# Also installs the save-to-spotify agent skill into the global skill
# directories of supported coding agents (Claude Code, Cursor, Codex/OpenCode,
# and any tool that reads ~/.agents/skills/). Pass --no-skills to opt out.
#
# Usage:
#   curl -fsSL https://saveto.spotify.com/install.sh | bash
#   curl -fsSL https://saveto.spotify.com/install.sh | bash -s -- --version 0.1.1-rc.1
#   curl -fsSL https://saveto.spotify.com/install.sh | bash -s -- --dir ~/.local/bin
#   curl -fsSL https://saveto.spotify.com/install.sh | bash -s -- --no-skills
#
# Environment variables:
#   SAVE_TO_SPOTIFY_INSTALL_DIR     Override install directory
#   SAVE_TO_SPOTIFY_VERSION         Override version to install (e.g. 0.1.1-rc.1)
#   SAVE_TO_SPOTIFY_INSTALL_SKILLS  Set to "false" to skip skill install
#   GITHUB_TOKEN / GH_TOKEN         Auth for private repo or higher API rate limits (optional)
#

set -euo pipefail

# --- Logging ---

_ansi=false
if [ -t 2 ] && [ "${TERM+set}" = "set" ]; then
  case "$TERM" in
  xterm* | rxvt* | urxvt* | linux* | vt* | screen* | tmux*) _ansi=true ;;
  esac
fi

info() {
  if $_ansi; then
    printf '\033[1;34m==>\033[0m %s\n' "$@" >&2
  else
    printf '==> %s\n' "$@" >&2
  fi
}

warn() {
  if $_ansi; then
    printf '\033[1;33m==> WARNING:\033[0m %s\n' "$@" >&2
  else
    printf '==> WARNING: %s\n' "$@" >&2
  fi
}

err() {
  if $_ansi; then
    printf '\033[1;31m==> ERROR:\033[0m %s\n' "$@" >&2
  else
    printf '==> ERROR: %s\n' "$@" >&2
  fi
  exit 1
}

# --- Utility ---

check_cmd() { command -v "$1" >/dev/null 2>&1; }

need_cmd() {
  if ! check_cmd "$1"; then
    err "need '$1' (command not found)"
  fi
}

json_string_field() {
  local field="$1"
  sed -n "s/.*\"${field}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -1
}

# Curl wrapper that adds a GitHub auth header when GITHUB_TOKEN/GH_TOKEN is set.
gh_curl() {
  local token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
  if [[ -n "$token" ]]; then
    curl -fsSL -H "Authorization: Bearer ${token}" "$@"
  else
    curl -fsSL "$@"
  fi
}

# Find an asset's numeric id within a release JSON document (read from stdin),
# matching the asset by name. Empty stdout if not found.
asset_id_for_name() {
  local target="$1"
  awk -v target="$target" '
    /"assets":/ { in_assets = 1 }
    in_assets && /"id":/ {
      id = $0
      gsub(/.*"id":[[:space:]]*/, "", id)
      gsub(/[[:space:]]*,.*/, "", id)
    }
    in_assets && /"name":/ {
      name = $0
      gsub(/.*"name":[[:space:]]*"/, "", name)
      gsub(/".*/, "", name)
      if (name == target) {
        print id
        exit
      }
    }
  '
}

# --- Core functions ---

determine_install_dir() {
  if [[ -n "${SAVE_TO_SPOTIFY_INSTALL_DIR:-}" ]]; then
    INSTALL_DIR="$SAVE_TO_SPOTIFY_INSTALL_DIR"
    return
  fi

  if [[ "$os" == "windows" ]]; then
    INSTALL_DIR="${LOCALAPPDATA:-$HOME/AppData/Local}/save-to-spotify"
    mkdir -p "$INSTALL_DIR"
    return
  fi

  if [[ -d /usr/local/bin && -w /usr/local/bin ]]; then
    INSTALL_DIR="/usr/local/bin"
    return
  fi

  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
}

verify_checksum() {
  local file="$1"
  local checksum_file="$2"

  info "Verifying SHA256 checksum..."
  local expected actual
  expected="$(awk '{print $1}' "$checksum_file")"

  if check_cmd sha256sum; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  elif check_cmd shasum; then
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  else
    err "No SHA256 tool found (need sha256sum or shasum)."
  fi

  if [[ "$expected" != "$actual" ]]; then
    err "Checksum mismatch! Expected ${expected}, got ${actual}."
  fi
  info "Checksum verified."
}

install_binary() {
  local binary="$1"
  local target="${INSTALL_DIR}/${BIN_NAME}${BIN_EXT}"

  mkdir -p "$INSTALL_DIR"

  if [[ -w "$INSTALL_DIR" ]]; then
    mv "$binary" "$target"
  else
    info "Need sudo to write to ${INSTALL_DIR}"
    sudo mv "$binary" "$target"
  fi

  info "Installed ${BIN_NAME} v${VERSION} to ${target}"
}

# Install the agent skill once into a canonical XDG data dir, then symlink
# each supported agent's global skill directory at it. Falls back to a copy
# on Windows, where symlinks require admin or Developer Mode.
install_skills() {
  local extracted_dir="$1"
  local src="${extracted_dir}/skills/${SKILL_NAME}"

  if [[ ! -d "$src" ]]; then
    warn "No skills bundle found in archive — skipping skill install."
    return
  fi

  local data_home="${XDG_DATA_HOME:-${HOME}/.local/share}"
  local canonical="${data_home}/save-to-spotify/skills/${SKILL_NAME}"

  mkdir -p "$(dirname "$canonical")"
  rm -rf "$canonical"
  cp -R "$src" "$canonical"
  info "Installed skill to ${canonical}"

  local targets=(
    "${HOME}/.claude|${HOME}/.claude/skills/${SKILL_NAME}"
    "${HOME}/.cursor|${HOME}/.cursor/skills/${SKILL_NAME}"
    "${HOME}/.config/opencode|${HOME}/.config/opencode/skills/${SKILL_NAME}"
    "${HOME}/.agents|${HOME}/.agents/skills/${SKILL_NAME}"
  )

  local entry root target
  for entry in "${targets[@]}"; do
    root="${entry%%|*}"
    target="${entry#*|}"

    if [[ ! -d "$root" ]]; then
      info "Skipping skill link for ${root} (directory not found)."
      continue
    fi

    mkdir -p "$(dirname "$target")"
    rm -rf "$target"
    if [[ "$os" == "windows" ]]; then
      cp -R "$canonical" "$target"
      info "Copied skill to ${target}"
    else
      ln -s "$canonical" "$target"
      info "Linked ${target} → ${canonical}"
    fi
  done
}

verify_in_path() {
  case ":${PATH}:" in
  *":${INSTALL_DIR}:"*)
    echo ""
    echo "\$ ${BIN_NAME} version"
    "${INSTALL_DIR}/${BIN_NAME}${BIN_EXT}" version
    return
    ;;
  esac

  echo ""
  warn "${INSTALL_DIR} is not in your PATH. Add it with:"
  if [[ "$os" == "windows" ]]; then
    warn "  Add ${INSTALL_DIR} to your system PATH via System Properties > Environment Variables"
  else
    warn "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    warn ""
    warn "To make it permanent, add that line to your shell config:"
    warn "  ~/.bashrc, ~/.zshrc, or ~/.profile"
  fi
}

# --- Main ---

main() {
  need_cmd curl
  need_cmd unzip
  need_cmd uname
  need_cmd mktemp
  need_cmd awk

  BIN_NAME="save-to-spotify"
  SKILL_NAME="save-to-spotify"
  BIN_EXT=""
  REPO="spotify/save-to-spotify"
  GITHUB_API="https://api.github.com"
  VERSION="${SAVE_TO_SPOTIFY_VERSION:-}"
  INSTALL_SKILLS="${SAVE_TO_SPOTIFY_INSTALL_SKILLS:-true}"
  DRY_RUN=false

  # --- Parse arguments ---
  while [[ $# -gt 0 ]]; do
    case "$1" in
    --version | -v)
      VERSION="$2"
      shift 2
      ;;
    --dir | -d)
      SAVE_TO_SPOTIFY_INSTALL_DIR="$2"
      shift 2
      ;;
    --dry-run | -n)
      DRY_RUN=true
      shift
      ;;
    --no-skills)
      INSTALL_SKILLS=false
      shift
      ;;
    --help | -h)
      echo "Usage: install.sh [--version VERSION] [--dir INSTALL_DIR] [--dry-run] [--no-skills]"
      echo ""
      echo "Options:"
      echo "  --version, -v   Version to install (default: latest)"
      echo "  --dir, -d       Installation directory"
      echo "  --dry-run, -n   Show what would be done without installing"
      echo "  --no-skills     Skip installing the agent skill files"
      echo ""
      echo "Environment variables:"
      echo "  SAVE_TO_SPOTIFY_INSTALL_DIR     Override install directory"
      echo "  SAVE_TO_SPOTIFY_VERSION         Override version to install"
      echo "  SAVE_TO_SPOTIFY_INSTALL_SKILLS  Set to 'false' to skip skill install"
      echo "  GITHUB_TOKEN / GH_TOKEN         Auth for private repo (optional)"
      exit 0
      ;;
    *) err "Unknown option: $1" ;;
    esac
  done

  # --- Detect OS and architecture ---
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
  linux) os="linux" ;;
  darwin) os="darwin" ;;
  mingw* | msys* | cygwin*) os="windows" ;;
  *) err "Unsupported OS: $os" ;;
  esac

  case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *) err "Unsupported architecture: $arch" ;;
  esac

  if [[ "$os" == "windows" ]]; then
    BIN_EXT=".exe"
  fi

  info "Detected platform: ${os}/${arch}"

  # --- Resolve release metadata ---
  local release_url release_json
  if [[ -n "$VERSION" ]]; then
    release_url="${GITHUB_API}/repos/${REPO}/releases/tags/v${VERSION#v}"
  else
    release_url="${GITHUB_API}/repos/${REPO}/releases/latest"
  fi

  info "Fetching release metadata..."
  release_json="$(gh_curl -H 'Accept: application/vnd.github+json' "$release_url" 2>/dev/null)" || true
  if [[ -z "$release_json" ]]; then
    err "Could not fetch release metadata from ${release_url}.
For a private repo, set GITHUB_TOKEN or GH_TOKEN."
  fi

  VERSION="$(printf '%s\n' "$release_json" | json_string_field "tag_name")"
  VERSION="${VERSION#v}"
  if [[ -z "$VERSION" ]]; then
    err "Could not determine release version from ${release_url}."
  fi

  # --- Locate the right asset ---
  local asset_name sha_asset_name asset_id sha_asset_id
  asset_name="${BIN_NAME}-${os}-${arch}-v${VERSION}.zip"
  sha_asset_name="${asset_name}.sha256"
  asset_id="$(printf '%s\n' "$release_json" | asset_id_for_name "$asset_name")"
  sha_asset_id="$(printf '%s\n' "$release_json" | asset_id_for_name "$sha_asset_name")"

  if [[ -z "$asset_id" ]]; then
    err "Asset not found in release v${VERSION}: ${asset_name}"
  fi

  # --- Determine install directory ---
  determine_install_dir
  info "Install directory: ${INSTALL_DIR}"

  # --- Download ---
  # work_dir is intentionally not `local` so the EXIT trap can clean it up
  # after main() returns.
  work_dir="$(mktemp -d)"
  trap 'rm -rf "$work_dir"' EXIT

  info "Downloading ${asset_name}..."
  gh_curl -H 'Accept: application/octet-stream' \
    -o "${work_dir}/${asset_name}" \
    "${GITHUB_API}/repos/${REPO}/releases/assets/${asset_id}" ||
    err "Download failed: ${asset_name}"

  if [[ -n "$sha_asset_id" ]]; then
    info "Downloading checksum..."
    gh_curl -H 'Accept: application/octet-stream' \
      -o "${work_dir}/${sha_asset_name}" \
      "${GITHUB_API}/repos/${REPO}/releases/assets/${sha_asset_id}" ||
      err "Download failed: ${sha_asset_name}"

    # Sidecar files are typically `<hash>  <filename>`. The verify step expects
    # the file path on disk to match the filename in the sidecar, so move into
    # work_dir and run verification there.
    (cd "$work_dir" && verify_checksum "${asset_name}" "${sha_asset_name}")
  else
    warn "No checksum sidecar found for ${asset_name} — skipping integrity check."
  fi

  # --- Extract ---
  info "Extracting archive..."
  unzip -qo "${work_dir}/${asset_name}" -d "$work_dir"

  local binary
  binary="${work_dir}/${BIN_NAME}${BIN_EXT}"
  if [[ ! -f "$binary" ]]; then
    err "Could not find binary in archive: ${binary}"
  fi

  if [[ "$os" != "windows" ]]; then
    chmod +x "$binary"
  fi

  if [[ "$DRY_RUN" == true ]]; then
    info "Dry run — would install to: ${INSTALL_DIR}/${BIN_NAME}${BIN_EXT}"
    exit 0
  fi

  # --- Install ---
  install_binary "$binary"
  if [[ "$INSTALL_SKILLS" == "true" ]]; then
    install_skills "$work_dir"
  fi
  verify_in_path
}

if [[ "${SAVE_TO_SPOTIFY_INSTALL_SH_SOURCE_ONLY:-false}" != "true" ]]; then
  main "$@"
fi
