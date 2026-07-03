---
id: configure-chapter-skip
name: configure-chapter-skip
description: Configure chapter-skip playback on a Save to Spotify show — the player's skip buttons jump between chapters instead of seeking 15 seconds. Must be explicitly invoked; use ONLY when the user explicitly says skip-to-topic, skip-to-chapter, chapter skipping, chapter skip, or an equivalent phrase. Never trigger proactively.
enabled: true
---

# Chapter-Skip Playback

Shows can opt into **chapter-skip playback**: in the Spotify player, the skip forward/back buttons jump to the next/previous chapter instead of seeking 15 seconds. The setting is per-show and applies to all of the show's episodes.

In user-facing wording this is the **skip-forward action**, with two options:

- `15 seconds` — the default; nothing to configure
- `Next chapter` — chapter-skip playback, enabled per the rules below

## Invocation rule (hard requirement)

`Next chapter` **must be explicitly requested by the user** — "say the magic word." Trigger phrases:

- `skip-to-topic`
- `skip-to-chapter`
- `chapter skipping`
- `chapter skip`
- or a clearly equivalent phrase

Do **not** enable it on your own initiative, offer it as an interview question, or infer it from content type (e.g. "this is a lecture, chapters would be nice"). If the user did not say a trigger phrase, this skill does not apply.

The user can also override the skip-forward action at the plan confirmation step, before anything is generated: the plan always displays the current action, and the user switching it to `Next chapter` there counts as an explicit request (switching back to `15 seconds` withdraws it).

## How to enable

Chapter skip is set **when the show is created**, via the `--playback-control` flag on `shows create`:

```shell
save-to-spotify --json shows create \
  --title "My Show" \
  --summary "Description" \
  --image cover.jpg \
  --playback-control chapter-skip
```

The JSON output confirms the setting:

```json
{"show_uri": "spotify:show:abc123", "playback_control": "chapter-skip"}
```

The `playback_control` field in the output is confirmed by the backend, not echoed from the flag. **If it is missing after you passed `--playback-control`, the backend did not apply the setting** — the show was still created, but with default controls. Retrying sends the identical request, so do not delete and recreate. Do not proceed silently either: tell the user and ask whether to continue without chapter skip or stop and report the failure.

When producing an episode for a chapter-skip show, create the show first with the flag above, then upload with `--show-id` — do **not** use `upload --new-show` (it cannot set playback control):

```shell
SHOW_URI=$(save-to-spotify --json shows create --title "My Show" --image cover.jpg --playback-control chapter-skip | jq -r .show_uri)
save-to-spotify --json upload episode.mp3 --title "Episode 1" --image ep-cover.jpg --show-id "$SHOW_URI"
```

## Requirements and constraints

- **Chapters are required for this to be useful.** Episodes in the show must have timeline chapters (`timeline set`) — otherwise there is nothing to skip to. Follow the main `save-to-spotify` skill's timeline pipeline; never skimp on chapters for a chapter-skip show.
- **Creation-time only.** Show metadata is immutable, so the setting cannot be added to or removed from an existing show. If the user wants it on an existing show, the show must be deleted and recreated — **this deletes all its episodes**, so confirm with the user before doing it.
- **Confirm visibly.** State the skip-forward action in the final summary after the episode is ready (the plan confirmation already displays it).
