# Timeline Format

Reference for building `timeline.json` — the chapter markers and in-player companion content (images, external source links, Spotify entity cards) that ride alongside every episode.

## Save the timeline

After saving, push a single `timeline.json` that carries chapters and all companions in one call:

```shell
save-to-spotify --json timeline set \
  --episode-id <EPISODE_URI> \
  --from-file timeline.json
```

Verify: `save-to-spotify --json timeline get <EPISODE_ID>`. Delete: `save-to-spotify --json timeline delete <EPISODE_ID>`.

## Verifying a pushed timeline

Three backend behaviors make a successful `timeline set` look like a failure. Expect all three:

1. **`timeline set` returns an empty body on success.** The response is `{"items":null}`, not an echo of what was stored. Do not infer from this response whether companions landed — call `timeline get` to check.
2. **Companions propagate slower than chapters.** `episodes status = READY` only confirms the audio is processable. Chapter markers appear shortly after, but `link`, `image`, and `spotify_entity` companions can take an additional 60-90 seconds. A `timeline get` immediately after `READY` may show chapters without their companions — that's propagation lag, not data loss. Wait 60-90 seconds past `READY` before fetching.
3. **`timeline get` does not echo `url` fields.** For image and link companions, the CLI response omits `url` and keeps the opaque `"companion_uri": "time-synced:companion-external-link:<hash>"`. The URL is stored on the backend and clients tap through to it correctly - it is just not echoed back on read. A non-empty `companion_uri` at the expected timestamp is the proof the link is live. Do not diff the fetched URL against what you sent.

If links or images are genuinely missing after a 2-3 minute wait, re-push `timeline set` — the endpoint is idempotent and replaces the full timeline on PUT.

## Data model

Each entry in `items` is one of four kinds: `chapter`, `image`, `link`, or `spotify_entity`. A single time window (say, one chapter's span) can contain *multiple* companion items; they only have to not overlap in time with each other. Chapters are independent and do not overlap with companions.

```json
{
  "items": [
    {"chapter": {"title": "Course intro & syllabus", "start_time_ms": 0}},
    {"chapter": {"title": "Lecture 3: Backpropagation", "start_time_ms": 18000}},
    {"image":   {"start_time_ms": 25000, "duration_ms": 15000, "image": "img_01_a.jpg", "url": "https://example.edu/cs231n/lecture3", "title": "Slide: chain rule diagram"}},
    {"spotify_entity": {"start_time_ms": 45000, "duration_ms": 20000, "uri": "spotify:episode:2abc3def4ghi"}},
    {"image":   {"start_time_ms": 70000, "duration_ms": 20000, "image": "img_01_b.jpg", "title": "Slide: computation graph"}},
    {"chapter": {"title": "Worked example: 2-layer net", "start_time_ms": 102000}},
    {"spotify_entity": {"start_time_ms": 110000, "uri": "spotify:track:4uLU6hMCjMI75M1A2tKUQC"}},
    {"link":    {"start_time_ms": 130000, "duration_ms": 25000, "url": "https://example.edu/readings/goodfellow-ch6.pdf"}},
    {"image":   {"start_time_ms": 160000, "duration_ms": 20000, "image": "img_02_a.jpg"}},
    {"chapter": {"title": "Recap & problem set", "start_time_ms": 270000}}
  ]
}
```

## Validation rules

Rules are checked both client-side and on the backend:

- **Chapters:** at least 2, first at `0 ms`, strictly increasing `start_time_ms` with consecutive starts ≥ 5 s apart (the final chapter may be shorter), `title` required, `description` optional. Every `start_time_ms` must be strictly less than the episode's audio duration — the CLI and backend can't verify this, so compute timestamps from cumulative segment durations and assert against the assembled MP3 before `timeline set` (see [audio-providers.md](audio-providers.md) "Timeline timestamp calculation").
- **Images:** positive `duration_ms`, local file path in `image` (`.jpg`/`.png`, ≤ 1 MB, dimensions 1×1..4096×4096). Optional `url` (tap-through) and `title` (alt text). When an image is tied to one canonical source URL, default to setting that URL here.
- **Links:** positive `duration_ms`, valid HTTP(S) `url`.
- **Spotify entities:** `uri` is required and must be a full `spotify:...` URI. `duration_ms` is optional, but when present it must be positive. Use this for Spotify-native references such as tracks, albums, artists, playlists, shows, episodes, and audiobook/catalog entities.
- **Companion non-overlap:** sort images, links, and `spotify_entity` items by `start_time_ms`; each item's `start + duration` must be ≤ the next item's `start`. A `spotify_entity` without `duration_ms` behaves like an instantaneous card at that timestamp. Chapters are *not* included in this check — a chapter can start at any time, including inside a companion's window.
- **URI format:** in `timeline.json`, `spotify_entity.uri` must be a full Spotify URI (`spotify:track:...`, `spotify:artist:...`, `spotify:episode:...`). Do not use bare IDs or `open.spotify.com` URLs there.
- Titles on chapters should match the description timestamps (they land in the show-notes HTML).

## Companion selection rules

**Preference order:** if the thing you want listeners to open already exists on Spotify, add a `spotify_entity`. Keep `link` for off-Spotify destinations, and include both when you want listeners to have both the Spotify destination and the original source/article.

**No duplicate artwork:** when a `spotify_entity` is present, do NOT add an `image` of the same entity's artwork — the card already renders it. Only pair an `image` with a `spotify_entity` when the image is editorially distinct (chart, infographic, screenshot).

**Image + link default:** if you have a representative image plus one canonical source URL for the same story, prefer one `image` item with `url` set. Use standalone `link` for additional URLs, or when there is no good image. Use standalone `image` only when there is no meaningful destination.

## Placing companions inside a chapter

For a chapter spanning `[chapter_start, chapter_end)`, place N companions by dividing the window into N equal slots (or picking natural anchor points in the script). Keep a short buffer (e.g., 1 second) between companions to avoid edge-case overlaps. A practical helper is in [audio-providers.md](audio-providers.md) under "Timeline timestamp calculation".

## Companion images: sourced, AI-generated, or mixed

The agent asked the user in the interview where companion images should come from. Follow that choice consistently across the episode:

- **AI-generated** -- after scripting, generate images from a themed prompt derived from each segment's content. Use the DALL-E / Stable Diffusion code under "Batch generation for timeline companions" below. Use the same naming pattern as above.
- **Mixed** -- sourced where a usable image exists, AI-generated fill everywhere else. Aim for at least one image per chapter.
- **Skip** -- emit a timeline with chapters plus Spotify and external-link companions only.

If the mode is `Sourced` or `Mixed`, attempting image extraction is required during sourcing. When a usable source image is found, include it in the timeline unless the user explicitly opted out later. Only omit an image companion when the source genuinely has no meaningful visual, or the user chose `Skip`.

The CLI's `timeline set` uploads each local image file to Spotify's image store, swaps the file path for the returned upload token, and sends the timeline to the backend. No separate image-upload step is needed.

## AI image generation

When the user picks `AI-generated` or `Mixed`, generate images with their preferred model. Constraints for timeline companions: JPEG/PNG, up to 4096x4096, max 1 MB each.

### OpenAI DALL-E

```python
from openai import OpenAI
client = OpenAI()
response = client.images.generate(
    model="dall-e-3",
    prompt="A clean, minimal illustration of distributed systems architecture, dark background, tech aesthetic",
    size="1024x1024",
    quality="standard",
    n=1,
)
import urllib.request
urllib.request.urlretrieve(response.data[0].url, "img.png")
```

### Stable Diffusion (local)

```shell
python3 -c "
from diffusers import StableDiffusionPipeline
pipe = StableDiffusionPipeline.from_pretrained('stabilityai/stable-diffusion-xl-base-1.0')
image = pipe('A clean illustration of neural networks, minimal, dark background').images[0]
image.save('img.png')
"
```

### Resizing with ffmpeg

```shell
# Square companion under 1 MB, within 4096x4096
ffmpeg -i raw.png -vf "scale=1400:1400:force_original_aspect_ratio=decrease" -q:v 3 img_01_a.jpg
```

### Batch generation for timeline companions

Generate one or more images per chapter from a themed prompt. File naming must match what the timeline builder expects: `img_<chapter_index>_<slot>.jpg` (slots `a`, `b`, `c` for multiple images inside one chapter).

```python
from openai import OpenAI
import urllib.request, subprocess, string

client = OpenAI()

# One entry per chapter; each item lists the visual prompts for that chapter's slots
chapter_images = [
    ("Introduction",             []),  # no companions for intro
    ("Chapter A",                [
        "Neutral map of Eastern Europe, minimal, muted colors",
        "Wide shot of a damaged street, documentary photo, no text",
    ]),
    ("Chapter B",                [
        "Empty supermarket shelves, documentary photo, overcast light",
    ]),
    ("Sign-off",                 []),
]

for idx, (_, prompts) in enumerate(chapter_images):
    for slot_letter, prompt in zip(string.ascii_lowercase, prompts):
        resp = client.images.generate(model="dall-e-3", prompt=prompt,
                                      size="1024x1024", quality="standard", n=1)
        tmp = f"/tmp/raw_{idx}_{slot_letter}.png"
        urllib.request.urlretrieve(resp.data[0].url, tmp)
        out = f"img_{idx:02d}_{slot_letter}.jpg"
        # Downsize + re-encode to guarantee <= 1 MB, <= 4096x4096, .jpg
        subprocess.check_call([
            "ffmpeg", "-y", "-i", tmp,
            "-vf", "scale=1400:1400:force_original_aspect_ratio=decrease",
            "-q:v", "3", out,
        ])
```

For Stable Diffusion, swap the `client.images.generate` call for `pipe(prompt).images[0].save(tmp)` from the SDXL example above. Keep the same file-naming convention so the timeline builder in [audio-providers.md](audio-providers.md) picks the files up automatically.
