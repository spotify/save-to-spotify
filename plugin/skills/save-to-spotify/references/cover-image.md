# Cover Image

**Every show & every episode MUST have a cover image.** Never save without `--image`.

**Format:** JPG or PNG, max 1 MB, 1400x1400 square.

## Paths (priority order)

1. **User-provided** — only when user supplies an image file. Skip otherwise. Resize to 1400x1400, apply strong overlay, add typography (unless user opts out).
2. **AI-generated** — default when a known image generation API is available (DALL-E, Stable Diffusion). Never use unvetted services. No overlay (prompt reserves negative space).
3. **CDN artwork** — terminal fallback. No overlay (built-in legibility). Always available, cannot fail.

**Fallthrough:** AI fails → CDN. CDN is the terminal fallback.

### Path 1: User-provided

**Skip this path entirely if the user did not provide an image file.** Do not generate a substitute image — proceed to Path 2.

Accept JPG/PNG at any aspect ratio. Reject if below 600x600 or corrupted. Crop to square, resize to 1400x1400, compress to <1 MB (JPG 90%). Apply strong overlay, then add typography unless user opts out.

**Never:** apply filters, AI enhancement, generate a stand-in image, or override with an agent-generated image.

### Path 2: AI-generated (default)

**Only use known image generation APIs:** DALL-E (OpenAI), Stable Diffusion, or Midjourney. Never use unvetted services (e.g., pollinations.ai) — quality is unreliable and licensing unclear. If no known API key is available, skip to CDN (Path 3).

**Never render text with the model** — composite with Pillow afterwards.

**Prompt pattern:** `"{style} of {concrete subject}, {composition}, {palette}, square composition, negative space in lower third, no text, no logos"`

Example: topic "Weekly Stockholm news briefing" → `"Minimalist illustration of a Stockholm rooftop skyline at dusk, muted blue-grey palette, square composition, negative space in lower third, no text, no logos"`

**Every prompt must include:** a specific concrete subject (not a concept), a composition direction, a palette descriptor, "square composition", "negative space in lower third", "no text, no logos".

**Style:** photorealistic or clean illustration only. No collage, 3D renders, faces, or AI-generated likenesses.

**Never produce:** podcast-meta imagery (mics, headphones), stock cliches (handshakes, lightbulbs), neon/HDR, baked-in text or logos.

**Skip AI if:** topic is abstract, involves real named people, refers to events after the model's training cutoff, or user requested otherwise.

See [timeline.md](timeline.md) for DALL-E / Stable Diffusion code examples.

### Path 3: CDN artwork (terminal fallback)

Pre-designed base artwork with Pillow typography. No overlay needed. 20 variants (`uts-01.png` through `uts-20.png`), selected by hash of show name. Always available, cannot fail.

**CDN endpoint:** `https://save-to-spotify.spotifycdn.com/assets/uts-{01..20}.png`

## Typography

**Mandatory** on every cover (unless user opted out in Path 1). Always composited with Pillow. Never rely on AI text rendering.

**Default copy:** show name only. Add date/episode number only to disambiguate >1 episode per day.

### Constraints

- **One label only.** No subtitles, taglines, or descriptors.
- **Max 3 lines.** If title doesn't fit, shorten: drop articles, use short forms. Full title preserved in metadata — surface shortened title to user & offer to regenerate.
- **No widows.** Don't strand a single short word on its own line.
- **Break on meaning.** Keep concepts together. Pick the split producing the most balanced line widths.

### Font & RTL

| Script | Font | Alignment |
| --- | --- | --- |
| Latin (default) | **Montserrat Bold** | bottom-left |
| Arabic | **Tajawal Bold** | bottom-right |
| Hebrew | **Noto Sans Hebrew Bold** | bottom-right |

All OFL-licensed Google Fonts. Downloaded and cached on first use (`~/.cache/save-to-spotify/fonts/`). Bold or heavier only. Never system defaults or decorative fonts.

**RTL detection:** if any character has `unicodedata.bidirectional(ch) in ('R', 'AL', 'AN')`, use RTL font and right-alignment.

**No reshaper libraries.** Do NOT use `arabic_reshaper` or `python-bidi` — modern fonts handle shaping natively in Pillow.

### Colour & effects

- **White text only.** No accent colours, no exceptions.
- **No text effects.** No drop shadows, strokes, outlines, glows.

## Pillow compositing recipe

Constants and thresholds are authoritative — see code below for exact values.

```python
from PIL import Image, ImageDraw, ImageFont
import os, hashlib, unicodedata, urllib.request

CANVAS = 1400
MARGIN = 64
MAX_TEXT_WIDTH = int((CANVAS - 2 * MARGIN) * 0.85)
MAX_TEXT_HEIGHT = CANVAS - MARGIN - CANVAS // 2  # 636px
MIN_FONT_SIZE = 100
MAX_FONT_SIZE = 400
LEADING_FACTOR = 0.97

FONT_CACHE = os.path.join(os.path.expanduser("~"), ".cache", "save-to-spotify", "fonts")
FONTS = {
    "latin":  ("Montserrat-Bold.ttf",       "https://raw.githubusercontent.com/JulietaUla/Montserrat/master/fonts/ttf/Montserrat-Bold.ttf"),
    "arabic": ("Tajawal-Bold.ttf",           "https://raw.githubusercontent.com/google/fonts/main/ofl/tajawal/Tajawal-Bold.ttf"),
    "hebrew": ("NotoSansHebrew-Bold.ttf",    "https://raw.githubusercontent.com/google/fonts/main/ofl/notosanshebrew/NotoSansHebrew-Bold.ttf"),
}

def detect_script(title):
    for ch in title:
        if '؀' <= ch <= 'ۿ' or 'ݐ' <= ch <= 'ݿ': return "arabic"
        if '֐' <= ch <= '׿': return "hebrew"
    return "latin"

def load_font(size, title=""):
    os.makedirs(FONT_CACHE, exist_ok=True)
    fname, url = FONTS[detect_script(title)]
    path = os.path.join(FONT_CACHE, fname)
    if not os.path.exists(path):
        urllib.request.urlretrieve(url, path)
    return ImageFont.truetype(path, size)

def measure_line(font, text):
    if not text: return (0, 0)
    b = font.getbbox(text)
    return b[2] - b[0], b[3] - b[1]

def _split_combos(words, n):
    if n == 1: yield [words]; return
    for i in range(1, len(words) - n + 2):
        for rest in _split_combos(words[i:], n - 1):
            yield [words[:i]] + rest

def break_lines(title, font):
    words = title.split()
    if not words: return [title]
    best, best_d = None, float("inf")
    for n in range(1, min(len(words), 3) + 1):
        for combo in _split_combos(words, n):
            lines = [" ".join(p) for p in combo if p]
            if not lines: continue
            ws = [measure_line(font, l)[0] for l in lines]
            if max(ws) > MAX_TEXT_WIDTH: continue
            d = max(ws) - min(ws)
            if d < best_d: best_d, best = d, lines
    return best or [title]

def fit_title(title):
    if not title: title = "Untitled"
    for sz in range(MAX_FONT_SIZE, MIN_FONT_SIZE - 1, -2):
        font = load_font(sz, title)
        lines = break_lines(title, font)
        if len(lines) > 3: continue
        if max(measure_line(font, l)[0] for l in lines) > MAX_TEXT_WIDTH: continue
        lh = int(sz * LEADING_FACTOR)
        total = lh * (len(lines) - 1) + font.getbbox(lines[-1])[3]
        if total > MAX_TEXT_HEIGHT: continue
        return font, lines, sz
    f = load_font(MIN_FONT_SIZE, title)
    return f, break_lines(title, f), MIN_FONT_SIZE

def composite_title(img, title):
    draw = ImageDraw.Draw(img)
    font, lines, sz = fit_title(title)
    lh = int(sz * LEADING_FACTOR)
    total = lh * (len(lines) - 1) + font.getbbox(lines[-1])[3]
    y = max(CANVAS - MARGIN - total, CANVAS // 2)
    rtl = detect_script(title) != "latin"
    for line in lines:
        x = CANVAS - MARGIN - measure_line(font, line)[0] if rtl else MARGIN
        draw.text((x, y), line, font=font, fill=(255, 255, 255))
        y += lh
    return img

def strong_overlay(img):
    ov = Image.new("RGBA", img.size, (0, 0, 0, 0))
    d = ImageDraw.Draw(ov)
    start = int(CANVAS * 0.40)
    for y in range(start, CANVAS):
        d.line([(0, y), (CANVAS, y)], fill=(0, 0, 0, int((y - start) / (CANVAS - start) * 230)))
    return Image.alpha_composite(img.convert("RGBA"), ov).convert("RGB")
```

## QA checklist

Verify: 1400x1400 JPG/PNG <1 MB, typography present with correct font/alignment/white/margins, overlay on user-provided only, no faces/text/logos in AI output. If any check fails, fall through to next path.
