# upload-to-spotify

A command-line tool for uploading audio and video content to Spotify for personal consumption. Built for agents and automation — generate a daily briefing, language lesson, or meeting recap, then push it to Spotify where it's available alongside your other listening.


## Install

### One-line install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/spotify/upload-to-spotify/master/install.sh | bash
```

Detects your OS and architecture, downloads the binary from GitHub Releases, verifies the SHA256 checksum, and installs to `/usr/local/bin` (or `~/.local/bin` if not writable).

Options:

```bash
# Install a specific version
curl -fsSL https://raw.githubusercontent.com/spotify/upload-to-spotify/master/install.sh | bash -s -- --version 0.1.0-alpha.22

# Install to a custom directory
curl -fsSL https://raw.githubusercontent.com/spotify/upload-to-spotify/master/install.sh | bash -s -- --dir ~/.local/bin
```

### Download a binary manually

Grab the latest release for your platform from the [releases page](https://github.com/spotify/upload-to-spotify/releases).

```bash
# macOS Apple Silicon
gh release download --repo spotify/upload-to-spotify --pattern "upload-to-spotify-darwin-arm64"
chmod +x upload-to-spotify-darwin-arm64
sudo mv upload-to-spotify-darwin-arm64 /usr/local/bin/upload-to-spotify

# macOS Intel
gh release download --repo spotify/upload-to-spotify --pattern "upload-to-spotify-darwin-amd64"
chmod +x upload-to-spotify-darwin-amd64
sudo mv upload-to-spotify-darwin-amd64 /usr/local/bin/upload-to-spotify

# Linux x86_64
gh release download --repo spotify/upload-to-spotify --pattern "upload-to-spotify-linux-amd64"
chmod +x upload-to-spotify-linux-amd64
sudo mv upload-to-spotify-linux-amd64 /usr/local/bin/upload-to-spotify
```

### Build from source

Requires [Go](https://go.dev/dl/) 1.21+.

```bash
go build -o upload-to-spotify .
sudo mv upload-to-spotify /usr/local/bin/
```

## Quick start

```bash
# 1. Log in
upload-to-spotify auth login

# 2. Upload a file (auto-creates a show if you don't have one yet)
upload-to-spotify upload ./recap.mp3 --title "Monday Standup Recap"

# 3. Check what you've uploaded
upload-to-spotify shows
```

## Authentication

Log in once — the CLI stores your token and refreshes it automatically.

```bash
# Opens your browser to authorize (default)
upload-to-spotify auth login

# Headless mode — prints a URL to open on any device, then you paste back the redirect URL
# Useful for SSH sessions and remote servers
upload-to-spotify auth login --no-browser

# Check if you're logged in
upload-to-spotify auth status

# Log out (removes stored credentials)
upload-to-spotify auth logout
```

Tokens are stored in `~/.config/upload-to-spotify/token.json` (using `$XDG_CONFIG_HOME` if set).

## Commands

### upload

The primary command. Uploads a media file and creates an episode in one step. This is the main entrypoint for agents — a single command handles show creation, file upload, and episode metadata.

```bash
upload-to-spotify upload <media-file> --title <episode-title> [flags]
```

| Flag | Description |
|---|---|
| `--title` | Episode title (required) |
| `--show-id` | Target show ID or URI |
| `--new-show` | Create a new show with this title and upload to it |
| `--summary` | Episode description |
| `--image` | Cover image file (`.jpg`/`.png`, max 1 MB) |
| `--language` | Language code (default: `en`) |

**Show resolution** — if you don't specify `--show-id` or `--new-show`, the CLI uses your most recently created show. If you have no shows yet, it creates one called "My Personal Podcast".

```bash
# Upload a file — uses your most recent show (or creates one automatically)
upload-to-spotify upload ./recap.mp3 --title "Monday Standup Recap"

# Upload to a specific show
upload-to-spotify upload ./lesson.mp3 --title "Spanish Lesson 12" --show-id spotify:show:abc123

# Organize uploads into separate shows
upload-to-spotify upload ./lecture.mp3 --title "CS 101 — Lecture Notes Week 6" --new-show "Lecture Notes"

# With cover art and description
upload-to-spotify upload ./briefing.mp3 \
  --title "Morning Briefing" \
  --summary "Headlines, weather, and calendar for today" \
  --image ./cover.jpg
```

**Supported formats:** `.mp3`, `.m4a`, `.mp4`, `.mov`, `.wav`, `.ogg`.

### shows

Shows are containers for your uploaded content. You can use a single show for everything or create separate shows to organize by topic (e.g. "Daily Briefings", "Language Practice", "Meeting Recaps").

```bash
# List your shows
upload-to-spotify shows

# Create a show
upload-to-spotify shows create --title "Lecture Notes" --summary "Audio summaries of CS 101 lectures"

# Get show details
upload-to-spotify shows get <show-id>

# Delete a show and all its episodes
upload-to-spotify shows delete <show-id>
```

`shows create` flags:

| Flag | Description |
|---|---|
| `--title` | Show title (required) |
| `--summary` | Show description (default: "(no description)") |
| `--image` | Cover image file (`.jpg`/`.png`, max 1 MB) |
| `--language` | Language code (default: `en`) |

### episodes

Each uploaded file becomes an episode within a show.

```bash
# List episodes (for your most recent show)
upload-to-spotify episodes

# List episodes for a specific show
upload-to-spotify episodes --show-id spotify:show:abc123

# Create an episode (more control than `upload` — same underlying operation)
upload-to-spotify episodes create --title "Sprint Review Notes" --file ./review.mp3

# Check if an episode is ready for playback
upload-to-spotify episodes status <episode-id>

# Delete an episode
upload-to-spotify episodes delete <episode-id>
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

### chapters

Add navigation markers to uploaded content. Useful for longer files with distinct sections (e.g. a lecture with intro, key concepts, and summary). Chapters require a JSON file with at least 2 entries, the first starting at `0` ms.

```bash
# Get chapters for an episode
upload-to-spotify chapters get <episode-id>

# Set (replace all) chapters from a JSON file
upload-to-spotify chapters set --episode-id <episode-id> --from-file chapters.json

# Delete all chapters
upload-to-spotify chapters delete <episode-id>
```

Example `chapters.json`:

```json
{
  "items": [
    { "title": "Introduction", "start_time_ms": 0 },
    { "title": "Key Concepts", "start_time_ms": 45000 },
    { "title": "Summary", "start_time_ms": 90000 }
  ]
}
```

### list

Alternative way to browse your content.

```bash
# List episodes for the most recent show
upload-to-spotify list

# List episodes for a specific show
upload-to-spotify list episodes --show-id spotify:show:abc123

# List all shows
upload-to-spotify list shows
```

### Other commands

| Command | Description |
|---|---|
| `me` | Show authenticated user's Spotify profile |
| `token` | Print access token to stdout (for piping to `curl`, etc.) |
| `version` | Print version and commit hash |

## Global flags

| Flag | Description |
|---|---|
| `--json` | Output results as JSON (for scripting and automation) |
| `--timeout <timeout>` | API request timeout (default: `30s`, e.g. `1m`, `90s`) |

## Agent integration

The CLI uploads files — it does **not** generate audio or video. Use TTS tools (edge-tts, say, ElevenLabs) to create media first, then upload with this CLI.

**`--json` mode** — always use `--json` when calling from an agent. Every command supports it.

```bash
upload-to-spotify --json upload ./recap.mp3 --title "Standup Recap" | jq -r .episode_uri
upload-to-spotify --json shows | jq '.shows[].title'
```

**Typical workflow:**

```bash
upload-to-spotify --json auth status
upload-to-spotify --json shows create --title "My Show"
upload-to-spotify --json upload episode.mp3 --title "Ep 1" --show-id <show-id>
upload-to-spotify --json episodes status <episode-id>    # poll until READY
upload-to-spotify --json chapters set --episode-id <episode-id> --from-file chapters.json
```

**Error handling** — exit code 0 = success, 1 = error. With `--json`, errors return `{"error": "message"}`. Always check `episodes status` before setting chapters — episodes need to be `READY` first (poll every 10-15s, most ready within 1-2 minutes).

**Headless auth** — use `--no-browser` for remote/CI environments. The CLI prints a URL that the **user** must open in their browser — agents cannot complete this step alone. After initial auth, the token auto-refreshes indefinitely. For fully non-interactive setups, set `UPLOAD_TO_SPOTIFY_AUTH_TOKEN`.

## Agent skills

The [`skills/`](skills/) directory contains detailed instructions for AI agents:

- **[`upload-to-spotify/SKILL.md`](skills/upload-to-spotify/SKILL.md)** — Core CLI reference (commands, flags, workflows, chapters format)
- **[`upload-to-spotify/audio-providers.md`](skills/upload-to-spotify/audio-providers.md)** — TTS engines, audio assembly with ffmpeg
- **[`upload-to-spotify/video-providers.md`](skills/upload-to-spotify/video-providers.md)** — Video creation, slideshows, cover art
- **[`upload-to-spotify/content-quality.md`](skills/upload-to-spotify/content-quality.md)** — Editorial guidelines for scripts

Example content recipes in [`skills/examples/`](skills/examples/):

| Skill | What it creates |
|---|---|
| [web-digest](skills/examples/web-digest/) | Audio digest from websites and feeds |
| [deep-dive](skills/examples/deep-dive/) | Narrated explainer on any topic |
| [sous-chef](skills/examples/sous-chef/) | Hands-free cooking audio guide |
| [spotlight](skills/examples/spotlight/) | Interview prep and presentation warm-up |
| [campfire](skills/examples/campfire/) | Bedtime stories |
| [rosetta](skills/examples/rosetta/) | Language lessons |
| [daybreak](skills/examples/daybreak/) | Morning briefing |
| [recall](skills/examples/recall/) | Flashcard study sessions |

## Notes

- Show and episode metadata is immutable after creation. To change a title, description, or cover image, delete and recreate.
- Supported formats: `.mp3`, `.m4a`, `.mp4`, `.mov`, `.wav`, `.ogg` (max 1 GB).
