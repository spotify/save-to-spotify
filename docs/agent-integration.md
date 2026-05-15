# Agent Integration

The CLI saves files to Spotify, it does **not** generate audio. Use TTS tools (edge-tts, say, ElevenLabs) to create media first, then upload with this CLI.

## JSON mode

Use **`--json`** when calling from an agent. Every command supports it.

```bash
save-to-spotify --json upload ./recap.mp3 --title "Standup Recap" | jq -r .episode_uri
save-to-spotify --json shows | jq '.shows[].title'
```

## Typical workflow

```bash
save-to-spotify --json auth status
save-to-spotify --json shows  # list first, then decide with the user whether to reuse or create
save-to-spotify --json shows create --title "My Show"
save-to-spotify --json upload episode.mp3 --title "Ep 1" --show-id <id>
save-to-spotify --json episodes status <episode_id>    # poll until READY
save-to-spotify --json timeline set --episode-id <id> --from-file timeline.json
```

## Episode status and error handling

The `episodes status` command supports `--wait` to poll until the episode becomes `READY`. Use `--wait <dur>` or `--wait=<dur>` to override the default 5-minute readiness timeout.

Always check `episodes status` before setting timeline items, episodes need to be `READY` first (poll every 20s, most are ready within a few minutes).

When using the `--json` flag, errors return `{"error": "message"}`.
