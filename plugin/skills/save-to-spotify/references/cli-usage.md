# Save to Spotify

## CLI Usage

Reference for the `save-to-spotify` binary: installation, authentication, commands, flags, JSON mode, error handling, and common agent workflows.

## Installation

### One-line install (recommended)

```shell
curl -fsSL https://saveto.spotify.com/install.sh | bash
```

Detects OS and architecture, downloads the binary from GitHub Releases, verifies the SHA256 checksum, and installs to `/usr/local/bin` (or `~/.local/bin` if not writable).

Pin a version or change the install directory:

```shell
# Specific version
curl -fsSL https://saveto.spotify.com/install.sh | bash -s -- --version 0.1.1

# Custom directory
curl -fsSL https://saveto.spotify.com/install.sh | bash -s -- --dir ~/.local/bin

# Via environment variables
SAVE_TO_SPOTIFY_VERSION=0.1.1 SAVE_TO_SPOTIFY_INSTALL_DIR=~/.local/bin \
  curl -fsSL https://saveto.spotify.com/install.sh | bash
```

### Download a binary manually

Grab the latest release for the platform from [releases](https://github.com/spotify/save-to-spotify/releases):

```shell
# macOS Apple Silicon
gh release download --repo spotify/save-to-spotify --pattern "save-to-spotify-darwin-arm64"
chmod +x save-to-spotify-darwin-arm64
sudo mv save-to-spotify-darwin-arm64 /usr/local/bin/save-to-spotify

# macOS Intel
gh release download --repo spotify/save-to-spotify --pattern "save-to-spotify-darwin-amd64"
chmod +x save-to-spotify-darwin-amd64
sudo mv save-to-spotify-darwin-amd64 /usr/local/bin/save-to-spotify

# Linux x86_64
gh release download --repo spotify/save-to-spotify --pattern "save-to-spotify-linux-amd64"
chmod +x save-to-spotify-linux-amd64
sudo mv save-to-spotify-linux-amd64 /usr/local/bin/save-to-spotify
```

### Build from source

Requires Go 1.21+.

```shell
git clone https://github.com/spotify/save-to-spotify.git && cd save-to-spotify
go build -ldflags "-X github.com/spotify/save-to-spotify/cmd.commit=$(git rev-parse --short HEAD)" \
  -o save-to-spotify .
sudo mv save-to-spotify /usr/local/bin/
```

Verify installation:

```shell
save-to-spotify version
```

## Authentication

The user must authenticate once before any save. The CLI uses OAuth 2.0 with PKCE -- no client secret needed.

### Interactive (user has a browser)

```shell
save-to-spotify auth login
```

This opens the browser, the user approves, and a token is saved to `~/.config/save-to-spotify/token.json`.

### Headless (remote server, CI, or agent environment)

```shell
save-to-spotify auth login --no-browser
```

This prints an authorization URL. The user visits it in any browser, approves, and pastes the redirect URL back into the terminal. The redirect URL will look like `http://127.0.0.1:8085/callback?code=...&state=...` -- it's fine if the page shows a connection error, the URL itself is what matters.

### Environment token (skip OAuth entirely)

If the user already has a Spotify access token (e.g. from another tool or CI secret):

```shell
export SAVE_TO_SPOTIFY_AUTH_TOKEN="BQD..."
```

When this env var is set, the CLI uses it directly with no file I/O and no token refresh. The token must be kept fresh externally.

### Check auth status

```shell
save-to-spotify --json auth status
```

Returns `{"authenticated": true, "token_valid": true, ...}` or `{"authenticated": false}`.

Token refresh is automatic -- if the saved token is expired, the CLI refreshes it silently on the next command. No action needed unless the refresh token itself is revoked, in which case the user must `auth login` again.

### Print the access token

```shell
save-to-spotify token
```

Prints the current access token to stdout -- directly usable as a **Spotify Web API bearer** for requests against `api.spotify.com`. Useful for catalog lookups (searching album/track URIs, fetching release metadata) from inside recipes. Exits non-zero and prints a diagnostic to stderr when the stored token cannot be refreshed. Always check the exit code before piping into an `Authorization` header -- otherwise an empty token produces a misleading HTTP 400 from Spotify rather than a clean auth error.

See [spotify-api.md](spotify-api.md) for the official `developer.spotify.com` references, OpenAPI spec URL, endpoint patterns, and URI-resolution helpers.

## Saving media

### Quick save (recommended for most cases)

The `upload` command is the simplest path -- one command to create an episode and save the file:

```shell
save-to-spotify --json upload recording.mp3 \
  --title "My Recording" \
  --summary "Description here" \
  --image cover.jpg
```

Output:
```json
{"episode_uri": "spotify:episode:abc123", "title": "My Recording", "status": "PROCESSING"}
```

If the user has no shows yet, one is auto-created as "My Podcast".

### Save to a specific show

```shell
save-to-spotify --json upload lecture.m4a \
  --title "Lecture 3: Distributed Systems" \
  --summary "CS 307 Spring 2024" \
  --show-id spotify:show:xyz789 \
  --image cover.jpg
```

When `--show-id` is omitted, the CLI uses the most recently created show.

### Create a new show and save in one step

```shell
save-to-spotify --json upload keynote.mp3 \
  --title "2024 Keynote" \
  --summary "Opening talk by <speaker>" \
  --new-show "Conference Talks" \
  --image cover.jpg
```

`--new-show` and `--show-id` are mutually exclusive.

### Granular episode creation

For more control, use `episodes create` instead of `upload`:

```shell
save-to-spotify --json episodes create \
  --title "Episode Title" \
  --file audio.mp3 \
  --summary "Episode description" \
  --show-id spotify:show:xyz789 \
  --image episode-cover.jpg \
  --language en
```

The difference: `episodes create` requires `--summary` and `--file` as explicit flags (not positional), and does not support `--new-show`.

## Supported file formats

| Extension | Type | MIME |
|-----------|------|------|
| `.mp3` | Audio | `audio/mpeg` |
| `.m4a` | Audio | `audio/mp4` |
| `.wav` | Audio | `audio/wav` |
| `.ogg` | Audio | `audio/ogg` |

**Maximum file size: 1 GB.**

## Managing shows

Shows are folders that group episodes. Every episode belongs to exactly one show.

```shell
# List all shows
save-to-spotify --json shows

# Create a show (always include --image)
save-to-spotify --json shows create --title "My Lectures" --summary "University recordings" --image cover.jpg

# Get show details
save-to-spotify --json shows get <show_id>

# Delete a show (and all its episodes)
save-to-spotify --json shows delete <show_id>
```

`save-to-spotify --json shows` should be the first show-management command you run. Check what already exists before creating a new show.

### Playback control

`shows create` accepts `--playback-control <mode>` (currently `chapter-skip`) to set a show's skip-forward behavior at creation time. The response includes `playback_control` only when the backend applied the setting — if the field is absent, the value was dropped and the show has default controls. For behavior, constraints, and when to enable it, defer to the `configure-chapter-skip` skill.

## Managing episodes

```shell
# List episodes in a show
save-to-spotify --json episodes --show-id <show_id>

# Check episode readiness
save-to-spotify --json episodes status <episode_id>

# Delete an episode
save-to-spotify --json episodes delete <episode_id>
```

Show and episode metadata is immutable after creation. To change a title or description, delete and re-create.

### Episode readiness

After saving, poll for readiness before sharing:

```shell
save-to-spotify --json episodes status <EPISODE_ID>
```

Output:
```json
{"episode_uri": "spotify:episode:abc123", "readiness": "READY"}
```

Readiness values:
- `READY` -- playable on Spotify
- `PROCESSING` -- still being processed (wait and retry)
- `FAILED` -- processing failed; check the episode metadata and re-save if needed

Most episodes are ready within 1-2 minutes. For large files, allow up to 5 minutes.

## IDs and URIs

The CLI accepts both bare IDs and full Spotify URIs interchangeably, but agents should prefer the full Spotify URI form whenever possible:
- `abc123def456` (bare ID)
- `spotify:show:abc123def456` (full URI)

JSON output always includes the full URI form.

## JSON mode

**Always use `--json` when operating as an agent.** It must appear before the command:

```shell
save-to-spotify --json <command> [flags]
```

In JSON mode:
- All output is valid JSON on stdout
- Errors are `{"error": "message"}` on stdout with exit code 1
- Progress bars and activity indicators are suppressed
- Informational messages to stderr are suppressed

Without `--json`, output is human-readable text.

## Timeout control

The default API timeout is 30 seconds. For large file uploads, the upload itself has no timeout (separate HTTP client), but the episode creation API call does. Override if needed:

```shell
save-to-spotify --json --timeout 2m upload large-recording.mp3 --title "Long Episode" --image cover.jpg
```

Or via environment variable:
```shell
export SAVE_TO_SPOTIFY_TIMEOUT=2m
```

## Common agent workflows

### Create a rich-timeline episode end-to-end

```shell
# 1. Generate cover image (MANDATORY — see cover-image.md)
python3 generate_cover.py

# 2. Save with cover image
RESULT=$(save-to-spotify --json upload episode.mp3 \
  --title "Daily Digest - April 15, 2026" \
  --summary "$(cat description.txt)" \
  --image cover.jpg)
EP_URI=$(echo "$RESULT" | jq -r .episode_uri)
EP_ID=${EP_URI#spotify:episode:}

# 3. Set timeline (chapters + image/link/Spotify companions in one call)
save-to-spotify --json timeline set --episode-id "$EP_ID" --from-file timeline.json

# 4. Poll until ready
while true; do
  STATUS=$(save-to-spotify --json episodes status "$EP_ID" | jq -r .readiness)
  [ "$STATUS" = "READY" ] && break
  [ "$STATUS" = "FAILED" ] && echo "Processing failed" && exit 1
  sleep 15
done

echo "Episode ready: $EP_URI"
```

### Save a single file

```shell
URI=$(save-to-spotify --json upload recording.mp3 --title "My Recording" --summary "A recording" --image cover.jpg | jq -r .episode_uri)
EP_ID=${URI#spotify:episode:}

while true; do
  STATUS=$(save-to-spotify --json episodes status "$EP_ID" | jq -r .readiness)
  [ "$STATUS" = "READY" ] && break
  [ "$STATUS" = "FAILED" ] && echo "Processing failed" && exit 1
  sleep 15
done
```

### Batch save multiple files to one show

```shell
SHOW_URI=$(save-to-spotify --json shows create --title "Conference Talks 2024" --image show-cover.jpg | jq -r .show_uri)

for f in talks/*.mp3; do
  TITLE=$(basename "$f" .mp3 | tr '-' ' ')
  save-to-spotify --json upload "$f" --title "$TITLE" --summary "Conference talk" --show-id "$SHOW_URI" --image cover.jpg
done
```

## Error handling

With `--json`, every command returns exit code 0 on success and exit code 1 on error. Errors are returned as `{"error": "message"}`. **Always check for errors after each command.**

```bash
RESULT=$(save-to-spotify --json upload episode.mp3 --title "My Episode" --image cover.jpg)
if echo "$RESULT" | jq -e .error > /dev/null 2>&1; then
  echo "Failed: $(echo "$RESULT" | jq -r .error)"
  exit 1
fi
EP_URI=$(echo "$RESULT" | jq -r .episode_uri)
EP_ID=${EP_URI#spotify:episode:}

while true; do
  STATUS=$(save-to-spotify --json episodes status "$EP_ID")
  READINESS=$(echo "$STATUS" | jq -r .readiness)
  [ "$READINESS" = "READY" ] && break
  [ "$READINESS" = "FAILED" ] && echo "Processing failed" && exit 1
  sleep 15
done
```

Common errors:

| Error | Cause | Action |
|-------|-------|--------|
| `not authenticated` | No token file | Run `auth login` |
| `token refresh failed` | Refresh token revoked | Run `auth login` again |
| `unsupported file extension` | Wrong file type | Convert to a supported format |
| `file too large` | Over 1 GB | Compress or split the file |
| `image too large` | Over 1 MB | Resize the image |
| `API error (429)` | Rate limited | Wait and retry after a delay |
| `API error (401)` | Token expired mid-request | Retry (auto-refresh will kick in) |
| `API error (403)` | Insufficient permissions | Re-authenticate with `auth login` |
| `--new-show and --show-id are mutually exclusive` | Conflicting flags | Use one or the other |

## Troubleshooting

### Episode not appearing after saving
Use `episodes status <id>` to check readiness. Processing can take a few minutes after the save completes.

### Image upload fails
Images must be `.jpg`, `.jpeg`, or `.png`, max 1 MB, with valid magic bytes (actual JPEG/PNG content, not just a renamed file).

### "unsupported file extension" error
Only `.mp3`, `.m4a`, `.wav`, and `.ogg` are supported. Convert other formats first (e.g., `ffmpeg -i input.webm output.mp3`).

## Environment variables reference

| Variable | Purpose |
|----------|---------|
| `SAVE_TO_SPOTIFY_AUTH_TOKEN` | Bearer token; skips OAuth entirely (no refresh, no expiry tracking) |
| `SAVE_TO_SPOTIFY_BACKEND_URL` | Override backend URL |
| `SAVE_TO_SPOTIFY_TIMEOUT` | API timeout duration (e.g. `30s`, `2m`) |

## Content policy

- **No copyrighted music:** Content that is classified as music by Spotify's moderation system will be taken down.
- **Moderation applies:** Standard Spotify podcast moderation policies apply. Content that violates policies will be removed and the user will be notified via email.
- **No sensitive data:** Do not save content containing passwords, credentials, PII of others, or confidential business information. The content is stored on Spotify's servers.
- **Save limits apply:** There are per-user rate limits on saves (per hour and per week). If a limit is hit, the API returns an error -- wait and retry later.
- **Never fabricate URLs:** Every source link in episode descriptions must come from actual content sourcing. If a URL isn't found, omit the link -- never invent one.
