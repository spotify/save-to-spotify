# Audio Providers & Assembly

Reference for generating speech and assembling audio files for saving via `save-to-spotify`. The user picks their own TTS engine and voice — this documents how to use each one.

Tell the user:

> The skill produces audio content that may be distributed via a streaming platform. 
> Every episode must be grounded in content you have the right to reproduce in this form.

## Production pipeline

Every episode walks the same steps. Recipes define what to write (sourcing, scripting, segment map). This reference covers generation and assembly.

1. Generate TTS audio per segment (one file each for exact chapter timing)
2. Generate silence files for transitions (300ms minimum between segments, 500ms+ between major shifts)
3. Concatenate all segments into a single MP3
4. Normalize volume levels
5. Calculate chapter timestamps from cumulative segment durations
6. Build `timeline.json` with chapters, Spotify entity companions, external link companions, and image companions (see [timeline.md](timeline.md))

**Accepted formats:** `.mp3`, `.m4a`, `.wav`, `.ogg` (max 1 GB). Default to `.mp3`. Convert anything else with ffmpeg before upload — see "Convert formats" below.

### Voice selection guide

- **Kokoro** (local, free): `af_alloy` (American female, recommended), `am_adam` (American male), `bf_emma` (British female), `bm_george` (British male)
- **ElevenLabs** (high quality, paid): Amelia, George, Bella
- **Edge TTS** (free, 300+ voices): `en-US-AriaNeural` (F), `en-US-GuyNeural` (M)
- **OpenAI TTS** (high quality, paid): `nova`, `alloy`, `echo`, `onyx`

### Multi-voice / bilingual segments

Default to one voice per episode, but when the content is bilingual, role-played, or otherwise needs multiple voices, use a part-based segment schema and a flattened render manifest.

Example source schema:

```json
{
  "segments": [
    {
      "title": "Swedish intro",
      "parts": [
        {"lang": "sv", "voice": "sv-SE-SofieNeural", "text": "Hej och valkommen."},
        {"lang": "en", "voice": "en-GB-LibbyNeural", "text": "Welcome back to the show."}
      ]
    }
  ]
}
```

Flatten that into a render manifest before synthesis:

```json
[
  {"segment_index": 0, "part_index": 0, "lang": "sv", "voice": "sv-SE-SofieNeural", "text": "Hej och valkommen.", "out": "seg_00_00.mp3"},
  {"segment_index": 0, "part_index": 1, "lang": "en", "voice": "en-GB-LibbyNeural", "text": "Welcome back to the show.", "out": "seg_00_01.mp3"}
]
```

Concatenate manifest outputs in order, then compute chapter timestamps from the flattened audio segments.

## Provider selection

When a content creation recipe is triggered and no TTS provider has been established, ask the user:

> What TTS (text-to-speech) tool would you like me to use?
>
> - **macOS `say`** -- built-in, no setup, limited voices
> - **Edge TTS** (`edge-tts`) -- free, 300+ voices, 70+ languages
> - **OpenAI TTS** -- high quality, paid
> - **ElevenLabs** -- most natural, paid
> - **Google Cloud TTS** -- high quality, paid
> - **Piper** -- offline, fast, open-source
> - **gTTS** -- free, Google Translate quality
> - **Amazon Polly** -- cloud, paid
> - Something else?
>
> Which voice do you prefer? (I can list available voices.)

### Verification

Before generating, verify the chosen provider is available:

```shell
which say          # macOS (always available)
which edge-tts     # pip install edge-tts
which piper        # separate install
python3 -c "import openai"      # OpenAI
python3 -c "import elevenlabs"  # ElevenLabs

# ffmpeg is required for assembly
which ffmpeg && which ffprobe
```

If not installed, offer to install or suggest an alternative.

## TTS provider reference

### macOS `say`

```shell
say --voice '?'                                          # List voices
say -v <Voice> -o output.m4a --data-format=aac "Text"    # Generate m4a
say -v <Voice> -o output.m4a --data-format=aac -f in.txt # From file
```

Voices: `Samantha` (en-US), `Daniel` (en-GB), `Alex` (en-US high quality). Limited languages.

### Edge TTS (recommended free option)

```shell
edge-tts --list-voices                                                  # List all
edge-tts --voice "en-US-AriaNeural" --text "Hello" --write-media o.mp3  # Generate
edge-tts --voice "en-US-GuyNeural" -f input.txt --write-media o.mp3     # From file
edge-tts --voice "en-US-AriaNeural" --rate="+10%" --text "Fast" --write-media o.mp3  # Faster
edge-tts --voice "en-US-AriaNeural" --rate="-30%" --text "Slow" --write-media o.mp3  # Slower
```

Key voices: `en-US-AriaNeural` (F), `en-US-GuyNeural` (M), `en-GB-SoniaNeural` (F), `en-GB-RyanNeural` (M), `es-ES-ElviraNeural`, `fr-FR-DeniseNeural`, `de-DE-KatjaNeural`, `ja-JP-NanamiNeural`, `zh-CN-XiaoxiaoNeural`.

### OpenAI TTS

```python
from openai import OpenAI
client = OpenAI()
resp = client.audio.speech.create(model="tts-1", voice="nova", input="Text")
resp.stream_to_file("output.mp3")
```

Voices: `alloy`, `echo`, `fable`, `onyx`, `nova`, `shimmer`. Use `tts-1-hd` for higher quality.

### ElevenLabs

```python
from elevenlabs import generate, save
audio = generate(text="Text", voice="Rachel", model="eleven_multilingual_v2")
save(audio, "output.mp3")
```

Most natural. Supports multiple languages.

### Piper (offline)

```shell
echo "Text" | piper --model en_US-lessac-medium.onnx --output_file output.wav
ffmpeg -i output.wav -codec:a libmp3lame -qscale:a 2 output.mp3  # convert
```

### Kokoro (local, free, no API limits)

```python
from kokoro_onnx import Kokoro
import soundfile as sf
import numpy as np
import json

kokoro = Kokoro('kokoro-v1.0.onnx', 'voices-v1.0.bin')

segments = [
    ("Introduction", intro_text),
    ("Main Topic", body_text),
    ("Sign-off", outro_text),
]

timeline_items = []
all_samples = []
cursor_ms = 0

for title, text in segments:
    samples, sr = kokoro.create(text, voice='af_alloy', speed=1.0)
    timeline_items.append({"chapter": {"title": title, "start_time_ms": cursor_ms}})
    all_samples.append(samples)
    # 300ms silence between segments
    silence = np.zeros(int(sr * 0.3))
    all_samples.append(silence)
    cursor_ms += int((len(samples) + len(silence)) / sr * 1000)

combined = np.concatenate(all_samples)
sf.write('episode.wav', combined, sr)

with open('timeline.json', 'w') as f:
    json.dump({"items": timeline_items}, f, indent=2)
```

Convert to MP3: `ffmpeg -i episode.wav -codec:a libmp3lame -b:a 192k episode.mp3`

Voices: `af_alloy` (American female, recommended), `am_adam` (American male), `bf_emma` (British female), `bm_george` (British male). Fully offline, no API limits. Generates audio + chapter timestamps in one pass.

### gTTS (free, basic)

```shell
gtts-cli "Text to speak" --lang en --output output.mp3
gtts-cli -f input.txt --lang en --output output.mp3
```

### Google Cloud TTS

```python
from google.cloud import texttospeech
client = texttospeech.TextToSpeechClient()
input_text = texttospeech.SynthesisInput(text="Text")
voice = texttospeech.VoiceSelectionParams(language_code="en-US", name="en-US-Neural2-F")
config = texttospeech.AudioConfig(audio_encoding=texttospeech.AudioEncoding.MP3)
resp = client.synthesize_speech(input=input_text, voice=voice, audio_config=config)
with open("output.mp3", "wb") as f:
    f.write(resp.audio_content)
```

## Text sanitization for TTS

Before sending text to any TTS engine, clean it:

- Strip markdown: `**bold**` -> `bold`, `# heading` -> `heading`
- Remove hashtags, emojis, and non-speech artifacts
- Expand abbreviations that sound wrong when spoken aloud
- Replace em dashes `—` with hyphens `-` to avoid encoding issues in shell commands
- Remove URLs from the spoken text (mention them as "link in the description" instead)

## Audio assembly with ffmpeg

All content recipes generate multiple segments that must be joined into a single file; the segment boundaries become chapters in the timeline.

### Generate silence

```shell
# 1.5 seconds (transitions between sections)
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 1.5 -q:a 9 -acodec libmp3lame /tmp/silence_1.5s.mp3

# 3 seconds (recall pauses for language drills)
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 3 -q:a 9 -acodec libmp3lame /tmp/silence_3s.mp3

# 5 seconds (speaking practice pauses)
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 5 -q:a 9 -acodec libmp3lame /tmp/silence_5s.mp3
```

### Concatenate segments

```shell
# Build a file list (order matters)
cat > /tmp/segments.txt << 'EOF'
file 'segment_01.mp3'
file 'silence_1.5s.mp3'
file 'segment_02.mp3'
file 'silence_1.5s.mp3'
file 'segment_03.mp3'
EOF

# Concatenate — re-encode rather than `-c copy`. Stream-copying small MP3
# segments yields non-monotonic frame timestamps that `loudnorm` silently
# drops audio on. The explicit `-ar 44100 -ac 1` also normalizes any stray
# stereo/48kHz segment to the common format so concat doesn't break.
ffmpeg -f concat -safe 0 -i /tmp/segments.txt -ar 44100 -ac 1 -c:a libmp3lame -b:a 192k /tmp/output.mp3
```

### Normalize volume

Always normalize after concatenation — different TTS segments may have different levels:

```shell
ffmpeg -i /tmp/output.mp3 -af loudnorm /tmp/output_normalized.mp3
```

### Convert formats

If the TTS outputs a format other than `.mp3`, `.m4a`, `.wav`, or `.ogg`, convert before upload:

```shell
ffmpeg -i input.aiff -codec:a libmp3lame -qscale:a 2 output.mp3
ffmpeg -i input.webm -codec:a libmp3lame -qscale:a 2 output.mp3
```

### Get segment duration (for timeline timestamps)

```shell
ffprobe -v error -show_entries format=duration -of csv=p=0 segment.mp3
```

### Timeline timestamp calculation

After generating all segments, build `timeline.json` by walking the file list, summing durations for chapter start times, and placing image/link companions inside each chapter's window. The only backend rule for companions is that they do not overlap with each other — chapter windows are independent.

```python
import json, subprocess
from pathlib import Path

def ms(path):
    out = subprocess.check_output(
        ['ffprobe', '-v', 'error', '-show_entries', 'format=duration',
         '-of', 'csv=p=0', str(path)], text=True).strip()
    return int(float(out) * 1000)

# Each segment: (chapter_title, audio_file, companions)
# companions is a list of dicts: {"image": "img_01_a.jpg", "url": "...", "title": "..."}
#                            or: {"link":  "https://..."}
segments = [
    ("Introduction",   "segment_01.mp3", []),
    ("Chapter A",      "segment_02.mp3", [
        {"image": "img_02_a.jpg", "url": "https://example.com/article-1", "title": "Source site"},
        {"link":  "https://example.org/statement"},
    ]),
    ("Chapter B",      "segment_03.mp3", [
        {"image": "img_03_a.jpg"},
        {"link":  "https://example.com/article-2"},
    ]),
    ("Sign-off",       "segment_04.mp3", []),
]
silence_ms = ms('silence_1.5s.mp3')

items = []
cursor = 0
for title, audio, companions in segments:
    items.append({"chapter": {"title": title, "start_time_ms": cursor}})
    dur = ms(audio)
    # Distribute companions evenly inside the chapter window, with a 500 ms buffer.
    if companions:
        usable = dur - 500 * (len(companions) + 1)
        slot = max(usable // len(companions), 1000)
        for i, c in enumerate(companions):
            start = cursor + 500 + i * (slot + 500)
            duration = slot
            if "image" in c:
                item = {"image": {"start_time_ms": start, "duration_ms": duration, "image": c["image"]}}
                if c.get("url"):   item["image"]["url"] = c["url"]
                if c.get("title"): item["image"]["title"] = c["title"]
                items.append(item)
            elif "link" in c:
                items.append({"link": {"start_time_ms": start, "duration_ms": duration, "url": c["link"]}})
    cursor += dur + silence_ms

# Every chapter's start_time_ms must be strictly less than the assembled
# audio duration; nothing downstream can verify this.
final_ms = ms('episode.mp3')
last_chapter_ms = max(it["chapter"]["start_time_ms"] for it in items if "chapter" in it)
assert last_chapter_ms < final_ms, (
    f"last chapter at {last_chapter_ms} ms >= episode duration {final_ms} ms"
)

Path('timeline.json').write_text(json.dumps({"items": items}, indent=2))
```

The agent passes `timeline.json` to `save-to-spotify --json timeline set --episode-id <EP_ID> --from-file timeline.json`. The CLI uploads each local image file to Spotify's image store and swaps the path for the returned upload token before PUT-ing the timeline.

## Standard workflow (used by all recipes)

```
1. User provides: topic + sources + voice preferences + companion-image source (sourced / AI-generated / mixed / skip)
2. Agent writes: structured script broken into segments, with source URL(s) and image hint per segment
3. Agent generates: one audio file per segment via TTS provider
4. Agent generates: silence files for pauses/transitions
5. Agent assembles: concatenate segments into single .mp3
6. Agent normalizes: volume levels
7. Agent gathers companion images: download from source and/or generate with DALL-E/SD
8. Agent calculates: chapter timestamps from segment durations
9. Agent builds: timeline.json with chapters + link companions (source URLs) + image companions
10. Agent saves: save-to-spotify --json upload ...
11. Agent sets timeline: save-to-spotify --json timeline set ...
12. Agent polls: episodes status until READY
```

Recipes define steps 1-2 (what to write, which URLs and images to gather). This reference covers steps 3-9. The main SKILL.md covers steps 10-12.
