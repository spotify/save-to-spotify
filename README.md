# Save to Spotify

A command-line tool for saving audio content to Spotify. Built for agents and automation, generate a daily briefing, language lesson, or meeting recap, then push it to Spotify where it's available alongside your other listening.

## Quick Start

Prompt your agent to install:

```text
> Install Save to Spotify by running https://saveto.spotify.com/install.sh
```

Installs the CLI and exposes the skill to Claude Code, Cursor, Codex, and any agent that reads `.agents/skills/`.

Once installed, invoke with `/save-to-spotify` (Claude Code) or `$save-to-spotify` (Codex), or just describe what you want in plain English.

## Install

### Curl-bash

Same script the Quick Start prompt runs:

```bash
curl -fsSL https://saveto.spotify.com/install.sh | bash
```

Common options:

```bash
# Pin a specific version
curl -fsSL https://saveto.spotify.com/install.sh | bash -s -- --version 0.1.1

# Custom install directory
curl -fsSL https://saveto.spotify.com/install.sh | bash -s -- --dir ~/.local/bin

# Binary only, skip the agent skill
curl -fsSL https://saveto.spotify.com/install.sh | bash -s -- --no-skills
```

### Skills.sh

Install the agent skill to Claude Code, Cursor, Codex, and 50+ other agents:

```bash
npx skills add spotify/save-to-spotify
```

### ClawHub / OpenClaw

Save to Spotify is listed on ClawHub as [`@spotify/save-to-spotify`](https://clawhub.ai/spotify/save-to-spotify). Install it into your active OpenClaw workspace:

```bash
openclaw skills install @spotify/save-to-spotify
```

The ClawHub listing installs the agent skill. The skill uses the `save-to-spotify` CLI, so if the binary is not already on your `PATH`, run the curl-bash installer above.

### Claude Code plugin marketplace

```
/plugin marketplace add spotify/save-to-spotify
/plugin install save-to-spotify@save-to-spotify
```

### Manual

If you can't run the install script:

1. Download `save-to-spotify-{os}-{arch}-v{version}.zip` and its matching `.sha256` from the [releases page](https://github.com/spotify/save-to-spotify/releases).
2. Verify integrity:
   ```bash
   shasum -c save-to-spotify-darwin-arm64-v0.1.1.zip.sha256
   ```
3. Unzip, you get the binary plus a `skills/save-to-spotify/` tree.
4. Move the binary to a directory on your `PATH` and `chmod +x` it.
5. (Optional) Copy `skills/save-to-spotify/` into your agent's skill directory, e.g. `~/.claude/skills/save-to-spotify/` or `~/.cursor/skills/save-to-spotify/`.

### Build from source

Requires [Go](https://go.dev/dl/) 1.21+.

```bash
go build -ldflags "-X github.com/spotify/save-to-spotify/cmd.commit=$(git rev-parse --short HEAD)" \
  -o save-to-spotify .
sudo mv save-to-spotify /usr/local/bin/
```

## Authentication

Log in once, the CLI stores your token and refreshes it automatically.

```bash
# Opens your browser to authorize (default)
save-to-spotify auth login

# Headless mode, prints a URL to open on any device, then you paste back the redirect URL
# Useful for SSH sessions and remote servers
save-to-spotify auth login --no-browser

# Check if you're logged in
save-to-spotify auth status

# Log out (removes stored credentials)
save-to-spotify auth logout
```

Tokens are stored in `~/.config/save-to-spotify/token.json` (using `$XDG_CONFIG_HOME` if set). If you use DPoP-bound tokens (the default), the sibling `dpop_key.json` is also required. For CI and headless environments, see the [CI / Automation guide](docs/ci-automation.md).

## Commands

### upload

The primary command. Uploads a media file and creates an episode in one step. This is the main entrypoint for agents, a single command handles show creation, file upload, and episode metadata.

```bash
save-to-spotify upload <file> --title <title> [flags]
```

| Flag | Description |
|---|---|
| `--title` | Episode title (required) |
| `--show-id` | Target show ID or URI |
| `--new-show` | Create a new show with this title and save the file to it |
| `--summary` | Episode description |
| `--image` | Cover image file (`.jpg`/`.png`, max 1 MB) |
| `--language` | Language code (default: `en`) |

If `--show-id` or `--new-show` is not specified, the CLI will use your most recently created show, or create a new one if none exists.

```bash
# Save an audio file to Spotify, uses your most recent show (or creates one automatically)
save-to-spotify upload ./recap.mp3 --title "Monday Standup Recap"

# Save to a specific show
save-to-spotify upload ./lesson.mp3 --title "Spanish Lesson 12" --show-id spotify:show:abc123

# Organize saved items into separate shows
save-to-spotify upload ./lecture.mp3 --title "CS 101 — Lecture Notes Week 6" --new-show "Lecture Notes"

# With cover image and description
save-to-spotify upload ./briefing.mp3 \
  --title "Morning Briefing" \
  --summary "Weather and calendar for today" \
  --image ./cover.jpg
```

**Supported formats:** `.mp3`, `.m4a`, `.wav`, `.ogg`.

### shows

Shows are containers for your saved content. You can use a single show for everything or create separate shows to organize by topic (e.g. "Daily Briefings", "Language Practice", "Meeting Recaps").

```bash
# List your shows
save-to-spotify shows

# Create a show
save-to-spotify shows create --title "Lecture Notes" --summary "Audio summaries of CS 101 lectures"

# Get show details
save-to-spotify shows get <id>

# Delete a show and all its episodes
save-to-spotify shows delete <id>
```

`shows create` flags:

| Flag | Description |
|---|---|
| `--title` | Show title (required) |
| `--summary` | Show description (default: "(no description)") |
| `--image` | Cover image file (`.jpg`/`.png`, max 1 MB) |
| `--language` | Language code (default: `en`) |

### episodes

Each saved file becomes an episode within a show.

```bash
# List episodes (for your most recent show)
save-to-spotify episodes

# List episodes for a specific show
save-to-spotify episodes --show-id spotify:show:abc123

# Create an episode (more control than `upload`, same underlying operation)
save-to-spotify episodes create --title "Sprint Review Notes" --file ./review.mp3

# Check if an episode is ready for playback
save-to-spotify episodes status <episode-id>

# Wait until an episode is ready (default timeout: 5m)
save-to-spotify episodes status <episode-id> --wait

# Wait with a custom readiness timeout
save-to-spotify episodes status <episode-id> --wait 2m

# Delete an episode
save-to-spotify episodes delete <episode-id>
```

`episodes create` flags:

| Flag | Description |
|---|---|
| `--title` | Episode title (required) |
| `--file` | Media file path (required) |
| `--show-id` | Target show ID or URI (default: most recent show) |
| `--summary` | Episode description |
| `--image` | Cover image file (`.jpg`/`.png`, max 1 MB) |
| `--language` | Language code |

### timeline

Add chapter markers and in-player companion content to saved episodes. A timeline can include chapters, images, external links, and Spotify-native references via `spotify_entity`. Prefer `spotify_entity` whenever the thing you want listeners to open already exists on Spotify, prefer full `spotify:...` URIs over bare IDs or `open.spotify.com` URLs, and note that a `link` and a `spotify_entity` can both appear in the same timeline when both destinations are useful.

```bash
# Get timeline items for an episode
save-to-spotify timeline get <episode-id>

# Set (replace all) timeline items from a JSON file
save-to-spotify timeline set --episode-id <episode-id> --from-file timeline.json

# Delete all timeline items
save-to-spotify timeline delete <episode-id>
```

Example `timeline.json`:

```json
{
  "items": [
    { "chapter": { "title": "Introduction", "start_time_ms": 0 } },
    { "chapter": { "title": "Key Concepts", "start_time_ms": 45000 } },
    { "spotify_entity": { "start_time_ms": 60000, "duration_ms": 30000, "uri": "spotify:track:abc123" } },
    { "link": { "start_time_ms": 95000, "duration_ms": 20000, "url": "https://example.com/slides" } },
    { "chapter": { "title": "Summary", "start_time_ms": 120000 } }
  ]
}
```

Timeline rules:

- `chapter`: optional, but if present there must be at least 2; the first must start at `0`.
- `image`: requires a positive `duration_ms` and a local `.jpg`/`.png` file path or an `image_token`.
- `link`: requires a positive `duration_ms` and an HTTP(S) URL.
- `spotify_entity`: requires a full `spotify:...` URI; `duration_ms` is optional but must be positive when set.
- Companion items (`image`, `link`, `spotify_entity`) must not overlap in time, but a `link` and a `spotify_entity` may share the same segment/chapter as long as their windows do not overlap.

### list

Alternative command for listing your content.

```bash
# List episodes for the most recent show
save-to-spotify list

# List episodes for a specific show
save-to-spotify list episodes --show-id spotify:show:abc123

# List all shows
save-to-spotify list shows
```

### Other commands

| Command | Description |
|---|---|
| `update` | Check for and install CLI updates |
| `token` | Print access token to stdout (for piping to `curl`, etc.) |
| `version` | Print version and commit hash |

## Global flags

| Flag | Description |
|---|---|
| `--json` | Output results as JSON (for scripting and automation) |
| `--timeout <duration>` | API request timeout (default: `30s`, e.g. `1m`, `90s`) |

## Documentation

- [Agent integration](docs/agent-integration.md) — JSON mode, typical workflow, error handling
- [CI / Automation](docs/ci-automation.md) — GitHub Actions setup, headless auth, credential persistence

## Environment variables

| Variable | Purpose | Default |
|---|---|---|
| `SAVE_TO_SPOTIFY_AUTH_TOKEN` | Bearer token override (expires in ~1 hour, no auto-refresh — see [CI guide](docs/ci-automation.md)) | — |
| `SAVE_TO_SPOTIFY_BACKEND_URL` | Override the backend API URL | `https://saveto.spotify.com` |
| `SAVE_TO_SPOTIFY_TIMEOUT` | API request timeout (e.g. `30s`, `2m`) | `30s` |
| `SAVE_TO_SPOTIFY_CLIENT_ID` | OAuth client ID override | built-in |
| `SAVE_TO_SPOTIFY_NO_UPDATE_CHECK` | Disable passive update checks | off |
