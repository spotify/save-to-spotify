# Local Preview Pages

Local browser preview pages, served before anything is uploaded: the **finished episode** (a mini Spotify player) and the **voice sample** (a one-card player). Nothing leaves the machine — no uploads, no external requests. Use them from any flow — onboarding or the standard checklist.

## Assets

Create a preview directory in the session temp dir: `preview/<short-id>/` containing `index.html`. Reference the episode MP3 and cover image in place — symlink them into the directory (`ln -s`, macOS/Linux) or serve the directory where they already live. On Windows, skip symlinks (they need elevated privileges): serve from where the assets live, or copy them. Do not copy multi-megabyte audio on other platforms just to preview it.

## The page

`index.html` is one self-contained file (inline CSS/JS, no CDN or external fetches) that mirrors the Spotify player look:

- Dark theme (near-black `#121212` background, Spotify green `#1DB954` accent, white/grey text)
- Two panes. Left: show name as a small uppercase eyebrow, episode title, duration, description, a thin timeline scrubber with a tick mark per chapter, then the full chapter list — each row an icon, chapter title, and `m:ss` start time from `timeline.json`. Right: the cover image (fallback: a green gradient placeholder with a music-note glyph).
- A round play/pause button wired to an `<audio>` element. The button must correctly reflect and control audio state:
  - Show a **play icon** (▶) when audio is paused/stopped, a **pause icon** (⏸) when playing. Never show both; never get stuck on one.
  - Wire `onclick` to toggle `audio.paused ? audio.play() : audio.pause()`.
  - Listen to the audio element's `play`, `pause`, and `ended` events to update the icon — do NOT rely only on the click handler, because seeks, chapter clicks, and browser controls also change state.
  - On `ended`, reset to the play icon.
  - Clicking a chapter row seeks to its `start_time_ms` **and starts playback** (call `audio.play()` after setting `currentTime`). The event listeners handle the icon update.
  - The currently playing chapter is highlighted in green — update the highlight on `timeupdate` by comparing `audio.currentTime` against chapter start times.
- **Load the audio as a blob, not a URL.** Simple static servers (`python3 -m http.server`) don't support HTTP Range requests, so an `<audio>` pointed straight at the MP3 URL cannot seek — chapter jumps and progress-bar clicks silently fail. Instead `fetch()` the MP3, convert with `.blob()`, and set `URL.createObjectURL(blob)` as the audio `src` (keep the object URL for repeat plays). With the whole file in memory the browser seeks freely without Range support.

## Serve

Start the server as soon as there is anything to preview — the voice sample at the voice-pick step, or the episode assets once they exist. It's cheap and makes previews instant. Bind to localhost only, and do **not** auto-open the browser:

```shell
python3 -m http.server 8374 --bind 127.0.0.1 --directory <session-temp-dir> &
```

On Windows use `python` instead of `python3`, and note the trailing `&` is bash-only — in Git Bash it works as shown; in PowerShell use `Start-Process python -ArgumentList '-m','http.server','8374','--bind','127.0.0.1','--directory','<session-temp-dir>'`. If port 8374 is taken, pick another free port. Fall back to opening `index.html` directly if Python is unavailable.

## Offer, open, verdict

Say: "Your episode is ready — a local preview is up at http://localhost:8374/preview/<short-id> (nothing uploaded yet)." with choices:

- **Open the preview** — open the URL in the browser (`open` on macOS, `xdg-open` on Linux, `start` on Windows), then ask the verdict below
- **Skip preview, save to Spotify** — stop the server, save now

Flows may add their own choices to this prompt (onboarding adds "Make it shorter" and "Change the voice").

Only open the browser when the user asks. Once the page is open, ask: "All good?" with choices:

- **All good, save to Spotify** — stop the server, proceed to save
- **Make changes** — see below

When delivering the episode, apply the "The user made this" principle from SKILL.md: emphasise what the user has created — "your episode is ready", never "we created your episode".

## Voice preview page

The voice sample from `tts test` gets the same treatment as the episode — a player page, not a raw `file://` path. Put a minimal one-card page at `preview/voice/index.html` on the same server (start the server at this point if it isn't running; it stays up through the episode preview):

- Centered dark card: "VOICE PREVIEW" eyebrow, the voice name as the title, engine + a couple of voice traits as the subtitle
- A round green play/pause button and a thin scrubber, wired to the sample (symlinked next to the page); blob-load it like the episode audio. Same event-driven state sync as the episode player: listen to `play`, `pause`, `ended` events on the audio element to toggle the icon — never rely on click alone.
- No auto-play — the user clicks play

Offer to open `http://localhost:8374/preview/voice/` the same way as the episode preview: open on request, never unprompted.

## Making changes from the preview

Distinguish what the edit actually touches:

- **Metadata edits** (title, description, chapter titles) — update the preview HTML in place. No audio regeneration.
- **Script changes** — go back through the flow's content-approval step before regenerating any audio, then rebuild the affected assets and refresh the preview.

## Lifecycle

While the user iterates, keep the server running and rebuild the page and assets in place — the URL stays stable, no port churn. Stop the server before saving, and whenever the flow exits — never leave it running.
